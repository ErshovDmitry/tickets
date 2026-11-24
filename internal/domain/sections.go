package domain

import (
	"slices"
	"strings"
)

// T-0241 A1: the section-selection model behind `ticket show --section`.
// It is the single place that knows how a section is NAMED on the command
// line and how its body is extracted from a parsed ticket, so the read path
// (T-0241) and the future write path (T-0240) cannot drift apart.
//
// The canonical section identifiers are NOT redeclared here: sectionName and
// its secName* constants (i18n_types.go) remain the source of truth, and
// sectionNames (i18n_detect.go) keeps their order. This file only adds the
// CLI token spelling and the body accessor on top of them.

// Section is one selectable ticket section. It is an alias, not a new type,
// so a Section compares directly with the parser's canonical identifiers and
// with the keys of Ticket.RawTemplates.
type Section = sectionName

const (
	SectionSummary      Section = secNameSummary
	SectionDetails      Section = secNameDetails
	SectionUserComments Section = secNameUserComments
	SectionComments     Section = secNameComments
	SectionJournal      Section = secNameJournal
)

// sectionAlias maps an accepted non-canonical CLI token to its section.
// Aliases exist because two sections are stored under a different Go field
// name than their header suggests (Summary lives in Ticket.Brief) and because
// the natural-language form of a header contains a space, which can never be
// a canonical token. They are deliberately NOT part of CanonicalSectionNames.
var sectionAlias = map[string]Section{
	"brief":         SectionSummary,
	"user_comments": SectionUserComments,
}

// sectionToken renders a canonical section identifier as its CLI token: lower
// case with spaces replaced by hyphens ("User comments" -> "user-comments").
// The raw identifiers are never RETURNED by CanonicalSectionNames — only the
// CLI tokens are. ParseSectionName lower-cases the input and compares it with
// exactly these tokens plus the aliases, so only a raw identifier that is a
// single word resolves: "--section Summary" works, "--section User comments"
// does not — that section is reachable as "user-comments" or via the
// "user_comments" alias.
func sectionToken(name sectionName) string {
	return strings.ReplaceAll(strings.ToLower(string(name)), " ", "-")
}

// CanonicalSectionNames returns the canonical CLI tokens in sectionNames
// order. It is the authoritative list for error messages: a caller must never
// hard-code the names in a message template, so a new section cannot become
// unselectable without telling the user about it.
func CanonicalSectionNames() []string {
	out := make([]string, 0, len(sectionNames))
	for _, name := range sectionNames {
		out = append(out, sectionToken(name))
	}
	return out
}

// SectionAliasNames returns the accepted non-canonical tokens in sorted order.
// It complements CanonicalSectionNames for the same reason: the unknown-section
// message names both sets, and neither may be hard-coded in the template.
func SectionAliasNames() []string {
	out := make([]string, 0, len(sectionAlias))
	for alias := range sectionAlias {
		out = append(out, alias)
	}
	slices.Sort(out)
	return out
}

// ParseSectionName resolves one `--section` value to a Section.
//
// Exactly two normalizations are applied: lower casing and trimming the
// surrounding whitespace. Inner spaces are NOT normalized into hyphens, so
// "user comments" is rejected while the canonical "user-comments" is
// accepted — a token either matches a canonical spelling or it does not.
// The accepted set is the five canonical tokens plus the sectionAlias
// entries; anything else yields ok=false and the caller renders the
// unknown-section message.
func ParseSectionName(s string) (Section, bool) {
	token := strings.TrimSpace(strings.ToLower(s))
	for _, name := range sectionNames {
		if sectionToken(name) == token {
			return name, true
		}
	}
	if sec, ok := sectionAlias[token]; ok {
		return sec, true
	}
	return "", false
}

// SectionBody returns the BODY of sec — the payload the parser stored, without
// the "##" header line. The payload is parser-normalized, not byte-verbatim
// with the file, and it is language independent: a RU dictionary header, a
// plain EN header and an EN header with a free suffix all yield the same body,
// because recognition already happened in Parse.
//
// An absent section yields "": SectionBody never fails and never reports
// whether the section existed (a legacy ticket simply has no User comments).
func (t Ticket) SectionBody(sec Section) string {
	switch sec {
	case SectionSummary:
		// Summary is stored as lines, not as one string (Ticket.Brief).
		return strings.Join(t.Brief, "\n")
	case SectionDetails:
		return t.Details
	case SectionUserComments:
		return t.UserComments
	case SectionComments:
		return t.Comments
	case SectionJournal:
		return journalBody(t.Journal)
	}
	return ""
}

// journalBody joins the original text of every recognized journal entry. The
// separator blank lines between entries are dropped by Parse, so the payload
// is exactly the entry lines; entries without Raw contribute nothing here.
// Raw is filled by Parse alone — every in-memory append site (new, the
// set/archive/delete paths, edit) leaves it empty — so Render, via
// writeJournalLine, and not this accessor, is what puts those lines into the
// file.
func journalBody(entries []JournalEntry) string {
	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Raw != "" {
			lines = append(lines, e.Raw)
		}
	}
	return strings.Join(lines, "\n")
}
