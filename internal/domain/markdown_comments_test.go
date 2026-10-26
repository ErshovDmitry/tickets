package domain

// Unit tests for the two comment sections (T-0032, T-0035): UC =
// "## User comments (Комментарии от пользователя)" (stub means empty) and FC =
// "## Comments (Комментарии)" (free-form, no stub, never blanked).

import (
	"bytes"
	"strings"
	"testing"
)

// ucTicket builds a minimal two-section ticket: uc goes under "## Comments (Комментарии)
// от пользователя", fc under "## Comments (Комментарии)"; empty fc renders the bare
// header form. uc/fc are inserted verbatim (no auto-stub).
func ucTicket(uc, fc string) string {
	head := "# T-0001 · BUG: x\n\n- Status (Статус): open\n- Priority (Приоритет): normal\n- Created (Создан): 2026-01-01 10:00 · by (кем): я\n- Project (Проект): p\n\n## Summary (Кратко)\n\nx\n\n## Details (Подробности)\n\nd\n\n"
	body := "## User comments (Комментарии от пользователя)\n\n" + uc + "\n\n## Comments (Комментарии)\n\n"
	if fc != "" {
		return head + body + fc + "\n\n## Journal (Журнал)\n\n- 2026-01-01 10:00 — тикет создан (я).\n"
	}
	return head + body + "## Journal (Журнал)\n\n- 2026-01-01 10:00 — тикет создан (я).\n"
}

// canonEmptySections returns the canonical byte layout of empty comment
// sections (T-0035; the placeholder emission was dropped by the empty-UC
// plan): bare UC header, bare free header, one blank line before
// "## Journal (Журнал)".
func canonEmptySections() string {
	return "## User comments (Комментарии от пользователя)\n\n## Comments (Комментарии)\n\n## Journal (Журнал)\n\n"
}

// TestNewTicketTemplateSectionOrder pins (a): the template emits the user
// section (stub STRICTLY on the line after its header) and the bare free
// section between "## Details (Подробности)" and "## Journal (Журнал)".
func TestNewTicketTemplateSectionOrder(t *testing.T) {
	out, err := RenderNewTicket(newTestTicket())
	if err != nil {
		t.Fatalf("RenderNewTicket: %v", err)
	}
	detailsIdx := bytes.Index(out, []byte("## Details (Подробности)\n"))
	blockIdx := bytes.Index(out, []byte(canonEmptySections()))
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

// TestCommentsTextNextToPlaceholderKept pins (e): real text next to the
// stub in the user section is kept verbatim, mirroring the detailsStub
// behavior (the free section has no placeholder at all).
func TestCommentsTextNextToPlaceholderKept(t *testing.T) {
	tk, _, err := Parse([]byte(ucTicket(dictRU.userCommentsStub+"\nпометка", "")))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if want := dictRU.userCommentsStub + "\nпометка"; tk.UserComments != want {
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
	tk := &Ticket{
		RawTemplates: rawTemplates{
			Headers: make(map[sectionName]string),
			Meta:    make(map[string]metaSegments),
		},
	}
	cases := []struct {
		name string
		sec  section
		line string
		want section
	}{
		{"details + user header", secDetails, "## User comments (Комментарии от пользователя)", secUserComments},
		{"details + comments header", secDetails, "## Comments (Комментарии)", secComments},
		{"details + journal header", secDetails, "## Journal (Журнал)", secJournal},
		{"user + comments header", secUserComments, "## Comments (Комментарии)", secComments},
		{"user + journal header", secUserComments, "## Journal (Журнал)", secJournal},
		{"user + stray line stays", secUserComments, "текст", secUserComments},
		{"user + duplicate header stays", secUserComments, "## User comments (Комментарии от пользователя)", secUserComments},
		{"comments + journal header", secComments, "## Journal (Журнал)", secJournal},
		{"comments + stray line stays", secComments, "текст", secComments},
		{"comments + duplicate header stays", secComments, "## Comments (Комментарии)", secComments},
		{"comments + details header stays", secComments, "## Details (Подробности)", secComments},
		{"comments + user header stays", secComments, "## User comments (Комментарии от пользователя)", secComments},
		{"brief + comments header", secBrief, "## Comments (Комментарии)", secComments},
		{"brief + user header", secBrief, "## User comments (Комментарии от пользователя)", secUserComments},
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
	src := ucTicket("текст\n## User comments (Комментарии от пользователя)\n\nещё\n## Details (Подробности)\n\nхвост", "текст\n## Comments (Комментарии)\n\nещё\n## Details (Подробности)\n\nхвост")
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if want := "текст\n## User comments (Комментарии от пользователя)\n\nещё\n## Details (Подробности)\n\nхвост"; tk.UserComments != want {
		t.Fatalf("UserComments = %q, want %q", tk.UserComments, want)
	}
	if want := "текст\n## Comments (Комментарии)\n\nещё\n## Details (Подробности)\n\nхвост"; tk.Comments != want {
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
// stub — or zero lines — parses to empty UserComments and renders the bare
// header (the placeholder is not re-emitted). The stub literal is pinned
// byte-exact per spec (T-0035): changing the constant must fail this test.
func TestUserCommentsPlaceholderMeansEmpty(t *testing.T) {
	const stub = "_Замечания пользователя: пишите сюда — агент прочитает эту секцию перед работой над тикетом. Агент сюда не пишет._"
	if dictRU.userCommentsStub != stub {
		t.Fatalf("dictRU.userCommentsStub drifted from the spec literal:\n got %q\nwant %q", dictRU.userCommentsStub, stub)
	}
	for _, tc := range []struct{ name, src string }{
		{"stub exactly", ucTicket(dictRU.userCommentsStub, "")},
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
		if strings.Contains(string(got), stub) {
			t.Errorf("%s: placeholder must not be re-emitted:\n%s", tc.name, got)
		}
		if !strings.Contains(string(got), "## User comments (Комментарии от пользователя)\n\n") {
			t.Errorf("%s: empty user section must render the bare header:\n%s", tc.name, got)
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

// TestFreeSectionLeadingBlankLinesPreserved pins (T-0088): the free
// "## Comments" section keeps its bytes verbatim — parse drops only the
// single blank line Render emits after the header (MD022); a second blank
// line is content and survives the parse→render cycle byte-for-byte.
func TestFreeSectionLeadingBlankLinesPreserved(t *testing.T) {
	src := "# T-0001 · BUG: x\n\n" +
		"- Status (Статус): open\n- Priority (Приоритет): normal\n" +
		"- Created (Создан): 2026-01-01 10:00 · by (кем): я\n- Project (Проект): p\n\n" +
		"## Summary (Кратко)\n\nx\n\n" +
		"## Details (Подробности)\n\nd\n\n" +
		"## User comments (Комментарии от пользователя)\n\n" +
		"## Comments (Комментарии)\n\n\nтекст\n\n" +
		"## Journal (Журнал)\n\n- 2026-01-01 10:00 — тикет создан (я).\n"
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if want := "\nтекст"; tk.Comments != want {
		t.Errorf("Comments = %q, want %q (one leading blank line kept)", tk.Comments, want)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Equal(got, []byte(src)) {
		t.Fatalf("round trip mismatch:\n got %q\nwant %q", got, src)
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
		t.Fatalf("second cycle mismatch:\n got %q\nwant %q", got2, got)
	}
}

// TestStubAfterTwoBlankLinesBlanksSection pins (T-0088): a placeholder
// preceded by extra blank lines is still a placeholder — Details and User
// comments parse to empty.
func TestStubAfterTwoBlankLinesBlanksSection(t *testing.T) {
	src := "# T-0001 · BUG: x\n\n" +
		"- Status (Статус): open\n- Priority (Приоритет): normal\n" +
		"- Created (Создан): 2026-01-01 10:00 · by (кем): я\n- Project (Проект): p\n\n" +
		"## Summary (Кратко)\n\nx\n\n" +
		"## Details (Подробности)\n\n\n" + dictRU.detailsStub + "\n\n" +
		"## User comments (Комментарии от пользователя)\n\n\n" + dictRU.userCommentsStub + "\n\n" +
		"## Comments (Комментарии)\n\n" +
		"## Journal (Журнал)\n\n- 2026-01-01 10:00 — тикет создан (я).\n"
	tk, _, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.Details != "" {
		t.Errorf("Details = %q, want empty (stub after two blank lines)", tk.Details)
	}
	if tk.UserComments != "" {
		t.Errorf("UserComments = %q, want empty (stub after two blank lines)", tk.UserComments)
	}
}
