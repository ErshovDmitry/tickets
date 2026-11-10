package domain

import (
	"fmt"
	"strings"
)

// dictRule is one ordered validation step of validateDictJSON: extract the
// field value, then check it. Error text and the order of
// dictValidationRules are observable behaviour and must not change (T-0146).
type dictRule struct {
	value func(dictJSON) string
	check func(v string) error
}

// nonEmpty rejects an empty value: "<name>: empty".
func nonEmpty(name string, get func(dictJSON) string) dictRule {
	return dictRule{value: get, check: func(v string) error {
		if v == "" {
			return fmt.Errorf("%s: empty", name)
		}
		return nil
	}}
}

// count requires exactly n occurrences of "%<verb>": "<name>: expected N
// %<verb>, got M" with M the actual occurrence count.
func count(name, verb string, n int, get func(dictJSON) string) dictRule {
	return dictRule{value: get, check: func(v string) error {
		if m := strings.Count(v, "%"+verb); m != n {
			return fmt.Errorf("%s: expected %d %%%s, got %d", name, n, verb, m)
		}
		return nil
	}}
}

// lineBreak requires a "\n" in the value: "<name>: expected a line break".
func lineBreak(name string, get func(dictJSON) string) dictRule {
	return dictRule{value: get, check: func(v string) error {
		if !strings.Contains(v, "\n") {
			return fmt.Errorf("%s: expected a line break", name)
		}
		return nil
	}}
}

// noPlaceholders rejects values containing "%s" or "%v":
// "<name>: unexpected placeholder".
func noPlaceholders(name string, get func(dictJSON) string) dictRule {
	return dictRule{value: get, check: func(v string) error {
		if strings.Contains(v, "%s") || strings.Contains(v, "%v") {
			return fmt.Errorf("%s: unexpected placeholder", name)
		}
		return nil
	}}
}

// Field getters: one closure per dictJSON field, shared by all rules that
// inspect the field.
var (
	getStatusFmt                   = func(dj dictJSON) string { return dj.StatusFmt }
	getPriorityFmt                 = func(dj dictJSON) string { return dj.PriorityFmt }
	getProjectFmt                  = func(dj dictJSON) string { return dj.ProjectFmt }
	getCreatedFmt                  = func(dj dictJSON) string { return dj.CreatedFmt }
	getDetailsStub                 = func(dj dictJSON) string { return dj.DetailsStub }
	getDetailsFormat               = func(dj dictJSON) string { return dj.DetailsFormat }
	getUserCommentsStub            = func(dj dictJSON) string { return dj.UserCommentsStub }
	getJournalCreation             = func(dj dictJSON) string { return dj.JournalCreation }
	getJournalTransition           = func(dj dictJSON) string { return dj.JournalTransition }
	getJournalArchive              = func(dj dictJSON) string { return dj.JournalArchive }
	getWarnNewProject              = func(dj dictJSON) string { return dj.WarnNewProject }
	getNoTickets                   = func(dj dictJSON) string { return dj.NoTickets }
	getErrTitleCTL                 = func(dj dictJSON) string { return dj.ErrTitleCTL }
	getErrJournalNewline           = func(dj dictJSON) string { return dj.ErrJournalNewline }
	getErrTicketArgNonNumeric      = func(dj dictJSON) string { return dj.ErrTicketArgNonNumeric }
	getErrUnknownFlag              = func(dj dictJSON) string { return dj.ErrUnknownFlag }
	getErrNoClosedTicketsToArchive = func(dj dictJSON) string { return dj.ErrNoClosedTicketsToArchive }
	getErrAlreadyArchived          = func(dj dictJSON) string { return dj.ErrAlreadyArchived }
	getErrArchiveOnlyDoneClosed    = func(dj dictJSON) string { return dj.ErrArchiveOnlyDoneClosed }
	getErrFileAlreadyExists        = func(dj dictJSON) string { return dj.ErrFileAlreadyExists }
	getMsgInitialized              = func(dj dictJSON) string { return dj.MsgInitialized }
	getErrInitError                = func(dj dictJSON) string { return dj.ErrInitError }
	getErrConflict                 = func(dj dictJSON) string { return dj.ErrConflict }
	getErrFlagPRequiresValue       = func(dj dictJSON) string { return dj.ErrFlagPRequiresValue }
	getErrFlagPRequiresNonEmpty    = func(dj dictJSON) string { return dj.ErrFlagPRequiresNonEmpty }
	getErrRepeatedFlagJson         = func(dj dictJSON) string { return dj.ErrRepeatedFlagJson }
	getErrUnknownFlagList          = func(dj dictJSON) string { return dj.ErrUnknownFlagList }
	getErrListOneArgMax            = func(dj dictJSON) string { return dj.ErrListOneArgMax }
	getErrListFilterInvalid        = func(dj dictJSON) string { return dj.ErrListFilterInvalid }
	getErrTitleStartsWithDash      = func(dj dictJSON) string { return dj.ErrTitleStartsWithDash }
	getErrTitleEmpty               = func(dj dictJSON) string { return dj.ErrTitleEmpty }
	getErrTypeInvalid              = func(dj dictJSON) string { return dj.ErrTypeInvalid }
	getErrPriorityInvalid          = func(dj dictJSON) string { return dj.ErrPriorityInvalid }
	getWarnTicketsSavedTo          = func(dj dictJSON) string { return dj.WarnTicketsSavedTo }
	getErrUnknownArgument          = func(dj dictJSON) string { return dj.ErrUnknownArgument }
	getErrFlagRequiresValue        = func(dj dictJSON) string { return dj.ErrFlagRequiresValue }
	getErrStatusInvalid            = func(dj dictJSON) string { return dj.ErrStatusInvalid }
	getErrTicketAlreadyStatus      = func(dj dictJSON) string { return dj.ErrTicketAlreadyStatus }
	getErrDetailsAtBare            = func(dj dictJSON) string { return dj.ErrDetailsAtBare }
	getErrDetailsFileRead          = func(dj dictJSON) string { return dj.ErrDetailsFileRead }
	getErrDetailsStdinRead         = func(dj dictJSON) string { return dj.ErrDetailsStdinRead }
	getWarnExtraArgsIgnored        = func(dj dictJSON) string { return dj.WarnExtraArgsIgnored }
	getErrTicketNotFound           = func(dj dictJSON) string { return dj.ErrTicketNotFound }
	getHintUseList                 = func(dj dictJSON) string { return dj.HintUseList }
	getHintNumbersStartAtOne       = func(dj dictJSON) string { return dj.HintNumbersStartAtOne }
	getHintStoreEmpty              = func(dj dictJSON) string { return dj.HintStoreEmpty }
	getHintNumbersUpTo             = func(dj dictJSON) string { return dj.HintNumbersUpTo }
	getErrAmbiguousNumber          = func(dj dictJSON) string { return dj.ErrAmbiguousNumber }
	getWarnAmbiguousNumber         = func(dj dictJSON) string { return dj.WarnAmbiguousNumber }
	getWarnAmbiguousNumberSkipped  = func(dj dictJSON) string { return dj.WarnAmbiguousNumberSkipped }
)

// dictValidationRules is the ordered validation table of validateDictJSON.
// The entry order mirrors the original if-chain exactly: precedence of error
// reports depends on it, so do not group or reorder entries by field.
var dictValidationRules = []dictRule{
	nonEmpty("statusFmt", getStatusFmt),
	nonEmpty("priorityFmt", getPriorityFmt),
	nonEmpty("projectFmt", getProjectFmt),
	nonEmpty("createdFmt", getCreatedFmt),
	nonEmpty("detailsStub", getDetailsStub),
	nonEmpty("detailsFormat", getDetailsFormat),
	lineBreak("detailsFormat", getDetailsFormat),
	noPlaceholders("detailsFormat", getDetailsFormat),
	nonEmpty("userCommentsStub", getUserCommentsStub),
	nonEmpty("journalCreation", getJournalCreation),
	nonEmpty("journalTransition", getJournalTransition),
	nonEmpty("journalArchive", getJournalArchive),

	// Validate %s counts to catch typos
	count("statusFmt", "s", 1, getStatusFmt),
	count("priorityFmt", "s", 1, getPriorityFmt),
	count("projectFmt", "s", 1, getProjectFmt),
	count("createdFmt", "s", 2, getCreatedFmt),
	count("journalCreation", "s", 2, getJournalCreation),
	count("journalTransition", "s", 3, getJournalTransition),
	count("journalArchive", "s", 2, getJournalArchive),

	// Validate new fields (T-0040)
	nonEmpty("warnNewProject", getWarnNewProject),
	count("warnNewProject", "s", 1, getWarnNewProject),
	nonEmpty("noTickets", getNoTickets),
	count("noTickets", "s", 1, getNoTickets),

	// T-0075: verbatim message, no %s slots — non-empty check only.
	nonEmpty("errTitleCTL", getErrTitleCTL),

	// T-0065: verbatim message, no %s slots — non-empty check only.
	nonEmpty("errJournalNewline", getErrJournalNewline),

	// T-0072: 1 %s slot (the offending argument).
	nonEmpty("errTicketArgNonNumeric", getErrTicketArgNonNumeric),
	count("errTicketArgNonNumeric", "s", 1, getErrTicketArgNonNumeric),

	// T-0068: 1 %s slot (the offending flag argument).
	nonEmpty("errUnknownFlag", getErrUnknownFlag),
	count("errUnknownFlag", "s", 1, getErrUnknownFlag),

	// T-0080: CLI keys — non-empty for all 28, placeholder counts per key.
	nonEmpty("errNoClosedTicketsToArchive", getErrNoClosedTicketsToArchive),
	nonEmpty("errAlreadyArchived", getErrAlreadyArchived),
	nonEmpty("errArchiveOnlyDoneClosed", getErrArchiveOnlyDoneClosed),
	count("errArchiveOnlyDoneClosed", "s", 1, getErrArchiveOnlyDoneClosed),
	nonEmpty("errFileAlreadyExists", getErrFileAlreadyExists),
	count("errFileAlreadyExists", "s", 1, getErrFileAlreadyExists),
	nonEmpty("msgInitialized", getMsgInitialized),
	count("msgInitialized", "s", 1, getMsgInitialized),
	nonEmpty("errInitError", getErrInitError),
	count("errInitError", "v", 1, getErrInitError),
	nonEmpty("errConflict", getErrConflict),
	count("errConflict", "s", 1, getErrConflict),
	nonEmpty("errFlagPRequiresValue", getErrFlagPRequiresValue),
	nonEmpty("errFlagPRequiresNonEmpty", getErrFlagPRequiresNonEmpty),
	nonEmpty("errRepeatedFlagJson", getErrRepeatedFlagJson),
	nonEmpty("errUnknownFlagList", getErrUnknownFlagList),
	count("errUnknownFlagList", "s", 1, getErrUnknownFlagList),
	nonEmpty("errListOneArgMax", getErrListOneArgMax),
	nonEmpty("errListFilterInvalid", getErrListFilterInvalid),
	nonEmpty("errTitleStartsWithDash", getErrTitleStartsWithDash),
	nonEmpty("errTitleEmpty", getErrTitleEmpty),
	nonEmpty("errTypeInvalid", getErrTypeInvalid),
	nonEmpty("errPriorityInvalid", getErrPriorityInvalid),
	nonEmpty("warnTicketsSavedTo", getWarnTicketsSavedTo),
	count("warnTicketsSavedTo", "s", 1, getWarnTicketsSavedTo),
	nonEmpty("errUnknownArgument", getErrUnknownArgument),
	count("errUnknownArgument", "s", 1, getErrUnknownArgument),
	nonEmpty("errFlagRequiresValue", getErrFlagRequiresValue),
	count("errFlagRequiresValue", "s", 1, getErrFlagRequiresValue),
	nonEmpty("errStatusInvalid", getErrStatusInvalid),
	nonEmpty("errTicketAlreadyStatus", getErrTicketAlreadyStatus),
	count("errTicketAlreadyStatus", "d", 1, getErrTicketAlreadyStatus),
	count("errTicketAlreadyStatus", "s", 1, getErrTicketAlreadyStatus),
	nonEmpty("errDetailsAtBare", getErrDetailsAtBare),
	nonEmpty("errDetailsFileRead", getErrDetailsFileRead),
	count("errDetailsFileRead", "s", 1, getErrDetailsFileRead),
	count("errDetailsFileRead", "v", 1, getErrDetailsFileRead),
	nonEmpty("errDetailsStdinRead", getErrDetailsStdinRead),
	count("errDetailsStdinRead", "v", 1, getErrDetailsStdinRead),
	nonEmpty("warnExtraArgsIgnored", getWarnExtraArgsIgnored),
	count("warnExtraArgsIgnored", "s", 1, getWarnExtraArgsIgnored),
	nonEmpty("errTicketNotFound", getErrTicketNotFound),
	count("errTicketNotFound", "s", 1, getErrTicketNotFound),
	nonEmpty("hintUseList", getHintUseList),
	nonEmpty("hintNumbersStartAtOne", getHintNumbersStartAtOne),
	nonEmpty("hintStoreEmpty", getHintStoreEmpty),
	nonEmpty("hintNumbersUpTo", getHintNumbersUpTo),
	count("hintNumbersUpTo", "d", 1, getHintNumbersUpTo),
	nonEmpty("errAmbiguousNumber", getErrAmbiguousNumber),
	count("errAmbiguousNumber", "d", 1, getErrAmbiguousNumber),
	nonEmpty("warnAmbiguousNumber", getWarnAmbiguousNumber),
	count("warnAmbiguousNumber", "d", 1, getWarnAmbiguousNumber),
	count("warnAmbiguousNumber", "s", 1, getWarnAmbiguousNumber),
	nonEmpty("warnAmbiguousNumberSkipped", getWarnAmbiguousNumberSkipped),
	count("warnAmbiguousNumberSkipped", "d", 1, getWarnAmbiguousNumberSkipped),
	count("warnAmbiguousNumberSkipped", "s", 1, getWarnAmbiguousNumberSkipped),
}
