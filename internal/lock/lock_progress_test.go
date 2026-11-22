package lock

// T-0226 regressions for holder-progress-aware lock timeouts. The
// failure they pin (CI run 44942515928): on an oversubscribed
// windows-latest runner a healthy holder kept the lock past a waiter's
// wall-clock budget and `ticket new` died with "still held by another
// process after 5s". These tests exercise the mechanism with injected
// slowness only — no scheduler dependence: timeouts are hundreds of
// milliseconds, holds are seconds, and the patience cap is an order of
// magnitude above both.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// progressTestPath returns a fresh .lock path inside a temp dir.
func progressTestPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), ".lock")
}

// TestAcquireLockedExtendsOnHolderProgress is the T-0226 regression: a
// holder that keeps making progress (the heartbeat every
// progressInterval) must not fail a waiter whose plain wall-clock budget
// expires mid-hold. On the pre-fix code the waiter errored with
// ErrLocked after 500ms; the fixed code extends the deadline on every
// observed heartbeat and is still waiting when the holder releases.
func TestAcquireLockedExtendsOnHolderProgress(t *testing.T) {
	path := progressTestPath(t)
	release, err := Acquire(path)
	if err != nil {
		t.Fatalf("acquire holder: %v", err)
	}
	const hold = 4 * time.Second // > waiter budget, < patience cap
	go func() {
		time.Sleep(hold)
		_ = release()
	}()

	start := time.Now()
	lk, err := AcquireLocked(path, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("waiter: healthy holder must not fail: %v", err)
	}
	if d := time.Since(start); d < hold-250*time.Millisecond {
		t.Fatalf("acquired before the holder released (%s): "+
			"flock does not conflict within this process", d)
	}
	_ = lk.Release()
}

// TestAcquireLockedTimesOutWithoutProgress pins the preserved half of
// the semantics: a holder that makes NO progress — an older binary, or
// one stuck before its heartbeat starts — still fails the waiter on the
// plain timeout. The raw flock below bypasses the heartbeat on purpose.
func TestAcquireLockedTimesOutWithoutProgress(t *testing.T) {
	path := progressTestPath(t)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open lock file: %v", err)
	}
	if err := lockExclusive(f); err != nil {
		t.Fatalf("raw flock: %v", err)
	}
	defer func() {
		_ = unlock(f)
		_ = f.Close()
	}()

	start := time.Now()
	lk, err := AcquireLocked(path, 300*time.Millisecond)
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("stalled holder: want ErrLocked, got %v (locker=%v)", err, lk)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("timeout not enforced promptly: %s", d)
	}
}

// TestAcquireLockedProgressPatienceCap pins the livelock backstop:
// holder progress extends patience only up to progressCapFactor ×
// timeout; a heartbeating holder that never finishes within the cap is
// still reported as stuck. The timeout must exceed a couple of heartbeat
// intervals or the first progress evidence lands after the plain
// deadline (small timeouts keep the pre-fix behavior).
func TestAcquireLockedProgressPatienceCap(t *testing.T) {
	path := progressTestPath(t)
	release, err := Acquire(path)
	if err != nil {
		t.Fatalf("acquire holder: %v", err)
	}
	const timeout = 400 * time.Millisecond
	const hold = 6 * time.Second // > cap 12×400ms = 4.8s
	go func() {
		time.Sleep(hold)
		_ = release()
	}()

	start := time.Now()
	lk, err := AcquireLocked(path, timeout)
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("livelocked holder: want ErrLocked, got %v (locker=%v)", err, lk)
	}
	if d := time.Since(start); d < progressCapFactor*timeout {
		t.Fatalf("patience cap ignored: failed after only %s", d)
	}
}
