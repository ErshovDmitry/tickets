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
// message from Line; the error carries no localized text.
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
// only trailing CRs and LFs are trimmed (DS7). One
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
		// T-0245: the domain method preserves the old block's raw bytes
		// (CRLF included) and keeps the public field parser-normalized.
		tk.AppendCommentPayload(payload)
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
		// T-0245: raw target + normalized public field + snapshot update
		// atomically; only the selected section changes. The default
		// branch guards a future allowlist extension against a silent no-op.
		if err := tk.SetEditablePayload(sec, payload); err != nil {
			return nil, err
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
// trailing-CRLF trim (interior bytes, including CR, stay verbatim), then the
// ATX heading guard with the offending line number. The journal-safe who check runs before it in each operation.
func validateWritePayload(text string) (string, error) {
	payload := strings.TrimRight(text, "\r\n")
	for i, line := range strings.Split(payload, "\n") {
		if payloadATX.MatchString(line) {
			return "", &PayloadHeadingError{Line: i + 1}
		}
	}
	return payload, nil
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
// language. The section is named by its CLI token (domain.SectionToken),
// the same spelling the read path uses.
func sectionJournalText(lang domain.Lang, sec domain.Section) string {
	token := domain.SectionToken(sec)
	if lang == domain.LangRU {
		return fmt.Sprintf("section %s: заменено", token)
	}
	return fmt.Sprintf("section %s: replaced", token)
}
