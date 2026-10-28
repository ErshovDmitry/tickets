package domain

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func newTestTicket() *Ticket {
	created := time.Date(2026, 9, 2, 3, 24, 0, 0, time.UTC)
	return &Ticket{
		Number:   1,
		Status:   StatusOpen,
		Type:     TypeBUG,
		Priority: PriorityNormal,
		Title:    "Ломается",
		Details:  "детали",
		Who:      "agent",
		Project:  "tickets",
		Created:  created,
		Lang:     LangRU,
		// Every real bash-era ticket carries the creation entry; without it
		// Render (data-driven) legitimately emits no journal while
		// RenderNewTicket always does.
		Journal: []JournalEntry{{At: created, From: StatusOpen, To: StatusOpen, Who: "agent"}},
	}
}

// renderLang renders a new ticket in the given language.
func renderLang(t *testing.T, lang Lang) []byte {
	t.Helper()
	tk := newTestTicket()
	tk.Lang = lang
	out, err := RenderNewTicket(tk)
	if err != nil {
		t.Fatalf("RenderNewTicket(%s): %v", lang, err)
	}
	return out
}

// TestRenderNewTicketGolden pins byte-exact output per language: RU is the
// bilingual form ("Status (Статус)"), EN is the plain form ("Status").
func TestRenderNewTicketGolden(t *testing.T) {
	tests := []struct {
		lang Lang
		want string
	}{
		{
			lang: LangRU,
			want: "# T-0001 · BUG: Ломается\n\n" +
				"- Status (Статус): open\n" +
				"- Priority (Приоритет): normal\n" +
				"- Created (Создан): 2026-09-02 03:24 · by (кем): agent\n" +
				"- Project (Проект): tickets\n\n" +
				"## Summary (Кратко)\n\nЛомается\n\n" +
				"## Details (Подробности)\n\nдетали\n\n" +
				"## User comments (Комментарии от пользователя)\n\n" +
				"## Comments (Комментарии)\n\n" +
				"## Journal (Журнал)\n\n- 2026-09-02 03:24 — тикет создан (agent).\n",
		},
		{
			lang: LangEN,
			want: "# T-0001 · BUG: Ломается\n\n" +
				"- Status: open\n" +
				"- Priority: normal\n" +
				"- Created: 2026-09-02 03:24 · by: agent\n" +
				"- Project: tickets\n\n" +
				"## Summary\n\nЛомается\n\n" +
				"## Details\n\nдетали\n\n" +
				"## User comments\n\n" +
				"## Comments\n\n" +
				"## Journal\n\n- 2026-09-02 03:24 — ticket created (agent).\n",
		},
	}
	for _, tc := range tests {
		t.Run(string(tc.lang), func(t *testing.T) {
			out := renderLang(t, tc.lang)
			if !bytes.Equal(out, []byte(tc.want)) {
				t.Errorf("RenderNewTicket(%s) mismatch:\n got %q\nwant %q", tc.lang, out, tc.want)
			}
		})
	}
}

// TestRenderNewTicketEmptyDetailsFormat pins the localized Details format
// block for both languages when Details is empty.
func TestRenderNewTicketEmptyDetailsFormat(t *testing.T) {
	tests := []struct {
		lang Lang
		want string
	}{
		{
			lang: LangRU,
			want: "# T-0001 · BUG: Ломается\n\n" +
				"- Status (Статус): open\n" +
				"- Priority (Приоритет): normal\n" +
				"- Created (Создан): 2026-09-02 03:24 · by (кем): agent\n" +
				"- Project (Проект): tickets\n\n" +
				"## Summary (Кратко)\n\nЛомается\n\n" +
				"## Details (Подробности)\n\n" + dictRU.detailsFormat + "\n\n" +
				"## User comments (Комментарии от пользователя)\n\n" +
				"## Comments (Комментарии)\n\n" +
				"## Journal (Журнал)\n\n- 2026-09-02 03:24 — тикет создан (agent).\n",
		},
		{
			lang: LangEN,
			want: "# T-0001 · BUG: Ломается\n\n" +
				"- Status: open\n" +
				"- Priority: normal\n" +
				"- Created: 2026-09-02 03:24 · by: agent\n" +
				"- Project: tickets\n\n" +
				"## Summary\n\nЛомается\n\n" +
				"## Details\n\n" + dictEN.detailsFormat + "\n\n" +
				"## User comments\n\n" +
				"## Comments\n\n" +
				"## Journal\n\n- 2026-09-02 03:24 — ticket created (agent).\n",
		},
	}
	for _, tc := range tests {
		t.Run(string(tc.lang), func(t *testing.T) {
			tk := newTestTicket()
			tk.Lang = tc.lang
			tk.Details = ""
			out, err := RenderNewTicket(tk)
			if err != nil {
				t.Fatalf("RenderNewTicket(%s): %v", tc.lang, err)
			}
			if !bytes.Equal(out, []byte(tc.want)) {
				t.Errorf("RenderNewTicket(%s) mismatch:\n got %q\nwant %q", tc.lang, out, tc.want)
			}
		})
	}
}

// TestDetailsFormatBlockRoundTrip pins the Details format block through
// the full cycle: a new ticket without details carries the block, Parse
// keeps it byte-exact as CONTENT (it is not a stub line), and Render
// re-emits it as content — the legacy detailsStub stays the marker of an
// empty Details section (T-0109).
func TestDetailsFormatBlockRoundTrip(t *testing.T) {
	cases := []struct {
		lang Lang
		d    *dict
		want string
	}{
		{LangRU, dictRU, "<!-- Details = нумерованный список:\n1. <что> — <файл:строка>; <решение>\n-->"},
		{LangEN, dictEN, "<!-- Details = numbered list:\n1. <what> — <file:line>; <fix>\n-->"},
	}
	for _, tc := range cases {
		lang, d, want := tc.lang, tc.d, tc.want
		t.Run(string(lang), func(t *testing.T) {
			if d.detailsFormat != want {
				t.Errorf("detailsFormat(%s) = %q, want %q", lang, d.detailsFormat, want)
			}
			tk := newTestTicket()
			tk.Lang = lang
			tk.Details = ""
			out, err := RenderNewTicket(tk)
			if err != nil {
				t.Fatalf("RenderNewTicket(%s): %v", lang, err)
			}
			if !strings.Contains(string(out), want) {
				t.Fatalf("rendered ticket missing the details format block:\n%s", out)
			}
			parsed, _, err := Parse(out)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if parsed.Details != want {
				t.Errorf("parsed Details = %q, want %q", parsed.Details, want)
			}
			rendered, err := Render(parsed, nil)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if !strings.Contains(string(rendered), want) {
				t.Errorf("Render did not re-emit the format block as content:\n%s", rendered)
			}
			if strings.Contains(string(rendered), d.detailsStub) {
				t.Errorf("Render emitted the legacy stub instead of the format block:\n%s", rendered)
			}
			// The LEGACY stub still marks an empty Details section: it
			// parses to an empty Details and Render re-emits it.
			legacy, _, err := Parse([]byte(d.headers[1] + "\n\n" + d.detailsStub + "\n"))
			if err != nil {
				t.Fatalf("Parse(legacy): %v", err)
			}
			if legacy.Details != "" {
				t.Errorf("legacy stub parsed to Details %q, want empty", legacy.Details)
			}
			legacyRendered, err := Render(legacy, nil)
			if err != nil {
				t.Fatalf("Render(legacy): %v", err)
			}
			if !strings.Contains(string(legacyRendered), d.detailsStub) {
				t.Errorf("Render did not re-emit the legacy stub:\n%s", legacyRendered)
			}
		})
	}
}

// TestRenderNewTicketMatchesRender pins template output to the canonical
// Render layout: both must be byte-identical for a new ticket, in every
// language.
func TestRenderNewTicketMatchesRender(t *testing.T) {
	for _, lang := range []Lang{LangRU, LangEN} {
		t.Run(string(lang), func(t *testing.T) {
			tk := newTestTicket()
			tk.Lang = lang
			tmplOut, err := RenderNewTicket(tk)
			if err != nil {
				t.Fatalf("RenderNewTicket: %v", err)
			}
			got, err := Render(tk, nil)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if !bytes.Equal(tmplOut, got) {
				t.Errorf("template vs Render mismatch:\n template %q\n render   %q", tmplOut, got)
			}
		})
	}
}
