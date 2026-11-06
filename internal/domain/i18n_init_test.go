package domain

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

// i18nGlobals is a snapshot of the package-level i18n state. Tests take it
// before exercising loadFromFS/publishCandidate and restore it afterwards so
// the rest of the package keeps the production dictionaries.
//
// Scope: the snapshot copies the MAP and SLICE STRUCTURE only (registry,
// journalDictPatterns, allDicts). The *dict values they reference are shared
// with the snapshot, not deep-copied: the tests treat every loaded dict as
// immutable, so they never mutate a dict through one side of a snapshot. A
// test that needed to mutate dict fields would have to clone them itself.
type i18nGlobals struct {
	registry map[Lang]*dict
	dictRU   *dict
	dictEN   *dict
	allDicts []dictEntry
	patterns map[*dict]journalPatterns
}

func snapshotI18nGlobals() i18nGlobals {
	return i18nGlobals{
		registry: cloneDictMap(registry),
		dictRU:   dictRU,
		dictEN:   dictEN,
		allDicts: cloneDictEntries(allDicts),
		patterns: cloneJournalPatternMap(journalDictPatterns),
	}
}

// cloneDictMap copies the language→dict MAP STRUCTURE (nil stays nil). The
// *dict values stay shared with the live registry: this guards against the
// map being rebound or mutated in place (add/delete/replace an entry), not
// against mutation of a dict's fields, which the tests treat as immutable.
func cloneDictMap(in map[Lang]*dict) map[Lang]*dict {
	return maps.Clone(in)
}

// cloneDictEntries copies the dict slice. The *dict values are shared and
// treated as immutable by the tests; only the slice header/backing array is
// isolated. nil stays nil and an empty-but-non-nil slice stays non-nil: the
// safe empty state uses []dictEntry{}, and append-to-nil would silently turn
// it into nil, making a snapshot/restore round trip fail DeepEqual.
func cloneDictEntries(in []dictEntry) []dictEntry {
	if in == nil {
		return nil
	}
	out := make([]dictEntry, len(in))
	copy(out, in)
	return out
}

// cloneJournalPatternMap copies the dict→patterns MAP STRUCTURE (nil stays
// nil). The *dict keys and the journalPatterns values (which hold compiled
// *regexp.Regexp pointers) are shared with the live map, not deep-copied:
// tests treat the dictionaries as immutable, and rebinding the map variable
// is what the snapshot has to survive.
func cloneJournalPatternMap(in map[*dict]journalPatterns) map[*dict]journalPatterns {
	return maps.Clone(in)
}

func restoreI18nGlobals(g i18nGlobals) {
	registry = cloneDictMap(g.registry)
	dictRU = g.dictRU
	dictEN = g.dictEN
	allDicts = cloneDictEntries(g.allDicts)
	journalDictPatterns = cloneJournalPatternMap(g.patterns)
}

// embeddedLangsMapFS builds a MapFS holding the real embedded dictionaries,
// so the happy path is exercised against the production bytes without
// touching langsFS.
func embeddedLangsMapFS(t *testing.T) fstest.MapFS {
	t.Helper()
	m := fstest.MapFS{}
	for _, name := range []string{"ru.json", "en.json"} {
		data, err := fs.ReadFile(langsFS, "langs/"+name)
		if err != nil {
			t.Fatalf("read embedded langs/%s: %v", name, err)
		}
		m["langs/"+name] = &fstest.MapFile{Data: data}
	}
	return m
}

// failingLangsFS wraps another fs.FS and forces a ReadDir or ReadFile
// failure for the selected paths.
type failingLangsFS struct {
	fsys     fs.FS
	failDir  bool
	failRead map[string]bool
}

func (f failingLangsFS) Open(name string) (fs.File, error) {
	if f.failRead[name] {
		return nil, errors.New("forced open failure")
	}
	return f.fsys.Open(name)
}

func (f failingLangsFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if f.failDir {
		return nil, errors.New("forced readdir failure")
	}
	if r, ok := f.fsys.(fs.ReadDirFS); ok {
		return r.ReadDir(name)
	}
	return fs.ReadDir(f.fsys, name)
}

// journalPatternText returns the source text of a compiled pattern, using an
// explicit sentinel for a nil pattern: a malformed template yields a nil
// regexp, and (*regexp.Regexp).String is not nil-safe.
func journalPatternText(re *regexp.Regexp) string {
	if re == nil {
		return "<nil>"
	}
	return re.String()
}

func TestGetDictAndIsKnownLangSafeEmptyState(t *testing.T) {
	saved := snapshotI18nGlobals()
	t.Cleanup(func() { restoreI18nGlobals(saved) })

	// The state package init leaves behind after a fatal load.
	registry = make(map[Lang]*dict)
	dictRU = nil
	dictEN = &dict{}
	allDicts = []dictEntry{}
	journalDictPatterns = make(map[*dict]journalPatterns)

	if got := getDict(LangRU); got != dictEN {
		t.Fatal("getDict(RU) must fall back to the non-nil empty EN dict")
	}
	if IsKnownLang(LangRU) || IsKnownLang(LangEN) {
		t.Fatal("IsKnownLang must report false for every language in the empty state")
	}

	// Even a nil dictEN must not make getDict panic.
	dictEN = nil
	if got := getDict(Lang("zz")); got == nil {
		t.Fatal("getDict must never return nil")
	}
}

func TestInitIsStoredAndIdempotent(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init() = %v, want nil with the embedded dictionaries", err)
	}
	if err := Init(); err != nil {
		t.Fatalf("second Init() = %v, want the stored nil", err)
	}
}

func TestClassifyInitErr(t *testing.T) {
	if got := classifyInitErr(nil); got != nil {
		t.Fatalf("classifyInitErr(nil) = %v, want nil", got)
	}
	fatal := fmt.Errorf("%w: en.json broken", ErrInitFatal)
	if got := classifyInitErr(fatal); got != fatal {
		t.Fatalf("classifyInitErr(fatal) = %v, want the error unchanged", got)
	}
	degraded := fmt.Errorf("%w: ru.json broken", ErrInitDegraded)
	if got := classifyInitErr(degraded); got != degraded {
		t.Fatalf("classifyInitErr(degraded) = %v, want the error unchanged", got)
	}
	unknown := errors.New("some new failure class")
	got := classifyInitErr(unknown)
	if !errors.Is(got, ErrInitFatal) {
		t.Fatalf("classifyInitErr(unknown) = %v, want errors.Is(ErrInitFatal)", got)
	}
	if errors.Is(got, ErrInitDegraded) {
		t.Fatalf("classifyInitErr(unknown) = %v, must not match ErrInitDegraded", got)
	}
	if !strings.Contains(got.Error(), unknown.Error()) {
		t.Fatalf("normalized error %q lost the original reason %q", got, unknown)
	}
}
