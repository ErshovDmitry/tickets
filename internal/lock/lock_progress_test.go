package lock

// T-0226 regressions for holder-progress-aware lock timeouts. The
// failure they pin (CI run 44942515928): on an oversubscribed
// windows-latest runner a healthy holder kept the lock past a waiter's
// wall-clock budget and `ticket new` died with "still held by another
// process after 5s". These tests shorten the holder heartbeat to 10ms
// so a waiter's progress-observation window (timeout − heartbeat
// interval) is ~2s — an order of magnitude above the 50ms poll interval
// and above whole-process scheduling stalls on shared 4-vCPU runners
// (a stall longer than the window cancels an extension: while frozen,
// no heartbeat can run, so the holder is genuinely stalled from any
// observable standpoint). The tests never run in parallel, so
// overriding the package vars is race-free.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// shortHeartbeat bounds how long these tests hold real time: the
// heartbeat is 40x faster than in production, so a healthy-holder wait
// of seconds and a patience-cap wait of 8s still exercise many
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

// shrinkCap replaces the patience-cap factor for one test and restores
// it afterwards, so the cap can be reached with a stall-resistant
// multi-second timeout without a multi-minute test. The mechanism under
// test is the cap itself, not the literal 12. Same no-Parallel rule as
// shortenHeartbeat.
func shrinkCap(t *testing.T, factor int64) {
	t.Helper()
	prev := progressCapFactor
	progressCapFactor = factor
	t.Cleanup(func() { progressCapFactor = prev })
}

// TestAcquireLockedExtendsOnHolderProgress is the T-0226 regression: a
// holder that keeps making progress (the heartbeat every
// progressInterval) must not fail a waiter whose plain wall-clock budget
// expires mid-hold. On the pre-fix code the waiter errored with
// ErrLocked after 2s; the fixed code extends the deadline on every
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
	lk, err := AcquireLocked(path, 2*time.Second)
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
	// Timeout 2s keeps the observation window (~2s) above whole-process
	// stalls; the cap factor shrinks to 4 so the cap (8s) is reached
	// within a short test.
	shrinkCap(t, 4)
	path := filepath.Join(t.TempDir(), ".lock")
	release, err := Acquire(path)
	if err != nil {
		t.Fatalf("acquire holder: %v", err)
	}
	const timeout = 2 * time.Second
	const hold = 9 * time.Second // > cap 4×2s = 8s
	released := make(chan struct{})
	go func() {
		defer close(released)
		time.Sleep(hold)
		_ = release()
	}()

	start := time.Now()
	lk, err := AcquireLocked(path, timeout)
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("livelocked holder: want ErrLocked, got %v (locker=%v)", err, lk)
	}
	if d := time.Since(start); d < time.Duration(progressCapFactor)*timeout {
		t.Fatalf("patience cap ignored: failed after only %s", d)
	}
	// The holder outlives the failed waiter: join its release so the
	// lock file is closed before t.TempDir cleanup (Windows refuses to
	// delete an open file).
	<-released
}
