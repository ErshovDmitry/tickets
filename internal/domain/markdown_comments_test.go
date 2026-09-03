package domain

// Unit tests for the "## Комментарии" section (T-0032).

import (
	"bytes"
	"strings"
	"testing"
)

// TestNewTicketTemplateSectionOrder pins (a): the template emits the
// comments section between "## Подробности" and "## Журнал", with the
// placeholder STRICTLY on the line right after the header.
func TestNewTicketTemplateSectionOrder(t *testing.T) {
	out, err := RenderNewTicket(newTestTicket())
	if err != nil {
		t.Fatalf("RenderNewTicket: %v", err)
	}
	detailsIdx := bytes.Index(out, []byte("## Подробности\n"))
	commentsIdx := bytes.Index(out, []byte("## Комментарии\n"))
	journalIdx := bytes.Index(out, []byte("## Журнал\n"))
	if detailsIdx < 0 || commentsIdx < 0 || journalIdx < 0 ||
		!(detailsIdx < commentsIdx && commentsIdx < journalIdx) {
		t.Fatalf("section order broken: details=%d comments=%d journal=%d", detailsIdx, commentsIdx, journalIdx)
	}
	want := "## Комментарии\n" + commentsStub + "\n\n## Журнал\n"
	if !bytes.Contains(out, []byte(want)) {
		t.Fatalf("placeholder must follow the header directly:\n%s", out)
	}
}

// TestParseRenderRoundTripNewTicket pins (b): a freshly rendered ticket
// round-trips parse→render byte-for-byte, with the stub blanked to "".
func TestParseRenderRoundTripNewTicket(t *testing.T) {
	out, err := RenderNewTicket(newTestTicket())
	if err != nil {
		t.Fatalf("RenderNewTicket: %v", err)
	}
	tk, unknown, err := Parse(out)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(unknown) != 0 {
		t.Errorf("unknown = %q, want empty", unknown)
	}
	if tk.Comments != "" {
		t.Errorf("Comments = %q, want empty (stub blanked)", tk.Comments)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Equal(got, out) {
		t.Errorf("round trip mismatch:\n got %q\nwant %q", got, out)
	}
}

// TestCommentsUserTextSurvivesRepeatedRoundTrips pins (c): user text in
// the section is byte-stable across repeated parse→render cycles.
func TestCommentsUserTextSurvivesRepeatedRoundTrips(t *testing.T) {
	want := "замечание раз\n\nзамечание два\n- пункт списка"
	src := "# T-0001 · BUG: x\n\n- Статус: open\n- Приоритет: normal\n- Создан: 2026-01-01 10:00 · кем: я\n- Проект: p\n\n## Кратко\nx\n\n## Подробности\nd\n\n## Комментарии\n" +
		want + "\n\n## Журнал\n- 2026-01-01 10:00 — тикет создан (я).\n"
	data := []byte(src)
	for i := range 3 {
		tk, unknown, err := Parse(data)
		if err != nil {
			t.Fatalf("cycle %d Parse: %v", i, err)
		}
		if tk.Comments != want {
			t.Fatalf("cycle %d Comments = %q, want %q", i, tk.Comments, want)
		}
		got, err := Render(tk, unknown)
		if err != nil {
			t.Fatalf("cycle %d Render: %v", i, err)
		}
		if !bytes.Equal(got, data) {
			t.Fatalf("cycle %d round trip mismatch:\n got %q\nwant %q", i, got, data)
		}
		data = got
	}
}

// TestCommentsSectionAddedToLegacyTicket pins (d): a bash-era ticket
// without the section gains it on the next parse→render (set/archive).
func TestCommentsSectionAddedToLegacyTicket(t *testing.T) {
	legacy := "# T-0001 · BUG: легаси\n\n- Статус: done\n- Приоритет: normal\n- Создан: 2026-01-01 10:00 · кем: я\n- Проект: p\n\n## Кратко\nлегаси\n\n## Подробности\nтекст легаси\n\n## Журнал\n- 2026-01-01 10:00 — тикет создан (я).\n"
	tk, unknown, err := Parse([]byte(legacy))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.Comments != "" {
		t.Fatalf("Comments = %q, want empty for a legacy ticket", tk.Comments)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Contains(got, []byte("## Комментарии\n"+commentsStub+"\n\n## Журнал\n")) {
		t.Fatalf("legacy ticket did not gain the section:\n%s", got)
	}
	// Everything else stays intact.
	for _, want := range []string{"текст легаси", "— тикет создан (я)."} {
		if !bytes.Contains(got, []byte(want)) {
			t.Fatalf("legacy content %q lost:\n%s", want, got)
		}
	}
}

// TestCommentsLegacyWithUnknownTailRoundTrip pins (d+e): a bash-era ticket
// without the section plus a manual Unknown tail after the last journal line
// round-trips byte-for-byte: the section+stub is inserted between
// "## Подробности" and "## Журнал", the tail is preserved verbatim.
func TestCommentsLegacyWithUnknownTailRoundTrip(t *testing.T) {
	tail := "заметка вручную\nещё строка\n"
	src := "# T-0001 · BUG: легаси\n\n- Статус: done\n- Приоритет: normal\n- Создан: 2026-01-01 10:00 · кем: я\n- Проект: p\n\n## Кратко\nлегаси\n\n## Подробности\nтекст легаси\n\n## Журнал\n- 2026-01-01 10:00 — тикет создан (я).\n" + tail
	tk, unknown, err := Parse([]byte(src))
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
	want := "# T-0001 · BUG: легаси\n\n- Статус: done\n- Приоритет: normal\n- Создан: 2026-01-01 10:00 · кем: я\n- Проект: p\n\n## Кратко\nлегаси\n\n## Подробности\nтекст легаси\n\n## Комментарии\n" +
		commentsStub + "\n\n## Журнал\n- 2026-01-01 10:00 — тикет создан (я).\n" + tail
	if !bytes.Equal(got, []byte(want)) {
		t.Fatalf("round trip mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestCommentsTextNextToPlaceholderKept pins (e): real text next to the
// placeholder is kept verbatim, mirroring the detailsStub behavior.
func TestCommentsTextNextToPlaceholderKept(t *testing.T) {
	src := "## Комментарии\n" + commentsStub + "\nпометка\n\n## Журнал\n"
	tk, _, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if want := commentsStub + "\nпометка"; tk.Comments != want {
		t.Errorf("Comments = %q, want %q", tk.Comments, want)
	}
}

// TestUnknownTailWithCommentsSection pins (f): when the journal ends early
// (manual tail → the early-return finish() path), Comments must still be
// finalized exactly as in the normal path.
func TestUnknownTailWithCommentsSection(t *testing.T) {
	want := "текст пользователя"
	src := "# T-0001 · BUG: x\n\n- Статус: open\n- Приоритет: normal\n- Создан: 2026-01-01 10:00 · кем: я\n- Проект: p\n\n## Кратко\nx\n\n## Подробности\nd\n\n## Комментарии\n" +
		want + "\n\n## Журнал\n- 2026-01-01 10:00 — тикет создан (я).\nзаметка вручную\nещё строка\n"
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.Comments != want {
		t.Fatalf("Comments = %q, want %q (early-return finish path)", tk.Comments, want)
	}
	if want := []byte("заметка вручную\nещё строка\n"); !bytes.Equal(unknown, want) {
		t.Fatalf("unknown = %q, want %q", unknown, want)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Equal(got, []byte(src)) {
		t.Fatalf("round trip mismatch:\n got %q\nwant %q", got, src)
	}
}

// TestAbsorbLineTransitions pins (g): the explicit section transitions of
// the absorbLine state machine.
func TestAbsorbLineTransitions(t *testing.T) {
	var details, comments []string
	tk := &Ticket{}
	cases := []struct {
		name string
		sec  section
		line string
		want section
	}{
		{"details + comments header", secDetails, "## Комментарии", secComments},
		{"details + journal header", secDetails, "## Журнал", secJournal},
		{"comments + journal header", secComments, "## Журнал", secJournal},
		{"comments + stray line stays", secComments, "текст", secComments},
		{"comments + duplicate header stays", secComments, "## Комментарии", secComments},
		{"comments + details header stays", secComments, "## Подробности", secComments},
		{"brief + comments header", secBrief, "## Комментарии", secComments},
	}
	for _, tc := range cases {
		if got := absorbLine(tk, &details, &comments, tc.sec, tc.line); got != tc.want {
			t.Errorf("%s: absorbLine = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestCommentsStrayLinesPolicy pins (h): stray lines inside "## Комментарии"
// — other headers and duplicate section headers included — stay verbatim
// in Comments (content-preserving policy, mirrors secDetails).
func TestCommentsStrayLinesPolicy(t *testing.T) {
	src := "# T-0001 · BUG: x\n\n- Статус: open\n- Приоритет: normal\n- Создан: 2026-01-01 10:00 · кем: я\n- Проект: p\n\n## Кратко\nx\n\n## Подробности\nd\n\n## Комментарии\nтекст\n## Комментарии\nещё\n## Подробности\nхвост\n\n## Журнал\n- 2026-01-01 10:00 — тикет создан (я).\n"
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if want := "текст\n## Комментарии\nещё\n## Подробности\nхвост"; tk.Comments != want {
		t.Fatalf("Comments = %q, want %q", tk.Comments, want)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Equal(got, []byte(src)) {
		t.Fatalf("round trip mismatch:\n got %q\nwant %q", got, src)
	}
}

// TestCommentsPlaceholderMeansEmpty pins the blanking rule: a section
// holding exactly the placeholder parses to empty Comments (and renders
// back to the placeholder), so no stale marker text leaks into the model.
func TestCommentsPlaceholderMeansEmpty(t *testing.T) {
	tk, _, err := Parse([]byte("## Комментарии\n" + commentsStub + "\n\n## Журнал\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.Comments != "" {
		t.Fatalf("Comments = %q, want empty", tk.Comments)
	}
	got, err := Render(tk, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(got), "## Комментарии\n"+commentsStub+"\n") {
		t.Fatalf("empty Comments must render the placeholder:\n%s", got)
	}
}
