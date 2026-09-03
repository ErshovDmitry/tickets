package domain

import (
	"bytes"
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
		// Every real bash ticket carries the creation entry; without it
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
				"## Summary (Кратко)\nЛомается\n\n" +
				"## Details (Подробности)\nдетали\n\n" +
				"## User comments (Комментарии от пользователя)\n" + dictRU.userCommentsStub + "\n\n" +
				"## Comments (Комментарии)\n\n" +
				"## Journal (Журнал)\n- 2026-09-02 03:24 — тикет создан (agent).\n",
		},
		{
			lang: LangEN,
			want: "# T-0001 · BUG: Ломается\n\n" +
				"- Status: open\n" +
				"- Priority: normal\n" +
				"- Created: 2026-09-02 03:24 · by: agent\n" +
				"- Project: tickets\n\n" +
				"## Summary\nЛомается\n\n" +
				"## Details\nдетали\n\n" +
				"## User comments\n" + dictEN.userCommentsStub + "\n\n" +
				"## Comments\n\n" +
				"## Journal\n- 2026-09-02 03:24 — ticket created (agent).\n",
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

// TestRenderNewTicketEmptyDetailsStub pins the localized Details stub for
// both languages when Details is empty.
func TestRenderNewTicketEmptyDetailsStub(t *testing.T) {
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
				"## Summary (Кратко)\nЛомается\n\n" +
				"## Details (Подробности)\n" + dictRU.detailsStub + "\n\n" +
				"## User comments (Комментарии от пользователя)\n" + dictRU.userCommentsStub + "\n\n" +
				"## Comments (Комментарии)\n\n" +
				"## Journal (Журнал)\n- 2026-09-02 03:24 — тикет создан (agent).\n",
		},
		{
			lang: LangEN,
			want: "# T-0001 · BUG: Ломается\n\n" +
				"- Status: open\n" +
				"- Priority: normal\n" +
				"- Created: 2026-09-02 03:24 · by: agent\n" +
				"- Project: tickets\n\n" +
				"## Summary\nЛомается\n\n" +
				"## Details\n" + dictEN.detailsStub + "\n\n" +
				"## User comments\n" + dictEN.userCommentsStub + "\n\n" +
				"## Comments\n\n" +
				"## Journal\n- 2026-09-02 03:24 — ticket created (agent).\n",
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
