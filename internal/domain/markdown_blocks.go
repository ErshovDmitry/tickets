package domain

import "strings"

// T-0230 / GitHub #56: MD022/MD032 blank-line spacing, NEW-ticket Details only
// (newData after normalizeDetails). Plan v3: blank lines only, bytes/EOLs kept,
// no payload-edge/double blanks, idempotent, conservative CommonMark edges.

// blockKind classifies one Details line for blank-line insertion.
type blockKind int

const (
	kindText     blockKind = iota // plain paragraph line (or lazy continuation)
	kindBlank                     // whitespace-only line
	kindHeading                   // ATX heading, 1-6 '#' after <=3 spaces
	kindList                      // column-0 list marker + whitespace
	kindThematic                  // thematic break (- - -, ***, ---, ...)
	kindFence                     // fenced-code delimiter line
	kindIndented                  // indented code outside a list (>=4 spaces/tab)
	kindOpaque                    // fence content or HTML block
)

// active: blank/opaque lines never neighbour an inserted blank; indented code
// only as the left neighbour of a heading/list demand (emission loop).
func (k blockKind) active() bool { return k != kindBlank && k != kindOpaque && k != kindIndented }

// detailLine keeps the original bytes (raw, including the EOL) alongside the
// classification text (raw minus the trailing LF/CRLF). above/below record a
// blank-line demand at the boundary before/after the line.
type detailLine struct {
	raw          string
	text         string
	kind         blockKind
	above, below bool
}

// HTML block shapes recognized conservatively.
const (
	htmlNone    = 0
	htmlComment = 1 // <!-- ... --> : opaque until the closing -->
	htmlTag     = 2 // <tag> ...    : opaque until a blank line
)

// blockScanner carries the fence/HTML state across lines.
type blockScanner struct {
	inFence   bool
	fenceChar byte
	fenceLen  int
	html      int
}

// spaceDetailBlocks returns details with MD022/MD032 blank lines inserted.
// details is expected to carry the normalizeDetails trailing LF; empty and
// whitespace-only payloads come back unchanged.
func spaceDetailBlocks(details string) string {
	chunks := strings.SplitAfter(details, "\n")
	lines := make([]detailLine, 0, len(chunks))
	var s blockScanner
	for _, chunk := range chunks {
		if chunk == "" { // trailing element after the final LF
			continue
		}
		text := strings.TrimSuffix(strings.TrimSuffix(chunk, "\n"), "\r")
		lines = append(lines, detailLine{raw: chunk, text: text, kind: s.classify(text)})
	}
	markBoundaries(lines)
	var b strings.Builder
	b.Grow(len(details))
	for i := range lines {
		if i > 0 && (lines[i-1].below || lines[i].above) && lines[i].kind.active() &&
			(lines[i-1].kind.active() || (lines[i-1].kind == kindIndented && lines[i].above)) {
			b.WriteString(blankEOL(lines[i-1].raw))
		}
		b.WriteString(lines[i].raw)
	}
	return b.String()
}

// blankEOL matches the inserted blank line to the preceding line's EOL so a
// CRLF payload stays CRLF.
func blankEOL(prevRaw string) string {
	if strings.HasSuffix(prevRaw, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

// classify assigns a kind to one line, advancing fence/HTML state.
func (s *blockScanner) classify(text string) blockKind {
	rest, spaces := trimSpaces(text)
	if s.inFence {
		if c, n := fenceRun(rest); c == s.fenceChar && n >= s.fenceLen &&
			spaces <= 3 && strings.Trim(rest[n:], " \t") == "" {
			s.inFence = false
			return kindFence // closing delimiter stays an active neighbour
		}
		return kindOpaque
	}
	if s.html == htmlComment {
		if strings.Contains(text, "-->") {
			s.html = htmlNone
		}
		return kindOpaque
	}
	if s.html == htmlTag {
		if strings.TrimSpace(text) == "" {
			s.html = htmlNone
			return kindBlank
		}
		return kindOpaque
	}
	if strings.TrimSpace(text) == "" {
		return kindBlank
	}
	if strings.HasPrefix(text, "\t") || spaces >= 4 {
		return kindIndented
	}
	if c, n := fenceRun(rest); c != 0 && (c == '~' || !strings.Contains(rest[n:], "`")) {
		s.inFence, s.fenceChar, s.fenceLen = true, c, n
		return kindFence
	}
	if k, ok := htmlOpen(rest); ok {
		s.html = k
		if k == htmlComment && (strings.Contains(rest[4:], "-->") || rest == "<!-->" || rest == "<!--->") {
			s.html = htmlNone // single-line comment, incl. abrupt closers
		}
		return kindOpaque
	}
	if thematicBreak(rest) {
		return kindThematic
	}
	if atxHeading(rest) {
		return kindHeading
	}
	if _, ok := listMarker(text); spaces == 0 && ok {
		return kindList
	}
	return kindText
}

// markBoundaries records blank-line demands: headings want both sides, a list
// block only its outer edges; ordered start != 1 never interrupts a paragraph
// or its indented continuation (CommonMark). A blank ends the block unless an
// item/indented content follows; indented lines in an open list are opaque.
func markBoundaries(lines []detailLine) {
	inList, paraOpen := false, false
	for i := range lines {
		ln := &lines[i]
		if inList && ln.indentedContent() {
			ln.kind = kindOpaque
			continue
		}
		switch ln.kind {
		case kindHeading:
			ln.above, ln.below = true, true
			inList = false
		case kindList:
			if start, _ := listMarker(ln.text); !inList && paraOpen && start != 1 {
				ln.kind = kindText // CommonMark: only start=1 interrupts a paragraph
			} else if !inList {
				ln.above = true
				inList = true
			}
		case kindFence, kindThematic:
			if inList && i > 0 {
				lines[i-1].below = true // MD032 below the finished list
			}
			inList = false
		case kindBlank:
			if inList && !listContinuesAfter(lines, i) {
				inList = false
			}
		case kindText, kindOpaque, kindIndented:
			// continuation of the open list (or plain text): no demand
		}
		paraOpen = ln.kind == kindText || (ln.kind == kindIndented && paraOpen) // paragraph continuation
	}
}

// indentedContent reports a non-blank line starting with whitespace.
func (ln detailLine) indentedContent() bool {
	return ln.kind != kindBlank && ln.text != "" &&
		(ln.text[0] == ' ' || ln.text[0] == '\t')
}

// listContinuesAfter reports whether the first non-blank line after the blank
// at index i keeps the list block alive (sibling item or indented content).
func listContinuesAfter(lines []detailLine, i int) bool {
	for j := i + 1; j < len(lines); j++ {
		if lines[j].kind == kindBlank {
			continue
		}
		if lines[j].kind == kindList {
			return true
		}
		t := lines[j].text
		return t != "" && (t[0] == ' ' || t[0] == '\t')
	}
	return false
}

// trimSpaces strips leading ASCII spaces only (tabs are not expanded).
func trimSpaces(text string) (rest string, spaces int) {
	for spaces < len(text) && text[spaces] == ' ' {
		spaces++
	}
	return text[spaces:], spaces
}

// atxHeading recognizes 1-6 '#' followed by whitespace or end of line;
// "##hashtag" is not a heading.
func atxHeading(rest string) bool {
	n := 0
	for n < len(rest) && rest[n] == '#' {
		n++
	}
	if n == 0 || n > 6 {
		return false
	}
	return n == len(rest) || rest[n] == ' ' || rest[n] == '\t'
}

// listMarker recognizes a column-0 unordered marker or a 1-9 digit ordered
// marker + space/tab + non-whitespace content (start number; unordered: 1 —
// they always interrupt). Marker-only/empty lines ("-", "- ", "1.") stay text.
func listMarker(text string) (int, bool) {
	if text == "" {
		return 0, false
	}
	if text[0] == '-' || text[0] == '+' || text[0] == '*' {
		return 1, len(text) > 1 && (text[1] == ' ' || text[1] == '\t') && strings.TrimSpace(text[2:]) != ""
	}
	i, n := 0, 0
	for i < len(text) && text[i] >= '0' && text[i] <= '9' {
		n = n*10 + int(text[i]-'0')
		i++
	}
	if i == 0 || i > 9 || i+1 >= len(text) {
		return 0, false
	}
	if (text[i] == '.' || text[i] == ')') && (text[i+1] == ' ' || text[i+1] == '\t') {
		return n, strings.TrimSpace(text[i+2:]) != ""
	}
	return 0, false
}

// thematicBreak reports a line of >=3 identical '-', '*' or '_' characters
// with only spaces/tabs between them. Checked before listMarker so "- - -"
// is never a list.
func thematicBreak(rest string) bool {
	var c byte
	count := 0
	for i := 0; i < len(rest); i++ {
		ch := rest[i]
		if ch == ' ' || ch == '\t' {
			continue
		}
		if ch != '-' && ch != '*' && ch != '_' {
			return false
		}
		if c == 0 {
			c = ch
		} else if ch != c {
			return false
		}
		count++
	}
	return count >= 3
}

// fenceRun returns the fence character and run length (>=3) or 0,0.
func fenceRun(rest string) (byte, int) {
	if rest == "" || (rest[0] != '`' && rest[0] != '~') {
		return 0, 0
	}
	n := 0
	for n < len(rest) && rest[n] == rest[0] {
		n++
	}
	if n < 3 {
		return 0, 0
	}
	return rest[0], n
}

// htmlOpen recognizes an HTML comment opener or a tag-like opener. Tag blocks
// are conservatively opaque until a blank line (CommonMark types 6/7 and a
// safe superset of the rest); no full HTML parser is attempted.
func htmlOpen(rest string) (int, bool) {
	if strings.HasPrefix(rest, "<!--") {
		return htmlComment, true
	}
	if len(rest) >= 2 && rest[0] == '<' {
		c := rest[1]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '/' || c == '!' || c == '?' {
			return htmlTag, true
		}
	}
	return htmlNone, false
}
