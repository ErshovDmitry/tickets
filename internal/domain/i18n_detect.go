package domain

import (
	"strings"
)

// This file recognizes section headers, meta lines and stubs in raw
// ticket markdown, and detects the language of a ticket file.

// matchSectionHeader tries to recognize line as a section header in any known
// dictionary. It returns (canonicalName, langSignal, dictMatched, ok):
// langSignal is the dict lang for an exact suffix match, LangEN for plain EN
// headers and free suffixes; dictMatched reports an exact dict form (plain
// EN counts, a free suffix does not) for detectLang (LOCK D4 v2).
func matchSectionHeader(line string) (sectionName, Lang, bool, bool) {
	// Try all dicts: match "## Name (suffix)" against dict headers.
	for _, entry := range allDicts {
		for i, h := range entry.d.headers {
			if line == h {
				return sectionNames[i], entry.lang, true, true
			}
		}
	}

	// Try plain "## Name" against EN canonical names (no suffix): an
	// en-dict match.
	plainNames := []string{"## Summary", "## Details", "## User comments", "## Comments", "## Journal"}
	for i, plain := range plainNames {
		if line == plain {
			return sectionNames[i], LangEN, true, true
		}
	}

	// Try "## Name (free-suffix)" where Name matches EN canonical but suffix
	// does not match any dict. Post-guard: suffix must not contain "##".
	for i, canonical := range []string{"Summary", "Details", "User comments", "Comments", "Journal"} {
		prefix := "## " + canonical + " ("
		if strings.HasPrefix(line, prefix) && strings.HasSuffix(line, ")") {
			suffix := line[len(prefix) : len(line)-1]
			if validSuffix(suffix) {
				return sectionNames[i], LangEN, false, true
			}
		}
	}

	return "", LangEN, false, false
}

var sectionNames = [5]sectionName{secNameSummary, secNameDetails, secNameUserComments, secNameComments, secNameJournal}

// validSuffix checks suffix constraints: no nested parens, no "##".
func validSuffix(s string) bool {
	return !strings.Contains(s, "(") && !strings.Contains(s, ")") && !strings.Contains(s, "##")
}

// matchMetaLine tries to parse line as a meta line (Status/Priority/Created/
// Project). It returns (key, langSignal, dictMatched, values, ok): key is the
// canonical meta key; langSignal is the dict lang for an exact form match,
// LangEN otherwise; dictMatched reports an exact dict form (plain EN counts,
// a free suffix does not) for detectLang; values holds one value, or two
// for Created (timestamp, author).
func matchMetaLine(line string) (metaKey string, lang Lang, dictMatched bool, values []string, ok bool) {
	// Try all dicts: match emission forms against line.
	for _, entry := range allDicts {
		if k, v, ok := tryMetaDict(line, entry.d, entry.lang); ok {
			return k, entry.lang, true, v, true
		}
	}

	// Try plain EN forms (no suffix in label).
	enPlain := map[string]string{
		"Status":   "- Status: ",
		"Priority": "- Priority: ",
		"Project":  "- Project: ",
	}
	for k, prefix := range enPlain {
		if strings.HasPrefix(line, prefix) {
			val := line[len(prefix):]
			return k, LangEN, true, []string{val}, true
		}
	}
	if strings.HasPrefix(line, "- Created: ") {
		rest := line[len("- Created: "):]
		if parts := strings.Split(rest, " · by: "); len(parts) == 2 {
			return "Created", LangEN, true, parts, true
		}
	}

	// Try EN canonical with free suffix: "- Name (suffix): value".
	for k, canonical := range map[string]string{"Status": "Status", "Priority": "Priority", "Project": "Project"} {
		prefix := "- " + canonical + " ("
		if strings.HasPrefix(line, prefix) {
			if idx := strings.Index(line[len(prefix):], "): "); idx >= 0 {
				suffix := line[len(prefix) : len(prefix)+idx]
				if validSuffix(suffix) {
					val := line[len(prefix)+idx+3:]
					return k, LangEN, false, []string{val}, true
				}
			}
		}
	}
	// Created with free suffix: "- Created (suffix): ts · by (suffix): author"
	if strings.HasPrefix(line, "- Created (") {
		rest := line[len("- Created ("):]
		if idx := strings.Index(rest, "): "); idx >= 0 {
			suffix1 := rest[:idx]
			if !validSuffix(suffix1) {
				return "", LangEN, false, nil, false
			}
			rest = rest[idx+3:]
			if parts := strings.Split(rest, " · by ("); len(parts) == 2 {
				ts := parts[0]
				if idx := strings.Index(parts[1], "): "); idx >= 0 {
					suffix2 := parts[1][:idx]
					if validSuffix(suffix2) {
						author := parts[1][idx+3:]
						return "Created", LangEN, false, []string{ts, author}, true
					}
				}
			}
		}
	}

	return "", LangEN, false, nil, false
}

func tryMetaDict(line string, d *dict, lang Lang) (string, []string, bool) {
	// Status/Priority/Project: "- Label (suffix): value"
	for k, prefix := range map[string]string{
		"Status":   d.statusFmt[:len(d.statusFmt)-2], // strip " %s"
		"Priority": d.priorityFmt[:len(d.priorityFmt)-2],
		"Project":  d.projectFmt[:len(d.projectFmt)-2],
	} {
		if strings.HasPrefix(line, prefix) {
			val := line[len(prefix):]
			return k, []string{val}, true
		}
	}
	// Created: "- Created (suffix): ts · by (suffix): author"
	createdPrefix := d.createdFmt[:strings.Index(d.createdFmt, "%s")]
	if strings.HasPrefix(line, createdPrefix) {
		rest := line[len(createdPrefix):]
		// The format is "ts%s · by (suffix): %s" where first %s is the slot.
		// We split on the middle part " · by (" + suffix + "): ".
		// Simpler: split on " · by " and strip the label suffix from the second part.
		if parts := strings.Split(rest, " · by "); len(parts) == 2 {
			ts := parts[0]
			// Strip label suffix from author part: "(suffix): author" -> "author"
			authorPart := parts[1]
			if idx := strings.Index(authorPart, "): "); idx >= 0 {
				author := authorPart[idx+3:]
				return "Created", []string{ts, author}, true
			}
		}
	}
	return "", nil, false
}

// detectLang implements LOCK D4 v2: the first signal (header or meta line)
// whose suffix/form exactly matches a known dict — plain EN forms count as
// en-dict matches — decides the ticket language; free (non-dict) signals
// never decide. Only a file with NO dict-matched signal at all falls back to
// LangEN (supersedes "first signal wins": a free suffix no longer hides a
// dict signal later in the file).
func detectLang(data []byte) Lang {
	for pos := 0; pos < len(data); {
		start, end := lineBounds(data, pos)
		pos = end
		line := string(trimEOL(data[start:end]))

		// A dict-matched section header decides; free-suffix signals are
		// skipped (D4 v2 precedence).
		if _, lang, dictMatched, ok := matchSectionHeader(line); ok && dictMatched {
			return lang
		}
		if _, lang, dictMatched, _, ok := matchMetaLine(line); ok && dictMatched {
			return lang
		}
	}
	return LangEN
}

// isStubLine returns true if line is exactly a stub (detailsStub or
// userCommentsStub) from any known dictionary.
func isStubLine(line string) bool {
	for _, entry := range allDicts {
		if line == entry.d.detailsStub || line == entry.d.userCommentsStub {
			return true
		}
	}
	return false
}
