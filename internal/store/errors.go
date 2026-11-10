package store

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidJournalInput is returned by SetStatus / appendSameStatusLocked
// / archiveOneLocked when either the journal comment or the author ("who")
// value contains a CR or LF. The journal is line-oriented (one entry per
// "- <ts> — ..." line), so an embedded newline lets a caller inject a
// forged journal line. T-0065.
//
// Field names the offending input ("comment" or "who"); Value is the
// first 64 bytes of the input, with newlines escaped to "\n" / "\r" so
// the error message stays single-line and machine-parseable. The store
// is the single source of truth: the CLI also pre-validates (defense in
// depth, T-0065 D2) for a localized user-facing message, but the store
// guard is what actually blocks the injection.
type ErrInvalidJournalInput struct {
	Field string
	Value string
}

// Error renders a one-line, non-UI description suitable for logs and
// internal error returns. The CLI translates this to a localized
// message via the i18n layer (see internal/cli/cmd_set.go).
func (e *ErrInvalidJournalInput) Error() string {
	return fmt.Sprintf("store: journal %s must not contain a line break (got %q)", e.Field, e.Value)
}

// ErrAmbiguousNumber: more than one ticket file carries the requested
// number. A commit interrupted between the link and the removal of the
// old file leaves both T-NNNN-<new>.md and T-NNNN-<old>.md behind, so
// the number no longer identifies exactly one ticket (T-0157). Use
// errors.Is to test.
var ErrAmbiguousNumber = errors.New("store: ambiguous ticket number")

// AmbiguousNumberError reports a ticket number owned by more than one
// file. Number is the requested number; Paths are the absolute,
// deduplicated, sorted paths of the FOREIGN conflicting files — a
// caller's own just-created file is never listed. The cli prints every
// path on its own line, so the store carries no UI text.
type AmbiguousNumberError struct {
	Number int
	Paths  []string
}

// Error renders the internal (non-UI) description of the conflict.
func (e *AmbiguousNumberError) Error() string {
	return fmt.Sprintf("store: ticket %d has %d files: %s", e.Number, len(e.Paths), strings.Join(e.Paths, ", "))
}

// Unwrap exposes the ErrAmbiguousNumber sentinel.
func (e *AmbiguousNumberError) Unwrap() error { return ErrAmbiguousNumber }
