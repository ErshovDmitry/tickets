package cli_test

// T-0157 i18n coverage for the ambiguity messages (plan c5 D5.2): every
// new message renders in EN under TICKET_LANG=en with no Cyrillic leak
// (runEN/assertNoCyrillic pattern of i18n_coverage_test.go).

import (
	"path/filepath"
	"strings"
	"testing"
)

// enAmbiguousPair seeds a fresh dir with ticket 1 ambiguous (healthy open
// via runEN new + foreign done) and returns the dir plus both sorted paths.
func enAmbiguousPair(t *testing.T) (dir, donePath, openPath string) {
	t.Helper()
	dir = mustEval(t, t.TempDir())
	if _, stderr, code := runEN(t, dir, "new", "seed ticket"); code != 0 {
		t.Fatalf("seed new = %d; stderr %q", code, stderr)
	}
	writeJSONTicket(t, dir, "T-0001-done.md", 1, "done", "Duplicate", "tickets", "2026-09-08 12:00")
	return dir, filepath.Join(dir, "T-0001-done.md"), filepath.Join(dir, "T-0001-open.md")
}

// TestAmbiguousShowEN pins the EN refusal report for show N: exit 2, the
// errAmbiguousNumber header plus every conflicting path, no Cyrillic.
func TestAmbiguousShowEN(t *testing.T) {
	dir, donePath, openPath := enAmbiguousPair(t)
	stdout, stderr, code := runEN(t, dir, "show", "1")
	if code != 2 {
		t.Fatalf("show 1 = %d, want 2; stderr %q", code, stderr)
	}
	want := "ticket: ticket number 1 is ambiguous: multiple files carry it\n" +
		"  " + donePath + "\n" +
		"  " + openPath + "\n"
	if stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	assertNoCyrillic(t, "stdout", stdout)
	assertNoCyrillic(t, "stderr", stderr)
}

// TestAmbiguousMutatingEN pins the EN refusal header for the mutating
// commands set N and archive N (exit 2, both paths), and the EN inventory
// warning for list (exit 0) and the EN skip warning for archive-all
// (exit 2) — with a Cyrillic sweep on every stream.
func TestAmbiguousMutatingEN(t *testing.T) {
	header := "ticket: ticket number 1 is ambiguous: multiple files carry it"
	dir, donePath, openPath := enAmbiguousPair(t)
	paths := donePath + "\n  " + openPath

	stdout, stderr, code := runEN(t, dir, "set", "1", "wip")
	if code != 2 || !strings.Contains(stderr, header) || !strings.Contains(stderr, paths) {
		t.Errorf("set: code=%d stderr=%q, want 2 with header+paths", code, stderr)
	}
	assertNoCyrillic(t, "set stdout", stdout)
	assertNoCyrillic(t, "set stderr", stderr)

	stdout, stderr, code = runEN(t, dir, "archive", "1")
	if code != 2 || !strings.Contains(stderr, header) || !strings.Contains(stderr, paths) {
		t.Errorf("archive 1: code=%d stderr=%q, want 2 with header+paths", code, stderr)
	}
	assertNoCyrillic(t, "archive stdout", stdout)
	assertNoCyrillic(t, "archive stderr", stderr)

	stdout, stderr, code = runEN(t, dir, "list", "all")
	warn := "ticket: warning: ticket number 1 is ambiguous: " + donePath + ", " + openPath
	if code != 0 || !strings.Contains(stderr, warn) {
		t.Errorf("list: code=%d stderr=%q, want 0 with warning", code, stderr)
	}
	assertNoCyrillic(t, "list stdout", stdout)
	assertNoCyrillic(t, "list stderr", stderr)

	// archive-all: number 2 (the unique done one) is skipped only if it is
	// a duplicate; here number 1 is the duplicate, so exit 2 with one skip
	// warning naming its paths and the unique done file still archived.
	if _, stderr, code := runEN(t, dir, "new", "second"); code != 0 {
		t.Fatalf("fixture new = %d; stderr %q", code, stderr)
	}
	if _, stderr, code := runEN(t, dir, "set", "2", "done"); code != 0 {
		t.Fatalf("fixture set = %d; stderr %q", code, stderr)
	}
	stdout, stderr, code = runEN(t, dir, "archive")
	skipWarn := "ticket: warning: ambiguous ticket number 1 skipped: " + donePath + ", " + openPath
	if code != 2 || !strings.Contains(stderr, skipWarn) {
		t.Errorf("archive-all: code=%d stderr=%q, want 2 with skip warning", code, stderr)
	}
	if !strings.Contains(stdout, filepath.Join(dir, "archive", "T-0002-done.md")) {
		t.Errorf("archive-all stdout = %q, want archived T-0002 path", stdout)
	}
	assertNoCyrillic(t, "archive-all stdout", stdout)
	assertNoCyrillic(t, "archive-all stderr", stderr)
}
