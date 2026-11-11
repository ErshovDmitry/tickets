package domain

import (
	"strconv"
	"strings"
)

// absorbLine consumes one line outside the journal section, updating the
// ticket and returning the new section. Transition rules preserve the
// pre-T-0036 state machine (bash era): only the NEXT section's header
// ends the current one. An unrecognized "## ..." line in head starts
// secHeadStray: the line and everything after it (until a recognized
// header) is captured into Ticket.HeadStray with no meta capture (D1,
// T-0166); unrecognized-header content inside the brief zone is captured
// into Ticket.Brief instead of being silently dropped. Every other line
// stays verbatim in the current section. Raw templates are stored only on
// a real transition. flags track the structural-blank positions in the
// head zone (only the FIRST blank after the H1 / after the metadata block
// is dropped, always — C2-1); Parse owns the variable.
func absorbLine(t *Ticket, details, userComments, comments *[]string, sec section, line string, flags *headFlags) section {
	switch sec {
	case secDetails:
		// Only the next section's header ends Details: Summary/Details
		// headers and unknown "## ..." lines stay details TEXT (pre-T-0036
		// exact-match semantics).
		if secName, _, _, ok := matchSectionHeader(line); ok {
			if secName == secNameUserComments || secName == secNameComments || secName == secNameJournal {
				t.RawTemplates.Headers[secName] = line
				return sectionFor(string(secName))
			}
		}
		*details = append(*details, line)
		return sec
	case secUserComments:
		// Only the next section's header ends the user-remarks section:
		// anything else — stray headers and a duplicate own header included —
		// stays verbatim (content-preserving policy, mirrors secComments).
		if secName, _, _, ok := matchSectionHeader(line); ok {
			if secName == secNameComments || secName == secNameJournal {
				t.RawTemplates.Headers[secName] = line
				return sectionFor(string(secName))
			}
		}
		*userComments = append(*userComments, line)
		return sec
	case secComments:
		// Only the journal header ends the section: stray lines — other
		// headers and duplicate "## Комментарии" included — stay verbatim
		// (content-preserving policy, mirrors secDetails).
		if secName, _, _, ok := matchSectionHeader(line); ok {
			if secName == secNameJournal {
				t.RawTemplates.Headers[secName] = line
				return sectionFor(string(secName))
			}
		}
		*comments = append(*comments, line)
		return sec
	case secBrief:
		if secName, _, _, ok := matchSectionHeader(line); ok {
			if secName == secNameSummary {
				// Duplicate own header (D3): the first raw template wins,
				// the repeated header line stays brief verbatim.
				t.Brief = append(t.Brief, line)
				return secBrief
			}
			t.RawTemplates.Headers[secName] = line
			return sectionFor(string(secName))
		}
		if strings.HasPrefix(line, "# ") {
			// D2: any "# " line in the brief zone would become a new H1
			// source on Render; it is dropped, never captured.
			return secBrief
		}
		t.Brief = append(t.Brief, line)
		return secBrief
	case secHeadStray:
		if secName, _, _, ok := matchSectionHeader(line); ok {
			t.RawTemplates.Headers[secName] = line
			return sectionFor(string(secName))
		}
		if strings.HasPrefix(line, "# ") {
			// D2: dropped like in the brief zone, never captured.
			return secHeadStray
		}
		openStray(flags)
		t.HeadStray = append(t.HeadStray, line)
		return secHeadStray
	default: // secHead
		return absorbHeadLine(t, flags, line)
	}
}

// ParseHeader parses only the ticket head — the H1 line and the metadata
// block — stopping at the first "## " section header (recognized or not)
// or at EOF. It fills Number/Type/Title/Status/Priority/Created/Who/Project
// and leaves the body sections (Details, Comments, Journal, Unknown) at
// their zero values. Like Parse it is tolerant: it never fails on malformed
// input, so the returned error is always nil and exists only so callers can
// treat header parsing as a fallible operation (mirroring Render). The
// stop semantics mirror secHead in Parse: a meta/H1 line is consumed, any
// "## " line ends the head, and blank or unrecognized lines are skipped
// while staying in the head. RawTemplates maps are initialized exactly as
// Parse does, so applyHeadMeta never writes into a nil map.
func ParseHeader(data []byte) (*Ticket, error) {
	t := &Ticket{
		RawTemplates: rawTemplates{
			Headers: make(map[sectionName]string),
			Meta:    make(map[string]metaSegments),
		},
	}
	h1Seen := false
	for pos := 0; pos < len(data); {
		start, end := lineBounds(data, pos)
		pos = end
		line := string(trimEOL(data[start:end]))
		if m := h1Re.FindStringSubmatch(line); m != nil {
			// Same first-only H1 rule as secHead (D2, T-0166): a repeated
			// H1 is dropped, never overwriting Number/Type/Title.
			if !h1Seen {
				h1Seen = true
				t.Number, _ = strconv.Atoi(m[1]) // tolerant: overflow -> 0
				t.Type = Type(m[2])
				t.Title = m[3]
			}
			continue
		}
		if applyHeadMeta(t, line) {
			continue
		}
		// Any section header ends the head. matchSectionHeader lines all
		// carry the "## " prefix, so this single check subsumes it.
		if strings.HasPrefix(line, "## ") {
			break
		}
		// Blank or unrecognized line inside the head: skip, stay in head.
	}
	return t, nil
}

// storeMeta splits a single-slot meta line into literal segments around the
// value slot. The value is the line tail in the current grammar, so Suffix
// is empty. A value that cannot be located stores nothing: Render emits the
// dict form.
func storeMeta(t *Ticket, key, line, value string) {
	idx := strings.LastIndex(line, value)
	if idx < 0 {
		return
	}
	t.RawTemplates.Meta[key] = metaSegments{Prefix: line[:idx], Suffix: line[idx+len(value):]}
}

// createdRawSegments splits the raw Created meta line into literal segments
// from grammar delimiters, never by searching for the captured values: the
// timestamp slot sits between the ": " that ends the label prefix and the
// first " · by" separator, the author slot is the line tail. If the values
// do not sit at those grammar positions (empty timestamp, values recurring
// inside free suffixes), the line is degenerate and ok is false: no segments
// are stored, Render emits the dict form.
func createdRawSegments(line, ts, who string) (metaSegments, bool) {
	if ts == "" {
		return metaSegments{}, false // degenerate: parseTS already failed
	}
	sep := strings.Index(line, " · by")
	if sep < len(ts)+2 || !strings.HasSuffix(line[:sep], ts) || !strings.HasSuffix(line, who) {
		return metaSegments{}, false
	}
	tsStart := sep - len(ts)
	whoStart := len(line) - len(who)
	if whoStart < 2 || line[tsStart-2:tsStart] != ": " || line[whoStart-2:whoStart] != ": " {
		return metaSegments{}, false
	}
	return metaSegments{
		Prefix: line[:tsStart],
		Middle: line[sep:whoStart],
		Suffix: line[whoStart+len(who):],
	}, true
}

func sectionFor(name string) section {
	switch sectionName(name) {
	case secNameSummary:
		return secBrief
	case secNameDetails:
		return secDetails
	case secNameUserComments:
		return secUserComments
	case secNameComments:
		return secComments
	case secNameJournal:
		return secJournal
	default:
		// Defensive default: absorbLine calls sectionFor only after
		// matchSectionHeader ok, so this branch is unreachable; kept for
		// exhaustiveness — content is never silently dropped (D1/D2).
		return secBrief
	}
}
