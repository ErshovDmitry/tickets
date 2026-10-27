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
	secComments                    // inside "## Comments" (T-0032, no bash-era counterpart)
	secJournal                     // inside "## Journal"
)

// Parse reads ticket markdown tolerantly: it never fails, missing or
// malformed parts become zero values, and any manual bytes after the last
// recognized journal line are preserved verbatim in Ticket.Unknown (also
// returned as the second result). Inside "## Журнал" blank lines are
// deferred, not terminating: a recognized section header after the journal
// resumes parsing (a repeated Journal header merges its entries and sets
// Ticket.JournalDup), while the first non-blank, non-header line still ends
// the recognized tail — Unknown starts at any pending blank run, EXCEPT the
// single strictly empty line Render emits right after the first Journal
// header (MD022): that one is markup, not content, and is swallowed, keeping
// a ticket with an empty journal byte-stable across parse/render cycles. A
// whitespace-only line is NOT that markup: it stays verbatim in Unknown. A
// pending blank run at EOF becomes Unknown too, so trailing blank bytes
// survive (blanks after the LAST journal entry included).
func Parse(data []byte) (*Ticket, []byte, error) {
	t := &Ticket{
		RawTemplates: rawTemplates{
			Headers: make(map[sectionName]string),
			Meta:    make(map[string]metaSegments),
		},
	}
	var details, userComments, comments []string
	sec := secHead
	seenJournal := false
	// journalHeaderBlank marks the one strictly empty MD022 line accepted
	// right after the FIRST journal header (see the doc comment). It is
	// raised only on entering the journal section and cleared by the next
	// line.
	journalHeaderBlank := false

	// Detect language first (LOCK D4).
	t.Lang = detectLang(data)

	// At most ONE leading blank line is trimmed alongside trailing ones:
	// Render emits exactly one blank line after each section header
	// (MD022), and without that trim the blank would be re-absorbed as
	// content and accumulate one extra blank per parse/render round trip.
	// Any FURTHER leading blank lines are content and stay verbatim
	// (T-0035: the "## Comments" section is byte-verbatim).
	finish := func() {
		t.Details = strings.Join(trimTrailingEmpty(trimLeadingOneEmpty(details)), "\n")
		if isStubLine(strings.TrimLeft(t.Details, "\n")) {
			t.Details = "" // stub from any dict means empty Details
		}
		t.UserComments = strings.Join(trimTrailingEmpty(trimLeadingOneEmpty(userComments)), "\n")
		if isStubLine(strings.TrimLeft(t.UserComments, "\n")) {
			// Placeholder means no user remarks. Known limitation (by
			// design): user text byte-identical to the stub is treated
			// the same.
			t.UserComments = ""
		}
		// The free-form "## Comments (Комментарии)" section has no stub: only
		// ONE leading blank line is trimmed — the one Render prints after the
		// header (MD022). Any further leading blank line is content and stays
		// byte-verbatim (T-0035).
		t.Comments = strings.Join(trimTrailingEmpty(trimLeadingOneEmpty(comments)), "\n")
	}
	blankRunStart := -1
	for pos := 0; pos < len(data); {
		start, end := lineBounds(data, pos)
		pos = end
		line := string(trimEOL(data[start:end]))
		if sec == secJournal {
			if journalHeaderBlank {
				// Exactly one STRICTLY empty line after the first journal
				// header is the MD022 blank Render prints: swallow it rather
				// than start a pending blank run. A whitespace-only line is
				// content and starts that run verbatim.
				journalHeaderBlank = false
				if line == "" {
					continue
				}
			}
			if strings.TrimSpace(line) == "" {
				if blankRunStart < 0 {
					blankRunStart = start
				}
				continue
			}
			if name, _, _, ok := matchSectionHeader(line); ok {
				blankRunStart = -1
				if name == secNameJournal {
					t.JournalDup = true
					continue
				}
				t.RawTemplates.Headers[name] = line
				sec = sectionFor(string(name))
				continue
			}
			if !appendJournalLine(t, line) {
				if blankRunStart >= 0 {
					start = blankRunStart
				}
				// Unknown tail: stop early — but Details must still be
				// finalized exactly as in the normal path, otherwise a
				// manual journal tail renders an empty stub (round-trip
				// regression fixed during integration).
				t.Unknown = data[start:]
				finish()
				return t, t.Unknown, nil
			}
			blankRunStart = -1
			continue
		}
		sec = absorbLine(t, &details, &userComments, &comments, sec, line)
		if sec == secJournal {
			// A Journal header reached from any non-journal section (head,
			// brief, details, user comments, comments) is the first or a
			// repeat journal: flag repeats so the store can warn, same as
			// the in-journal adjacent-dup branch above (T-0070 F1).
			if seenJournal {
				// A repeated journal header (adjacent or not) never raises the
				// blank-flag: the blank line after it is separators/content.
				t.JournalDup = true
			} else {
				seenJournal = true
				journalHeaderBlank = true
			}
		}
	}
	if blankRunStart >= 0 {
		t.Unknown = data[blankRunStart:]
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

	// Section headers: prefer raw templates, else dict. Every header is
	// followed by a blank line (MD022 blanks-around-headings).
	if raw, ok := t.RawTemplates.Headers[secNameSummary]; ok {
		b.WriteString(raw + "\n\n")
	} else {
		b.WriteString(d.headers[0] + "\n\n")
	}
	fmt.Fprintf(&b, "%s\n\n", t.Title)

	if raw, ok := t.RawTemplates.Headers[secNameDetails]; ok {
		b.WriteString(raw + "\n\n")
	} else {
		b.WriteString(d.headers[1] + "\n\n")
	}
	if t.Details == "" {
		b.WriteString(d.detailsStub)
	} else {
		b.WriteString(t.Details)
	}
	b.WriteString("\n\n")

	if raw, ok := t.RawTemplates.Headers[secNameUserComments]; ok {
		b.WriteString(raw + "\n\n")
	} else {
		b.WriteString(d.headers[2] + "\n\n")
	}
	// Empty section renders the bare header: the long placeholder is no
	// longer emitted (plan "User comments" Option A) — new tickets create
	// the section empty. Legacy files holding the placeholder still parse
	// to "" via isStubLine. Note: the shorter T-0034 stub variant is NOT
	// an isStubLine match and stays content by design.
	if t.UserComments != "" {
		b.WriteString(t.UserComments)
	}
	// Separator after content: a non-empty section ends with a blank line
	// before the next header; an empty section needs nothing — the header
	// already emitted its own trailing blank line.
	if t.UserComments != "" {
		b.WriteString("\n\n")
	}

	if raw, ok := t.RawTemplates.Headers[secNameComments]; ok {
		b.WriteString(raw + "\n\n")
	} else {
		b.WriteString(d.headers[3] + "\n\n")
	}
	if t.Comments != "" {
		b.WriteString(t.Comments)
	}
	// Same conditional separator as User comments above.
	if t.Comments != "" {
		b.WriteString("\n\n")
	}

	if raw, ok := t.RawTemplates.Headers[secNameJournal]; ok {
		b.WriteString(raw + "\n\n")
	} else {
		b.WriteString(d.headers[4] + "\n\n")
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

// trimLeadingOneEmpty drops AT MOST ONE blank line at the start of a
// section body: exactly the blank line Render emits after each section
// header (MD022) must not become content on the next parse. Every further
// leading blank line is content and stays verbatim — the free "## Comments"
// section is byte-verbatim (T-0035).
func trimLeadingOneEmpty(lines []string) []string {
	if len(lines) > 0 && lines[0] == "" {
		return lines[1:]
	}
	return lines
}
