package store

// T-0240 A1: the text write path — AddComment appends a block to the
// Comments section, SetSection replaces the body of an editable section.
// Both commit exclusively through rewriteInPlaceLocked (the single
// re-render commit seam, DS8): per-store lock, tmp+rename, one one-line
// journal event. Payload/argument errors are returned BEFORE the lookup,
// so a refused write leaves the file and journal byte-identical (DS9).

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"ticket/internal/domain"
)

// ErrEmptyCommentPayload refuses AddComment with an empty payload: a
// comment appends a block, and an empty block would only churn separators.
// SetSection accepts an empty payload — it clears the section (T-0240).
var ErrEmptyCommentPayload = errors.New("store: comment: empty payload")

// PayloadHeadingError refuses a payload carrying an ATX heading line
// (^#{1,6}(\s|$)): on re-parse the line would read as a section header and
// the text would escape its section (T-0240 DS6). Line is the 1-based
// offending line number within the payload. Ordinary lines merely
// containing '#' are not headings and pass. The CLI renders the localized
// message from Line; the store carries no UI text.
type PayloadHeadingError struct {
	Line int
}

// Error renders the internal (non-UI) description of the rejection.
func (e *PayloadHeadingError) Error() string {
	return fmt.Sprintf("store: payload line %d is a markdown heading", e.Line)
}

// SectionNotEditableError refuses SetSection on a protected (user comments,
// journal) or unknown section (T-0240 DS2). Accepted carries the allowlist
// from domain.EditableSections() so the refusal lists the editable names
// without hard-coding them — never CanonicalSectionNames/SectionAliasNames.
type SectionNotEditableError struct {
	Number   int
	Section  domain.Section
	Accepted []domain.Section
}

// Error renders the internal (non-UI) description of the rejection.
func (e *SectionNotEditableError) Error() string {
	names := make([]string, len(e.Accepted))
	for i, sec := range e.Accepted {
		names[i] = string(sec)
	}
	return fmt.Sprintf("store: ticket %d: section %q is not editable (accepted: %s)",
		e.Number, e.Section, strings.Join(names, ", "))
}

// payloadATX is the DS6 heading guard: an ATX heading is 1-6 '#' followed
// by whitespace or end of line. Anchored per line — leading spaces are
// content, and a '#' inside a line never matches.
var payloadATX = regexp.MustCompile(`^#{1,6}(\s|$)`)

// AddComment appends text as a new block to the Comments section of ticket
// n — never a replacement (T-0240 DS1). Exactly one blank line separates
// the old body from the new block; an empty old body takes the payload
// alone. The payload keeps its interior bytes verbatim (including CRLF);
// only trailing LFs are trimmed (DS7, normalizeDetails precedent). One
// one-line journal event records the append. The flow mirrors Edit:
// withLock → journal-safe who → payload validation → findOneLocked →
// rewriteInPlaceLocked; a read-only Store is rejected by withLock with
// ErrReadOnly, an ambiguous number is refused before any read. Success
// returns the parse warnings of the rewritten file; errors return nil
// warnings and leave the file byte-identical.
func (s *Store) AddComment(n int, text string, who string) ([]ParseWarning, error) {
	var warnings []ParseWarning
	err := s.withLock(func() error {
		var serr error
		warnings, serr = s.addCommentLocked(n, text, who)
		return serr
	})
	if err != nil {
		return nil, err
	}
	return warnings, nil
}

// SetSection replaces the body of an editable section of ticket n with
// text (T-0240 DS1): summary → Ticket.Brief, details → Ticket.Details,
// comments → Ticket.Comments. Protected and unknown sections are refused
// with *SectionNotEditableError listing domain.EditableSections(). An
// empty payload clears the section (an empty Brief renders as the title —
// Render behavior). Payload policy and commit path are AddComment's; the
// journal event names the replaced section.
func (s *Store) SetSection(n int, sec domain.Section, text string, who string) ([]ParseWarning, error) {
	var warnings []ParseWarning
	err := s.withLock(func() error {
		var serr error
		warnings, serr = s.setSectionLocked(n, sec, text, who)
		return serr
	})
	if err != nil {
		return nil, err
	}
	return warnings, nil
}

// addCommentLocked assumes the per-store advisory lock is held.
func (s *Store) addCommentLocked(n int, text, who string) ([]ParseWarning, error) {
	if err := validateJournalInput("", who); err != nil {
		return nil, err
	}
	payload, err := validateWritePayload(text)
	if err != nil {
		return nil, err
	}
	if payload == "" {
		return nil, ErrEmptyCommentPayload
	}
	cur, err := s.findOneLocked(n, "store.comment.failure")
	if err != nil {
		return nil, err
	}
	_, warnings, err := s.rewriteInPlaceLocked(n, cur.fileEntry, cur.dir, func(tk *domain.Ticket, at time.Time) ([]domain.JournalEntry, error) {
		tk.Comments = appendCommentBlock(tk.Comments, payload)
		return []domain.JournalEntry{{
			From:    tk.Status,
			To:      tk.Status,
			Comment: commentJournalText(tk.Lang, strings.Count(payload, "\n")+1),
			Who:     who,
			At:      at,
		}}, nil
	}, "store.comment")
	return warnings, err
}

// setSectionLocked assumes the per-store advisory lock is held.
func (s *Store) setSectionLocked(n int, sec domain.Section, text, who string) ([]ParseWarning, error) {
	if err := validateJournalInput("", who); err != nil {
		return nil, err
	}
	if !domain.IsEditableSection(sec) {
		return nil, &SectionNotEditableError{Number: n, Section: sec, Accepted: domain.EditableSections()}
	}
	payload, err := validateWritePayload(text)
	if err != nil {
		return nil, err
	}
	cur, err := s.findOneLocked(n, "store.section.failure")
	if err != nil {
		return nil, err
	}
	_, warnings, err := s.rewriteInPlaceLocked(n, cur.fileEntry, cur.dir, func(tk *domain.Ticket, at time.Time) ([]domain.JournalEntry, error) {
		switch sec {
		case domain.SectionSummary:
			// Brief is line-oriented; an empty payload yields nil so
			// Render falls back to the title.
			tk.Brief = nil
			if payload != "" {
				tk.Brief = strings.Split(payload, "\n")
			}
		case domain.SectionDetails:
			tk.Details = payload
		case domain.SectionComments:
			tk.Comments = payload
		default:
			// Unreachable while the switch covers EditableSections();
			// guards a future allowlist extension against a silent no-op.
			return nil, fmt.Errorf("store: section %q has no field mapping", sec)
		}
		return []domain.JournalEntry{{
			From:    tk.Status,
			To:      tk.Status,
			Comment: sectionJournalText(tk.Lang, sec),
			Who:     who,
			At:      at,
		}}, nil
	}, "store.section")
	return warnings, err
}

// validateWritePayload applies the shared payload policy (T-0240 DS6/DS7):
// trailing-LF trim (interior bytes, including CR, stay verbatim —
// normalizeDetails precedent), then the ATX heading guard with the offending
// line number. The journal-safe who check runs before it in each operation.
func validateWritePayload(text string) (string, error) {
	payload := strings.TrimRight(text, "\n")
	for i, line := range strings.Split(payload, "\n") {
		if payloadATX.MatchString(line) {
			return "", &PayloadHeadingError{Line: i + 1}
		}
	}
	return payload, nil
}

// appendCommentBlock appends payload as a new block: exactly one blank
// line separates it from the old body; an empty old body takes payload
// alone. Both parts carry no trailing LF, so Render's single separator
// keeps the section ending at one LF and re-writes never accumulate
// blanks (DS7 idempotency).
func appendCommentBlock(old, payload string) string {
	if old == "" {
		return payload
	}
	return old + "\n\n" + payload
}

// commentJournalText renders the one-line append event in the ticket's
// language (T-0240 DS8); the RU form is the pinned example, EN is the
// fallback for every other language, mirroring getDict. lines is the
// payload line count.
func commentJournalText(lang domain.Lang, lines int) string {
	if lang == domain.LangRU {
		return fmt.Sprintf("comment: +%d строк", lines)
	}
	return fmt.Sprintf("comment: +%d lines", lines)
}

// sectionJournalText renders the one-line replace event in the ticket's
// language. The section is named by its CLI token: every editable section
// is a single-word identifier, so lower casing is exactly the read-path
// token spelling (domain/sections.go sectionToken).
func sectionJournalText(lang domain.Lang, sec domain.Section) string {
	token := strings.ToLower(string(sec))
	if lang == domain.LangRU {
		return fmt.Sprintf("section %s: заменено", token)
	}
	return fmt.Sprintf("section %s: replaced", token)
}
