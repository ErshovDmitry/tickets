package domain

import (
	"bytes"
	"fmt"
	"strings"
)

// normalizeDetails returns s without its trailing LF bytes. Renderers add
// their own "\n\n" separator after the details payload, so trailing LF would
// otherwise produce a triple newline (markdownlint MD012). Only LF is
// trimmed: CR of a CRLF payload, spaces and all other bytes are preserved.
func normalizeDetails(s string) string {
	return strings.TrimRight(s, "\n")
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
		b.WriteString(seg.Prefix)
		b.WriteString(string(t.Status))
		b.WriteString(seg.Suffix)
		b.WriteString("\n")
	} else {
		fmt.Fprintf(&b, d.statusFmt+"\n", t.Status)
	}
	if seg, ok := t.RawTemplates.Meta["Priority"]; ok {
		b.WriteString(seg.Prefix)
		b.WriteString(string(t.Priority))
		b.WriteString(seg.Suffix)
		b.WriteString("\n")
	} else {
		fmt.Fprintf(&b, d.priorityFmt+"\n", t.Priority)
	}
	if seg, ok := t.RawTemplates.Meta["Created"]; ok {
		b.WriteString(seg.Prefix)
		b.WriteString(t.Created.Format(tsLayout))
		b.WriteString(seg.Middle)
		b.WriteString(t.Who)
		b.WriteString(seg.Suffix)
		b.WriteString("\n")
	} else {
		fmt.Fprintf(&b, d.createdFmt+"\n", t.Created.Format(tsLayout), t.Who)
	}
	if seg, ok := t.RawTemplates.Meta["Project"]; ok {
		b.WriteString(seg.Prefix)
		b.WriteString(string(t.Project))
		b.WriteString(seg.Suffix)
		b.WriteString("\n")
	} else {
		fmt.Fprintf(&b, d.projectFmt+"\n", t.Project)
	}
	b.WriteString("\n")

	// Head-strays anchor right after the metadata block, before the
	// Summary header (T-0166).
	if len(t.HeadStray) > 0 {
		b.WriteString(strings.Join(t.HeadStray, "\n"))
		b.WriteString("\n\n")
	}

	// Section headers: prefer raw templates, else dict. Every header is
	// followed by a blank line (MD022 blanks-around-headings).
	if raw, ok := t.RawTemplates.Headers[secNameSummary]; ok {
		b.WriteString(raw)
		b.WriteString("\n\n")
	} else {
		b.WriteString(d.headers[0])
		b.WriteString("\n\n")
	}
	if len(t.Brief) > 0 {
		b.WriteString(strings.Join(t.Brief, "\n"))
		b.WriteString("\n\n")
	} else {
		fmt.Fprintf(&b, "%s\n\n", t.Title)
	}

	if raw, ok := t.RawTemplates.Headers[secNameDetails]; ok {
		b.WriteString(raw)
		b.WriteString("\n\n")
	} else {
		b.WriteString(d.headers[1])
		b.WriteString("\n\n")
	}
	// Only trailing LF bytes are normalized away: the separator below is
	// added unconditionally, so a payload keeping its trailing LF would
	// emit three consecutive newlines (markdownlint MD012). CR, spaces and
	// every other byte stay verbatim, and t.Details itself is not mutated.
	if details := normalizeDetails(t.Details); details != "" {
		b.WriteString(details)
	} else {
		b.WriteString(d.detailsStub)
	}
	b.WriteString("\n\n")

	if raw, ok := t.RawTemplates.Headers[secNameUserComments]; ok {
		b.WriteString(raw)
		b.WriteString("\n\n")
	} else {
		b.WriteString(d.headers[2])
		b.WriteString("\n\n")
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
		b.WriteString(raw)
		b.WriteString("\n\n")
	} else {
		b.WriteString(d.headers[3])
		b.WriteString("\n\n")
	}
	if t.Comments != "" {
		b.WriteString(t.Comments)
	}
	// Same conditional separator as User comments above.
	if t.Comments != "" {
		b.WriteString("\n\n")
	}

	if raw, ok := t.RawTemplates.Headers[secNameJournal]; ok {
		b.WriteString(raw)
		b.WriteString("\n\n")
	} else {
		b.WriteString(d.headers[4])
		b.WriteString("\n\n")
	}
	for _, e := range t.Journal {
		writeJournalLine(&b, e, t.Lang)
	}
	b.Write(unknown)
	return b.Bytes(), nil
}
