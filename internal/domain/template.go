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
// layout tsLayout. Outer fields shadow the promoted Ticket fields. All
// localized strings — the four meta lines, the five section headers, the
// empty-section stubs and the journal creation line — are pre-formatted
// from the getDict(t.Lang) dictionary so the template file itself carries
// no language-specific text (T-0036).
type newTicketData struct {
	Ticket
	Number           string
	Timestamp        string
	Author           string
	Stub             string
	UserCommentsStub string

	// Meta lines with their values already substituted.
	StatusLine   string
	PriorityLine string
	CreatedLine  string
	ProjectLine  string

	// Section header lines, dict order: Summary, Details, UserComments,
	// Comments, Journal.
	HeaderSummary      string
	HeaderDetails      string
	HeaderUserComments string
	HeaderComments     string
	HeaderJournal      string

	// Journal creation line for the single creation entry.
	JournalCreation string
}

// newData assembles newTicketData for t from its language dictionary.
func newData(t *Ticket) newTicketData {
	d := getDict(t.Lang)
	return newTicketData{
		Ticket:             *t,
		Number:             fmt.Sprintf("%04d", t.Number),
		Timestamp:          t.Created.Format(tsLayout),
		Author:             t.Who,
		Stub:               d.detailsStub,
		UserCommentsStub:   d.userCommentsStub,
		StatusLine:         fmt.Sprintf(d.statusFmt, t.Status),
		PriorityLine:       fmt.Sprintf(d.priorityFmt, t.Priority),
		CreatedLine:        fmt.Sprintf(d.createdFmt, t.Created.Format(tsLayout), t.Who),
		ProjectLine:        fmt.Sprintf(d.projectFmt, t.Project),
		HeaderSummary:      d.headers[0],
		HeaderDetails:      d.headers[1],
		HeaderUserComments: d.headers[2],
		HeaderComments:     d.headers[3],
		HeaderJournal:      d.headers[4],
		JournalCreation:    creationLine(t, d),
	}
}

// creationLine formats the journal creation entry. A new ticket carries
// exactly one creation entry whose At/Who equal the ticket's Created/Who
// (cmd_new seeds it); the first journal entry is preferred to mirror the
// previous range-based rendering, with the ticket fields as fallback so
// the creation line is always emitted.
func creationLine(t *Ticket, d *dict) string {
	ts, who := t.Created.Format(tsLayout), t.Who
	if len(t.Journal) > 0 {
		ts, who = t.Journal[0].At.Format(tsLayout), t.Journal[0].Who
	}
	return fmt.Sprintf(d.journalCreation, ts, who)
}

// RenderNewTicket renders the initial markdown for a new ticket from the
// embedded templates/new_ticket.md.tmpl.
func RenderNewTicket(t *Ticket) ([]byte, error) {
	tmpl, err := template.ParseFS(templates, "templates/new_ticket.md.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse ticket template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "new_ticket.md.tmpl", newData(t)); err != nil {
		return nil, fmt.Errorf("execute ticket template: %w", err)
	}
	return buf.Bytes(), nil
}
