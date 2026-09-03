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
		// Every real bash ticket carries the creation entry; without it
		// Render (data-driven) legitimately emits no journal while
		// RenderNewTicket always does.
		Journal: []JournalEntry{{At: created, From: StatusOpen, To: StatusOpen, Who: "agent"}},
	}
}

func TestRenderNewTicketWithDetails(t *testing.T) {
	out, err := RenderNewTicket(newTestTicket())
	if err != nil {
		t.Fatalf("RenderNewTicket: %v", err)
	}
	want := "# T-0001 · BUG: Ломается\n\n" +
		"- Статус: open\n" +
		"- Приоритет: normal\n" +
		"- Создан: 2026-09-02 03:24 · кем: agent\n" +
		"- Проект: tickets\n\n" +
		"## Кратко\nЛомается\n\n" +
		"## Подробности\nдетали\n\n" +
		"## Комментарии от пользователя\n" + userCommentsStub + "\n\n" +
		"## Комментарии\n\n" +
		"## Журнал\n- 2026-09-02 03:24 — тикет создан (agent).\n"
	if !bytes.Equal(out, []byte(want)) {
		t.Errorf("RenderNewTicket mismatch:\n got %q\nwant %q", out, want)
	}
}

func TestRenderNewTicketEmptyDetailsStub(t *testing.T) {
	tk := newTestTicket()
	tk.Details = ""
	out, err := RenderNewTicket(tk)
	if err != nil {
		t.Fatalf("RenderNewTicket: %v", err)
	}
	want := "# T-0001 · BUG: Ломается\n\n" +
		"- Статус: open\n" +
		"- Приоритет: normal\n" +
		"- Создан: 2026-09-02 03:24 · кем: agent\n" +
		"- Проект: tickets\n\n" +
		"## Кратко\nЛомается\n\n" +
		"## Подробности\n" + detailsStub + "\n\n" +
		"## Комментарии от пользователя\n" + userCommentsStub + "\n\n" +
		"## Комментарии\n\n" +
		"## Журнал\n- 2026-09-02 03:24 — тикет создан (agent).\n"
	if !bytes.Equal(out, []byte(want)) {
		t.Errorf("RenderNewTicket mismatch:\n got %q\nwant %q", out, want)
	}
}

// TestRenderNewTicketMatchesRender pins template output to the canonical
// Render layout: both must be byte-identical for a new ticket.
func TestRenderNewTicketMatchesRender(t *testing.T) {
	tk := newTestTicket()
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
}
