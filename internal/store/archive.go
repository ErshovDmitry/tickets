package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

// AlreadyStatusError reports a set to the status the ticket already has
// with no comment: a same-status set with a comment is a journal-only
// append (appendSameStatusLocked), but without one it is refused as a
// no-op. Number and Status are carried so the cli can render the pinned
// message without the store spelling out UI text (same pattern as
// CollisionError / NotClosedError).
type AlreadyStatusError struct {
	Number int
	Status domain.Status
}

// Error renders the internal (non-UI) description of the rejection.
func (e *AlreadyStatusError) Error() string {
	return fmt.Sprintf("store: ticket %d already in status %s", e.Number, e.Status)
}

// ScanWarningsError reports ticket-directory entries the scanner
// rejected (broken names, non-.md files, non-regular entries). Warnings
// carries the raw scan findings; the cli renders them. Rejected entries
// can never be archive candidates (status is filename-derived), so the
// move of valid done/closed tickets proceeds and the warnings are
// reported alongside the result.
type ScanWarningsError struct{ Warnings []ParseWarning }

// Error renders the internal (non-UI) description: every warning on one
// joined line.
func (e *ScanWarningsError) Error() string {
	msgs := make([]string, len(e.Warnings))
	for i, w := range e.Warnings {
		msgs[i] = w.Error()
	}
	return fmt.Sprintf("store: scan warnings: %s", strings.Join(msgs, "; "))
}

// Archive moves ticket n from tickets/ to tickets/archive/, appending a
// "перенесён в архив" journal line, and returns the full path of the
// archived file (the cli prints it, as the pre-Go implementation did).
//
// Errors: ErrNotFound (absent number), ErrAlreadyArchived (already under
// archive/), *NotClosedError (status is not done/closed) and
// *CollisionError (target exists). The status check runs after the
// already-archived check, as in the pre-Go implementation.
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

// ArchiveClosed archives every done/closed ticket whose number is unique:
// it is ArchiveClosedSkipping with no skips (the pre-T-0157 behaviour).
func (s *Store) ArchiveClosed(who string) ([]string, error) {
	return s.ArchiveClosedSkipping(who, nil)
}

// ArchiveClosedSkipping moves every done/closed ticket from tickets/ to
// tickets/archive/, leaving every number listed in skip untouched (T-0157:
// an ambiguous number must not be archived on an arbitrary first match),
// and returns the full paths of the moved files in the
// inherited order: all done files first, then all closed files, each
// group in ticket-number order (the two passes run back to back, as in
// the bash era). No closed tickets is NOT an error (exit 0) — but
// only when the scan produced no warnings: the result is an empty slice
// with a nil error, and the cli prints the
// "Нет закрытых тикетов для архивации." line.
//
// Scan warnings (broken names, non-.md files, non-regular entries) do
// not block archiving: those entries can never be done/closed
// candidates (status is filename-derived), so valid moves proceed.
// Errors:
//   - the directory itself is unreadable: "store: scan: %w", nothing
//     is moved;
//   - *ScanWarningsError after the loop when any entry was rejected;
//   - a mid-run archiveOneLocked failure joined with the
//     *ScanWarningsError accumulated so far (warnings are never lost).
//
// On failure the paths archived before the error are still returned, so
// the caller can report the work that did land (each target is echoed
// as it goes, then dies).
func (s *Store) ArchiveClosedSkipping(who string, skip map[int]struct{}) ([]string, error) {
	var moved []string
	err := s.withLock(func() error {
		entries, warnings, dirErr := s.scan()
		if dirErr != nil {
			return fmt.Errorf("store: scan: %w", dirErr)
		}
		var warnErr error
		if len(warnings) > 0 {
			warnErr = &ScanWarningsError{Warnings: warnings}
		}
		// Two passes over the number-sorted entries follow the bash-era
		// glob sequence T-NNNN-done.md then T-NNNN-closed.md.
		for _, want := range []domain.Status{domain.StatusDone, domain.StatusClosed} {
			for _, e := range entries {
				if _, dup := skip[e.Number]; dup || e.Status != want {
					continue
				}
				target, aerr := s.archiveOneLocked(e.Number, who)
				if aerr != nil {
					return errors.Join(aerr, warnErr)
				}
				moved = append(moved, target)
			}
		}
		return warnErr
	})
	return moved, err
}

// logArchiveFailure emits the single terminal failure event of one
// archiveOneLocked operation; only the number and stage are recorded.
func (s *Store) logArchiveFailure(n int, stage string) {
	s.Logger().Error("store.archive.failure", "number", n, "stage", stage)
}

// archiveOneLocked assumes the per-store advisory lock is held. The
// filesystem sequence mirrors setStatusLocked: Lstat(target) fast-path,
// writeTmp in the archive directory, no-replace linkFile commit, tmp
// name removal, then removal of the old file with target rollback on
// failure. Returns the full target path.
func (s *Store) archiveOneLocked(n int, who string) (string, error) {
	// T-0065: archive journal line ("- <ts> — перенесён в архив (<who>)")
	// is line-oriented, so who with CR/LF is a parallel injection vector
	// (cycle-1 review finding). Guard up-front via the shared helper.
	if err := validateJournalInput("", who); err != nil {
		return "", err
	}
	cur, err := s.findOneLocked(n, "store.archive.failure")
	if err != nil {
		return "", err
	}
	if filepath.Base(cur.dir) == "archive" {
		s.logArchiveFailure(n, "already")
		return "", ErrAlreadyArchived
	}
	if cur.Status != domain.StatusDone && cur.Status != domain.StatusClosed {
		s.logArchiveFailure(n, "not_closed")
		return "", &NotClosedError{Status: cur.Status}
	}
	archiveDir, aerr := validArchiveDir(s.Dir)
	if aerr != nil {
		return "", aerr
	}
	if archiveDir == "" { // no archive yet: create it now (T-0078)
		archiveDir = filepath.Join(s.Dir, "archive")
		if mkErr := os.MkdirAll(archiveDir, 0o755); mkErr != nil {
			return "", fmt.Errorf("store: mkdir archive: %w", mkErr)
		}
	}
	target := filepath.Join(archiveDir, cur.Name)
	if _, lerr := os.Lstat(target); lerr == nil {
		s.logArchiveFailure(n, "collision")
		return "", &CollisionError{Target: target}
	} else if !errors.Is(lerr, fs.ErrNotExist) {
		return "", fmt.Errorf("store: lstat target: %w", lerr)
	}
	old := filepath.Join(cur.dir, cur.Name)
	body, rerr := renderArchived(old, cur.fileEntry, who)
	if rerr != nil {
		return "", rerr
	}
	if cerr := s.commitArchive(n, cur.Status, body, old, target, archiveDir); cerr != nil {
		return "", cerr
	}
	s.Logger().Info("store.archive.commit", "number", n, "status", cur.Status, "path", target)
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

// commitArchive commits an archive move through commitMove, the shared
// archive/delete sequence (T-0189): tmp write in archiveDir, no-replace
// link commit, tmp-name removal, then old removal with target rollback
// on any post-commit failure. The full failure contract lives in
// commitMove; archive failure events stay store.archive.failure.
func (s *Store) commitArchive(n int, st domain.Status, body []byte, old, target, archiveDir string) error {
	return s.commitMove(n, st, body, old, target, archiveDir, s.logArchiveFailure)
}
