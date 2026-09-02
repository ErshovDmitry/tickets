// Package lock provides a portable advisory exclusive lock used to
// serialise ticket-number issuance across cooperating processes.
//
// On Unix-like systems the lock is a flock(2) advisory lock; on Windows it
// is a LockFileEx region lock. Both are released automatically when the
// file descriptor / handle is closed, so a process death cannot strand
// the lock the way a PID-file scheme would.
//
// The exported surface is small: Acquire and TryAcquire are the entry
// points and ErrLocked is the sentinel.
//
//	func Acquire(path string) (release func() error, err error)
//	func TryAcquire(path string) (release func() error, err error)
//	var ErrLocked = errors.New(...)
//
// Both entry points hand back (*Locker).OnceRelease as a func() error
// suitable for `defer`; the closure is non-nil iff err is nil, and it
// is idempotent. (*Locker).Release is the equivalent direct-method
// form. Both methods are nil-safe: called on a nil *Locker they
// release nothing and return nil.
package lock

import (
	"errors"
	"fmt"
	"os"
	"sync"
)

// ErrLocked is returned by TryAcquire when the lock is currently held by
// another process. Callers detect it with errors.Is.
var ErrLocked = errors.New("lock: already held by another process")

// Locker owns the open file behind an acquired lock and a sync.Once guard
// that makes Release idempotent. Callers obtain a Locker only indirectly
// via Acquire / TryAcquire, which hand back the OnceRelease method value
// as a func() error suitable for `defer`.
type Locker struct {
	f    *os.File
	once sync.Once
	// err captures the result of the first Release call so that
	// subsequent (idempotent) calls return the same value instead of
	// silently dropping a real I/O error.
	err error
}

// Acquire opens path and blocks until the exclusive lock is granted.
// On success the returned release closure unlocks and closes the file
// when invoked. The closure is nil iff err is non-nil. Open and lock
// failures are wrapped with path context; match them with errors.Is.
func Acquire(path string) (release func() error, err error) {
	f, openErr := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if openErr != nil {
		return nil, fmt.Errorf("lock: open %s: %w", path, openErr)
	}
	if lockErr := lockExclusive(f); lockErr != nil {
		_ = f.Close()
		return nil, fmt.Errorf("lock: lock %s: %w", path, lockErr)
	}
	l := &Locker{f: f}
	return l.OnceRelease, nil
}

// TryAcquire is the non-blocking counterpart of Acquire. When the lock is
// held by another process it returns (nil, ErrLocked) without spinning.
// Other open and lock failures are wrapped with path context; detect
// both cases with errors.Is.
func TryAcquire(path string) (release func() error, err error) {
	f, openErr := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if openErr != nil {
		return nil, fmt.Errorf("lock: open %s: %w", path, openErr)
	}
	lockErr := tryLockExclusive(f)
	if lockErr != nil {
		_ = f.Close()
		if errors.Is(lockErr, ErrLocked) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("lock: lock %s: %w", path, lockErr)
	}
	l := &Locker{f: f}
	return l.OnceRelease, nil
}

// Release unlocks the file and closes it exactly once. Subsequent calls
// return the same error as the first invocation (usually nil). Safe to
// call on a nil receiver.
func (l *Locker) Release() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		l.err = errors.Join(unlock(l.f), l.f.Close())
	})
	return l.err
}

// OnceRelease is the method-value form of Release. Acquire and
// TryAcquire return it as a func() error so callers can write
// `defer release()`. It is nil-safe: calling it on a nil *Locker returns nil
// without panicking.
func (l *Locker) OnceRelease() error {
	if l == nil {
		return nil
	}
	return l.Release()
}
