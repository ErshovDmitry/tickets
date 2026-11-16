package cli

// T-0189 W2-A2: CLI coverage for `ticket recover` against an interrupted
// Delete (main source + tickets/deleted/ successor). The deleted successor
// is produced by the real store.Delete (the delete command itself lands in
// a later wave); the stale source is then restored byte-for-byte — the same
// construction idiom as crashPair in cmd_recover_test.go. Everything runs
// in a t.TempDir() sandbox, never the repository's tickets/.

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// deletedCrashPair leaves the interrupted-Delete state on disk:
// T-0001-open.md (stale source, restored byte-for-byte) plus
// tickets/deleted/T-0001-deleted.md (the real Delete successor, one
// trailing open→deleted journal record).
func deletedCrashPair(t *testing.T, dir string) (srcPath, deletedPath string) {
	t.Helper()
	if _, se, code := runRecoverCLI(t, dir, "new", "crash ticket"); code != 0 {
		t.Fatalf("seed new: code=%d stderr=%q", code, se)
	}
	srcPath = filepath.Join(dir, "T-0001-open.md")
	srcBytes, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	st, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	deletedPath, err = st.Delete("1", false, "tester")
	if err != nil {
		t.Fatalf("store.Delete: %v", err)
	}
	if err := os.WriteFile(srcPath, srcBytes, 0o644); err != nil {
		t.Fatalf("restore stale source: %v", err)
	}
	return srcPath, deletedPath
}

func TestRecoverDeletedYesKeepsDeletedAndIsIdempotent(t *testing.T) {
	dir := resolvedTempDir(t)
	srcPath, deletedPath := deletedCrashPair(t, dir)
	keepBytes, err := os.ReadFile(deletedPath)
	if err != nil {
		t.Fatalf("read deleted: %v", err)
	}
	stdout, stderr, code := runRecoverCLI(t, dir, "recover", "1", "T-0001-deleted.md", "--yes")
	if code != 0 {
		t.Fatalf("recover: code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, deletedPath) {
		t.Errorf("stdout must name the kept deleted file %q, got %q", deletedPath, stdout)
	}
	if _, err := os.Stat(srcPath); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stale source must be removed, stat err: %v", err)
	}
	after, err := os.ReadFile(deletedPath)
	if err != nil || !bytes.Equal(after, keepBytes) {
		t.Errorf("deleted file must stay byte-identical (err=%v)", err)
	}
	// Idempotent repeat: the deleted file alone remains — a no-op success.
	stdout, stderr, code = runRecoverCLI(t, dir, "recover", "1", "T-0001-deleted.md", "--yes")
	if code != 0 || !strings.Contains(stdout, deletedPath) {
		t.Fatalf("idempotent repeat: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestRecoverDeletedKeepOldRefusedCLI(t *testing.T) {
	dir := resolvedTempDir(t)
	srcPath, _ := deletedCrashPair(t, dir)
	before := treeState(t, dir)
	stdout, stderr, code := runRecoverCLI(t, dir, "recover", "1", "T-0001-open.md", "--yes")
	if code != 1 {
		t.Fatalf("keep-old must refuse with exit 1: code=%d stderr=%q", code, stderr)
	}
	if stdout != "" {
		t.Errorf("a refusal must not print a kept path, got %q", stdout)
	}
	// The localized reason distinguishes keep-old from divergence and from a
	// plain selection mismatch: pin the message that names both halves.
	want := fmt.Sprintf(recoverStringsFor(domain.LangRU).keepOldFmt, 1, "T-0001-open.md", "T-0001-deleted.md")
	if stderr != want+"\n" {
		t.Errorf("keep-old stderr = %q, want byte-exact %q", stderr, want+"\n")
	}
	if _, err := os.Stat(srcPath); err != nil {
		t.Errorf("stale source must survive a keep-old refusal: %v", err)
	}
	assertSameTree(t, dir, before)
}

func TestRecoverDeletedDivergentRefusedCLI(t *testing.T) {
	dir := resolvedTempDir(t)
	_, deletedPath := deletedCrashPair(t, dir)
	body, err := os.ReadFile(deletedPath)
	if err != nil {
		t.Fatalf("read deleted: %v", err)
	}
	divergent := bytes.Replace(body, []byte("crash ticket"), []byte("tampered ticket"), 1)
	if bytes.Equal(divergent, body) {
		t.Fatal("tamper marker not found in deleted body")
	}
	if err := os.WriteFile(deletedPath, divergent, 0o644); err != nil {
		t.Fatalf("tamper deleted: %v", err)
	}
	before := treeState(t, dir)
	stdout, stderr, code := runRecoverCLI(t, dir, "recover", "1", "T-0001-deleted.md", "--yes")
	if code != 2 {
		t.Fatalf("divergent must exit 2: code=%d stderr=%q", code, stderr)
	}
	if stdout != "" {
		t.Errorf("a refusal must not print a kept path, got %q", stdout)
	}
	// Divergent contents are their own reason, not a generic state refusal.
	want := fmt.Sprintf(recoverStringsFor(domain.LangRU).divergentFmt, 1, "T-0001-deleted.md", "T-0001-open.md")
	if stderr != want+"\n" {
		t.Errorf("divergent stderr = %q, want byte-exact %q", stderr, want+"\n")
	}
	assertSameTree(t, dir, before)
}

func TestRecoverDeletedTTYConfirm(t *testing.T) {
	dir := resolvedTempDir(t)
	srcPath, deletedPath := deletedCrashPair(t, dir)
	withRecoverSeams(t, "y\n", true)
	stdout, stderr, code := runRecoverCLI(t, dir, "recover", "1", "T-0001-deleted.md")
	if code != 0 {
		t.Fatalf("TTY-confirmed recover: code=%d stderr=%q", code, stderr)
	}
	// The prompt names both halves: the kept deleted successor and the
	// discarded stale source.
	if !strings.Contains(stdout, deletedPath) {
		t.Errorf("prompt must name the kept deleted file %q, got %q", deletedPath, stdout)
	}
	if !strings.Contains(stdout, srcPath) {
		t.Errorf("prompt must name the discarded source %q, got %q", srcPath, stdout)
	}
	if _, err := os.Stat(srcPath); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stale source must be removed after yes, stat err: %v", err)
	}
}
