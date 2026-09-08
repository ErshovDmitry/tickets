package domain

import (
	"bytes"
	"strings"
	"testing"
)

// ticketMixedOrder is the canonical ticket body with the Journal section
// moved before Summary (T-0070): the journal-tail resume path must parse
// the remaining sections normally instead of dumping them into Unknown.
const ticketMixedOrder = `# T-0002 · BUG: Mixed section order

- Status (Статус): open
- Priority (Приоритет): high
- Created (Создан): 2026-09-10 10:00 · by (кем): tester
- Project (Проект): tickets

## Journal (Журнал)
- 2026-09-10 10:00 — тикет создан (tester).

## Summary (Кратко)
Mixed section order

## Details (Подробности)
details body

## User comments (Комментарии от пользователя)

## Comments (Комментарии)
`

// TestParseMixedOrderJournalFirst documents that a Journal section placed
// before the body sections resumes parsing: Details is captured and nothing
// leaks into Unknown (T-0070).
func TestParseMixedOrderJournalFirst(t *testing.T) {
	tk, unknown, err := Parse([]byte(ticketMixedOrder))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if want := "details body"; tk.Details != want {
		t.Errorf("Details = %q, want %q", tk.Details, want)
	}
	if len(unknown) != 0 {
		t.Errorf("unknown = %q, want empty", unknown)
	}
	if len(tk.Journal) != 1 {
		t.Errorf("len(Journal) = %d, want 1", len(tk.Journal))
	}
	if tk.JournalDup {
		t.Errorf("JournalDup = true, want false")
	}
}

// TestParseNonAdjacentDuplicateJournal documents that a second "## Journal"
// reached through intermediate sections (Summary/Details, not adjacency)
// still sets Ticket.JournalDup: the duplicate detection covers the
// absorbLine transition path, not only the in-journal branch (T-0070 F1).
func TestParseNonAdjacentDuplicateJournal(t *testing.T) {
	src := ticketMixedOrder + "\n## Journal (Журнал)\n- 2099-01-01 10:00 — статус: open → wip · dup (someone)\n"
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !tk.JournalDup {
		t.Errorf("JournalDup = false, want true")
	}
	if len(tk.Journal) != 2 {
		t.Fatalf("len(Journal) = %d, want 2", len(tk.Journal))
	}
	dup := tk.Journal[1]
	if dup.From != StatusOpen || dup.To != StatusWip || dup.Comment != "dup" || dup.Who != "someone" {
		t.Errorf("merged entry = %+v, want open→wip · dup (someone)", dup)
	}
	if len(unknown) != 0 {
		t.Errorf("unknown = %q, want empty", unknown)
	}
}

// TestParseDuplicateJournalMerges documents that a second "## Journal"
// section adjacent to the first merges its entries into the canonical
// journal and sets Ticket.JournalDup (T-0070).
func TestParseDuplicateJournalMerges(t *testing.T) {
	src := ticketT0001 + "\n## Journal (Журнал)\n- 2099-01-01 10:00 — статус: open → wip · dup (someone)\n"
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(tk.Journal) != 2 {
		t.Fatalf("len(Journal) = %d, want 2", len(tk.Journal))
	}
	dup := tk.Journal[1]
	if dup.From != StatusOpen || dup.To != StatusWip || dup.Comment != "dup" || dup.Who != "someone" {
		t.Errorf("merged entry = %+v, want open→wip · dup (someone)", dup)
	}
	if !tk.JournalDup {
		t.Errorf("JournalDup = false, want true")
	}
	if len(unknown) != 0 {
		t.Errorf("unknown = %q, want empty", unknown)
	}
}

// TestParseDuplicateJournalManualTail documents that after a merged second
// Journal section the first manual line ends the tail: Unknown starts at the
// pending blank run before it (T-0070).
func TestParseDuplicateJournalManualTail(t *testing.T) {
	src := ticketT0001 + "\n## Journal (Журнал)\n- 2099-01-01 10:00 — статус: open → wip · dup (someone)\n\nручная заметка\n"
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(tk.Journal) != 2 {
		t.Fatalf("len(Journal) = %d, want 2", len(tk.Journal))
	}
	if !tk.JournalDup {
		t.Errorf("JournalDup = false, want true")
	}
	if want := []byte("\nручная заметка\n"); !bytes.Equal(unknown, want) {
		t.Errorf("unknown = %q, want %q", unknown, want)
	}
}

// TestParseJournalTrailingBlanksFlushed documents that a pending blank run
// at EOF becomes Unknown, so trailing blank bytes survive (T-0070).
func TestParseJournalTrailingBlanksFlushed(t *testing.T) {
	src := ticketT0001 + "\n\n"
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if want := []byte("\n\n"); !bytes.Equal(unknown, want) {
		t.Errorf("unknown = %q, want %q", unknown, want)
	}
	if len(tk.Journal) != 1 {
		t.Errorf("len(Journal) = %d, want 1", len(tk.Journal))
	}
}

// TestParseJournalEntriesAfterBlank documents that blank lines inside the
// journal are deferred: a valid entry after a blank line still merges into
// the journal (T-0070).
func TestParseJournalEntriesAfterBlank(t *testing.T) {
	src := ticketT0001 + "\n- 2099-01-01 10:00 — статус: open → wip (tester)\n"
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(tk.Journal) != 2 {
		t.Fatalf("len(Journal) = %d, want 2", len(tk.Journal))
	}
	if len(unknown) != 0 {
		t.Errorf("unknown = %q, want empty", unknown)
	}
}

// TestParseJournalFreeSuffixHeaderResumes documents that a recognized
// section header after the journal resumes parsing and stores its raw line
// (T-0070); following brief text is ignored, not captured.
func TestParseJournalFreeSuffixHeaderResumes(t *testing.T) {
	src := ticketT0001 + "\n## Summary (any lang)\nMixed section order\n"
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := tk.RawTemplates.Headers[secNameSummary]; got != "## Summary (any lang)" {
		t.Errorf("Headers[Summary] = %q, want %q", got, "## Summary (any lang)")
	}
	wantTitle := "Реализовать Go-версию ticket по AGENTS_ARCHITECTURE.md"
	if tk.Title != wantTitle {
		t.Errorf("Title = %q, want unchanged %q", tk.Title, wantTitle)
	}
	if len(unknown) != 0 {
		t.Errorf("unknown = %q, want empty", unknown)
	}
}

// TestParseJournalH1AndMetaNotResumed documents that only section headers
// resume parsing after the journal: an H1 or meta line ends the tail, and
// Unknown starts exactly at that line's byte offset (T-0070).
func TestParseJournalH1AndMetaNotResumed(t *testing.T) {
	cases := []struct {
		name string
		tail string
	}{
		{"H1 line", "# T-0001 · TD: x\n"},
		{"meta line", "- Status (Статус): open\n"},
	}
	wantTitle := "Реализовать Go-версию ticket по AGENTS_ARCHITECTURE.md"
	for _, tc := range cases {
		src := ticketT0001 + tc.tail
		tk, unknown, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("Parse(%s): %v", tc.name, err)
		}
		if want := []byte(tc.tail); !bytes.Equal(unknown, want) {
			t.Errorf("%s: unknown = %q, want %q (line offset)", tc.name, unknown, want)
		}
		if len(tk.Journal) != 1 {
			t.Errorf("%s: len(Journal) = %d, want 1", tc.name, len(tk.Journal))
		}
		if tk.Title != wantTitle {
			t.Errorf("%s: Title = %q, want unchanged %q", tc.name, tk.Title, wantTitle)
		}
	}
}

// TestRenderMixedOrderNoDuplicates documents that Parse→Render of the
// mixed-order fixture re-emits each resumed section once from its raw
// header, with the Details body intact and no stub placeholder (T-0070).
func TestRenderMixedOrderNoDuplicates(t *testing.T) {
	tk, unknown, err := Parse([]byte(ticketMixedOrder))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, hdr := range []string{"## Summary", "## Details"} {
		if n := strings.Count(string(out), hdr); n != 1 {
			t.Errorf("%q occurs %d times, want 1:\n%s", hdr, n, out)
		}
	}
	if !bytes.Contains(out, []byte("details body")) {
		t.Errorf("render output misses the Details body:\n%s", out)
	}
	if bytes.Contains(out, []byte("<!--")) {
		t.Errorf("render output contains a stub placeholder:\n%s", out)
	}
}
