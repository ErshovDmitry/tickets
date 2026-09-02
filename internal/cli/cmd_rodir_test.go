package cli_test

// CLI-level read-only tests (T-0012): `list` must succeed on a read-only
// tickets directory without mutating it (NewReadOnly, no probe, no
// .lock), while `new` must fail with the write-probe error. Same skip
// conditions as the store-level ro-dir tests: root bypasses permissions
// and Windows chmod has no read-only semantics.

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ticket/internal/cli"
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

// makeReadOnlyTicketsDir creates a tickets dir with one open ticket via
// the CLI itself, then chmods it 0555 (restored on cleanup).
func makeReadOnlyTicketsDir(t *testing.T) map[string]string {
	t.Helper()
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"new", "seed ticket"}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(new) = %d, want 0; stderr: %q", code, stderr.String())
	}
	// `new` takes the advisory lock and leaves .lock behind; remove it
	// while the dir is still writable so the post-List assertion proves
	// list itself creates none.
	if err := os.Remove(filepath.Join(dir, ".lock")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	return env
}

// TestRun_ListOnReadOnlyDir pins read-only parity at the CLI boundary:
// list exits 0 on a read-only directory and leaves it untouched.
func TestRun_ListOnReadOnlyDir(t *testing.T) {
	skipRootOrWindows(t)
	env := makeReadOnlyTicketsDir(t)
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"list"}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(list) = %d, want 0; stderr: %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "T-0001") {
		t.Errorf("stdout must list the ticket; got %q", stdout.String())
	}
	dir := env["TICKETS_DIR"]
	if _, err := os.Lstat(filepath.Join(dir, ".lock")); !os.IsNotExist(err) {
		t.Errorf("list must not create .lock; Lstat err=%v", err)
	}
}

// TestRun_NewOnReadOnlyDirFails pins the writable path: new on a
// read-only directory exits 1 with the probe error on stderr and writes
// no ticket file.
func TestRun_NewOnReadOnlyDirFails(t *testing.T) {
	skipRootOrWindows(t)
	env := makeReadOnlyTicketsDir(t)
	dir := env["TICKETS_DIR"]
	before, err := filepath.Glob(filepath.Join(dir, "T-*.md"))
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"new", "must fail"}, env, &stdout, &stderr); code != 1 {
		t.Fatalf("Run(new) = %d, want 1; stderr: %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "store: probe") {
		t.Errorf("stderr must carry the probe error; got %q", stderr.String())
	}
	after, err := filepath.Glob(filepath.Join(dir, "T-*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Errorf("new must not write a ticket on a read-only dir: before=%v after=%v", before, after)
	}
}
