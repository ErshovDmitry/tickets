//go:build unix

package lock

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestEINTRRetry exercises the EINTR-retry path of lockExclusive: the
// holder owns the lock, Acquire blocks in flock(2), and SIGUSR1 is fired
// REPEATEDLY DURING that blocked wait — the signals must overlap the
// flock(2) call, not precede it.
//
// Fundamental limitation (documented per T-0013): Go portably cannot
// control per-thread signal masks. The kernel delivers a process-directed
// SIGUSR1 to an arbitrary unblocked thread, so delivery specifically to
// the thread blocked in flock(2) is NOT guaranteed; forcing it would
// require a Linux-only RT_SIGPROCMASK hack (raw rt_sigprocmask on a
// dedicated thread), which is out of scope for this test-only change.
// Consequently:
//
//   - Liveness is deterministic: the test fails only if Acquire deadlocks,
//     returns an error, or the process dies from the signal. No timing
//     assertion can flip the verdict on a slow machine.
//   - EINTR-path coverage is best-effort: on runs where the kernel
//     delivers every signal to other threads, flock(2) is never
//     interrupted and the retry path is silently skipped.
//
// It lives in a unix-tagged file: POSIX flock(2) can deliver EINTR to a
// blocking wait, while Windows has no SIGUSR1 and never EINTR-interrupts
// user-mode blocking syscalls.
func TestEINTRRetry(t *testing.T) {
	if MaxEINTRRetries != 8 {
		t.Errorf("MaxEINTRRetries = %d, want 8", MaxEINTRRetries)
	}

	path := filepath.Join(t.TempDir(), ".lock")

	// Absorb SIGUSR1 so the runtime does not terminate the test process.
	// signal.Stop is deliberately NOT called: Stop restores SIGUSR1's
	// default (terminating) disposition, and a signal already sent but
	// not yet delivered could then kill the process nondeterministically.
	// Notify stays registered for the life of the process; surplus
	// signals are buffered in sigCh or dropped by the runtime, never fatal.
	sigCh := make(chan os.Signal, 64)
	signal.Notify(sigCh, syscall.SIGUSR1)

	// Drain signals asynchronously: the runtime relay needs a consumer,
	// and no other code may block on signal delivery.
	drainQuit := make(chan struct{})
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		for {
			select {
			case <-sigCh:
			case <-drainQuit:
				return
			}
		}
	}()
	defer func() {
		close(drainQuit)
		<-drainDone
	}()

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
	// leaked flock deadlocks both the holder and the pending Acquire.
	probeDeadline := time.Now().Add(time.Second)
	for {
		release, err := TryAcquire(path)
		if errors.Is(err, ErrLocked) {
			break
		}
		if err != nil {
			t.Fatalf("TryAcquire probe: %v", err)
		}
		_ = release()
		if time.Now().After(probeDeadline) {
			t.Fatal("holder never acquired the lock")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Start Acquire while the holder owns the lock: it blocks in flock(2).
	type acquireResult struct {
		release func() error
		err     error
	}
	acqCh := make(chan acquireResult, 1)
	go func() {
		r, err := Acquire(path)
		acqCh <- acquireResult{release: r, err: err}
	}()

	// Fire SIGUSR1 every ~5 ms WHILE Acquire is blocked in flock(2),
	// until Acquire returns or the timeout expires.
	stopFire := make(chan struct{})
	fireDone := make(chan struct{})
	go func() {
		defer close(fireDone)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = syscall.Kill(syscall.Getpid(), syscall.SIGUSR1)
			case <-stopFire:
				return
			}
		}
	}()

	var res acquireResult
	select {
	case res = <-acqCh:
	case <-time.After(2 * time.Second):
		close(stopFire)
		<-fireDone
		t.Fatal("Acquire did not return while SIGUSR1 was firing")
	}
	close(stopFire)
	<-fireDone

	if res.err != nil {
		t.Fatalf("Acquire under SIGUSR1: %v", res.err)
	}
	if res.release == nil {
		t.Fatal("Acquire returned nil release after SIGUSR1")
	}
	defer func() {
		if err := res.release(); err != nil {
			t.Errorf("release of acquired lock: %v", err)
		}
	}()

	// Holder must finish cleanly after releasing.
	select {
	case err := <-holderDone:
		if err != nil {
			t.Errorf("holder release: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("holder goroutine did not finish")
	}
}

// openFaultLockFile creates a throwaway lock file in t.TempDir() and closes it
// via t.Cleanup, so every fault-injection test owns its fd. The name avoids the
// openLockFile(path) helper in lock.go.
func openFaultLockFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(t.TempDir(), ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("open lock file: %v", err)
	}
	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Errorf("close lock file: %v", err)
		}
	})
	return f
}

// boundedFlock returns a flock(2) replacement for lockExclusiveWithFlock plus a
// call counter. The hook asserts the injected fd/how arguments, counts calls,
// and fails the test as soon as the loop exceeds MaxEINTRRetries+1 attempts, so
// a broken retry bound fails fast instead of spinning. The errno for each call
// comes from fn, which receives the 1-based call number.
func boundedFlock(t *testing.T, f *os.File, fn func(call int) error) (func(fd, how int) error, func() int) {
	t.Helper()
	n := 0
	hook := func(fd, how int) error {
		t.Helper()
		if want := int(f.Fd()); fd != want {
			t.Fatalf("flock fd = %d, want %d", fd, want)
		}
		if how != syscall.LOCK_EX {
			t.Fatalf("flock how = %#x, want LOCK_EX", how)
		}
		n++
		if n > MaxEINTRRetries+1 {
			t.Fatalf("flock called %d times, want at most %d (retry bound not enforced)", n, MaxEINTRRetries+1)
		}
		return fn(n)
	}
	return hook, func() int { return n }
}

// TestLockExclusiveEINTRExhausted pins the retry bound: a flock(2) that always
// returns EINTR must be attempted exactly MaxEINTRRetries+1 times and then fail
// with the documented "exceeded N EINTR retries" error.
func TestLockExclusiveEINTRExhausted(t *testing.T) {
	f := openFaultLockFile(t)
	flock, calls := boundedFlock(t, f, func(int) error { return syscall.EINTR })

	err := lockExclusiveWithFlock(f, flock)

	if got, want := calls(), MaxEINTRRetries+1; got != want {
		t.Errorf("flock calls = %d, want %d", got, want)
	}
	want := fmt.Sprintf("flock LOCK_EX: exceeded %d EINTR retries", MaxEINTRRetries)
	if err == nil || err.Error() != want {
		t.Fatalf("lockExclusiveWithFlock error = %v, want %q", err, want)
	}
}

// TestLockExclusiveEINTRThenSuccess checks that an EINTR interrupted wait is
// retried and that the following successful attempt returns nil.
func TestLockExclusiveEINTRThenSuccess(t *testing.T) {
	f := openFaultLockFile(t)
	flock, calls := boundedFlock(t, f, func(call int) error {
		if call <= MaxEINTRRetries {
			return syscall.EINTR
		}
		return nil
	})

	if err := lockExclusiveWithFlock(f, flock); err != nil {
		t.Fatalf("lockExclusiveWithFlock: %v", err)
	}
	if got, want := calls(), MaxEINTRRetries+1; got != want {
		t.Errorf("flock calls = %d, want %d", got, want)
	}
}

// TestLockExclusiveNonEINTR checks that a non-EINTR errno is returned
// immediately, wrapped with the "flock LOCK_EX:" prefix and still matchable
// with errors.Is.
func TestLockExclusiveNonEINTR(t *testing.T) {
	f := openFaultLockFile(t)
	flock, calls := boundedFlock(t, f, func(int) error { return syscall.EBADF })

	err := lockExclusiveWithFlock(f, flock)

	if err == nil {
		t.Fatal("lockExclusiveWithFlock returned nil, want EBADF")
	}
	if !strings.HasPrefix(err.Error(), "flock LOCK_EX:") {
		t.Errorf("error %q lacks %q prefix", err, "flock LOCK_EX:")
	}
	if !errors.Is(err, syscall.EBADF) {
		t.Errorf("error %v does not wrap EBADF", err)
	}
	if got := calls(); got != 1 {
		t.Errorf("flock calls = %d, want 1", got)
	}
}
