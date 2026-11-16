package domain

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// W1/A (T-0189, GitHub #22): a ticket in the terminal deleted state
// round-trips byte-identically through Parse/Render in RU and EN layouts,
// preserving metadata, Unknown tail, RawTemplates, sections and journal.

// deletedTicketRU is a canonical RU-layout ticket already in the terminal
// deleted state, with a delete transition in the journal and a manual tail
// after the journal (Unknown bytes).
const deletedTicketRU = "# T-0007 · BUG: Удалённый тикет\n" +
	"\n" +
	"- Status (Статус): deleted\n" +
	"- Priority (Приоритет): normal\n" +
	"- Created (Создан): 2026-11-16 10:00 · by (кем): агент\n" +
	"- Project (Проект): tickets\n" +
	"\n" +
	"## Summary (Кратко)\n" +
	"\n" +
	"Удалённый тикет\n" +
	"\n" +
	"## Details (Подробности)\n" +
	"\n" +
	"Проверка soft-delete.\n" +
	"\n" +
	"## User comments (Комментарии от пользователя)\n" +
	"\n" +
	"Замечание пользователя.\n" +
	"\n" +
	"## Comments (Комментарии)\n" +
	"\n" +
	"Рабочая заметка агента.\n" +
	"\n" +
	"## Journal (Журнал)\n" +
	"\n" +
	"- 2026-11-16 10:00 — тикет создан (агент).\n" +
	"- 2026-11-16 11:00 — статус: open → deleted · удалён по запросу (агент)\n" +
	"- ручная строка после журнала\n"

func TestDeletedRoundTripRU(t *testing.T) {
	tk, unknown, err := Parse([]byte(deletedTicketRU))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.Status != StatusDeleted || tk.Priority != PriorityNormal || tk.Project != "tickets" || tk.Who != "агент" {
		t.Errorf("metadata = %q/%q/%q/%q, want deleted/normal/tickets/агент", tk.Status, tk.Priority, tk.Project, tk.Who)
	}
	if !tk.Created.Equal(time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("Created = %v, want 2026-11-16 10:00 UTC", tk.Created)
	}
	if tk.Lang != LangRU {
		t.Errorf("Lang = %v, want LangRU", tk.Lang)
	}
	if tk.UserComments != "Замечание пользователя." || tk.Comments != "Рабочая заметка агента." {
		t.Errorf("comment sections = %q/%q, want preserved bodies", tk.UserComments, tk.Comments)
	}
	assertDeletedJournalRU(t, tk)
	assertDeletedTemplates(t, tk, "## Journal (Журнал)")
	if want := []byte("- ручная строка после журнала\n"); !bytes.Equal(unknown, want) {
		t.Errorf("unknown = %q, want %q", unknown, want)
	}
	assertDeletedRoundTrip(t, tk, unknown, deletedTicketRU)
}

// assertDeletedJournalRU checks the creation plus open→deleted transition
// parsed from the RU fixture, including the verbatim Raw line.
func assertDeletedJournalRU(t *testing.T, tk *Ticket) {
	t.Helper()
	if len(tk.Journal) != 2 {
		t.Fatalf("len(Journal) = %d, want 2", len(tk.Journal))
	}
	del := tk.Journal[1]
	if del.From != StatusOpen || del.To != StatusDeleted || del.Comment != "удалён по запросу" || del.Who != "агент" {
		t.Errorf("delete entry = %+v, want open→deleted with comment", del)
	}
	if del.Raw == "" {
		t.Error("delete entry Raw is empty, want the original line preserved")
	}
}

// assertDeletedTemplates checks that Parse recognized the Status meta form
// and the journal header as raw templates (LOCK D1 byte-stability).
func assertDeletedTemplates(t *testing.T, tk *Ticket, journalHeader string) {
	t.Helper()
	if _, ok := tk.RawTemplates.Meta["Status"]; !ok {
		t.Error("RawTemplates.Meta[Status] missing, want the original meta form preserved")
	}
	if tk.RawTemplates.Headers[secNameJournal] != journalHeader {
		t.Errorf("RawTemplates.Headers[journal] = %q, want %q", tk.RawTemplates.Headers[secNameJournal], journalHeader)
	}
}

// deletedTicketEN is the same ticket in the plain EN layout.
const deletedTicketEN = "# T-0008 · ENH: Deleted ticket\n" +
	"\n" +
	"- Status: deleted\n" +
	"- Priority: low\n" +
	"- Created: 2026-11-16 10:00 · by: agent\n" +
	"- Project: tickets\n" +
	"\n" +
	"## Summary\n" +
	"\n" +
	"Deleted ticket\n" +
	"\n" +
	"## Details\n" +
	"\n" +
	"Soft-delete check.\n" +
	"\n" +
	"## User comments\n" +
	"\n" +
	"User remark.\n" +
	"\n" +
	"## Comments\n" +
	"\n" +
	"Agent note.\n" +
	"\n" +
	"## Journal\n" +
	"\n" +
	"- 2026-11-16 10:00 — ticket created (agent).\n" +
	"- 2026-11-16 11:00 — status: open → deleted · removed on request (agent)\n"

func TestDeletedRoundTripEN(t *testing.T) {
	tk, unknown, err := Parse([]byte(deletedTicketEN))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.Status != StatusDeleted || tk.Lang != LangEN {
		t.Errorf("Status/Lang = %q/%v, want deleted/LangEN", tk.Status, tk.Lang)
	}
	if len(tk.Journal) != 2 || tk.Journal[1].To != StatusDeleted || tk.Journal[1].From != StatusOpen {
		t.Fatalf("journal = %+v, want creation plus open→deleted", tk.Journal)
	}
	if len(unknown) != 0 {
		t.Errorf("unknown = %q, want empty", unknown)
	}
	assertDeletedRoundTrip(t, tk, unknown, deletedTicketEN)
}

// TestDeletedRoundTripFreeSuffix pins RawTemplates byte-stability: a
// non-dict meta form "- Status (free suffix): deleted" must survive
// Parse/Render verbatim (LOCK D1).
func TestDeletedRoundTripFreeSuffix(t *testing.T) {
	src := strings.Replace(deletedTicketRU,
		"- Status (Статус): deleted",
		"- Status (удалено вручную): deleted", 1)
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.Status != StatusDeleted {
		t.Errorf("Status = %q, want deleted", tk.Status)
	}
	assertDeletedTemplates(t, tk, "## Journal (Журнал)")
	assertDeletedRoundTrip(t, tk, unknown, src)
}

// assertDeletedRoundTrip checks byte-identical re-emission and stability of
// a second Parse/Render cycle.
func assertDeletedRoundTrip(t *testing.T, tk *Ticket, unknown []byte, want string) {
	t.Helper()
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Equal(got, []byte(want)) {
		t.Errorf("round trip mismatch:\n got %q\nwant %q", got, want)
	}
	tk2, unknown2, err := Parse(got)
	if err != nil {
		t.Fatalf("second Parse: %v", err)
	}
	got2, err := Render(tk2, unknown2)
	if err != nil {
		t.Fatalf("second Render: %v", err)
	}
	if !bytes.Equal(got2, got) {
		t.Errorf("second round trip drifted:\n got %q\nwant %q", got2, got)
	}
}
