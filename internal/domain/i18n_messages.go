package domain

import (
	"fmt"
)

// This file holds the exported message helpers: every user-visible string
// is formatted from the dictionary of the requested language.

// WarnNewProject formats the first-use warning for a new project name.
func WarnNewProject(lang Lang, project string) string {
	return fmt.Sprintf(getDict(lang).warnNewProject, project)
}

// NoTickets formats the "no tickets found" message with filter description.
func NoTickets(lang Lang, filter string) string {
	return fmt.Sprintf(getDict(lang).noTickets, filter)
}

// ErrTitleCTL returns the localized rejection message for a ticket title
// containing a C0 control character (0x00–0x1F except TAB) or DEL (0x7F)
// (T-0075): `new` prints it before any file is created, because such bytes
// corrupt the H1 and the terminal.
func ErrTitleCTL(lang Lang) string {
	return getDict(lang).errTitleCTL
}

// ContainsCTL reports whether s contains a C0 control character
// (0x00–0x1F, excluding TAB 0x09) or DEL (0x7F). CR/LF are covered
// (T-0075 supersedes the T-0044 ContainsAny check).
func ContainsCTL(s string) bool {
	for _, r := range s {
		if (r <= 0x1F && r != 0x09) || r == 0x7F {
			return true
		}
	}
	return false
}

// ErrJournalNewline returns the localized rejection message for a
// journal input (comment or TICKET_WHO) carrying CR or LF (T-0065):
// `set` and `archive` print it before any file is mutated, because
// the journal is line-oriented and an embedded newline would forge a
// journal entry.
func ErrJournalNewline(lang Lang) string {
	return getDict(lang).errJournalNewline
}

// ErrTicketArgNonNumeric returns the localized rejection for a `show`
// argument containing no digits (T-0072): it is not a ticket number at
// all, so the «не найден» report would mislead.
func ErrTicketArgNonNumeric(lang Lang, arg string) string {
	return fmt.Sprintf(getDict(lang).errTicketArgNonNumeric, arg)
}

// ErrUnknownFlag returns the localized rejection for a flag-like argument
// in `set`'s number or status position (T-0068): cmd_set parses no flags,
// so a leading '-' there is never a valid positional.
func ErrUnknownFlag(lang Lang, flagArg string) string {
	return fmt.Sprintf(getDict(lang).errUnknownFlag, flagArg)
}

// ErrNoClosedTicketsToArchive returns the localized message for `archive`
// with no done/closed tickets to move (T-0080).
func ErrNoClosedTicketsToArchive(lang Lang) string {
	return getDict(lang).errNoClosedTicketsToArchive
}

// ErrAlreadyArchived returns the localized message for archiving a ticket
// that is already in the archive (T-0080).
func ErrAlreadyArchived(lang Lang) string {
	return getDict(lang).errAlreadyArchived
}

// ErrArchiveOnlyDoneClosed formats the localized rejection for archiving a
// ticket that is neither done nor closed (1 %s: current status).
func ErrArchiveOnlyDoneClosed(lang Lang, status string) string {
	return fmt.Sprintf(getDict(lang).errArchiveOnlyDoneClosed, status)
}

// ErrFileAlreadyExists formats the localized rejection for a name collision
// on ticket creation (1 %s: existing file path).
func ErrFileAlreadyExists(lang Lang, path string) string {
	return fmt.Sprintf(getDict(lang).errFileAlreadyExists, path)
}

// MsgInitialized formats the localized `init` success message (1 %s: path).
func MsgInitialized(lang Lang, path string) string {
	return fmt.Sprintf(getDict(lang).msgInitialized, path)
}

// ErrInitError formats the localized `init` failure message (1 %v: error).
func ErrInitError(lang Lang, err error) string {
	return fmt.Sprintf(getDict(lang).errInitError, err)
}

// ErrConflict formats the localized conflict report (1 %s: path).
func ErrConflict(lang Lang, path string) string {
	return fmt.Sprintf(getDict(lang).errConflict, path)
}

// ErrFlagPRequiresValue returns the localized rejection for a bare -P flag
// given without a value (T-0080).
func ErrFlagPRequiresValue(lang Lang) string {
	return getDict(lang).errFlagPRequiresValue
}

// ErrFlagPRequiresNonEmpty returns the localized rejection for -P with an
// empty project name (T-0080).
func ErrFlagPRequiresNonEmpty(lang Lang) string {
	return getDict(lang).errFlagPRequiresNonEmpty
}

// ErrRepeatedFlagJson returns the localized rejection for a repeated --json
// flag (T-0080).
func ErrRepeatedFlagJson(lang Lang) string {
	return getDict(lang).errRepeatedFlagJson
}

// ErrUnknownFlagList formats the localized rejection for an unknown flag in
// `list` (1 %s: the offending flag).
func ErrUnknownFlagList(lang Lang, flag string) string {
	return fmt.Sprintf(getDict(lang).errUnknownFlagList, flag)
}

// ErrListOneArgMax returns the localized rejection for more than one
// positional argument to `list` (T-0080).
func ErrListOneArgMax(lang Lang) string {
	return getDict(lang).errListOneArgMax
}

// ErrListFilterInvalid returns the localized rejection for an invalid list
// filter value (T-0080).
func ErrListFilterInvalid(lang Lang) string {
	return getDict(lang).errListFilterInvalid
}

// ErrTitleStartsWithDash returns the localized rejection for a title
// beginning with '-' (T-0080).
func ErrTitleStartsWithDash(lang Lang) string {
	return getDict(lang).errTitleStartsWithDash
}

// ErrTitleEmpty returns the localized rejection for an empty title (T-0080).
func ErrTitleEmpty(lang Lang) string {
	return getDict(lang).errTitleEmpty
}

// ErrTypeInvalid returns the localized rejection for an invalid ticket type
// value (T-0080).
func ErrTypeInvalid(lang Lang) string {
	return getDict(lang).errTypeInvalid
}

// ErrPriorityInvalid returns the localized rejection for an invalid priority
// value (T-0080).
func ErrPriorityInvalid(lang Lang) string {
	return getDict(lang).errPriorityInvalid
}

// WarnTicketsSavedTo formats the localized warning naming the directory the
// tickets will be stored in (1 %s: directory).
func WarnTicketsSavedTo(lang Lang, dir string) string {
	return fmt.Sprintf(getDict(lang).warnTicketsSavedTo, dir)
}

// ErrUnknownArgument formats the localized rejection for an unrecognized
// positional argument (1 %s: the argument).
func ErrUnknownArgument(lang Lang, arg string) string {
	return fmt.Sprintf(getDict(lang).errUnknownArgument, arg)
}

// ErrFlagRequiresValue formats the localized rejection for a flag given
// without its value (1 %s: the flag).
func ErrFlagRequiresValue(lang Lang, flag string) string {
	return fmt.Sprintf(getDict(lang).errFlagRequiresValue, flag)
}

// ErrStatusInvalid returns the localized rejection for an invalid status
// value (T-0080).
func ErrStatusInvalid(lang Lang) string {
	return getDict(lang).errStatusInvalid
}

// ErrTicketAlreadyStatus formats the localized report for a `set` on a
// ticket already in the requested status (1 %d: number, 1 %s: status).
func ErrTicketAlreadyStatus(lang Lang, num int, status string) string {
	return fmt.Sprintf(getDict(lang).errTicketAlreadyStatus, num, status)
}

// ErrDetailsAtBare returns the localized rejection for a bare "@" details
// reference without a path (no placeholders) (T-0109).
func ErrDetailsAtBare(lang Lang) string {
	return getDict(lang).errDetailsAtBare
}

// ErrDetailsFileRead formats the localized failure of reading the details
// from a file (1 %s: path, 1 %v: cause) (T-0109).
func ErrDetailsFileRead(lang Lang, path string, cause error) string {
	return fmt.Sprintf(getDict(lang).errDetailsFileRead, path, cause)
}

// ErrDetailsStdinRead formats the localized failure of reading the details
// from stdin (1 %v: error) (T-0109).
func ErrDetailsStdinRead(lang Lang, err error) string {
	return fmt.Sprintf(getDict(lang).errDetailsStdinRead, err)
}

// WarnExtraArgsIgnored formats the localized warning for extra positional
// arguments (1 %s: the extra args).
func WarnExtraArgsIgnored(lang Lang, args string) string {
	return fmt.Sprintf(getDict(lang).warnExtraArgsIgnored, args)
}

// ErrTicketNotFound formats the localized "not found" report for a `show`
// argument (1 %s: the argument).
func ErrTicketNotFound(lang Lang, arg string) string {
	return fmt.Sprintf(getDict(lang).errTicketNotFound, arg)
}

// HintUseList returns the localized hint to use `ticket list` (show also
// searches archive/) (T-0080).
func HintUseList(lang Lang) string {
	return getDict(lang).hintUseList
}

// HintNumbersStartAtOne returns the localized hint that ticket numbers start
// at 1 (T-0080).
func HintNumbersStartAtOne(lang Lang) string {
	return getDict(lang).hintNumbersStartAtOne
}

// HintStoreEmpty returns the localized hint that the store is empty (T-0080).
func HintStoreEmpty(lang Lang) string {
	return getDict(lang).hintStoreEmpty
}

// HintNumbersUpTo formats the localized hint naming the largest ticket
// number present in the store (1 %d: max number).
func HintNumbersUpTo(lang Lang, max int) string {
	return fmt.Sprintf(getDict(lang).hintNumbersUpTo, max)
}

// ErrAmbiguousNumber formats the localized refusal for a ticket number
// owned by more than one file (T-0157, 1 %d: number). The conflicting
// paths are printed by the caller, one per line.
func ErrAmbiguousNumber(lang Lang, num int) string {
	return fmt.Sprintf(getDict(lang).errAmbiguousNumber, num)
}

// WarnAmbiguousNumber formats the localized non-fatal warning naming a
// duplicated ticket number in the list inventory (T-0157, 1 %d: number,
// 1 %s: conflicting absolute paths).
func WarnAmbiguousNumber(lang Lang, num int, paths string) string {
	return fmt.Sprintf(getDict(lang).warnAmbiguousNumber, num, paths)
}

// WarnAmbiguousNumberSkipped formats the localized warning for a
// duplicated ticket number skipped by archive-all (T-0157, 1 %d: number,
// 1 %s: conflicting absolute paths).
func WarnAmbiguousNumberSkipped(lang Lang, num int, paths string) string {
	return fmt.Sprintf(getDict(lang).warnAmbiguousNumberSkipped, num, paths)
}
