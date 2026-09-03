package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// tamperBodyStatus hand-edits the "- Status (Статус): " line of ticket n's file
// inside dir to claim claimed, failing the test if the line is absent.
// The filename is left untouched — this is the divergence the V19 fix
// covers: filename status is authoritative, body status is untrusted.
func tamperBodyStatus(t *testing.T, dir string, n int, claimed domain.Status) {
	t.Helper()
	p := filepath.Join(dir, domain.Filename(n, domain.StatusOpen))
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	tampered := strings.Replace(string(data), "- Status (Статус): open", "- Status (Статус): "+string(claimed), 1)
	if tampered == string(data) {
		t.Fatalf("status line not found in body of %s", p)
	}
	if err := os.WriteFile(p, []byte(tampered), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

// tamperedOpenStore returns a store holding T-0001-open.md whose body
// falsely claims the given status.
func tamperedOpenStore(t *testing.T, claimed domain.Status) (*Store, string) {
	t.Helper()
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(1)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	tamperBodyStatus(t, dir, 1, claimed)
	return s, dir
}

// TestList_StatusFromFilenameNotBody is the V19 regression: T-0001-open.md
// with a hand-edited body status "done" must list as open (the filename
// status), so "list open" keeps finding it and "list done" cannot.
func TestList_StatusFromFilenameNotBody(t *testing.T) {
	s, _ := tamperedOpenStore(t, domain.StatusDone)

	tickets, warns := s.List()
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %+v", warns)
	}
	if len(tickets) != 1 {
		t.Fatalf("tickets = %d, want 1", len(tickets))
	}
	if tickets[0].Status != domain.StatusOpen {
		t.Fatalf("List status = %q (tampered body leaked), want open (filename)", tickets[0].Status)
	}
	if tickets[0].Number != 1 || tickets[0].Title != "ticket-1" {
		t.Fatalf("other fields corrupted: %+v", tickets[0])
	}
}

// TestFind_StatusFromFilenameNotBody pins the same authority for Find and
// FindNamed consumers (show renders status from this ticket).
func TestFind_StatusFromFilenameNotBody(t *testing.T) {
	s, _ := tamperedOpenStore(t, domain.StatusDone)

	tk, err := s.Find(1)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if tk.Status != domain.StatusOpen {
		t.Fatalf("Find status = %q, want open (filename)", tk.Status)
	}

	tkNamed, name, err := s.FindNamed(1)
	if err != nil {
		t.Fatalf("FindNamed: %v", err)
	}
	if name != "T-0001-open.md" {
		t.Fatalf("FindNamed name = %q, want T-0001-open.md", name)
	}
	if tkNamed.Status != domain.StatusOpen {
		t.Fatalf("FindNamed status = %q, want open (filename)", tkNamed.Status)
	}
}

// TestSetStatus_TransitionFromFilenameStatusNotBody pins the transition
// gate: the body claims done, the filename says open, so a set to wip is
// a legal open→wip transition (never rejected as "already done"), and the
// final on-disk file is T-0001-wip.md with metadata status wip.
func TestSetStatus_TransitionFromFilenameStatusNotBody(t *testing.T) {
	s, dir := tamperedOpenStore(t, domain.StatusDone)

	if _, err := s.SetStatus(1, domain.StatusWip, "tester", "from open"); err != nil {
		t.Fatalf("SetStatus (open→wip) rejected via tampered body: %v", err)
	}

	oldPath := filepath.Join(dir, "T-0001-open.md")
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old open file still present, stat err=%v", err)
	}
	newPath := filepath.Join(dir, "T-0001-wip.md")
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("new wip file missing: %v", err)
	}
	body := string(mustRead(t, dir, 1))

	tk, _, err := domain.Parse([]byte(body))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.Status != domain.StatusWip {
		t.Fatalf("final metadata status = %q, want wip", tk.Status)
	}
	if last := tk.Journal[len(tk.Journal)-1]; last.From != domain.StatusOpen || last.To != domain.StatusWip {
		t.Fatalf("journal entry = %s→%s, want open→wip", last.From, last.To)
	}
	assertNoTmpFiles(t, dir)
}
