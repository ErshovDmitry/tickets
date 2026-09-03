package domain

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
)

//go:embed templates/*.tmpl
var templates embed.FS

// newTicketData feeds new_ticket.md.tmpl. Number, Timestamp and Author are
// pre-formatted views of Ticket fields: the template prints the H1 number
// zero-padded (as bash tickets/bin/ticket does) and uses the timestamp
// layout tsLayout. Outer fields shadow the promoted Ticket fields. Stub and
// UserCommentsStub are the empty placeholders passed from the single
// detailsStub/userCommentsStub constants so each literal lives in exactly
// one place (wave-1 #3, T-0035).
type newTicketData struct {
	Ticket
	Number           string
	Timestamp        string
	Author           string
	Stub             string
	UserCommentsStub string
}

// RenderNewTicket renders the initial markdown for a new ticket from the
// embedded templates/new_ticket.md.tmpl.
func RenderNewTicket(t *Ticket) ([]byte, error) {
	tmpl, err := template.ParseFS(templates, "templates/new_ticket.md.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse ticket template: %w", err)
	}
	data := newTicketData{
		Ticket:           *t,
		Number:           fmt.Sprintf("%04d", t.Number),
		Timestamp:        t.Created.Format(tsLayout),
		Author:           t.Who,
		Stub:             detailsStub,
		UserCommentsStub: userCommentsStub,
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "new_ticket.md.tmpl", data); err != nil {
		return nil, fmt.Errorf("execute ticket template: %w", err)
	}
	return buf.Bytes(), nil
}
