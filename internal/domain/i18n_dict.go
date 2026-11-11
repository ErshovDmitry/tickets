package domain

import (
	"encoding/json"
)

// dictJSON mirrors one langs/*.json document. Field names and json tags
// match the on-disk dictionary format exactly.
type dictJSON struct {
	Headers                     json.RawMessage `json:"headers"`
	StatusFmt                   string          `json:"statusFmt"`
	PriorityFmt                 string          `json:"priorityFmt"`
	ProjectFmt                  string          `json:"projectFmt"`
	CreatedFmt                  string          `json:"createdFmt"`
	DetailsStub                 string          `json:"detailsStub"`
	DetailsFormat               string          `json:"detailsFormat"`
	UserCommentsStub            string          `json:"userCommentsStub"`
	JournalCreation             string          `json:"journalCreation"`
	JournalTransition           string          `json:"journalTransition"`
	JournalArchive              string          `json:"journalArchive"`
	WarnNewProject              string          `json:"warnNewProject"`
	NoTickets                   string          `json:"noTickets"`
	ErrTitleCTL                 string          `json:"errTitleCTL"`
	ErrJournalNewline           string          `json:"errJournalNewline"`
	ErrTicketArgNonNumeric      string          `json:"errTicketArgNonNumeric"`
	ErrUnknownFlag              string          `json:"errUnknownFlag"`
	ErrNoClosedTicketsToArchive string          `json:"errNoClosedTicketsToArchive"`
	ErrAlreadyArchived          string          `json:"errAlreadyArchived"`
	ErrArchiveOnlyDoneClosed    string          `json:"errArchiveOnlyDoneClosed"`
	ErrFileAlreadyExists        string          `json:"errFileAlreadyExists"`
	MsgInitialized              string          `json:"msgInitialized"`
	ErrInitError                string          `json:"errInitError"`
	ErrConflict                 string          `json:"errConflict"`
	ErrFlagPRequiresValue       string          `json:"errFlagPRequiresValue"`
	ErrFlagPRequiresNonEmpty    string          `json:"errFlagPRequiresNonEmpty"`
	ErrRepeatedFlagJson         string          `json:"errRepeatedFlagJson"`
	ErrUnknownFlagList          string          `json:"errUnknownFlagList"`
	ErrListOneArgMax            string          `json:"errListOneArgMax"`
	ErrListFilterInvalid        string          `json:"errListFilterInvalid"`
	ErrTitleStartsWithDash      string          `json:"errTitleStartsWithDash"`
	ErrTitleEmpty               string          `json:"errTitleEmpty"`
	ErrTypeInvalid              string          `json:"errTypeInvalid"`
	ErrPriorityInvalid          string          `json:"errPriorityInvalid"`
	WarnTicketsSavedTo          string          `json:"warnTicketsSavedTo"`
	ErrUnknownArgument          string          `json:"errUnknownArgument"`
	ErrFlagRequiresValue        string          `json:"errFlagRequiresValue"`
	ErrStatusInvalid            string          `json:"errStatusInvalid"`
	ErrTicketAlreadyStatus      string          `json:"errTicketAlreadyStatus"`
	ErrDetailsAtBare            string          `json:"errDetailsAtBare"`
	ErrDetailsFileRead          string          `json:"errDetailsFileRead"`
	ErrDetailsStdinRead         string          `json:"errDetailsStdinRead"`
	WarnExtraArgsIgnored        string          `json:"warnExtraArgsIgnored"`
	ErrTicketNotFound           string          `json:"errTicketNotFound"`
	HintUseList                 string          `json:"hintUseList"`
	HintNumbersStartAtOne       string          `json:"hintNumbersStartAtOne"`
	HintStoreEmpty              string          `json:"hintStoreEmpty"`
	HintNumbersUpTo             string          `json:"hintNumbersUpTo"`
	ErrAmbiguousNumber          string          `json:"errAmbiguousNumber"`
	WarnAmbiguousNumber         string          `json:"warnAmbiguousNumber"`
	WarnAmbiguousNumberSkipped  string          `json:"warnAmbiguousNumberSkipped"`
	UpdateCheckUpToDate         string          `json:"updateCheckUpToDate"`
	UpdateCheckAvailable        string          `json:"updateCheckAvailable"`
	UpdateCheckNoReleases       string          `json:"updateCheckNoReleases"`
	UpdateCheckDev              string          `json:"updateCheckDev"`
	ErrUpdateCheck              string          `json:"errUpdateCheck"`
}

// loadDict parses JSON data into a dict, with validation.
func loadDict(data []byte) (*dict, error) {
	var dj dictJSON
	if err := json.Unmarshal(data, &dj); err != nil {
		return nil, err
	}

	headers, err := validateDictJSON(dj)
	if err != nil {
		return nil, err
	}

	return &dict{
		headers:                     headers,
		statusFmt:                   dj.StatusFmt,
		priorityFmt:                 dj.PriorityFmt,
		projectFmt:                  dj.ProjectFmt,
		createdFmt:                  dj.CreatedFmt,
		detailsStub:                 dj.DetailsStub,
		detailsFormat:               dj.DetailsFormat,
		userCommentsStub:            dj.UserCommentsStub,
		journalCreation:             dj.JournalCreation,
		journalTransition:           dj.JournalTransition,
		journalArchive:              dj.JournalArchive,
		warnNewProject:              dj.WarnNewProject,
		noTickets:                   dj.NoTickets,
		errTitleCTL:                 dj.ErrTitleCTL,
		errJournalNewline:           dj.ErrJournalNewline,
		errTicketArgNonNumeric:      dj.ErrTicketArgNonNumeric,
		errUnknownFlag:              dj.ErrUnknownFlag,
		errNoClosedTicketsToArchive: dj.ErrNoClosedTicketsToArchive,
		errAlreadyArchived:          dj.ErrAlreadyArchived,
		errArchiveOnlyDoneClosed:    dj.ErrArchiveOnlyDoneClosed,
		errFileAlreadyExists:        dj.ErrFileAlreadyExists,
		msgInitialized:              dj.MsgInitialized,
		errInitError:                dj.ErrInitError,
		errConflict:                 dj.ErrConflict,
		errFlagPRequiresValue:       dj.ErrFlagPRequiresValue,
		errFlagPRequiresNonEmpty:    dj.ErrFlagPRequiresNonEmpty,
		errRepeatedFlagJson:         dj.ErrRepeatedFlagJson,
		errUnknownFlagList:          dj.ErrUnknownFlagList,
		errListOneArgMax:            dj.ErrListOneArgMax,
		errListFilterInvalid:        dj.ErrListFilterInvalid,
		errTitleStartsWithDash:      dj.ErrTitleStartsWithDash,
		errTitleEmpty:               dj.ErrTitleEmpty,
		errTypeInvalid:              dj.ErrTypeInvalid,
		errPriorityInvalid:          dj.ErrPriorityInvalid,
		warnTicketsSavedTo:          dj.WarnTicketsSavedTo,
		errUnknownArgument:          dj.ErrUnknownArgument,
		errFlagRequiresValue:        dj.ErrFlagRequiresValue,
		errStatusInvalid:            dj.ErrStatusInvalid,
		errTicketAlreadyStatus:      dj.ErrTicketAlreadyStatus,
		errDetailsAtBare:            dj.ErrDetailsAtBare,
		errDetailsFileRead:          dj.ErrDetailsFileRead,
		errDetailsStdinRead:         dj.ErrDetailsStdinRead,
		warnExtraArgsIgnored:        dj.WarnExtraArgsIgnored,
		errTicketNotFound:           dj.ErrTicketNotFound,
		hintUseList:                 dj.HintUseList,
		hintNumbersStartAtOne:       dj.HintNumbersStartAtOne,
		hintStoreEmpty:              dj.HintStoreEmpty,
		hintNumbersUpTo:             dj.HintNumbersUpTo,
		errAmbiguousNumber:          dj.ErrAmbiguousNumber,
		warnAmbiguousNumber:         dj.WarnAmbiguousNumber,
		warnAmbiguousNumberSkipped:  dj.WarnAmbiguousNumberSkipped,
		updateCheckUpToDate:         dj.UpdateCheckUpToDate,
		updateCheckAvailable:        dj.UpdateCheckAvailable,
		updateCheckNoReleases:       dj.UpdateCheckNoReleases,
		updateCheckDev:              dj.UpdateCheckDev,
		errUpdateCheck:              dj.ErrUpdateCheck,
	}, nil
}
