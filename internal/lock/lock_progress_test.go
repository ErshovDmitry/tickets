package lock

// T-0226 regressions for holder-progress-aware lock timeouts. The
// failure they pin (CI run 44942515928): on an oversubscribed
// windows-latest runner a healthy holder kept the lock past a waiter's
// wall-clock budget and `ticket new` died with "still held by another
// process after 5s". These tests shorten the holder heartbeat to 10ms
// so a waiter's progress-observation window (timeout − heartbeat
// interval) stays an order of magnitude above the 50ms poll interval:
// cancelling an extension takes a scheduler stall longer than the whole
// window, not jitter of one poll. The tests never run in parallel, so
// overriding the package var is race-free.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// shortHeartbeat bounds how long these tests hold real time: the
// heartbeat is 40x faster than in production, so a healthy-holder wait
// of seconds and a patience-cap wait of 3.6s still exercise many
// heartbeat/poll interactions instead of a handful.
const shortHeartbeat = 10 * time.Millisecond

// shortenHeartbeat replaces the holder-refresh interval for one test
// and restores it afterwards. Must not be used under t.Parallel: the
// interval is a package var.
func shortenHeartbeat(t *testing.T) {
	t.Helper()
	prev := progressInterval
	progressInterval = shortHeartbeat
	t.Cleanup(func() { progressInterval = prev })
}

// TestAcquireLockedExtendsOnHolderProgress is the T-0226 regression: a
// holder that keeps making progress (the heartbeat every
// progressInterval) must not fail a waiter whose plain wall-clock budget
// expires mid-hold. On the pre-fix code the waiter errored with
// ErrLocked after 500ms; the fixed code extends the deadline on every
// observed heartbeat and is still waiting when the holder releases.
func TestAcquireLockedExtendsOnHolderProgress(t *testing.T) {
	shortenHeartbeat(t)
	path := filepath.Join(t.TempDir(), ".lock")
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
	path := filepath.Join(t.TempDir(), ".lock")
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
	shortenHeartbeat(t)
	path := filepath.Join(t.TempDir(), ".lock")
	release, err := Acquire(path)
	if err != nil {
		t.Fatalf("acquire holder: %v", err)
	}
	const timeout = 300 * time.Millisecond
	const hold = 4 * time.Second // > cap 12×300ms = 3.6s
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
