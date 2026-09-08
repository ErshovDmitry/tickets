package lock

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// watchdog is the wall-clock budget for the timeout tests: a hang or an
// unbounded spin must fail the test well before CI timeouts.
const watchdog = 5 * time.Second

// TestAcquireTimeoutLocked (plan test 1): while the lock is held,
// AcquireTimeout spins until the deadline and fails with ErrLocked, and
// no earlier than the requested timeout.
func TestAcquireTimeoutLocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".lock")

	release, err := Acquire(path)
	if err != nil {
		t.Fatalf("holder Acquire: %v", err)
	}
	defer release()

	start := time.Now()
	r, err := AcquireTimeout(path, 300*time.Millisecond)
	elapsed := time.Since(start)

	if r != nil {
		t.Error("AcquireTimeout returned non-nil release under contention")
	}
	if !errors.Is(err, ErrLocked) {
		t.Errorf("err = %v, want ErrLocked", err)
	}
	if elapsed < 300*time.Millisecond {
		t.Errorf("elapsed = %s, want >= 300ms", elapsed)
	}
	if elapsed >= watchdog {
		t.Errorf("elapsed = %s, want < %s (watchdog)", elapsed, watchdog)
	}
}

// TestAcquireTimeoutFree (plan test 2): a free lock is taken quickly,
// and timeout <= 0 means exactly one attempt — success when free,
// immediate ErrLocked when held.
func TestAcquireTimeoutFree(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".lock")

	start := time.Now()
	release, err := AcquireTimeout(path, 300*time.Millisecond)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("AcquireTimeout on free lock: %v", err)
	}
	if release == nil {
		t.Fatal("AcquireTimeout returned nil release without error")
	}
	if err := release(); err != nil {
		t.Errorf("release: %v", err)
	}
	if elapsed >= watchdog {
		t.Errorf("elapsed = %s, want < %s (watchdog)", elapsed, watchdog)
	}

	// timeout <= 0: one attempt, immediate ErrLocked while held.
	held, err := Acquire(path)
	if err != nil {
		t.Fatalf("holder Acquire: %v", err)
	}
	defer held()
	start = time.Now()
	r, err := AcquireTimeout(path, 0)
	if !errors.Is(err, ErrLocked) {
		t.Errorf("AcquireTimeout(0) while held: err = %v, want ErrLocked", err)
	}
	if r != nil {
		t.Error("AcquireTimeout(0) while held returned non-nil release")
	}
	if elapsed := time.Since(start); elapsed >= 200*time.Millisecond {
		t.Errorf("single attempt took %s, want < 200ms (no retries)", elapsed)
	}

	// Negative timeout: same single-attempt semantics as 0.
	start = time.Now()
	r, err = AcquireTimeout(path, -time.Second)
	if !errors.Is(err, ErrLocked) {
		t.Errorf("AcquireTimeout(-1s) while held: err = %v, want ErrLocked", err)
	}
	if r != nil {
		t.Error("AcquireTimeout(-1s) while held returned non-nil release")
	}
	if elapsed := time.Since(start); elapsed >= 200*time.Millisecond {
		t.Errorf("single attempt (-1s) took %s, want < 200ms (no retries)", elapsed)
	}
}

// TestStillValid (plan test 3): the post-hoc guard detects an external
// rm or replacement of the lock file while it is held. Skipped on
// Windows, where an open file cannot be removed (no FILE_SHARE_DELETE),
// making the scenario unreachable.
func TestStillValid(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("rm of an open file is impossible on windows; scenario unreachable")
	}
	path := filepath.Join(t.TempDir(), ".lock")

	lk, err := AcquireLocked(path, time.Second)
	if err != nil {
		t.Fatalf("AcquireLocked: %v", err)
	}
	defer lk.Release()

	if err := lk.StillValid(); err != nil {
		t.Fatalf("StillValid on intact lock: %v, want nil", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("rm: %v", err)
	}
	if err := lk.StillValid(); err == nil {
		t.Error("StillValid after rm: nil, want error")
	}
	// Recreating the path must not resurrect validity: the Locker still
	// refers to the old, unlinked inode.
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("recreate: %v", err)
	}
	if err := lk.StillValid(); err == nil {
		t.Error("StillValid after recreate: nil, want error")
	}
}

// TestCheckCurrent (plan test 4): unit coverage of the swap detector —
// match, ENOENT, inode swap and a hard stat error. The hard-error case
// uses an ENOTDIR path, which on Windows maps to ErrNotExist and is
// therefore unobservable there; skip it.
func TestCheckCurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".lock")

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	if swapped, err := checkCurrent(f, path); swapped || err != nil {
		t.Errorf("match: swapped=%v err=%v, want false/nil", swapped, err)
	}

	if runtime.GOOS == "windows" {
		t.Skip("ENOENT test: removing under open fd is Unix-specific")
	}

	// ENOENT: path removed under the open fd → swap, no error.
	if err := os.Remove(path); err != nil {
		t.Fatalf("rm: %v", err)
	}
	if swapped, err := checkCurrent(f, path); err != nil || !swapped {
		t.Errorf("ENOENT: swapped=%v err=%v, want true/nil", swapped, err)
	}

	// Swap: path recreated with a fresh inode.
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("recreate: %v", err)
	}
	if swapped, err := checkCurrent(f, path); err != nil || !swapped {
		t.Errorf("swap: swapped=%v err=%v, want true/nil", swapped, err)
	}

	if runtime.GOOS == "windows" {
		t.Skip("ENOTDIR maps to ErrNotExist on windows; hard-error case unobservable")
	}
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatalf("blocker: %v", err)
	}
	swapped, err := checkCurrent(f, filepath.Join(dir, "blocker", ".lock"))
	if err == nil {
		t.Error("hard err: nil, want error")
	}
	if swapped {
		t.Error("hard err: swapped=true, want false")
	}
	if errors.Is(err, ErrLocked) {
		t.Errorf("hard err %v must not be ErrLocked", err)
	}
}

// TestTryAcquireSwapMapsToErrLocked (plan test 4): TryAcquire must map a
// detected inode swap to ErrLocked, never to a success or a hard error.
// The open→check window cannot be raced deterministically from a test,
// so the branch is driven through tryAcquireFile — the exact body
// TryAcquire runs — with the swap applied where the race would hit.
func TestTryAcquireSwapMapsToErrLocked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("rm of an open file is impossible on windows; scenario unreachable")
	}
	path := filepath.Join(t.TempDir(), ".lock")

	f, err := openLockFile(path) // TryAcquire's open
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	// The swap, applied where the race would place it: between the open
	// above and the check inside tryAcquireFile.
	if err := os.Remove(path); err != nil {
		t.Fatalf("rm: %v", err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("recreate: %v", err)
	}

	lk, err := tryAcquireFile(f, path) // TryAcquire's tryLock + check
	if lk != nil {
		t.Error("tryAcquireFile returned non-nil Locker on swap")
	}
	if !errors.Is(err, ErrLocked) {
		t.Errorf("err = %v, want ErrLocked", err)
	}
}

// TestAcquireLockedHappy (plan test 5): the Locker form acquires, is
// StillValid, releases — and a nil Locker fails closed.
func TestAcquireLockedHappy(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".lock")

	lk, err := AcquireLocked(path, time.Second)
	if err != nil {
		t.Fatalf("AcquireLocked: %v", err)
	}
	if lk == nil {
		t.Fatal("AcquireLocked returned nil Locker without error")
	}
	if err := lk.StillValid(); err != nil {
		t.Errorf("StillValid: %v, want nil", err)
	}
	if err := lk.Release(); err != nil {
		t.Errorf("Release: %v", err)
	}

	var nilLocker *Locker
	if err := nilLocker.StillValid(); err == nil {
		t.Error("nil StillValid: nil, want error")
	}
}
