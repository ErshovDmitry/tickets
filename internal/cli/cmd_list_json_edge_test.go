package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ticket/internal/cli"
)

// TestListJSON_Empty pins the empty contract: "[]\n" with exit 0 — never
// the text-mode domain.NoTickets message.
func TestListJSON_Empty(t *testing.T) {
	dir := newJSONTicketsDir(t)
	code, out, errOut := runJSONList([]string{"list", "--json"}, dir)
	if code != 0 {
		t.Fatalf("list --json = %d, stderr: %s", code, errOut)
	}
	if out != "[]\n" {
		t.Errorf("empty output = %q, want []\\n", out)
	}
}

// TestListJSON_BrokenFile pins the parse-warning contract: broken files
// warn on stderr while stdout stays valid JSON and exit stays 0 (pipeline
// safety).
func TestListJSON_BrokenFile(t *testing.T) {
	dir := newJSONTicketsDir(t)
	writeJSONTicket(t, dir, "T-0001-open.md", 1, "open", "good", "tickets", "2026-09-08 12:00")
	if err := os.WriteFile(filepath.Join(dir, "T-0002-open.md"), []byte("garbage\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, out, errOut := runJSONList([]string{"list", "--json"}, dir)
	if code != 0 {
		t.Fatalf("list --json = %d, want 0", code)
	}
	if !strings.Contains(errOut, "T-0002") {
		t.Errorf("stderr missing warning about T-0002: %s", errOut)
	}
	if got := jsonNumbers(t, out); !reflect.DeepEqual(got, []int{1}) {
		t.Errorf("numbers = %v, want [1]", got)
	}
}

// TestListJSON_InvalidFlags pins typo protection: an unknown flag,
// the --json=true form and a duplicate --json all error with exit 1
// (T-0040 deliberate deviation from bash parity).
func TestListJSON_InvalidFlags(t *testing.T) {
	dir := newJSONTicketsDir(t)
	tests := []struct {
		args    []string
		wantErr string
	}{
		{[]string{"list", "--jsno"}, "неизвестный флаг"},
		{[]string{"list", "--json=true"}, "неизвестный флаг"},
		{[]string{"list", "--json", "--json"}, "повторный флаг --json"},
	}
	for _, tt := range tests {
		code, _, errOut := runJSONList(tt.args, dir)
		if code != 1 {
			t.Errorf("%v: exit = %d, want 1", tt.args, code)
		}
		if !strings.Contains(errOut, tt.wantErr) {
			t.Errorf("%v: stderr = %q, want substring %q", tt.args, errOut, tt.wantErr)
		}
	}
}

// TestListJSON_ValidParse verifies the flag/filter mix (active -P foo
// --json) parses and stdout is valid JSON with the expected ticket.
func TestListJSON_ValidParse(t *testing.T) {
	dir := newJSONTicketsDir(t)
	writeJSONTicket(t, dir, "T-0001-open.md", 1, "open", "test", "foo", "2026-09-08 12:00")

	code, out, errOut := runJSONList([]string{"list", "active", "-P", "foo", "--json"}, dir)
	if code != 0 {
		t.Fatalf("list active -P foo --json = %d, stderr: %s", code, errOut)
	}
	if got := jsonNumbers(t, out); !reflect.DeepEqual(got, []int{1}) {
		t.Errorf("numbers = %v, want [1]", got)
	}
}

// TestListJSON_UsageGuard requires --json documentation in BOTH usage
// fixtures (ru + en, selected via TICKET_LANG) with the 6-field
// description, and keeps the pinned signature substring
// (cmd_archive_test.go TestUsageArchiveKeyLines) intact.
func TestListJSON_UsageGuard(t *testing.T) {
	for lang, desc := range map[string]string{
		"ru": "6 полей: number, status, type, priority, title, created",
		"en": "6 fields: number, status, type, priority, title, created",
	} {
		var stdout, errBuf bytes.Buffer
		env := map[string]string{"TICKET_LANG": lang}
		if code := cli.Run([]string{"help"}, env, &stdout, &errBuf); code != 0 {
			t.Fatalf("help(%s) = %d", lang, code)
		}
		if errBuf.Len() != 0 {
			t.Errorf("help(%s) wrote to stderr: %q", lang, errBuf.String())
		}
		help := stdout.String()
		if !strings.Contains(help, "--json") {
			t.Errorf("usage(%s) missing --json documentation", lang)
		}
		if !strings.Contains(help, desc) {
			t.Errorf("usage(%s) missing --json field list: %q", lang, desc)
		}
		// Signature prefix pinned by TestUsageArchiveKeyLines — flags may
		// only be appended at the end of the list signature line.
		if !strings.Contains(help, "ticket list [active|open|wip|done|closed|archive|all]") {
			t.Errorf("usage(%s) list signature changed (breaks TestUsageArchiveKeyLines)", lang)
		}
	}
}
