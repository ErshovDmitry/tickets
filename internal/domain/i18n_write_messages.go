package domain

// T-0240 A2: message helpers for the text write path (ticket comment /
// ticket section set). The strings live on the shared dict (i18n_types.go):
// loadDict unmarshals them from the same embedded langs/*.json documents as
// the rest of the dictionary and validateDictJSON checks them (placeholder
// counts, non-empty) through writeDictRules below. Runtime access goes
// through getDict, so RU/EN rendering and the EN fallback for unknown
// languages mirror every other message — no separate registry or fallback
// ladder here.

import (
	"fmt"
	"strings"
)

// Field getters for the write keys: one closure per dictJSON field, shared
// by writeDictRules. They live here rather than in the i18n_dict_rules.go
// getter block so the ordered rules table keeps its one-file budget, and so
// a new write key is added next to its message helper (show precedent).
var (
	getErrWriteOneSource          = func(dj dictJSON) string { return dj.ErrWriteOneSource }
	getErrWriteSchemaConflict     = func(dj dictJSON) string { return dj.ErrWriteSchemaConflict }
	getErrWriteUnknownFlag        = func(dj dictJSON) string { return dj.ErrWriteUnknownFlag }
	getErrWriteFileRead           = func(dj dictJSON) string { return dj.ErrWriteFileRead }
	getErrWriteStdinRead          = func(dj dictJSON) string { return dj.ErrWriteStdinRead }
	getErrWriteJSONInvalid        = func(dj dictJSON) string { return dj.ErrWriteJSONInvalid }
	getErrWriteJSONUnknownFields  = func(dj dictJSON) string { return dj.ErrWriteJSONUnknownFields }
	getErrWriteEmptyComment       = func(dj dictJSON) string { return dj.ErrWriteEmptyComment }
	getErrWritePayloadHeading     = func(dj dictJSON) string { return dj.ErrWritePayloadHeading }
	getErrWriteSectionNotEditable = func(dj dictJSON) string { return dj.ErrWriteSectionNotEditable }
)

// ErrWriteOneSource returns the localized usage refusal when the payload
// source is missing or given twice: exactly one of - (stdin) or --file is
// required, and the terminal is never read to resolve the ambiguity.
func ErrWriteOneSource(lang Lang) string {
	return getDict(lang).errWriteOneSource
}

// ErrWriteSchemaConflict returns the localized refusal for combining
// --schema with a payload source: --schema only prints the schema.
func ErrWriteSchemaConflict(lang Lang) string {
	return getDict(lang).errWriteSchemaConflict
}

// ErrWriteUnknownFlag returns the localized rejection for a flag argument
// that is not one of the write command's long flags, including the refused
// "--flag=value" glued forms, short flags and the "--" separator. cmd names
// the command (comment / section set) so one key serves both.
func ErrWriteUnknownFlag(lang Lang, cmd, flagArg string) string {
	return fmt.Sprintf(getDict(lang).errWriteUnknownFlag, cmd, flagArg)
}

// ErrWriteFileRead returns the localized report for an unreadable --file
// payload source. cause carries only the OS error (the path is named
// separately), mirroring the details resolver precedent.
func ErrWriteFileRead(lang Lang, path string, cause error) string {
	return fmt.Sprintf(getDict(lang).errWriteFileRead, path, cause)
}

// ErrWriteStdinRead returns the localized report for a failed stdin read.
func ErrWriteStdinRead(lang Lang, cause error) string {
	return fmt.Sprintf(getDict(lang).errWriteStdinRead, cause)
}

// ErrWriteJSONInvalid returns the localized rejection for a --json payload
// that is not a strict {"text":"<payload>"} object: offset is the byte
// position of the parse failure and cause the underlying error; the expected
// format and a ready example are baked into the template so the caller can
// self-correct. There is no fallback to plain text.
func ErrWriteJSONInvalid(lang Lang, offset int64, cause error) string {
	return fmt.Sprintf(getDict(lang).errWriteJSONInvalid, offset, cause)
}

// ErrWriteJSONUnknownFields returns the localized refusal listing the
// unexpected JSON fields found (only "text" is allowed).
func ErrWriteJSONUnknownFields(lang Lang, fields string) string {
	return fmt.Sprintf(getDict(lang).errWriteJSONUnknownFields, fields)
}

// ErrWriteEmptyComment returns the localized refusal for an empty comment
// payload: a comment appends a block, and an empty block is rejected.
func ErrWriteEmptyComment(lang Lang) string {
	return getDict(lang).errWriteEmptyComment
}

// ErrWritePayloadHeading returns the localized refusal for a payload line
// that is an ATX heading (it would escape its section on re-parse). line is
// the 1-based offending line number reported by the store.
func ErrWritePayloadHeading(lang Lang, line int) string {
	return fmt.Sprintf(getDict(lang).errWritePayloadHeading, line)
}

// ErrWriteSectionNotEditable returns the localized refusal for a protected
// or unknown section. accepted is the write-path allowlist from
// EditableSections() (never hard-coded); each name is rendered as its CLI
// token — every editable section is a single-word identifier, so lower
// casing is exactly the read-path token spelling (sectionJournalText
// precedent).
func ErrWriteSectionNotEditable(lang Lang, name string, accepted []Section) string {
	tokens := make([]string, len(accepted))
	for i, sec := range accepted {
		tokens[i] = strings.ToLower(string(sec))
	}
	return fmt.Sprintf(getDict(lang).errWriteSectionNotEditable, name, strings.Join(tokens, ", "))
}

// writeDictRules is the ordered validation block for the write keys,
// appended to dictValidationRules (i18n_dict_rules.go). Appending keeps the
// existing entries' observable order intact (T-0146) while the block lives
// next to its message helpers to respect the rules file's line budget.
var writeDictRules = []dictRule{
	nonEmpty("errWriteOneSource", getErrWriteOneSource),
	count("errWriteOneSource", "s", 0, getErrWriteOneSource),
	nonEmpty("errWriteSchemaConflict", getErrWriteSchemaConflict),
	count("errWriteSchemaConflict", "s", 0, getErrWriteSchemaConflict),
	nonEmpty("errWriteUnknownFlag", getErrWriteUnknownFlag),
	count("errWriteUnknownFlag", "s", 2, getErrWriteUnknownFlag),
	nonEmpty("errWriteFileRead", getErrWriteFileRead),
	count("errWriteFileRead", "s", 1, getErrWriteFileRead),
	count("errWriteFileRead", "v", 1, getErrWriteFileRead),
	nonEmpty("errWriteStdinRead", getErrWriteStdinRead),
	count("errWriteStdinRead", "v", 1, getErrWriteStdinRead),
	nonEmpty("errWriteJSONInvalid", getErrWriteJSONInvalid),
	count("errWriteJSONInvalid", "d", 1, getErrWriteJSONInvalid),
	count("errWriteJSONInvalid", "v", 1, getErrWriteJSONInvalid),
	nonEmpty("errWriteJSONUnknownFields", getErrWriteJSONUnknownFields),
	count("errWriteJSONUnknownFields", "s", 1, getErrWriteJSONUnknownFields),
	nonEmpty("errWriteEmptyComment", getErrWriteEmptyComment),
	count("errWriteEmptyComment", "s", 0, getErrWriteEmptyComment),
	nonEmpty("errWritePayloadHeading", getErrWritePayloadHeading),
	count("errWritePayloadHeading", "d", 1, getErrWritePayloadHeading),
	nonEmpty("errWriteSectionNotEditable", getErrWriteSectionNotEditable),
	count("errWriteSectionNotEditable", "s", 2, getErrWriteSectionNotEditable),
}
