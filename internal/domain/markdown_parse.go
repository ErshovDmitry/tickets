package domain

import (
	"strconv"
	"strings"
)

// absorbLine consumes one line outside the journal section, updating the
// ticket and returning the new section. Transition rules preserve the
// pre-T-0036 state machine (bash reference): only the NEXT section's header
// ends the current one, an unrecognized "## ..." line in head/brief leaves
// for brief (content ignored there, no meta capture), and every other line
// stays verbatim in the current section. Raw templates are stored only on a
// real transition.
func absorbLine(t *Ticket, details, userComments, comments *[]string, sec section, line string) section {
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
			t.RawTemplates.Headers[secName] = line
			return sectionFor(string(secName))
		}
		if strings.HasPrefix(line, "## ") {
			// Unrecognized header (pre-T-0036 headerName path): stay brief
			// with no raw storage; following meta lines stay ignored.
			return secBrief
		}
		return sec
	default: // secHead
		if applyHeadMeta(t, line) {
			return sec
		}
		if secName, _, _, ok := matchSectionHeader(line); ok {
			t.RawTemplates.Headers[secName] = line
			return sectionFor(string(secName))
		}
		if strings.HasPrefix(line, "## ") {
			// Unrecognized header (pre-T-0036 headerName path): leave head
			// with no raw storage so a meta line after it is NOT captured.
			return secBrief
		}
		return sec
	}
}

// applyHeadMeta captures the H1 line and the metadata block lines.
func applyHeadMeta(t *Ticket, line string) bool {
	if m := h1Re.FindStringSubmatch(line); m != nil {
		t.Number, _ = strconv.Atoi(m[1]) // tolerant: overflow -> 0
		t.Type = Type(m[2])
		t.Title = m[3]
		return true
	}

	// Try to match meta line.
	if key, _, _, values, ok := matchMetaLine(line); ok {
		switch key {
		case "Status":
			t.Status = Status(values[0])
			storeMeta(t, "Status", line, values[0])
		case "Priority":
			t.Priority = Priority(values[0])
			storeMeta(t, "Priority", line, values[0])
		case "Project":
			t.Project = values[0]
			storeMeta(t, "Project", line, values[0])
		case "Created":
			if len(values) == 2 {
				t.Created, t.Who = parseTS(values[0]), values[1]
				// Degenerate lines (empty/garbled timestamp, values not at
				// grammar positions) store nothing: Render falls back to
				// dict emission.
				if seg, ok := createdRawSegments(line, values[0], values[1]); ok {
					t.RawTemplates.Meta["Created"] = seg
				}
			}
		}
		return true
	}

	return false
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
		return secBrief // unknown header: ignore its content
	}
}
