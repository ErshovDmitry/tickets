package cli

// T-0159 A2: focused CLI tests for `ticket recover`. Every run uses a
// /tmp-style t.TempDir() sandbox — never the repository's tickets/ — and
// drives the command through cli.Run (the real dispatch, so the writable
// store opening is exercised too). The crash pair is produced without
// touching package internals of store: create a ticket, keep its bytes, run a
// real set (which leaves old+new transiently), then restore the old file —
// byte-for-byte the state an interrupted status commit leaves behind.

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// recoverEnv is the sandbox environment: an isolated tickets dir and the RU
// language (the ticket language of this project).
func recoverEnv(dir string) map[string]string {
	return map[string]string{"TICKETS_DIR": dir, "TICKET_LANG": "ru", "USER": "tester"}
}

// resolvedTempDir returns a t.TempDir with symlinks resolved, mirroring
// paths.Resolve (EvalSymlinks) used by the CLI before the store opens the
// directory: macOS t.TempDir() is /var/..., a symlink to /private/var/...,
// so any test comparing a printed path byte-for-byte must use the
// canonical spelling.
func resolvedTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks(t.TempDir()): %v", err)
	}
	return dir
}

// runCLI drives one command through the real dispatch and returns both
// streams and the exit code.
func runRecoverCLI(t *testing.T, dir string, args ...string) (string, string, int) {
	t.Helper()
	var so, se bytes.Buffer
	code := Run(args, recoverEnv(dir), &so, &se)
	return so.String(), se.String(), code
}

// withRecoverSeams installs the stdin and TTY seams for one test and restores
// them afterwards: the prompt must never read the real terminal in tests.
func withRecoverSeams(t *testing.T, stdin string, tty bool) {
	t.Helper()
	prevIn, prevTTY := stdinReader, recoverPromptTTY
	t.Cleanup(func() { stdinReader, recoverPromptTTY = prevIn, prevTTY })
	stdinReader = strings.NewReader(stdin)
	recoverPromptTTY = func() bool { return tty }
}

// crashPair seeds ticket 1 and leaves the interrupted-commit state on disk:
// T-0001-open.md (stale) plus T-0001-done.md (old + one journal record).
func crashPair(t *testing.T, dir string) (oldPath, newPath string) {
	t.Helper()
	if _, se, code := runRecoverCLI(t, dir, "new", "crash ticket"); code != 0 {
		t.Fatalf("seed new: code=%d stderr=%q", code, se)
	}
	oldPath = filepath.Join(dir, "T-0001-open.md")
	oldBytes, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatalf("read old: %v", err)
	}
	if _, se, code := runRecoverCLI(t, dir, "set", "1", "done", "transition"); code != 0 {
		t.Fatalf("seed set: code=%d stderr=%q", code, se)
	}
	newPath = filepath.Join(dir, "T-0001-done.md")
	if err := os.WriteFile(oldPath, oldBytes, 0o644); err != nil {
		t.Fatalf("restore old: %v", err)
	}
	return oldPath, newPath
}

// treeState snapshots every file (relative path → contents) under dir,
// including archive/, so a refusal can be proven to have changed nothing.
func treeState(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel(dir, p)
		if relErr != nil {
			return relErr
		}
		b, readErr := os.ReadFile(p)
		if readErr != nil {
			return readErr
		}
		out[rel] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", dir, err)
	}
	return out
}

// assertFileMetaUnchanged fails when the kept file's mode or mtime changed.
// treeState snapshots contents only, so a rewrite (or a chmod) of the kept
// file would be invisible to it.
func assertFileMetaUnchanged(t *testing.T, path string, before fs.FileInfo) {
	t.Helper()
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat kept file %s after: %v", path, err)
	}
	if after.Mode() != before.Mode() {
		t.Errorf("kept file mode changed: %v -> %v", before.Mode(), after.Mode())
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("kept file mtime changed: %v -> %v", before.ModTime(), after.ModTime())
	}
}

// assertSameTree fails when the on-disk tree differs from before.
func assertSameTree(t *testing.T, dir string, before map[string]string) {
	t.Helper()
	after := treeState(t, dir)
	if len(after) != len(before) {
		t.Fatalf("tree changed: %d files before, %d after (%v)", len(before), len(after), after)
	}
	for k, v := range before {
		if after[k] != v {
			t.Fatalf("file %s changed by a refusal", k)
		}
	}
}

func TestRecoverYesKeepsNewAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	oldPath, newPath := crashPair(t, dir)
	keepBytes, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read new: %v", err)
	}
	stdout, stderr, code := runRecoverCLI(t, dir, "recover", "1", "T-0001-done.md", "--yes")
	if code != 0 {
		t.Fatalf("recover: code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, newPath) {
		t.Errorf("stdout must name the kept file %q, got %q", newPath, stdout)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("stale old file must be removed, stat err: %v", err)
	}
	after, err := os.ReadFile(newPath)
	if err != nil || !bytes.Equal(after, keepBytes) {
		t.Errorf("kept file must not be rewritten (err=%v)", err)
	}
	stdout, stderr, code = runRecoverCLI(t, dir, "recover", "1", "T-0001-done.md", "--yes")
	if code != 0 || !strings.Contains(stdout, newPath) {
		t.Fatalf("idempotent repeat: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

// TestRecoverLoneCleanTicketNoOp pins the documented CLI postcondition
// (T-0162): a lone file that already carries the selected name — an ordinary
// ticket without any crash pair — is exit 0 with the file named on stdout,
// the tree is byte-identical afterwards, and a repeat stays a success.
func TestRecoverLoneCleanTicketNoOp(t *testing.T) {
	dir := resolvedTempDir(t)
	_, stderr, code := runRecoverCLI(t, dir, "new", "lone ticket")
	if code != 0 {
		t.Fatalf("seed new: code=%d stderr=%q", code, stderr)
	}
	path := filepath.Join(dir, "T-0001-open.md")
	before := treeState(t, dir)
	metaBefore, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat kept file before: %v", err)
	}
	wantStdout := path + "\n"
	stdout, stderr, code := runRecoverCLI(t, dir, "recover", "1", "T-0001-open.md", "--yes")
	if code != 0 {
		t.Fatalf("lone no-op: code=%d want 0 (stderr=%q)", code, stderr)
	}
	if stdout != wantStdout {
		t.Errorf("stdout must be exactly the kept path line %q, got %q", wantStdout, stdout)
	}
	assertSameTree(t, dir, before)
	assertFileMetaUnchanged(t, path, metaBefore)
	stdout, stderr, code = runRecoverCLI(t, dir, "recover", "1", "T-0001-open.md", "--yes")
	if code != 0 {
		t.Fatalf("repeat lone no-op: code=%d want 0 (stderr=%q)", code, stderr)
	}
	if stdout != wantStdout {
		t.Errorf("repeat stdout must be exactly the kept path line %q, got %q", wantStdout, stdout)
	}
	assertSameTree(t, dir, before)
	assertFileMetaUnchanged(t, path, metaBefore)
}

func TestRecoverPromptYesAndNo(t *testing.T) {
	dir := t.TempDir()
	oldPath, newPath := crashPair(t, dir)
	before := treeState(t, dir)

	withRecoverSeams(t, "no\n", true)
	stdout, stderr, code := runRecoverCLI(t, dir, "recover", "1", "T-0001-done.md")
	if code != 1 {
		t.Fatalf("refused confirmation: code=%d want 1 (stderr=%q)", code, stderr)
	}
	for _, want := range []string{newPath, oldPath, "yes/no"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("prompt must show %q, got %q", want, stdout)
		}
	}
	assertSameTree(t, dir, before)

	withRecoverSeams(t, "yes\n", true)
	if _, stderr, code := runRecoverCLI(t, dir, "recover", "1", "T-0001-done.md"); code != 0 {
		t.Fatalf("accepted confirmation: code=%d stderr=%q", code, stderr)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("old file must be removed after yes, stat err: %v", err)
	}
}

func TestRecoverNonTTYWithoutYesRefuses(t *testing.T) {
	dir := t.TempDir()
	crashPair(t, dir)
	before := treeState(t, dir)

	withRecoverSeams(t, "yes\n", false)
	stdout, stderr, code := runRecoverCLI(t, dir, "recover", "1", "T-0001-done.md")
	if code != 1 {
		t.Fatalf("non-TTY without --yes: code=%d want 1", code)
	}
	if !strings.Contains(stderr, "--yes") {
		t.Errorf("refusal must point at --yes, got %q", stderr)
	}
	if strings.Contains(stdout, "yes/no") {
		t.Errorf("no prompt may be shown without a terminal: %q", stdout)
	}
	assertSameTree(t, dir, before)
}
