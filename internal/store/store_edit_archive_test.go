package store

// Store.Edit archive and preservation coverage (T-0187, C3/3): an archived
// ticket is edited in place under archive/ without a reopen, "project" is a
// metadata-only change (no move), the title edit updates the H1 and the
// default Summary while a custom Brief survives, and unrelated content —
// UserComments, Comments, old journal, unknown bytes — round-trips.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// TestEdit_ArchivedInPlace edits a done+archived ticket: the file stays in
// archive/ under its done name, the status is untouched and the journal
// entry is From==To==done appended after the archive line.
func TestEdit_ArchivedInPlace(t *testing.T) {
	s, dir := makeDone(t)
	if _, err := s.Archive(1, "tester"); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	warnings, err := s.Edit(1, EditOptions{Priority: new(domain.PriorityHigh)}, "alice")
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}

	// Exactly one T-0001 file must exist across main + archive: the edit
	// rewrites the archived file in place, never copies it back.
	if total := countTicketFiles(t, dir, 1); total != 1 {
		t.Fatalf("T-0001 files across main+archive = %d, want exactly 1", total)
	}
	archived := filepath.Join(dir, "archive", "T-0001-done.md")
	if _, err := os.Stat(archived); err != nil {
		t.Fatalf("archived file missing after edit: %v", err)
	}
	body, rerr := os.ReadFile(archived)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if strings.Contains(string(body), "- Priority (Приоритет): high") == false {
		t.Fatalf("priority not updated:\n%s", body)
	}
	// The pre-existing open→done transition stays; the new entry is the
	// last transition line, From==To==done.
	tr := journalTransitions(string(body))
	if len(tr) != 2 {
		t.Fatalf("transition lines = %v, want 2", tr)
	}
	last := tr[len(tr)-1]
	if len(last) < 2+len(tsLayout) {
		t.Fatalf("journal line too short for a timestamp: %q", last)
	}
	ts := last[2 : 2+len(tsLayout)]
	wantLine := "- " + ts + " — статус: done → done · priority: normal → high (alice)"
	if last != wantLine {
		t.Fatalf("journal line\n got: %q\nwant: %q", last, wantLine)
	}

	tk, _, perr := domain.Parse(body)
	if perr != nil {
		t.Fatalf("Parse: %v", perr)
	}
	if tk.Status != domain.StatusDone {
		t.Fatalf("status = %q, want done", tk.Status)
	}
}

// countTicketFiles globs main + archive for T-NNNN-*.md files and returns
// the total match count.
func countTicketFiles(t *testing.T, dir string, n int) int {
	t.Helper()
	pat := fmt.Sprintf("T-%04d-*.md", n)
	main, err := filepath.Glob(filepath.Join(dir, pat))
	if err != nil {
		t.Fatal(err)
	}
	arch, err := filepath.Glob(filepath.Join(dir, "archive", pat))
	if err != nil {
		t.Fatal(err)
	}
	return len(main) + len(arch)
}

// TestEdit_ProjectOnlyIsMetadataOnly: changing the project never moves or
// renames the file — per-project directories do not exist (T-0187
// decision).
func TestEdit_ProjectOnlyIsMetadataOnly(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	before := mustRead(t, dir, 1)
	if _, err := s.Edit(1, EditOptions{Project: new("infra")}, "alice"); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	after := mustRead(t, dir, 1)
	if string(after) == string(before) {
		t.Fatal("file unchanged after project edit")
	}
	matches, gerr := filepath.Glob(filepath.Join(dir, "T-0001-*.md"))
	if gerr != nil || len(matches) != 1 {
		t.Fatalf("main files = %v, err = %v", matches, gerr)
	}
	if filepath.Base(matches[0]) != "T-0001-open.md" {
		t.Fatalf("file renamed: %s", matches[0])
	}
	left, gerr := filepath.Glob(filepath.Join(dir, "archive", "T-*.md"))
	if gerr != nil {
		t.Fatal(gerr)
	}
	if len(left) != 0 {
		t.Fatalf("archive holds %v, want none", left)
	}
	if !strings.Contains(string(after), "- Project (Проект): infra") {
		t.Fatalf("project not updated:\n%s", after)
	}
}

// customSectionsFixture is a canonical ticket with a multi-line custom
// Brief, user comments, agent comments and trailing unknown bytes after
// the journal — everything Edit must preserve verbatim.
const customSectionsFixture = `# T-0001 · ENH: Old title

- Status (Статус): open
- Priority (Приоритет): normal
- Created (Создан): 2026-09-10 10:00 · by (кем): tester
- Project (Проект): tickets

## Summary (Кратко)
custom brief line one
custom brief line two

## Details (Подробности)
details body

## User comments (Комментарии от пользователя)
user remark

## Comments (Комментарии)
agent note

## Journal (Журнал)
- 2026-09-10 10:00 — тикет создан (tester).

<!-- manual trailing note -->
`

// TestEdit_PreservesCustomSections edits the title of a ticket carrying a
// custom Brief, user/agent comments and unknown bytes: the H1 and the
// journal reflect the new title, everything else is byte-preserved.
func TestEdit_PreservesCustomSections(t *testing.T) {
	s, dir := newStore(t)
	if err := os.WriteFile(filepath.Join(dir, "T-0001-open.md"), []byte(customSectionsFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Edit(1, EditOptions{Title: new("Brand new title")}, "alice"); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	body := string(mustRead(t, dir, 1))

	if !strings.Contains(body, "# T-0001 · ENH: Brand new title") {
		t.Errorf("H1 not updated:\n%s", body)
	}
	// Custom Brief differs from the old title, so it survives the edit.
	for _, sub := range []string{"custom brief line one", "custom brief line two"} {
		if !strings.Contains(body, sub) {
			t.Errorf("custom Brief lost %q:\n%s", sub, body)
		}
	}
	for _, sub := range []string{"user remark", "agent note", "<!-- manual trailing note -->"} {
		if !strings.Contains(body, sub) {
			t.Errorf("content lost %q:\n%s", sub, body)
		}
	}
	assertJournalTransition(t, body, "open", "open", "title: Old title → Brand new title", "alice")
}

// TestEdit_DefaultSummaryFollowsTitle: a created ticket's Summary body is
// the title itself (the renderer's default, T-0166); editing the title
// carries the Summary along, so the old title disappears entirely.
func TestEdit_DefaultSummaryFollowsTitle(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := s.Edit(1, EditOptions{Title: new("Carried title")}, "alice"); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	body := string(mustRead(t, dir, 1))
	if !strings.Contains(body, "Carried title") {
		t.Fatalf("new title missing:\n%s", body)
	}
	// The old title legitimately survives inside the journal comment, so
	// assert on the parsed Summary body: the default (title-echoing) Brief
	// was carried to the new title.
	tk, _, err := domain.Parse(mustRead(t, dir, 1))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := strings.Join(tk.Brief, "\n"); got != "Carried title" {
		t.Fatalf("Brief = %q, want %q (default Summary not carried)", got, "Carried title")
	}
}

// TestEdit_ParenthesesRoundTrip: field values containing parentheses round
// trip through the journal parser — the appended line is recognized and the
// comment parses back with the parentheses intact.
func TestEdit_ParenthesesRoundTrip(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	opts := EditOptions{
		Title:   new("Fix (urgent) bug"),
		Project: new("alpha (core)"),
	}
	if _, err := s.Edit(1, opts, "alice"); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	tk, _, err := domain.Parse(mustRead(t, dir, 1))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.Title != "Fix (urgent) bug" || tk.Project != "alpha (core)" {
		t.Fatalf("parsed values = %q / %q", tk.Title, tk.Project)
	}
	if len(tk.Journal) != 3 { // creation + title + project
		t.Fatalf("journal entries = %d, want 3: %+v", len(tk.Journal), tk.Journal)
	}
	last := tk.Journal[len(tk.Journal)-1]
	if last.Comment != "project: tickets → alpha (core)" {
		t.Fatalf("last comment = %q", last.Comment)
	}
	if last.From != domain.StatusOpen || last.To != domain.StatusOpen {
		t.Fatalf("last From/To = %q/%q", last.From, last.To)
	}
}
