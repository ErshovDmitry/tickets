package domain

import (
	"bytes"
	"fmt"
	"regexp"
)

// Journal entry wire-format patterns. They live beside the journal codec so
// the "## Журнал" line grammar is defined in one place (T-0035 file split to
// keep markdown.go within the 300-line budget).
var (
	createdEntryRe = regexp.MustCompile(`^- (.+) — тикет создан \((.*)\)\.$`)
	transitionRe   = regexp.MustCompile(`^- (.+) — статус: (\S+) → (\S+)(?: · (.*))? \((.*)\)$`)
	archiveRe      = regexp.MustCompile(`^- (.+) — перенесён в архив \((.*)\)$`)
)

// appendJournalLine parses one journal line into t.Journal. It reports
// whether the line was a recognized creation, transition or archive entry.
// The three patterns are mutually exclusive alternatives, hence the
// else-if chain: at most one can match a given line.
func appendJournalLine(t *Ticket, line string) bool {
	if m := createdEntryRe.FindStringSubmatch(line); m != nil {
		t.Journal = append(t.Journal, JournalEntry{
			At:   parseTS(m[1]),
			From: StatusOpen,
			To:   StatusOpen,
			Who:  m[2],
		})
		return true
	} else if m := transitionRe.FindStringSubmatch(line); m != nil {
		t.Journal = append(t.Journal, JournalEntry{
			At:      parseTS(m[1]),
			From:    Status(m[2]),
			To:      Status(m[3]),
			Comment: m[4],
			Who:     m[5],
		})
		return true
	} else if m := archiveRe.FindStringSubmatch(line); m != nil {
		t.Journal = append(t.Journal, JournalEntry{
			At:       parseTS(m[1]),
			Who:      m[2],
			Archived: true,
		})
		return true
	}
	return false
}

// writeJournalLine emits one journal entry in the bash-compatible format:
// creation "- TS — тикет создан (who)." and
// transition "- TS — статус: <from> → <to>[ · comment] (who)".
func writeJournalLine(b *bytes.Buffer, e JournalEntry) {
	ts := e.At.Format(tsLayout)
	switch {
	case e.Archived:
		fmt.Fprintf(b, "- %s — перенесён в архив (%s)\n", ts, e.Who)
	case e.From == StatusOpen && e.To == StatusOpen && e.Comment == "":
		fmt.Fprintf(b, "- %s — тикет создан (%s).\n", ts, e.Who)
	case e.Comment == "":
		fmt.Fprintf(b, "- %s — статус: %s → %s (%s)\n", ts, e.From, e.To, e.Who)
	default:
		fmt.Fprintf(b, "- %s — статус: %s → %s · %s (%s)\n", ts, e.From, e.To, e.Comment, e.Who)
	}
}
