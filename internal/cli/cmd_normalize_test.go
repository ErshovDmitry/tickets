package cli_test

// CLI-level normalize tests (T-0125). The store-level contract (what is
// rewritten, guard skips, atomicity) is pinned in internal/store; here we
// check the command surface: exit codes, the stdout path list, warnings on
// stderr and the read-only dry run.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/cli"
)

// normalizeEnv creates an empty tickets directory and the env that points the
// CLI at it; the directory is env["TICKETS_DIR"].
func normalizeEnv(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{"TICKETS_DIR": t.TempDir()}
}

// runNormalizeCLI runs the CLI, dying on an unexpected exit code. It returns
// stdout, stderr and the code for the callers that assert the code.
func runNormalizeCLI(t *testing.T, args []string, env map[string]string) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run(args, env, &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

// seedNormalizeTicket creates one ticket through the CLI and returns its path.
func seedNormalizeTicket(t *testing.T, env map[string]string, title string, extra ...string) string {
	t.Helper()
	args := append([]string{"new", title, "-t", "BUG", "-w", "tester"}, extra...)
	stdout, stderr, code := runNormalizeCLI(t, args, env)
	if code != 0 {
		t.Fatalf("new = %d, want 0; stderr: %q", code, stderr)
	}
	return strings.TrimSpace(stdout)
}

// delintTicket rewrites the ticket at path into the pre-fix shape: the blank
// line before and after every "## " header is dropped (the MD022 fix added
// them), which is exactly what normalize has to restore.
func delintTicket(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.Split(string(body), "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if line == "" && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "## ") {
			continue
		}
		out = append(out, line)
		if strings.HasPrefix(line, "## ") && i+1 < len(lines) && lines[i+1] == "" {
			i++
		}
	}
	legacy := []byte(strings.Join(out, "\n"))
	if err := os.WriteFile(path, legacy, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return legacy
}

// TestCmdNormalize_PrintsChangedPaths pins the output shape: one absolute path
// per line on stdout, nothing on stderr, exit 0, and the file canonical again.
func TestCmdNormalize_PrintsChangedPaths(t *testing.T) {
	env := normalizeEnv(t)
	path := seedNormalizeTicket(t, env, "Legacy ticket")
	legacy := delintTicket(t, path)

	stdout, stderr, code := runNormalizeCLI(t, []string{"normalize"}, env)
	if code != 0 {
		t.Fatalf("normalize = %d, want 0; stderr: %q", code, stderr)
	}
	if got, want := stdout, path+"\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(body, legacy) {
		t.Errorf("file was not rewritten")
	}
	if !strings.Contains(string(body), "\n\n## Summary") {
		t.Errorf("canonical blank line before the header is missing:\n%s", body)
	}
}

// TestCmdNormalize_NoChangesEmptyStdout pins the second run: nothing to do
// means empty stdout and exit 0, so the command is safe to script.
func TestCmdNormalize_NoChangesEmptyStdout(t *testing.T) {
	env := normalizeEnv(t)
	path := seedNormalizeTicket(t, env, "Canonical ticket")
	delintTicket(t, path)

	if _, _, code := runNormalizeCLI(t, []string{"normalize"}, env); code != 0 {
		t.Fatalf("first normalize = %d, want 0", code)
	}
	stdout, stderr, code := runNormalizeCLI(t, []string{"normalize"}, env)
	if code != 0 {
		t.Fatalf("second normalize = %d, want 0; stderr: %q", code, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

// TestCmdNormalize_DryRunWritesNothing pins --dry-run: the path is printed,
// the bytes are not touched.
func TestCmdNormalize_DryRunWritesNothing(t *testing.T) {
	env := normalizeEnv(t)
	path := seedNormalizeTicket(t, env, "Dry run ticket")
	legacy := delintTicket(t, path)

	stdout, stderr, code := runNormalizeCLI(t, []string{"normalize", "--dry-run"}, env)
	if code != 0 {
		t.Fatalf("normalize --dry-run = %d, want 0; stderr: %q", code, stderr)
	}
	if got, want := stdout, path+"\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, legacy) {
		t.Errorf("--dry-run rewrote the file")
	}
}

// TestCmdNormalize_ProjectFilter pins -P at the CLI boundary: only the named
// project is rewritten, matched case-sensitively against the ticket's own
// Project field.
func TestCmdNormalize_ProjectFilter(t *testing.T) {
	env := normalizeEnv(t)
	alpha := seedNormalizeTicket(t, env, "Alpha ticket", "-P", "alpha")
	beta := seedNormalizeTicket(t, env, "Beta ticket", "-P", "beta")
	alphaLegacy := delintTicket(t, alpha)
	betaLegacy := delintTicket(t, beta)

	stdout, stderr, code := runNormalizeCLI(t, []string{"normalize", "-P", "alpha"}, env)
	if code != 0 {
		t.Fatalf("normalize -P alpha = %d, want 0; stderr: %q", code, stderr)
	}
	if got, want := stdout, alpha+"\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if body, err := os.ReadFile(beta); err != nil || !bytes.Equal(body, betaLegacy) {
		t.Errorf("filtered-out ticket was rewritten (err %v)", err)
	}
	if body, err := os.ReadFile(alpha); err != nil || bytes.Equal(body, alphaLegacy) {
		t.Errorf("selected ticket was not rewritten (err %v)", err)
	}
}

// TestCmdNormalize_WarningsExitZero pins the non-fatal class: a file the guard
// refuses to rewrite is reported on stderr and the command still exits 0.
func TestCmdNormalize_WarningsExitZero(t *testing.T) {
	env := normalizeEnv(t)
	dir := env["TICKETS_DIR"]
	good := seedNormalizeTicket(t, env, "Rewritable ticket")
	delintTicket(t, good)
	bashEra := "# T-0002 · BUG: Bash era\n\n" +
		"- Статус: open\n- Приоритет: normal\n- Проект: tickets\n\n## Краткое описание\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(dir, "T-0002-open.md"), []byte(bashEra), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runNormalizeCLI(t, []string{"normalize"}, env)
	if code != 0 {
		t.Fatalf("normalize = %d, want 0; stderr: %q", code, stderr)
	}
	if got, want := stdout, good+"\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if !strings.Contains(stderr, "ticket: T-0002-open.md:") ||
		!strings.Contains(stderr, "bash-era metadata labels") {
		t.Errorf("stderr = %q, want the guard warning", stderr)
	}
}

// TestCmdNormalize_DryRunOnReadOnlyDir pins the read-only preview: dry-run
// takes no write probe, so it works on a 0555 tickets directory.
func TestCmdNormalize_DryRunOnReadOnlyDir(t *testing.T) {
	skipRootOrWindows(t)
	env := normalizeEnv(t)
	dir := env["TICKETS_DIR"]
	path := seedNormalizeTicket(t, env, "Read-only preview")
	legacy := delintTicket(t, path)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cerr := os.Chmod(dir, 0o755); cerr != nil {
			t.Errorf("restore dir mode: %v", cerr)
		}
	})

	stdout, stderr, code := runNormalizeCLI(t, []string{"normalize", "--dry-run"}, env)
	if code != 0 {
		t.Fatalf("normalize --dry-run = %d, want 0; stderr: %q", code, stderr)
	}
	if got, want := stdout, path+"\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, legacy) {
		t.Errorf("dry run on a read-only dir rewrote the ticket")
	}
}

// TestCmdNormalize_WriteErrorExit1 pins the failure surface: a write error is
// exit 1 with an empty stdout, the already-canonical active ticket keeps its
// bytes and mtime, and the offending archive ticket stays byte-identical.
func TestCmdNormalize_WriteErrorExit1(t *testing.T) {
	skipRootOrWindows(t)
	env := normalizeEnv(t)
	dir := env["TICKETS_DIR"]
	active := seedNormalizeTicket(t, env, "Active ticket") // canonical by construction
	seedNormalizeTicket(t, env, "Archived ticket")
	if _, _, code := runNormalizeCLI(t, []string{"set", "2", "done", "closing"}, env); code != 0 {
		t.Fatalf("set = %d, want 0", code)
	}
	if _, stderr, code := runNormalizeCLI(t, []string{"archive"}, env); code != 0 {
		t.Fatalf("archive = %d, want 0; stderr: %q", code, stderr)
	}
	archiveDir := filepath.Join(dir, "archive")
	archived := filepath.Join(archiveDir, "T-0002-done.md")
	archivedLegacy := delintTicket(t, archived)
	activeBefore, err := os.Stat(active)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(archiveDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cerr := os.Chmod(archiveDir, 0o755); cerr != nil {
			t.Errorf("restore archive mode: %v", cerr)
		}
	})

	stdout, stderr, code := runNormalizeCLI(t, []string{"normalize"}, env)
	if code != 1 {
		t.Fatalf("normalize = %d, want 1; stderr: %q", code, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "ticket: ") {
		t.Errorf("stderr = %q, want a ticket: error line", stderr)
	}
	activeAfter, err := os.Stat(active)
	if err != nil {
		t.Fatal(err)
	}
	if !activeAfter.ModTime().Equal(activeBefore.ModTime()) {
		t.Errorf("canonical active ticket was rewritten before the failure")
	}
	if body, err := os.ReadFile(archived); err != nil || !bytes.Equal(body, archivedLegacy) {
		t.Errorf("archive ticket changed despite the write failure (err %v)", err)
	}
}
