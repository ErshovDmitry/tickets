package cli_test

// T-0241 A5: table-driven tests for the show --section mode. The plain show
// path is pinned by the older cmd_show*_test.go files; here every case runs in
// a TICKETS_DIR sandbox (t.TempDir) and never against the real tickets/.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/cli"
)

// showTicketSrc builds one ticket whose five sections carry distinct bodies.
// The Summary and Details headers are parameters so a case can vary the
// recognized spelling (RU dictionary, plain EN, free suffix).
func showTicketSrc(num int, status, summaryHeader, detailsHeader, userComments, comments string) string {
	return fmt.Sprintf(`# T-%04d · BUG: x

- Status (Статус): %s
- Priority (Приоритет): normal
- Created (Создан): 2026-09-08 12:00 · by (кем): tester
- Project (Проект): tickets

%s

summary body

%s

details body

## User comments (Комментарии от пользователя)

%s

## Comments (Комментарии)

%s

## Journal (Журнал)

- 2026-09-08 12:00 — ticket created (tester).
`, num, status, summaryHeader, detailsHeader, userComments, comments)
}

// writeShowFile writes src as dir/name, creating dir first.
func writeShowFile(t *testing.T, dir, name, src string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

// runShow runs the CLI with TICKETS_DIR=dir and returns code, stdout, stderr.
func runShow(args []string, dir string) (int, string, string) {
	var stdout, stderr strings.Builder
	code := cli.Run(args, map[string]string{"TICKETS_DIR": dir}, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// newShowSandbox returns an empty sandbox directory and a standard T-0001
// fixture inside it.
func newShowSandbox(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "tickets")
	writeShowFile(t, dir, "T-0001-open.md", showTicketSrc(1, "open", "## Summary (Кратко)", "## Details (Подробности)", "user comments body", "comments body"))
	return dir
}

// TestShowSectionCanonicalAndAliases pins the five canonical tokens, the two
// aliases and the case-insensitive spelling: each prints the section body plus
// exactly one LF and never the header line.
func TestShowSectionCanonicalAndAliases(t *testing.T) {
	dir := newShowSandbox(t)
	cases := []struct {
		name    string
		token   string
		want    string
		wantOut bool
	}{
		{"summary", "summary", "summary body\n", true},
		{"details", "details", "details body\n", true},
		{"user-comments", "user-comments", "user comments body\n", true},
		{"comments", "comments", "comments body\n", true},
		{"journal", "journal", "- 2026-09-08 12:00 — ticket created (tester).\n", true},
		{"alias brief", "brief", "summary body\n", true},
		{"alias user_comments", "user_comments", "user comments body\n", true},
		{"upper SUMMARY", "SUMMARY", "summary body\n", true},
		{"upper USER-COMMENTS", "USER-COMMENTS", "user comments body\n", true},
		{"upper BRIEF", "BRIEF", "summary body\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out, errOut := runShow([]string{"show", "1", "--section", tc.token}, dir)
			if code != 0 {
				t.Fatalf("show --section %s = %d, stderr: %s", tc.token, code, errOut)
			}
			if out != tc.want {
				t.Errorf("stdout = %q, want %q", out, tc.want)
			}
			if strings.Contains(out, "## ") {
				t.Errorf("payload must not carry a header line, got %q", out)
			}
		})
	}
}

// TestShowSectionRefusals pins every refusal of the new mode: an unknown or
// space-separated name (the canonical list is named on stderr), a missing flag
// value, a repeated flag, the --section+--json conflict, a glued flag, a
// missing ticket argument and extra positionals. All of them exit 1 with an
// empty stdout, and none of them reads the ticket.
func TestShowSectionRefusals(t *testing.T) {
	dir := newShowSandbox(t)
	cases := []struct {
		name       string
		args       []string
		wantErrSub string
	}{
		{"name with a space", []string{"show", "1", "--section", "user comments"}, "user-comments"},
		{"garbage name", []string{"show", "1", "--section", "nope"}, "nope"},
		{"missing value", []string{"show", "1", "--section"}, "--section"},
		{"repeated flag", []string{"show", "1", "--section", "summary", "--section", "details"}, "--section"},
		{"conflict with json", []string{"show", "1", "--section", "summary", "--json"}, "--section"},
		{"glued flag", []string{"show", "1", "--section=comments", "--json"}, "--section=comments"},
		{"no ticket argument", []string{"show", "--section", "comments"}, ""},
		{"extra positional", []string{"show", "1", "extra", "--section", "comments"}, "extra"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out, errOut := runShow(tc.args, dir)
			if code != 1 {
				t.Fatalf("code = %d, want 1 (stderr: %s)", code, errOut)
			}
			if out != "" {
				t.Errorf("stdout = %q, want empty", out)
			}
			if !strings.Contains(errOut, tc.wantErrSub) {
				t.Errorf("stderr = %q, want it to contain %q", errOut, tc.wantErrSub)
			}
		})
	}
}

// TestShowSectionPlainModeUnchanged pins the compatibility rule: without a bare
// mode token a glued --section=x is just an extra argument — the historic
// warning on stderr, the whole file on stdout and exit 0.
func TestShowSectionPlainModeUnchanged(t *testing.T) {
	dir := newShowSandbox(t)
	code, out, errOut := runShow([]string{"show", "1", "--section=comments"}, dir)
	if code != 0 {
		t.Fatalf("plain show = %d, want 0 (stderr: %s)", code, errOut)
	}
	if !strings.Contains(out, "## Comments") {
		t.Errorf("plain show must print the whole file, got %q", out)
	}
	if !strings.Contains(errOut, "--section=comments") {
		t.Errorf("stderr = %q, want the extra-args warning naming the argument", errOut)
	}
}

// TestShowSectionLanguageIndependence pins that the payload is the parsed body:
// RU dictionary headers, plain EN headers and an EN header with a free suffix
// select the same text.
func TestShowSectionLanguageIndependence(t *testing.T) {
	cases := []struct{ name, summaryHeader, detailsHeader string }{
		{"ru dictionary", "## Summary (Кратко)", "## Details (Подробности)"},
		{"plain en", "## Summary", "## Details"},
		{"free suffix", "## Summary (Итог)", "## Details (Итог)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "tickets")
			writeShowFile(t, dir, "T-0001-open.md", showTicketSrc(1, "open", tc.summaryHeader, tc.detailsHeader, "user comments body", "comments body"))
			for token, want := range map[string]string{"summary": "summary body\n", "details": "details body\n"} {
				code, out, errOut := runShow([]string{"show", "1", "--section", token}, dir)
				if code != 0 || out != want {
					t.Errorf("%s: code = %d, stdout = %q, want %q (stderr: %s)", token, code, out, want, errOut)
				}
			}
		})
	}
}

// TestShowSectionEmptyAndMissing pins the empty-payload contract: a section
// with no text and a section the legacy file does not carry both succeed and
// print zero bytes.
func TestShowSectionEmptyAndMissing(t *testing.T) {
	dir := newShowSandbox(t)
	// T-0002 keeps both comment sections but leaves them without text.
	writeShowFile(t, dir, "T-0002-open.md", showTicketSrc(2, "open", "## Summary", "## Details", "", ""))
	// T-0003 is the legacy layout: no User comments section at all.
	legacy := "# T-0003 · BUG: x\n\n- Status (Статус): open\n- Priority (Приоритет): normal\n" +
		"- Created (Создан): 2026-09-08 12:00 · by (кем): tester\n- Project (Проект): tickets\n\n" +
		"## Summary (Кратко)\n\nsummary body\n\n## Details (Подробности)\n\ndetails body\n"
	writeShowFile(t, dir, "T-0003-open.md", legacy)
	cases := []struct {
		name string
		args []string
	}{
		{"empty user comments", []string{"show", "2", "--section", "user-comments"}},
		{"empty comments", []string{"show", "2", "--section", "comments"}},
		{"missing user comments", []string{"show", "3", "--section", "user-comments"}},
		{"missing comments and journal", []string{"show", "3", "--section", "comments"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out, errOut := runShow(tc.args, dir)
			if code != 0 {
				t.Fatalf("code = %d, want 0 (stderr: %s)", code, errOut)
			}
			if out != "" {
				t.Errorf("stdout = %q, want empty", out)
			}
		})
	}
}

// TestShowSectionAmbiguous pins the only exit-2 case: two files carry the
// number, every conflicting path is reported and nothing reaches stdout.
func TestShowSectionAmbiguous(t *testing.T) {
	dir := newShowSandbox(t)
	writeShowFile(t, dir, "T-0001-wip.md", showTicketSrc(1, "wip", "## Summary", "## Details", "", ""))
	code, out, errOut := runShow([]string{"show", "1", "--section", "details"}, dir)
	if code != 2 {
		t.Fatalf("code = %d, want 2 (stderr: %s)", code, errOut)
	}
	if out != "" {
		t.Errorf("stdout = %q, want empty", out)
	}
	for _, name := range []string{"T-0001-open.md", "T-0001-wip.md"} {
		if !strings.Contains(errOut, name) {
			t.Errorf("stderr = %q, want it to name %s", errOut, name)
		}
	}
}

// TestShowSectionAcrossDirectoriesAndExactName pins the shared lookup rules:
// archive/ and deleted/ are readable in the new mode, and an exact file name
// still resolves its own file.
func TestShowSectionAcrossDirectoriesAndExactName(t *testing.T) {
	dir := newShowSandbox(t)
	writeShowFile(t, dir, "archive/T-0007-done.md", showTicketSrc(7, "done", "## Summary", "## Details", "uc seven", "c seven"))
	writeShowFile(t, dir, "deleted/T-0008-deleted.md", showTicketSrc(8, "deleted", "## Summary", "## Details", "uc eight", "c eight"))
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"archive number", []string{"show", "7", "--section", "comments"}, "c seven\n"},
		{"deleted number", []string{"show", "8", "--section", "user-comments"}, "uc eight\n"},
		{"exact file name", []string{"show", "T-0001-open.md", "--section", "comments"}, "comments body\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out, errOut := runShow(tc.args, dir)
			if code != 0 || out != tc.want {
				t.Errorf("code = %d, stdout = %q, want %q (stderr: %s)", code, out, tc.want, errOut)
			}
		})
	}
}
