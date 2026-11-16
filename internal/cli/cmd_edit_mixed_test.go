package cli_test

// T-0187 F6 mixed-path CLI tests for `ticket edit`: a duplicate Journal
// section must merge with a single ErrJournalDup stderr warning while the
// edit still succeeds (cmd_set_mixed_test.go precedent), and a
// present-but-unreadable ticket must surface the wrapped store read error
// through the GENERIC editError branch — never the «не найден» message
// (TestSet_UnreadableTicketReportsRealError precedent). Split from
// cmd_edit_success_test.go (300-line cap).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEditDupJournalWarnsAndSucceeds pins T-0070 parity for edit: the
// command exits 0, prints the ErrJournalDup warning on stderr, rewrites
// the H1 and leaves a single merged Journal holding the created, the dup
// and the edit entries.
func TestEditDupJournalWarnsAndSucceeds(t *testing.T) {
	dir := t.TempDir()
	createTestTicket(t, dir)
	path := filepath.Join(dir, "T-0001-open.md")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(before, []byte(dupJournalSuffix)...), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := editRun(t, dir, "edit", "1", "--title", "После дубля")
	if code != 0 {
		t.Fatalf("Run(edit) = %d, want 0; stderr: %q", code, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "дубль секции") {
		t.Errorf("stderr = %q, want ErrJournalDup warning («дубль секции»)", stderr)
	}
	body := readTicket(t, dir)
	if !strings.Contains(body, "# T-0001 · BUG: После дубля") {
		t.Errorf("H1 not rewritten:\n%s", body)
	}
	if n := countJournalHeaders(body, "## Journal"); n != 1 {
		t.Errorf("## Journal occurs %d times, want 1:\n%s", n, body)
	}
	for _, want := range []string{"тикет создан", "dup entry (someone)", "title: Тестовый тикет → После дубля"} {
		if !strings.Contains(body, want) {
			t.Errorf("body misses %q:\n%s", want, body)
		}
	}
}

// TestEditUnreadableTicketReportsRealError pins the generic editError
// branch (cmd_edit.go default case): a present-but-unreadable ticket
// (chmod 000) fails with the wrapped store read error («ticket: store:
// read …») and exit 1 — never the not-found report — and the file stays
// untouched. Skips where chmod cannot produce the condition.
func TestEditUnreadableTicketReportsRealError(t *testing.T) {
	skipRootOrWindows(t)
	dir := t.TempDir()
	createTestTicket(t, dir)
	ticketPath := filepath.Join(dir, "T-0001-open.md")
	if err := os.Chmod(ticketPath, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ticketPath, 0o644) })

	stdout, stderr, code := editRun(t, dir, "edit", "1", "--title", "x")
	if code != 1 {
		t.Fatalf("Run(edit unreadable) = %d, want 1; stderr: %q", code, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.HasPrefix(stderr, "ticket: store: read T-0001-open.md:") {
		t.Errorf("stderr = %q, want prefix %q", stderr, "ticket: store: read T-0001-open.md:")
	}
	if strings.Contains(stderr, "не найден") {
		t.Errorf("stderr = %q masks a real error as not-found", stderr)
	}
	if _, err := os.Stat(ticketPath); err != nil {
		t.Errorf("file should remain untouched: %v", err)
	}
}
