package domain

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"reflect"
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

func TestLoadFromFS(t *testing.T) {
	// No t.Parallel anywhere in this file: the tests replace/snapshot the
	// package-level dictionaries.
	tests := []struct {
		name       string
		build      func(t *testing.T) fs.FS
		wantErr    error
		wantNotErr error
		check      func(t *testing.T, c candidate)
	}{
		{
			name:  "successful discovery",
			build: func(t *testing.T) fs.FS { return embeddedLangsMapFS(t) },
			check: func(t *testing.T, c candidate) {
				t.Helper()
				if c.registry[LangRU] == nil || c.registry[LangEN] == nil {
					t.Fatalf("registry misses RU/EN: %v", c.registry)
				}
				if c.dictRU == nil || c.dictEN == nil {
					t.Fatal("dictRU/dictEN must be non-nil")
				}
				if len(c.allDicts) < 2 || c.allDicts[0].lang != LangRU || c.allDicts[1].lang != LangEN {
					t.Fatalf("allDicts order = %+v, want RU then EN", c.allDicts)
				}
			},
		},
		{
			name: "non-json entries are ignored",
			build: func(t *testing.T) fs.FS {
				m := embeddedLangsMapFS(t)
				m["langs/README.txt"] = &fstest.MapFile{Data: []byte("not a dictionary")}
				m["langs/nested"] = &fstest.MapFile{Mode: fs.ModeDir}
				return m
			},
			check: func(t *testing.T, c candidate) {
				t.Helper()
				if len(c.registry) != 2 {
					t.Fatalf("registry = %v, want exactly RU+EN", c.registry)
				}
			},
		},
		{
			name: "readdir failure is fatal",
			build: func(t *testing.T) fs.FS {
				return failingLangsFS{fsys: embeddedLangsMapFS(t), failDir: true}
			},
			wantErr:    ErrInitFatal,
			wantNotErr: ErrInitDegraded,
		},
		{
			name: "missing EN is fatal",
			build: func(t *testing.T) fs.FS {
				m := embeddedLangsMapFS(t)
				delete(m, "langs/en.json")
				return m
			},
			wantErr:    ErrInitFatal,
			wantNotErr: ErrInitDegraded,
		},
		{
			name: "EN read error is fatal",
			build: func(t *testing.T) fs.FS {
				return failingLangsFS{
					fsys:     embeddedLangsMapFS(t),
					failRead: map[string]bool{"langs/en.json": true},
				}
			},
			wantErr:    ErrInitFatal,
			wantNotErr: ErrInitDegraded,
		},
		{
			name: "malformed EN is fatal",
			build: func(t *testing.T) fs.FS {
				m := embeddedLangsMapFS(t)
				m["langs/en.json"] = &fstest.MapFile{Data: []byte("{")}
				return m
			},
			wantErr:    ErrInitFatal,
			wantNotErr: ErrInitDegraded,
		},
		{
			name: "duplicate EN code is fatal",
			build: func(t *testing.T) fs.FS {
				m := embeddedLangsMapFS(t)
				m["langs/EN.json"] = &fstest.MapFile{Data: m["langs/en.json"].Data}
				return m
			},
			wantErr:    ErrInitFatal,
			wantNotErr: ErrInitDegraded,
		},
		{
			name: "RU read error degrades with partial candidate",
			build: func(t *testing.T) fs.FS {
				return failingLangsFS{
					fsys:     embeddedLangsMapFS(t),
					failRead: map[string]bool{"langs/ru.json": true},
				}
			},
			wantErr:    ErrInitDegraded,
			wantNotErr: ErrInitFatal,
			check:      checkDegradedRU,
		},
		{
			name: "malformed RU degrades with partial candidate",
			build: func(t *testing.T) fs.FS {
				m := embeddedLangsMapFS(t)
				m["langs/ru.json"] = &fstest.MapFile{Data: []byte("{")}
				return m
			},
			wantErr:    ErrInitDegraded,
			wantNotErr: ErrInitFatal,
			check:      checkDegradedRU,
		},
		{
			name: "duplicate RU code is fatal",
			build: func(t *testing.T) fs.FS {
				m := embeddedLangsMapFS(t)
				m["langs/RU.json"] = &fstest.MapFile{Data: m["langs/ru.json"].Data}
				return m
			},
			wantErr:    ErrInitFatal,
			wantNotErr: ErrInitDegraded,
		},
		{
			name: "duplicate stays fatal when the first copy is malformed",
			build: func(t *testing.T) fs.FS {
				m := embeddedLangsMapFS(t)
				// RU.json (earlier in ReadDir order) is malformed, so the first
				// copy is actually parsed and fails; reaching RU.json must still
				// return the fatal duplicate class, not a degraded parse error.
				m["langs/RU.json"] = &fstest.MapFile{Data: []byte("{")}
				return m
			},
			wantErr:    ErrInitFatal,
			wantNotErr: ErrInitDegraded,
		},
		{
			name: "duplicate third optional code is fatal",
			build: func(t *testing.T) fs.FS {
				m := embeddedLangsMapFS(t)
				m["langs/xx.json"] = &fstest.MapFile{Data: m["langs/en.json"].Data}
				m["langs/XX.json"] = &fstest.MapFile{Data: m["langs/en.json"].Data}
				return m
			},
			wantErr:    ErrInitFatal,
			wantNotErr: ErrInitDegraded,
		},
		{
			name: "third optional languages follow EN alphabetically",
			build: func(t *testing.T) fs.FS {
				m := embeddedLangsMapFS(t)
				m["langs/de.json"] = &fstest.MapFile{Data: m["langs/en.json"].Data}
				m["langs/cs.json"] = &fstest.MapFile{Data: m["langs/en.json"].Data}
				return m
			},
			check: func(t *testing.T, c candidate) {
				t.Helper()
				if len(c.registry) != 4 {
					t.Fatalf("registry = %v, want RU+EN+cs+de", c.registry)
				}
				want := []Lang{LangRU, LangEN, "cs", "de"}
				if len(c.allDicts) != len(want) {
					t.Fatalf("allDicts = %+v, want %v", c.allDicts, want)
				}
				for i, lang := range want {
					if c.allDicts[i].lang != lang {
						t.Fatalf("allDicts[%d].lang = %q, want %q (order %v)", i, c.allDicts[i].lang, lang, want)
					}
					if c.allDicts[i].d == nil {
						t.Fatalf("allDicts[%d] has a nil dict", i)
					}
				}
			},
		},
		{
			name: "failed third optional language degrades and is not registered",
			build: func(t *testing.T) fs.FS {
				m := embeddedLangsMapFS(t)
				m["langs/xx.json"] = &fstest.MapFile{Data: []byte("{")}
				return m
			},
			wantErr:    ErrInitDegraded,
			wantNotErr: ErrInitFatal,
			check: func(t *testing.T, c candidate) {
				t.Helper()
				if _, ok := c.registry[Lang("xx")]; ok {
					t.Fatal("a broken third language must not be registered")
				}
				if len(c.registry) != 2 || c.dictRU == nil || c.dictEN == nil {
					t.Fatalf("registry = %v, want RU+EN only", c.registry)
				}
				if len(c.allDicts) != 2 {
					t.Fatalf("allDicts = %+v, want RU+EN only", c.allDicts)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := snapshotI18nGlobals()
			defer func() {
				if !reflect.DeepEqual(snapshotI18nGlobals(), before) {
					t.Fatal("loadFromFS mutated package globals")
				}
			}()

			cand, err := loadFromFS(tc.build(t))
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("loadFromFS error = %v, want nil", err)
				}
			} else {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("loadFromFS error = %v, want errors.Is(%v)", err, tc.wantErr)
				}
				if tc.wantNotErr != nil && errors.Is(err, tc.wantNotErr) {
					t.Fatalf("error %v also matches %v, classes must be exclusive", err, tc.wantNotErr)
				}
				if tc.wantErr == ErrInitDegraded && strings.Contains(err.Error(), "\n") {
					t.Fatalf("degraded error must be one line, got %q", err.Error())
				}
			}
			if tc.check != nil {
				tc.check(t, cand)
			}
		})
	}
}

func checkDegradedRU(t *testing.T, c candidate) {
	t.Helper()
	if c.dictEN == nil {
		t.Fatal("dictEN must stay non-nil in a degraded candidate")
	}
	if c.dictRU != nil {
		t.Fatal("dictRU must be nil when ru.json failed")
	}
	if _, ok := c.registry[LangRU]; ok {
		t.Fatal("a broken RU must not be registered (no alias)")
	}
	if len(c.allDicts) == 0 || c.allDicts[0].lang != LangEN {
		t.Fatalf("allDicts = %+v, want EN first when RU is unavailable", c.allDicts)
	}
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

func TestPublishCandidateSnapshotRestore(t *testing.T) {
	saved := snapshotI18nGlobals()
	t.Cleanup(func() { restoreI18nGlobals(saved) })

	cand, err := loadFromFS(embeddedLangsMapFS(t))
	if err != nil {
		t.Fatalf("loadFromFS: %v", err)
	}
	publishCandidate(&cand)
	if registry[LangRU] != cand.dictRU || registry[LangEN] != cand.dictEN {
		t.Fatal("registry was not installed from the candidate")
	}
	if dictRU != cand.dictRU || dictEN != cand.dictEN {
		t.Fatal("dictRU/dictEN were not installed from the candidate")
	}
	if !reflect.DeepEqual(allDicts, cand.allDicts) {
		t.Fatal("allDicts was not installed from the candidate")
	}
	// Compare the journal patterns by ACTUAL key and content: one entry per
	// published dict, each compiled from that dict's own templates. A length
	// check alone would pass for a map keyed by the wrong dicts or holding
	// stale/compiled-from-other-dict patterns.
	wantPatterns := buildJournalPatterns()
	if len(journalDictPatterns) != len(wantPatterns) {
		t.Fatalf("journalDictPatterns = %d dicts, want %d", len(journalDictPatterns), len(wantPatterns))
	}
	for _, entry := range allDicts {
		got, ok := journalDictPatterns[entry.d]
		if !ok {
			t.Fatalf("journalDictPatterns has no entry for the %s dict", entry.lang)
		}
		if journalPatternText(got.creation) != journalPatternText(wantPatterns[entry.d].creation) ||
			journalPatternText(got.archive) != journalPatternText(wantPatterns[entry.d].archive) ||
			journalPatternText(got.transition) != journalPatternText(wantPatterns[entry.d].transition) {
			t.Fatalf("journalDictPatterns[%s] = %+v, want the patterns compiled from that dict", entry.lang, got)
		}
	}

	restoreI18nGlobals(saved)
	if !reflect.DeepEqual(snapshotI18nGlobals(), saved) {
		t.Fatal("restoreI18nGlobals did not reproduce the snapshot")
	}
}

func TestPublishCandidateNormalizesNilCollections(t *testing.T) {
	saved := snapshotI18nGlobals()
	t.Cleanup(func() { restoreI18nGlobals(saved) })

	en := &dict{}
	publishCandidate(&candidate{registry: map[Lang]*dict{LangEN: en}, dictEN: en})

	if registry == nil || allDicts == nil {
		t.Fatalf("registry nil = %v, allDicts nil = %v, want both non-nil", registry == nil, allDicts == nil)
	}
	if len(allDicts) != 0 {
		t.Fatalf("allDicts = %+v, want empty", allDicts)
	}
	if journalDictPatterns == nil {
		t.Fatal("journalDictPatterns must be non-nil after publication")
	}
	if len(journalDictPatterns) != 0 {
		t.Fatalf("journalDictPatterns = %d entries, want 0 for an empty allDicts", len(journalDictPatterns))
	}
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

func TestBootstrapPublishesOnlyUsableCandidates(t *testing.T) {
	saved := snapshotI18nGlobals()
	t.Cleanup(func() { restoreI18nGlobals(saved) })

	t.Run("fatal publishes nothing", func(t *testing.T) {
		// A recognizable marker state: a fatal load must leave it untouched.
		marker := &dict{}
		registry = map[Lang]*dict{LangEN: marker}
		dictRU = nil
		dictEN = marker
		allDicts = []dictEntry{{LangEN, marker}}
		journalDictPatterns = map[*dict]journalPatterns{marker: {}}
		before := snapshotI18nGlobals()

		err := bootstrap(fstest.MapFS{}) // no langs/en.json: fatal
		if !errors.Is(err, ErrInitFatal) {
			t.Fatalf("bootstrap = %v, want ErrInitFatal", err)
		}
		if !reflect.DeepEqual(snapshotI18nGlobals(), before) {
			t.Fatal("a fatal load must not publish the candidate")
		}
	})

	t.Run("degraded publishes the partial candidate", func(t *testing.T) {
		m := embeddedLangsMapFS(t)
		m["langs/ru.json"] = &fstest.MapFile{Data: []byte("{")}

		err := bootstrap(m)
		if !errors.Is(err, ErrInitDegraded) {
			t.Fatalf("bootstrap = %v, want ErrInitDegraded", err)
		}
		if dictEN == nil || dictRU != nil {
			t.Fatal("degraded publication must install EN and drop RU")
		}
		if _, ok := registry[LangRU]; ok {
			t.Fatal("the broken RU must not be registered")
		}
		if len(allDicts) != 1 || allDicts[0].lang != LangEN {
			t.Fatalf("allDicts = %+v, want EN only", allDicts)
		}
		if len(journalDictPatterns) != 1 {
			t.Fatalf("journalDictPatterns = %d dicts, want 1", len(journalDictPatterns))
		}
		if !IsKnownLang(LangEN) || IsKnownLang(LangRU) {
			t.Fatal("IsKnownLang must report EN known and RU unknown after degradation")
		}
	})
}

func TestPublishCandidateNilSafety(t *testing.T) {
	saved := snapshotI18nGlobals()
	t.Cleanup(func() { restoreI18nGlobals(saved) })

	publishCandidate(&candidate{})
	if registry == nil {
		t.Fatal("a nil candidate registry must be replaced by an empty map")
	}
	if len(registry) != 0 {
		t.Fatalf("registry = %v, want empty", registry)
	}
	if dictEN == nil {
		t.Fatal("a nil candidate dictEN must be replaced by a non-nil empty dict")
	}
	if dictRU != nil {
		t.Fatal("dictRU must stay nil")
	}
	if len(allDicts) != 0 || len(journalDictPatterns) != 0 {
		t.Fatalf("allDicts=%d patterns=%d, want 0/0", len(allDicts), len(journalDictPatterns))
	}
	if got := getDict(Lang("zz")); got != dictEN {
		t.Fatal("getDict must fall back to the empty EN dict installed from the nil candidate")
	}
	if IsKnownLang(LangEN) || IsKnownLang(Lang("zz")) {
		t.Fatal("nothing may be known after publishing an empty candidate")
	}
}

// TestSnapshotPreservesNonNilEmptyAllDicts guards the safe empty state: the
// package uses a non-nil empty []dictEntry{}, and an append-to-nil clone would
// turn it into nil, so a snapshot/restore round trip would no longer be
// DeepEqual and could hide a real mutation.
func TestSnapshotPreservesNonNilEmptyAllDicts(t *testing.T) {
	saved := snapshotI18nGlobals()
	t.Cleanup(func() { restoreI18nGlobals(saved) })

	allDicts = []dictEntry{}
	emptySnap := snapshotI18nGlobals()
	if emptySnap.allDicts == nil {
		t.Fatal("snapshot turned a non-nil empty allDicts into nil")
	}
	if len(emptySnap.allDicts) != 0 {
		t.Fatalf("snapshot allDicts = %+v, want empty", emptySnap.allDicts)
	}

	// The snapshot must still be an independent copy, not an alias: an
	// in-place element write on the live slice must not reach it.
	marker := &dict{}
	allDicts = []dictEntry{{LangEN, marker}}
	snap := snapshotI18nGlobals()
	allDicts[0] = dictEntry{LangRU, &dict{}}
	if snap.allDicts[0] != (dictEntry{LangEN, marker}) {
		t.Fatal("snapshot aliases the live allDicts backing array")
	}

	// Restore must reproduce the non-nil empty slice.
	restoreI18nGlobals(emptySnap)
	if allDicts == nil || len(allDicts) != 0 {
		t.Fatalf("restored allDicts = %#v, want a non-nil empty slice", allDicts)
	}
}

func TestSnapshotRestoreIsolatesMaps(t *testing.T) {
	base := snapshotI18nGlobals()
	t.Cleanup(func() { restoreI18nGlobals(base) })

	if registry[LangEN] == nil {
		t.Fatal("precondition: production registry must hold EN")
	}
	// In-place mutations of the live maps must not reach the snapshot.
	delete(registry, LangEN)
	journalDictPatterns = map[*dict]journalPatterns{}
	registry[Lang("zz")] = &dict{}

	if base.registry[LangEN] == nil {
		t.Fatal("snapshot shares the registry map: an in-place delete escaped into it")
	}
	if _, ok := base.registry[Lang("zz")]; ok {
		t.Fatal("snapshot shares the registry map: an in-place insert escaped into it")
	}
	if len(base.patterns) == 0 {
		t.Fatal("snapshot shares the patterns map: an in-place reassignment escaped into it")
	}

	restoreI18nGlobals(base)
	if registry[LangEN] == nil {
		t.Fatal("restore did not bring EN back")
	}
	if _, ok := registry[Lang("zz")]; ok {
		t.Fatal("restore kept a language that was never in the snapshot")
	}
	// Restore must hand out a copy too; otherwise the next in-place mutation
	// would corrupt the snapshot for a second restore.
	delete(registry, LangEN)
	restoreI18nGlobals(base)
	if registry[LangEN] == nil {
		t.Fatal("restore shares the snapshot map: a second in-place delete corrupted it")
	}
}
