//go:build windows

package store

import (
	"os"
	"slices"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// fakeRename is an injected rename primitive that records its call count and
// returns the error scripted for that call number (1-based).
type fakeRename struct {
	calls int
	fn    func(call int) error
}

// do implements the rename primitive signature.
func (f *fakeRename) do(string, string) error {
	f.calls++
	return f.fn(f.calls)
}

// fakeClock replaces both the wall clock and the sleep so retry-policy tests
// never wait for real time. step, when set, advances the clock by that amount
// per sleep instead of the requested delay. freeze leaves the wall clock still,
// so the attempt limit — not the budget — is what ends the retry loop.
type fakeClock struct {
	now    time.Time
	sleeps []time.Duration
	step   time.Duration
	freeze bool
}

// nowFn reports the fake wall clock.
func (c *fakeClock) nowFn() time.Time { return c.now }

// sleep records the requested delay and advances the fake clock.
func (c *fakeClock) sleep(d time.Duration) {
	c.sleeps = append(c.sleeps, d)
	if c.freeze {
		return
	}
	if c.step > 0 {
		c.now = c.now.Add(c.step)
		return
	}
	c.now = c.now.Add(d)
}

// withRenameHooks installs the fake rename/sleep/clock primitives for t and
// restores the production ones on cleanup.
func withRenameHooks(t *testing.T, op func(string, string) error, clock *fakeClock) {
	t.Helper()
	prevOp, prevSleep, prevNow := renameOp, renameSleep, renameNow
	renameOp, renameSleep, renameNow = op, clock.sleep, clock.nowFn
	t.Cleanup(func() { renameOp, renameSleep, renameNow = prevOp, prevSleep, prevNow })
}

// linkError builds the error shape os.Rename actually returns on Windows: a
// LinkError wrapping the syscall errno.
func linkError(errno syscall.Errno) error {
	return &os.LinkError{Op: "rename", Old: "old", New: "new", Err: errno}
}

// expectedRenameDelays returns the delays the retry loop must sleep before each
// retry: one pause per attempt after the first, so renameRetryAttempts-1
// entries. The sequence is pinned here explicitly — renameRetryBaseDelay
// doubled for every attempt until it would exceed renameRetryMaxDelay, then
// the cap — on purpose: deriving it through the production nextRenameDelay
// would make the test agree with any change to that helper instead of failing
// on it.
func expectedRenameDelays() []time.Duration {
	return []time.Duration{
		renameRetryBaseDelay,
		2 * renameRetryBaseDelay,
		4 * renameRetryBaseDelay,
		8 * renameRetryBaseDelay,
		16 * renameRetryBaseDelay,
		32 * renameRetryBaseDelay,
		renameRetryMaxDelay,
		renameRetryMaxDelay,
		renameRetryMaxDelay,
	}
}

func TestRenameWithRetryAccessDeniedThenSuccess(t *testing.T) {
	const failures = 2
	fake := &fakeRename{fn: func(call int) error {
		if call <= failures {
			return linkError(syscall.ERROR_ACCESS_DENIED)
		}
		return nil
	}}
	clock := &fakeClock{}
	withRenameHooks(t, fake.do, clock)

	if err := renameWithRetry("old", "new"); err != nil {
		t.Fatalf("renameWithRetry = %v, want nil", err)
	}
	if want := failures + 1; fake.calls != want {
		t.Errorf("rename calls = %d, want %d", fake.calls, want)
	}
	if want := failures; len(clock.sleeps) != want {
		t.Errorf("sleeps = %d, want %d", len(clock.sleeps), want)
	}
}

func TestRenameWithRetrySharingViolationThenSuccess(t *testing.T) {
	fake := &fakeRename{fn: func(call int) error {
		if call == 1 {
			return linkError(windows.ERROR_SHARING_VIOLATION)
		}
		return nil
	}}
	clock := &fakeClock{}
	withRenameHooks(t, fake.do, clock)

	if err := renameWithRetry("old", "new"); err != nil {
		t.Fatalf("renameWithRetry = %v, want nil", err)
	}
	if fake.calls != 2 {
		t.Errorf("rename calls = %d, want 2", fake.calls)
	}
	if len(clock.sleeps) != 1 {
		t.Errorf("sleeps = %d, want 1", len(clock.sleeps))
	}
}

func TestRenameWithRetryPermanentErrorNotRetried(t *testing.T) {
	tests := []struct {
		name  string
		errno syscall.Errno
	}{
		{"file not found", syscall.ERROR_FILE_NOT_FOUND},
		{"lock violation", windows.ERROR_LOCK_VIOLATION},
		{"delete pending", windows.ERROR_DELETE_PENDING},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := linkError(tt.errno)
			fake := &fakeRename{fn: func(int) error { return want }}
			clock := &fakeClock{}
			withRenameHooks(t, fake.do, clock)

			if err := renameWithRetry("old", "new"); err != want {
				t.Errorf("renameWithRetry = %v, want the permanent error %v", err, want)
			}
			if fake.calls != 1 {
				t.Errorf("rename calls = %d, want 1", fake.calls)
			}
			if len(clock.sleeps) != 0 {
				t.Errorf("sleeps = %d, want 0", len(clock.sleeps))
			}
		})
	}
}

func TestRenameWithRetryExhaustionReturnsLastError(t *testing.T) {
	var last error
	fake := &fakeRename{fn: func(int) error {
		last = linkError(syscall.ERROR_ACCESS_DENIED)
		return last
	}}
	// The wall clock is frozen so the attempt limit ends the loop: the real
	// backoff pauses sum to more than renameRetryBudget, and that path is
	// pinned separately by TestRenameWithRetryStopsAtBudget.
	clock := &fakeClock{freeze: true}
	withRenameHooks(t, fake.do, clock)

	err := renameWithRetry("old", "new")
	if err != last {
		t.Errorf("renameWithRetry = %v, want the last error %v", err, last)
	}
	if fake.calls != renameRetryAttempts {
		t.Errorf("rename calls = %d, want %d", fake.calls, renameRetryAttempts)
	}
	if want := renameRetryAttempts - 1; len(clock.sleeps) != want {
		t.Errorf("sleeps = %d, want one pause per retry (%d)", len(clock.sleeps), want)
	}
	if want := expectedRenameDelays(); !slices.Equal(clock.sleeps, want) {
		t.Errorf("sleeps = %v, want %v", clock.sleeps, want)
	}
}

func TestRenameWithRetryStopsAtBudget(t *testing.T) {
	var last error
	fake := &fakeRename{fn: func(int) error {
		last = linkError(syscall.ERROR_ACCESS_DENIED)
		return last
	}}
	// Every sleep consumes the whole wall-clock budget, so the second
	// transient failure finds no room left for another delay.
	clock := &fakeClock{step: renameRetryBudget}
	withRenameHooks(t, fake.do, clock)

	err := renameWithRetry("old", "new")
	if err != last {
		t.Errorf("renameWithRetry = %v, want the transient error %v", err, last)
	}
	if fake.calls >= renameRetryAttempts {
		t.Errorf("rename calls = %d, want fewer than the attempt limit %d", fake.calls, renameRetryAttempts)
	}
	if fake.calls != 2 {
		t.Errorf("rename calls = %d, want 2", fake.calls)
	}
	if want := []time.Duration{renameRetryBaseDelay}; !slices.Equal(clock.sleeps, want) {
		t.Errorf("sleeps = %v, want %v", clock.sleeps, want)
	}
}

func TestRenameWithRetryDoesNotUseRenameFileHook(t *testing.T) {
	fake := &fakeRename{fn: func(int) error { return nil }}
	clock := &fakeClock{}
	withRenameHooks(t, fake.do, clock)

	prev := renameFile
	renameFile = func(string, string) error {
		t.Error("renameWithRetry must call os.Rename directly, not the renameFile hook")
		return nil
	}
	t.Cleanup(func() { renameFile = prev })

	if err := renameWithRetry("old", "new"); err != nil {
		t.Fatalf("renameWithRetry = %v, want nil", err)
	}
}

func TestNextRenameDelay(t *testing.T) {
	const justBelowCap = renameRetryMaxDelay/2 - 1
	tests := []struct {
		name  string
		delay time.Duration
		want  time.Duration
	}{
		{"uncapped doubles", renameRetryBaseDelay, 2 * renameRetryBaseDelay},
		{"just below cap doubles", justBelowCap, 2 * justBelowCap},
		{"exactly half the cap clamps", renameRetryMaxDelay / 2, renameRetryMaxDelay},
		{"at the cap clamps", renameRetryMaxDelay, renameRetryMaxDelay},
		{"above the cap clamps", renameRetryMaxDelay + 1, renameRetryMaxDelay},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextRenameDelay(tt.delay); got != tt.want {
				t.Errorf("nextRenameDelay(%v) = %v, want %v", tt.delay, got, tt.want)
			}
		})
	}
}

// TestRenameFileProductionHookRetries pins the production wiring: init installs
// renameFile = renameWithRetry, so a transient failure observed through the
// package hook must be retried by the fake renameOp instead of surfacing.
func TestRenameFileProductionHookRetries(t *testing.T) {
	fake := &fakeRename{fn: func(call int) error {
		if call == 1 {
			return linkError(windows.ERROR_SHARING_VIOLATION)
		}
		return nil
	}}
	clock := &fakeClock{}
	withRenameHooks(t, fake.do, clock)

	if err := renameFile("old", "new"); err != nil {
		t.Fatalf("renameFile = %v, want nil", err)
	}
	if fake.calls != 2 {
		t.Errorf("rename calls = %d, want 2 (renameFile must be the retrying wrapper)", fake.calls)
	}
	if len(clock.sleeps) != 1 {
		t.Errorf("sleeps = %d, want 1", len(clock.sleeps))
	}
}
