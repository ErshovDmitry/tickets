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
	"log/slog"
	"os"
	"path/filepath"

	"ticket/internal/domain"
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

// ParseWarning records a ticket file with parse anomalies: scan
// rejections (a file the scanner could not interpret — List skips it,
// never blocking the whole read) and mutation-path parse notices (e.g.
// SetStatus normalizing a duplicate journal section). Both are
// non-fatal and surfaced to the user.
type ParseWarning struct {
	Name string
	Err  error
	// Reason is the fixed reason code for Err (see warn_reason.go). It is
	// additive: Error() ignores it, so warning text is byte-identical.
	Reason string
}

// Error renders a one-line, human-readable description of the warning.
func (w ParseWarning) Error() string {
	if w.Err == nil {
		return w.Name
	}
	return fmt.Sprintf("%s: %v", w.Name, w.Err)
}

// journalDupWarnings returns the mutation-path parse notice for a parsed
// ticket whose journal merged a duplicate "## Journal" section (T-0070):
// one non-fatal ParseWarning carrying domain.ErrJournalDup. Canonical
// files yield nil.
func journalDupWarnings(name string, tk *domain.Ticket) []ParseWarning {
	if tk.JournalDup {
		return []ParseWarning{{Name: name, Err: domain.ErrJournalDup, Reason: reasonJournalDup}}
	}
	return nil
}

// Store is the per-directory handle. It is normally built by New (writable)
// or NewReadOnly; the zero value has no directory and cannot serve ticket
// operations. Logger is the exception: it is explicitly nil- and zero-value
// safe and never returns nil.
type Store struct {
	// Dir is the absolute path to the tickets directory.
	Dir string

	// readOnly marks a Store opened via NewReadOnly; withLock rejects
	// mutations on it. New leaves it false (zero value).
	readOnly bool

	// logger is the logger injected via WithLogger; nil means unset, and
	// Logger then falls back to a discarding logger. New and NewReadOnly
	// leave it nil (zero value).
	logger *slog.Logger
}

// Option configures a Store built by New or NewReadOnly.
type Option func(*Store)

// WithLogger injects l as the Store logger, so callers can hand a command's
// logger to the store instead of relying on a package global. A nil l counts
// as no injection: Logger then returns a discarding logger.
func WithLogger(l *slog.Logger) Option {
	return func(s *Store) { s.logger = l }
}

// Logger returns the logger injected by WithLogger. When nothing usable was
// injected — no option, WithLogger(nil), a zero-value Store or a nil receiver
// — it builds and returns a discarding logger (slog.DiscardHandler) on
// demand, so the result is always non-nil and Enabled reports false for every
// level. Building per call keeps the fallback out of package state.
func (s *Store) Logger() *slog.Logger {
	if s != nil && s.logger != nil {
		return s.logger
	}
	return slog.New(slog.DiscardHandler)
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
		return fmt.Errorf("store: %w", err)
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
//
// Options are applied only after validation and the write probe succeed, so
// no option ever runs against a rejected directory.
func New(dir string, opts ...Option) (*Store, error) {
	if err := validateDir(dir); err != nil {
		return nil, err
	}
	if err := probeWritable(dir); err != nil {
		return nil, err
	}
	return newStoreWithOptions(dir, false, opts), nil
}

// newStoreWithOptions builds a Store for an already-validated directory and
// applies opts in order.
func newStoreWithOptions(dir string, readOnly bool, opts []Option) *Store {
	s := &Store{Dir: dir, readOnly: readOnly}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// NewReadOnly validates the directory and returns a Store for read-only
// commands (List, Find and friends) only: it skips the write probe, so
// opening a directory never mutates it. Mutations (Create, SetStatus,
// Archive) are rejected at runtime: withLock returns ErrReadOnly for a
// read-only Store.
func NewReadOnly(dir string, opts ...Option) (*Store, error) {
	if err := validateDir(dir); err != nil {
		return nil, err
	}
	return newStoreWithOptions(dir, true, opts), nil
}

// List returns every parseable ticket sorted by number. Unreadable or
// unparseable files are reported via ParseWarning rather than as errors.
// List never returns an error itself; an unreadable directory yields a
// single ParseWarning and no tickets.
func (s *Store) List() ([]domain.Ticket, []ParseWarning) {
	entries, warnings, dirErr := s.scan()
	if dirErr != nil {
		// The directory itself could not be read: dir-read is assigned here,
		// so its *fs.PathError never falls through to "parse".
		return nil, []ParseWarning{{Name: s.Dir, Err: dirErr, Reason: reasonDirRead}}
	}
	var tickets []domain.Ticket
	for _, e := range entries {
		tk, err := readTicketHeader(filepath.Join(s.Dir, e.Name), e.Number)
		if err != nil {
			warnings = append(warnings, ParseWarning{Name: e.Name, Err: err, Reason: reasonOf(err)})
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
//
// Mutation contract: Create mutates *t in place as follows.
//   - If t.Status is empty it is set to open before the lock is taken;
//     this assignment persists even when Create later returns an error.
//   - On success t.Number holds the assigned number.
//   - Early-out errors (a nil ticket or a non-open status) leave t
//     untouched.
//   - Errors from createLocked after a numbering attempt (including
//     ErrCollision) leave t.Number holding the last attempted number;
//     callers must not rely on it. The number-limit error (n > 9999)
//     is the exception in that it returns before that attempt's
//     assignment, so t.Number keeps whatever a previous attempt left
//     (untouched only when the first attempt hits the limit).
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
	s.Logger().Info("store.number.assigned", "number", assigned)
	return assigned, nil
}
