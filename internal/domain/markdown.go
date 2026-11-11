package domain

import (
	"bytes"
	"regexp"
	"strings"
	"time"
)

// tsLayout is the timestamp format used everywhere in ticket files.
const tsLayout = "2006-01-02 15:04"

var (
	h1Re = regexp.MustCompile(`^# T-(\d+) · (\S+): (.*)$`)
)

type section uint8

const (
	secHead section = iota // H1 + metadata block
	// secHeadStray: head after an unrecognized "## " header (D1). No H1 or
	// meta capture happens here; every line lands in Ticket.HeadStray until
	// a recognized section header.
	secHeadStray
	// secBrief: inside "## Summary". The body lines are captured into
	// Ticket.Brief (T-0166); a duplicate own header stays brief verbatim.
	secBrief
	secDetails      // inside "## Details"
	secUserComments // inside "## User comments" (T-0035)
	secComments     // inside "## Comments" (T-0032, no bash-era counterpart)
	secJournal      // inside "## Journal"
)

// headFlags tracks the only facts the head zone needs about structural
// blanks (C2-1; final blank policy T-0166 CFF-1/CFF-2):
//
//	(i)   the FIRST strictly-empty blank right after the H1 and the first
//	      right after the metadata block are structural: markup, dropped
//	      ALWAYS, even when strays were captured before (C2-1);
//	(ii)  a strictly-empty blank BEFORE the H1 / metadata block is
//	      markup: dropped, it never opens a stray series — and the
//	      WHOLE blank run right after a dropped "# " line (D2) is
//	      markup too (afterDroppedH1 is not one-shot: dropping only
//	      the first blank would let the second reopen the series with
//	      a leading "" and Render would print "\n\n\n" (MD012)).
//	      Rationale: HeadStray starting with "" would make Render
//	      print two consecutive blanks (MD012) the source never had;
//	(iii) a strictly-empty blank INSIDE a stray series is verbatim
//	      content and stays (T-0035);
//	(iv)  a strictly-empty blank TRAILING the stray series (before the
//	      next header) is the separator Render re-emits after the
//	      strays: finish trims that run, like every section body.
//
// Rules apply in order (i)→(ii)→(iii)→(iv): (iii) matches only blanks
// INSIDE an already-open stray series, so it never overrides a still-
// pending structural blank (i); (ii) matches only blanks that would
// OPEN the series (before the H1 / metadata block, right after a
// dropped "# " line), so once the series is open (iii) keeps blanks
// verbatim even right after a dropped "# " line.
//
// A whitespace-only line is never structural: it is content.
type headFlags struct {
	h1BlankPending   bool // next strictly-empty blank closes the H1 position: drop it (i)
	metaBlankPending bool // next strictly-empty blank closes the metadata block: drop it (i)
	h1Seen           bool // a valid H1 was accepted; later ones are dropped (D2)
	metaSeen         bool // at least one metadata block line was captured
	strayOpen        bool // stray content captured: blanks are verbatim content (iii)
	afterDroppedH1   bool // a dropped "# " line was seen (D2): the WHOLE blank run after it is markup (ii); stays set until H1/meta/openStray, not one-shot
}

// Parse reads ticket markdown tolerantly: it never fails, missing or
// malformed parts become zero values, and any manual bytes after the last
// recognized journal line are preserved verbatim in Ticket.Unknown (also
// returned as the second result). Inside "## Журнал" blank lines are
// deferred, not terminating: a recognized section header after the journal
// resumes parsing (a repeated Journal header merges its entries and sets
// Ticket.JournalDup), while the first non-blank, non-header line still ends
// the recognized tail — Unknown starts at any pending blank run, EXCEPT the
// single strictly empty line Render emits right after the first Journal
// header (MD022): that one is markup, not content, and is swallowed, keeping
// a ticket with an empty journal byte-stable across parse/render cycles. A
// whitespace-only line is NOT that markup: it stays verbatim in Unknown. A
// pending blank run at EOF becomes Unknown too, so trailing blank bytes
// survive (blanks after the LAST journal entry included).
func Parse(data []byte) (*Ticket, []byte, error) {
	t := &Ticket{
		RawTemplates: rawTemplates{
			Headers: make(map[sectionName]string),
			Meta:    make(map[string]metaSegments),
		},
	}
	var details, userComments, comments []string
	sec := secHead
	// Structural-blank tracking for the head zone (C2-1); see headFlags.
	var flags headFlags
	seenJournal := false
	// journalHeaderBlank marks the one strictly empty MD022 line accepted
	// right after the FIRST journal header (see the doc comment). It is
	// raised only on entering the journal section and cleared by the next
	// line.
	journalHeaderBlank := false

	// Detect language first (LOCK D4).
	t.Lang = detectLang(data)

	// At most ONE leading blank line is trimmed alongside trailing ones:
	// Render emits exactly one blank line after each section header
	// (MD022), and without that trim the blank would be re-absorbed as
	// content and accumulate one extra blank per parse/render round trip.
	// Any FURTHER leading blank lines are content and stay verbatim
	// (T-0035: the "## Comments" section is byte-verbatim).
	finish := func() {
		t.Details = strings.Join(trimTrailingEmpty(trimLeadingOneEmpty(details)), "\n")
		if isStubLine(strings.TrimLeft(t.Details, "\n")) {
			t.Details = "" // stub from any dict means empty Details
		}
		t.UserComments = strings.Join(trimTrailingEmpty(trimLeadingOneEmpty(userComments)), "\n")
		if isStubLine(strings.TrimLeft(t.UserComments, "\n")) {
			// Placeholder means no user remarks. Known limitation (by
			// design): user text byte-identical to the stub is treated
			// the same.
			t.UserComments = ""
		}
		// The free-form "## Comments (Комментарии)" section has no stub: only
		// ONE leading blank line is trimmed — the one Render prints after the
		// header (MD022). Any further leading blank line is content and stays
		// byte-verbatim (T-0035).
		t.Comments = strings.Join(trimTrailingEmpty(trimLeadingOneEmpty(comments)), "\n")
		// Head-strays: strictly-empty blanks INSIDE the stray series are
		// verbatim content and stay (rule iii, T-0035); only a TRAILING
		// run of strictly-empty blanks is trimmed (rule iv) — Render
		// re-emits that separator after the strays itself, so keeping it
		// would add a blank per round trip. Blanks could only lead the
		// series here when the head already captured H1 + metadata with
		// no dropped "# " line in between; every other leading blank was
		// dropped as markup at parse time (rules i/ii, CFF-1), so no
		// leading-blank trim is needed. An empty result stays nil so
		// Render emits nothing.
		t.HeadStray = trimTrailingEmpty(t.HeadStray)
		if len(t.HeadStray) == 0 {
			t.HeadStray = nil
		}
		t.Brief = trimTrailingEmpty(trimLeadingOneEmpty(t.Brief))
		if len(t.Brief) == 0 {
			t.Brief = nil
		}
	}
	blankRunStart := -1
	for pos := 0; pos < len(data); {
		start, end := lineBounds(data, pos)
		pos = end
		line := string(trimEOL(data[start:end]))
		if sec == secJournal {
			if journalHeaderBlank {
				// Exactly one STRICTLY empty line after the first journal
				// header is the MD022 blank Render prints: swallow it rather
				// than start a pending blank run. A whitespace-only line is
				// content and starts that run verbatim.
				journalHeaderBlank = false
				if line == "" {
					continue
				}
			}
			if strings.TrimSpace(line) == "" {
				if blankRunStart < 0 {
					blankRunStart = start
				}
				continue
			}
			if name, _, _, ok := matchSectionHeader(line); ok {
				blankRunStart = -1
				if name == secNameJournal {
					t.JournalDup = true
					continue
				}
				t.RawTemplates.Headers[name] = line
				sec = sectionFor(string(name))
				continue
			}
			if !appendJournalLine(t, line) {
				if blankRunStart >= 0 {
					start = blankRunStart
				}
				// Unknown tail: stop early — but Details must still be
				// finalized exactly as in the normal path, otherwise a
				// manual journal tail renders an empty stub (round-trip
				// regression fixed during integration).
				t.Unknown = data[start:]
				finish()
				return t, t.Unknown, nil
			}
			blankRunStart = -1
			continue
		}
		sec = absorbLine(t, &details, &userComments, &comments, sec, line, &flags)
		if sec == secJournal {
			// A Journal header reached from any non-journal section (head,
			// brief, details, user comments, comments) is the first or a
			// repeat journal: flag repeats so the store can warn, same as
			// the in-journal adjacent-dup branch above (T-0070 F1).
			if seenJournal {
				// A repeated journal header (adjacent or not) never raises the
				// blank-flag: the blank line after it is separators/content.
				t.JournalDup = true
			} else {
				seenJournal = true
				journalHeaderBlank = true
			}
		}
	}
	if blankRunStart >= 0 {
		t.Unknown = data[blankRunStart:]
	}
	finish()
	return t, t.Unknown, nil
}

func lineBounds(data []byte, pos int) (start, end int) {
	start = pos
	if i := bytes.IndexByte(data[pos:], '\n'); i >= 0 {
		end = pos + i + 1
	} else {
		end = len(data)
	}
	return start, end
}

func trimEOL(line []byte) []byte {
	line = bytes.TrimSuffix(line, []byte("\n"))
	return bytes.TrimSuffix(line, []byte("\r"))
}

func parseTS(s string) time.Time {
	t, err := time.Parse(tsLayout, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func trimTrailingEmpty(lines []string) []string {
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// trimLeadingOneEmpty drops AT MOST ONE blank line at the start of a
// section body: exactly the blank line Render emits after each section
// header (MD022) must not become content on the next parse. Every further
// leading blank line is content and stays verbatim — the free "## Comments"
// section is byte-verbatim (T-0035).
func trimLeadingOneEmpty(lines []string) []string {
	if len(lines) > 0 && lines[0] == "" {
		return lines[1:]
	}
	return lines
}
