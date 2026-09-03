package domain

import (
	"strings"
	"testing"
)

// TestMatchSectionHeaderReturnsLangSignal tests that matchSectionHeader
// returns the correct lang signal and dict-matched flag (LOCK D4 v2).
func TestMatchSectionHeaderReturnsLangSignal(t *testing.T) {
	cases := []struct {
		line     string
		wantName sectionName
		wantLang Lang
		wantDict bool
		wantOK   bool
	}{
		{"## Summary (Кратко)", secNameSummary, LangRU, true, true},
		{"## Summary", secNameSummary, LangEN, true, true},       // plain EN = en-dict match
		{"## Summary (任意)", secNameSummary, LangEN, false, true}, // free suffix
		{"## Details (Подробности)", secNameDetails, LangRU, true, true},
		{"## Journal (Журнал)", secNameJournal, LangRU, true, true},
		{"## Journal", secNameJournal, LangEN, true, true},
		{"## Unknown", "", LangEN, false, false},
		{"## Summary (foo##bar)", "", LangEN, false, false}, // invalid suffix
	}
	for _, tc := range cases {
		name, lang, dictMatched, ok := matchSectionHeader(tc.line)
		if ok != tc.wantOK {
			t.Errorf("matchSectionHeader(%q) ok = %v, want %v", tc.line, ok, tc.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if name != tc.wantName {
			t.Errorf("matchSectionHeader(%q) name = %q, want %q", tc.line, name, tc.wantName)
		}
		if lang != tc.wantLang {
			t.Errorf("matchSectionHeader(%q) lang = %q, want %q", tc.line, lang, tc.wantLang)
		}
		if dictMatched != tc.wantDict {
			t.Errorf("matchSectionHeader(%q) dictMatched = %v, want %v", tc.line, dictMatched, tc.wantDict)
		}
	}
}

// TestMatchMetaLineReturnsLangSignal tests that matchMetaLine returns the
// correct lang signal and dict-matched flag (LOCK D4 v2).
func TestMatchMetaLineReturnsLangSignal(t *testing.T) {
	cases := []struct {
		line     string
		wantKey  string
		wantLang Lang
		wantDict bool
		wantVals []string
		wantOK   bool
	}{
		{"- Status (Статус): open", "Status", LangRU, true, []string{"open"}, true},
		{"- Status: open", "Status", LangEN, true, []string{"open"}, true},
		{"- Status (任意): open", "Status", LangEN, false, []string{"open"}, true}, // free suffix
		{"- Priority (Приоритет): high", "Priority", LangRU, true, []string{"high"}, true},
		{"- Created (Создан): 2026-01-01 10:00 · by (кем): user", "Created", LangRU, true,
			[]string{"2026-01-01 10:00", "user"}, true},
		{"- Created: 2026-01-01 10:00 · by: user", "Created", LangEN, true,
			[]string{"2026-01-01 10:00", "user"}, true},
		{"- Project (Проект): p", "Project", LangRU, true, []string{"p"}, true},
		{"- Unknown: value", "", LangEN, false, nil, false},
	}
	for _, tc := range cases {
		key, lang, dictMatched, vals, ok := matchMetaLine(tc.line)
		if ok != tc.wantOK {
			t.Errorf("matchMetaLine(%q) ok = %v, want %v", tc.line, ok, tc.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if key != tc.wantKey {
			t.Errorf("matchMetaLine(%q) key = %q, want %q", tc.line, key, tc.wantKey)
		}
		if lang != tc.wantLang {
			t.Errorf("matchMetaLine(%q) lang = %q, want %q", tc.line, lang, tc.wantLang)
		}
		if dictMatched != tc.wantDict {
			t.Errorf("matchMetaLine(%q) dictMatched = %v, want %v", tc.line, dictMatched, tc.wantDict)
		}
		if strings.Join(vals, "|") != strings.Join(tc.wantVals, "|") {
			t.Errorf("matchMetaLine(%q) vals = %v, want %v", tc.line, vals, tc.wantVals)
		}
	}
}

// TestDetectLangDictPrecedence pins LOCK D4 v2: the first dict-matched signal
// (exact dict suffix/form; plain EN counts as an en-dict match) decides the
// language; free (non-dict) signals never decide — a free suffix early in
// the file no longer hides a dict signal later in the file.
func TestDetectLangDictPrecedence(t *testing.T) {
	cases := []struct {
		name string
		head string
		want Lang
	}{
		{"free meta then RU dict meta", "- Status (a%sb): open\n- Priority (Приоритет): normal\n", LangRU},
		{"free header then RU dict meta", "## Summary (任意)\n- Priority (Приоритет): normal\n", LangRU},
		{"RU dict meta then free meta", "- Status (Статус): open\n- Priority (任意): low\n", LangRU},
		{"plain EN meta then RU dict meta", "- Status: open\n- Priority (Приоритет): normal\n", LangEN},
		{"plain EN header then RU header", "## Summary\n## Details (Подробности)\n", LangEN},
		{"only free signals", "- Status (任意): open\n## Summary (任意)\n", LangEN},
	}
	for _, tc := range cases {
		src := "# T-0007 · BUG: x\n\n" + tc.head
		tk, _, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("%s: Parse: %v", tc.name, err)
		}
		if tk.Lang != tc.want {
			t.Errorf("%s: Lang = %q, want %q", tc.name, tk.Lang, tc.want)
		}
	}
}
