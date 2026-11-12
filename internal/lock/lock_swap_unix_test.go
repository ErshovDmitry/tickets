//go:build unix

package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestAcquireSwapRetriesGivesUpAfterMaxSwapRetries pins the swap-retry
// bound of acquireVia: every open hands back an fd whose file is then
// unlinked and recreated under the same path, so checkCurrent reports a
// swap on every iteration. The hook fails the test as soon as the loop
// asks for one open more than maxSwapRetries, so a broken bound (inverted
// counter or a strict ">" comparison) fails fast instead of spinning.
func TestAcquireSwapRetriesGivesUpAfterMaxSwapRetries(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".lock")
	calls := 0

	open := func(p string) (*os.File, error) {
		calls++
		if calls > maxSwapRetries {
			t.Fatalf("open called %d times, want at most %d (swap retry bound not enforced)", calls, maxSwapRetries)
		}
		if p != path {
			t.Fatalf("open path = %q, want %q", p, path)
		}
		f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			t.Fatalf("open lock file: %v", err)
		}
		// Unlink and recreate the file so f keeps guarding an unlinked
		// inode: checkCurrent must then report a swap.
		if err := os.Remove(p); err != nil {
			_ = f.Close()
			t.Fatalf("remove lock file: %v", err)
		}
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			_ = f.Close()
			t.Fatalf("recreate lock file: %v", err)
		}
		return f, nil
	}

	lk, err := acquireVia(path, open)

	if lk != nil {
		t.Errorf("acquireVia returned Locker %v, want nil", lk)
	}
	want := fmt.Sprintf("lock: %s: lock file keeps being replaced; gave up after %d attempts", path, maxSwapRetries)
	if err == nil || err.Error() != want {
		t.Fatalf("acquireVia error = %v, want %q", err, want)
	}
	if calls != maxSwapRetries {
		t.Errorf("open calls = %d, want %d", calls, maxSwapRetries)
	}
}

// TestAcquireOpenErrorReturnsNilRelease pins the error branch of the
// Acquire wrapper: an open failure surfaces as an error and the release
// closure stays nil, so a caller's defer sees nothing to release.
func TestAcquireOpenErrorReturnsNilRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", ".lock")

	release, err := Acquire(path)

	if err == nil {
		t.Fatal("Acquire returned nil error for an unopenable path")
	}
	if release != nil {
		t.Error("Acquire returned non-nil release alongside an error")
	}
}
