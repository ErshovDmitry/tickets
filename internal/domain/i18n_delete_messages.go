package domain

// T-0189 `ticket delete` message helpers. The seven delete strings live on
// the shared dict (i18n_types.go): loadDict unmarshals them from the same
// embedded langs/*.json documents as the rest of the dictionary and
// validateDictJSON checks them (placeholders, non-empty) through the rules
// in i18n_dict_rules.go. Runtime access goes through getDict, so RU/EN
// rendering and the EN fallback for unknown languages mirror every other
// message — no separate registry, reparse or fallback ladder here. Wording
// follows the recover refusals (cmd_recover.go recoverStrings).

import "fmt"

// ErrDeleteOnlyOpen returns the localized guard refusal for a delete
// without --force on a ticket that is not an open main-directory ticket
// (T-0189): without force only main/open is deletable. status is the
// filename-derived status, mirroring ErrArchiveOnlyDoneClosed.
func ErrDeleteOnlyOpen(lang Lang, status string) string {
	return fmt.Sprintf(getDict(lang).errDeleteOnlyOpen, status)
}

// ErrDeleteAlreadyDeleted returns the localized refusal for deleting a
// ticket that already lives in tickets/deleted/ (T-0189): delete is
// terminal, a repeat is an error, never success-idempotent.
func ErrDeleteAlreadyDeleted(lang Lang) string {
	return getDict(lang).errDeleteAlreadyDeleted
}

// ErrDeleteInvalidRef returns the localized rejection for a delete ref
// that is neither a ticket number nor an exact ticket base name (T-0189).
func ErrDeleteInvalidRef(lang Lang, ref string) string {
	return fmt.Sprintf(getDict(lang).errDeleteInvalidRef, ref)
}

// DeletePrompt formats the confirmation line naming the absolute candidate
// path about to be deleted (T-0189), printed before the yes/no ask.
func DeletePrompt(lang Lang, path string) string {
	return fmt.Sprintf(getDict(lang).deletePromptFmt, path)
}

// DeleteAsk returns the localized yes/no confirmation prompt (T-0189).
func DeleteAsk(lang Lang) string {
	return getDict(lang).deleteAsk
}

// DeleteRefused returns the localized refusal for a non-terminal stdin
// without --yes (T-0189): nothing is changed, re-run with --yes.
func DeleteRefused(lang Lang) string {
	return getDict(lang).deleteRefused
}

// DeleteCancelled returns the localized notice for a declined/EOF/invalid
// confirmation answer (T-0189): nothing was changed.
func DeleteCancelled(lang Lang) string {
	return getDict(lang).deleteCancelled
}
