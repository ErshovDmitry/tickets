package domain

// Legacy-ticket tests for the comment sections (T-0032, T-0035): a bash-era
// ticket body without the comment sections gains both of them on parse.

import (
	"bytes"
	"testing"
)

// legacyTicket is a bash-era ticket body without the comment sections,
// ending right before "## Journal (Журнал)"; legacyJournal appends the creation entry.
func legacyTicket() string {
	return "# T-0001 · BUG: легаси\n\n- Status (Статус): done\n- Priority (Приоритет): normal\n- Created (Создан): 2026-01-01 10:00 · by (кем): я\n- Project (Проект): p\n\n## Summary (Кратко)\n\nлегаси\n\n## Details (Подробности)\n\nтекст легаси\n\n"
}

func legacyJournal() string {
	return legacyTicket() + "## Journal (Журнал)\n\n- 2026-01-01 10:00 — тикет создан (я).\n"
}

// TestBothSectionsInjectedIntoLegacy pins (#4, re-targeted from
// TestCommentsSectionAddedToLegacyTicket): a bash-era ticket without the
// sections gains BOTH between "## Details (Подробности)" and "## Journal (Журнал)"; Details
// and journal stay intact.
func TestBothSectionsInjectedIntoLegacy(t *testing.T) {
	tk, unknown, err := Parse([]byte(legacyJournal()))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.UserComments != "" || tk.Comments != "" {
		t.Fatalf("UserComments/Comments = %q/%q, want empty for a legacy ticket", tk.UserComments, tk.Comments)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := legacyTicket() + canonEmptySections() + "- 2026-01-01 10:00 — тикет создан (я).\n"
	if !bytes.Equal(got, []byte(want)) {
		t.Fatalf("legacy ticket did not gain both sections:\n got %q\nwant %q", got, want)
	}
}

// TestCommentsLegacyWithUnknownTailRoundTrip pins (d+e): a legacy ticket
// with a manual Unknown tail after the last journal line round-trips
// byte-for-byte: BOTH sections are inserted between "## Details (Подробности)" and
// "## Journal (Журнал)", the tail is preserved verbatim.
func TestCommentsLegacyWithUnknownTailRoundTrip(t *testing.T) {
	tail := "заметка вручную\nещё строка\n"
	tk, unknown, err := Parse([]byte(legacyJournal() + tail))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if want := []byte(tail); !bytes.Equal(unknown, want) {
		t.Fatalf("unknown = %q, want %q", unknown, want)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := legacyTicket() + canonEmptySections() + "- 2026-01-01 10:00 — тикет создан (я).\n" + tail
	if !bytes.Equal(got, []byte(want)) {
		t.Fatalf("round trip mismatch:\n got %q\nwant %q", got, want)
	}
}
