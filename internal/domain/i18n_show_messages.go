package domain

// T-0241 `ticket show --section/--json` message helpers. Six strings live
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

// Field getters for the show keys: one closure per dictJSON field, shared by
// the validation rules in i18n_dict_rules.go. They live here rather than in
// that file's shared getter block so the ordered rules table keeps its
// one-file budget, and so a new show key is added next to its message helper.
var (
	getErrRepeatedFlagSection = func(dj dictJSON) string { return dj.ErrRepeatedFlagSection }
	getErrUnknownSection      = func(dj dictJSON) string { return dj.ErrUnknownSection }
	getErrFlagsConflictShow   = func(dj dictJSON) string { return dj.ErrFlagsConflictShow }
	getErrShowExtraArgsModes  = func(dj dictJSON) string { return dj.ErrShowExtraArgsModes }
	getErrJSONEncodeShow      = func(dj dictJSON) string { return dj.ErrJSONEncodeShow }
	getErrJSONShortWriteShow  = func(dj dictJSON) string { return dj.ErrJSONShortWriteShow }
)

// ErrRepeatedFlagSection returns the localized rejection for a second
// --section flag. It is a message of its own (not the --json one): a user who
// repeated --section must not be told about --json.
func ErrRepeatedFlagSection(lang Lang) string {
	return getDict(lang).errRepeatedFlagSection
}

// ErrUnknownSection returns the localized rejection for an unrecognized
// --section value. names is the canonical token list (CanonicalSectionNames)
// and aliases the accepted non-canonical ones (SectionAliasNames) — both
// passed in, never hard-coded, so the message cannot drift from what
// ParseSectionName accepts.
func ErrUnknownSection(lang Lang, name string, names, aliases []string) string {
	return fmt.Sprintf(getDict(lang).errUnknownSection,
		name, strings.Join(names, ", "), strings.Join(aliases, ", "))
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

// ErrJSONEncodeShow returns the localized report for a JSON document that could
// not be marshalled. Unreachable for the current DTO, but the write path must
// still speak RU/EN instead of a hard-coded English line.
func ErrJSONEncodeShow(lang Lang, err error) string {
	return fmt.Sprintf(getDict(lang).errJSONEncodeShow, err)
}

// ErrJSONShortWriteShow returns the localized report for a short write of the
// JSON document: written bytes of expected ones. It is a real error, never
// patched up with a second write.
func ErrJSONShortWriteShow(lang Lang, written, expected int) string {
	return fmt.Sprintf(getDict(lang).errJSONShortWriteShow, written, expected)
}
