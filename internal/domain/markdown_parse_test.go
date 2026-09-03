package domain

// Tests for the T-0036 code-review fixes in markdown_parse.go: gated
// section transitions in Details, unrecognized-header handling in head/brief
// (pre-T-0036 semantics), and grammar-derived raw "Created" meta template.

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func newAbsorbTestTicket() *Ticket {
	return &Ticket{
		RawTemplates: rawTemplates{
			Headers: make(map[sectionName]string),
			Meta:    make(map[string]metaSegments),
		},
	}
}

// TestAbsorbLineDetailsGatedTransitions pins finding 1: inside Details only
// the next section's header (UserComments/Comments/Journal) ends the
// section; Summary/Details headers and unknown "## ..." lines stay details
// TEXT with no raw header storage.
func TestAbsorbLineDetailsGatedTransitions(t *testing.T) {
	cases := []struct {
		name    string
		line    string
		want    section
		wantRaw bool
	}{
		{"summary header stays text", "## Summary (Кратко)", secDetails, false},
		{"details header stays text", "## Details (Подробности)", secDetails, false},
		{"unknown header stays text", "## Что-то своё", secDetails, false},
		{"user comments header ends", "## User comments (Комментарии от пользователя)", secUserComments, true},
		{"comments header ends", "## Comments (Комментарии)", secComments, true},
		{"journal header ends", "## Journal (Журнал)", secJournal, true},
	}
	for _, tc := range cases {
		tk := newAbsorbTestTicket()
		var details, userComments, comments []string
		got := absorbLine(tk, &details, &userComments, &comments, secDetails, tc.line)
		if got != tc.want {
			t.Errorf("%s: section = %v, want %v", tc.name, got, tc.want)
		}
		if !tc.wantRaw && (len(details) != 1 || details[0] != tc.line) {
			t.Errorf("%s: details = %q, want the line kept as text", tc.name, details)
		}
		if tc.wantRaw && len(details) != 0 {
			t.Errorf("%s: details = %q, want empty (line was the transition)", tc.name, details)
		}
		if tc.wantRaw != (len(tk.RawTemplates.Headers) == 1) {
			t.Errorf("%s: raw headers stored = %d, wantRaw=%v", tc.name, len(tk.RawTemplates.Headers), tc.wantRaw)
		}
	}
}

// TestAbsorbLineHeadBriefUnknownHeader pins finding 2: an unrecognized
// "## ..." line leaves head/brief for brief with NO raw storage (old
// headerName semantics), so a meta line after it is not captured.
func TestAbsorbLineHeadBriefUnknownHeader(t *testing.T) {
	// Head: stray header moves to brief; a following meta line is ignored.
	tk := newAbsorbTestTicket()
	var details, userComments, comments []string
	sec := absorbLine(tk, &details, &userComments, &comments, secHead, "## Что-то своё")
	if sec != secBrief {
		t.Fatalf("head + unknown header: section = %v, want %v", sec, secBrief)
	}
	if len(tk.RawTemplates.Headers) != 0 {
		t.Errorf("head + unknown header: raw headers stored = %d, want 0", len(tk.RawTemplates.Headers))
	}
	sec = absorbLine(tk, &details, &userComments, &comments, sec, "- Project: late")
	if sec != secBrief {
		t.Fatalf("brief + meta line: section = %v, want %v", sec, secBrief)
	}
	if tk.Project == "late" {
		t.Error("meta line after unknown header was captured, want ignored (pre-T-0036)")
	}

	// Head: recognized header still transitions with raw storage.
	tk = newAbsorbTestTicket()
	sec = absorbLine(tk, &details, &userComments, &comments, secHead, "## Comments (Комментарии)")
	if sec != secComments || tk.RawTemplates.Headers[secNameComments] != "## Comments (Комментарии)" {
		t.Errorf("head + recognized header: sec=%v raw=%q", sec, tk.RawTemplates.Headers[secNameComments])
	}

	// Brief: unknown header stays brief with no raw storage; recognized
	// header transitions with raw storage.
	tk = newAbsorbTestTicket()
	sec = absorbLine(tk, &details, &userComments, &comments, secBrief, "## Что-то своё")
	if sec != secBrief || len(tk.RawTemplates.Headers) != 0 {
		t.Errorf("brief + unknown header: sec=%v raw=%d", sec, len(tk.RawTemplates.Headers))
	}
	sec = absorbLine(tk, &details, &userComments, &comments, sec, "## Journal (Журнал)")
	if sec != secJournal || tk.RawTemplates.Headers[secNameJournal] != "## Journal (Журнал)" {
		t.Errorf("brief + recognized header: sec=%v raw=%q", sec, tk.RawTemplates.Headers[secNameJournal])
	}
}

// TestCreatedRawSegmentsGrammar pins finding 3: the raw "Created" meta
// segments are derived from grammar delimiters, so normal lines round-trip
// byte-identically while degenerate ones (empty timestamp, values recurring
// inside free suffixes) store nothing instead of malformed segments.
func TestCreatedRawSegmentsGrammar(t *testing.T) {
	cases := []struct {
		name string
		line string
		ts   string
		who  string
		want metaSegments
		ok   bool
	}{
		{"normal EN", "- Created: 2026-01-02 03:04 · by: alice", "2026-01-02 03:04", "alice",
			metaSegments{Prefix: "- Created: ", Middle: " · by: "}, true},
		{"normal RU", "- Created (Создан): 2026-01-02 03:04 · by (кем): я", "2026-01-02 03:04", "я",
			metaSegments{Prefix: "- Created (Создан): ", Middle: " · by (кем): "}, true},
		{"empty ts", "- Created:  · by: alice", "", "alice", metaSegments{}, false},
		{"ts inside suffix", "- Created (2026-01-02 03:04 note): 2026-01-02 03:04 · by (кем): bob", "2026-01-02 03:04", "bob",
			metaSegments{Prefix: "- Created (2026-01-02 03:04 note): ", Middle: " · by (кем): "}, true},
		{"who inside suffix", "- Created (by alice): 2026-01-02 03:04 · by (кем): alice", "2026-01-02 03:04", "alice",
			metaSegments{Prefix: "- Created (by alice): ", Middle: " · by (кем): "}, true},
		{"ts contains separator", "- Created: a · by b · by: alice", "a · by b", "alice", metaSegments{}, false},
	}
	for _, tc := range cases {
		got, ok := createdRawSegments(tc.line, tc.ts, tc.who)
		if ok != tc.ok || got != tc.want {
			t.Errorf("%s: createdRawSegments = (%+v, %v), want (%+v, %v)", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

// TestParseCreatedRawRoundTrip checks end-to-end: a normal EN Created line
// stores a raw template and renders back byte-identically (full-file round
// trip); an empty-timestamp line stores no raw template (dict fallback).
func TestParseCreatedRawRoundTrip(t *testing.T) {
	src := "# T-0001 · BUG: x\n\n- Status (Статус): open\n- Priority (Приоритет): low\n" +
		"- Created: 2026-01-02 03:04 · by: alice\n- Project (Проект): p\n\n" +
		"## Summary (Кратко)\nx\n\n## Details (Подробности)\ndetail\n\n" +
		"## User comments (Комментарии от пользователя)\n" + dictRU.userCommentsStub + "\n\n" +
		"## Comments (Комментарии)\n\n## Journal (Журнал)\n- 2026-01-02 03:04 — тикет создан (alice).\n"
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := tk.RawTemplates.Meta["Created"]; got != (metaSegments{Prefix: "- Created: ", Middle: " · by: "}) {
		t.Errorf("Meta[Created] = %+v, want %+v", got, metaSegments{Prefix: "- Created: ", Middle: " · by: "})
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Equal(got, []byte(src)) {
		t.Errorf("normal Created round trip mismatch:\n got %q\nwant %q", got, src)
	}

	// Empty ts: degenerate, no raw template; parse stays tolerant.
	tk, _, err = Parse([]byte("# T-0001 · BUG: x\n\n- Created:  · by: alice\n"))
	if err != nil {
		t.Fatalf("Parse empty ts: %v", err)
	}
	if _, ok := tk.RawTemplates.Meta["Created"]; ok {
		t.Errorf("empty ts: raw template stored %q, want none (dict fallback)", tk.RawTemplates.Meta["Created"])
	}

	// Who inside suffix: raw template sound, meta line renders back verbatim.
	line := "- Created (by alice): 2026-01-02 03:04 · by (кем): alice"
	tk, _, err = Parse([]byte("# T-0001 · BUG: x\n\n" + line + "\n"))
	if err != nil {
		t.Fatalf("Parse who-in-suffix: %v", err)
	}
	if got := tk.RawTemplates.Meta["Created"]; got != (metaSegments{Prefix: "- Created (by alice): ", Middle: " · by (кем): "}) {
		t.Errorf("who-in-suffix: Meta[Created] = %+v", got)
	}
	got, err = Render(tk, nil)
	if err != nil {
		t.Fatalf("Render who-in-suffix: %v", err)
	}
	if !bytes.Contains(got, []byte(line)) {
		t.Errorf("who-in-suffix: rendered meta line missing:\n got %q", got)
	}
}

// roundTripSrc assembles a full ticket around the given head meta lines,
// with sections, stubs and journal in lang's dict form, so Parse/Render
// round trips can be compared byte-exactly.
func roundTripSrc(head string, lang Lang) string {
	d := getDict(lang)
	journal := fmt.Sprintf(d.journalCreation, "2026-01-02 03:04", "alice") + "\n"
	return "# T-0001 · BUG: x\n\n" + head + "\n\n" +
		d.headers[0] + "\nx\n\n" + d.headers[1] + "\ndetail\n\n" +
		d.headers[2] + "\nuser text\n\n" + d.headers[3] + "\n\n" +
		d.headers[4] + "\n" + journal
}

// TestRawMetaSegmentsLiteralPercentS pins the T-0036 🔴 fix: raw meta
// storage holds literal segments, not printf templates, so a literal "%s"
// anywhere in a meta line — Created label suffix, Created middle, Created
// author value, single-meta label suffix or value — round-trips
// byte-identically. (The old LastIndex "%s" substitution corrupted the
// Created middle and author-value shapes.)
func TestRawMetaSegmentsLiteralPercentS(t *testing.T) {
	cases := []struct {
		name string
		line string
		lang Lang
	}{
		{"created label suffix", "- Created (created %s): 2026-01-02 03:04 · by (author): carol", LangRU},
		{"created middle", "- Created: 2026-01-02 03:04 · by (%s help): carol", LangRU},
		{"created author value", "- Created: 2026-01-02 03:04 · by: car%sol", LangRU},
		{"status label suffix", "- Status (%s check): open", LangEN},
		{"status value", "- Status (Статус): open (was %s)", LangRU},
		{"project value", "- Project (Проект): p%s", LangRU},
	}
	for _, tc := range cases {
		head := []string{
			"- Status (Статус): open",
			"- Priority (Приоритет): low",
			"- Created: 2026-01-02 03:04 · by: alice",
			"- Project (Проект): p",
		}
		switch {
		case strings.HasPrefix(tc.line, "- Status"):
			head[0] = tc.line
		case strings.HasPrefix(tc.line, "- Created"):
			head[2] = tc.line
		case strings.HasPrefix(tc.line, "- Project"):
			head[3] = tc.line
		}
		src := roundTripSrc(strings.Join(head, "\n"), tc.lang)
		tk, unknown, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("%s: Parse: %v", tc.name, err)
		}
		got, err := Render(tk, unknown)
		if err != nil {
			t.Fatalf("%s: Render: %v", tc.name, err)
		}
		if !bytes.Equal(got, []byte(src)) {
			t.Errorf("%s: round trip mismatch:\n got %q\nwant %q", tc.name, got, src)
		}
	}
}
