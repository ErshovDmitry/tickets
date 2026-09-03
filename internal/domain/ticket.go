// Package domain holds the ticket model and its markdown/filename codecs.
// The layout is byte-compatible with the bash reference tickets/bin/ticket
// except for the "## Комментарии от пользователя" and "## Комментарии"
// sections (T-0032, T-0035): the Go implementation emits them on every
// ticket it renders, while bash has no such sections — bash byte-compat of
// `new` is intentionally diverged for new tickets.
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
	// "## Комментарии от пользователя" user remarks; empty renders the stub.
	// The agent reads it before working on the ticket and never writes here.
	UserComments string
	// "## Комментарии" free-form notes (agent working notes etc.); empty
	// renders a bare header.
	Comments string
	Who      string
	Project  string
	Created  time.Time
	Journal  []JournalEntry
	// Unknown holds manual bytes after the journal section, preserved verbatim.
	Unknown []byte

	// Lang is the detected language of the ticket file (LOCK D4). It affects
	// only: emission of new journal lines, stub choice for empty sections,
	// default forms for absent components. It does NOT affect raw templates.
	Lang Lang

	// RawTemplates stores the original lines recognized during Parse
	// (LOCK D1). Render prefers these over dict emission, guaranteeing
	// byte-stability for free suffixes like "## Summary (任意语言)". Zero/absent
	// entries mean "not recognized" -> Render emits dict form.
	RawTemplates rawTemplates
}

// rawTemplates holds original lines from the parsed file.
type rawTemplates struct {
	// Section headers: original full line for each recognized section, keyed
	// by canonical section name.
	Headers map[sectionName]string

	// Meta forms: original lines split into literal segments around their
	// value slots (T-0036). No printf placeholders are stored, so a literal
	// "%s" in a user free suffix or value survives round trips byte-
	// identically.
	Meta map[string]metaSegments
}

// metaSegments is one meta line as literal text around its value slots.
// Single-slot metas (Status/Priority/Project) render Prefix+value+Suffix
// (Middle is empty); the two-slot Created renders
// Prefix+timestamp+Middle+author+Suffix.
type metaSegments struct {
	Prefix string // literal text before the first value slot
	Middle string // Created only: literal text between ts and author slots
	Suffix string // literal text after the last value slot
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

	// Raw is the original line text for entries recognized during Parse
	// (plan §B invariant). Non-empty Raw re-emits verbatim regardless of
	// t.Lang, so a journal line in a foreign dict form (e.g. RU line in a
	// file detected as EN) survives round trips byte-identically. Empty Raw
	// (entries appended by set/archive) emits the t.Lang dict form.
	Raw string
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
