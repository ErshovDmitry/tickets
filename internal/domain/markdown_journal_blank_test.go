package domain

import (
	"bytes"
	"strings"
	"testing"
)

// jbEntry is one canonical creation journal line (RU dict template).
const jbEntry = "- 2026-09-02 03:24 — тикет создан (erdmitry).\n"

// jbHead is the canonical harness ticket head plus its empty "User comments"
// section: everything rendered before the Comments section.
const jbHead = "# T-0001 · ENH: Journal blank harness\n" +
	"\n" +
	"- Status (Статус): open\n" +
	"- Priority (Приоритет): high\n" +
	"- Created (Создан): 2026-09-02 03:24 · by (кем): erdmitry\n" +
	"- Project (Проект): tickets\n" +
	"\n" +
	"## Summary (Кратко)\n" +
	"\n" +
	"Journal blank harness\n" +
	"\n" +
	"## Details (Подробности)\n" +
	"\n" +
	"Body.\n" +
	"\n" +
	"## User comments (Комментарии от пользователя)\n" +
	"\n"

// jbTicket builds the harness ticket: canonical head, an empty User comments
// section, the Comments section BODY as it follows the MD022 blank line, and
// the Journal tail as it follows the Journal header line.
func jbTicket(commentsBody, journalTail string) string {
	return jbHead + "## Comments (Комментарии)\n\n" + commentsBody +
		"## Journal (Журнал)\n" + journalTail
}

// jbResult is one Parse→Render cycle over a ticket source.
type jbResult struct {
	tk       *Ticket
	unknown  []byte
	rendered []byte
}

// jbRun runs one Parse→Render cycle over src.
func jbRun(t *testing.T, src string) jbResult {
	t.Helper()
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rendered, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return jbResult{tk: tk, unknown: unknown, rendered: rendered}
}

// jbStable runs cycles Parse→Render passes over src and fails as soon as one
// rendered result differs from the previous one.
func jbStable(t *testing.T, src string, cycles int) {
	t.Helper()
	prev := jbRun(t, src).rendered
	for i := 2; i <= cycles; i++ {
		next := jbRun(t, string(prev)).rendered
		if !bytes.Equal(next, prev) {
			t.Fatalf("cycle %d drifted:\nprev %q\nnext %q", i, prev, next)
		}
		prev = next
	}
}

// TestJournalBlankEmptyJournalRoundTrip is case 1: a ticket with an EMPTY
// journal parses to no Unknown bytes and renders back byte-identical on every
// further cycle (no growing blank line per Parse→Render round trip).
func TestJournalBlankEmptyJournalRoundTrip(t *testing.T) {
	src := jbTicket("", "\n") // Journal header + the MD022 blank line
	r := jbRun(t, src)
	if len(r.unknown) != 0 {
		t.Errorf("unknown = %q, want empty", r.unknown)
	}
	if len(r.tk.Journal) != 0 {
		t.Errorf("len(Journal) = %d, want 0", len(r.tk.Journal))
	}
	if !bytes.Equal(r.rendered, []byte(src)) {
		t.Fatalf("round trip mismatch:\n got %q\nwant %q", r.rendered, src)
	}
	jbStable(t, src, 3)
}

// TestJournalBlankNonEmptyJournalRoundTrip is case 2: a non-empty journal is
// stable both in the MD022 style (blank line after the header) and in the
// legacy style without that blank line.
func TestJournalBlankNonEmptyJournalRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"MD022 style", jbTicket("", "\n"+jbEntry)},
		{"legacy no blank", jbTicket("", jbEntry)},
	}
	for _, tc := range cases {
		r := jbRun(t, tc.src)
		if len(r.unknown) != 0 {
			t.Errorf("%s: unknown = %q, want empty", tc.name, r.unknown)
		}
		if len(r.tk.Journal) != 1 {
			t.Errorf("%s: len(Journal) = %d, want 1", tc.name, len(r.tk.Journal))
		}
		if want := jbTicket("", "\n"+jbEntry); !bytes.Equal(r.rendered, []byte(want)) {
			t.Errorf("%s: round trip mismatch:\n got %q\nwant %q", tc.name, r.rendered, want)
		}
		jbStable(t, tc.src, 3)
	}
}

// TestJournalBlankManualTailAfterEntries is case 3 (T-0070 regression): the
// manual tail after the last journal entry stays Unknown verbatim and does not
// grow across cycles.
func TestJournalBlankManualTailAfterEntries(t *testing.T) {
	tail := "- заметка вручную\nещё строка\n"
	src := jbTicket("", "\n"+jbEntry+tail)
	r := jbRun(t, src)
	if want := []byte(tail); !bytes.Equal(r.unknown, want) {
		t.Errorf("unknown = %q, want %q", r.unknown, want)
	}
	if len(r.tk.Journal) != 1 {
		t.Errorf("len(Journal) = %d, want 1", len(r.tk.Journal))
	}
	if !bytes.Equal(r.rendered, []byte(src)) {
		t.Fatalf("round trip mismatch:\n got %q\nwant %q", r.rendered, src)
	}
	jbStable(t, src, 3)
}

// TestJournalBlankCommentsVerbatim is case 4 (T-0035 contract): the free
// Comments section stays byte-verbatim, including a second leading blank line,
// while the empty journal adds no Unknown bytes.
func TestJournalBlankCommentsVerbatim(t *testing.T) {
	src := jbTicket("\nnote\n\n", "\n")
	r := jbRun(t, src)
	if len(r.unknown) != 0 {
		t.Errorf("unknown = %q, want empty", r.unknown)
	}
	if want := "\nnote"; r.tk.Comments != want {
		t.Errorf("Comments = %q, want %q", r.tk.Comments, want)
	}
	if !bytes.Equal(r.rendered, []byte(src)) {
		t.Fatalf("round trip mismatch:\n got %q\nwant %q", r.rendered, src)
	}
	jbStable(t, src, 3)
}

// TestJournalBlankManualTailRightAfterHeader is case 5: a manual tail placed
// right after the header blank line starts at the note itself — the render is
// still byte-identical to the source.
func TestJournalBlankManualTailRightAfterHeader(t *testing.T) {
	src := jbTicket("", "\n- заметка вручную\nещё строка\n")
	r := jbRun(t, src)
	if want := []byte("- заметка вручную\nещё строка\n"); !bytes.Equal(r.unknown, want) {
		t.Errorf("unknown = %q, want %q", r.unknown, want)
	}
	if strings.HasPrefix(string(r.unknown), "\n") {
		t.Errorf("unknown starts with a blank line: %q", r.unknown)
	}
	if len(r.tk.Journal) != 0 {
		t.Errorf("len(Journal) = %d, want 0", len(r.tk.Journal))
	}
	if !bytes.Equal(r.rendered, []byte(src)) {
		t.Fatalf("round trip mismatch:\n got %q\nwant %q", r.rendered, src)
	}
}

// TestJournalBlankExactlyOneSwallowed is case 6: only ONE blank line is
// swallowed after the header — a second one stays Unknown.
func TestJournalBlankExactlyOneSwallowed(t *testing.T) {
	src := jbTicket("", "\n\n")
	r := jbRun(t, src)
	if want := []byte("\n"); !bytes.Equal(r.unknown, want) {
		t.Errorf("unknown = %q, want %q", r.unknown, want)
	}
	if len(r.tk.Journal) != 0 {
		t.Errorf("len(Journal) = %d, want 0", len(r.tk.Journal))
	}
	if !bytes.Equal(r.rendered, []byte(src)) {
		t.Fatalf("round trip mismatch:\n got %q\nwant %q", r.rendered, src)
	}
}

// TestJournalBlankAdjacentDupHeader is case 7: an adjacent repeated Journal
// header does not swallow the blank line that separates it from a manual note.
func TestJournalBlankAdjacentDupHeader(t *testing.T) {
	src := jbTicket("", "\n"+jbEntry+"## Journal (Журнал)\n\n- заметка вручную\n")
	r := jbRun(t, src)
	if want := []byte("\n- заметка вручную\n"); !bytes.Equal(r.unknown, want) {
		t.Errorf("unknown = %q, want %q", r.unknown, want)
	}
	if !r.tk.JournalDup {
		t.Errorf("JournalDup = false, want true")
	}
	if len(r.tk.Journal) != 1 {
		t.Errorf("len(Journal) = %d, want 1", len(r.tk.Journal))
	}
	jbStable(t, string(r.rendered), 2)
}

// TestJournalBlankNonAdjacentDupHeader is case 8: a repeated Journal header
// reached from another section also keeps the separating blank line in Unknown.
func TestJournalBlankNonAdjacentDupHeader(t *testing.T) {
	src := jbTicket("", "\n"+jbEntry) + "## Comments (Комментарии)\n\n" +
		"## Journal (Журнал)\n\n- заметка вручную\n"
	r := jbRun(t, src)
	if want := []byte("\n- заметка вручную\n"); !bytes.Equal(r.unknown, want) {
		t.Errorf("unknown = %q, want %q", r.unknown, want)
	}
	if !r.tk.JournalDup {
		t.Errorf("JournalDup = false, want true")
	}
	if len(r.tk.Journal) != 1 {
		t.Errorf("len(Journal) = %d, want 1", len(r.tk.Journal))
	}
	jbStable(t, string(r.rendered), 2)
}
