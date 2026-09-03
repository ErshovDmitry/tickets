package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// TestSetStatus_JournalFromUsesFilenameStatus is the V16 regression: the
// scanner pairs a ticket with its filename status (cur.Status), while the
// parsed body status is untrusted — a hand-edited body may claim any status
// inside T-NNNN-open.md. The transition journal must derive From from the
// scan-matched filename status, and the final metadata status must be next.
func TestSetStatus_JournalFromUsesFilenameStatus(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(1)); err != nil {
		t.Fatal(err)
	}

	// Hand-edit the body: filename stays T-0001-open.md, body claims done.
	oldPath := filepath.Join(dir, "T-0001-open.md")
	data, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(data), "- Status (Статус): open", "- Status (Статус): done", 1)
	if tampered == string(data) {
		t.Fatalf("status line not found in body:\n%s", data)
	}
	if err := os.WriteFile(oldPath, []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := s.SetStatus(1, domain.StatusWip, "tester", "regression"); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	// The open (filename-status) file is gone; exactly one wip file remains.
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old open file still present, stat err=%v", err)
	}
	body := string(mustRead(t, dir, 1))

	// Journal transition must be open→wip (from the filename), never
	// done→wip (from the tampered body).
	assertJournalTransition(t, body, "open", "wip", "regression", "tester")

	// Final metadata status is wip, and the body keeps its other fields.
	tk, _, err := domain.Parse([]byte(body))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.Status != domain.StatusWip {
		t.Fatalf("metadata status = %q, want wip", tk.Status)
	}
	if n := len(tk.Journal); n != 2 {
		t.Fatalf("journal entries = %d, want 2", n)
	}
	if last := tk.Journal[1]; last.From != domain.StatusOpen || last.To != domain.StatusWip {
		t.Fatalf("journal entry = %s→%s, want open→wip", last.From, last.To)
	}
	if tk.Title != "ticket-1" {
		t.Fatalf("title changed by SetStatus: %q", tk.Title)
	}
	assertNoTmpFiles(t, dir)
}
