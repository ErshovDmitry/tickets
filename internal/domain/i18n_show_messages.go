package domain

// T-0241 `ticket show --section/--json` message helpers. The four strings live
// on the shared dict (i18n_types.go): loadDict unmarshals them from the same
// embedded langs/*.json documents as the rest of the dictionary and
// validateDictJSON checks them (placeholder counts, non-empty) through the
// rules in i18n_dict_rules.go. Runtime access goes through getDict, so RU/EN
// rendering and the EN fallback for unknown languages mirror every other
// message — no separate registry or fallback ladder here.

import (
	"fmt"
	"strings"
)

// ErrRepeatedFlagSection returns the localized rejection for a second
// --section flag. It is a message of its own (not the --json one): a user who
// repeated --section must not be told about --json.
func ErrRepeatedFlagSection(lang Lang) string {
	return getDict(lang).errRepeatedFlagSection
}

// ErrUnknownSection returns the localized rejection for an unrecognized
// --section value. names is the canonical token list (A1
// CanonicalSectionNames) — passed in, never hard-coded, so the message cannot
// drift from what ParseSectionName accepts. The two accepted aliases are named
// by the template itself and are deliberately absent from names.
func ErrUnknownSection(lang Lang, name string, names []string) string {
	return fmt.Sprintf(getDict(lang).errUnknownSection, name, strings.Join(names, ", "))
}

// ErrFlagsConflictShow returns the localized rejection for combining --section
// with --json: the modes are mutually exclusive and neither prints the ticket.
func ErrFlagsConflictShow(lang Lang) string {
	return getDict(lang).errFlagsConflictShow
}

// ErrShowExtraArgsModes returns the localized rejection for extra positional
// arguments in the new modes. The plain show path keeps its warning instead
// (WarnExtraArgsIgnored), so this message is never used there.
func ErrShowExtraArgsModes(lang Lang, args []string) string {
	return fmt.Sprintf(getDict(lang).errShowExtraArgsModes, strings.Join(args, " "))
}
