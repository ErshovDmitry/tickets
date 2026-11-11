package domain

// Stray-content tests for T-0166: unrecognized lines in the ticket head are
// captured into Ticket.HeadStray instead of being silently dropped, and
// every "# "-prefixed line outside the verbatim zones is dropped (D2) so
// Render never gains a new H1 source.

import (
	"bytes"
	"strings"
	"testing"
)

// TestParseStrayHeadPreserved pins D1: stray lines before the H1 (preamble)
// and between the H1 and the metadata block are captured into HeadStray in
// their original order, and Render anchors them after the metadata block.
func TestParseStrayHeadPreserved(t *testing.T) {
	src := "stray before h1\n" + strings.Replace(ticketT0001,
		"# T-0001 · ENH: Реализовать Go-версию ticket по AGENTS_ARCHITECTURE.md\n",
		"# T-0001 · ENH: Реализовать Go-версию ticket по AGENTS_ARCHITECTURE.md\nstray after h1\n", 1)
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []string{"stray before h1", "stray after h1"}
	if got := strings.Join(tk.HeadStray, "\n"); got != strings.Join(want, "\n") {
		t.Errorf("HeadStray = %q, want %q", got, want)
	}
	if tk.Number != 1 || tk.Status != StatusOpen || tk.Project != "tickets" {
		t.Errorf("meta parsed wrong: number=%d status=%q project=%q", tk.Number, tk.Status, tk.Project)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := string(got)
	metaAt := strings.Index(out, "- Project (Проект): tickets")
	firstAt := strings.Index(out, "stray before h1")
	secondAt := strings.Index(out, "stray after h1")
	summaryAt := strings.Index(out, "## Summary (Кратко)")
	if !(metaAt >= 0 && metaAt < firstAt && firstAt < secondAt && secondAt < summaryAt) {
		t.Errorf("strays not anchored between meta block and Summary header:\n%s", out)
	}
	// Preamble H1-like line is dropped (D2), never captured.
	tk2, _, err := Parse([]byte("# Notes\n" + ticketT0001))
	if err != nil {
		t.Fatalf("Parse preamble: %v", err)
	}
	if len(tk2.HeadStray) != 0 {
		t.Errorf("preamble '# Notes' captured into HeadStray = %q, want dropped (D2)", tk2.HeadStray)
	}
}

// TestParseStrayHeadUnknownHeaderKeepsBody pins D1: an unrecognized "## "
// header in head starts the stray zone — the header and its body stay
// together in HeadStray, and a meta-looking line after the header is NOT
// captured.
func TestParseStrayHeadUnknownHeaderKeepsBody(t *testing.T) {
	src := strings.Replace(ticketT0001,
		"- Project (Проект): tickets\n\n## Summary (Кратко)",
		"- Project (Проект): tickets\n\n## Unknown head\nunknown head body\n- Status: forged\n\n## Summary (Кратко)", 1)
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := "## Unknown head\nunknown head body\n- Status: forged"
	if got := strings.Join(tk.HeadStray, "\n"); got != want {
		t.Errorf("HeadStray = %q, want %q", got, want)
	}
	if tk.Status != StatusOpen {
		t.Errorf("Status = %q, want %q (meta after unknown header must not be captured)", tk.Status, StatusOpen)
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

// TestParseStrayH1LikeLineDropped pins D2: ANY "# "-prefixed line in the
// head/Summary/brief zones is dropped — Render must not gain a new H1
// source. Subcases (a)/(f) share the secHead drop branch.
func TestParseStrayH1LikeLineDropped(t *testing.T) {
	const title = "Реализовать Go-версию ticket по AGENTS_ARCHITECTURE.md"
	cases := []struct {
		name     string
		src      string
		headWant string // expected HeadStray content ("" = none)
	}{
		{"a: '# Notes' in head", strings.Replace(ticketT0001,
			"- Project (Проект): tickets\n\n## Summary (Кратко)",
			"- Project (Проект): tickets\n# Notes\n\n## Summary (Кратко)", 1), ""},
		{"b: '# Notes' after unknown header in head", strings.Replace(ticketT0001,
			"- Project (Проект): tickets\n\n## Summary (Кратко)",
			"- Project (Проект): tickets\n\n## Orphan\norphan body\n# Notes\n\n## Summary (Кратко)", 1),
			"## Orphan\norphan body"},
		{"c: '# Notes' inside Summary", strings.Replace(ticketT0001,
			title+"\n\n## Details (Подробности)",
			title+"\n# Notes\n\n## Details (Подробности)", 1), ""},
		{"d: valid H1 inside Summary", strings.Replace(ticketT0001,
			title+"\n\n## Details (Подробности)",
			title+"\n# T-0002 · BUG: y\n\n## Details (Подробности)", 1), ""},
		{"e: valid H1 after unknown header in head", strings.Replace(ticketT0001,
			"- Project (Проект): tickets\n\n## Summary (Кратко)",
			"- Project (Проект): tickets\n\n## Orphan\n# T-0002 · BUG: y\n\n## Summary (Кратко)", 1),
			"## Orphan"},
		{"f: '# Notes' in preamble", "# Notes\n" + ticketT0001, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tk, unknown, err := Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := strings.Join(tk.HeadStray, "\n"); got != tc.headWant {
				t.Errorf("HeadStray = %q, want %q", got, tc.headWant)
			}
			if strings.Contains(strings.Join(tk.Brief, "\n"), "# ") {
				t.Errorf("Brief contains a '# ' line: %q", tk.Brief)
			}
			if tk.Number != 1 {
				t.Errorf("Number = %d, want 1 (dropped H1 must not re-parse)", tk.Number)
			}
			got, err := Render(tk, unknown)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if strings.Contains(string(got), "# Notes") || strings.Contains(string(got), "# T-0002") {
				t.Errorf("dropped '# ' line re-emitted:\n%s", got)
			}
			tk2, unknown2, err := Parse(got)
			if err != nil {
				t.Fatalf("reparse: %v", err)
			}
			got2, err := Render(tk2, unknown2)
			if err != nil {
				t.Fatalf("second Render: %v", err)
			}
			if !bytes.Equal(got2, got) {
				t.Errorf("double round trip mismatch:\n got %q\nwant %q", got2, got)
			}
			// Canonical-render check: with every stray dropped the render
			// is byte-identical to the canonical fixture (the legacy
			// user-comments stub is dropped by Render, Option A).
			if tc.headWant == "" {
				if want := legacyRoundTripWant(ticketT0001); !bytes.Equal(got, []byte(want)) {
					t.Errorf("render differs from the canonical fixture:\n got %q\nwant %q", got, want)
				}
			}
		})
	}
}

// TestParseStrayBlankAfterMetaDroppedWithPriorStrays pins the C2-1 fix: a
// blank right after the metadata block is a structural position and is
// dropped ALWAYS — even when strays were captured earlier — so Render
// never emits consecutive blank lines (MD012). The blank between two
// strays stays verbatim content, and the output is byte-stable across a
// second parse→render cycle.
func TestParseStrayBlankAfterMetaDroppedWithPriorStrays(t *testing.T) {
	h1 := "# T-0001 · ENH: Реализовать Go-версию ticket по AGENTS_ARCHITECTURE.md"
	cases := []struct {
		name string
		src  string
	}{
		{"stray after H1 and after meta", strings.Replace(strings.Replace(ticketT0001,
			h1+"\n\n- Status (Статус): open",
			h1+"\nstray X\n\n- Status (Статус): open", 1),
			"- Project (Проект): tickets\n\n## Summary (Кратко)",
			"- Project (Проект): tickets\n\nstray Y\n\n## Summary (Кратко)", 1)},
		{"preamble stray and stray after meta", "stray before h1\n" + strings.Replace(ticketT0001,
			"- Project (Проект): tickets\n\n## Summary (Кратко)",
			"- Project (Проект): tickets\n\nstray Y\n\n## Summary (Кратко)", 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tk, unknown, err := Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			// The structural blank after the metadata block must not be
			// captured: strays are separated by exactly one content blank.
			want := []string{"stray X", "", "stray Y"}
			if tc.name == "preamble stray and stray after meta" {
				want = []string{"stray before h1", "stray Y"}
			}
			if got := strings.Join(tk.HeadStray, "\n"); got != strings.Join(want, "\n") {
				t.Fatalf("HeadStray = %q, want %q (structural blank after meta dropped)", got, want)
			}
			data, err := Render(tk, unknown)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if strings.Contains(string(data), "\n\n\n") {
				t.Errorf("render contains consecutive blank lines:\n%s", data)
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
		})
	}
}
