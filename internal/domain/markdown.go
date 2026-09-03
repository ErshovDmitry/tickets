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

// userCommentsStub is the visible (italic) placeholder under
// "## Комментарии от пользователя" (T-0035) inviting user remarks.
// Like detailsStub it means "empty" once parsed back: a section holding
// exactly this one line blanks to "" on Parse, and Render emits the stub
// only for an empty section — user text is kept verbatim, so the placeholder
// never duplicates and parse→render round trips stay byte-stable. Known
// limitation (by design): user text byte-identical to the stub is
// indistinguishable from the placeholder and blanks to "" as well.
const userCommentsStub = `_Замечания пользователя: пишите сюда — агент прочитает эту секцию перед работой над тикетом. Агент сюда не пишет._`

var (
	h1Re       = regexp.MustCompile(`^# T-(\d+) · (\S+): (.*)$`)
	statusRe   = regexp.MustCompile(`^- Статус: (.*)$`)
	priorityRe = regexp.MustCompile(`^- Приоритет: (.*)$`)
	createdRe  = regexp.MustCompile(`^- Создан: (.+) · кем: (.*)$`)
	projectRe  = regexp.MustCompile(`^- Проект: (.*)$`)
)

type section uint8

const (
	secHead         section = iota // H1 + metadata block
	secBrief                       // inside "## Кратко" (duplicates Title — ignored)
	secDetails                     // inside "## Подробности"
	secUserComments                // inside "## Комментарии от пользователя" (T-0035)
	secComments                    // inside "## Комментарии" (T-0032, no bash counterpart)
	secJournal                     // inside "## Журнал"
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
	var details, userComments, comments []string
	sec := secHead
	finish := func() {
		t.Details = strings.Join(trimTrailingEmpty(details), "\n")
		if t.Details == detailsStub {
			t.Details = "" // bash placeholder means empty Details
		}
		t.UserComments = strings.Join(trimTrailingEmpty(userComments), "\n")
		if t.UserComments == userCommentsStub {
			// Placeholder means no user remarks. Known limitation (by
			// design): user text byte-identical to the stub is treated
			// the same — see userCommentsStub.
			t.UserComments = ""
		}
		// The free-form "## Комментарии" section has no stub: its bytes are
		// kept verbatim with no blanking (T-0035).
		t.Comments = strings.Join(trimTrailingEmpty(comments), "\n")
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
		sec = absorbLine(t, &details, &userComments, &comments, sec, line)
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
	b.WriteString("\n\n## Комментарии от пользователя\n")
	if t.UserComments == "" {
		b.WriteString(userCommentsStub)
	} else {
		b.WriteString(t.UserComments)
	}
	b.WriteString("\n\n## Комментарии\n")
	if t.Comments != "" {
		b.WriteString(t.Comments)
	}
	// Conditional journal separator (review fix c2, T-0035): with an empty
	// free section the output must be "## Комментарии\n\n## Журнал\n" —
	// exactly one blank line, never "\n\n\n".
	if t.Comments == "" {
		b.WriteString("\n## Журнал\n")
	} else {
		b.WriteString("\n\n## Журнал\n")
	}
	for _, e := range t.Journal {
		writeJournalLine(&b, e)
	}
	b.Write(unknown)
	return b.Bytes(), nil
}

// absorbLine consumes one line outside the journal section, updating the
// ticket and returning the new section.
func absorbLine(t *Ticket, details, userComments, comments *[]string, sec section, line string) section {
	switch sec {
	case secDetails:
		switch line {
		case "## Комментарии от пользователя":
			return secUserComments
		case "## Комментарии":
			return secComments
		case "## Журнал":
			return secJournal
		}
		*details = append(*details, line)
		return sec
	case secUserComments:
		// Only the next section's header ends the user-remarks section:
		// anything else — stray headers and a duplicate own header included —
		// stays verbatim (content-preserving policy, mirrors secComments).
		switch line {
		case "## Комментарии":
			return secComments
		case "## Журнал":
			return secJournal
		}
		*userComments = append(*userComments, line)
		return sec
	case secComments:
		// Only the journal header ends the section: stray lines — other
		// headers and duplicate "## Комментарии" included — stay verbatim
		// (content-preserving policy, mirrors secDetails).
		if line == "## Журнал" {
			return secJournal
		}
		*comments = append(*comments, line)
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

func headerName(line string) (string, bool) {
	return strings.CutPrefix(line, "## ")
}

func sectionFor(name string) section {
	switch name {
	case "Кратко":
		return secBrief
	case "Подробности":
		return secDetails
	case "Комментарии от пользователя":
		return secUserComments
	case "Комментарии":
		return secComments
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
