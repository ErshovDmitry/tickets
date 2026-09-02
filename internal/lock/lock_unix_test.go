//go:build unix

package lock

import (
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestEINTRRetry fires SIGUSR1 at the test process while Acquire is
// running, then verifies that Acquire still returns success within the
// MaxEINTRRetries budget. It lives in a unix-tagged file: POSIX
// flock(2) can deliver EINTR to a blocking wait, while Windows has no
// SIGUSR1 and never EINTR-interrupts user-mode blocking syscalls.
func TestEINTRRetry(t *testing.T) {
	if MaxEINTRRetries != 8 {
		t.Errorf("MaxEINTRRetries = %d, want 8", MaxEINTRRetries)
	}

	path := filepath.Join(t.TempDir(), ".lock")

	// Absorb SIGUSR1 so the runtime does not terminate the test process.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGUSR1)
	defer signal.Stop(sigCh)

	// Holder goroutine: acquire the lock, sleep briefly, release.
	holderDone := make(chan error, 1)
	go func() {
		r, err := Acquire(path)
		if err != nil {
			holderDone <- err
			return
		}
		time.Sleep(150 * time.Millisecond)
		holderDone <- r()
	}()

	// Wait until TryAcquire confirms the holder owns the lock. If the
	// probe wins the race it must release immediately, otherwise its own
	// leaked flock deadlocks both the holder and the final Acquire.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		release, err := TryAcquire(path)
		if errors.Is(err, ErrLocked) {
			break
		}
		if err == nil {
			_ = release()
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Fire SIGUSR1 — kernel may deliver EINTR to the blocked flock.
	_ = syscall.Kill(syscall.Getpid(), syscall.SIGUSR1)

	// Drain the signal so the runtime can move on.
	select {
	case <-sigCh:
	case <-time.After(time.Second):
	}

	// Acquire must succeed after holder releases (with EINTR retries).
	start := time.Now()
	release, err := Acquire(path)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Acquire after SIGUSR1: %v (elapsed %v)", err, elapsed)
	}
	if release == nil {
		t.Fatal("Acquire returned nil release after SIGUSR1")
	}
	defer release()

	select {
	case err := <-holderDone:
		if err != nil {
			t.Errorf("holder release: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("holder goroutine did not finish")
	}
}
