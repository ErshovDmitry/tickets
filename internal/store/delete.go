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

// ErrAlreadyDeleted: the ticket already lives in tickets/deleted/.
// Delete is terminal — a repeat is an error, never success-idempotent
// (Decision 4bbcd1e8). Use errors.Is to test.
var ErrAlreadyDeleted = errors.New("ticket already deleted")

// NotOpenError reports a delete attempt without force on a ticket
// that is not an open main-directory ticket: without force only
// main/open is deletable; force additionally covers every other
// main status and archive (Decision 4bbcd1e8). Status is the
// current filename-derived status; the cli renders it, the store
// carries no UI text (same pattern as NotClosedError).
type NotOpenError struct{ Status domain.Status }

// Error renders the internal (non-UI) description of the rejection.
func (e *NotOpenError) Error() string {
	return fmt.Sprintf("store: delete without force requires status open, got %s", e.Status)
}

// Delete moves the ticket identified by ref (a number or an exact
// base name, resolved across main+archive+deleted) into
// tickets/deleted/T-NNNN-deleted.md, appending exactly one
// old→deleted journal transition, and returns the deleted file's
// full path. The body is preserved: metadata, unknown sections,
// raw templates and the existing journal round-trip through
// parse/render; the filename-derived status is authoritative.
//
// Without force only a main-directory open ticket is deletable
// (*NotOpenError otherwise); force covers every main status and
// archive. Errors: ErrNotFound, *AmbiguousNumberError (duplicate
// number or identical base name across directories),
// ErrAlreadyDeleted, *CollisionError (target exists — nothing is
// changed), *ErrInvalidJournalInput (who with CR/LF), and wrapped
// scan/read errors. The whole resolve/revalidate/mutate sequence
// runs under the per-store advisory lock; mutations on a read-only
// Store (NewReadOnly) are rejected by withLock with ErrReadOnly.
func (s *Store) Delete(ref string, force bool, who string) (string, error) {
	var target string
	err := s.withLock(func() error {
		var derr error
		target, derr = s.deleteOneLocked(ref, force, who)
		return derr
	})
	if err != nil {
		return "", err
	}
	return target, nil
}

// logDeleteFailure emits the single terminal failure event of one
// deleteOneLocked operation; only the number and stage are recorded.
func (s *Store) logDeleteFailure(n int, stage string) {
	s.Logger().Error("store.delete.failure", "number", n, "stage", stage)
}

// deleteEntryHook is the test-only seam of the concurrent Delete
// verification (same idiom as afterCreateHook in store_lock.go): it runs
// at the entry of deleteOneLocked, before any resolve or mutation, so a
// test can observe that Delete entered the lock-protected section. It is
// never nil.
var deleteEntryHook = func(ref string) {}

// deleteOneLocked assumes the per-store advisory lock is held: it
// re-resolves the ref (revalidation — no mutation can interleave),
// applies the already-deleted/force guards, then commits through
// commitMove, the shared archive/delete no-replace sequence. A
// crash before the commit leaves the source untouched.
func (s *Store) deleteOneLocked(ref string, force bool, who string) (string, error) {
	deleteEntryHook(ref)
	// T-0065: the delete journal line is line-oriented, so who with
	// CR/LF is an injection vector. Guard up-front, before any scan.
	if err := validateJournalInput("", who); err != nil {
		return "", err
	}
	cur, err := s.resolveDeleteRef(ref)
	if err != nil {
		return "", err
	}
	n := cur.Number
	deletedPath := filepath.Join(s.Dir, "deleted")
	if cur.dir == deletedPath {
		s.logDeleteFailure(n, "already")
		return "", ErrAlreadyDeleted
	}
	if !force && (cur.dir != s.Dir || cur.Status != domain.StatusOpen) {
		s.logDeleteFailure(n, "not_open")
		return "", &NotOpenError{Status: cur.Status}
	}
	deletedDir, derr := validDeletedDir(s.Dir)
	if derr != nil {
		return "", derr
	}
	if deletedDir == "" { // no deleted/ yet: create it now (mirrors archive)
		if mkErr := os.MkdirAll(deletedPath, 0o755); mkErr != nil {
			return "", fmt.Errorf("store: mkdir deleted: %w", mkErr)
		}
		deletedDir = deletedPath
	}
	target := filepath.Join(deletedDir, domain.Filename(n, domain.StatusDeleted))
	if _, lerr := os.Lstat(target); lerr == nil {
		s.logDeleteFailure(n, "collision")
		return "", &CollisionError{Target: target}
	} else if !errors.Is(lerr, fs.ErrNotExist) {
		return "", fmt.Errorf("store: lstat target: %w", lerr)
	}
	old := filepath.Join(cur.dir, cur.Name)
	body, rerr := renderDeleted(old, cur.fileEntry, who)
	if rerr != nil {
		return "", rerr
	}
	if cerr := s.commitMove(n, domain.StatusDeleted, body, old, target, deletedDir, s.logDeleteFailure); cerr != nil {
		return "", cerr
	}
	s.Logger().Info("store.delete.commit", "number", n, "from", cur.Status, "path", target)
	return target, nil
}

// renderDeleted reads the ticket at path, appends exactly one
// old→deleted transition journal entry (existing journalTransition
// dict template, no new template) and returns the rendered body.
// The filename-derived status (entry.Status) is the transition's
// From; the rendered status is deleted. Everything else — metadata,
// sections, Unknown, RawTemplates, prior journal lines — survives
// the parse/render round trip (mirrors renderArchived).
func renderDeleted(path string, entry fileEntry, who string) ([]byte, error) {
	tk, _, unknown, err := readTicketFile(path, entry.Number)
	if err != nil {
		return nil, fmt.Errorf("store: read %s: %w", entry.Name, err)
	}
	tk.Status = domain.StatusDeleted
	tk.Journal = append(tk.Journal, domain.JournalEntry{
		From: entry.Status,
		To:   domain.StatusDeleted,
		Who:  who,
		At:   time.Now(),
	})
	body, err := domain.Render(tk, unknown)
	if err != nil {
		return nil, fmt.Errorf("store: render: %w", err)
	}
	return body, nil
}
