package store

// T-0240 A1 tests: AddComment/SetSection — append/replace, DS6 ATX guard,
// DS7 LF/CRLF idempotency, DS8 surroundings preservation, DS9 byte-identical
// refusals, lookup/lock gates (edit policy).

import (
	"bytes"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// seededStore is a fresh store holding ticket 1 (fakeTicket).
func seededStore(t *testing.T) (*Store, string) {
	t.Helper()
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	return s, dir
}

// assertWriteRefused pins DS9: a refused write leaves the file byte-identical
// (journal included) and no tmp behind.
func assertWriteRefused(t *testing.T, dir string, n int, before []byte, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("write succeeded, want refusal")
	}
	if after := mustRead(t, dir, n); !bytes.Equal(after, before) {
		t.Fatalf("file changed on refusal:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	assertNoTmpFiles(t, dir)
}

func TestAddComment_AppendsBlock(t *testing.T) {
	s, dir := seededStore(t)
	if _, err := s.AddComment(1, "first note", "alice"); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	assertJournalTransition(t, string(mustRead(t, dir, 1)), "open", "open", "comment: +1 строк", "alice")
	if _, err := s.AddComment(1, "second\nnote", "bob"); err != nil {
		t.Fatalf("AddComment 2: %v", err)
	}
	body := string(mustRead(t, dir, 1))
	if !strings.Contains(body, "first note\n\nsecond\nnote") || strings.Contains(body, "\n\n\n") {
		t.Fatalf("want one blank separator, no accumulation:\n%s", body)
	}
	if tr := journalTransitions(body); len(tr) != 2 || !strings.Contains(tr[1], "comment: +2 строк (bob)") {
		t.Fatalf("journal = %v, want 2 single-line events", tr)
	}
}

// TestWriteText_Refusals covers the DS6 ATX guard (with offending line
// number, both operations), protected/unknown sections, empty comment
// payload and journal-unsafe who — every refusal byte-identical (DS9).
func TestWriteText_Refusals(t *testing.T) {
	cases := []struct {
		name    string
		sec     domain.Section // zero = AddComment
		payload string
		who     string
		want    string // error fragment
		line    int    // PayloadHeadingError.Line (0 = not a heading error)
		ok      bool   // payload must be accepted
	}{
		{name: "atx journal", payload: "## Journal", want: "markdown heading", line: 1},
		{name: "atx h1", payload: "# title", want: "markdown heading", line: 1},
		{name: "atx h6", payload: "###### deep", want: "markdown heading", line: 1},
		{name: "atx second line", payload: "ok\n## Details", want: "markdown heading", line: 2},
		{name: "atx third line crlf", payload: "a\r\nb\r\n### x", want: "markdown heading", line: 3},
		{name: "atx bare hashes", payload: "text\n##", want: "markdown heading", line: 2},
		{name: "atx section op", sec: domain.SectionDetails, payload: "## x", want: "markdown heading", line: 1},
		{name: "seven hashes ok", payload: "####### not a heading", ok: true},
		{name: "hash inside ok", payload: "issue #42 and C#", ok: true},
		{name: "no space ok", payload: "#tag", ok: true},
		{name: "indented ok", payload: "  ## indented", ok: true},
		{name: "protected user comments", sec: domain.SectionUserComments, payload: "x", want: "not editable"},
		{name: "protected journal", sec: domain.SectionJournal, payload: "x", want: "not editable"},
		{name: "unknown section", sec: domain.Section("bogus"), payload: "x", want: "not editable"},
		{name: "empty comment", payload: "", want: "empty payload"},
		{name: "empty comment lf", payload: "\n\n", want: "empty payload"},
		{name: "who newline", payload: "x", who: "a\nb", want: "line break"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, dir := seededStore(t)
			before := mustRead(t, dir, 1)
			who := tc.who
			if who == "" {
				who = "alice"
			}
			var err error
			if tc.sec == "" {
				_, err = s.AddComment(1, tc.payload, who)
			} else {
				_, err = s.SetSection(1, tc.sec, tc.payload, who)
			}
			if tc.ok {
				if err != nil {
					t.Fatalf("payload %q refused: %v", tc.payload, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want fragment %q", err, tc.want)
			}
			var he *PayloadHeadingError
			if errors.As(err, &he) && he.Line != tc.line {
				t.Fatalf("heading line = %d, want %d", he.Line, tc.line)
			}
			assertWriteRefused(t, dir, 1, before, err)
		})
	}
}

// TestWriteText_ValidationSkipsLookup: payload/argument errors precede the
// lookup — a missing number never masks them (plan case 10).
func TestWriteText_ValidationSkipsLookup(t *testing.T) {
	s, dir := newStore(t)
	var he *PayloadHeadingError
	if _, err := s.AddComment(99, "## x", "alice"); !errors.As(err, &he) {
		t.Fatalf("err = %v, want payload error", err)
	}
	if _, err := s.AddComment(99, "", "alice"); !errors.Is(err, ErrEmptyCommentPayload) {
		t.Fatalf("err = %v, want empty-payload error", err)
	}
	var ne *SectionNotEditableError
	if _, err := s.SetSection(99, domain.SectionJournal, "x", "alice"); !errors.As(err, &ne) {
		t.Fatalf("err = %v, want not-editable error", err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "T-*.md"))
	if err != nil || len(files) != 0 {
		t.Fatalf("tree touched: %v (%v)", files, err)
	}
	assertNoTmpFiles(t, dir)
}

// TestSetSection_NotEditableMessage: the refusal lists exactly the editable
// names from the owner (EditableSections), never the read-path lists.
func TestSetSection_NotEditableMessage(t *testing.T) {
	s, dir := seededStore(t)
	before := mustRead(t, dir, 1)
	_, err := s.SetSection(1, domain.SectionJournal, "x", "alice")
	var ne *SectionNotEditableError
	if !errors.As(err, &ne) {
		t.Fatalf("err = %v, want SectionNotEditableError", err)
	}
	if !slices.Equal(ne.Accepted, domain.EditableSections()) {
		t.Fatalf("Accepted = %v, want EditableSections()", ne.Accepted)
	}
	for _, a := range domain.EditableSections() {
		if !strings.Contains(err.Error(), string(a)) {
			t.Fatalf("refusal omits editable name %q: %v", a, err)
		}
	}
	assertWriteRefused(t, dir, 1, before, err)
}

func TestSetSection_Replaces(t *testing.T) {
	cases := []struct {
		sec     domain.Section
		payload string
		comment string
	}{
		{domain.SectionSummary, "new brief", "section summary: заменено"},
		{domain.SectionDetails, "new details", "section details: заменено"},
		{domain.SectionComments, "new comments", "section comments: заменено"},
	}
	for _, tc := range cases {
		t.Run(string(tc.sec), func(t *testing.T) {
			s, dir := seededStore(t)
			if _, err := s.AddComment(1, "old note", "seed"); err != nil {
				t.Fatalf("seed: %v", err)
			}
			if _, err := s.SetSection(1, tc.sec, tc.payload, "alice"); err != nil {
				t.Fatalf("SetSection: %v", err)
			}
			body := mustRead(t, dir, 1)
			tk, _, err := domain.Parse(body)
			if err != nil {
				t.Fatal(err)
			}
			if got := tk.SectionBody(tc.sec); got != tc.payload {
				t.Fatalf("SectionBody(%s) = %q, want %q", tc.sec, got, tc.payload)
			}
			if (tc.sec != domain.SectionComments && tk.Comments != "old note") ||
				(tc.sec != domain.SectionDetails && tk.Details != "details for ticket 0") ||
				(tc.sec == domain.SectionComments && strings.Contains(string(body), "old note")) {
				t.Fatalf("neighbour changed or old body survived:\n%s", body)
			}
			tr := journalTransitions(string(body))
			if len(tr) != 2 || !strings.Contains(tr[1], tc.comment+" (alice)") {
				t.Fatalf("journal = %v, want last event %q", tr, tc.comment)
			}
		})
	}
}

func TestWriteText_EmptyPayloadClears(t *testing.T) {
	s, dir := seededStore(t)
	if _, err := s.SetSection(1, domain.SectionSummary, "", "alice"); err != nil {
		t.Fatalf("SetSection empty: %v", err)
	}
	if body := string(mustRead(t, dir, 1)); !strings.Contains(body, "## Summary (Кратко)\n\nticket-0\n\n") {
		t.Fatalf("empty Brief must render the title:\n%s", body)
	}
	if _, err := s.SetSection(1, domain.SectionDetails, "\n", "alice"); err != nil {
		t.Fatalf("SetSection details: %v", err)
	}
	if _, err := s.SetSection(1, domain.SectionComments, "", "alice"); err != nil {
		t.Fatalf("SetSection comments: %v", err)
	}
	tk, _, err := domain.Parse(mustRead(t, dir, 1))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(tk.Brief, "\n") != "ticket-0" || tk.Details != "" || tk.Comments != "" {
		t.Fatalf("sections not cleared: brief=%v details=%q comments=%q", tk.Brief, tk.Details, tk.Comments)
	}
}

func TestAddComment_CRLFIdempotent(t *testing.T) {
	s, dir := seededStore(t)
	const payload = "line1\r\nline2\r\n"
	if _, err := s.AddComment(1, payload, "alice"); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	body := mustRead(t, dir, 1)
	if !bytes.Contains(body, []byte("line1\r\nline2\n\n")) || !strings.Contains(string(body), "comment: +2 строк (alice)") {
		t.Fatalf("interior CRLF lost, trailing CRLF kept, or line count wrong:\n%q", body)
	}
	if _, err := s.AddComment(1, payload, "alice"); err != nil {
		t.Fatalf("AddComment 2: %v", err)
	}
	body = mustRead(t, dir, 1)
	if !bytes.Contains(body, []byte("line2\n\nline1\r\nline2\n\n")) || bytes.Contains(body, []byte("\n\n\n")) {
		t.Fatalf("want one blank separator, no accumulation:\n%q", body)
	}
}

// TestWriteText_TrailingCRLFTrimmed pins that a CRLF-terminated payload
// loses its trailing CR as well as its LF, so no stray CR ends the section
// body, while an interior CRLF stays verbatim.
func TestWriteText_TrailingCRLFTrimmed(t *testing.T) {
	s, dir := seededStore(t)
	if _, err := s.SetSection(1, domain.SectionDetails, "alpha\r\nbeta\r\n", "alice"); err != nil {
		t.Fatalf("SetSection: %v", err)
	}
	body := mustRead(t, dir, 1)
	if !bytes.Contains(body, []byte("alpha\r\nbeta\n")) || bytes.Contains(body, []byte("beta\r")) {
		t.Fatalf("trailing CR kept or interior CRLF lost:\n%q", body)
	}
}

// preservedSrc: free-suffix header templates, populated neighbours, manual
// unknown tail — the DS8 preservation fixture.
