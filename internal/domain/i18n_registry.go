package domain

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// This file embeds the language dictionaries and owns their one-time
// loading, validation of that load and publication into package state.

//go:embed langs/*.json
var langsFS embed.FS

// dictEntry pairs a registered language with its dictionary. It is a named
// type (rather than an inline struct) because loadFromFS builds the list in
// a candidate before publishing it.
type dictEntry struct {
	lang Lang
	d    *dict
}

var (
	// dictRU is the Russian dictionary, loaded from embedded langs/ru.json.
	// It is nil when that file failed to load: the language is then not
	// registered at all (no alias) and getDict falls back to EN.
	dictRU *dict

	// dictEN is the mandatory English dictionary. A fatal initialisation
	// leaves a non-nil EMPTY dict here so getDict never has to panic.
	dictEN *dict

	allDicts []dictEntry

	registry map[Lang]*dict

	// initErr is the stored result of the one package-initialisation
	// attempt. domain.Init returns it unchanged on every call.
	initErr error
)

// candidate is the outcome of one dictionary load. loadFromFS never touches
// package state; publishCandidate is the only writer.
type candidate struct {
	registry map[Lang]*dict
	dictRU   *dict
	dictEN   *dict
	allDicts []dictEntry
}

// init installs the dictionaries exactly once. It never panics: a fatal load
// failure leaves the safe empty state in place and only stores the error for
// domain.Init (the CLI turns it into one stderr line and exit 1).
func init() {
	// Safe empty state first: even on fatal failure getDict, IsKnownLang and
	// the parser must remain callable without panicking.
	registry = make(map[Lang]*dict)
	dictRU = nil
	dictEN = &dict{}
	allDicts = []dictEntry{}
	journalDictPatterns = make(map[*dict]journalPatterns)

	initErr = bootstrap(langsFS)
}

// classifyInitErr maps the loader result onto the two public error classes.
// nil stays nil and an already classified error is returned unchanged;
// anything else is an unrecognised failure and is normalized to ErrInitFatal,
// so consumers never see a third class and can rely on errors.Is alone.
func classifyInitErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrInitFatal), errors.Is(err, ErrInitDegraded):
		return err
	default:
		return fmt.Errorf("%w: %v", ErrInitFatal, err)
	}
}

// bootstrap performs the one dictionary load and publishes the result
// according to its class: nil and degraded publish the candidate (EN is
// usable in both cases), while a fatal error publishes nothing and leaves the
// caller's state untouched. It is separate from init so tests can exercise
// the publication decision against any fs.FS.
func bootstrap(fsys fs.FS) error {
	cand, err := loadFromFS(fsys)
	err = classifyInitErr(err)
	if err == nil || errors.Is(err, ErrInitDegraded) {
		publishCandidate(&cand)
	}
	return err
}

// Init returns the stored package-initialisation result: nil on success,
// ErrInitDegraded when an optional language was skipped, and ErrInitFatal
// when no dictionary is usable. It is the single entry point for consumers
// checking i18n health; the result was stored by package init, so repeated
// calls are idempotent.
func Init() error {
	return initErr
}

// loadFromFS loads every langs/*.json dictionary from fsys into a candidate.
// It never mutates package state. EN is mandatory: any failure on it (and a
// ReadDir failure) is fatal. A failure of an optional language (RU or any
// other code) degrades the result: that language is skipped, not registered,
// and the rest of the candidate is returned with ErrInitDegraded. A duplicate
// language code (e.g. RU.json next to ru.json) is fatal for EVERY language,
// EN included. It is discovered when the second entry for that code is
// reached, so the first copy may already have been read and parsed; the
// returned class is fatal regardless, even when that first copy was malformed.
func loadFromFS(fsys fs.FS) (candidate, error) {
	cand := candidate{registry: make(map[Lang]*dict)}
	var failures []string

	entries, err := fs.ReadDir(fsys, "langs")
	if err != nil {
		return cand, fmt.Errorf("%w: read langs dir: %v", ErrInitFatal, err)
	}

	// Track the first-seen filename per language code to detect duplicates
	// such as RU.json vs ru.json.
	firstSeen := make(map[Lang]string)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		fname := "langs/" + entry.Name()
		code := strings.ToLower(strings.TrimSuffix(entry.Name(), ".json"))
		lang := Lang(code)

		// A duplicate code is fatal regardless of the language and is
		// rejected as soon as the second entry for that code is reached
		// (the first copy may already be parsed): two files must never race
		// for one registry slot, and a malformed first copy must not
		// downgrade the duplicate into a degraded parse failure.
		if prior, dup := firstSeen[lang]; dup {
			return cand, fmt.Errorf("%w: duplicate language code %q from %s and %s", ErrInitFatal, code, prior, fname)
		}
		firstSeen[lang] = fname

		// fail classifies one local failure. It returns a non-nil fatal error
		// for EN and nil for an optional language (recording the reason).
		fail := func(format string, args ...any) error {
			reason := fmt.Sprintf(format, args...)
			if lang == LangEN {
				return fmt.Errorf("%w: %s", ErrInitFatal, reason)
			}
			failures = append(failures, reason)
			return nil
		}

		data, err := fs.ReadFile(fsys, fname)
		if err != nil {
			if fatal := fail("load %s: %v", fname, err); fatal != nil {
				return cand, fatal
			}
			continue
		}
		d, err := loadDict(data)
		if err != nil {
			if fatal := fail("parse %s: %v", fname, err); fatal != nil {
				return cand, fatal
			}
			continue
		}
		cand.registry[lang] = d
	}

	cand.dictEN = cand.registry[LangEN]
	if cand.dictEN == nil {
		return cand, fmt.Errorf("%w: missing usable en.json", ErrInitFatal)
	}
	cand.dictRU = cand.registry[LangRU]
	cand.allDicts = buildAllDicts(cand.registry)

	if len(failures) > 0 {
		// One line: the CLI prints the returned error verbatim.
		return cand, fmt.Errorf("%w: %s", ErrInitDegraded, strings.Join(failures, "; "))
	}
	return cand, nil
}

// buildAllDicts orders the registered dictionaries: RU first, then EN, then
// every other language alphabetically. A missing RU simply drops out of the
// list (degraded initialisation).
func buildAllDicts(reg map[Lang]*dict) []dictEntry {
	out := make([]dictEntry, 0, len(reg))
	if d := reg[LangRU]; d != nil {
		out = append(out, dictEntry{LangRU, d})
	}
	if d := reg[LangEN]; d != nil {
		out = append(out, dictEntry{LangEN, d})
	}
	others := make([]Lang, 0, len(reg))
	for code := range reg {
		if code != LangRU && code != LangEN {
			others = append(others, code)
		}
	}
	sort.Slice(others, func(i, j int) bool { return others[i] < others[j] })
	for _, code := range others {
		out = append(out, dictEntry{code, reg[code]})
	}
	return out
}

// publishCandidate installs a loaded candidate into the package globals. The
// assignment order is deliberate: registry and the RU/EN views first, then
// allDicts, and only then the journal patterns compiled from allDicts. A nil
// registry or allDicts is normalized to a non-nil empty value, so the
// published state never has to distinguish "absent" from "empty".
func publishCandidate(c *candidate) {
	registry = c.registry
	if registry == nil {
		registry = make(map[Lang]*dict)
	}
	dictRU = c.dictRU
	dictEN = c.dictEN
	if dictEN == nil {
		dictEN = &dict{}
	}
	allDicts = c.allDicts
	if allDicts == nil {
		allDicts = []dictEntry{}
	}
	journalDictPatterns = buildJournalPatterns()
}

// getDict returns the dictionary for lang. It is nil-safe in every state,
// including the safe empty state after a fatal initialisation: unknown or
// unavailable languages fall back to EN (and to an empty dict only when even
// dictEN is nil, which package init never leaves behind).
func getDict(lang Lang) *dict {
	if d, ok := registry[lang]; ok && d != nil {
		return d
	}
	if dictEN != nil {
		return dictEN // fallback to EN for unknown languages
	}
	return &dict{}
}

// IsKnownLang reports whether l was registered from a valid, loadable
// langs/*.json file in init(). A language whose file was missing or broken
// is not registered (no alias), so langPrefix falls back to EN.
func IsKnownLang(l Lang) bool {
	d, ok := registry[l]
	return ok && d != nil
}
