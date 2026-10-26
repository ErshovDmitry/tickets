package domain

// Pre-T-0035 ticket tests for the comment sections (T-0032, T-0035): a
// ticket body written before the comment sections existed gains both of them
// on parse. The fixtures below are in the CURRENT MD022 layout (one blank
// line after every header); the bash-era layout without blank lines after
// headers is pinned by TestBashEraTicketInjectsSectionsAndNormalizes.

import (
	"bytes"
	"strings"
	"testing"
)

// preT0035Ticket is a ticket body without the comment sections, written
// before T-0035 but already in the current MD022 layout (one blank line
// after every header); it ends right before "## Journal (Журнал)".
// preT0035Journal appends the creation entry.
func preT0035Ticket() string {
	return "# T-0001 · BUG: легаси\n\n- Status (Статус): done\n- Priority (Приоритет): normal\n- Created (Создан): 2026-01-01 10:00 · by (кем): я\n- Project (Проект): p\n\n## Summary (Кратко)\n\nлегаси\n\n## Details (Подробности)\n\nтекст легаси\n\n"
}

func preT0035Journal() string {
	return preT0035Ticket() + "## Journal (Журнал)\n\n- 2026-01-01 10:00 — тикет создан (я).\n"
}

// TestBothSectionsInjectedIntoLegacy pins (#4, re-targeted from
// TestCommentsSectionAddedToLegacyTicket): a pre-T-0035 ticket (current MD022
// layout, no comment sections) gains BOTH between "## Details (Подробности)"
// and "## Journal (Журнал)"; Details and journal stay intact.
func TestBothSectionsInjectedIntoLegacy(t *testing.T) {
	tk, unknown, err := Parse([]byte(preT0035Journal()))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.UserComments != "" || tk.Comments != "" {
		t.Fatalf("UserComments/Comments = %q/%q, want empty for a pre-T-0035 ticket", tk.UserComments, tk.Comments)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := preT0035Ticket() + canonEmptySections() + "- 2026-01-01 10:00 — тикет создан (я).\n"
	if !bytes.Equal(got, []byte(want)) {
		t.Fatalf("pre-T-0035 ticket did not gain both sections:\n got %q\nwant %q", got, want)
	}
}

// TestCommentsLegacyWithUnknownTailRoundTrip pins (d+e): a pre-T-0035
// ticket with a manual Unknown tail after the last journal line round-trips
// byte-for-byte: BOTH sections are inserted between "## Details (Подробности)" and
// "## Journal (Журнал)", the tail is preserved verbatim.
func TestCommentsLegacyWithUnknownTailRoundTrip(t *testing.T) {
	tail := "заметка вручную\nещё строка\n"
	tk, unknown, err := Parse([]byte(preT0035Journal() + tail))
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
	want := preT0035Ticket() + canonEmptySections() + "- 2026-01-01 10:00 — тикет создан (я).\n" + tail
	if !bytes.Equal(got, []byte(want)) {
		t.Fatalf("round trip mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestBashEraTicketInjectsSectionsAndNormalizes pins (T-0035/T-0088): a
// bash-era ticket — NO blank line after ANY header and NO comment sections —
// gains both comment sections as bare headers, gets exactly one blank line
// after every header, keeps its Summary/Details/journal content, and stays
// byte-stable on the next parse→render cycle.
func TestBashEraTicketInjectsSectionsAndNormalizes(t *testing.T) {
	src := "# T-0001 · BUG: легаси\n" +
		"- Status (Статус): done\n" +
		"- Priority (Приоритет): normal\n" +
		"- Created (Создан): 2026-01-01 10:00 · by (кем): я\n" +
		"- Project (Проект): p\n" +
		"## Summary (Кратко)\n" +
		"легаси\n" +
		"## Details (Подробности)\n" +
		"текст легаси\n" +
		"## Journal (Журнал)\n" +
		"- 2026-01-01 10:00 — тикет создан (я).\n"
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.UserComments != "" || tk.Comments != "" {
		t.Fatalf("UserComments/Comments = %q/%q, want empty (source has no comment sections)", tk.UserComments, tk.Comments)
	}
	if tk.Details != "текст легаси" {
		t.Fatalf("Details = %q, want %q", tk.Details, "текст легаси")
	}
	if len(tk.Journal) != 1 {
		t.Fatalf("journal entries = %d, want 1", len(tk.Journal))
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := "# T-0001 · BUG: легаси\n\n" +
		"- Status (Статус): done\n" +
		"- Priority (Приоритет): normal\n" +
		"- Created (Создан): 2026-01-01 10:00 · by (кем): я\n" +
		"- Project (Проект): p\n\n" +
		"## Summary (Кратко)\n\nлегаси\n\n" +
		"## Details (Подробности)\n\nтекст легаси\n\n" +
		"## User comments (Комментарии от пользователя)\n\n" +
		"## Comments (Комментарии)\n\n" +
		"## Journal (Журнал)\n\n- 2026-01-01 10:00 — тикет создан (я).\n"
	if !bytes.Equal(got, []byte(want)) {
		t.Fatalf("bash-era ticket not normalized:\n got %q\nwant %q", got, want)
	}
	// Exactly one blank line after every header.
	if strings.Contains(string(got), "\n\n\n") {
		t.Fatalf("more than one blank line after a header:\n%s", got)
	}
	for _, h := range []string{
		"## Summary (Кратко)",
		"## Details (Подробности)",
		"## User comments (Комментарии от пользователя)",
		"## Comments (Комментарии)",
		"## Journal (Журнал)",
	} {
		if !strings.Contains(string(got), h+"\n\n") {
			t.Fatalf("header %q is not followed by exactly one blank line:\n%s", h, got)
		}
	}
	// Second cycle: byte-stable.
	tk2, unknown2, err := Parse(got)
	if err != nil {
		t.Fatalf("second Parse: %v", err)
	}
	got2, err := Render(tk2, unknown2)
	if err != nil {
		t.Fatalf("second Render: %v", err)
	}
	if !bytes.Equal(got2, got) {
		t.Fatalf("second cycle mismatch:\n got %q\nwant %q", got2, got)
	}
}
