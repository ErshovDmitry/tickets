package domain

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Lang identifies the language of a ticket file's format (section headers,
// meta labels, stubs, journal templates). It affects only: (1) emission of
// new journal lines, (2) stub choice for empty sections, (3) default forms
// for absent components. It does NOT affect preserved raw templates (LOCK
// D1): recognized lines re-emit byte-identically, free suffixes intact.
type Lang string

const (
	LangRU Lang = "ru"
	LangEN Lang = "en"
)

// dict holds all emission and recognition templates for one language.
type dict struct {
	// Section headers: full emission strings "## Name" or "## Name (suffix)".
	// Index order: Summary, Details, UserComments, Comments, Journal.
	headers [5]string

	// Meta emission forms with %s placeholder(s). Status/Priority/Project
	// have one slot; Created has two (timestamp · by author).
	statusFmt   string
	priorityFmt string
	projectFmt  string
	createdFmt  string

	// Stubs for empty Details and UserComments. No stub for Comments (by
	// design: Comments section has no stub, T-0035).
	detailsStub      string
	userCommentsStub string

	// Journal templates for emission (also used for parsing). Three forms:
	// creation "- TS — ticket created (who).", transition "- TS — status:
	// from → to[ · comment] (who)", archive "- TS — moved to archive (who)".
	journalCreation   string
	journalTransition string
	journalArchive    string

	// T-0040: multi-project support
	warnNewProject string // 1 %s: project name
	noTickets      string // 1 %s: filter description

	// T-0075: verbatim CLI error message (no placeholders).
	errTitleCTL string

	// T-0065: rejection for journal inputs (comment/who) carrying CR/LF.
	errJournalNewline string

	// T-0072: rejection for a `show` argument with no digits (1 %s: the
	// offending argument).
	errTicketArgNonNumeric string

	// T-0068: rejection for a flag-like arg in `set`'s number/status
	// position (1 %s: the offending flag argument).
	errUnknownFlag string

	// T-0080: CLI error/message keys (foundation layer; CLI call-site
	// wiring is a follow-up). Placeholder counts are validated in loadDict.
	errNoClosedTicketsToArchive string
	errAlreadyArchived          string
	errArchiveOnlyDoneClosed    string // 1 %s: current status
	errFileAlreadyExists        string // 1 %s: file path
	msgInitialized              string // 1 %s: path
	errInitError                string // 1 %v: error
	errConflict                 string // 1 %s: path
	errFlagPRequiresValue       string
	errFlagPRequiresNonEmpty    string
	errRepeatedFlagJson         string
	errUnknownFlagList          string // 1 %s: flag
	errListOneArgMax            string
	errListFilterInvalid        string
	errTitleStartsWithDash      string
	errTitleEmpty               string
	errTypeInvalid              string
	errPriorityInvalid          string
	warnTicketsSavedTo          string // 1 %s: directory
	errUnknownArgument          string // 1 %s: argument
	errFlagRequiresValue        string // 1 %s: flag
	errStatusInvalid            string
	errTicketAlreadyStatus      string // 1 %d: number, 1 %s: status
	warnExtraArgsIgnored        string // 1 %s: extra args
	errTicketNotFound           string // 1 %s: argument
	hintUseList                 string
	hintNumbersStartAtOne       string
	hintStoreEmpty              string
	hintNumbersUpTo             string // 1 %d: max number
}

//go:embed langs/*.json
var langsFS embed.FS

var (
	// dictRU is the Russian dictionary, loaded from embedded langs/ru.json.
	dictRU *dict

	// dictEN is the English dictionary, loaded from embedded langs/en.json.
	dictEN *dict

	allDicts []struct {
		lang Lang
		d    *dict
	}

	registry map[Lang]*dict
)

func init() {
	registry = make(map[Lang]*dict)

	// Auto-discover all embedded langs/*.json files
	entries, err := langsFS.ReadDir("langs")
	if err != nil {
		panic(fmt.Errorf("read langs dir: %w", err))
	}

	// Track first-seen filename per lang code so we can panic with both
	// filenames when a duplicate code is encountered (e.g. RU.json vs ru.json).
	firstSeen := make(map[Lang]string)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		fname := "langs/" + entry.Name()
		data, err := langsFS.ReadFile(fname)
		if err != nil {
			panic(fmt.Errorf("load %s: %w", fname, err))
		}
		d, err := loadDict(data)
		if err != nil {
			panic(fmt.Errorf("parse %s: %w", fname, err))
		}
		// Lang code = filename stem lowercased
		code := strings.ToLower(strings.TrimSuffix(entry.Name(), ".json"))
		lang := Lang(code)
		if prior, dup := firstSeen[lang]; dup {
			panic(fmt.Sprintf("i18n: duplicate language code %q from %s and %s", code, prior, fname))
		}
		firstSeen[lang] = fname
		registry[lang] = d
	}

	// Assign package-level variables
	dictRU = registry[LangRU]
	dictEN = registry[LangEN]
	if dictRU == nil || dictEN == nil {
		panic("missing ru.json or en.json")
	}

	// Fixed order: RU first, then EN (LOCK), then rest in sorted order
	allDicts = []struct {
		lang Lang
		d    *dict
	}{
		{LangRU, dictRU},
		{LangEN, dictEN},
	}
	// Append other languages in alphabetical order
	var otherLangs []Lang
	for code := range registry {
		if code != LangRU && code != LangEN {
			otherLangs = append(otherLangs, code)
		}
	}
	// Sort otherLangs
	sort.Slice(otherLangs, func(i, j int) bool { return otherLangs[i] < otherLangs[j] })
	for _, code := range otherLangs {
		allDicts = append(allDicts, struct {
			lang Lang
			d    *dict
		}{code, registry[code]})
	}

	// Build journal patterns after allDicts is populated
	journalDictPatterns = buildJournalPatterns()
}

// loadDict parses JSON data into a dict, with validation.
func loadDict(data []byte) (*dict, error) {
	var rawHeaders []string
	var dj struct {
		Headers                     json.RawMessage `json:"headers"`
		StatusFmt                   string          `json:"statusFmt"`
		PriorityFmt                 string          `json:"priorityFmt"`
		ProjectFmt                  string          `json:"projectFmt"`
		CreatedFmt                  string          `json:"createdFmt"`
		DetailsStub                 string          `json:"detailsStub"`
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
		WarnExtraArgsIgnored        string          `json:"warnExtraArgsIgnored"`
		ErrTicketNotFound           string          `json:"errTicketNotFound"`
		HintUseList                 string          `json:"hintUseList"`
		HintNumbersStartAtOne       string          `json:"hintNumbersStartAtOne"`
		HintStoreEmpty              string          `json:"hintStoreEmpty"`
		HintNumbersUpTo             string          `json:"hintNumbersUpTo"`
	}
	if err := json.Unmarshal(data, &dj); err != nil {
		return nil, err
	}

	// Parse headers into slice first to validate length
	if err := json.Unmarshal(dj.Headers, &rawHeaders); err != nil {
		return nil, fmt.Errorf("headers: %w", err)
	}
	if len(rawHeaders) != 5 {
		return nil, fmt.Errorf("headers: expected 5 elements, got %d", len(rawHeaders))
	}
	for i, h := range rawHeaders {
		if h == "" {
			return nil, fmt.Errorf("headers[%d]: empty string not allowed", i)
		}
	}
	var headers [5]string
	copy(headers[:], rawHeaders)

	// Validate non-empty string fields
	if dj.StatusFmt == "" {
		return nil, fmt.Errorf("statusFmt: empty")
	}
	if dj.PriorityFmt == "" {
		return nil, fmt.Errorf("priorityFmt: empty")
	}
	if dj.ProjectFmt == "" {
		return nil, fmt.Errorf("projectFmt: empty")
	}
	if dj.CreatedFmt == "" {
		return nil, fmt.Errorf("createdFmt: empty")
	}
	if dj.DetailsStub == "" {
		return nil, fmt.Errorf("detailsStub: empty")
	}
	if dj.UserCommentsStub == "" {
		return nil, fmt.Errorf("userCommentsStub: empty")
	}
	if dj.JournalCreation == "" {
		return nil, fmt.Errorf("journalCreation: empty")
	}
	if dj.JournalTransition == "" {
		return nil, fmt.Errorf("journalTransition: empty")
	}
	if dj.JournalArchive == "" {
		return nil, fmt.Errorf("journalArchive: empty")
	}

	// Validate %s counts to catch typos
	if strings.Count(dj.StatusFmt, "%s") != 1 {
		return nil, fmt.Errorf("statusFmt: expected 1 %%s, got %d", strings.Count(dj.StatusFmt, "%s"))
	}
	if strings.Count(dj.PriorityFmt, "%s") != 1 {
		return nil, fmt.Errorf("priorityFmt: expected 1 %%s, got %d", strings.Count(dj.PriorityFmt, "%s"))
	}
	if strings.Count(dj.ProjectFmt, "%s") != 1 {
		return nil, fmt.Errorf("projectFmt: expected 1 %%s, got %d", strings.Count(dj.ProjectFmt, "%s"))
	}
	if strings.Count(dj.CreatedFmt, "%s") != 2 {
		return nil, fmt.Errorf("createdFmt: expected 2 %%s, got %d", strings.Count(dj.CreatedFmt, "%s"))
	}
	if strings.Count(dj.JournalCreation, "%s") != 2 {
		return nil, fmt.Errorf("journalCreation: expected 2 %%s, got %d", strings.Count(dj.JournalCreation, "%s"))
	}
	if strings.Count(dj.JournalTransition, "%s") != 3 {
		return nil, fmt.Errorf("journalTransition: expected 3 %%s, got %d", strings.Count(dj.JournalTransition, "%s"))
	}
	if strings.Count(dj.JournalArchive, "%s") != 2 {
		return nil, fmt.Errorf("journalArchive: expected 2 %%s, got %d", strings.Count(dj.JournalArchive, "%s"))
	}
	// Validate new fields (T-0040)
	if dj.WarnNewProject == "" {
		return nil, fmt.Errorf("warnNewProject: empty")
	}
	if strings.Count(dj.WarnNewProject, "%s") != 1 {
		return nil, fmt.Errorf("warnNewProject: expected 1 %%s, got %d", strings.Count(dj.WarnNewProject, "%s"))
	}
	if dj.NoTickets == "" {
		return nil, fmt.Errorf("noTickets: empty")
	}
	if strings.Count(dj.NoTickets, "%s") != 1 {
		return nil, fmt.Errorf("noTickets: expected 1 %%s, got %d", strings.Count(dj.NoTickets, "%s"))
	}
	// T-0075: verbatim message, no %s slots — non-empty check only.
	if dj.ErrTitleCTL == "" {
		return nil, fmt.Errorf("errTitleCTL: empty")
	}
	// T-0065: verbatim message, no %s slots — non-empty check only.
	if dj.ErrJournalNewline == "" {
		return nil, fmt.Errorf("errJournalNewline: empty")
	}
	// T-0072: 1 %s slot (the offending argument).
	if dj.ErrTicketArgNonNumeric == "" {
		return nil, fmt.Errorf("errTicketArgNonNumeric: empty")
	}
	if strings.Count(dj.ErrTicketArgNonNumeric, "%s") != 1 {
		return nil, fmt.Errorf("errTicketArgNonNumeric: expected 1 %%s, got %d", strings.Count(dj.ErrTicketArgNonNumeric, "%s"))
	}
	// T-0068: 1 %s slot (the offending flag argument).
	if dj.ErrUnknownFlag == "" {
		return nil, fmt.Errorf("errUnknownFlag: empty")
	}
	if strings.Count(dj.ErrUnknownFlag, "%s") != 1 {
		return nil, fmt.Errorf("errUnknownFlag: expected 1 %%s, got %d", strings.Count(dj.ErrUnknownFlag, "%s"))
	}

	// T-0080: CLI keys — non-empty for all 28, placeholder counts per key.
	if dj.ErrNoClosedTicketsToArchive == "" {
		return nil, fmt.Errorf("errNoClosedTicketsToArchive: empty")
	}
	if dj.ErrAlreadyArchived == "" {
		return nil, fmt.Errorf("errAlreadyArchived: empty")
	}
	if dj.ErrArchiveOnlyDoneClosed == "" {
		return nil, fmt.Errorf("errArchiveOnlyDoneClosed: empty")
	}
	if strings.Count(dj.ErrArchiveOnlyDoneClosed, "%s") != 1 {
		return nil, fmt.Errorf("errArchiveOnlyDoneClosed: expected 1 %%s, got %d", strings.Count(dj.ErrArchiveOnlyDoneClosed, "%s"))
	}
	if dj.ErrFileAlreadyExists == "" {
		return nil, fmt.Errorf("errFileAlreadyExists: empty")
	}
	if strings.Count(dj.ErrFileAlreadyExists, "%s") != 1 {
		return nil, fmt.Errorf("errFileAlreadyExists: expected 1 %%s, got %d", strings.Count(dj.ErrFileAlreadyExists, "%s"))
	}
	if dj.MsgInitialized == "" {
		return nil, fmt.Errorf("msgInitialized: empty")
	}
	if strings.Count(dj.MsgInitialized, "%s") != 1 {
		return nil, fmt.Errorf("msgInitialized: expected 1 %%s, got %d", strings.Count(dj.MsgInitialized, "%s"))
	}
	if dj.ErrInitError == "" {
		return nil, fmt.Errorf("errInitError: empty")
	}
	if strings.Count(dj.ErrInitError, "%v") != 1 {
		return nil, fmt.Errorf("errInitError: expected 1 %%v, got %d", strings.Count(dj.ErrInitError, "%v"))
	}
	if dj.ErrConflict == "" {
		return nil, fmt.Errorf("errConflict: empty")
	}
	if strings.Count(dj.ErrConflict, "%s") != 1 {
		return nil, fmt.Errorf("errConflict: expected 1 %%s, got %d", strings.Count(dj.ErrConflict, "%s"))
	}
	if dj.ErrFlagPRequiresValue == "" {
		return nil, fmt.Errorf("errFlagPRequiresValue: empty")
	}
	if dj.ErrFlagPRequiresNonEmpty == "" {
		return nil, fmt.Errorf("errFlagPRequiresNonEmpty: empty")
	}
	if dj.ErrRepeatedFlagJson == "" {
		return nil, fmt.Errorf("errRepeatedFlagJson: empty")
	}
	if dj.ErrUnknownFlagList == "" {
		return nil, fmt.Errorf("errUnknownFlagList: empty")
	}
	if strings.Count(dj.ErrUnknownFlagList, "%s") != 1 {
		return nil, fmt.Errorf("errUnknownFlagList: expected 1 %%s, got %d", strings.Count(dj.ErrUnknownFlagList, "%s"))
	}
	if dj.ErrListOneArgMax == "" {
		return nil, fmt.Errorf("errListOneArgMax: empty")
	}
	if dj.ErrListFilterInvalid == "" {
		return nil, fmt.Errorf("errListFilterInvalid: empty")
	}
	if dj.ErrTitleStartsWithDash == "" {
		return nil, fmt.Errorf("errTitleStartsWithDash: empty")
	}
	if dj.ErrTitleEmpty == "" {
		return nil, fmt.Errorf("errTitleEmpty: empty")
	}
	if dj.ErrTypeInvalid == "" {
		return nil, fmt.Errorf("errTypeInvalid: empty")
	}
	if dj.ErrPriorityInvalid == "" {
		return nil, fmt.Errorf("errPriorityInvalid: empty")
	}
	if dj.WarnTicketsSavedTo == "" {
		return nil, fmt.Errorf("warnTicketsSavedTo: empty")
	}
	if strings.Count(dj.WarnTicketsSavedTo, "%s") != 1 {
		return nil, fmt.Errorf("warnTicketsSavedTo: expected 1 %%s, got %d", strings.Count(dj.WarnTicketsSavedTo, "%s"))
	}
	if dj.ErrUnknownArgument == "" {
		return nil, fmt.Errorf("errUnknownArgument: empty")
	}
	if strings.Count(dj.ErrUnknownArgument, "%s") != 1 {
		return nil, fmt.Errorf("errUnknownArgument: expected 1 %%s, got %d", strings.Count(dj.ErrUnknownArgument, "%s"))
	}
	if dj.ErrFlagRequiresValue == "" {
		return nil, fmt.Errorf("errFlagRequiresValue: empty")
	}
	if strings.Count(dj.ErrFlagRequiresValue, "%s") != 1 {
		return nil, fmt.Errorf("errFlagRequiresValue: expected 1 %%s, got %d", strings.Count(dj.ErrFlagRequiresValue, "%s"))
	}
	if dj.ErrStatusInvalid == "" {
		return nil, fmt.Errorf("errStatusInvalid: empty")
	}
	if dj.ErrTicketAlreadyStatus == "" {
		return nil, fmt.Errorf("errTicketAlreadyStatus: empty")
	}
	if strings.Count(dj.ErrTicketAlreadyStatus, "%d") != 1 {
		return nil, fmt.Errorf("errTicketAlreadyStatus: expected 1 %%d, got %d", strings.Count(dj.ErrTicketAlreadyStatus, "%d"))
	}
	if strings.Count(dj.ErrTicketAlreadyStatus, "%s") != 1 {
		return nil, fmt.Errorf("errTicketAlreadyStatus: expected 1 %%s, got %d", strings.Count(dj.ErrTicketAlreadyStatus, "%s"))
	}
	if dj.WarnExtraArgsIgnored == "" {
		return nil, fmt.Errorf("warnExtraArgsIgnored: empty")
	}
	if strings.Count(dj.WarnExtraArgsIgnored, "%s") != 1 {
		return nil, fmt.Errorf("warnExtraArgsIgnored: expected 1 %%s, got %d", strings.Count(dj.WarnExtraArgsIgnored, "%s"))
	}
	if dj.ErrTicketNotFound == "" {
		return nil, fmt.Errorf("errTicketNotFound: empty")
	}
	if strings.Count(dj.ErrTicketNotFound, "%s") != 1 {
		return nil, fmt.Errorf("errTicketNotFound: expected 1 %%s, got %d", strings.Count(dj.ErrTicketNotFound, "%s"))
	}
	if dj.HintUseList == "" {
		return nil, fmt.Errorf("hintUseList: empty")
	}
	if dj.HintNumbersStartAtOne == "" {
		return nil, fmt.Errorf("hintNumbersStartAtOne: empty")
	}
	if dj.HintStoreEmpty == "" {
		return nil, fmt.Errorf("hintStoreEmpty: empty")
	}
	if dj.HintNumbersUpTo == "" {
		return nil, fmt.Errorf("hintNumbersUpTo: empty")
	}
	if strings.Count(dj.HintNumbersUpTo, "%d") != 1 {
		return nil, fmt.Errorf("hintNumbersUpTo: expected 1 %%d, got %d", strings.Count(dj.HintNumbersUpTo, "%d"))
	}

	return &dict{
		headers:                     headers,
		statusFmt:                   dj.StatusFmt,
		priorityFmt:                 dj.PriorityFmt,
		projectFmt:                  dj.ProjectFmt,
		createdFmt:                  dj.CreatedFmt,
		detailsStub:                 dj.DetailsStub,
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
		warnExtraArgsIgnored:        dj.WarnExtraArgsIgnored,
		errTicketNotFound:           dj.ErrTicketNotFound,
		hintUseList:                 dj.HintUseList,
		hintNumbersStartAtOne:       dj.HintNumbersStartAtOne,
		hintStoreEmpty:              dj.HintStoreEmpty,
		hintNumbersUpTo:             dj.HintNumbersUpTo,
	}, nil
}

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

// getDict returns the dictionary for lang.
func getDict(lang Lang) *dict {
	if d, ok := registry[lang]; ok {
		return d
	}
	return dictEN // fallback to EN for unknown languages
}

// IsKnownLang reports whether l was registered from a langs/*.json file
// in init(). Used by langPrefix to validate language codes before lookup.
func IsKnownLang(l Lang) bool {
	_, ok := registry[l]
	return ok
}

// sectionName is the canonical section identifier used internally and in raw
// template storage.
type sectionName string

const (
	secNameSummary      sectionName = "Summary"
	secNameDetails      sectionName = "Details"
	secNameUserComments sectionName = "User comments"
	secNameComments     sectionName = "Comments"
	secNameJournal      sectionName = "Journal"
)

// matchSectionHeader tries to recognize line as a section header in any known
// dictionary. It returns (canonicalName, langSignal, dictMatched, ok):
// langSignal is the dict lang for an exact suffix match, LangEN for plain EN
// headers and free suffixes; dictMatched reports an exact dict form (plain
// EN counts, a free suffix does not) for detectLang (LOCK D4 v2).
func matchSectionHeader(line string) (sectionName, Lang, bool, bool) {
	// Try all dicts: match "## Name (suffix)" against dict headers.
	for _, entry := range allDicts {
		for i, h := range entry.d.headers {
			if line == h {
				return sectionNames[i], entry.lang, true, true
			}
		}
	}

	// Try plain "## Name" against EN canonical names (no suffix): an
	// en-dict match.
	plainNames := []string{"## Summary", "## Details", "## User comments", "## Comments", "## Journal"}
	for i, plain := range plainNames {
		if line == plain {
			return sectionNames[i], LangEN, true, true
		}
	}

	// Try "## Name (free-suffix)" where Name matches EN canonical but suffix
	// does not match any dict. Post-guard: suffix must not contain "##".
	for i, canonical := range []string{"Summary", "Details", "User comments", "Comments", "Journal"} {
		prefix := "## " + canonical + " ("
		if strings.HasPrefix(line, prefix) && strings.HasSuffix(line, ")") {
			suffix := line[len(prefix) : len(line)-1]
			if validSuffix(suffix) {
				return sectionNames[i], LangEN, false, true
			}
		}
	}

	return "", LangEN, false, false
}

var sectionNames = [5]sectionName{secNameSummary, secNameDetails, secNameUserComments, secNameComments, secNameJournal}

// validSuffix checks suffix constraints: no nested parens, no "##".
func validSuffix(s string) bool {
	return !strings.Contains(s, "(") && !strings.Contains(s, ")") && !strings.Contains(s, "##")
}

// matchMetaLine tries to parse line as a meta line (Status/Priority/Created/
// Project). It returns (key, langSignal, dictMatched, values, ok): key is the
// canonical meta key; langSignal is the dict lang for an exact form match,
// LangEN otherwise; dictMatched reports an exact dict form (plain EN counts,
// a free suffix does not) for detectLang; values holds one value, or two
// for Created (timestamp, author).
func matchMetaLine(line string) (metaKey string, lang Lang, dictMatched bool, values []string, ok bool) {
	// Try all dicts: match emission forms against line.
	for _, entry := range allDicts {
		if k, v, ok := tryMetaDict(line, entry.d, entry.lang); ok {
			return k, entry.lang, true, v, true
		}
	}

	// Try plain EN forms (no suffix in label).
	enPlain := map[string]string{
		"Status":   "- Status: ",
		"Priority": "- Priority: ",
		"Project":  "- Project: ",
	}
	for k, prefix := range enPlain {
		if strings.HasPrefix(line, prefix) {
			val := line[len(prefix):]
			return k, LangEN, true, []string{val}, true
		}
	}
	if strings.HasPrefix(line, "- Created: ") {
		rest := line[len("- Created: "):]
		if parts := strings.Split(rest, " · by: "); len(parts) == 2 {
			return "Created", LangEN, true, parts, true
		}
	}

	// Try EN canonical with free suffix: "- Name (suffix): value".
	for k, canonical := range map[string]string{"Status": "Status", "Priority": "Priority", "Project": "Project"} {
		prefix := "- " + canonical + " ("
		if strings.HasPrefix(line, prefix) {
			if idx := strings.Index(line[len(prefix):], "): "); idx >= 0 {
				suffix := line[len(prefix) : len(prefix)+idx]
				if validSuffix(suffix) {
					val := line[len(prefix)+idx+3:]
					return k, LangEN, false, []string{val}, true
				}
			}
		}
	}
	// Created with free suffix: "- Created (suffix): ts · by (suffix): author"
	if strings.HasPrefix(line, "- Created (") {
		rest := line[len("- Created ("):]
		if idx := strings.Index(rest, "): "); idx >= 0 {
			suffix1 := rest[:idx]
			if !validSuffix(suffix1) {
				return "", LangEN, false, nil, false
			}
			rest = rest[idx+3:]
			if parts := strings.Split(rest, " · by ("); len(parts) == 2 {
				ts := parts[0]
				if idx := strings.Index(parts[1], "): "); idx >= 0 {
					suffix2 := parts[1][:idx]
					if validSuffix(suffix2) {
						author := parts[1][idx+3:]
						return "Created", LangEN, false, []string{ts, author}, true
					}
				}
			}
		}
	}

	return "", LangEN, false, nil, false
}

func tryMetaDict(line string, d *dict, lang Lang) (string, []string, bool) {
	// Status/Priority/Project: "- Label (suffix): value"
	for k, prefix := range map[string]string{
		"Status":   d.statusFmt[:len(d.statusFmt)-2], // strip " %s"
		"Priority": d.priorityFmt[:len(d.priorityFmt)-2],
		"Project":  d.projectFmt[:len(d.projectFmt)-2],
	} {
		if strings.HasPrefix(line, prefix) {
			val := line[len(prefix):]
			return k, []string{val}, true
		}
	}
	// Created: "- Created (suffix): ts · by (suffix): author"
	createdPrefix := d.createdFmt[:strings.Index(d.createdFmt, "%s")]
	if strings.HasPrefix(line, createdPrefix) {
		rest := line[len(createdPrefix):]
		// The format is "ts%s · by (suffix): %s" where first %s is the slot.
		// We split on the middle part " · by (" + suffix + "): ".
		// Simpler: split on " · by " and strip the label suffix from the second part.
		if parts := strings.Split(rest, " · by "); len(parts) == 2 {
			ts := parts[0]
			// Strip label suffix from author part: "(suffix): author" -> "author"
			authorPart := parts[1]
			if idx := strings.Index(authorPart, "): "); idx >= 0 {
				author := authorPart[idx+3:]
				return "Created", []string{ts, author}, true
			}
		}
	}
	return "", nil, false
}

// detectLang implements LOCK D4 v2: the first signal (header or meta line)
// whose suffix/form exactly matches a known dict — plain EN forms count as
// en-dict matches — decides the ticket language; free (non-dict) signals
// never decide. Only a file with NO dict-matched signal at all falls back to
// LangEN (supersedes "first signal wins": a free suffix no longer hides a
// dict signal later in the file).
func detectLang(data []byte) Lang {
	for pos := 0; pos < len(data); {
		start, end := lineBounds(data, pos)
		pos = end
		line := string(trimEOL(data[start:end]))

		// A dict-matched section header decides; free-suffix signals are
		// skipped (D4 v2 precedence).
		if _, lang, dictMatched, ok := matchSectionHeader(line); ok && dictMatched {
			return lang
		}
		if _, lang, dictMatched, _, ok := matchMetaLine(line); ok && dictMatched {
			return lang
		}
	}
	return LangEN
}

// isStubLine returns true if line is exactly a stub (detailsStub or
// userCommentsStub) from any known dictionary.
func isStubLine(line string) bool {
	for _, entry := range allDicts {
		if line == entry.d.detailsStub || line == entry.d.userCommentsStub {
			return true
		}
	}
	return false
}
