package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// TestSetStatus_RejectsNewlineInComment pins T-0065 cross-status path:
// a SetStatus call with a comment containing LF must be REJECTED
// before any file is read, written, renamed, or linked. The original
// ticket stays byte-identical on disk.
func TestSetStatus_RejectsNewlineInComment(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(dir, "T-0001-open.md"))
	if err != nil {
		t.Fatal(err)
	}

	bad := "line1\n- 1999-01-01 — forged entry (root)"
	_, err = s.SetStatus(1, domain.StatusWip, "tester", bad)
	var inv *ErrInvalidJournalInput
	if !errors.As(err, &inv) {
		t.Fatalf("expected *ErrInvalidJournalInput, got %v", err)
	}
	if inv.Field != "comment" {
		t.Errorf("Field = %q, want %q", inv.Field, "comment")
	}
	if !strings.Contains(inv.Value, "\\n") {
		t.Errorf("Value should escape newline, got %q", inv.Value)
	}

	after, rerr := os.ReadFile(filepath.Join(dir, "T-0001-open.md"))
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(after) != string(before) {
		t.Error("ticket file changed on disk after a rejected injection")
	}
	if _, lerr := os.Lstat(filepath.Join(dir, "T-0001-wip.md")); !errors.Is(lerr, fs.ErrNotExist) {
		t.Errorf("target must not exist, Lstat err=%v", lerr)
	}
	assertNoTmpFiles(t, dir)
}

// TestSetStatus_RejectsCRInComment covers the CR half of the "\r\n"
// pair: a comment with a bare CR is equally line-breaking on classic
// Mac / CRLF inputs and must be rejected the same way.
func TestSetStatus_RejectsCRInComment(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "T-0001-open.md"))

	_, err := s.SetStatus(1, domain.StatusWip, "tester", "line1\rforged")
	var inv *ErrInvalidJournalInput
	if !errors.As(err, &inv) {
		t.Fatalf("expected *ErrInvalidJournalInput, got %v", err)
	}
	if inv.Field != "comment" {
		t.Errorf("Field = %q, want %q", inv.Field, "comment")
	}

	after, _ := os.ReadFile(filepath.Join(dir, "T-0001-open.md"))
	if string(after) != string(before) {
		t.Error("ticket file changed on disk after a rejected CR injection")
	}
}

// TestSetStatus_RejectsNewlineInWho is the parallel guard for the
// TICKET_WHO env value: the same line-oriented invariant means a
// who with a newline is also a forged-entry vector, and the store
// must reject it on the cross-status path.
func TestSetStatus_RejectsNewlineInWho(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "T-0001-open.md"))

	_, err := s.SetStatus(1, domain.StatusWip, "evil\n- 1999-01-01 — x", "ok")
	var inv *ErrInvalidJournalInput
	if !errors.As(err, &inv) {
		t.Fatalf("expected *ErrInvalidJournalInput, got %v", err)
	}
	if inv.Field != "who" {
		t.Errorf("Field = %q, want %q", inv.Field, "who")
	}

	after, _ := os.ReadFile(filepath.Join(dir, "T-0001-open.md"))
	if string(after) != string(before) {
		t.Error("ticket file changed on disk after a rejected who injection")
	}
}

// TestAppendSameStatus_RejectsNewlineInComment pins the same-status
// journal-only path (T-0031): a comment with a newline on
// `set <n> <same> "bad\n- forged"` must be rejected before the
// rename-over commit, leaving the file byte-identical.
func TestAppendSameStatus_RejectsNewlineInComment(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "T-0001-open.md")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.SetStatus(1, domain.StatusOpen, "tester", "note\n- forged")
	var inv *ErrInvalidJournalInput
	if !errors.As(err, &inv) {
		t.Fatalf("expected *ErrInvalidJournalInput, got %v", err)
	}
	if inv.Field != "comment" {
		t.Errorf("Field = %q, want %q", inv.Field, "comment")
	}

	after, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(after) != string(before) {
		t.Error("same-status file changed on disk after a rejected injection")
	}
	assertNoTmpFiles(t, dir)
}

// TestSetStatus_ValidRoundTrip is the regression guard: a normal
// single-line comment and who survive parse/render unchanged. This
// pins the "round-trip preserved" half of the T-0065 acceptance
// criteria so a future tightening of the guard does not silently
// start rejecting valid inputs.
func TestSetStatus_ValidRoundTrip(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetStatus(1, domain.StatusWip, "tester", "starting work"); err != nil {
		t.Fatalf("SetStatus with valid inputs: %v", err)
	}
	body := string(mustRead(t, dir, 1))
	if !strings.Contains(body, "starting work") {
		t.Errorf("comment not present in journal body:\n%s", body)
	}
	assertJournalTransition(t, body, "open", "wip", "starting work", "tester")
}
