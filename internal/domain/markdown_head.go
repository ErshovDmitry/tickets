package domain

import (
	"strconv"
	"strings"
)

// Head-zone parsing: H1 / metadata capture, stray series, and the
// strictly-empty blank policy (headFlags rules i–iv, T-0166 CFF-1/CFF-2).

// absorbHeadLine consumes one line in the head zone (secHead): the first
// valid H1 and the metadata block are captured, recognized "## " headers
// end the head, unrecognized "## " lines open the stray zone (D1), any
// other "# " line is dropped (D2). Strictly-empty blanks follow the
// headFlags policy: the FIRST blank at each structural position (right
// after the H1, right after the metadata block) is dropped always (rule
// i, C2-1), a blank before the H1 / metadata block, and the whole blank
// run right after a dropped "# " line, are markup too (rule ii, CFF-1) —
// only a
// blank after a complete head opens the stray series as content, blanks
// inside it stay verbatim (rule iii, T-0035) and finish trims a trailing
// run (rule iv). Rules apply in order (i)→(ii)→(iii)→(iv): (iii) matches
// only blanks inside an already-open stray series — it never overrides a
// pending structural blank (i) or the (ii) run-drop after a dropped "# "
// line, which fire only while the series is still closed. A
// whitespace-only line is always content.
func absorbHeadLine(t *Ticket, flags *headFlags, line string) section {
	if m := h1Re.FindStringSubmatch(line); m != nil {
		// D2 (T-0166): only the FIRST valid H1 is accepted. A repeated
		// H1 would overwrite Number/Type/Title and desync the head
		// from the file name (safeopen rejects such a file), so any
		// later "# " line in the head zone is dropped, never captured.
		if !flags.h1Seen {
			flags.h1Seen = true
			flags.h1BlankPending = true
			flags.metaBlankPending = false
			flags.afterDroppedH1 = false
			t.Number, _ = strconv.Atoi(m[1]) // tolerant: overflow -> 0
			t.Type = Type(m[2])
			t.Title = m[3]
		} else {
			// A repeated valid H1 is a dropped "# " line: a blank right
			// after it is markup (rule ii).
			flags.afterDroppedH1 = true
		}
		return secHead
	}
	if applyHeadMeta(t, line) {
		flags.h1BlankPending = false
		flags.metaBlankPending = true
		flags.metaSeen = true
		flags.afterDroppedH1 = false
		return secHead
	}
	if secName, _, _, ok := matchSectionHeader(line); ok {
		t.RawTemplates.Headers[secName] = line
		return sectionFor(string(secName))
	}
	if strings.HasPrefix(line, "## ") {
		// Unrecognized header (D1): start secHeadStray — the line is
		// captured and no meta line after it is captured.
		openStray(flags)
		t.HeadStray = append(t.HeadStray, line)
		return secHeadStray
	}
	if strings.HasPrefix(line, "# ") {
		// D2: an H1-like line that is not a valid H1 is dropped, never
		// captured — Render must not gain a new H1 source. A blank right
		// after it is markup (rule ii).
		flags.afterDroppedH1 = true
		return secHead
	}
	if line == "" && dropHeadBlank(flags) {
		return secHead
	}
	openStray(flags)
	t.HeadStray = append(t.HeadStray, line)
	return secHead
}

// dropHeadBlank reports whether a strictly-empty blank in the head zone
// is markup (rules i/ii, see headFlags) and drops it, clearing any pending
// structural position. The structural blank right after the H1 / metadata
// block is dropped ALWAYS, even when strays were captured before it
// (rule i, C2-1) — otherwise Render would emit consecutive blank lines
// (MD012). A blank before the H1 / metadata block, and the whole blank
// run right after a dropped "# " line, never opens a stray series (rule
// ii, CFF-1): a HeadStray starting with "" would render as two consecutive
// blanks the source never had. Inside an open stray series a blank is verbatim
// content (rule iii) and is kept. Rules apply in order
// (i)→(ii)→(iii)→(iv): a pending structural blank (i) is always dropped
// first, and (ii) matches only blanks that would OPEN the series, so
// (iii) never overrides either.
func dropHeadBlank(flags *headFlags) bool {
	if flags.h1BlankPending || flags.metaBlankPending {
		flags.h1BlankPending = false
		flags.metaBlankPending = false
		return true
	}
	if flags.strayOpen {
		return false
	}
	return flags.afterDroppedH1 || !flags.h1Seen || !flags.metaSeen
}

// openStray marks the head zone as stray-open: stray content was captured,
// so every further blank is verbatim content, and no structural-blank
// position is pending anymore.
func openStray(flags *headFlags) {
	flags.strayOpen = true
	flags.h1BlankPending = false
	flags.metaBlankPending = false
}

// applyHeadMeta captures a metadata block line. The H1 is captured by the
// callers, not here: only the FIRST valid H1 wins (D2, T-0166), so both
// secHead and ParseHeader run their own first-only check.
func applyHeadMeta(t *Ticket, line string) bool {
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
