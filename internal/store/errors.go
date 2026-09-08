package store

import "fmt"

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
