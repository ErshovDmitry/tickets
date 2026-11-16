package domain

// T-0187 `ticket edit` message helpers. The four edit strings live on the
// shared dict (i18n_types.go): loadDict unmarshals them from the same
// embedded langs/*.json documents as the rest of the dictionary and
// validateDictJSON checks them (placeholders, non-empty) through the rules
// in i18n_dict_rules.go. Runtime access goes through getDict, so RU/EN
// rendering and the EN fallback for unknown languages mirror every other
// message — no separate registry, reparse or fallback ladder here.

import "fmt"

// ErrEditUnknownFlag returns the localized rejection for a flag argument
// that is not one of edit's four long flags (T-0187), including the
// refused `--flag=value` glued forms and any short flag.
func ErrEditUnknownFlag(lang Lang, flagArg string) string {
	return fmt.Sprintf(getDict(lang).errEditUnknownFlag, flagArg)
}

// ErrEditFieldNewline returns the localized rejection for an edited
// title/project value carrying CR or LF (T-0187): the journal is
// line-oriented, and an embedded newline would forge a journal entry.
// Unlike `set`'s ErrJournalNewline (comment/who only), this one names
// the edited metadata fields.
func ErrEditFieldNewline(lang Lang) string {
	return getDict(lang).errEditFieldNewline
}

// ErrEditAlreadyField formats the localized refusal for an edit that
// explicitly passes the value the ticket already has (T-0187): one
// unchanged field rejects the whole edit, mirroring the same-status
// `set` rule (ErrTicketAlreadyStatus precedent).
func ErrEditAlreadyField(lang Lang, number int, field, value string) string {
	return fmt.Sprintf(getDict(lang).errEditAlreadyField, number, field, value)
}

// ErrProjectEmpty returns the localized rejection for a whitespace-only
// --project value (T-0187), matching the title's ErrTitleEmpty rule.
func ErrProjectEmpty(lang Lang) string {
	return getDict(lang).errProjectEmpty
}
