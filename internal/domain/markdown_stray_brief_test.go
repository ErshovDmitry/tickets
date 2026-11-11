package domain

// Brief-content tests for T-0166: the "## Summary" body is captured into
// Ticket.Brief verbatim (previously discarded as a Title duplicate) and
// emitted between the Summary header and the Details header.

import (
	"bytes"
	"strings"
	"testing"
)

const straySummaryBody = "Реализовать Go-версию ticket по AGENTS_ARCHITECTURE.md\n\n## Details (Подробности)"

// TestParseStrayBriefContentPreserved pins T-0166: two or more Summary body
// lines are captured into Brief in order and emitted between the Summary
// header and the Details header; the fixture is byte-stable across the
// parse→render cycle.
func TestParseStrayBriefContentPreserved(t *testing.T) {
	src := strings.Replace(ticketT0001, straySummaryBody,
		"первая строка\nвторая строка\n\n## Details (Подробности)", 1)
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := strings.Join(tk.Brief, "\n"); got != "первая строка\nвторая строка" {
		t.Errorf("Brief = %q, want %q", got, "первая строка\nвторая строка")
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(got), "## Summary (Кратко)\n\nпервая строка\nвторая строка\n\n## Details (Подробности)") {
		t.Errorf("Brief not anchored between Summary and Details:\n%s", got)
	}
	if want := legacyRoundTripWant(src); !bytes.Equal(got, []byte(want)) {
		t.Errorf("round trip mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestParseStrayBriefUnknownSection pins T-0166: an unrecognized "## "
// header between Summary and Details is captured into Brief verbatim —
// only a recognized section header ends the brief zone.
func TestParseStrayBriefUnknownSection(t *testing.T) {
	src := strings.Replace(ticketT0001, straySummaryBody,
		"первая строка\n## Notes\nnotes body\n\n## Details (Подробности)", 1)
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := "первая строка\n## Notes\nnotes body"
	if got := strings.Join(tk.Brief, "\n"); got != want {
		t.Errorf("Brief = %q, want %q", got, want)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if want := legacyRoundTripWant(src); !bytes.Equal(got, []byte(want)) {
		t.Errorf("round trip mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestParseStrayLeadingBlanksBeyondRenderBlankPreserved pins the
// one-leading-blank rule for Brief (T-0035 semantics): trimLeadingOneEmpty
// removes exactly the MD022 blank Render emits; any further leading blank
// line is content. An empty Summary body parses to nil Brief and Render
// falls back to Title.
func TestParseStrayLeadingBlanksBeyondRenderBlankPreserved(t *testing.T) {
	src := strings.Replace(ticketT0001,
		"## Summary (Кратко)\n\nРеализовать Go-версию ticket по AGENTS_ARCHITECTURE.md\n",
		"## Summary (Кратко)\n\n\n\ntext\n", 1)
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []string{"", "", "text"}
	if got := strings.Join(tk.Brief, "\n"); got != strings.Join(want, "\n") {
		t.Errorf("Brief = %q, want %q (only one leading blank trimmed)", got, want)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if want := legacyRoundTripWant(src); !bytes.Equal(got, []byte(want)) {
		t.Errorf("round trip mismatch:\n got %q\nwant %q", got, want)
	}
	// Empty Summary body: all-blank Brief trims to nil and the render is
	// the canonical Title-fallback file.
	tk2, unknown2, err := Parse([]byte(strings.Replace(ticketT0001, straySummaryBody,
		"\n\n## Details (Подробности)", 1)))
	if err != nil {
		t.Fatalf("Parse empty brief: %v", err)
	}
	if tk2.Brief != nil {
		t.Errorf("empty Summary body: Brief = %q, want nil", tk2.Brief)
	}
	got2, err := Render(tk2, unknown2)
	if err != nil {
		t.Fatalf("Render empty brief: %v", err)
	}
	if want := legacyRoundTripWant(ticketT0001); !bytes.Equal(got2, []byte(want)) {
		t.Errorf("nil Brief must fall back to Title:\n got %q\nwant %q", got2, want)
	}
}

// TestParseStrayDuplicateSummaryHeader pins D3: a repeated "## Summary"
// header before the journal stays brief verbatim — the raw template keeps
// the FIRST header, the duplicate line is Brief content.
func TestParseStrayDuplicateSummaryHeader(t *testing.T) {
	src := strings.Replace(ticketT0001, straySummaryBody,
		"Реализовать Go-версию ticket по AGENTS_ARCHITECTURE.md\n\n## Summary (any lang)\n\ndup body\n\n## Details (Подробности)", 1)
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := tk.RawTemplates.Headers[secNameSummary]; got != "## Summary (Кратко)" {
		t.Errorf("Headers[Summary] = %q, want the FIRST header kept (the duplicate is a different recognized line, so a last-wins bug would fail)", got)
	}
	want := "Реализовать Go-версию ticket по AGENTS_ARCHITECTURE.md\n\n## Summary (any lang)\n\ndup body"
	if got := strings.Join(tk.Brief, "\n"); got != want {
		t.Errorf("Brief = %q, want %q", got, want)
	}
	data, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	tk2, unknown2, err := Parse(data)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	got2, err := Render(tk2, unknown2)
	if err != nil {
		t.Fatalf("second Render: %v", err)
	}
	if !bytes.Equal(got2, data) {
		t.Errorf("double round trip mismatch:\n got %q\nwant %q", got2, data)
	}
}

// TestRenderStrayBriefNilFallsBackToTitle pins the Render fallback: a nil
// Brief renders the Title line (the pre-T-0166 output), and a nil
// HeadStray inserts no stray block — both keep the canonical file
// byte-identical.
func TestRenderStrayBriefNilFallsBackToTitle(t *testing.T) {
	tk, unknown, err := Parse([]byte(ticketT0001))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := strings.Join(tk.Brief, "\n"); got != "Реализовать Go-версию ticket по AGENTS_ARCHITECTURE.md" {
		t.Errorf("canonical Brief = %q, want the Summary body line", got)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if want := legacyRoundTripWant(ticketT0001); !bytes.Equal(got, []byte(want)) {
		t.Errorf("Brief=[title] render differs from the canonical fixture:\n got %q\nwant %q", got, want)
	}
	tk.Brief = nil
	got, err = Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render nil Brief: %v", err)
	}
	if want := legacyRoundTripWant(ticketT0001); !bytes.Equal(got, []byte(want)) {
		t.Errorf("nil Brief must fall back to Title:\n got %q\nwant %q", got, want)
	}
}

// TestParseStrayJournalResumeAnchored pins D4: a journal-resumed "##
// Summary" header captures its following text into Brief, and Render emits
// it in the canonical Summary position — between the Summary header and
// the Details header — byte-stable across a double round trip.
func TestParseStrayJournalResumeAnchored(t *testing.T) {
	src := ticketT0001 + "\n## Summary (any lang)\n\nMixed section order\n"
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := tk.RawTemplates.Headers[secNameSummary]; got != "## Summary (any lang)" {
		t.Errorf("Headers[Summary] = %q, want the resumed raw header", got)
	}
	if got := tk.Brief; len(got) == 0 || got[len(got)-1] != "Mixed section order" {
		t.Errorf("Brief = %q, want the resume text %q captured (D4)", got, "Mixed section order")
	}
	if want := "Реализовать Go-версию ticket по AGENTS_ARCHITECTURE.md"; tk.Title != want {
		t.Errorf("Title = %q, want unchanged %q", tk.Title, want)
	}
	data, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// The resumed text lands after the original Summary body verbatim;
	// the double blank between them is the original zone's pre-Details
	// MD022 blank, kept as content (T-0035) — lossless and byte-stable.
	if !strings.Contains(string(data), "## Summary (any lang)\n\nРеализовать Go-версию ticket по AGENTS_ARCHITECTURE.md\n\n\nMixed section order\n\n## Details (Подробности)") {
		t.Errorf("resumed brief not anchored between Summary and Details:\n%s", data)
	}
	tk2, unknown2, err := Parse(data)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	got2, err := Render(tk2, unknown2)
	if err != nil {
		t.Fatalf("second Render: %v", err)
	}
	if !bytes.Equal(got2, data) {
		t.Errorf("double round trip mismatch:\n got %q\nwant %q", got2, data)
	}
}
