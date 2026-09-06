package cli_test

import (
	"strings"
	"testing"
)

// TestNewRejectsMultilineTitle runs the real binary (T-0044 regression:
// `ticket new "line1<newline>line2"` used to create a multiline H1 that a
// later `set` re-render truncates to the first line): in a fresh temp
// TICKETS_DIR the run must exit 1, print the localized rejection to stderr
// and leave no T-*.md file. Default lang (no lang env) is ru; a second
// sub-run pins the EN message via TICKET_LANG=en in the child env.
func TestNewRejectsMultilineTitle(t *testing.T) {
	requireBin(t)
	dir := t.TempDir()
	_, stderr, code := runBin(t, ticketBin, dir, dir, "new", "line1\nline2")
	if code != 1 {
		t.Fatalf("new with newline title exit = %d, want 1; stderr: %q", code, stderr)
	}
	if !strings.Contains(stderr, "не может содержать перевод строки") {
		t.Fatalf("stderr %q does not contain the RU newline-title error", stderr)
	}
	assertEmptyDir(t, dir, "tickets dir")

	// EN sub-run: TICKET_LANG=en must switch the rejection message.
	dirEn := t.TempDir()
	_, stderrEn, codeEn := runBinEnv(t, ticketBin, dirEn, dirEn, []string{"TICKET_LANG=en"}, "new", "line1\nline2")
	if codeEn != 1 {
		t.Fatalf("new (en) with newline title exit = %d, want 1; stderr: %q", codeEn, stderrEn)
	}
	if !strings.Contains(stderrEn, "must not contain a line break") {
		t.Fatalf("stderr %q does not contain the EN newline-title error", stderrEn)
	}
	assertEmptyDir(t, dirEn, "tickets dir (en)")
}
