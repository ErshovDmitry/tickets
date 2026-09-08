// Package lock provides a portable advisory exclusive lock used to
// serialise ticket-number issuance across cooperating processes.
//
// On Unix-like systems the lock is a flock(2) advisory lock; on Windows it
// is a LockFileEx region lock. Both are released automatically when the
// file descriptor / handle is closed, so a process death cannot strand
// the lock the way a PID-file scheme would.
//
// The exported surface:
//
//	func Acquire(path string) (release func() error, err error)
//	func TryAcquire(path string) (release func() error, err error)
//	func AcquireTimeout(path string, timeout time.Duration) (release func() error, err error)
//	func AcquireLocked(path string, timeout time.Duration) (*Locker, error)
//	func (*Locker) StillValid() error
//	var ErrLocked = errors.New(...)
//
// Both Acquire and TryAcquire hand back (*Locker).OnceRelease as a
// func() error suitable for `defer`; the closure is non-nil iff err is
// nil, and it is idempotent. (*Locker).Release is the equivalent
// direct-method form. All three are nil-safe: called on a nil *Locker
// they release nothing and return nil.
//
// The lock file can be removed or replaced by an external actor while it
// is held (T-0073 inode swap). Acquire and TryAcquire verify right after
// locking that the flock still guards the file currently at path, and
// (*Locker).StillValid re-checks this after a critical section (post-hoc).
package lock

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync"
	"time"
)

// ErrLocked is returned by TryAcquire when the lock is currently held by
// another process, and wrapped by AcquireLocked when its deadline
// expires. Callers detect it with errors.Is.
var ErrLocked = errors.New("lock: already held by another process")

// maxSwapRetries bounds the unlock-close-reopen loop in Acquire for the
// case where the lock file keeps being replaced between open and flock:
// up to maxSwapRetries attempts total, then Acquire gives up.
const maxSwapRetries = 8

// pollInterval is the pause between TryAcquire attempts in the
// AcquireLocked timeout loop, capped by the remaining deadline.
const pollInterval = 50 * time.Millisecond

// Locker owns the open file behind an acquired lock, the path it was
// acquired from (used by StillValid), and a sync.Once guard that makes
// Release idempotent. Callers obtain a Locker via Acquire / TryAcquire
// (as the OnceRelease method value, suitable for `defer`) or via
// AcquireLocked.
type Locker struct {
	f    *os.File
	path string
	once sync.Once
	// err captures the result of the first Release call so that
	// subsequent (idempotent) calls return the same value instead of
	// silently dropping a real I/O error.
	err error
}

// openLockFile opens (creating if needed) the lock file at path,
// wrapping open failures with path context.
func openLockFile(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock: open %s: %w", path, err)
	}
	return f, nil
}

// Acquire opens path and blocks until the exclusive lock is granted.
// On success the returned release closure unlocks and closes the file
// when invoked. The closure is nil iff err is non-nil. Open and lock
// failures are wrapped with path context; match them with errors.Is.
//
// Right after the lock is granted, Acquire verifies that the locked file
// is still the one at path (T-0073 guard): if another process replaced
// the file in the open→flock window, the flock would guard a stale inode
// nobody else can observe. On such a swap it unlocks, closes, reopens
// whatever is at path now and retries, up to maxSwapRetries times;
// a stat failure of path other than "does not exist" is returned as a
// hard error instead.
func Acquire(path string) (release func() error, err error) {
	f, err := openLockFile(path)
	if err != nil {
		return nil, err
	}
	for retries := 0; ; retries++ {
		if lockErr := lockExclusive(f); lockErr != nil {
			_ = f.Close()
			return nil, fmt.Errorf("lock: lock %s: %w", path, lockErr)
		}
		swapped, checkErr := checkCurrent(f, path)
		if checkErr != nil {
			_ = unlock(f)
			_ = f.Close()
			return nil, fmt.Errorf("lock: check %s: %w", path, checkErr)
		}
		if !swapped {
			return (&Locker{f: f, path: path}).OnceRelease, nil
		}
		// Stale inode: drop the useless flock and retry on the file
		// that is at path now (fail-closed).
		_ = unlock(f)
		_ = f.Close()
		if retries+1 >= maxSwapRetries {
			return nil, fmt.Errorf("lock: %s: lock file keeps being replaced; gave up after %d attempts", path, retries+1)
		}
		if f, err = openLockFile(path); err != nil {
			return nil, err
		}
	}
}

// tryAcquireFile finishes a non-blocking acquire on an already-open f:
// it takes the non-blocking exclusive lock and verifies the locked file
// is still the one at path. A detected swap closes f and reports
// ErrLocked (fail-closed, T-0073); a stat failure of path other than
// "does not exist" closes f and is returned as a hard error wrapped
// with path context. Split out of TryAcquire so the swap branch can be
// exercised deterministically by tests.
func tryAcquireFile(f *os.File, path string) (*Locker, error) {
	lockErr := tryLockExclusive(f)
	if lockErr != nil {
		_ = f.Close()
		if errors.Is(lockErr, ErrLocked) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("lock: lock %s: %w", path, lockErr)
	}
	swapped, checkErr := checkCurrent(f, path)
	if checkErr != nil {
		_ = unlock(f)
		_ = f.Close()
		return nil, fmt.Errorf("lock: check %s: %w", path, checkErr)
	}
	if swapped {
		_ = unlock(f)
		_ = f.Close()
		return nil, ErrLocked
	}
	return &Locker{f: f, path: path}, nil
}

// tryAcquire performs one non-blocking acquire attempt at path.
func tryAcquire(path string) (*Locker, error) {
	f, err := openLockFile(path)
	if err != nil {
		return nil, err
	}
	return tryAcquireFile(f, path)
}

// TryAcquire is the non-blocking counterpart of Acquire. When the lock is
// held by another process it returns (nil, ErrLocked) without spinning.
// Other open and lock failures are wrapped with path context; detect
// both cases with errors.Is. If the lock file was replaced in the
// open→flock window (T-0073), the attempt fails with ErrLocked instead
// of reporting a success on a stale inode.
func TryAcquire(path string) (release func() error, err error) {
	lk, err := tryAcquire(path)
	if err != nil {
		return nil, err
	}
	return lk.OnceRelease, nil
}

// checkCurrent reports whether the already-open f still refers to the
// file currently living at path, guarding the T-0073 inode-swap window
// between opening the lock file and taking the flock on it.
//
// swapped is true (fail-closed) when f's own stat fails, when path no
// longer exists (fs.ErrNotExist), or when the two refer to different
// inodes. A stat failure of path other than fs.ErrNotExist is NOT a
// swap: it is returned as err (wrapped with path context), so it is
// never mistaken for ErrLocked.
func checkCurrent(f *os.File, path string) (swapped bool, err error) {
	fi, fiErr := f.Stat()
	pi, statErr := os.Stat(path)
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return false, fmt.Errorf("lock: %w", statErr)
	}
	if fiErr != nil || errors.Is(statErr, fs.ErrNotExist) || !os.SameFile(fi, pi) {
		return true, nil
	}
	return false, nil
}

// stillHeld builds the deadline-expiry error for AcquireLocked. It wraps
// ErrLocked so callers can keep matching with errors.Is.
func stillHeld(path string, timeout time.Duration) error {
	return fmt.Errorf("lock: %s: still held by another process after %s (another ticket command may be running?): %w", path, timeout, ErrLocked)
}

// AcquireLocked is the Locker-returning form of AcquireTimeout: it spins
// on non-blocking attempts until the lock is granted or the deadline
// (now + timeout) passes, sleeping at most pollInterval between
// attempts. The first attempt never sleeps; timeout <= 0 means exactly
// one attempt. Deadline expiry wraps ErrLocked (match with errors.Is);
// other errors are wrapped with path context.
func AcquireLocked(path string, timeout time.Duration) (*Locker, error) {
	deadline := time.Now().Add(timeout)
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			d := time.Until(deadline)
			if d <= 0 {
				return nil, stillHeld(path, timeout)
			}
			if d > pollInterval {
				d = pollInterval
			}
			time.Sleep(d)
		}
		lk, err := tryAcquire(path)
		if err == nil {
			return lk, nil
		}
		if !errors.Is(err, ErrLocked) {
			return nil, err
		}
		if timeout <= 0 {
			// Single-attempt mode: never sleep, never retry.
			return nil, stillHeld(path, timeout)
		}
	}
}

// AcquireTimeout blocks until the exclusive lock at path is granted or
// timeout elapses; timeout <= 0 means exactly one attempt. On deadline
// expiry it returns an error wrapping ErrLocked so callers can detect
// the contention case with errors.Is. The release closure is nil iff
// err is non-nil.
func AcquireTimeout(path string, timeout time.Duration) (release func() error, err error) {
	lk, err := AcquireLocked(path, timeout)
	if err != nil {
		return nil, err
	}
	return lk.OnceRelease, nil
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

// StillValid reports — after the fact — whether the lock this Locker
// holds still guards the file currently at the path it was acquired
// from. It detects the holder-side damage of the T-0073 inode swap: an
// external removal or replacement of the lock file while it is held
// leaves the flock guarding an unlinked inode, so a competing process
// can already hold the lock on the replacement file.
//
// ⚠ StillValid is POST-HOC: it detects a swap, it cannot prevent the
// window during which two holders may have been inside their critical
// sections concurrently. Call it right after the critical section and
// treat a non-nil result as "the operation may have raced; rerun it".
// On Windows the triggering scenario is unreachable (an open file cannot
// be removed there), making the check a harmless no-op that succeeds.
//
// A nil receiver fails closed with an error.
func (l *Locker) StillValid() error {
	if l == nil {
		return errors.New("lock: StillValid on nil Locker")
	}
	swapped, err := checkCurrent(l.f, l.path)
	if err != nil {
		return fmt.Errorf("lock: check %s: %w", l.path, err)
	}
	if swapped {
		return fmt.Errorf("lock: %s: lock file replaced while held", l.path)
	}
	return nil
}
