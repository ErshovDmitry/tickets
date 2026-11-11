package domain

// Structural-blank and repeated-H1 tests for the ticket head zone
// (T-0166 CF-1/CF-2): only the FIRST strictly-empty blank at a structural
// position (right after the H1, right after the metadata block) is
// dropped — every further blank is verbatim content — and only the FIRST
// valid H1 is accepted, a repeated one is dropped (D2).

import (
	"bytes"
	"strings"
	"testing"
)

// TestParseStrayBlankSeriesAfterMetaOnlyFirstDropped pins the narrowed
// structural-blank rule (CF-1): only the FIRST strictly-empty blank at a
// structural position (right after the H1, right after the metadata
// block) is dropped; a blank BEFORE the H1 / metadata block, and the
// whole blank run right after a dropped "# " line, are markup and dropped
// too (rule ii, CFF-1); only a blank INSIDE a stray series is verbatim content
// (rule iii) — a trailing run at the series end is trimmed by finish as
// Render's separator (rule iv). Rules apply in order
// (i)→(ii)→(iii)→(iv): (iii) matches only blanks INSIDE an already-open
// stray series, so it never overrides a pending structural blank (i);
// (ii) matches only blanks that would OPEN the series (before the H1 /
// metadata block, right after a dropped "# " line), so inside an open
// series (iii) keeps blanks verbatim even there. A "\\n\\n\\n" series
// after the metadata block therefore keeps its surviving blanks as
// content, and the output
// is byte-stable across a second parse-render cycle. A whitespace-only
// line is never structural: it stays content even right after the H1.
func TestParseStrayBlankSeriesAfterMetaOnlyFirstDropped(t *testing.T) {
	h1 := "# T-0001 · ENH: Реализовать Go-версию ticket по AGENTS_ARCHITECTURE.md"
	cases := []struct {
		name string
		src  string
		want string // expected HeadStray content
	}{
		{"three blanks after meta", strings.Replace(ticketT0001,
			"- Project (Проект): tickets\n\n## Summary (Кратко)",
			"- Project (Проект): tickets\n\n\n\nstray Y\n\n## Summary (Кратко)", 1), "\n\nstray Y"},
		// The second blank here sits BEFORE the metadata block is
		// captured: markup (rule ii), dropped at parse time, so the
		// stray series opens only at "stray M". The content after the
		// blanks pins the drop: with rule ii rolled back the leading
		// "" would survive into HeadStray and the want would fail.
		{"two blanks after H1", strings.Replace(ticketT0001,
			h1+"\n\n- Status (Статус): open",
			h1+"\n\n\nstray M\n- Status (Статус): open", 1), "stray M"},
		{"whitespace-only after H1 is content", strings.Replace(ticketT0001,
			h1+"\n\n- Status (Статус): open",
			h1+"\n   \n- Status (Статус): open", 1), "   "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tk, unknown, err := Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := strings.Join(tk.HeadStray, "\n"); got != tc.want {
				t.Fatalf("HeadStray = %q, want %q (only the first structural blank dropped)", got, tc.want)
			}
			data, err := Render(tk, unknown)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if !strings.Contains(string(data), "\n\n"+tc.want) {
				t.Errorf("surviving blanks not rendered after the meta block:\n%s", data)
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

// TestParseStrayBlankNeverOpensSeries pins CFF-1: a strictly-empty blank
// must NOT open a stray series while the head is incomplete (rule ii) —
// neither a blank BEFORE the H1 nor the blank run right AFTER a dropped
// "# " line. Both are markup and dropped; otherwise HeadStray would start with
// "" and Render would print two consecutive blank lines (MD012) the
// source never had. The rendered output must contain no "\n\n\n" and be
// byte-stable across a second parse-render cycle.
func TestParseStrayBlankNeverOpensSeries(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string // expected HeadStray content
	}{
		// Content after the blank pins the drop: with rule ii rolled
		// back the leading "" would survive into HeadStray.
		{"blank before H1", "\nstray P\n" + ticketT0001, "stray P"},
		// The blank between the metadata block and "# Notes" is the
		// structural one (rule i): it consumes metaBlankPending, so the
		// two blanks after the dropped "# Notes" reach dropHeadBlank
		// with NO pending position and are pinned ONLY by rule ii
		// (afterDroppedH1). With rule ii rolled back they survive as
		// verbatim content, the stray series opens with two leading ""
		// and the want below fails.
		{"blank after dropped hash line", strings.Replace(ticketT0001,
			"- Project (Проект): tickets\n\n## Summary (Кратко)",
			"- Project (Проект): tickets\n\n# Notes\n\n\nstray Z\n\n## Summary (Кратко)", 1), "stray Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tk, unknown, err := Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := strings.Join(tk.HeadStray, "\n"); got != tc.want {
				t.Fatalf("HeadStray = %q, want %q (a blank must not open the stray series)", got, tc.want)
			}
			data, err := Render(tk, unknown)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if strings.Contains(string(data), "\n\n\n") {
				t.Errorf("rendered output has two consecutive blank lines (MD012):\n%s", data)
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

// TestParseStraySecondH1InHeadDropped pins D2 for a repeated VALID H1
// (CF-2): only the FIRST valid H1 in the head zone sets
// Number/Type/Title; any later one is dropped like any other "# " line,
// never captured — so a file carrying two H1 lines still opens (Number
// matches the file name instead of being overwritten) and Render does not
// gain a second H1 source.
func TestParseStraySecondH1InHeadDropped(t *testing.T) {
	src := strings.Replace(ticketT0001,
		"- Project (Проект): tickets\n\n## Summary (Кратко)",
		"- Project (Проект): tickets\n# T-0002 · BUG: второй заголовок\n\n## Summary (Кратко)", 1)
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.Number != 1 || tk.Type != Type("ENH") || !strings.Contains(tk.Title, "AGENTS_ARCHITECTURE") {
		t.Errorf("second H1 overwrote the head: number=%d type=%q title=%q", tk.Number, tk.Type, tk.Title)
	}
	if got := strings.Join(tk.HeadStray, "\n"); strings.Contains(got, "T-0002") {
		t.Errorf("dropped second H1 captured into HeadStray = %q", got)
	}
	hk, err := ParseHeader([]byte(src))
	if err != nil {
		t.Fatalf("ParseHeader: %v", err)
	}
	if hk.Number != 1 || hk.Type != Type("ENH") {
		t.Errorf("ParseHeader: second H1 overwrote number=%d type=%q, want 1/ENH", hk.Number, hk.Type)
	}
	data, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(string(data), "T-0002") {
		t.Errorf("dropped second H1 re-emitted:\n%s", data)
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
