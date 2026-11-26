// Package domain holds the ticket model and its markdown/filename codecs.
// The layout is byte-compatible with the bash-era (pre-Go) implementation
// except for the "## Комментарии от пользователя" and "## Комментарии"
// sections (T-0032, T-0035): the Go implementation emits them on every
// ticket it renders, while the bash-era layout has no such sections —
// byte-compat of `new` is intentionally diverged for new tickets.
//
// The pre-Go bash implementation is not part of this repository: the
// upstream prompt file was replaced by this Go binary. Mentions of the bash
// era are provenance markers for the on-disk format, not a specification:
// the bash source is not part of the repo (the copy under docs_api/ is a
// different, stale revision) and must not be
// consulted. The authoritative definitions are this package (file format),
// internal/cli (UX) and the golden fixtures in internal/cli/testdata.
package domain

import (
	"fmt"
	"strings"
	"time"
)

// Mode selects how strict Ticket validation is.
type Mode uint8

const (
	// Tolerant accepts anything — used when reading existing files.
	Tolerant Mode = iota
	// Strict enforces all new-ticket invariants.
	Strict
)

// Ticket is one ticket file. Fields mirror the markdown layout inherited
// from the bash era.
type Ticket struct {
	Number   int
	Status   Status
	Type     Type
	Priority Priority
	Title    string
	Details  string
	// "## Комментарии от пользователя" user remarks; empty renders the stub.
	// The agent reads it before working on the ticket and never writes here.
	UserComments string
	// "## Комментарии" free-form notes (agent working notes etc.); empty
	// renders a bare header.
	Comments string
	Who      string
	Project  string
	Created  time.Time
	Journal  []JournalEntry
	// Unknown holds manual bytes after the journal section, preserved verbatim.
	Unknown []byte
	// HeadStray holds unrecognized head lines (any line in the head zone
	// that is neither H1/meta nor a recognized section header) in their
	// original order; Render anchors them after the metadata block, right
	// before the Summary header (T-0166).
	HeadStray []string
	// Brief holds the "## Summary" body lines verbatim (T-0166): before
	// T-0166 the body duplicated Title and was discarded. Empty means the
	// Summary body was empty and Render falls back to Title.
	Brief      []string
	JournalDup bool // set by Parse when a second "## Journal" section merges into the canonical journal

	// Lang is the detected language of the ticket file (LOCK D4). It affects
	// only: emission of new journal lines, stub choice for empty sections,
	// default forms for absent components. It does NOT affect raw templates.
	Lang Lang

	// RawTemplates stores the original lines recognized during Parse
	// (LOCK D1). Render prefers these over dict emission, guaranteeing
	// byte-stability for free suffixes like "## Summary (任意语言)". Zero/absent
	// entries mean "not recognized" -> Render emits dict form.
	RawTemplates rawTemplates
}

// rawTemplates holds original lines from the parsed file.
type rawTemplates struct {
	// Section headers: original full line for each recognized section, keyed
	// by canonical section name.
	Headers map[sectionName]string

	// Meta forms: original lines split into literal segments around their
	// value slots (T-0036). No printf placeholders are stored, so a literal
	// "%s" in a user free suffix or value survives round trips byte-
	// identically.
	Meta map[string]metaSegments

	// Bodies: per-section raw body sidecar (T-0245). Zero/absent entries
	// mean "not captured" (empty or stub body) -> Render emits the public
	// value / dict default.
	Bodies map[sectionName]rawBody
}

// metaSegments is one meta line as literal text around its value slots.
// Single-slot metas (Status/Priority/Project) render Prefix+value+Suffix
// (Middle is empty); the two-slot Created renders
// Prefix+timestamp+Middle+author+Suffix.
type metaSegments struct {
	Prefix string // literal text before the first value slot
	Middle string // Created only: literal text between ts and author slots
	Suffix string // literal text after the last value slot
}

// rawBody pairs one section's original body bytes (CR preserved, T-0245)
// with the normalized semantic value it was captured with. Render re-emits
// raw only while the public field still equals snapshot; any other edit
// falls back to the current public value.
type rawBody struct {
	raw      string
	snapshot string
}

// JournalEntry is one line of the "## Журнал" section. The creation entry has
// From == To == StatusOpen and an empty Comment. Archived entries have
// Archived=true and empty From/To/Comment: archival is a directory move, not a
// status transition.
type JournalEntry struct {
	At       time.Time
	From     Status
	To       Status
	Comment  string
	Who      string
	Archived bool

	// Raw is the original line text for entries recognized during Parse
	// (plan §B invariant). Non-empty Raw re-emits verbatim regardless of
	// t.Lang, so a journal line in a foreign dict form (e.g. RU line in a
	// file detected as EN) survives round trips byte-identically. Empty Raw
	// (entries appended by set/archive) emits the t.Lang dict form.
	Raw string
}

// Validate checks the ticket. In Strict mode (new tickets) it requires a
// positive Number, valid Status/Type/Priority and a non-empty Title;
// Details and Who may be empty. Tolerant mode always succeeds.
func (t Ticket) Validate(mode Mode) error {
	if mode == Tolerant {
		return nil
	}
	if t.Number <= 0 {
		return fmt.Errorf("номер тикета должен быть > 0")
	}
	if _, err := ParseStatus(string(t.Status)); err != nil {
		return err
	}
	if _, err := ParseType(string(t.Type)); err != nil {
		return err
	}
	if _, err := ParsePriority(string(t.Priority)); err != nil {
		return err
	}
	if strings.TrimSpace(t.Title) == "" {
		return fmt.Errorf("краткое описание не может быть пустым")
	}
	return nil
}

// AppendCommentPayload appends payload as a new block to Comments
// (T-0245): the old block keeps its raw bytes (CRLF included), exactly one
// LF blank line separates the blocks, and the payload enters the sidecar
// verbatim — never normalized. The public field and snapshot receive the
// normalized equivalent. The caller owns payload validation (trailing
// CR/LF trim, ATX guard, non-empty check).
func (t *Ticket) AppendCommentPayload(payload string) {
	oldRaw := t.bodyRawOr(secNameComments, t.Comments)
	t.Comments = appendBlock(t.Comments, normalizeBody(payload))
	t.setBodyRaw(secNameComments, appendBlock(oldRaw, payload), t.Comments)
}

// SetEditablePayload replaces one editable section's body with payload
// (T-0245): raw keeps the payload bytes verbatim, the public field gets
// the parser-normalized equivalent, and the snapshot binds the two. Only
// the target section changes. An empty payload clears the section and its
// sidecar. The caller owns payload validation and the editability gate.
func (t *Ticket) SetEditablePayload(sec Section, payload string) error {
	norm := normalizeBody(payload)
	switch sec {
	case SectionSummary:
		// Brief is line-oriented; an empty payload yields nil so Render
		// falls back to the title.
		t.Brief = nil
		if norm != "" {
			t.Brief = strings.Split(norm, "\n")
		}
	case SectionDetails:
		t.Details = norm
	case SectionComments:
		t.Comments = norm
	default:
		return fmt.Errorf("domain: section %q has no field mapping", sec)
	}
	t.setBodyRaw(sec, payload, norm)
	return nil
}

// DropRawBodies clears the raw-body sidecar so Render emits the normalized
// semantic view. Canonicalization paths (normalize) call it: their mandate
// is the canonical LF form, not byte preservation (T-0245). It never
// touches the public fields, so SectionBody/show are unaffected.
func (t *Ticket) DropRawBodies() {
	t.RawTemplates.Bodies = nil
}

// bodyRawOr returns the preserved raw body while the public value still
// equals the captured snapshot; after any other edit the current public
// value is the only truth (T-0245 fallback).
func (t *Ticket) bodyRawOr(name sectionName, current string) string {
	if rb, ok := t.RawTemplates.Bodies[name]; ok && rb.snapshot == current {
		return rb.raw
	}
	return current
}

// setBodyRaw binds raw to snapshot; an empty raw or snapshot removes the
// entry so Render never resurrects a placeholder or a stale body.
func (t *Ticket) setBodyRaw(name sectionName, raw, snapshot string) {
	if raw == "" || snapshot == "" {
		delete(t.RawTemplates.Bodies, name)
		return
	}
	if t.RawTemplates.Bodies == nil {
		t.RawTemplates.Bodies = make(map[sectionName]rawBody)
	}
	t.RawTemplates.Bodies[name] = rawBody{raw: raw, snapshot: snapshot}
}

// normalizeBody mirrors Parse's per-line trimEOL on a body string: each
// LF-delimited line loses one trailing CR, nothing else changes. It is
// the semantic (public-field) equivalent of a raw payload.
func normalizeBody(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSuffix(line, "\r")
	}
	return strings.Join(lines, "\n")
}

// appendBlock joins old and payload with exactly one LF blank line; an
// empty old body takes payload alone (the DS7 separator, T-0240).
func appendBlock(old, payload string) string {
	if old == "" {
		return payload
	}
	return old + "\n\n" + payload
}
