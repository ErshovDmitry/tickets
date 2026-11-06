package domain

import (
	"testing"
)

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
