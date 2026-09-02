package domain

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// tsLayout is the timestamp format used everywhere in ticket files.
const tsLayout = "2006-01-02 15:04"

// detailsStub is the placeholder bash writes when Details are empty.
const detailsStub = `<!-- что найдено, где (файл:строка), логи/вывод, как воспроизвести, предложение по исправлению -->`

var (
	h1Re           = regexp.MustCompile(`^# T-(\d+) · (\S+): (.*)$`)
	statusRe       = regexp.MustCompile(`^- Статус: (.*)$`)
	priorityRe     = regexp.MustCompile(`^- Приоритет: (.*)$`)
	createdRe      = regexp.MustCompile(`^- Создан: (.+) · кем: (.*)$`)
	projectRe      = regexp.MustCompile(`^- Проект: (.*)$`)
	createdEntryRe = regexp.MustCompile(`^- (.+) — тикет создан \((.*)\)\.$`)
	transitionRe   = regexp.MustCompile(`^- (.+) — статус: (\S+) → (\S+)(?: · (.*))? \((.*)\)$`)
	archiveRe      = regexp.MustCompile(`^- (.+) — перенесён в архив \((.*)\)$`)
)

type section uint8

const (
	secHead    section = iota // H1 + metadata block
	secBrief                  // inside "## Кратко" (duplicates Title — ignored)
	secDetails                // inside "## Подробности"
	secJournal                // inside "## Журнал"
)

// Parse reads ticket markdown tolerantly: it never fails, missing or
// malformed parts become zero values, and any manual bytes after the last
// recognized journal line are preserved verbatim in Ticket.Unknown (also
// returned as the second result). Inside "## Журнал" the first line that is
// not a recognized entry — a blank or free-form manual line included — ends
// the recognized tail; everything from that byte on becomes Unknown by
// design so manual edits survive parse/render round trips intact.
func Parse(data []byte) (*Ticket, []byte, error) {
	t := &Ticket{}
	var details []string
	sec := secHead
	finish := func() {
		t.Details = strings.Join(trimTrailingEmpty(details), "\n")
		if t.Details == detailsStub {
			t.Details = "" // bash placeholder means empty Details
		}
	}
	for pos := 0; pos < len(data); {
		start, end := lineBounds(data, pos)
		pos = end
		line := string(trimEOL(data[start:end]))
		if sec == secJournal {
			if !appendJournalLine(t, line) {
				// Unknown tail: stop early — but Details must still be
				// finalized exactly as in the normal path, otherwise a
				// manual journal tail renders an empty stub (round-trip
				// regression fixed during integration).
				t.Unknown = data[start:]
				finish()
				return t, t.Unknown, nil
			}
			continue
		}
		sec = absorbLine(t, &details, sec, line)
	}
	finish()
	return t, t.Unknown, nil
}

// Render re-emits the canonical markdown for the ticket and appends unknown
// (manual) bytes after the journal section. Rendering into a bytes.Buffer
// cannot fail, so the returned error is always nil; it exists so callers can
// handle rendering as a fallible operation.
func Render(t *Ticket, unknown []byte) ([]byte, error) {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# T-%04d · %s: %s\n\n", t.Number, t.Type, t.Title)
	fmt.Fprintf(&b, "- Статус: %s\n", t.Status)
	fmt.Fprintf(&b, "- Приоритет: %s\n", t.Priority)
	fmt.Fprintf(&b, "- Создан: %s · кем: %s\n", t.Created.Format(tsLayout), t.Who)
	fmt.Fprintf(&b, "- Проект: %s\n\n", t.Project)
	fmt.Fprintf(&b, "## Кратко\n%s\n\n## Подробности\n", t.Title)
	if t.Details == "" {
		b.WriteString(detailsStub)
	} else {
		b.WriteString(t.Details)
	}
	b.WriteString("\n\n## Журнал\n")
	for _, e := range t.Journal {
		writeJournalLine(&b, e)
	}
	b.Write(unknown)
	return b.Bytes(), nil
}

// absorbLine consumes one line outside the journal section, updating the
// ticket and returning the new section.
func absorbLine(t *Ticket, details *[]string, sec section, line string) section {
	switch sec {
	case secDetails:
		if line == "## Журнал" {
			return secJournal
		}
		*details = append(*details, line)
		return sec
	case secBrief:
		if h, ok := headerName(line); ok {
			return sectionFor(h)
		}
		return sec
	default: // secHead
		if applyHeadMeta(t, line) {
			return sec
		}
		if h, ok := headerName(line); ok {
			return sectionFor(h)
		}
		return sec
	}
}

// applyHeadMeta captures the H1 line and the metadata block lines.
func applyHeadMeta(t *Ticket, line string) bool {
	if m := h1Re.FindStringSubmatch(line); m != nil {
		t.Number, _ = strconv.Atoi(m[1]) // tolerant: overflow -> 0
		t.Type = Type(m[2])
		t.Title = m[3]
		return true
	}
	if m := statusRe.FindStringSubmatch(line); m != nil {
		t.Status = Status(m[1])
		return true
	}
	if m := priorityRe.FindStringSubmatch(line); m != nil {
		t.Priority = Priority(m[1])
		return true
	}
	if m := createdRe.FindStringSubmatch(line); m != nil {
		t.Created, t.Who = parseTS(m[1]), m[2]
		return true
	}
	if m := projectRe.FindStringSubmatch(line); m != nil {
		t.Project = m[1]
		return true
	}
	return false
}

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

func headerName(line string) (string, bool) {
	return strings.CutPrefix(line, "## ")
}

func sectionFor(name string) section {
	switch name {
	case "Кратко":
		return secBrief
	case "Подробности":
		return secDetails
	case "Журнал":
		return secJournal
	default:
		return secBrief // unknown header: ignore its content
	}
}

func lineBounds(data []byte, pos int) (start, end int) {
	start = pos
	if i := bytes.IndexByte(data[pos:], '\n'); i >= 0 {
		end = pos + i + 1
	} else {
		end = len(data)
	}
	return start, end
}

func trimEOL(line []byte) []byte {
	line = bytes.TrimSuffix(line, []byte("\n"))
	return bytes.TrimSuffix(line, []byte("\r"))
}

func parseTS(s string) time.Time {
	t, err := time.Parse(tsLayout, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func trimTrailingEmpty(lines []string) []string {
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
