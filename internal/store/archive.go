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

// ErrAlreadyArchived: the ticket already lives in tickets/archive/.
// Use errors.Is to test.
var ErrAlreadyArchived = errors.New("ticket already archived")

// NotClosedError reports an archive attempt on a ticket that is neither
// done nor closed. Status is the current, filename-derived status; the
// cli renders it in the user-facing message, so the store never spells
// out UI text itself.
type NotClosedError struct{ Status domain.Status }

// Error renders the internal (non-UI) description of the rejection.
func (e *NotClosedError) Error() string {
	return fmt.Sprintf("store: archive requires status done or closed, got %s", e.Status)
}

// CollisionError reports a commit target that already exists. Target is
// the full path; the cli prints it. Unwrap yields ErrCollision, so
// errors.Is(err, ErrCollision) keeps working for callers that only need
// the class of failure.
type CollisionError struct{ Target string }

// Error renders the internal (non-UI) description of the collision.
func (e *CollisionError) Error() string {
	return fmt.Sprintf("store: target %s already exists", e.Target)
}

// Unwrap exposes the ErrCollision sentinel.
func (e *CollisionError) Unwrap() error { return ErrCollision }

// Archive moves ticket n from tickets/ to tickets/archive/, appending a
// "перенесён в архив" journal line, and returns the full path of the
// archived file (the cli prints it, mirroring bash `echo "$target"`).
//
// Errors: ErrNotFound (absent number), ErrAlreadyArchived (already under
// archive/), *NotClosedError (status is not done/closed) and
// *CollisionError (target exists). The status check runs after the
// already-archived check, matching bash cmd_archive.
//
// Mutations on a read-only Store (NewReadOnly) are rejected by withLock
// with ErrReadOnly.
func (s *Store) Archive(n int, who string) (string, error) {
	var target string
	err := s.withLock(func() error {
		var aerr error
		target, aerr = s.archiveOneLocked(n, who)
		return aerr
	})
	if err != nil {
		return "", err
	}
	return target, nil
}

// ArchiveClosed moves every done/closed ticket from tickets/ to
// tickets/archive/ and returns the full paths of the moved files in the
// bash glob order: all done files first, then all closed files, each
// group in ticket-number order (bash cmd_archive iterates the two globs
// back to back). No closed tickets is NOT an error (bash rc=0): the
// result is an empty slice with a nil error, and the cli prints the
// "Нет закрытых тикетов для архивации." line.
//
// On failure the paths archived before the error are still returned, so
// the caller can report the work that did land (bash echoes each target
// as it goes, then dies).
func (s *Store) ArchiveClosed(who string) ([]string, error) {
	var moved []string
	err := s.withLock(func() error {
		entries, _, _ := s.scan()
		// Two passes over the number-sorted entries mirror the bash
		// glob sequence T-NNNN-done.md then T-NNNN-closed.md.
		for _, want := range []domain.Status{domain.StatusDone, domain.StatusClosed} {
			for _, e := range entries {
				if e.Status != want {
					continue
				}
				target, aerr := s.archiveOneLocked(e.Number, who)
				if aerr != nil {
					return aerr
				}
				moved = append(moved, target)
			}
		}
		return nil
	})
	return moved, err
}

// archiveOneLocked assumes the per-store advisory lock is held. The
// filesystem sequence mirrors setStatusLocked: Lstat(target) fast-path,
// writeTmp in the archive directory, no-replace linkFile commit, tmp
// name removal, then removal of the old file with target rollback on
// failure. Returns the full target path.
func (s *Store) archiveOneLocked(n int, who string) (string, error) {
	cur, curDir, err := s.findLocked(n)
	if err != nil {
		return "", err
	}
	if filepath.Base(curDir) == "archive" {
		return "", ErrAlreadyArchived
	}
	if cur.Status != domain.StatusDone && cur.Status != domain.StatusClosed {
		return "", &NotClosedError{Status: cur.Status}
	}
	archiveDir := filepath.Join(s.Dir, "archive")
	if mkErr := os.MkdirAll(archiveDir, 0o755); mkErr != nil {
		return "", fmt.Errorf("store: mkdir archive: %w", mkErr)
	}
	target := filepath.Join(archiveDir, cur.Name)
	if _, lerr := os.Lstat(target); lerr == nil {
		return "", &CollisionError{Target: target}
	} else if !errors.Is(lerr, fs.ErrNotExist) {
		return "", fmt.Errorf("store: lstat target: %w", lerr)
	}
	old := filepath.Join(curDir, cur.Name)
	body, rerr := renderArchived(old, cur, who)
	if rerr != nil {
		return "", rerr
	}
	if cerr := s.commitArchive(n, cur.Status, body, old, target, archiveDir); cerr != nil {
		return "", cerr
	}
	return target, nil
}

// renderArchived reads the ticket at path, appends the archive journal
// entry and returns the rendered body. The filename-derived status
// (entry.Status) stays authoritative; the body's "- Статус: " line is
// untrusted (V16/V19).
func renderArchived(path string, entry fileEntry, who string) ([]byte, error) {
	tk, _, unknown, err := readTicketFile(path, entry.Number)
	if err != nil {
		return nil, fmt.Errorf("store: read %s: %w", entry.Name, err)
	}
	tk.Status = entry.Status
	tk.Journal = append(tk.Journal, domain.JournalEntry{
		At:       time.Now(),
		Who:      who,
		Archived: true,
	})
	body, err := domain.Render(tk, unknown)
	if err != nil {
		return nil, fmt.Errorf("store: render: %w", err)
	}
	return body, nil
}

// commitArchive writes body to a tmp file in archiveDir, commits it to
// target via the no-replace link, then removes old. A failed commit
// cleans the tmp file up. A failed removal of the tmp name after the
// commit triggers a rollback (removeFile(target), the archive never
// gains a duplicate) plus a best-effort tmp-removal retry; a failed
// retry is joined with the tmp-removal error. A failed removal of old
// rolls the target back for the same reason. An already-absent old file
// is success: only the new file remains, which is the invariant.
func (s *Store) commitArchive(n int, st domain.Status, body []byte, old, target, archiveDir string) error {
	tmpPath, werr := s.writeTmp(n, st, body, archiveDir)
	if werr != nil {
		return werr
	}
	if lerr := linkFile(tmpPath, target); lerr != nil {
		if errors.Is(lerr, fs.ErrExist) {
			lerr = &CollisionError{Target: target}
		} else {
			lerr = fmt.Errorf("store: link commit: %w", lerr)
		}
		return errors.Join(lerr, removeTmp(tmpPath))
	}
	// tmp and target share one inode now; drop the tmp name via the
	// removeTmp hook, NOT the removeFile hook (the removeFile call-count
	// stays at remove-old + rollback). A removal failure rolls back the
	// committed target (removeFile) and retries the tmp removal
	// best-effort; a failed retry is joined with the tmp-removal error.
	if rerr := removeTmp(tmpPath); rerr != nil {
		// The commit already happened (target holds the archived
		// body); roll it back by removing the target.
		tmpErr := fmt.Errorf("store: remove tmp %s: %w", tmpPath, rerr)
		if rbErr := removeFile(target); rbErr != nil {
			return errors.Join(
				tmpErr,
				fmt.Errorf("rollback remove %s: %w", target, rbErr),
			)
		}
		// Rollback succeeded: retry the tmp removal best-effort; a
		// failed retry is joined with the original tmp-removal error.
		if retryErr := removeTmp(tmpPath); retryErr != nil {
			tmpErr = errors.Join(tmpErr, retryErr)
		}
		return tmpErr
	}
	if rerr := removeFile(old); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
		rbErr := removeFile(target)
		if rbErr != nil {
			return errors.Join(
				fmt.Errorf("remove old %s: %w", old, rerr),
				fmt.Errorf("rollback remove %s: %w", target, rbErr),
				errors.New("invariant breach: old+new coexist"),
			)
		}
		return fmt.Errorf("remove old %s: %w", old, rerr)
	}
	return nil
}

// ListArchive returns the parseable tickets under tickets/archive/,
// sorted by number. A missing archive/ directory is not an error: the
// result is empty. Warning semantics match List.
func (s *Store) ListArchive() ([]domain.Ticket, []ParseWarning) {
	archiveDir := filepath.Join(s.Dir, "archive")
	entries, warnings, dirErr := scanDir(archiveDir)
	if dirErr != nil {
		if errors.Is(dirErr, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, []ParseWarning{{Name: archiveDir, Err: dirErr}}
	}
	var tickets []domain.Ticket
	for _, e := range entries {
		tk, _, _, err := readTicketFile(filepath.Join(archiveDir, e.Name), e.Number)
		if err != nil {
			warnings = append(warnings, ParseWarning{Name: e.Name, Err: err})
			continue
		}
		tk.Status = e.Status
		tickets = append(tickets, *tk)
	}
	return tickets, warnings
}
