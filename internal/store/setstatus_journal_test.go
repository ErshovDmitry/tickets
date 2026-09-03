package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// TestSetStatus_SameStatusJournalOnly pins T-0031: a same-status
// SetStatus with a non-empty comment appends a From==To journal entry in
// place — the file name and directory stay unchanged, the returned
// target is the old path, and body content (including unknown bytes)
// survives the rename-over.
func TestSetStatus_SameStatusJournalOnly(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	extra := "\n\n<!-- manual analyst note: do not touch -->\n"
	path := filepath.Join(dir, "T-0001-open.md")
	if err := os.WriteFile(path, append(mustRead(t, dir, 1), []byte(extra)...), 0o644); err != nil {
		t.Fatal(err)
	}

	target, err := s.SetStatus(1, domain.StatusOpen, "tester", "note")
	if err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if target != path {
		t.Errorf("target = %q, want unchanged %q", target, path)
	}
	matches, gerr := filepath.Glob(filepath.Join(dir, "T-0001-*.md"))
	if gerr != nil {
		t.Fatal(gerr)
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly one T-0001-*.md, got %v", matches)
	}
	body := string(mustRead(t, dir, 1))
	assertJournalTransition(t, body, "open", "open", "note", "tester")
	if !strings.Contains(body, "manual analyst note") {
		t.Errorf("unknown bytes lost; body:\n%s", body)
	}
	tk, ferr := s.Find(1)
	if ferr != nil {
		t.Fatal(ferr)
	}
	if tk.Status != domain.StatusOpen {
		t.Errorf("status = %s, want open", tk.Status)
	}
	last := tk.Journal[len(tk.Journal)-1]
	if last.From != domain.StatusOpen || last.To != domain.StatusOpen || last.Comment != "note" {
		t.Errorf("journal entry = %+v, want open→open with comment", last)
	}
}

// TestSetStatus_SameStatusRenameFails pins the rename-over failure path:
// a failing renameFile commit joins the cause with the tmp cleanup,
// removes the tmp file, and leaves the old file byte-identical.
func TestSetStatus_SameStatusRenameFails(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "T-0001-open.md")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	swapRename(t, func(string, string) error { return errors.New("boom") })
	_, err = s.SetStatus(1, domain.StatusOpen, "tester", "note")
	if err == nil {
		t.Fatal("expected rename commit error")
	}
	if !strings.Contains(err.Error(), "store: rename commit:") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %v, want joined rename-commit cause", err)
	}
	assertNoTmpFiles(t, dir)
	after, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if !bytes.Equal(before, after) {
		t.Error("old file was modified by a failed rename-over")
	}
}

// swapRename sets renameFile for the duration of t.
func swapRename(t *testing.T, fn func(string, string) error) {
	t.Helper()
	orig := renameFile
	renameFile = fn
	t.Cleanup(func() { renameFile = orig })
}
