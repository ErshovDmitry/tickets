package domain

// T-0241 A1 tests: the section-selection model (sections.go). The payload is
// parser output, so every case goes through Parse and never through a
// hand-built Ticket unless the case is about SectionBody alone.

import (
	"strings"
	"testing"
)

// sectionsTicketSrc builds a fully formed ticket whose Summary and Details
// headers are given by the caller (to vary the recognized spelling) and whose
// remaining sections are the canonical RU dictionary forms.
func sectionsTicketSrc(summaryHeader, detailsHeader string) string {
	return "# T-0001 · BUG: x\n\n" +
		"- Status (Статус): open\n- Priority (Приоритет): normal\n" +
		"- Created (Создан): 2026-01-01 10:00 · by (кем): я\n- Project (Проект): p\n\n" +
		summaryHeader + "\n\nx\n\n" +
		detailsHeader + "\n\nd\n\n" +
		"## User comments (Комментарии от пользователя)\n\nu\n\n" +
		"## Comments (Комментарии)\n\nc\n\n" +
		"## Journal (Журнал)\n\n- 2026-01-01 10:00 — тикет создан (я).\n"
}

// parseSectionsSrc parses a source string or fails the test.
func parseSectionsSrc(t *testing.T, src string) Ticket {
	t.Helper()
	tk, _, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return *tk
}

// TestSectionBodyHeaderSpellings pins the language independence of the
// payload: a RU dictionary header, a plain EN header and an EN header with a
// free (non-dictionary) suffix select the same body, because recognition
// already happened in Parse.
func TestSectionBodyHeaderSpellings(t *testing.T) {
	cases := []struct {
		name          string
		summaryHeader string
		detailsHeader string
	}{
		{"ru dictionary forms", "## Summary (Кратко)", "## Details (Подробности)"},
		{"plain en forms", "## Summary", "## Details"},
		{"free suffix", "## Summary (Итог)", "## Details (Итог)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tk := parseSectionsSrc(t, sectionsTicketSrc(tc.summaryHeader, tc.detailsHeader))
			if got := tk.SectionBody(SectionSummary); got != "x" {
				t.Errorf("Summary body = %q, want %q", got, "x")
			}
			if got := tk.SectionBody(SectionDetails); got != "d" {
				t.Errorf("Details body = %q, want %q", got, "d")
			}
		})
	}
}

// TestSectionBodyPayloadIsBodyOnly pins that the "##" header line never
// appears in the payload and that a stray "## ..." line inside Details is
// body text, not a section boundary.
func TestSectionBodyPayloadIsBodyOnly(t *testing.T) {
	src := "# T-0001 · BUG: x\n\n- Status (Статус): open\n- Priority (Приоритет): normal\n" +
		"- Created (Создан): 2026-01-01 10:00 · by (кем): я\n- Project (Проект): p\n\n" +
		"## Summary (Кратко)\n\nx\n\n## Details (Подробности)\n\nd\n## Что-то своё\nещё\n\n" +
		"## User comments (Комментарии от пользователя)\n\nu\n\n## Comments (Комментарии)\n\nc\n\n" +
		"## Journal (Журнал)\n\n- 2026-01-01 10:00 — тикет создан (я).\n"
	tk := parseSectionsSrc(t, src)
	got := tk.SectionBody(SectionDetails)
	if !strings.Contains(got, "## Что-то своё") {
		t.Errorf("stray header must stay in the Details body, got %q", got)
	}
	for _, body := range []string{got, tk.SectionBody(SectionComments)} {
		if strings.HasPrefix(body, "## ") {
			t.Errorf("payload must not start with a header line, got %q", body)
		}
	}
}

// TestSectionBodyDuplicateSummary pins that a second "## Summary" header
// inside the Summary section stays in the body instead of opening a section.
func TestSectionBodyDuplicateSummary(t *testing.T) {
	src := "# T-0001 · BUG: x\n\n- Status (Статус): open\n- Priority (Приоритет): normal\n" +
		"- Created (Создан): 2026-01-01 10:00 · by (кем): я\n- Project (Проект): p\n\n" +
		"## Summary (Кратко)\n\nfirst\n## Summary\nsecond\n\n" +
		"## Details (Подробности)\n\nd\n\n## Journal (Журнал)\n\n- 2026-01-01 10:00 — тикет создан (я).\n"
	tk := parseSectionsSrc(t, src)
	got := tk.SectionBody(SectionSummary)
	if !strings.Contains(got, "## Summary") || !strings.Contains(got, "second") {
		t.Errorf("duplicate Summary header must stay in the body, got %q", got)
	}
}

// TestSectionBodyMissingAndEmpty pins the empty-payload cases: a section the
// legacy file does not carry, a section with no text, and a dictionary stub
// line (Parse drops it) all yield "".
func TestSectionBodyMissingAndEmpty(t *testing.T) {
	src := "# T-0001 · BUG: x\n\n- Status (Статус): open\n- Priority (Приоритет): normal\n" +
		"- Created (Создан): 2026-01-01 10:00 · by (кем): я\n- Project (Проект): p\n\n" +
		"## Summary (Кратко)\n\nx\n\n## Details (Подробности)\n\n" + dictRU.detailsStub + "\n\n" +
		"## Comments (Комментарии)\n\n## Journal (Журнал)\n\n- 2026-01-01 10:00 — тикет создан (я).\n"
	tk := parseSectionsSrc(t, src)
	for _, sec := range []Section{SectionUserComments, SectionDetails, SectionComments} {
		if got := tk.SectionBody(sec); got != "" {
			t.Errorf("%s body = %q, want empty", sec, got)
		}
	}
}

// TestSectionBodyUnknownSection pins the zero value contract: an unknown
// Section yields "", never a panic.
func TestSectionBodyUnknownSection(t *testing.T) {
	tk := parseSectionsSrc(t, sectionsTicketSrc("## Summary", "## Details"))
	if got := tk.SectionBody(Section("Nowhere")); got != "" {
		t.Errorf("unknown section body = %q, want empty", got)
	}
}

// TestSectionBodyJournal pins that the journal payload is the entry lines
// with the separator blanks dropped, and that in-memory entries without Raw
// contribute nothing.
func TestSectionBodyJournal(t *testing.T) {
	tk := parseSectionsSrc(t, sectionsTicketSrc("## Summary", "## Details"))
	want := "- 2026-01-01 10:00 — тикет создан (я)."
	if got := tk.SectionBody(SectionJournal); got != want {
		t.Errorf("journal body = %q, want %q", got, want)
	}
	tk.Journal = append(tk.Journal, JournalEntry{From: StatusOpen, To: StatusWip, Who: "я"})
	if got := tk.SectionBody(SectionJournal); got != want {
		t.Errorf("journal body with a Raw-less entry = %q, want %q", got, want)
	}
}

// TestCanonicalSectionNames pins the exported token list: five canonically
// spelled tokens in sectionNames order, no aliases.
func TestCanonicalSectionNames(t *testing.T) {
	got := CanonicalSectionNames()
	want := []string{"summary", "details", "user-comments", "comments", "journal"}
	if len(got) != len(want) {
		t.Fatalf("CanonicalSectionNames() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("CanonicalSectionNames() = %v, want %v", got, want)
		}
	}
}

// TestParseSectionName pins the accepted spellings (canonical tokens, the two
// aliases, case-insensitivity) and the refusals (inner spaces, garbage,
// empty).
func TestParseSectionName(t *testing.T) {
	cases := []struct {
		in     string
		want   Section
		wantOK bool
	}{
		{"summary", SectionSummary, true},
		{"details", SectionDetails, true},
		{"user-comments", SectionUserComments, true},
		{"comments", SectionComments, true},
		{"journal", SectionJournal, true},
		{"brief", SectionSummary, true},
		{"user_comments", SectionUserComments, true},
		{"SUMMARY", SectionSummary, true},
		{"USER-COMMENTS", SectionUserComments, true},
		{"BRIEF", SectionSummary, true},
		{"  details  ", SectionDetails, true},
		{"user comments", "", false},
		{"User Comments", "", false},
		{"Summary", SectionSummary, true},
		{"nope", "", false},
		{"", "", false},
		{"user-comments-extra", "", false},
	}
	for _, tc := range cases {
		got, ok := ParseSectionName(tc.in)
		if ok != tc.wantOK || got != tc.want {
			t.Errorf("ParseSectionName(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.wantOK)
		}
	}
}

// TestEditableSections pins the write-path allowlist (T-0240 DS2): exactly
// the summary/details/comments constants in sectionNames order, and the
// returned slice is a copy — a caller cannot mutate the owner list.
func TestEditableSections(t *testing.T) {
	want := []Section{SectionSummary, SectionDetails, SectionComments}
	got := EditableSections()
	if len(got) != len(want) {
		t.Fatalf("EditableSections() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("EditableSections()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	got[0] = SectionJournal
	if again := EditableSections(); again[0] != SectionSummary {
		t.Fatalf("EditableSections() aliasing: owner list mutated to %v", again)
	}
}

// TestIsEditableSection pins the protected/unknown refusal set: user
// comments and journal are never editable, nor is any unknown identifier.
func TestIsEditableSection(t *testing.T) {
	cases := []struct {
		sec  Section
		want bool
	}{
		{SectionSummary, true},
		{SectionDetails, true},
		{SectionComments, true},
		{SectionUserComments, false},
		{SectionJournal, false},
		{Section("bogus"), false},
		{Section(""), false},
		{Section("summary"), false}, // raw lower case is not a canonical identifier
	}
	for _, tc := range cases {
		if got := IsEditableSection(tc.sec); got != tc.want {
			t.Errorf("IsEditableSection(%q) = %v, want %v", tc.sec, got, tc.want)
		}
	}
}
