package cli

// T-0241 A3.1: the `show --json` document.
//
// One valid JSON document on stdout, zero diagnostics there (every diagnostic
// goes to stderr) and exactly one trailing LF. A streaming json.Encoder would
// leave a prefix on stdout if the write failed halfway, so the document is
// marshalled into a buffer and written in ONE call; a short write is an error,
// never patched up with a second write.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"time"
	"unicode/utf8"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// showJSONTicket is the frozen field set (declaration order = key order). No
// field is omitempty: every key is always present, so a consumer can address
// each one without a presence check.
type showJSONTicket struct {
	Number          int             `json:"number"`
	Status          string          `json:"status"`
	Type            string          `json:"type"`
	Priority        string          `json:"priority"`
	Project         string          `json:"project"`
	Title           string          `json:"title"`
	Summary         string          `json:"summary"`
	Created         string          `json:"created"`
	Who             string          `json:"who"`
	Details         string          `json:"details"`
	UserComments    string          `json:"user_comments"`
	Comments        string          `json:"comments"`
	Journal         []showJSONEntry `json:"journal"`
	Unknown         string          `json:"unknown"`
	UnknownEncoding string          `json:"unknown_encoding"`
	Warnings        []string        `json:"warnings"`
}

// showJSONEntry is one structured journal record. The original journal line
// (JournalEntry.Raw) is deliberately absent: the structured fields carry the
// same facts and a raw string would invite a second, divergent parser.
type showJSONEntry struct {
	At       string `json:"at"`
	From     string `json:"from"`
	To       string `json:"to"`
	Who      string `json:"who"`
	Comment  string `json:"comment"`
	Archived bool   `json:"archived"`
}

// runShowJSON writes the JSON document for an already located ticket: it never
// reads the store again and never rejects a ticket for parse anomalies — those
// are reported inside the document.
func runShowJSON(stdout, stderr io.Writer, lang domain.Lang, tk domain.Ticket, unknown []byte, warnings []store.ParseWarning) int {
	payload, err := json.Marshal(newShowJSONTicket(tk, unknown, warnings))
	if err != nil {
		fmt.Fprintln(stderr, domain.ErrJSONEncodeShow(lang, err))
		return exitError
	}
	// json.Marshal never appends a newline; exactly one is added here.
	payload = append(payload, '\n')
	n, werr := stdout.Write(payload)
	if werr != nil {
		return reportShowWriteError(stderr, lang, werr)
	}
	if n != len(payload) {
		fmt.Fprintln(stderr, domain.ErrJSONShortWriteShow(lang, n, len(payload)))
		return exitError
	}
	return exitOK
}

// newShowJSONTicket converts a parsed ticket into the DTO. Status comes from
// the file name (the store already overwrote the untrusted body status);
// summary is the Summary section body; journal and warnings are always
// non-nil so they marshal as [] and never as null.
func newShowJSONTicket(tk domain.Ticket, unknown []byte, warnings []store.ParseWarning) showJSONTicket {
	entries := make([]showJSONEntry, 0, len(tk.Journal))
	for _, e := range tk.Journal {
		entries = append(entries, showJSONEntry{
			At:       showJSONTime(e.At),
			From:     string(e.From),
			To:       string(e.To),
			Who:      e.Who,
			Comment:  e.Comment,
			Archived: e.Archived,
		})
	}
	text, encoding := showJSONUnknown(unknown)
	return showJSONTicket{
		Number:          tk.Number,
		Status:          string(tk.Status),
		Type:            string(tk.Type),
		Priority:        string(tk.Priority),
		Project:         tk.Project,
		Title:           tk.Title,
		Summary:         tk.SectionBody(domain.SectionSummary),
		Created:         showJSONTime(tk.Created),
		Who:             tk.Who,
		Details:         tk.Details,
		UserComments:    tk.UserComments,
		Comments:        tk.Comments,
		Journal:         entries,
		Unknown:         text,
		UnknownEncoding: encoding,
		Warnings:        showJSONWarnings(warnings),
	}
}

// showJSONTime formats one documented timestamp layout. A zero time (an
// unparsable source timestamp) becomes "" rather than a fabricated instant.
func showJSONTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// showJSONUnknown renders the manual trailing bytes losslessly: valid UTF-8 is
// emitted as text, anything else as base64. A plain []byte-to-string conversion
// is forbidden here — it would replace invalid bytes with U+FFFD and lose them.
func showJSONUnknown(unknown []byte) (string, string) {
	if utf8.Valid(unknown) {
		return string(unknown), "utf-8"
	}
	return base64.StdEncoding.EncodeToString(unknown), "base64"
}

// showJSONWarnings renders the parse warnings as their user-facing one-liners
// ("<name>: <err>", or just the name when the warning carries no error). The
// service reason code never enters the text.
func showJSONWarnings(warnings []store.ParseWarning) []string {
	out := make([]string, 0, len(warnings))
	for _, w := range warnings {
		out = append(out, w.Error())
	}
	return out
}
