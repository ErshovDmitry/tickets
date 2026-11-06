package domain

import (
	"errors"
	"reflect"
	"testing"
	"testing/fstest"
)

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
