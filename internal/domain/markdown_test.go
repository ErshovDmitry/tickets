package domain

import (
	"bytes"
	"testing"
	"time"
)

const ticketT0001 = `# T-0001 · ENH: Реализовать Go-версию ticket по AGENTS_ARCHITECTURE.md

- Status (Статус): open
- Priority (Приоритет): high
- Created (Создан): 2026-09-02 03:24 · by (кем): erdmitry
- Project (Проект): tickets

## Summary (Кратко)

Реализовать Go-версию ticket по AGENTS_ARCHITECTURE.md

## Details (Подробности)

Bootstrap завершён 2026-09-02: схема project_tickets, AGENTS.md, AGENTS_ARCHITECTURE.md, локальные тикеты. Реализация: cmd/ticket + internal/{domain,store,lock,paths,cli}, шаблоны через go:embed. Гейт: план в wiki -> план-ревью -> код.

## User comments (Комментарии от пользователя)

_Замечания пользователя: пишите сюда — агент прочитает эту секцию перед работой над тикетом. Агент сюда не пишет._

## Comments (Комментарии)

## Journal (Журнал)

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
	// The stubs under both comment sections mean empty (T-0032, T-0035).
	if tk.UserComments != "" || tk.Comments != "" {
		t.Errorf("UserComments/Comments = %q/%q, want both empty (stubs blanked)", tk.UserComments, tk.Comments)
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
	want := []byte(legacyRoundTripWant(ticketT0001))
	tk, unknown, err := Parse([]byte(ticketT0001))
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
		tk := &Ticket{
			Journal: []JournalEntry{tc.e},
			Lang:    LangRU,
		}
		got, err := Render(tk, nil)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if !bytes.Contains(got, []byte(tc.want)) {
			t.Errorf("%s: render output %q misses %q", tc.name, got, tc.want)
		}
		// Canonical two-section layout for empty sections (T-0035).
		if !bytes.Contains(got, []byte("## User comments (Комментарии от пользователя)\n\n## Comments (Комментарии)\n\n## Journal (Журнал)\n")) {
			t.Errorf("%s: render output misses the canonical layout:\n%s", tc.name, got)
		}
	}
}

// TestLegacyNoBlankAfterHeaderNormalizes pins the MD022/MD032 migration
// (T-0198): a pre-MD022 ticket whose section headers are immediately
// followed by content still parses correctly, and Render emits the new
// canonical form with one blank line after every section header — no
// content is lost and no blanks accumulate on repeated round trips.
func TestLegacyNoBlankAfterHeaderNormalizes(t *testing.T) {
	legacy := "# T-0001 · BUG: x\n\n" +
		"- Status (Статус): open\n" +
		"- Priority (Приоритет): normal\n" +
		"- Created (Создан): 2026-01-01 10:00 · by (кем): я\n" +
		"- Project (Проект): p\n\n" +
		"## Summary (Кратко)\nx\n\n" +
		"## Details (Подробности)\nd1\n\nd2\n\n" +
		"## User comments (Комментарии от пользователя)\nзамечание\n\n" +
		"## Comments (Комментарии)\nзаметка\n\n" +
		"## Journal (Журнал)\n- 2026-01-01 10:00 — тикет создан (я).\n"
	want := "# T-0001 · BUG: x\n\n" +
		"- Status (Статус): open\n" +
		"- Priority (Приоритет): normal\n" +
		"- Created (Создан): 2026-01-01 10:00 · by (кем): я\n" +
		"- Project (Проект): p\n\n" +
		"## Summary (Кратко)\n\nx\n\n" +
		"## Details (Подробности)\n\nd1\n\nd2\n\n" +
		"## User comments (Комментарии от пользователя)\n\nзамечание\n\n" +
		"## Comments (Комментарии)\n\nзаметка\n\n" +
		"## Journal (Журнал)\n\n- 2026-01-01 10:00 — тикет создан (я).\n"
	tk, unknown, err := Parse([]byte(legacy))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.Details != "d1\n\nd2" || tk.UserComments != "замечание" || tk.Comments != "заметка" {
		t.Fatalf("content drift: Details=%q UC=%q FC=%q", tk.Details, tk.UserComments, tk.Comments)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Equal(got, []byte(want)) {
		t.Fatalf("legacy did not normalize to canonical form:\n got %q\nwant %q", got, want)
	}
	// A second parse/render round trip is byte-stable (no accumulation).
	tk2, unknown2, _ := Parse(got)
	got2, _ := Render(tk2, unknown2)
	if !bytes.Equal(got2, got) {
		t.Fatalf("second round trip not byte-stable:\n first %q\nsecond %q", got, got2)
	}
}
