// Package domain holds the ticket model and its markdown/filename codecs.
// The layout is byte-compatible with the bash reference tickets/bin/ticket
// except for the "## Комментарии" section (T-0032): the Go implementation
// emits it on every ticket it renders, while bash has no such section —
// bash byte-compat of `new` is intentionally diverged for new tickets.
package domain

import (
	"fmt"
	"strings"
	"time"
)

// Mode selects how strict Ticket validation is.
type Mode uint8

const (
	// Tolerant accepts anything — used when reading existing files.
	Tolerant Mode = iota
	// Strict enforces all new-ticket invariants.
	Strict
)

// Ticket is one ticket file. Fields mirror the markdown layout produced by
// tickets/bin/ticket.
type Ticket struct {
	Number   int
	Status   Status
	Type     Type
	Priority Priority
	Title    string
	Details  string
	Comments string // "## Комментарии" user text; empty renders the placeholder
	Who      string
	Project  string
	Created  time.Time
	Journal  []JournalEntry
	// Unknown holds manual bytes after the journal section, preserved verbatim.
	Unknown []byte
}

// JournalEntry is one line of the "## Журнал" section. The creation entry has
// From == To == StatusOpen and an empty Comment. Archived entries have
// Archived=true and empty From/To/Comment: archival is a directory move, not a
// status transition.
type JournalEntry struct {
	At       time.Time
	From     Status
	To       Status
	Comment  string
	Who      string
	Archived bool
}

// Validate checks the ticket. In Strict mode (new tickets) it requires a
// positive Number, valid Status/Type/Priority and a non-empty Title;
// Details and Who may be empty. Tolerant mode always succeeds.
func (t Ticket) Validate(mode Mode) error {
	if mode == Tolerant {
		return nil
	}
	if t.Number <= 0 {
		return fmt.Errorf("номер тикета должен быть > 0")
	}
	if _, err := ParseStatus(string(t.Status)); err != nil {
		return err
	}
	if _, err := ParseType(string(t.Type)); err != nil {
		return err
	}
	if _, err := ParsePriority(string(t.Priority)); err != nil {
		return err
	}
	if strings.TrimSpace(t.Title) == "" {
		return fmt.Errorf("краткое описание не может быть пустым")
	}
	return nil
}
