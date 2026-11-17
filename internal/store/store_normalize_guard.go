package store

// Normalize guard (T-0125): the deliberately conservative render check and
// its warning-error carriers. Split out of store_normalize.go to keep every
// file inside the 300-line budget (T-0189 W3 file-budget split; no behavior
// change).

import (
	"bytes"
	"fmt"
	"strings"

	"ticket/internal/domain"
)

// Warning errors behind the normalize reasons: the two guard skips (the
// file is left byte-identical) and the post-rename restore failure. They
// carry the user-facing sentence only; the reason code names the condition.
var (
	errUnrecognizedMeta = &parseErr{msg: "bash-era metadata labels, file left unchanged"}
	errIncompleteMeta   = &parseErr{msg: "incomplete metadata, file left unchanged"}
	// errRestoreAttrs labels a post-rename restore failure: the bytes ARE
	// committed, so it must never read like the read-only skip above.
	errRestoreAttrs = &parseErr{msg: "file rewritten, but its attributes could not be restored"}
)

// legacyMetaLabels are the bash-era metadata label names (docs_api/
// reference-init-tickets.md:107-110). matchMetaLine knows only the dict form
// ("- Status (Статус): open"), so these lines are never captured as head
// metadata. Since T-0166 Render can in fact reproduce such a line via
// Ticket.HeadStray; the guard below stays deliberately conservative:
// bash-era files are handed to a human instead of being rewritten, and
// loosening the guard is out of scope for T-0166.
var legacyMetaLabels = []string{"Статус", "Приоритет", "Создан", "Проект"}

// normalizeRenderable reports why raw must not be rewritten, or "" when a
// Render round-trip is safe. Two independent checks, both on the raw bytes
// and both before any content decision:
//
//  1. a bash-era label line anywhere in the head region (the H1 line and the
//     metadata block, up to the first "## " header — the ParseHeader/Parse
//     secHead boundary). The post-T-0166 renderer reproduces the line via
//     Ticket.HeadStray, yet the guard intentionally remains conservative:
//     a bash-era file signals a human-edited or pre-tool origin, so it is
//     skipped for a human to review rather than rewritten silently;
//  2. an empty required field means the render would invent a value or lose
//     one, so the file is left for a human.
func normalizeRenderable(tk *domain.Ticket, raw []byte) string {
	if hasLegacyMetaLabels(raw) {
		return reasonUnrecognizedMeta
	}
	if tk.Type == "" || tk.Priority == "" || tk.Project == "" ||
		strings.TrimSpace(tk.Title) == "" || tk.Created.IsZero() {
		return reasonIncompleteMeta
	}
	return ""
}

// hasLegacyMetaLabels scans the head region of raw for a legacy label line.
// CRLF files are handled by trimming the CR of every line, exactly as the
// parser does, so a converted file cannot slip past the guard.
func hasLegacyMetaLabels(raw []byte) bool {
	for line := range bytes.SplitSeq(raw, []byte("\n")) {
		line = bytes.TrimSuffix(line, []byte("\r"))
		if bytes.HasPrefix(line, []byte("## ")) {
			return false
		}
		if isLegacyMetaLine(line) {
			return true
		}
	}
	return false
}

// isLegacyMetaLine reports whether line is a bash-era label bullet. The
// marker may be "-", "*" or "+", it may be indented, and a space may sit
// before the colon ("  - Статус : open"): all three are label lines a
// hand-edited or pre-tool file can carry. Since T-0166, Render can
// reproduce such lines via Ticket.HeadStray; the guard stays deliberately
// conservative anyway (see normalizeRenderable). The label must be followed by the colon (optionally after spaces and
// tabs) so an ordinary word that merely starts with "Статус" is not matched.
func isLegacyMetaLine(line []byte) bool {
	line = bytes.TrimLeft(line, " \t")
	if len(line) == 0 || (line[0] != '-' && line[0] != '*' && line[0] != '+') {
		return false
	}
	line = bytes.TrimLeft(line[1:], " \t")
	for _, label := range legacyMetaLabels {
		if !bytes.HasPrefix(line, []byte(label)) {
			continue
		}
		rest := bytes.TrimLeft(line[len(label):], " \t")
		if len(rest) > 0 && rest[0] == ':' {
			return true
		}
	}
	return false
}

// normalizeGuardErr maps a guard reason to the warning error that carries it.
// Only the two reasons normalizeRenderable returns are known here, and an
// unknown one is refused by name instead of being reported as another
// guard's condition (that would hide a producer bug behind a wrong message).
func normalizeGuardErr(reason string) error {
	switch reason {
	case reasonUnrecognizedMeta:
		return errUnrecognizedMeta
	case reasonIncompleteMeta:
		return errIncompleteMeta
	default:
		return fmt.Errorf("store: unknown normalize guard reason %q", reason)
	}
}
