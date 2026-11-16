package cli_test

// T-0187 CLI tests: flag parsing (separate pairs only, blind values,
// repeat last-wins), every validation path, the no-fields usage refusal
// and the not-found/ambiguous handlers. Success/journal/archive rendering
// lives in cmd_edit_success_test.go, EN localization in
// cmd_edit_i18n_test.go.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/cli"
)

// editRun invokes cli.Run with TICKETS_DIR=dir (default RU locale) and
// returns both streams and the exit code.
func editRun(t *testing.T, dir string, args ...string) (string, string, int) {
	t.Helper()
	env := map[string]string{"TICKETS_DIR": dir}
	var stdout, stderr bytes.Buffer
	code := cli.Run(args, env, &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

// readTicket reads the current T-0001 file from dir (status-agnostic) and
// fails on anything else.
func readTicket(t *testing.T, dir string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "T-0001-*.md"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("glob T-0001: matches=%v err=%v", matches, err)
	}
	body, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	return string(body)
}

// TestEditUsageRefusals pins: no args and a bare number (no field flags)
// both print usage and exit 1 without touching the store.
func TestEditUsageRefusals(t *testing.T) {
	dir := t.TempDir()
	createTestTicket(t, dir)
	before := readTicket(t, dir)

	for _, args := range [][]string{{"edit"}, {"edit", "1"}} {
		stdout, stderr, code := editRun(t, dir, args...)
		if code != 1 {
			t.Errorf("Run(%q) = %d, want 1; stderr: %q", args, code, stderr)
		}
		if stdout == "" || !strings.Contains(stdout, "ticket edit") {
			t.Errorf("Run(%q) stdout = %q, want usage text mentioning edit", args, stdout)
		}
		if after := readTicket(t, dir); after != before {
			t.Errorf("Run(%q) mutated the ticket file", args)
		}
	}
}

// TestEditFlagParsing pins C1: only separate "--flag value" pairs; unknown
// long/short/glued flags get the edit-specific message; a missing value
// gets ErrFlagRequiresValue; a bare positional gets ErrUnknownArgument;
// values are consumed blindly (even '-'-prefixed); a repeat keeps the last
// value. All cases exit 1 before any store I/O.
func TestEditFlagParsing(t *testing.T) {
	dir := t.TempDir()
	createTestTicket(t, dir)

	cases := []struct {
		name    string
		args    []string
		wantSub string
	}{
		{"unknown long flag", []string{"edit", "1", "--bogus", "x"}, "неизвестный флаг для команды edit: --bogus"},
		{"short flag refused", []string{"edit", "1", "-t", "BUG"}, "неизвестный флаг для команды edit: -t"},
		{"glued form refused", []string{"edit", "1", "--title=x"}, "неизвестный флаг для команды edit: --title=x"},
		{"glued type refused", []string{"edit", "1", "--type=BUG"}, "неизвестный флаг для команды edit: --type=BUG"},
		{"missing value", []string{"edit", "1", "--title"}, "флаг --title требует значение"},
		{"extra positional", []string{"edit", "1", "--title", "x", "y"}, "неизвестный аргумент: y"},
		{"positional before flags", []string{"edit", "1", "x", "--title", "y"}, "неизвестный аргумент: x"},
		{"blind value then extra", []string{"edit", "1", "--title", "--project", "x"}, "неизвестный аргумент: x"},
		{"number is not a flag", []string{"edit", "x", "--title", "y"}, "не найден"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := editRun(t, dir, tc.args...)
			if code != 1 {
				t.Fatalf("Run(%q) = %d, want 1; stderr: %q", tc.args, code, stderr)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, tc.wantSub) {
				t.Errorf("stderr = %q, want substring %q", stderr, tc.wantSub)
			}
		})
	}
}

// TestEditValidations pins every pre-I/O value validation in C1 order:
// type, priority, title (empty/CTL/dash-prefix), project (empty), CR/LF.
func TestEditValidations(t *testing.T) {
	dir := t.TempDir()
	createTestTicket(t, dir)

	cases := []struct {
		name    string
		args    []string
		wantSub string
	}{
		{"invalid type", []string{"edit", "1", "--type", "BAD"}, "тип — один из: BUG OPS TD ENH"},
		{"invalid priority", []string{"edit", "1", "--priority", "BAD"}, "приоритет — один из: low normal high"},
		{"whitespace title", []string{"edit", "1", "--title", "   "}, "краткое описание не может быть пустым"},
		{"CTL in title", []string{"edit", "1", "--title", "a\x01b"}, "управляющие символы"},
		{"CRLF in title is CTL", []string{"edit", "1", "--title", "a\nb"}, "управляющие символы"},
		{"dash title", []string{"edit", "1", "--title", "-foo"}, "краткое описание не может начинаться с '-'"},
		{"whitespace project", []string{"edit", "1", "--project", "  "}, "проект не может быть пустым"},
		{"CRLF in project", []string{"edit", "1", "--project", "a\nb"}, "новое значение не может содержать перевод строки"},
		{"CR in project", []string{"edit", "1", "--project", "a\rb"}, "новое значение не может содержать перевод строки"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, stderr, code := editRun(t, dir, tc.args...)
			if code != 1 {
				t.Fatalf("Run(%q) = %d, want 1; stderr: %q", tc.args, code, stderr)
			}
			if !strings.Contains(stderr, tc.wantSub) {
				t.Errorf("stderr = %q, want substring %q", stderr, tc.wantSub)
			}
		})
	}
}

// TestEditNoOpRefusals pins the C1 no-op rule: an explicitly unchanged
// value (alone or mixed with changed ones) refuses the WHOLE edit with
// the localized AlreadyField message, exit 1, file byte-identical.
func TestEditNoOpRefusals(t *testing.T) {
	dir := t.TempDir()
	createTestTicket(t, dir)

	// First move the project to a known value so its no-op is pinned too.
	if _, stderr, code := editRun(t, dir, "edit", "1", "--project", "alpha"); code != 0 {
		t.Fatalf("seed project edit = %d; stderr: %q", code, stderr)
	}

	cases := []struct {
		name    string
		args    []string
		wantSub string
	}{
		{"unchanged title", []string{"edit", "1", "--title", "Тестовый тикет"}, "у тикета 1 поле title уже «\"Тестовый тикет\"»"},
		{"unchanged type", []string{"edit", "1", "--type", "BUG"}, "у тикета 1 поле type уже «\"BUG\"»"},
		{"unchanged priority", []string{"edit", "1", "--priority", "normal"}, "у тикета 1 поле priority уже «\"normal\"»"},
		{"unchanged project", []string{"edit", "1", "--project", "alpha"}, "у тикета 1 поле project уже «\"alpha\"»"},
		{"mixed changed and unchanged", []string{"edit", "1", "--title", "Новый", "--type", "BUG"}, "у тикета 1 поле type уже «\"BUG\"»"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := readTicket(t, dir)
			stdout, stderr, code := editRun(t, dir, tc.args...)
			if code != 1 {
				t.Fatalf("Run(%q) = %d, want 1; stderr: %q", tc.args, code, stderr)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, tc.wantSub) {
				t.Errorf("stderr = %q, want substring %q", stderr, tc.wantSub)
			}
			if after := readTicket(t, dir); after != before {
				t.Errorf("refused edit mutated the file")
			}
		})
	}
}

// TestEditBlindValueAccepted pins C1 blind consumption: a '-'-prefixed
// value of --project is taken literally (validated afterwards) — here it
// passes validation and lands in the metadata.
func TestEditBlindValueAccepted(t *testing.T) {
	dir := t.TempDir()
	createTestTicket(t, dir)

	stdout, stderr, code := editRun(t, dir, "edit", "1", "--project", "--title")
	if code != 0 {
		t.Fatalf("Run(edit 1 --project --title) = %d, want 0; stderr: %q", code, stderr)
	}
	if stdout != "" || stderr != "" {
		t.Errorf("streams = %q/%q, want both empty", stdout, stderr)
	}
	if body := readTicket(t, dir); !strings.Contains(body, "- Project (Проект): --title") {
		t.Errorf("body missing blind project value:\n%s", body)
	}
}

// TestEditRepeatLastWins pins the parseNewFlags precedent: a repeated flag
// keeps the LAST value, flags may come in any order.
func TestEditRepeatLastWins(t *testing.T) {
	dir := t.TempDir()
	createTestTicket(t, dir)

	stdout, stderr, code := editRun(t, dir,
		"edit", "1", "--type", "OPS", "--title", "Первый", "--type", "TD", "--title", "Последний")
	if code != 0 {
		t.Fatalf("Run = %d, want 0; stderr: %q", code, stderr)
	}
	if stdout != "" || stderr != "" {
		t.Errorf("streams = %q/%q, want both empty", stdout, stderr)
	}
	body := readTicket(t, dir)
	if !strings.Contains(body, "# T-0001 · TD: Последний") {
		t.Errorf("H1 missing last-wins title/type:\n%s", body)
	}
	if !strings.Contains(body, "title: Тестовый тикет → Последний") ||
		!strings.Contains(body, "type: BUG → TD") {
		t.Errorf("journal missing both changes:\n%s", body)
	}
}
