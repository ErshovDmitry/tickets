//go:build windows

package lock

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// lockExclusive takes an exclusive 1-byte region lock on f via
// LockFileEx. A fresh OVERLAPPED is allocated per call so concurrent
// Acquire calls do not share kernel-side bookkeeping.
func lockExclusive(f *os.File) error {
	ol := &windows.Overlapped{}
	err := windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0, // reserved
		1, // bytesLow  — 1-byte region
		0, // bytesHigh —   starting at offset 0
		ol,
	)
	if err != nil {
		return fmt.Errorf("LockFileEx: %w", err)
	}
	return nil
}

// tryLockExclusive is the non-blocking counterpart of lockExclusive.
// LOCKFILE_FAIL_IMMEDIATELY makes LockFileEx return ERROR_LOCK_VIOLATION
// (33 / 0x21) when another process already holds the region; that maps
// to ErrLocked.
func tryLockExclusive(f *os.File) error {
	ol := &windows.Overlapped{}
	err := windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, ol,
	)
	if err == nil {
		return nil
	}
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return ErrLocked
	}
	return fmt.Errorf("LockFileEx(LOCKFILE_FAIL_IMMEDIATELY): %w", err)
}

// unlock releases the same 1-byte region via UnlockFileEx. As with
// lockExclusive, a fresh OVERLAPPED is used.
func unlock(f *os.File) error {
	ol := &windows.Overlapped{}
	err := windows.UnlockFileEx(
		windows.Handle(f.Fd()),
		0, 1, 0, ol,
	)
	if err != nil {
		return fmt.Errorf("UnlockFileEx: %w", err)
	}
	return nil
}
