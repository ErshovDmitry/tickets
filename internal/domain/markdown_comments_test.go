package domain

// Unit tests for the two comment sections (T-0032, T-0035): UC =
// "## Комментарии от пользователя" (stub means empty) and FC =
// "## Комментарии" (free-form, no stub, never blanked).

import (
	"bytes"
	"strings"
	"testing"
)

// ucTicket builds a minimal two-section ticket: uc goes under "## Комментарии
// от пользователя", fc under "## Комментарии"; empty fc renders the bare
// header form. uc/fc are inserted verbatim (no auto-stub).
func ucTicket(uc, fc string) string {
	head := "# T-0001 · BUG: x\n\n- Статус: open\n- Приоритет: normal\n- Создан: 2026-01-01 10:00 · кем: я\n- Проект: p\n\n## Кратко\nx\n\n## Подробности\nd\n\n"
	body := "## Комментарии от пользователя\n" + uc + "\n\n## Комментарии\n"
	if fc != "" {
		return head + body + fc + "\n\n## Журнал\n- 2026-01-01 10:00 — тикет создан (я).\n"
	}
	return head + body + "\n## Журнал\n- 2026-01-01 10:00 — тикет создан (я).\n"
}

// legacyTicket is a bash-era ticket body without the comment sections,
// ending right before "## Журнал"; legacyJournal appends the creation entry.
func legacyTicket() string {
	return "# T-0001 · BUG: легаси\n\n- Статус: done\n- Приоритет: normal\n- Создан: 2026-01-01 10:00 · кем: я\n- Проект: p\n\n## Кратко\nлегаси\n\n## Подробности\nтекст легаси\n\n"
}

func legacyJournal() string {
	return legacyTicket() + "## Журнал\n- 2026-01-01 10:00 — тикет создан (я).\n"
}

// canonEmptySections is the canonical byte layout of empty comment sections
// (T-0035): UC with its stub, then the bare free header, one blank line
// before "## Журнал".
const canonEmptySections = "## Комментарии от пользователя\n" + userCommentsStub + "\n\n## Комментарии\n\n## Журнал\n"

// TestNewTicketTemplateSectionOrder pins (a): the template emits the user
// section (stub STRICTLY on the line after its header) and the bare free
// section between "## Подробности" and "## Журнал".
func TestNewTicketTemplateSectionOrder(t *testing.T) {
	out, err := RenderNewTicket(newTestTicket())
	if err != nil {
		t.Fatalf("RenderNewTicket: %v", err)
	}
	detailsIdx := bytes.Index(out, []byte("## Подробности\n"))
	blockIdx := bytes.Index(out, []byte(canonEmptySections))
	if detailsIdx < 0 || blockIdx < 0 || detailsIdx > blockIdx {
		t.Fatalf("canonical layout between Подробности and Журнал broken:\n%s", out)
	}
}

// TestParseRenderRoundTripNewTicket pins (b): a freshly rendered ticket
// round-trips parse→render byte-for-byte, with BOTH stubs blanked to "".
func TestParseRenderRoundTripNewTicket(t *testing.T) {
	out, err := RenderNewTicket(newTestTicket())
	if err != nil {
		t.Fatalf("RenderNewTicket: %v", err)
	}
	tk, unknown, err := Parse(out)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(unknown) != 0 || tk.UserComments != "" || tk.Comments != "" {
		t.Errorf("unknown = %q, UserComments = %q, Comments = %q, want all empty (stubs blanked)", unknown, tk.UserComments, tk.Comments)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Equal(got, out) {
		t.Errorf("round trip mismatch:\n got %q\nwant %q", got, out)
	}
}

// TestBothSectionsInjectedIntoLegacy pins (#4, re-targeted from
// TestCommentsSectionAddedToLegacyTicket): a bash-era ticket without the
// sections gains BOTH between "## Подробности" and "## Журнал"; Details
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
	want := legacyTicket() + canonEmptySections + "- 2026-01-01 10:00 — тикет создан (я).\n"
	if !bytes.Equal(got, []byte(want)) {
		t.Fatalf("legacy ticket did not gain both sections:\n got %q\nwant %q", got, want)
	}
}

// TestCommentsLegacyWithUnknownTailRoundTrip pins (d+e): a legacy ticket
// with a manual Unknown tail after the last journal line round-trips
// byte-for-byte: BOTH sections are inserted between "## Подробности" and
// "## Журнал", the tail is preserved verbatim.
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
	want := legacyTicket() + canonEmptySections + "- 2026-01-01 10:00 — тикет создан (я).\n" + tail
	if !bytes.Equal(got, []byte(want)) {
		t.Fatalf("round trip mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestCommentsTextNextToPlaceholderKept pins (e): real text next to the
// stub in the user section is kept verbatim, mirroring the detailsStub
// behavior (the free section has no placeholder at all).
func TestCommentsTextNextToPlaceholderKept(t *testing.T) {
	tk, _, err := Parse([]byte(ucTicket(userCommentsStub+"\nпометка", "")))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if want := userCommentsStub + "\nпометка"; tk.UserComments != want {
		t.Errorf("UserComments = %q, want %q", tk.UserComments, want)
	}
}

// TestUnknownTailWithCommentsSection pins (f): when the journal ends early
// (manual tail → the early-return finish() path), BOTH sections are
// finalized exactly as in the normal path.
func TestUnknownTailWithCommentsSection(t *testing.T) {
	uc, fc := "текст пользователя", "текст агента"
	src := ucTicket(uc, fc) + "заметка вручную\nещё строка\n"
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.UserComments != uc || tk.Comments != fc {
		t.Fatalf("UserComments/Comments = %q/%q, want %q/%q (early-return finish path)", tk.UserComments, tk.Comments, uc, fc)
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
// the absorbLine state machine, including the T-0035 user section.
func TestAbsorbLineTransitions(t *testing.T) {
	var details, userComments, comments []string
	tk := &Ticket{}
	cases := []struct {
		name string
		sec  section
		line string
		want section
	}{
		{"details + user header", secDetails, "## Комментарии от пользователя", secUserComments},
		{"details + comments header", secDetails, "## Комментарии", secComments},
		{"details + journal header", secDetails, "## Журнал", secJournal},
		{"user + comments header", secUserComments, "## Комментарии", secComments},
		{"user + journal header", secUserComments, "## Журнал", secJournal},
		{"user + stray line stays", secUserComments, "текст", secUserComments},
		{"user + duplicate header stays", secUserComments, "## Комментарии от пользователя", secUserComments},
		{"comments + journal header", secComments, "## Журнал", secJournal},
		{"comments + stray line stays", secComments, "текст", secComments},
		{"comments + duplicate header stays", secComments, "## Комментарии", secComments},
		{"comments + details header stays", secComments, "## Подробности", secComments},
		{"comments + user header stays", secComments, "## Комментарии от пользователя", secComments},
		{"brief + comments header", secBrief, "## Комментарии", secComments},
		{"brief + user header", secBrief, "## Комментарии от пользователя", secUserComments},
	}
	for _, tc := range cases {
		if got := absorbLine(tk, &details, &userComments, &comments, tc.sec, tc.line); got != tc.want {
			t.Errorf("%s: absorbLine = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestUserCommentsStrayLinesPolicy pins (#9, re-targeted from
// TestCommentsStrayLinesPolicy): stray lines inside EITHER comment section
// — other headers and duplicate section headers included — stay verbatim
// (content-preserving policy).
func TestUserCommentsStrayLinesPolicy(t *testing.T) {
	src := ucTicket("текст\n## Комментарии от пользователя\nещё\n## Подробности\nхвост", "текст\n## Комментарии\nещё\n## Подробности\nхвост")
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if want := "текст\n## Комментарии от пользователя\nещё\n## Подробности\nхвост"; tk.UserComments != want {
		t.Fatalf("UserComments = %q, want %q", tk.UserComments, want)
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

// TestUserCommentsPlaceholderMeansEmpty pins (#5, re-targeted from
// TestCommentsPlaceholderMeansEmpty): a user section holding exactly the
// stub — or zero lines — parses to empty UserComments and renders the stub
// back. The stub literal is pinned byte-exact per spec (T-0035): changing
// the constant must fail this test.
func TestUserCommentsPlaceholderMeansEmpty(t *testing.T) {
	const stub = "_Замечания пользователя: пишите сюда — агент прочитает эту секцию перед работой над тикетом. Агент сюда не пишет._"
	if userCommentsStub != stub {
		t.Fatalf("userCommentsStub drifted from the spec literal:\n got %q\nwant %q", userCommentsStub, stub)
	}
	for _, tc := range []struct{ name, src string }{
		{"stub exactly", ucTicket(userCommentsStub, "")},
		{"zero lines", ucTicket("", "")},
	} {
		tk, _, err := Parse([]byte(tc.src))
		if err != nil {
			t.Fatalf("%s: Parse: %v", tc.name, err)
		}
		if tk.UserComments != "" {
			t.Errorf("%s: UserComments = %q, want empty", tc.name, tk.UserComments)
		}
		got, err := Render(tk, nil)
		if err != nil {
			t.Fatalf("%s: Render: %v", tc.name, err)
		}
		if !strings.Contains(string(got), "## Комментарии от пользователя\n"+stub+"\n") {
			t.Errorf("%s: empty user section must render the stub:\n%s", tc.name, got)
		}
	}
}

// TestUserCommentsSurviveRepeatedRoundTrips pins (#6, re-targeted from
// TestCommentsUserTextSurvivesRepeatedRoundTrips): text in EITHER section
// is byte-stable across repeated parse→render cycles.
func TestUserCommentsSurviveRepeatedRoundTrips(t *testing.T) {
	src := ucTicket("замечание раз\n\nзамечание два\n- пункт списка", "заметка агента")
	data := []byte(src)
	for i := range 3 {
		tk, unknown, err := Parse(data)
		if err != nil {
			t.Fatalf("cycle %d Parse: %v", i, err)
		}
		if want := "замечание раз\n\nзамечание два\n- пункт списка"; tk.UserComments != want {
			t.Fatalf("cycle %d UserComments = %q, want %q", i, tk.UserComments, want)
		}
		if want := "заметка агента"; tk.Comments != want {
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
