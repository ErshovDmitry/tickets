package cli_test

// T-0187 i18n coverage for `ticket edit` (runEN/assertNoCyrillic pattern
// of i18n_coverage_test.go): every new message renders in EN under
// TICKET_LANG=en with no Cyrillic leak, success stays silent, and the
// global --tickets-dir= glued form keeps working for the new command.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/cli"
)

// TestEditErrorsLocalizeToEN runs edit through cli.Run with TICKET_LANG=en,
// triggers each error path and asserts the EN substring, the exit code and
// the absence of ANY Cyrillic in both streams.
func TestEditErrorsLocalizeToEN(t *testing.T) {
	dir := mustEval(t, t.TempDir())
	if _, stderr, code := runEN(t, dir, "new", "seed ticket"); code != 0 {
		t.Fatalf("seed new = %d; stderr %q", code, stderr)
	}
	// Move the project to a fixed value so its no-op message is pinned.
	if _, stderr, code := runEN(t, dir, "edit", "1", "--project", "alpha"); code != 0 {
		t.Fatalf("seed edit = %d; stderr %q", code, stderr)
	}

	cases := []struct {
		name    string
		args    []string
		wantSub string
		wantCod int
	}{
		{"unknown flag", []string{"edit", "1", "--bogus", "x"}, "unknown flag for edit command: --bogus", 1},
		{"short flag", []string{"edit", "1", "-t", "BUG"}, "unknown flag for edit command: -t", 1},
		{"glued form", []string{"edit", "1", "--title=x"}, "unknown flag for edit command: --title=x", 1},
		{"missing value", []string{"edit", "1", "--title"}, "flag --title requires a value", 1},
		{"extra positional", []string{"edit", "1", "--title", "x", "y"}, "unknown argument: y", 1},
		{"invalid type", []string{"edit", "1", "--type", "BAD"}, "type must be one of", 1},
		{"invalid priority", []string{"edit", "1", "--priority", "BAD"}, "priority must be one of", 1},
		{"empty title", []string{"edit", "1", "--title", "  "}, "title must not be empty", 1},
		{"CTL title", []string{"edit", "1", "--title", "a\x01b"}, "control characters", 1},
		{"dash title", []string{"edit", "1", "--title", "-x"}, "must not start with '-'", 1},
		{"empty project", []string{"edit", "1", "--project", " "}, "project must not be empty", 1},
		{"CRLF project", []string{"edit", "1", "--project", "a\nb"}, "edit value must not contain a line break", 1},
		{"unchanged title", []string{"edit", "1", "--title", "seed ticket"}, "ticket 1 already has title \"seed ticket\"", 1},
		{"unchanged project", []string{"edit", "1", "--project", "alpha"}, "ticket 1 already has project \"alpha\"", 1},
		{"not found", []string{"edit", "42", "--title", "x"}, "not found", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sout, serr, code := runEN(t, dir, tc.args...)
			if code != tc.wantCod {
				t.Fatalf("Run(%q) = %d, want %d; stderr: %q", tc.args, code, tc.wantCod, serr)
			}
			got := serr
			if !strings.Contains(got, tc.wantSub) {
				t.Errorf("stderr = %q, want substring %q", got, tc.wantSub)
			}
			assertNoCyrillic(t, "stdout", sout)
			assertNoCyrillic(t, "stderr", serr)
		})
	}
}

// TestEditSuccessEN pins the silent EN success: exit 0, both streams empty,
// the H1 and project metadata rewritten, one journal line.
func TestEditSuccessEN(t *testing.T) {
	dir := mustEval(t, t.TempDir())
	if _, stderr, code := runEN(t, dir, "new", "seed ticket"); code != 0 {
		t.Fatalf("seed new = %d; stderr %q", code, stderr)
	}

	sout, serr, code := runEN(t, dir, "edit", "1", "--title", "new title", "--project", "beta")
	if code != 0 {
		t.Fatalf("Run = %d, want 0; stderr: %q", code, serr)
	}
	if sout != "" || serr != "" {
		t.Errorf("streams = %q/%q, want both empty", sout, serr)
	}
	assertNoCyrillic(t, "stdout", sout)
	assertNoCyrillic(t, "stderr", serr)

	body, err := os.ReadFile(filepath.Join(dir, "T-0001-open.md"))
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	for _, want := range []string{
		"# T-0001 · BUG: new title",
		"- Project: beta",
		"title: seed ticket → new title",
		"project:",
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}

// TestEditGlobalTicketsDirGlued pins that the global --tickets-dir= form
// (accepted before the command) keeps working with edit.
func TestEditGlobalTicketsDirGlued(t *testing.T) {
	dir := mustEval(t, t.TempDir())
	if _, stderr, code := runEN(t, dir, "new", "seed ticket"); code != 0 {
		t.Fatalf("seed new = %d; stderr %q", code, stderr)
	}

	env := map[string]string{"TICKET_LANG": "en"}
	var stdout, stderr bytes.Buffer
	code := cli.Run([]string{"--tickets-dir=" + dir, "edit", "1", "--title", "glued"}, env, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(--tickets-dir= edit 1) = %d, want 0; stderr: %q", code, stderr.String())
	}
	if stdout.String() != "" || stderr.String() != "" {
		t.Errorf("streams = %q/%q, want both empty", stdout.String(), stderr.String())
	}
	body, err := os.ReadFile(filepath.Join(dir, "T-0001-open.md"))
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	if !strings.Contains(string(body), "# T-0001 · BUG: glued") {
		t.Errorf("H1 not rewritten via the glued global flag:\n%s", body)
	}
}

// TestEditAmbiguousEN pins the EN T-0157 refusal for edit: exit 2, header
// plus both conflicting paths, no Cyrillic.
func TestEditAmbiguousEN(t *testing.T) {
	dir, donePath, openPath := enAmbiguousPair(t)
	stdout, stderr, code := runEN(t, dir, "edit", "1", "--title", "x")
	if code != 2 {
		t.Fatalf("edit 1 = %d, want 2; stderr %q", code, stderr)
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
