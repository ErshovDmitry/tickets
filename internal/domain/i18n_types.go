package domain

import (
	"errors"
)

// This file holds the i18n value types: the language identifier, the
// per-language dictionary shape, the init error classes and the canonical
// section identifiers shared by the parser and renderer.

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

	// T-0109: detailsFormat is the NEW empty-Details block (a multiline
	// HTML comment carrying the numbered-list rule). detailsStub stays the
	// LEGACY stub, emitted by Render for legacy tickets.
	detailsFormat string

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

	// T-0109: details resolver errors (file reference / stdin).
	errDetailsAtBare      string // no placeholders
	errDetailsFileRead    string // 1 %s: path, 1 %v: cause
	errDetailsStdinRead   string // 1 %v: error
	warnExtraArgsIgnored  string // 1 %s: extra args
	errTicketNotFound     string // 1 %s: argument
	hintUseList           string
	hintNumbersStartAtOne string
	hintStoreEmpty        string
	hintNumbersUpTo       string // 1 %d: max number

	// T-0157: a ticket number owned by more than one file.
	errAmbiguousNumber         string // 1 %d: number
	warnAmbiguousNumber        string // 1 %d: number, 1 %s: conflicting absolute paths
	warnAmbiguousNumberSkipped string // 1 %d: number, 1 %s: conflicting absolute paths
}

// Init error classes returned (wrapped) by loadFromFS and stored by the
// package initialisation for domain.Init.
// ErrInitFatal means no dictionary was published at all: the package keeps
// its safe empty state (empty registry, dictRU nil, dictEN a non-nil EMPTY
// dict, empty allDicts/patterns), so callers must abort.
// ErrInitDegraded means the mandatory EN dictionary is usable but at least
// one optional dictionary failed to load: the partial candidate is published
// and the broken language is simply unknown (getDict falls back to EN,
// IsKnownLang reports false).
var (
	ErrInitFatal    = errors.New("ticket: i18n initialization failed")
	ErrInitDegraded = errors.New("ticket: i18n initialization degraded")
)

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
