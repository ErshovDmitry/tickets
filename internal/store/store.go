// Package store is the only filesystem-touching layer of the tickets CLI.
// It owns ticket directories, file creation, atomic status mutation, and the
// advisory lock that serialises numbering/mutation under parallel agents.
//
// The package surface (Store, New, List, Find, Create, SetStatus) follows
// the plan page d41b726c-d614-45ad-a8b1-f97f7cbcdf4c.
package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"ticket/internal/domain"
	"ticket/internal/lock"
)

// Sentinel errors. Use errors.Is to test.
var (
	// ErrNotFound: no ticket with the requested number exists.
	ErrNotFound = errors.New("ticket not found")
	// ErrCollision: a parallel agent has already taken this number or
	// status; the operation was retried up to the cap and gave up.
	ErrCollision = errors.New("ticket file collision")
)

// ParseWarning records a ticket file the scanner could not interpret.
// List returns these alongside valid tickets; the bad file is skipped,
// never blocking the whole read.
type ParseWarning struct {
	Name string
	Err  error
}

// Error renders a one-line, human-readable description of the warning.
func (w ParseWarning) Error() string {
	if w.Err == nil {
		return w.Name
	}
	return fmt.Sprintf("%s: %v", w.Name, w.Err)
}

// Store is the per-directory handle. The zero value is invalid; use New.
type Store struct {
	// Dir is the absolute path to the tickets directory.
	Dir string
}

// validateDir checks that dir is a non-empty path to an existing
// directory. The error strings are user-facing contract; callers keep
// them verbatim.
func validateDir(dir string) error {
	if dir == "" {
		return errors.New("store: empty directory")
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("store: stat %s: %w", dir, err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("store: %s is not a directory", dir)
	}
	return nil
}

// New validates the directory and returns a writable Store. The directory
// must exist, be a directory, and be writable by the current user (checked
// via a unique O_EXCL probe file — probe.go). Read-only commands must open
// the Store via NewReadOnly instead.
func New(dir string) (*Store, error) {
	if err := validateDir(dir); err != nil {
		return nil, err
	}
	if err := probeWritable(dir); err != nil {
		return nil, err
	}
	return &Store{Dir: dir}, nil
}

// NewReadOnly validates the directory and returns a Store for read-only
// commands (List, Find and friends) only: it skips the write probe, so
// opening a directory never mutates it. Caller MUST obtain the Store via
// New, not NewReadOnly, before calling Create or SetStatus (documented
// contract, no runtime guard — the cli package is the only caller).
func NewReadOnly(dir string) (*Store, error) {
	if err := validateDir(dir); err != nil {
		return nil, err
	}
	return &Store{Dir: dir}, nil
}

// lockPath returns the per-store advisory lock file.
func (s *Store) lockPath() string { return filepath.Join(s.Dir, ".lock") }

// withLock acquires the per-store advisory lock, runs fn, then releases.
// A release error is joined with fn's error (errors.Join discards nils),
// so neither is lost; the signature and callers are unchanged.
func (s *Store) withLock(fn func() error) (err error) {
	release, err := lock.Acquire(s.lockPath())
	if err != nil {
		return fmt.Errorf("store: lock: %w", err)
	}
	defer func() { err = errors.Join(err, release()) }()
	return fn()
}

// List returns every parseable ticket sorted by number. Unreadable or
// unparseable files are reported via ParseWarning rather than as errors.
// List never returns an error itself; an unreadable directory yields a
// single ParseWarning and no tickets.
func (s *Store) List() ([]domain.Ticket, []ParseWarning) {
	entries, warnings, dirErr := s.scan()
	if dirErr != nil {
		return nil, []ParseWarning{{Name: s.Dir, Err: dirErr}}
	}
	var tickets []domain.Ticket
	for _, e := range entries {
		tk, _, _, err := readTicketFile(filepath.Join(s.Dir, e.Name), e.Number)
		if err != nil {
			warnings = append(warnings, ParseWarning{Name: e.Name, Err: err})
			continue
		}
		// The scan-matched filename status is authoritative (V19): the
		// body's "- Статус: " line is untrusted and must not drive List
		// consumers or list filters.
		tk.Status = e.Status
		tickets = append(tickets, *tk)
	}
	return tickets, warnings
}

// Find returns the ticket by number. ErrNotFound is returned for an absent
// number; an unreadable or corrupt file produces a wrapped error carrying
// the file's name (§5: "Find(n) wrapped corrupt error").
func (s *Store) Find(n int) (domain.Ticket, error) {
	tk, _, err := s.FindNamed(n)
	return tk, err
}

// Create writes t as a new open-status ticket and returns its assigned
// number. Create overwrites t.Number with the value it assigns, derives
// the filename from t.Status (defaulting to open if empty), and renders
// the body via domain.RenderNewTicket.
//
// On a race with another Create for the same number the operation
// re-scans and retries; after MaxCreateAttempts unsuccessful tries it
// returns ErrCollision wrapped with the last underlying error.
//
// Caller MUST obtain the Store via New, not NewReadOnly (documented
// contract, no runtime guard).
func (s *Store) Create(t *domain.Ticket) (int, error) {
	if t == nil {
		return 0, errors.New("store: Create: ticket is nil")
	}
	if t.Status == "" {
		t.Status = domain.StatusOpen
	}
	if t.Status != domain.StatusOpen {
		return 0, fmt.Errorf("store: Create requires status open, got %q", t.Status)
	}
	var assigned int
	err := s.withLock(func() error {
		n, err := s.createLocked(t)
		assigned = n
		return err
	})
	if err != nil {
		return 0, err
	}
	return assigned, nil
}

// MaxCreateAttempts caps Create retries on O_EXCL races.
const MaxCreateAttempts = 5

// maxTicketNumber is the largest number that fits the 4-digit filename
// field. Beyond it Filename would emit a name ParseFilename rejects
// (5 digits), producing an invisible, unscannable ticket.
const maxTicketNumber = 9999

// createLocked assumes the per-store advisory lock is held. Any failure
// after the O_EXCL open removes the just-created file, joining cleanup
// errors, so no corrupt ticket remains.
func (s *Store) createLocked(t *domain.Ticket) (int, error) {
	var lastErr error
	for attempt := 0; attempt < MaxCreateAttempts; attempt++ {
		n := s.maxNumberLocked() + 1
		if n > maxTicketNumber {
			return 0, fmt.Errorf("store: next number %d exceeds the 4-digit filename limit (%d)", n, maxTicketNumber)
		}
		t.Number = n
		fname := domain.Filename(n, t.Status)
		path := filepath.Join(s.Dir, fname)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			if errors.Is(err, fs.ErrExist) {
				lastErr = err
				continue
			}
			return 0, fmt.Errorf("store: open %s: %w", fname, err)
		}
		body, rerr := domain.RenderNewTicket(t)
		if rerr != nil {
			return 0, scrubFile(fmt.Errorf("store: render: %w", rerr), f, path)
		}
		if _, werr := f.Write(body); werr != nil {
			return 0, scrubFile(fmt.Errorf("store: write %s: %w", fname, werr), f, path)
		}
		if serr := f.Sync(); serr != nil {
			return 0, scrubFile(fmt.Errorf("store: sync %s: %w", fname, serr), f, path)
		}
		if cerr := f.Close(); cerr != nil {
			return 0, scrubFile(fmt.Errorf("store: close %s: %w", fname, cerr), nil, path)
		}
		return n, nil
	}
	return 0, fmt.Errorf("%w: after %d attempts, last err: %v", ErrCollision, MaxCreateAttempts, lastErr)
}

// SetStatus mutates ticket n to next, appending a journal entry. The
// filesystem sequence (under the per-store advisory lock) is:
//
//  1. Lstat(target). Existence => *CollisionError (carries Target).
//  2. Write the rendered body to a same-directory .tmp file; Sync, Close.
//  3. Link the .tmp to target — an atomic no-replace commit. If the
//     target appeared since the Lstat fast-path (an out-of-lock agent),
//     linkFile fails with fs.ErrExist => *CollisionError; any other
//     failure => "store: link commit: %w"; both join os.Remove(tmp) as
//     cleanup. After a
//     successful link tmp and target share one inode, so the tmp name
//     is removed directly (os.Remove, NOT the removeFile hook); a
//     removal failure is surfaced but the target is NOT rolled back.
//  4. os.Remove(old). ENOENT => success: the old file is already gone
//     (e.g. removed by a lockless external agent), so the success
//     invariant — ONLY the new file remains — already holds; rolling
//     back would delete the only remaining copy. Any other failure =>
//     rollback (os.Remove(target)); failures =>
//     errors.Join(removeOldErr, rollbackErr, breach).
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
// Caller MUST obtain the Store via New, not NewReadOnly (documented
// contract, no runtime guard).
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
		return "", fmt.Errorf("store: ticket %d already in status %s", n, next)
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
		cleanup := os.Remove(tmpPath)
		return "", errors.Join(lerr, cleanup)
	}
	// tmp and target share one inode now; drop the tmp name directly
	// (os.Remove, NOT the removeFile hook — the swapRemove call-count
	// stays at remove-old + rollback). A failure is surfaced but the
	// target is NOT rolled back.
	if rerr := os.Remove(tmpPath); rerr != nil {
		return "", fmt.Errorf("store: remove tmp %s: %w", tmpPath, rerr)
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

// findLocked scans s.Dir then s.Dir/archive for ticket n, returning the
// fileEntry and the directory where it was found. Used by SetStatus and
// Archive to handle archived tickets.
func (s *Store) findLocked(n int) (fileEntry, string, error) {
	entries, _, dirErr := s.scan()
	if dirErr != nil {
		return fileEntry{}, "", fmt.Errorf("store: read dir %s: %w", s.Dir, dirErr)
	}
	for _, e := range entries {
		if e.Number == n {
			return e, s.Dir, nil
		}
	}
	// Check archive/.
	archiveDir := filepath.Join(s.Dir, "archive")
	if _, err := os.Stat(archiveDir); err == nil {
		archiveEntries, _, _ := scanDir(archiveDir)
		for _, e := range archiveEntries {
			if e.Number == n {
				return e, archiveDir, nil
			}
		}
	}
	return fileEntry{}, "", ErrNotFound
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
// implementations call into the os package directly.
var (
	linkFile   = os.Link
	removeFile = os.Remove
	nanoNow    = func() int64 { return time.Now().UnixNano() }
)
