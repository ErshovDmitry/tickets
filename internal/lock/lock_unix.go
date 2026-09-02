//go:build unix

package lock

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// MaxEINTRRetries is the upper bound on how many times lockExclusive will
// re-issue a blocking flock(2) after it returns EINTR. POSIX allows
// signals to interrupt blocking syscalls; the underlying syscall.Flock
// does NOT auto-retry, so the bound lives here. Exported so callers and
// tests can reason about the contract.
const MaxEINTRRetries = 8

// lockExclusive takes the blocking exclusive flock(2) on f. It issues
// one initial call and, on EINTR, re-issues the syscall up to
// MaxEINTRRetries times (1 + MaxEINTRRetries attempts in total); any
// other errno is wrapped and returned immediately.
func lockExclusive(f *os.File) error {
	for i := 0; i <= MaxEINTRRetries; i++ {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if err == nil {
			return nil
		}
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		return fmt.Errorf("flock LOCK_EX: %w", err)
	}
	return fmt.Errorf("flock LOCK_EX: exceeded %d EINTR retries", MaxEINTRRetries)
}

// tryLockExclusive attempts a non-blocking exclusive flock(2). EWOULDBLOCK
// (lock held by another process) and EINTR (interrupted before the lock
// could be taken) both map to ErrLocked; the function MUST NOT spin on
// either. Any other errno is wrapped and returned.
func tryLockExclusive(f *os.File) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return nil
	}
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EINTR) {
		return ErrLocked
	}
	return fmt.Errorf("flock LOCK_EX|LOCK_NB: %w", err)
}

// unlock drops the flock(2) on f. EINTR is not retried — by definition
// the lock is already ours, and a stray signal here just means the
// unlock completed; the eventual Close will reap any kernel-side state.
func unlock(f *os.File) error {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("flock LOCK_UN: %w", err)
	}
	return nil
}
