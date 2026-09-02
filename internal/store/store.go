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
	// ErrReadOnly: a mutation (Create/SetStatus/Archive) was attempted on
	// a Store opened via NewReadOnly. Use errors.Is to test.
	ErrReadOnly = errors.New("store: read-only")
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

	// readOnly marks a Store opened via NewReadOnly; withLock rejects
	// mutations on it. New leaves it false (zero value).
	readOnly bool
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
// opening a directory never mutates it. Mutations (Create, SetStatus,
// Archive) are rejected at runtime: withLock returns ErrReadOnly for a
// read-only Store.
func NewReadOnly(dir string) (*Store, error) {
	if err := validateDir(dir); err != nil {
		return nil, err
	}
	return &Store{Dir: dir, readOnly: true}, nil
}

// lockPath returns the per-store advisory lock file.
func (s *Store) lockPath() string { return filepath.Join(s.Dir, ".lock") }

// withLock acquires the per-store advisory lock, runs fn, then releases.
// A release error is joined with fn's error (errors.Join discards nils),
// so neither is lost; the signature and callers are unchanged.
// Read-only Stores (NewReadOnly) are rejected before the lock is taken,
// so a rejected mutation never touches the directory.
func (s *Store) withLock(fn func() error) (err error) {
	if s.readOnly {
		return ErrReadOnly
	}
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
// Mutations on a read-only Store (NewReadOnly) are rejected by withLock
// with ErrReadOnly.
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
