package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"ticket/internal/domain"
)

// SetStatus mutates ticket n to next, appending a journal entry. The
// filesystem sequence (under the per-store advisory lock) is:
//
//  1. Lstat(target). Existence => *CollisionError (carries Target).
//  2. Write the rendered body to a same-directory .tmp file; Sync, Close.
//  3. Link the .tmp to target — an atomic no-replace commit. If the
//     target appeared since the Lstat fast-path (an out-of-lock agent),
//     linkFile fails with fs.ErrExist => *CollisionError; any other
//     failure => "store: link commit: %w"; both join removeTmp(tmp) as
//     cleanup. After a
//     successful link tmp and target share one inode, so the tmp name
//     is removed via the removeTmp hook (NOT the removeFile hook); a
//     removal failure triggers a rollback (removeFile(target)) plus a
//     best-effort tmp-removal retry; a failed retry is joined with the
//     tmp-removal error.
//  4. os.Remove(old). ENOENT => success: the old file is already gone
//     (e.g. removed by a lockless external agent), so the success
//     invariant — ONLY the new file remains — already holds; rolling
//     back would delete the only remaining copy. Any other failure =>
//     rollback (os.Remove(target)); failures =>
//     errors.Join(removeOldErr, rollbackErr, breach).
//
// A same-status set with a non-empty comment takes the journal-only
// path of appendSameStatusLocked instead: the file name and directory
// never change, one From==To journal entry is appended, and the commit
// is a rename-over through the renameFile hook (see there for the
// POSIX/Windows semantics and the lost-update-window caveat).
// Same-status without a comment stays the "already in status" error.
//
// On success the on-disk state contains ONLY the new file. The link
// commit is atomic no-replace on POSIX (link(2) fails with EEXIST) and
// on Windows (CreateHardLink fails with ERROR_ALREADY_EXISTS, mapped by
// Go to fs.ErrExist). Windows caveat: hard links require NTFS; on
// FAT/exFAT volumes the commit fails loudly rather than risking data
// loss.
//
// On success the full path of the new file is returned: an archived
// ticket kept at done/closed stays under archive/, so the caller cannot
// derive the path from s.Dir alone.
//
// Mutations on a read-only Store (NewReadOnly) are rejected by withLock
// with ErrReadOnly.
func (s *Store) SetStatus(n int, next domain.Status, who, comment string) (string, error) {
	var target string
	err := s.withLock(func() error {
		var serr error
		target, serr = s.setStatusLocked(n, next, who, comment)
		return serr
	})
	if err != nil {
		return "", err
	}
	return target, nil
}

// setStatusLocked assumes the per-store advisory lock is held. It
// returns the full path of the committed new file.
func (s *Store) setStatusLocked(n int, next domain.Status, who, comment string) (string, error) {
	cur, curDir, err := s.findLocked(n)
	if err != nil {
		return "", err
	}
	if cur.Status == next {
		if comment == "" {
			return "", fmt.Errorf("store: ticket %d already in status %s", n, next)
		}
		return s.appendSameStatusLocked(n, cur, curDir, who, comment)
	}

	// Determine target directory: reopen from archive stays in archive,
	// done/closed from main stays in main.
	var targetDir string
	if next == domain.StatusOpen || next == domain.StatusWip {
		targetDir = s.Dir // reopen always goes to main
	} else {
		// done/closed: stay in current directory
		targetDir = curDir
	}

	target := filepath.Join(targetDir, domain.Filename(n, next))
	if _, lerr := os.Lstat(target); lerr == nil {
		return "", &CollisionError{Target: target}
	} else if !errors.Is(lerr, fs.ErrNotExist) {
		return "", fmt.Errorf("store: lstat target: %w", lerr)
	}

	old := filepath.Join(curDir, cur.Name)
	tk, _, unknown, rerr := readTicketFile(old, cur.Number)
	if rerr != nil {
		return "", fmt.Errorf("store: read %s: %w", cur.Name, rerr)
	}
	oldStatus := cur.Status // filename-derived; body status is untrusted (V16)
	tk.Status = next
	tk.Journal = append(tk.Journal, domain.JournalEntry{
		From:    oldStatus,
		To:      next,
		Comment: comment,
		Who:     who,
		At:      time.Now(),
	})

	rendered, rerr := domain.Render(tk, unknown)
	if rerr != nil {
		return "", fmt.Errorf("store: render: %w", rerr)
	}

	tmpPath, werr := s.writeTmp(n, next, rendered, targetDir)
	if werr != nil {
		return "", werr
	}

	if lerr := linkFile(tmpPath, target); lerr != nil {
		// Cleanup may itself fail; join both.
		if errors.Is(lerr, fs.ErrExist) {
			lerr = &CollisionError{Target: target}
		} else {
			lerr = fmt.Errorf("store: link commit: %w", lerr)
		}
		cleanup := removeTmp(tmpPath)
		return "", errors.Join(lerr, cleanup)
	}
	// tmp and target share one inode now; drop the tmp name via the
	// removeTmp hook, NOT the removeFile hook (the swapRemove call-count
	// stays at remove-old + rollback). A removal failure rolls back the
	// committed target (removeFile) and retries the tmp removal
	// best-effort; a failed retry is joined with the tmp-removal error.
	if rerr := removeTmp(tmpPath); rerr != nil {
		// The commit already happened (target holds the new status);
		// roll it back by removing the target.
		tmpErr := fmt.Errorf("store: remove tmp %s: %w", tmpPath, rerr)
		if rbErr := removeFile(target); rbErr != nil {
			return "", errors.Join(
				tmpErr,
				fmt.Errorf("rollback remove %s: %w", target, rbErr),
			)
		}
		// Rollback succeeded: retry the tmp removal best-effort; a
		// failed retry is joined with the original tmp-removal error.
		if retryErr := removeTmp(tmpPath); retryErr != nil {
			tmpErr = errors.Join(tmpErr, retryErr)
		}
		return "", tmpErr
	}

	if rerr := removeFile(old); rerr != nil {
		if errors.Is(rerr, fs.ErrNotExist) {
			// The old file vanished after the successful commit (an
			// out-of-lock agent). The invariant "ONLY the new file
			// remains" already holds, so this is success; a rollback
			// here would delete the only remaining ticket file.
			return target, nil
		}
		// Try to roll back: remove target.
		rbErr := removeFile(target)
		if rbErr != nil {
			return "", errors.Join(
				fmt.Errorf("remove old %s: %w", cur.Name, rerr),
				fmt.Errorf("rollback remove %s: %w", target, rbErr),
				errors.New("invariant breach: old+new coexist"),
			)
		}
		// Rollback succeeded: surface the original error. A single-arg
		// errors.Join would add a pointless wrapper layer (wave-1 #4).
		return "", fmt.Errorf("remove old %s: %w", cur.Name, rerr)
	}
	return target, nil
}

// appendSameStatusLocked journals a same-status set (comment != "") in
// place: the file name and directory never change (target == old), one
// From==To==cur.Status journal entry is appended, and the commit is a
// rename-over via the renameFile hook. The rename-over replaces the
// existing file atomically on POSIX (rename(2)); on Windows
// MoveFileEx(MOVEFILE_REPLACE_EXISTING) replaces it non-atomically, and
// a sharing violation (target held open without FILE_SHARE_DELETE)
// fails the commit with the old file intact and the tmp removed. Any
// rename error is joined with the tmp cleanup; no rollback is needed
// because old == target stays alive. Caveat: a journal append written
// out of lock between readTicketFile and the rename is silently lost —
// the same lost-update window the cross-status path has.
func (s *Store) appendSameStatusLocked(n int, cur fileEntry, curDir, who, comment string) (string, error) {
	old := filepath.Join(curDir, cur.Name)
	tk, _, unknown, err := readTicketFile(old, cur.Number)
	if err != nil {
		return "", fmt.Errorf("store: read %s: %w", cur.Name, err)
	}
	tk.Status = cur.Status
	tk.Journal = append(tk.Journal, domain.JournalEntry{
		From:    cur.Status,
		To:      cur.Status,
		Comment: comment,
		Who:     who,
		At:      time.Now(),
	})
	rendered, err := domain.Render(tk, unknown)
	if err != nil {
		return "", fmt.Errorf("store: render: %w", err)
	}
	tmpPath, err := s.writeTmp(n, cur.Status, rendered, curDir)
	if err != nil {
		return "", err
	}
	if err := renameFile(tmpPath, old); err != nil {
		return "", errors.Join(fmt.Errorf("store: rename commit: %w", err), removeTmp(tmpPath))
	}
	return old, nil
}

// MaxTmpAttempts caps writeTmp's O_EXCL name-collision retries (§5).
const MaxTmpAttempts = 5

// scrubFile best-effort closes f (nil-safe) and removes path, joining the
// cause with every cleanup error so a failed Create/writeTmp leaves no
// partial file behind. Removing an already-absent file is not an error.
func scrubFile(cause error, f *os.File, path string) error {
	errs := []error{cause}
	if f != nil {
		if cerr := f.Close(); cerr != nil {
			errs = append(errs, fmt.Errorf("close %s: %w", path, cerr))
		}
	}
	if rerr := os.Remove(path); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
		errs = append(errs, fmt.Errorf("remove %s: %w", path, rerr))
	}
	return errors.Join(errs...)
}

// writeTmp creates a .tmp file in dir with O_CREATE|O_EXCL, retrying
// name collisions up to MaxTmpAttempts, writes body, syncs, closes, and
// returns the path. dir MUST be the commit target's directory so the
// link commit stays inside one filesystem: s.Dir for main-directory
// mutations, s.Dir/archive for archive commits. On any failure the
// partial tmp file is removed and the joined error returned. Caller owns
// the no-replace link commit and tmp cleanup on success.
func (s *Store) writeTmp(n int, next domain.Status, body []byte, dir string) (string, error) {
	var lastErr error
	for attempt := 0; attempt < MaxTmpAttempts; attempt++ {
		tmpName := fmt.Sprintf(".T-%04d-%s.%d-%d.md.tmp", n, next, os.Getpid(), nanoNow())
		tmpPath := filepath.Join(dir, tmpName)
		tf, terr := os.OpenFile(tmpPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if terr != nil {
			if errors.Is(terr, fs.ErrExist) {
				lastErr = terr
				continue
			}
			return "", fmt.Errorf("store: tmp open %s: %w", tmpName, terr)
		}
		if _, werr := tf.Write(body); werr != nil {
			return "", scrubFile(fmt.Errorf("store: tmp write %s: %w", tmpName, werr), tf, tmpPath)
		}
		if serr := tf.Sync(); serr != nil {
			return "", scrubFile(fmt.Errorf("store: tmp sync %s: %w", tmpName, serr), tf, tmpPath)
		}
		if cerr := tf.Close(); cerr != nil {
			return "", scrubFile(fmt.Errorf("store: tmp close %s: %w", tmpName, cerr), nil, tmpPath)
		}
		return tmpPath, nil
	}
	return "", fmt.Errorf("%w: tmp name after %d attempts, last err: %v", ErrCollision, MaxTmpAttempts, lastErr)
}

// Swappable os primitives for failure-path tests. The zero-value production
// implementations call into the os package directly. removeTmp removes the
// post-commit tmp name (initial removal, link-fail cleanup, best-effort
// retry); removeFile stays reserved for old/rollback removals so swapRemove
// call-counts remain unchanged; renameFile is the same-status journal-only
// rename-over commit (replaces the existing target: atomic on POSIX, on
// Windows MoveFileEx(MOVEFILE_REPLACE_EXISTING) non-atomically).
var (
	linkFile   = os.Link
	renameFile = os.Rename
	removeFile = os.Remove
	removeTmp  = os.Remove
	nanoNow    = func() int64 { return time.Now().UnixNano() }
)
