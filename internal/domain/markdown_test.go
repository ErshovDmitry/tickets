package domain

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

const ticketT0001 = `# T-0001 · ENH: Реализовать Go-версию ticket по AGENTS_ARCHITECTURE.md

- Статус: open
- Приоритет: high
- Создан: 2026-09-02 03:24 · кем: erdmitry
- Проект: tickets

## Кратко
Реализовать Go-версию ticket по AGENTS_ARCHITECTURE.md

## Подробности
Bootstrap завершён 2026-09-02: схема project_tickets, AGENTS.md, AGENTS_ARCHITECTURE.md, локальные тикеты. Реализация: cmd/ticket + internal/{domain,store,lock,paths,cli}, шаблоны через go:embed. Гейт: план в wiki -> план-ревью -> код.

## Комментарии
_Замечания пользователя: пишите сюда — агент прочитает эту секцию перед работой над тикетом._

## Журнал
- 2026-09-02 03:24 — тикет создан (erdmitry).
`

func TestParseT0001Fields(t *testing.T) {
	tk, _, err := Parse([]byte(ticketT0001))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.Number != 1 {
		t.Errorf("Number = %d, want 1", tk.Number)
	}
	if tk.Status != StatusOpen || tk.Type != TypeENH || tk.Priority != PriorityHigh {
		t.Errorf("enums = %q/%q/%q, want open/ENH/high", tk.Status, tk.Type, tk.Priority)
	}
	wantTitle := "Реализовать Go-версию ticket по AGENTS_ARCHITECTURE.md"
	if tk.Title != wantTitle {
		t.Errorf("Title = %q, want %q", tk.Title, wantTitle)
	}
	if tk.Who != "erdmitry" || tk.Project != "tickets" {
		t.Errorf("Who/Project = %q/%q, want erdmitry/tickets", tk.Who, tk.Project)
	}
	if !tk.Created.Equal(time.Date(2026, 9, 2, 3, 24, 0, 0, time.UTC)) {
		t.Errorf("Created = %v, want 2026-09-02 03:24", tk.Created)
	}
	// The placeholder under "## Комментарии" means empty (T-0032).
	if tk.Comments != "" {
		t.Errorf("Comments = %q, want empty (stub blanked)", tk.Comments)
	}
	if len(tk.Journal) != 1 {
		t.Fatalf("len(Journal) = %d, want 1", len(tk.Journal))
	}
	e := tk.Journal[0]
	if e.From != StatusOpen || e.To != StatusOpen || e.Comment != "" || e.Who != "erdmitry" {
		t.Errorf("creation entry = %+v, want open→open, no comment, erdmitry", e)
	}
	if !e.At.Equal(time.Date(2026, 9, 2, 3, 24, 0, 0, time.UTC)) {
		t.Errorf("entry At = %v, want 2026-09-02 03:24", e.At)
	}
}

func TestParseRenderRoundTripT0001(t *testing.T) {
	want := []byte(ticketT0001)
	tk, unknown, err := Parse(want)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(unknown) != 0 {
		t.Errorf("unknown = %q, want empty", unknown)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("round trip mismatch:\n got %q\nwant %q", got, want)
	}
}

func TestUnknownPreservedAfterJournal(t *testing.T) {
	src := ticketT0001 + "- заметка вручную\nещё строка\n"
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if want := []byte("- заметка вручную\nещё строка\n"); !bytes.Equal(unknown, want) {
		t.Errorf("unknown = %q, want %q", unknown, want)
	}
	if len(tk.Unknown) != len(unknown) {
		t.Errorf("Ticket.Unknown length = %d, want %d", len(tk.Unknown), len(unknown))
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Equal(got, []byte(src)) {
		t.Errorf("unknown re-emit mismatch:\n got %q\nwant %q", got, src)
	}
}

// TestUnknownBlankLinesPreservedAfterJournal documents that a blank line in
// "## Журнал" is Unknown by design, not a parser bug: bytes after the last
// recognized journal line — blank or manual — are preserved verbatim.
func TestUnknownBlankLinesPreservedAfterJournal(t *testing.T) {
	cases := []struct {
		name string
		tail string
	}{
		{"trailing blank line", "\n"},
		{"blank then manual lines", "\n\n- заметка вручную\n\nещё строка\n"},
	}
	for _, tc := range cases {
		src := ticketT0001 + tc.tail
		tk, unknown, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("Parse(%s): %v", tc.name, err)
		}
		if want := []byte(tc.tail); !bytes.Equal(unknown, want) {
			t.Errorf("%s: unknown = %q, want %q", tc.name, unknown, want)
		}
		if len(tk.Journal) != 1 {
			t.Errorf("%s: len(Journal) = %d, want 1", tc.name, len(tk.Journal))
		}
		got, err := Render(tk, unknown)
		if err != nil {
			t.Fatalf("Render(%s): %v", tc.name, err)
		}
		if !bytes.Equal(got, []byte(src)) {
			t.Errorf("%s: round trip mismatch:\n got %q\nwant %q", tc.name, got, src)
		}
	}
}

func TestParseJournalTransitions(t *testing.T) {
	src := ticketT0001 +
		"- 2026-09-02 05:00 — статус: open → wip (система)\n" +
		"- 2026-09-02 06:30 — статус: wip → done · починено (агент)\n"
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(unknown) != 0 {
		t.Errorf("unknown = %q, want empty", unknown)
	}
	if len(tk.Journal) != 3 {
		t.Fatalf("len(Journal) = %d, want 3", len(tk.Journal))
	}
	wip, done := tk.Journal[1], tk.Journal[2]
	if wip.From != StatusOpen || wip.To != StatusWip || wip.Comment != "" || wip.Who != "система" {
		t.Errorf("wip entry = %+v", wip)
	}
	if done.From != StatusWip || done.To != StatusDone || done.Comment != "починено" || done.Who != "агент" {
		t.Errorf("done entry = %+v", done)
	}
}

func TestParseTolerantEmptyDetails(t *testing.T) {
	cases := []string{
		"## Подробности\n\n## Журнал\n",         // empty section
		"## Подробности\n" + detailsStub + "\n", // stub
		"## Журнал\n", // absent section
	}
	for _, src := range cases {
		tk, _, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("Parse(%q): %v", src, err)
		}
		if tk.Details != "" {
			t.Errorf("Parse(%q).Details = %q, want empty", src, tk.Details)
		}
	}

	// Stub followed by real content is kept verbatim, not normalized.
	src := "## Подробности\n" + detailsStub + "\nпометка\n\n## Журнал\n"
	tk, _, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if want := detailsStub + "\nпометка"; tk.Details != want {
		t.Errorf("Details = %q, want %q", tk.Details, want)
	}
}

func TestParseTolerantNeverErrors(t *testing.T) {
	for _, src := range []string{
		"",
		"\n\n\n",
		"мусор без структуры",
		"# T-XXXX · ???",
		"# T-1 · BUG: x\n- Статус: bogus\n- Создан: мусор · кем: \n",
		"## Журнал\nне журнал вовсе\n",
		"# T-999999999999999999999 · BUG: overflow\n",
	} {
		tk, unknown, err := Parse([]byte(src))
		if err != nil {
			t.Errorf("Parse(%q) error: %v", src, err)
			continue
		}
		if tk == nil {
			t.Errorf("Parse(%q) ticket = nil", src)
		}
		if unknown == nil && tk.Unknown != nil {
			t.Errorf("Parse(%q): Unknown mismatch between return and field", src)
		}
	}
}

func TestParseUnknownDetailsKept(t *testing.T) {
	src := "# T-0001 · BUG: x\n\n- Статус: open\n- Приоритет: low\n- Создан: 2026-01-01 10:00 · кем: я\n- Проект: p\n\n## Кратко\nx\n\n## Подробности\nстрока1\n\nстрока2\n\n## Комментарии\n" + commentsStub + "\n\n## Журнал\n- 2026-01-01 10:00 — тикет создан (я).\n"
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if want := "строка1\n\nстрока2"; tk.Details != want {
		t.Errorf("Details = %q, want %q", tk.Details, want)
	}
	if tk.Comments != "" {
		t.Errorf("Comments = %q, want empty (stub blanked)", tk.Comments)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Equal(got, []byte(src)) {
		t.Errorf("round trip mismatch:\n got %q\nwant %q", got, src)
	}
}

func TestRenderJournalLineFormats(t *testing.T) {
	at := time.Date(2026, 9, 2, 5, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		e    JournalEntry
		want string
	}{
		{"creation", JournalEntry{At: at, From: StatusOpen, To: StatusOpen, Who: "я"},
			"- 2026-09-02 05:00 — тикет создан (я).\n"},
		{"transition no comment", JournalEntry{At: at, From: StatusOpen, To: StatusWip, Who: "я"},
			"- 2026-09-02 05:00 — статус: open → wip (я)\n"},
		{"transition with comment", JournalEntry{At: at, From: StatusOpen, To: StatusClosed, Comment: "дубликат", Who: "я"},
			"- 2026-09-02 05:00 — статус: open → closed · дубликат (я)\n"},
	}
	for _, tc := range cases {
		tk := &Ticket{Journal: []JournalEntry{tc.e}}
		got, err := Render(tk, nil)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if !bytes.Contains(got, []byte(tc.want)) {
			t.Errorf("%s: render output %q misses %q", tc.name, got, tc.want)
		}
		// Empty Comments always renders the placeholder (T-0032).
		if !bytes.Contains(got, []byte("## Комментарии\n"+commentsStub+"\n")) {
			t.Errorf("%s: render output misses the comments stub:\n%s", tc.name, got)
		}
	}
}

// TestParseCommentsStubCollision documents the known T-0034 limitation (by
// design): user text in "## Комментарии" byte-identical to commentsStub is
// indistinguishable from the placeholder and blanks to "" on Parse; Render
// then re-emits the stub, keeping the round trip byte-stable. Control: text
// differing from the stub (stub + an extra line) survives verbatim.
func TestParseCommentsStubCollision(t *testing.T) {
	tk, _, err := Parse([]byte(ticketT0001))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.Comments != "" {
		t.Errorf("Comments = %q, want empty (stub blanked)", tk.Comments)
	}

	src := strings.Replace(ticketT0001, commentsStub+"\n", commentsStub+"\nдоп. строка\n", 1)
	tk, _, err = Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if want := commentsStub + "\nдоп. строка"; tk.Comments != want {
		t.Errorf("Comments = %q, want %q", tk.Comments, want)
	}
	got, err := Render(tk, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Equal(got, []byte(src)) {
		t.Errorf("round trip mismatch:\n got %q\nwant %q", got, src)
	}
}
