package domain

import (
	"testing"
)

// TestLangDetectionRUBilingual tests that RU bilingual format (parenthesized
// Russian suffixes) is detected as LangRU.
func TestLangDetectionRUBilingual(t *testing.T) {
	src := `# T-0001 · BUG: test

- Status (Статус): open
- Priority (Приоритет): high
- Created (Создан): 2026-01-01 10:00 · by (кем): user
- Project (Проект): p

## Summary (Кратко)
test

## Details (Подробности)
details

## User comments (Комментарии от пользователя)

## Comments (Комментарии)

## Journal (Журнал)
- 2026-01-01 10:00 — тикет создан (user).
`
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.Lang != LangRU {
		t.Errorf("Lang = %q, want %q", tk.Lang, LangRU)
	}
	// Round-trip: must be byte-identical.
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if string(got) != src {
		t.Errorf("round trip mismatch:\n got %q\nwant %q", got, src)
	}
}

// TestLangDetectionENPlain tests that plain EN format (no parentheses) is
// detected as LangEN.
func TestLangDetectionENPlain(t *testing.T) {
	src := `# T-0001 · BUG: test

- Status: open
- Priority: high
- Created: 2026-01-01 10:00 · by: user
- Project: p

## Summary
test

## Details
details

## User comments

## Comments

## Journal
- 2026-01-01 10:00 — ticket created (user).
`
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.Lang != LangEN {
		t.Errorf("Lang = %q, want %q", tk.Lang, LangEN)
	}
	// Round-trip: must be byte-identical.
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if string(got) != src {
		t.Errorf("round trip mismatch:\n got %q\nwant %q", got, src)
	}
}

// TestLangDetectionFreeSuffix tests that a free suffix (not matching any dict)
// is treated as LangEN fallback and survives round trip byte-identically.
func TestLangDetectionFreeSuffix(t *testing.T) {
	src := `# T-0001 · BUG: test

- Status (任意语言): open
- Priority (自由后缀): high
- Created (创建时间): 2026-01-01 10:00 · by (作者): user
- Project (项目): p

## Summary (任意语言)
test

## Details (自由后缀)
details

## User comments (用户评论)
text

## Comments (评论)

## Journal (日志)
- 2026-01-01 10:00 — ticket created (user).
`
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.Lang != LangEN {
		t.Errorf("Lang = %q, want %q (free suffix fallback)", tk.Lang, LangEN)
	}
	// Round-trip: must be byte-identical (raw templates preserve free suffixes).
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if string(got) != src {
		t.Errorf("round trip mismatch:\n got %q\nwant %q", got, src)
	}
}

// TestLangDetectionGarbageFile tests that a garbage file (no recognized
// headers/meta) defaults to LangEN and parses tolerantly without panic.
func TestLangDetectionGarbageFile(t *testing.T) {
	src := "garbage\nno structure\n## Unknown Section\nmore garbage\n"
	tk, _, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.Lang != LangEN {
		t.Errorf("Lang = %q, want %q (garbage fallback)", tk.Lang, LangEN)
	}
	// No panic is success.
}

// TestJournalParseBothLanguages tests that journal lines in both RU and EN
// formats are recognized and round-trip correctly.
func TestJournalParseBothLanguages(t *testing.T) {
	cases := []struct {
		name string
		line string
		lang Lang
		want JournalEntry
	}{
		{"RU creation", "- 2026-01-01 10:00 — тикет создан (user).", LangRU,
			JournalEntry{At: parseTS("2026-01-01 10:00"), From: StatusOpen, To: StatusOpen, Who: "user"}},
		{"EN creation", "- 2026-01-01 10:00 — ticket created (user).", LangEN,
			JournalEntry{At: parseTS("2026-01-01 10:00"), From: StatusOpen, To: StatusOpen, Who: "user"}},
		{"RU transition", "- 2026-01-01 11:00 — статус: open → wip (user)", LangRU,
			JournalEntry{At: parseTS("2026-01-01 11:00"), From: StatusOpen, To: StatusWip, Who: "user"}},
		{"EN transition", "- 2026-01-01 11:00 — status: open → wip (user)", LangEN,
			JournalEntry{At: parseTS("2026-01-01 11:00"), From: StatusOpen, To: StatusWip, Who: "user"}},
		{"RU transition with comment", "- 2026-01-01 12:00 — статус: wip → done · fixed (user)", LangRU,
			JournalEntry{At: parseTS("2026-01-01 12:00"), From: StatusWip, To: StatusDone, Comment: "fixed", Who: "user"}},
		{"EN transition with comment", "- 2026-01-01 12:00 — status: wip → done · fixed (user)", LangEN,
			JournalEntry{At: parseTS("2026-01-01 12:00"), From: StatusWip, To: StatusDone, Comment: "fixed", Who: "user"}},
		{"RU archive", "- 2026-01-01 13:00 — перенесён в архив (user)", LangRU,
			JournalEntry{At: parseTS("2026-01-01 13:00"), Who: "user", Archived: true}},
		{"EN archive", "- 2026-01-01 13:00 — moved to archive (user)", LangEN,
			JournalEntry{At: parseTS("2026-01-01 13:00"), Who: "user", Archived: true}},
	}
	for _, tc := range cases {
		e, ok := parseJournalEntry(tc.line)
		if !ok {
			t.Errorf("%s: parseJournalEntry(%q) = false, want true", tc.name, tc.line)
			continue
		}
		if e.At != tc.want.At || e.From != tc.want.From || e.To != tc.want.To || e.Comment != tc.want.Comment || e.Who != tc.want.Who || e.Archived != tc.want.Archived {
			t.Errorf("%s: parseJournalEntry(%q) = %+v, want %+v", tc.name, tc.line, e, tc.want)
		}
	}
}

// TestStubRecognitionAllDicts tests that stubs from all dicts are recognized
// as empty sections during Parse.
func TestStubRecognitionAllDicts(t *testing.T) {
	for _, entry := range allDicts {
		if !isStubLine(entry.d.detailsStub) {
			t.Errorf("lang %q: detailsStub not recognized as stub", entry.lang)
		}
		if !isStubLine(entry.d.userCommentsStub) {
			t.Errorf("lang %q: userCommentsStub not recognized as stub", entry.lang)
		}
	}
}

// TestSuffixValidation tests that suffixes containing "##" or nested parens
// are rejected (post-guard per §B).
func TestSuffixValidation(t *testing.T) {
	cases := []struct {
		suffix string
		valid  bool
	}{
		{"任意", true},
		{"Статус", true},
		{"", true},
		{"foo##bar", false},
		{"foo(bar)", false},
		{"foo)bar", false},
		{"(nested)", false},
	}
	for _, tc := range cases {
		got := validSuffix(tc.suffix)
		if got != tc.valid {
			t.Errorf("validSuffix(%q) = %v, want %v", tc.suffix, got, tc.valid)
		}
	}
}
