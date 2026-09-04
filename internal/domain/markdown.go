package domain

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// tsLayout is the timestamp format used everywhere in ticket files.
const tsLayout = "2006-01-02 15:04"

var (
	h1Re = regexp.MustCompile(`^# T-(\d+) · (\S+): (.*)$`)
)

type section uint8

const (
	secHead         section = iota // H1 + metadata block
	secBrief                       // inside "## Summary" (duplicates Title — ignored)
	secDetails                     // inside "## Details"
	secUserComments                // inside "## User comments" (T-0035)
	secComments                    // inside "## Comments" (T-0032, no bash counterpart)
	secJournal                     // inside "## Journal"
)

// Parse reads ticket markdown tolerantly: it never fails, missing or
// malformed parts become zero values, and any manual bytes after the last
// recognized journal line are preserved verbatim in Ticket.Unknown (also
// returned as the second result). Inside "## Журнал" the first line that is
// not a recognized entry — a blank or free-form manual line included — ends
// the recognized tail; everything from that byte on becomes Unknown by
// design so manual edits survive parse/render round trips intact.
func Parse(data []byte) (*Ticket, []byte, error) {
	t := &Ticket{
		RawTemplates: rawTemplates{
			Headers: make(map[sectionName]string),
			Meta:    make(map[string]metaSegments),
		},
	}
	var details, userComments, comments []string
	sec := secHead

	// Detect language first (LOCK D4).
	t.Lang = detectLang(data)

	finish := func() {
		t.Details = strings.Join(trimTrailingEmpty(details), "\n")
		if isStubLine(t.Details) {
			t.Details = "" // stub from any dict means empty Details
		}
		t.UserComments = strings.Join(trimTrailingEmpty(userComments), "\n")
		if isStubLine(t.UserComments) {
			// Placeholder means no user remarks. Known limitation (by
			// design): user text byte-identical to the stub is treated
			// the same.
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
	d := getDict(t.Lang)

	// H1 line is always the same format.
	fmt.Fprintf(&b, "# T-%04d · %s: %s\n\n", t.Number, t.Type, t.Title)

	// Meta lines: prefer raw segments — literal text around the value slots
	// captured at Parse time (T-0036). Segments contain no printf
	// placeholders, so a literal "%s" in a user free suffix or value is
	// emitted verbatim. Absent entries emit the dict form.
	if seg, ok := t.RawTemplates.Meta["Status"]; ok {
		b.WriteString(seg.Prefix + string(t.Status) + seg.Suffix + "\n")
	} else {
		fmt.Fprintf(&b, d.statusFmt+"\n", t.Status)
	}
	if seg, ok := t.RawTemplates.Meta["Priority"]; ok {
		b.WriteString(seg.Prefix + string(t.Priority) + seg.Suffix + "\n")
	} else {
		fmt.Fprintf(&b, d.priorityFmt+"\n", t.Priority)
	}
	if seg, ok := t.RawTemplates.Meta["Created"]; ok {
		b.WriteString(seg.Prefix + t.Created.Format(tsLayout) + seg.Middle + t.Who + seg.Suffix + "\n")
	} else {
		fmt.Fprintf(&b, d.createdFmt+"\n", t.Created.Format(tsLayout), t.Who)
	}
	if seg, ok := t.RawTemplates.Meta["Project"]; ok {
		b.WriteString(seg.Prefix + string(t.Project) + seg.Suffix + "\n")
	} else {
		fmt.Fprintf(&b, d.projectFmt+"\n", t.Project)
	}
	b.WriteString("\n")

	// Section headers: prefer raw templates, else dict.
	if raw, ok := t.RawTemplates.Headers[secNameSummary]; ok {
		b.WriteString(raw + "\n")
	} else {
		b.WriteString(d.headers[0] + "\n")
	}
	fmt.Fprintf(&b, "%s\n\n", t.Title)

	if raw, ok := t.RawTemplates.Headers[secNameDetails]; ok {
		b.WriteString(raw + "\n")
	} else {
		b.WriteString(d.headers[1] + "\n")
	}
	if t.Details == "" {
		b.WriteString(d.detailsStub)
	} else {
		b.WriteString(t.Details)
	}
	b.WriteString("\n\n")

	if raw, ok := t.RawTemplates.Headers[secNameUserComments]; ok {
		b.WriteString(raw + "\n")
	} else {
		b.WriteString(d.headers[2] + "\n")
	}
	// Empty section renders the bare header: the long placeholder is no
	// longer emitted (plan "User comments" Option A) — new tickets create
	// the section empty. Legacy files holding the placeholder still parse
	// to "" via isStubLine. Note: the shorter T-0034 stub variant is NOT
	// an isStubLine match and stays content by design.
	if t.UserComments != "" {
		b.WriteString(t.UserComments)
	}
	// Conditional separator mirrors the Comments section: an empty section
	// yields exactly one blank line before the next header — never "\n\n\n".
	if t.UserComments == "" {
		b.WriteString("\n")
	} else {
		b.WriteString("\n\n")
	}

	if raw, ok := t.RawTemplates.Headers[secNameComments]; ok {
		b.WriteString(raw + "\n")
	} else {
		b.WriteString(d.headers[3] + "\n")
	}
	if t.Comments != "" {
		b.WriteString(t.Comments)
	}
	// Conditional journal separator (review fix c2, T-0035): with an empty
	// free section the output must be "## Комментарии\n\n## Журнал\n" —
	// exactly one blank line, never "\n\n\n".
	if t.Comments == "" {
		b.WriteString("\n")
	} else {
		b.WriteString("\n\n")
	}

	if raw, ok := t.RawTemplates.Headers[secNameJournal]; ok {
		b.WriteString(raw + "\n")
	} else {
		b.WriteString(d.headers[4] + "\n")
	}
	for _, e := range t.Journal {
		writeJournalLine(&b, e, t.Lang)
	}
	b.Write(unknown)
	return b.Bytes(), nil
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
