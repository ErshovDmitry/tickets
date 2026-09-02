package store

// Read-only store opening (T-0012): NewReadOnly validates the directory
// without the write probe, so List/Find work on a read-only directory
// and never mutate it. Tests relying on chmod-based read-only semantics
// skip for root (permissions bypassed) and on Windows (chmod gives no
// read-only semantics there) — runtime skips, no build tags.

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// skipRootOrWindows skips chmod-dependent tests where they cannot hold.
func skipRootOrWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("chmod provides no read-only semantics on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permission checks")
	}
}

// makeReadOnlyDir builds a directory holding one valid ticket and chmods
// it 0555 (restored on cleanup so TempDir removal succeeds).
func makeReadOnlyDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Seeding takes the advisory lock (Create → withLock), leaving a
	// .lock behind; remove it while the dir is still writable so the
	// post-List assertion below proves List itself creates none.
	if err := os.Remove(filepath.Join(dir, ".lock")); err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	return dir
}

func TestNewReadOnly_ListFindOnReadOnlyDir(t *testing.T) {
	skipRootOrWindows(t)
	ro, err := NewReadOnly(makeReadOnlyDir(t))
	if err != nil {
		t.Fatalf("NewReadOnly on read-only dir: %v", err)
	}
	tickets, warns := ro.List()
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	if len(tickets) != 1 || tickets[0].Number != 1 {
		t.Fatalf("expected exactly ticket #1, got %+v", tickets)
	}
	tk, err := ro.Find(1)
	if err != nil || tk.Number != 1 {
		t.Fatalf("Find(1) on read-only dir: tk=%+v err=%v", tk, err)
	}
}

func TestNew_ProbeFailsOnReadOnlyDir(t *testing.T) {
	skipRootOrWindows(t)
	if _, err := New(makeReadOnlyDir(t)); err == nil {
		t.Fatal("expected probe error from New on read-only dir")
	}
}

// TestNewReadOnly_ListCreatesNoLock pins the read-only guarantee at the
// lock level: List must not create .lock (lock.Acquire uses O_CREATE).
func TestNewReadOnly_ListCreatesNoLock(t *testing.T) {
	skipRootOrWindows(t)
	dir := makeReadOnlyDir(t)
	ro, err := NewReadOnly(dir)
	if err != nil {
		t.Fatalf("NewReadOnly: %v", err)
	}
	ro.List()
	if _, err := os.Lstat(filepath.Join(dir, ".lock")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("List must not create .lock on a read-only dir; Lstat err=%v", err)
	}
}

// TestNewReadOnly_ValidationErrors pins the validateDir error contract on
// the read-only constructor: the same messages New produces.
func TestNewReadOnly_ValidationErrors(t *testing.T) {
	if _, err := NewReadOnly(filepath.Join(t.TempDir(), "missing")); err == nil || !strings.HasPrefix(err.Error(), "store: stat ") {
		t.Fatalf("missing dir: want 'store: stat ...' error, got %v", err)
	}
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewReadOnly(file); err == nil || !strings.HasSuffix(err.Error(), " is not a directory") {
		t.Fatalf("file path: want 'is not a directory' error, got %v", err)
	}
}

// TestNewReadOnly_MutationRejected pins the runtime guard: a Store from
// NewReadOnly rejects Create and SetStatus with ErrReadOnly (no panic,
// no silent write), and the directory keeps exactly the seeded ticket.
func TestNewReadOnly_MutationRejected(t *testing.T) {
	dir := t.TempDir()
	seed, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := seed.Create(fakeTicket(0)); err != nil {
		t.Fatalf("seed Create: %v", err)
	}
	ro, err := NewReadOnly(dir)
	if err != nil {
		t.Fatalf("NewReadOnly: %v", err)
	}
	if _, err := ro.Create(fakeTicket(0)); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Create on read-only store: want ErrReadOnly, got %v", err)
	}
	if _, err := ro.SetStatus(1, domain.StatusWip, "t", "must fail"); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("SetStatus on read-only store: want ErrReadOnly, got %v", err)
	}
	tickets, warns := ro.List()
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	if len(tickets) != 1 || tickets[0].Number != 1 || tickets[0].Status != domain.StatusOpen {
		t.Fatalf("read-only store mutated the directory: %+v", tickets)
	}
}
