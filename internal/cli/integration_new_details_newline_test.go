package cli_test

// T-0131 integration: the real binary must leave exactly one blank line
// between the details payload and the next section header, whichever form
// supplied the payload. The resolver preserves the input bytes verbatim;
// only the render step normalizes trailing LF.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// detailsBlockEN is the exact Details to User comments block a single-line
// payload must produce: one blank line before the next header.
const detailsBlockEN = detailsHeaderEN + "detail line" + "\n\n" + nextHeaderEN

// assertDetailsBlock checks the raw block and the sliced region: the block
// must be exact, so a third newline (the pre-T-0131 output) fails here.
func assertDetailsBlock(t *testing.T, raw []byte, step string) {
	t.Helper()
	if !strings.Contains(string(raw), detailsBlockEN) {
		t.Fatalf("%s: block %q missing from:\n%s", step, detailsBlockEN, raw)
	}
	if got := detailsRegion(t, raw); got != "detail line" {
		t.Fatalf("%s: details region = %q, want %q", step, got, "detail line")
	}
}

// createWithDetails creates a ticket with the given -d value and stdin in a
// fresh sandbox and returns its raw bytes plus the sandbox TICKETS_DIR.
func createWithDetails(t *testing.T, value, stdin string) ([]byte, string) {
	t.Helper()
	tickets := t.TempDir()
	cwd := t.TempDir()
	out, stderr, code := runBinWithStdin(t, ticketBin, cwd, tickets, stdin,
		"new", "Trailing LF layout", "-d", value)
	if code != 0 {
		t.Fatalf("new -d %q: code=%d stderr=%q", value, code, stderr)
	}
	assertEmptyDir(t, cwd, "foreign cwd")
	return readCreatedTicket(t, out), tickets
}

// setWipAndRead re-renders the sandbox ticket through the set command and
// returns the bytes of the renamed file.
func setWipAndRead(t *testing.T, tickets string) []byte {
	t.Helper()
	_, stderr, code := runBinEnv(t, ticketBin, tickets, tickets, []string{"TICKET_LANG=en"},
		"set", "T-0001", "wip")
	if code != 0 {
		t.Fatalf("set T-0001 wip: code=%d stderr=%q", code, stderr)
	}
	raw, err := os.ReadFile(filepath.Join(tickets, "T-0001-wip.md"))
	if err != nil {
		t.Fatalf("read ticket after set: %v", err)
	}
	return raw
}

// TestNewDetailsTrailingLFLayout covers the inline, @file and stdin forms
// with trailing LF (plus a multi-LF file) and re-checks the layout after
// the set command re-renders the same ticket.
func TestNewDetailsTrailingLFLayout(t *testing.T) {
	requireBin(t)
	writeDetails := func(t *testing.T, payload string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "details.md")
		if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
			t.Fatalf("write details file: %v", err)
		}
		return "@" + path
	}
	tests := []struct {
		name  string
		stdin string
		value func(t *testing.T) string
	}{
		{name: "inline", value: func(*testing.T) string { return "detail line" }},
		{name: "from file with one trailing LF", value: func(t *testing.T) string {
			return writeDetails(t, "detail line\n")
		}},
		{name: "from file with multiple trailing LF", value: func(t *testing.T) string {
			return writeDetails(t, "detail line\n\n\n")
		}},
		{name: "from stdin with one trailing LF", stdin: "detail line\n",
			value: func(*testing.T) string { return "-" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw, tickets := createWithDetails(t, tc.value(t), tc.stdin)
			assertDetailsBlock(t, raw, tc.name+" (new)")
			assertDetailsBlock(t, setWipAndRead(t, tickets), tc.name+" (after set)")
		})
	}
}
