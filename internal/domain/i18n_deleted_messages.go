package domain

// T-0189 W3: tickets/deleted/ is terminal and read-only - the set/edit
// guard (internal/cli/cmd_deleted_guard.go) refuses any status change,
// reopen or edit targeted at a ticket whose file lives in deleted/. The
// string rides the shared dict like the T-0189 delete keys: loadDict
// unmarshals it from the same embedded langs/*.json documents,
// validateDictJSON checks it (placeholder count, non-empty) through
// i18n_dict_rules.go, and runtime access goes through getDict, so RU/EN
// rendering and the EN fallback mirror every other message.

import "fmt"

// ErrTicketDeletedReadOnly returns the localized refusal for a set/edit/
// reopen aimed at ticket n whose file lives in tickets/deleted/ (T-0189):
// the ticket is terminal, no undelete exists, the file is unchanged.
func ErrTicketDeletedReadOnly(lang Lang, n int) string {
	return fmt.Sprintf(getDict(lang).errTicketDeletedReadOnly, n)
}
