package store

// Crash-window scenario (plan c5 D5.3): a commit interrupted between the
// link of the new file and the removal of the old one leaves a same-number
// pair on disk. The pair must be refused loudly — ambiguity, never an
// arbitrary first match — and no mutation may touch the tree.
//
// The window is produced deterministically through the removeFile hook
// (internal/store/tmpfile.go): the no-replace link commit succeeds, the
// removal of the old file fails, and so does the rollback removal of the
// committed target. No timing, no signal, no kill — the state is set up by
// the hook alone.

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// TestRemoveOldAndRollbackFail_LeavesPairAmbiguous pins the whole
// crash-window chain:
//
//	(a) the failed commit still surfaces its error (the old-file removal
//	    cause plus the invariant-breach note);
//	(b) with the hook restored, the number is refused as ambiguous by
//	    Find/FindNamed/FindRaw with BOTH absolute paths;
//	(c) a SetStatus on that number mutates nothing — the directory stays
//	    byte-identical.
func TestRemoveOldAndRollbackFail_LeavesPairAmbiguous(t *testing.T) {
	s, dir := newStore(t)
	seedTicketOne(t, s, dir)

	oldPath := filepath.Join(dir, "T-0001-open.md")
	target := filepath.Join(dir, "T-0001-done.md")

	// removeFile backs the old-file removal and the rollback removal of the
	// committed target; the tmp name is dropped through removeTmp, so the
	// link commit itself succeeds and the pair is created.
	orig := removeFile
	t.Cleanup(func() { removeFile = orig })
	removeFile = func(p string) error {
		return fmt.Errorf("synthetic-remove(%s)", filepath.Base(p))
	}

	_, _, err := s.SetStatus(1, domain.StatusDone, "tester", "close")
	if err == nil {
		t.Fatal("expected the failed old-file removal to surface an error")
	}
	// (a) the error reports the failure and the breach.
	for _, want := range []string{"synthetic-remove(T-0001-open.md)", "invariant breach: old+new coexist"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q must mention %q", err, want)
		}
	}
	// The pair is on disk: the old file and the committed new file.
	for _, p := range []string{oldPath, target} {
		if _, serr := os.Stat(p); serr != nil {
			t.Fatalf("expected %s to exist after the breach: %v", p, serr)
		}
	}
	// Exactly those two ticket files — no stray file, no leftover tmp.
	assertTicketFileNames(t, dir, "T-0001-done.md", "T-0001-open.md")

	// Restore the hook: from here on the store behaves normally.
	removeFile = orig

	// (b) the number no longer identifies exactly one ticket.
	wantPaths := []string{target, oldPath} // absolute, sorted
	if _, ferr := s.Find(1); ferr == nil {
		t.Fatal("Find(1) must refuse an ambiguous number")
	} else {
		assertAmbiguous(t, ferr, 1, wantPaths)
	}
	_, _, nerr := s.FindNamed(1)
	assertAmbiguous(t, nerr, 1, wantPaths)
	_, _, _, rerr := s.FindRaw(1)
	assertAmbiguous(t, rerr, 1, wantPaths)
	// The conflict is also reported by the non-fatal inventory.
	conflicts, derr := s.Duplicates()
	if derr != nil {
		t.Fatalf("Duplicates: %v", derr)
	}
	if len(conflicts) != 1 || conflicts[0].Number != 1 || !reflect.DeepEqual(conflicts[0].Paths, wantPaths) {
		t.Errorf("Duplicates = %v, want one conflict for number 1 with paths %v", conflicts, wantPaths)
	}

	// (c) a mutation on the ambiguous number changes nothing.
	before := snapshotTree(t, dir)
	_, _, serr := s.SetStatus(1, domain.StatusDone, "tester", "close again")
	assertAmbiguous(t, serr, 1, wantPaths)
	after := snapshotTree(t, dir)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("SetStatus mutated the tree on ambiguity\nbefore: %v\nafter:  %v", before, after)
	}
}

// assertTicketFileNames fails the test if dir contains ticket files other
// than the expected ones. The store lock file and tmp files are not ticket
// files and are ignored (tmp files are checked separately).
func assertTicketFileNames(t *testing.T, dir string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	var got []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || name == ".lock" || strings.HasSuffix(name, ".tmp") {
			continue
		}
		got = append(got, name)
	}
	sort.Strings(got)
	expected := append([]string(nil), want...)
	sort.Strings(expected)
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("ticket files in %s = %v, want %v", dir, got, expected)
	}
}
