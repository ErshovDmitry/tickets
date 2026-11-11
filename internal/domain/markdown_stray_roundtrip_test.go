package domain

// Round-trip pins for T-0166 plan S3 (C3-1): whitespace-only stray lines
// stay verbatim content, repeated parse→render cycles are byte-stable with
// head and Summary strays combined, and unknown "## " headers inside the
// known verbatim sections (Details/UserComments/Comments) survive e2e.

import (
	"bytes"
	"strings"
	"testing"
)

// TestParseStrayWhitespaceOnly pins T-0035 for the stray zones: a
// whitespace-only line ("   ") is content — it is captured into HeadStray
// and Brief verbatim and never collapsed into a structural blank (the
// structural-blank rule matches only the strictly empty line, so replacing
// line == "" with a TrimSpace check would fail this test).
func TestParseStrayWhitespaceOnly(t *testing.T) {
	t.Run("head zone", func(t *testing.T) {
		src := strings.Replace(ticketT0001,
			"- Project (Проект): tickets\n\n## Summary (Кратко)",
			"- Project (Проект): tickets\n\n   \n## Orphan head\n   \norphan body\n\n## Summary (Кратко)", 1)
		tk, unknown, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		want := "   \n## Orphan head\n   \norphan body"
		if got := strings.Join(tk.HeadStray, "\n"); got != want {
			t.Fatalf("HeadStray = %q, want %q (whitespace-only lines verbatim)", got, want)
		}
		data, err := Render(tk, unknown)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if want := legacyRoundTripWant(src); !bytes.Equal(data, []byte(want)) {
			t.Errorf("round trip mismatch:\n got %q\nwant %q", data, want)
		}
	})
	t.Run("summary body", func(t *testing.T) {
		src := strings.Replace(ticketT0001, straySummaryBody,
			"Реализовать Go-версию ticket по AGENTS_ARCHITECTURE.md\n   \n\n## Details (Подробности)", 1)
		tk, unknown, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		want := "Реализовать Go-версию ticket по AGENTS_ARCHITECTURE.md\n   "
		if got := strings.Join(tk.Brief, "\n"); got != want {
			t.Fatalf("Brief = %q, want %q (whitespace-only line verbatim)", got, want)
		}
		data, err := Render(tk, unknown)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if want := legacyRoundTripWant(src); !bytes.Equal(data, []byte(want)) {
			t.Errorf("round trip mismatch:\n got %q\nwant %q", data, want)
		}
	})
}

// TestParseStrayDoubleRoundTripByteStable pins byte-stability across three
// full parse→render cycles (sample: TestUserCommentsSurviveRepeatedRoundTrips)
// with head strays (an unknown "## " header plus body) and a Summary stray
// present at the same time.
func TestParseStrayDoubleRoundTripByteStable(t *testing.T) {
	const title = "Реализовать Go-версию ticket по AGENTS_ARCHITECTURE.md"
	src := strings.Replace(ticketT0001,
		"- Project (Проект): tickets\n\n## Summary (Кратко)\n\n"+title+"\n\n## Details (Подробности)",
		"- Project (Проект): tickets\n\n## Unknown head\nunknown head body\n\n"+
			"## Summary (Кратко)\n\n"+title+"\nsummary stray line\n\n## Details (Подробности)", 1)
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	data, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for i := range 3 {
		tk, unknown, err := Parse(data)
		if err != nil {
			t.Fatalf("cycle %d Parse: %v", i, err)
		}
		if got := strings.Join(tk.HeadStray, "\n"); got != "## Unknown head\nunknown head body" {
			t.Fatalf("cycle %d HeadStray = %q", i, got)
		}
		if got := strings.Join(tk.Brief, "\n"); got != title+"\nsummary stray line" {
			t.Fatalf("cycle %d Brief = %q", i, got)
		}
		got, err := Render(tk, unknown)
		if err != nil {
			t.Fatalf("cycle %d Render: %v", i, err)
		}
		if !bytes.Equal(got, data) {
			t.Fatalf("cycle %d round trip mismatch:\n got %q\nwant %q", i, got, data)
		}
		data = got
	}
}

// TestParseStrayUnknownHeaderInKnownSections pins the verbatim policy e2e:
// an unknown "## " header inside Details, UserComments or Comments stays
// section text (never a transition), and the file is byte-stable across
// the parse→render cycle. Unit-level absorbLine coverage lives in
// TestAbsorbLineDetailsGatedTransitions; this is the end-to-end pin.
func TestParseStrayUnknownHeaderInKnownSections(t *testing.T) {
	t.Run("details", func(t *testing.T) {
		src := strings.Replace(ticketT0001,
			"Гейт: план в wiki -> план-ревью -> код.",
			"Гейт: план в wiki -> план-ревью -> код.\n## Orphan details\norphan details body", 1)
		tk, unknown, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if !strings.Contains(tk.Details, "## Orphan details\norphan details body") {
			t.Fatalf("Details lost the stray header:\n%s", tk.Details)
		}
		data, err := Render(tk, unknown)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if want := legacyRoundTripWant(src); !bytes.Equal(data, []byte(want)) {
			t.Errorf("round trip mismatch:\n got %q\nwant %q", data, want)
		}
	})
	t.Run("user comments and comments", func(t *testing.T) {
		src := ucTicket("замечание раз\n## Orphan UC\norphan uc body",
			"заметка агента\n## Orphan C\norphan c body")
		tk, unknown, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if want := "замечание раз\n## Orphan UC\norphan uc body"; tk.UserComments != want {
			t.Fatalf("UserComments = %q, want %q", tk.UserComments, want)
		}
		if want := "заметка агента\n## Orphan C\norphan c body"; tk.Comments != want {
			t.Fatalf("Comments = %q, want %q", tk.Comments, want)
		}
		data, err := Render(tk, unknown)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if !bytes.Equal(data, []byte(src)) {
			t.Errorf("round trip mismatch:\n got %q\nwant %q", data, src)
		}
	})
}
