package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/cli"
)

// TestNewNoTitle checks the contract: `new` without a title prints the
// usage text to stdout and exits 1. Since T-0028 the usage output
// is prefixed by the version line, so the expectation is version line +
// fixture via the shared wantUsage helper (cli_test.go).
func TestNewNoTitle(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := cli.Run([]string{"new"}, testEnv(t), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(new) = %d, want 1", code)
	}
	if got := stdout.String(); got != wantUsage(usageFixture) {
		t.Fatalf("stdout is not the version line + verbatim usage text (got %d bytes, want %d)", len(got), len(wantUsage(usageFixture)))
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

// TestNewEmptyTitleRejected pins the V16 fix: empty and whitespace-only
// titles are rejected with exit 1 and the domain.Strict message before any
// ticket file is created. The no-arg path (usage, TestNewNoTitle) and valid
// titles are unaffected.
func TestNewEmptyTitleRejected(t *testing.T) {
	cases := []struct {
		name  string
		title string
	}{
		{"empty", ""},
		{"spaces", "   "},
		{"tabs and newlines", "\t\n "},
		{"unicode spaces", "\u00a0\u2003"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			var stdout, stderr bytes.Buffer
			code := cli.Run([]string{"new", tc.title}, map[string]string{"TICKETS_DIR": dir}, &stdout, &stderr)
			if code != 1 {
				t.Fatalf("Run(new %q) = %d, want 1; stderr: %q", tc.title, code, stderr.String())
			}
			if !strings.Contains(stderr.String(), "краткое описание не может быть пустым") {
				t.Fatalf("stderr %q does not contain the empty-title error", stderr.String())
			}
			created, err := filepath.Glob(filepath.Join(dir, "T-*.md"))
			if err != nil {
				t.Fatalf("glob: %v", err)
			}
			if len(created) != 0 {
				t.Fatalf("ticket file created despite empty title: %v", created)
			}
		})
	}
}

// TestNewTitlePreservedVerbatim guards against over-trimming: a title with
// surrounding whitespace is non-empty, so the ticket must be created and the
// title stored byte-for-byte (no normalization of valid input).
func TestNewTitlePreservedVerbatim(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	title := "  padded title  "
	if code := cli.Run([]string{"new", title}, map[string]string{"TICKETS_DIR": dir}, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(new) = %d, want 0; stderr: %q", code, stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(dir, "T-0001-open.md"))
	if err != nil {
		t.Fatalf("created ticket is not readable: %v", err)
	}
	if !strings.Contains(string(data), "# T-0001 · BUG: "+title) {
		t.Fatalf("title not preserved verbatim in H1:\n%s", data)
	}
}

// TestNewTitleStartingWithDash pins the T-0041 validation: a title starting
// with '-' (a mistyped flag) is rejected with exit 1 and no ticket file;
// the bare "-" argument is a subcase. ("new --help" never reaches cmdNew —
// it is intercepted centrally in Run, see TestSubcommandHelp.)
func TestNewTitleStartingWithDash(t *testing.T) {
	for _, title := range []string{"-foo", "-"} {
		t.Run(title, func(t *testing.T) {
			dir := t.TempDir()
			var stdout, stderr bytes.Buffer
			code := cli.Run([]string{"new", title}, map[string]string{"TICKETS_DIR": dir}, &stdout, &stderr)
			if code != 1 {
				t.Fatalf("Run(new %q) = %d, want 1; stderr: %q", title, code, stderr.String())
			}
			if !strings.Contains(stderr.String(), "ticket: краткое описание не может начинаться с '-'") {
				t.Fatalf("stderr %q missing the dash-title error", stderr.String())
			}
			created, err := filepath.Glob(filepath.Join(dir, "T-*.md"))
			if err != nil {
				t.Fatalf("glob: %v", err)
			}
			if len(created) != 0 {
				t.Fatalf("ticket file created despite dash title: %v", created)
			}
		})
	}
}

// TestNewHelpInComment guards the args[1]-only interception: --help inside
// a -d comment value (args[3]) must not trigger help — the ticket is
// created and no usage text is printed.
func TestNewHelpInComment(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	args := []string{"new", "Тикет", "-d", "see --help"}
	if code := cli.Run(args, map[string]string{"TICKETS_DIR": dir}, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(%q) = %d, want 0; stderr: %q", args, code, stderr.String())
	}
	if strings.Contains(stdout.String(), "ticket version") {
		t.Fatalf("usage printed for a real ticket creation: %q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "T-0001-open.md")); err != nil {
		t.Fatalf("ticket not created: %v", err)
	}
}
