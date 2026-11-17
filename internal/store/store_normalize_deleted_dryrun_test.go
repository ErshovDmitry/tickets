package store

// T-0189 W3-C1a: dry-run contracts for the deleted normalize pass
// (split out of store_normalize_deleted_test.go to keep every file inside
// the 300-line budget).

import (
	"bytes"
	"os"
	"testing"
)

// TestNormalize_DeletedDryRunWritesNothing pins the preview contract on
// the deleted pass: --dry-run reports the path that would change and
// leaves the bytes alone.
func TestNormalize_DeletedDryRunWritesNothing(t *testing.T) {
	s, dir := newStore(t)
	tk := normalizeTicket("tickets")
	tk.Number = 1
	_, path := deletedFixture(t, dir, tk)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	res, err := s.Normalize(NormalizeOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	wantChanged(t, res.Changed, path)
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, before) {
		t.Errorf("dry run rewrote the deleted file")
	}
}

// TestNormalize_DeletedDryRunOnReadOnlyDir pins the read-only parity: a
// dry run on a read-only directory (opened via NewReadOnly, which skips
// the write probe) still reports the deleted path — no lock, no byte
// written, deleted/ untouched.
func TestNormalize_DeletedDryRunOnReadOnlyDir(t *testing.T) {
	skipRootOrWindows(t)
	dir := t.TempDir()
	if _, err := New(dir); err != nil {
		t.Fatalf("New: %v", err)
	}
	tk := normalizeTicket("tickets")
	tk.Number = 1
	deletedDir, path := deletedFixture(t, dir, tk)
	if err := os.Chmod(deletedDir, 0o555); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o755)
		_ = os.Chmod(deletedDir, 0o755)
	})

	ro, err := NewReadOnly(dir)
	if err != nil {
		t.Fatalf("NewReadOnly: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	res, err := ro.Normalize(NormalizeOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Normalize dry-run: %v", err)
	}
	wantChanged(t, res.Changed, path)
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, before) {
		t.Errorf("dry run on a read-only dir rewrote the deleted file")
	}
}
