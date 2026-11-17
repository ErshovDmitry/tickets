package cli

// T-0189 W3: recover stays W2 code, but its outcome must compose with the
// W3 deleted-aware list/show: after a crash-pair recovery that KEEPS the
// deleted successor, the ticket is still terminal (list deleted shows it,
// the other modes hide it) and show still resolves it from deleted/.

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// TestRecoverKeepDeletedComposesWithListShowDeleted drives the real
// recover against a source+deleted crash pair and then checks the W3
// read surfaces: list deleted lists the kept file, list all hides it,
// and show resolves the number from deleted/.
func TestRecoverKeepDeletedComposesWithListShowDeleted(t *testing.T) {
	dir := resolvedTempDir(t)
	srcPath, deletedPath := deletedCrashPair(t, dir)
	raw, err := os.ReadFile(deletedPath)
	if err != nil {
		t.Fatalf("read deleted: %v", err)
	}
	if _, se, code := runRecoverCLI(t, dir, "recover", "1", "T-0001-deleted.md", "--yes"); code != 0 {
		t.Fatalf("recover: code=%d stderr=%q", code, se)
	}

	// The stale source is removed by the recovery...
	if _, err := os.Stat(srcPath); !os.IsNotExist(err) {
		t.Errorf("stale source %s still present after recover (err=%v)", srcPath, err)
	}
	// ...and the kept deleted file is left byte-identical.
	after, err := os.ReadFile(deletedPath)
	if err != nil {
		t.Fatalf("read kept deleted: %v", err)
	}
	if !bytes.Equal(after, raw) {
		t.Errorf("kept deleted file %s changed across recover", deletedPath)
	}

	// list deleted shows the kept ticket...
	so, se, code := runRecoverCLI(t, dir, "list", "deleted")
	if code != 0 {
		t.Fatalf("list deleted: code=%d stderr=%q", code, se)
	}
	if !strings.Contains(so, "T-0001") {
		t.Errorf("list deleted stdout = %q, want the kept ticket", so)
	}
	// ...and the ordinary modes still hide it.
	so, se, code = runRecoverCLI(t, dir, "list", "all")
	if code != 0 {
		t.Fatalf("list all: code=%d stderr=%q", code, se)
	}
	if strings.Contains(so, "T-0001") {
		t.Errorf("list all stdout = %q leaks the deleted ticket", so)
	}
	// show resolves the number from deleted/ and prints its exact bytes.
	so, se, code = runRecoverCLI(t, dir, "show", "1")
	if code != 0 {
		t.Fatalf("show 1: code=%d stderr=%q", code, se)
	}
	if so != string(raw) {
		t.Errorf("show stdout = %q, want the deleted file bytes", so)
	}
}
