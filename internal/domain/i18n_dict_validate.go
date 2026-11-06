package domain

import (
	"encoding/json"
	"fmt"
	"strings"
)

// validateDictJSON validates one decoded dictionary in a fixed order and
// returns its parsed section headers. Error text and precedence are part
// of the observable behaviour.
func validateDictJSON(dj dictJSON) ([5]string, error) {
	var rawHeaders []string

	// Parse headers into slice first to validate length
	if err := json.Unmarshal(dj.Headers, &rawHeaders); err != nil {
		return [5]string{}, fmt.Errorf("headers: %w", err)
	}
	if len(rawHeaders) != 5 {
		return [5]string{}, fmt.Errorf("headers: expected 5 elements, got %d", len(rawHeaders))
	}
	for i, h := range rawHeaders {
		if h == "" {
			return [5]string{}, fmt.Errorf("headers[%d]: empty string not allowed", i)
		}
	}
	var headers [5]string
	copy(headers[:], rawHeaders)

	// Validate non-empty string fields
	if dj.StatusFmt == "" {
		return [5]string{}, fmt.Errorf("statusFmt: empty")
	}
	if dj.PriorityFmt == "" {
		return [5]string{}, fmt.Errorf("priorityFmt: empty")
	}
	if dj.ProjectFmt == "" {
		return [5]string{}, fmt.Errorf("projectFmt: empty")
	}
	if dj.CreatedFmt == "" {
		return [5]string{}, fmt.Errorf("createdFmt: empty")
	}
	if dj.DetailsStub == "" {
		return [5]string{}, fmt.Errorf("detailsStub: empty")
	}
	if dj.DetailsFormat == "" {
		return [5]string{}, fmt.Errorf("detailsFormat: empty")
	}
	if !strings.Contains(dj.DetailsFormat, "\n") {
		return [5]string{}, fmt.Errorf("detailsFormat: expected a line break")
	}
	if strings.Contains(dj.DetailsFormat, "%s") || strings.Contains(dj.DetailsFormat, "%v") {
		return [5]string{}, fmt.Errorf("detailsFormat: unexpected placeholder")
	}
	if dj.UserCommentsStub == "" {
		return [5]string{}, fmt.Errorf("userCommentsStub: empty")
	}
	if dj.JournalCreation == "" {
		return [5]string{}, fmt.Errorf("journalCreation: empty")
	}
	if dj.JournalTransition == "" {
		return [5]string{}, fmt.Errorf("journalTransition: empty")
	}
	if dj.JournalArchive == "" {
		return [5]string{}, fmt.Errorf("journalArchive: empty")
	}

	// Validate %s counts to catch typos
	if strings.Count(dj.StatusFmt, "%s") != 1 {
		return [5]string{}, fmt.Errorf("statusFmt: expected 1 %%s, got %d", strings.Count(dj.StatusFmt, "%s"))
	}
	if strings.Count(dj.PriorityFmt, "%s") != 1 {
		return [5]string{}, fmt.Errorf("priorityFmt: expected 1 %%s, got %d", strings.Count(dj.PriorityFmt, "%s"))
	}
	if strings.Count(dj.ProjectFmt, "%s") != 1 {
		return [5]string{}, fmt.Errorf("projectFmt: expected 1 %%s, got %d", strings.Count(dj.ProjectFmt, "%s"))
	}
	if strings.Count(dj.CreatedFmt, "%s") != 2 {
		return [5]string{}, fmt.Errorf("createdFmt: expected 2 %%s, got %d", strings.Count(dj.CreatedFmt, "%s"))
	}
	if strings.Count(dj.JournalCreation, "%s") != 2 {
		return [5]string{}, fmt.Errorf("journalCreation: expected 2 %%s, got %d", strings.Count(dj.JournalCreation, "%s"))
	}
	if strings.Count(dj.JournalTransition, "%s") != 3 {
		return [5]string{}, fmt.Errorf("journalTransition: expected 3 %%s, got %d", strings.Count(dj.JournalTransition, "%s"))
	}
	if strings.Count(dj.JournalArchive, "%s") != 2 {
		return [5]string{}, fmt.Errorf("journalArchive: expected 2 %%s, got %d", strings.Count(dj.JournalArchive, "%s"))
	}
	// Validate new fields (T-0040)
	if dj.WarnNewProject == "" {
		return [5]string{}, fmt.Errorf("warnNewProject: empty")
	}
	if strings.Count(dj.WarnNewProject, "%s") != 1 {
		return [5]string{}, fmt.Errorf("warnNewProject: expected 1 %%s, got %d", strings.Count(dj.WarnNewProject, "%s"))
	}
	if dj.NoTickets == "" {
		return [5]string{}, fmt.Errorf("noTickets: empty")
	}
	if strings.Count(dj.NoTickets, "%s") != 1 {
		return [5]string{}, fmt.Errorf("noTickets: expected 1 %%s, got %d", strings.Count(dj.NoTickets, "%s"))
	}
	// T-0075: verbatim message, no %s slots — non-empty check only.
	if dj.ErrTitleCTL == "" {
		return [5]string{}, fmt.Errorf("errTitleCTL: empty")
	}
	// T-0065: verbatim message, no %s slots — non-empty check only.
	if dj.ErrJournalNewline == "" {
		return [5]string{}, fmt.Errorf("errJournalNewline: empty")
	}
	// T-0072: 1 %s slot (the offending argument).
	if dj.ErrTicketArgNonNumeric == "" {
		return [5]string{}, fmt.Errorf("errTicketArgNonNumeric: empty")
	}
	if strings.Count(dj.ErrTicketArgNonNumeric, "%s") != 1 {
		return [5]string{}, fmt.Errorf("errTicketArgNonNumeric: expected 1 %%s, got %d", strings.Count(dj.ErrTicketArgNonNumeric, "%s"))
	}
	// T-0068: 1 %s slot (the offending flag argument).
	if dj.ErrUnknownFlag == "" {
		return [5]string{}, fmt.Errorf("errUnknownFlag: empty")
	}
	if strings.Count(dj.ErrUnknownFlag, "%s") != 1 {
		return [5]string{}, fmt.Errorf("errUnknownFlag: expected 1 %%s, got %d", strings.Count(dj.ErrUnknownFlag, "%s"))
	}

	// T-0080: CLI keys — non-empty for all 28, placeholder counts per key.
	if dj.ErrNoClosedTicketsToArchive == "" {
		return [5]string{}, fmt.Errorf("errNoClosedTicketsToArchive: empty")
	}
	if dj.ErrAlreadyArchived == "" {
		return [5]string{}, fmt.Errorf("errAlreadyArchived: empty")
	}
	if dj.ErrArchiveOnlyDoneClosed == "" {
		return [5]string{}, fmt.Errorf("errArchiveOnlyDoneClosed: empty")
	}
	if strings.Count(dj.ErrArchiveOnlyDoneClosed, "%s") != 1 {
		return [5]string{}, fmt.Errorf("errArchiveOnlyDoneClosed: expected 1 %%s, got %d", strings.Count(dj.ErrArchiveOnlyDoneClosed, "%s"))
	}
	if dj.ErrFileAlreadyExists == "" {
		return [5]string{}, fmt.Errorf("errFileAlreadyExists: empty")
	}
	if strings.Count(dj.ErrFileAlreadyExists, "%s") != 1 {
		return [5]string{}, fmt.Errorf("errFileAlreadyExists: expected 1 %%s, got %d", strings.Count(dj.ErrFileAlreadyExists, "%s"))
	}
	if dj.MsgInitialized == "" {
		return [5]string{}, fmt.Errorf("msgInitialized: empty")
	}
	if strings.Count(dj.MsgInitialized, "%s") != 1 {
		return [5]string{}, fmt.Errorf("msgInitialized: expected 1 %%s, got %d", strings.Count(dj.MsgInitialized, "%s"))
	}
	if dj.ErrInitError == "" {
		return [5]string{}, fmt.Errorf("errInitError: empty")
	}
	if strings.Count(dj.ErrInitError, "%v") != 1 {
		return [5]string{}, fmt.Errorf("errInitError: expected 1 %%v, got %d", strings.Count(dj.ErrInitError, "%v"))
	}
	if dj.ErrConflict == "" {
		return [5]string{}, fmt.Errorf("errConflict: empty")
	}
	if strings.Count(dj.ErrConflict, "%s") != 1 {
		return [5]string{}, fmt.Errorf("errConflict: expected 1 %%s, got %d", strings.Count(dj.ErrConflict, "%s"))
	}
	if dj.ErrFlagPRequiresValue == "" {
		return [5]string{}, fmt.Errorf("errFlagPRequiresValue: empty")
	}
	if dj.ErrFlagPRequiresNonEmpty == "" {
		return [5]string{}, fmt.Errorf("errFlagPRequiresNonEmpty: empty")
	}
	if dj.ErrRepeatedFlagJson == "" {
		return [5]string{}, fmt.Errorf("errRepeatedFlagJson: empty")
	}
	if dj.ErrUnknownFlagList == "" {
		return [5]string{}, fmt.Errorf("errUnknownFlagList: empty")
	}
	if strings.Count(dj.ErrUnknownFlagList, "%s") != 1 {
		return [5]string{}, fmt.Errorf("errUnknownFlagList: expected 1 %%s, got %d", strings.Count(dj.ErrUnknownFlagList, "%s"))
	}
	if dj.ErrListOneArgMax == "" {
		return [5]string{}, fmt.Errorf("errListOneArgMax: empty")
	}
	if dj.ErrListFilterInvalid == "" {
		return [5]string{}, fmt.Errorf("errListFilterInvalid: empty")
	}
	if dj.ErrTitleStartsWithDash == "" {
		return [5]string{}, fmt.Errorf("errTitleStartsWithDash: empty")
	}
	if dj.ErrTitleEmpty == "" {
		return [5]string{}, fmt.Errorf("errTitleEmpty: empty")
	}
	if dj.ErrTypeInvalid == "" {
		return [5]string{}, fmt.Errorf("errTypeInvalid: empty")
	}
	if dj.ErrPriorityInvalid == "" {
		return [5]string{}, fmt.Errorf("errPriorityInvalid: empty")
	}
	if dj.WarnTicketsSavedTo == "" {
		return [5]string{}, fmt.Errorf("warnTicketsSavedTo: empty")
	}
	if strings.Count(dj.WarnTicketsSavedTo, "%s") != 1 {
		return [5]string{}, fmt.Errorf("warnTicketsSavedTo: expected 1 %%s, got %d", strings.Count(dj.WarnTicketsSavedTo, "%s"))
	}
	if dj.ErrUnknownArgument == "" {
		return [5]string{}, fmt.Errorf("errUnknownArgument: empty")
	}
	if strings.Count(dj.ErrUnknownArgument, "%s") != 1 {
		return [5]string{}, fmt.Errorf("errUnknownArgument: expected 1 %%s, got %d", strings.Count(dj.ErrUnknownArgument, "%s"))
	}
	if dj.ErrFlagRequiresValue == "" {
		return [5]string{}, fmt.Errorf("errFlagRequiresValue: empty")
	}
	if strings.Count(dj.ErrFlagRequiresValue, "%s") != 1 {
		return [5]string{}, fmt.Errorf("errFlagRequiresValue: expected 1 %%s, got %d", strings.Count(dj.ErrFlagRequiresValue, "%s"))
	}
	if dj.ErrStatusInvalid == "" {
		return [5]string{}, fmt.Errorf("errStatusInvalid: empty")
	}
	if dj.ErrTicketAlreadyStatus == "" {
		return [5]string{}, fmt.Errorf("errTicketAlreadyStatus: empty")
	}
	if strings.Count(dj.ErrTicketAlreadyStatus, "%d") != 1 {
		return [5]string{}, fmt.Errorf("errTicketAlreadyStatus: expected 1 %%d, got %d", strings.Count(dj.ErrTicketAlreadyStatus, "%d"))
	}
	if strings.Count(dj.ErrTicketAlreadyStatus, "%s") != 1 {
		return [5]string{}, fmt.Errorf("errTicketAlreadyStatus: expected 1 %%s, got %d", strings.Count(dj.ErrTicketAlreadyStatus, "%s"))
	}
	if dj.ErrDetailsAtBare == "" {
		return [5]string{}, fmt.Errorf("errDetailsAtBare: empty")
	}
	if dj.ErrDetailsFileRead == "" {
		return [5]string{}, fmt.Errorf("errDetailsFileRead: empty")
	}
	if strings.Count(dj.ErrDetailsFileRead, "%s") != 1 {
		return [5]string{}, fmt.Errorf("errDetailsFileRead: expected 1 %%s, got %d", strings.Count(dj.ErrDetailsFileRead, "%s"))
	}
	if strings.Count(dj.ErrDetailsFileRead, "%v") != 1 {
		return [5]string{}, fmt.Errorf("errDetailsFileRead: expected 1 %%v, got %d", strings.Count(dj.ErrDetailsFileRead, "%v"))
	}
	if dj.ErrDetailsStdinRead == "" {
		return [5]string{}, fmt.Errorf("errDetailsStdinRead: empty")
	}
	if strings.Count(dj.ErrDetailsStdinRead, "%v") != 1 {
		return [5]string{}, fmt.Errorf("errDetailsStdinRead: expected 1 %%v, got %d", strings.Count(dj.ErrDetailsStdinRead, "%v"))
	}
	if dj.WarnExtraArgsIgnored == "" {
		return [5]string{}, fmt.Errorf("warnExtraArgsIgnored: empty")
	}
	if strings.Count(dj.WarnExtraArgsIgnored, "%s") != 1 {
		return [5]string{}, fmt.Errorf("warnExtraArgsIgnored: expected 1 %%s, got %d", strings.Count(dj.WarnExtraArgsIgnored, "%s"))
	}
	if dj.ErrTicketNotFound == "" {
		return [5]string{}, fmt.Errorf("errTicketNotFound: empty")
	}
	if strings.Count(dj.ErrTicketNotFound, "%s") != 1 {
		return [5]string{}, fmt.Errorf("errTicketNotFound: expected 1 %%s, got %d", strings.Count(dj.ErrTicketNotFound, "%s"))
	}
	if dj.HintUseList == "" {
		return [5]string{}, fmt.Errorf("hintUseList: empty")
	}
	if dj.HintNumbersStartAtOne == "" {
		return [5]string{}, fmt.Errorf("hintNumbersStartAtOne: empty")
	}
	if dj.HintStoreEmpty == "" {
		return [5]string{}, fmt.Errorf("hintStoreEmpty: empty")
	}
	if dj.HintNumbersUpTo == "" {
		return [5]string{}, fmt.Errorf("hintNumbersUpTo: empty")
	}
	if strings.Count(dj.HintNumbersUpTo, "%d") != 1 {
		return [5]string{}, fmt.Errorf("hintNumbersUpTo: expected 1 %%d, got %d", strings.Count(dj.HintNumbersUpTo, "%d"))
	}

	return headers, nil
}
