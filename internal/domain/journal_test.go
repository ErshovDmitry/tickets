package domain

import "testing"

// TestJournalDictPatternsPrecompiled verifies that the package-init pass
// compiled non-nil patterns for every registered dict (T-0036: parsing must
// not recompile regexes per line).
func TestJournalDictPatternsPrecompiled(t *testing.T) {
	if len(journalDictPatterns) != len(allDicts) {
		t.Fatalf("journalDictPatterns = %d dicts, want %d", len(journalDictPatterns), len(allDicts))
	}
	for _, entry := range allDicts {
		p := journalDictPatterns[entry.d]
		if p.creation == nil || p.archive == nil || p.transition == nil {
			t.Errorf("lang %q: patterns not fully compiled (creation/archive/transition nil: %v/%v/%v)",
				entry.lang, p.creation == nil, p.archive == nil, p.transition == nil)
		}
	}
}

// TestParseJournalEntryGarbageLines verifies that unrecognized journal lines
// return ok=false instead of a false-positive entry.
func TestParseJournalEntryGarbageLines(t *testing.T) {
	for _, line := range []string{
		"",
		"garbage",
		"- not a journal line",
		"- 2026-01-01 10:00 — totally unknown (user)",
		"- 2026-01-01 10:00 — status: (user)",
		"## Summary",
	} {
		if e, ok := parseJournalEntry(line); ok {
			t.Errorf("parseJournalEntry(%q) = %+v, want false", line, e)
		}
	}
}

// TestTryParseJournalDictMalformedTemplates verifies that a dict whose
// journal templates deviate from the expected shape is skipped safely:
// tryParseJournalDict returns false (never panics) for any line.
func TestTryParseJournalDictMalformedTemplates(t *testing.T) {
	cases := []struct {
		name string
		d    dict
	}{
		{"no placeholders", dict{
			journalCreation:   "- ticket created.",
			journalTransition: "- status change",
			journalArchive:    "- moved to archive",
		}},
		{"archive tail precedes %s", dict{
			journalCreation:   "- %s — ticket created.",
			journalTransition: "- %s — status change",
			journalArchive:    "(%s) moved to archive %s",
		}},
		{"transition two slots", dict{
			journalCreation:   "- %s — ticket created.",
			journalTransition: "- %s — status: %s",
			journalArchive:    "(%s) moved to archive %s",
		}},
		{"transition four slots", dict{
			journalCreation:   "- %s — ticket created.",
			journalTransition: "- %s — status: %s → %s at %s",
			journalArchive:    "(%s) moved to archive %s",
		}},
		{"empty templates", dict{}},
	}
	lines := []string{
		"garbage",
		"- 2026-01-01 10:00 — ticket created (user).",
		"- 2026-01-01 10:00 — status: open → wip (user)",
		"- 2026-01-01 10:00 — moved to archive (user)",
	}
	for _, tc := range cases {
		for _, line := range lines {
			if e, ok := tryParseJournalDict(line, &tc.d); ok {
				t.Errorf("%s: tryParseJournalDict(%q) = %+v, want false", tc.name, line, e)
			}
		}
	}
}

// TestTryParseJournalDictSkipsOnlyMalformedPattern verifies that a malformed
// template skips only its own pattern: sibling templates of the same dict
// still parse (no panic, no whole-dict blackout).
func TestTryParseJournalDictSkipsOnlyMalformedPattern(t *testing.T) {
	d := dict{
		journalCreation:   "- %s — ticket created.", // malformed: no (%s) tail
		journalTransition: "- %s — status: %s → %s",
		journalArchive:    "- %s — moved to archive (%s)",
	}
	if e, ok := tryParseJournalDict("- 2026-01-01 10:00 — ticket created (user).", &d); ok {
		t.Errorf("malformed creation must be skipped, got %+v", e)
	}
	want := JournalEntry{At: parseTS("2026-01-01 10:00"), Who: "user", Archived: true}
	e, ok := tryParseJournalDict("- 2026-01-01 10:00 — moved to archive (user)", &d)
	if !ok || e.At != want.At || e.Who != want.Who || !e.Archived {
		t.Errorf("archive line: tryParseJournalDict = (%+v, %v), want (%+v, true)", e, ok, want)
	}
}
