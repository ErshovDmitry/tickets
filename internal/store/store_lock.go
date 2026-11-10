package store

// This file holds the store's advisory-lock helpers and the ticket-numbering
// path used by Create. It is a pure code-motion split of store.go: no
// behaviour, signature or API change.

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

// lockPath returns the per-store advisory lock file.
func (s *Store) lockPath() string { return filepath.Join(s.Dir, ".lock") }

// defaultLockTimeout is the withLock wait budget when TICKET_LOCK_TIMEOUT
// is unset or unparsable.
const defaultLockTimeout = 5 * time.Second

// parseLockTimeout reads TICKET_LOCK_TIMEOUT on every withLock call (so
// tests can t.Setenv it). An empty or unparsable value — including one
// missing its time unit, like "5" — falls back to defaultLockTimeout;
// values <= 0 pass through and mean a single lock attempt.
func parseLockTimeout() time.Duration {
	v := os.Getenv("TICKET_LOCK_TIMEOUT")
	if v == "" {
		return defaultLockTimeout
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return defaultLockTimeout
	}
	return d
}

// withLock acquires the per-store advisory lock — waiting up to
// parseLockTimeout() for it, with deadline expiry reported as
// lock.ErrLocked — runs fn, then releases. A release error is joined
// with fn's error (errors.Join discards nils), so neither is lost; the
// signature and callers are unchanged.
// Read-only Stores (NewReadOnly) are rejected before the lock is taken,
// so a rejected mutation never touches the directory.
//
// After a successful fn the lock is verified post-hoc (lock.Locker
// StillValid): if the lock file was replaced while held, the operation
// may have raced with another process and the returned error says so.
// When fn itself fails the check is skipped — the operation's own error
// takes precedence.
func (s *Store) withLock(fn func() error) (err error) {
	if s.readOnly {
		return ErrReadOnly
	}
	log, path := s.Logger(), s.lockPath()
	lk, err := lock.AcquireLocked(path, parseLockTimeout())
	if err != nil {
		reason := "error"
		if errors.Is(err, lock.ErrLocked) {
			reason = "timeout"
		}
		log.Error("store.lock.error", "path", path, "reason", reason)
		return fmt.Errorf("store: lock: %w", err)
	}
	log.Debug("store.lock.acquired", "path", path)
	defer func() {
		rerr := lk.Release()
		if rerr != nil {
			log.Error("store.lock.error", "path", path, "reason", "error")
		} else {
			log.Debug("store.lock.released", "path", path)
		}
		err = errors.Join(err, rerr)
	}()
	if err := fn(); err != nil {
		return err
	}
	if serr := lk.StillValid(); serr != nil {
		log.Error("store.lock.error", "path", path, "reason", "swapped")
		return fmt.Errorf("store: lock: lock file replaced while held — operation may have raced: %w", serr)
	}
	return nil
}

// MaxCreateAttempts caps Create retries on O_EXCL races.
const MaxCreateAttempts = 5

// maxTicketNumber is the largest number that fits the 4-digit filename
// field. Beyond it Filename would emit a name ParseFilename rejects
// (5 digits), producing an invisible, unscannable ticket.
const maxTicketNumber = 9999

// afterCreateHook is the test-only seam of the T-0157 Create verification
// (same idiom as the swappable os primitives in tmpfile.go): it runs after
// the new file has been created, written, synced and closed, and before
// the verification scan, so a test can inject a competing file for the
// same number. It is never nil.
var afterCreateHook = func(n int, path string) {}

// createLocked assumes the per-store advisory lock is held. Any failure
// after the O_EXCL open removes the just-created file, joining cleanup
// errors, so no corrupt ticket remains.
func (s *Store) createLocked(t *domain.Ticket) (int, error) {
	var lastErr error
	for attempt := 0; attempt < MaxCreateAttempts; attempt++ {
		hi, err := s.maxNumberLocked()
		if err != nil {
			return 0, fmt.Errorf("store: determine next number: %w", err)
		}
		n := hi + 1
		if n > maxTicketNumber {
			return 0, fmt.Errorf("store: next number %d exceeds the 4-digit filename limit (%d)", n, maxTicketNumber)
		}
		t.Number = n
		fname := domain.Filename(n, t.Status)
		path := filepath.Join(s.Dir, fname)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			if errors.Is(err, fs.ErrExist) {
				s.Logger().Debug("store.number.retry",
					"attempt", attempt+1, "max", MaxCreateAttempts, "number", n)
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
		afterCreateHook(n, path)
		if verr := s.verifyCreatedNumber(n, fname, path); verr != nil {
			return 0, verr
		}
		return n, nil
	}
	return 0, fmt.Errorf("%w: after %d attempts, last err: %v", ErrCollision, MaxCreateAttempts, lastErr)
}

// verifyCreatedNumber checks that the file this Create just committed is
// the only file carrying number n (T-0157). A foreign file with the same
// number — any status, in s.Dir or archive/ — means a competing writer
// exists: the created file is removed and the bare *AmbiguousNumberError
// is returned, so the caller must not use n. A failed removal leaves a
// fully valid ticket on disk (visible in list); the returned error names
// its absolute path. When the verification scan itself fails, ambiguity
// is never invented: the created file is scrubbed and the scan error is
// returned instead.
func (s *Store) verifyCreatedNumber(n int, fname, path string) error {
	files, err := s.findAllNumber(n)
	if err != nil {
		return fmt.Errorf("store: verify number %d: %w", n, errors.Join(err, scrubFile(nil, nil, path)))
	}
	conflicts := make([]locatedFile, 0, len(files))
	for _, f := range files {
		if f.Number == n && !(f.dir == s.Dir && f.Name == fname) {
			conflicts = append(conflicts, f)
		}
	}
	if len(conflicts) == 0 {
		return nil
	}
	amb := ambiguousNumberError(n, conflicts)
	if rerr := removeFile(path); rerr != nil {
		return errors.Join(amb, rerr,
			fmt.Errorf("store: created ticket file left on disk and visible in list: %s", path))
	}
	return amb
}
