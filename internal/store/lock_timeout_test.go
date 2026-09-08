package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"ticket/internal/domain"
	"ticket/internal/lock"
)

// lockTestWatchdog is the wall-clock budget for the lock-timeout tests:
// a hang must fail the test well before CI timeouts.
const lockTestWatchdog = 5 * time.Second

// TestParseLockTimeout (plan test 6): empty, unparsable and unit-less
// values fall back to the default; valid durations pass through.
func TestParseLockTimeout(t *testing.T) {
	cases := []struct {
		name string
		env  string
		want time.Duration
	}{
		{"empty", "", defaultLockTimeout},
		{"invalid", "garbage", defaultLockTimeout},
		{"no unit", "5", defaultLockTimeout},
		{"millis", "500ms", 500 * time.Millisecond},
		{"zero", "0", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TICKET_LOCK_TIMEOUT", tc.env)
			if got := parseLockTimeout(); got != tc.want {
				t.Errorf("parseLockTimeout(%q) = %s, want %s", tc.env, got, tc.want)
			}
		})
	}
}

// TestWithLockTimeoutEnv (plan test 7): with TICKET_LOCK_TIMEOUT set,
// Create against a lock held in-process (the way a competing CLI would
// hold it) fails with lock.ErrLocked after ~the configured timeout.
func TestWithLockTimeoutEnv(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Setenv("TICKET_LOCK_TIMEOUT", "300ms")

	release, err := lock.Acquire(filepath.Join(dir, ".lock"))
	if err != nil {
		t.Fatalf("holder Acquire: %v", err)
	}
	defer release()

	start := time.Now()
	_, err = s.Create(&domain.Ticket{Title: "timeout probe"})
	elapsed := time.Since(start)

	if !errors.Is(err, lock.ErrLocked) {
		t.Errorf("Create err = %v, want lock.ErrLocked", err)
	}
	if elapsed < 300*time.Millisecond {
		t.Errorf("elapsed = %s, want >= 300ms", elapsed)
	}
	if elapsed >= lockTestWatchdog {
		t.Errorf("elapsed = %s, want < %s (watchdog)", elapsed, lockTestWatchdog)
	}
}
