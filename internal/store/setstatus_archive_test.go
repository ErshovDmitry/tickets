package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"ticket/internal/domain"
)

// TestSetStatus_ReopenArchivedGoesToMain pins §8a item 5: setting an
// archived ticket back to open/wip returns it to the main directory
// (nothing stays under archive/) and the journal keeps chronological
// order: creation → open→done → archived → done→open.
func TestSetStatus_ReopenArchivedGoesToMain(t *testing.T) {
	s, dir := makeDone(t)
	if _, err := s.Archive(1, "tester"); err != nil {
		t.Fatal(err)
	}
	target, err := s.SetStatus(1, domain.StatusOpen, "tester", "")
	if err != nil {
		t.Fatalf("SetStatus(reopen): %v", err)
	}
	if want := filepath.Join(dir, "T-0001-open.md"); target != want {
		t.Fatalf("target = %q, want %q (reopen goes to main)", target, want)
	}
	left, gerr := filepath.Glob(filepath.Join(dir, "archive", "T-*.md"))
	if gerr != nil {
		t.Fatal(gerr)
	}
	if len(left) != 0 {
		t.Fatalf("archive still holds %v", left)
	}
	body, rerr := os.ReadFile(target)
	if rerr != nil {
		t.Fatal(rerr)
	}
	tk, _, perr := domain.Parse(body)
	if perr != nil {
		t.Fatal(perr)
	}
	if len(tk.Journal) != 4 {
		t.Fatalf("len(Journal) = %d, want 4\n%s", len(tk.Journal), body)
	}
	e := tk.Journal
	if e[0].From != domain.StatusOpen || e[0].To != domain.StatusOpen {
		t.Errorf("entry[0] = %v→%v, want open→open (creation)", e[0].From, e[0].To)
	}
	if e[1].From != domain.StatusOpen || e[1].To != domain.StatusDone {
		t.Errorf("entry[1] = %v→%v, want open→done", e[1].From, e[1].To)
	}
	if !e[2].Archived {
		t.Errorf("entry[2].Archived = false, want true")
	}
	if e[3].From != domain.StatusDone || e[3].To != domain.StatusOpen {
		t.Errorf("entry[3] = %v→%v, want done→open (reopen)", e[3].From, e[3].To)
	}
}

// TestSetStatus_ArchivedDoneClosedStaysInArchive pins §8a item 5: moving
// an archived ticket between done and closed rewrites it in place under
// archive/ and returns the archive path (the caller cannot derive it from
// s.Dir alone).
func TestSetStatus_ArchivedDoneClosedStaysInArchive(t *testing.T) {
	s, dir := makeDone(t)
	if _, err := s.Archive(1, "tester"); err != nil {
		t.Fatal(err)
	}
	target, err := s.SetStatus(1, domain.StatusClosed, "tester", "")
	if err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	want := filepath.Join(dir, "archive", "T-0001-closed.md")
	if target != want {
		t.Fatalf("target = %q, want %q (done↔closed stays in archive)", target, want)
	}
	if _, serr := os.Stat(want); serr != nil {
		t.Fatalf("archived file missing: %v", serr)
	}
	if _, lerr := os.Lstat(filepath.Join(dir, "archive", "T-0001-done.md")); !errors.Is(lerr, fs.ErrNotExist) {
		t.Errorf("old done file must be gone, err=%v", lerr)
	}
	mainGlob, gerr := filepath.Glob(filepath.Join(dir, "T-0001-*.md"))
	if gerr != nil {
		t.Fatal(gerr)
	}
	if len(mainGlob) != 0 {
		t.Errorf("main dir must not hold ticket 1: %v", mainGlob)
	}
	body, rerr := os.ReadFile(want)
	if rerr != nil {
		t.Fatal(rerr)
	}
	tk, _, perr := domain.Parse(body)
	if perr != nil {
		t.Fatal(perr)
	}
	last := tk.Journal[len(tk.Journal)-1]
	if last.From != domain.StatusDone || last.To != domain.StatusClosed {
		t.Errorf("last journal entry = %v→%v, want done→closed", last.From, last.To)
	}
}
