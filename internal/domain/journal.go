package domain

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// journalPatterns holds the journal regexes of one dict, compiled once from
// its templates. A nil pattern means the dict template is malformed and the
// pattern is skipped (T-0036: parsing used to recompile them per line).
type journalPatterns struct {
	creation   *regexp.Regexp
	archive    *regexp.Regexp
	transition *regexp.Regexp
}

// journalDictPatterns caches compiled patterns per registered dict, built at
// package init from allDicts (templates validated once by the same guards
// the old per-call path applied).
var journalDictPatterns = buildJournalPatterns()

func buildJournalPatterns() map[*dict]journalPatterns {
	m := make(map[*dict]journalPatterns, len(allDicts))
	for _, entry := range allDicts {
		m[entry.d] = compileJournalPatterns(entry.d)
	}
	return m
}

func compileJournalPatterns(d *dict) journalPatterns {
	return journalPatterns{
		creation:   compileCreationPattern(d.journalCreation),
		archive:    compileArchivePattern(d.journalArchive),
		transition: compileTransitionPattern(d.journalTransition),
	}
}

// compileCreationPattern compiles the creation template ("- %s…(%s).").
// A malformed template yields nil and the pattern is skipped.
func compileCreationPattern(template string) *regexp.Regexp {
	middle, ok := journalMiddle(template, "(%s)")
	if !ok {
		return nil
	}
	return regexp.MustCompile(`^- (.+)` + regexp.QuoteMeta(middle) + `\((.*)\)\.$`)
}

// compileArchivePattern compiles the archive template ("- %s…(%s)").
func compileArchivePattern(template string) *regexp.Regexp {
	middle, ok := journalMiddle(template, "(%s)")
	if !ok {
		return nil
	}
	return regexp.MustCompile(`^- (.+)` + regexp.QuoteMeta(middle) + `\((.*)\)$`)
}

// compileTransitionPattern compiles the transition template
// ("- %s…%s…%s"); the template must contain exactly three "%s" slots.
func compileTransitionPattern(template string) *regexp.Regexp {
	if strings.Count(template, "%s") != 3 {
		return nil
	}
	start := strings.Index(template, "%s") + len("%s")
	end := strings.LastIndex(template, "%s")
	if end < start {
		return nil
	}
	parts := strings.Split(template[start:end], "%s")
	if len(parts) != 2 {
		return nil
	}
	return regexp.MustCompile(`^- (.+)` + regexp.QuoteMeta(parts[0]) + `(\S+)` +
		regexp.QuoteMeta(parts[1]) + `(\S+)(?: · (.*))? \((.*)\)$`)
}

// parseJournalEntry tries to parse one journal line against all known
// dictionaries. It returns (entry, ok).
func parseJournalEntry(line string) (JournalEntry, bool) {
	// Try all dicts.
	for _, entry := range allDicts {
		if e, ok := tryParseJournalDict(line, entry.d); ok {
			return e, true
		}
	}
	return JournalEntry{}, false
}

// tryParseJournalDict tries to parse one journal line against the creation,
// archive and transition patterns of d. The three patterns are mutually
// exclusive alternatives: at most one can match, so the first hit wins.
// A malformed template skips only its own pattern (nil patterns are
// skipped, never panicked on); ok is false when nothing matches.
func tryParseJournalDict(line string, d *dict) (JournalEntry, bool) {
	p, ok := journalDictPatterns[d]
	if !ok {
		p = compileJournalPatterns(d) // ad-hoc dicts (tests): compile on demand
	}
	return matchJournalPatterns(line, p)
}

// matchJournalPatterns matches line against the creation, archive and
// transition patterns in order; nil patterns (malformed templates) are
// skipped.
func matchJournalPatterns(line string, p journalPatterns) (JournalEntry, bool) {
	if p.creation != nil {
		if m := p.creation.FindStringSubmatch(line); m != nil {
			return JournalEntry{At: parseTS(m[1]), From: StatusOpen, To: StatusOpen, Who: m[2]}, true
		}
	}
	if p.archive != nil {
		if m := p.archive.FindStringSubmatch(line); m != nil {
			return JournalEntry{At: parseTS(m[1]), Who: m[2], Archived: true}, true
		}
	}
	if p.transition != nil {
		if m := p.transition.FindStringSubmatch(line); m != nil {
			return JournalEntry{
				At:      parseTS(m[1]),
				From:    Status(m[2]),
				To:      Status(m[3]),
				Comment: m[4],
				Who:     m[5],
			}, true
		}
	}
	return JournalEntry{}, false
}

// journalMiddle extracts the literal text of a journal template between the
// first "%s" placeholder and the tail marker. It reports whether the template
// has the expected "prefix %s ... tail" structure; a template without a "%s"
// or with the tail missing (or preceding the first "%s") is rejected instead
// of sliced out of bounds.
func journalMiddle(template, tail string) (string, bool) {
	first := strings.Index(template, "%s")
	if first < 0 {
		return "", false
	}
	start := first + len("%s")
	end := strings.Index(template, tail)
	if end < start {
		return "", false
	}
	return template[start:end], true
}

// appendJournalLine parses one journal line into t.Journal. It reports
// whether the line was a recognized creation, transition or archive entry.
// The original text is kept in entry.Raw so Render can re-emit it verbatim
// regardless of t.Lang (plan §B invariant).
func appendJournalLine(t *Ticket, line string) bool {
	if e, ok := parseJournalEntry(line); ok {
		e.Raw = line
		t.Journal = append(t.Journal, e)
		return true
	}
	return false
}

// writeJournalLine emits one journal entry: a non-empty Raw (recognized
// during Parse) re-emits verbatim regardless of lang; an empty Raw (entries
// appended by set/archive) emits the lang's dict format.
func writeJournalLine(b *bytes.Buffer, e JournalEntry, lang Lang) {
	if e.Raw != "" {
		b.WriteString(e.Raw + "\n")
		return
	}
	d := getDict(lang)
	ts := e.At.Format(tsLayout)
	switch {
	case e.Archived:
		fmt.Fprintf(b, d.journalArchive+"\n", ts, e.Who)
	case e.From == StatusOpen && e.To == StatusOpen && e.Comment == "":
		fmt.Fprintf(b, d.journalCreation+"\n", ts, e.Who)
	case e.Comment == "":
		fmt.Fprintf(b, d.journalTransition+" (%s)\n", ts, e.From, e.To, e.Who)
	default:
		fmt.Fprintf(b, d.journalTransition+" · %s (%s)\n", ts, e.From, e.To, e.Comment, e.Who)
	}
}
