package store

// Tmp-name removal failure regressions: after the no-replace link commit
// succeeds, a failed removal of the tmp name (hooked as removeTmp) must
// roll back the committed target (removeFile), leave the old file intact
// with its old status, and retry the tmp removal best-effort. The
// removeFile hook must stay untouched by tmp removals (call-count
// contract: old + rollback only).

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// swapRemoveTmp sets removeTmp for the duration of t.
func swapRemoveTmp(t *testing.T, fn func(string) error) {
	t.Helper()
	orig := removeTmp
	removeTmp = fn
	t.Cleanup(func() { removeTmp = orig })
}

// failTmpOnce returns a removeTmp hook that fails the first removal of a
// tmp name with a synthetic PathError and really removes on later calls,
// so the best-effort retry leaves no *.tmp behind.
func failTmpOnce(tmpCalls *int) func(string) error {
	return func(p string) error {
		if !strings.HasSuffix(p, ".md.tmp") {
			return os.Remove(p)
		}
		*tmpCalls++
		if *tmpCalls == 1 {
			return &fs.PathError{Op: "remove", Path: p, Err: errors.New("synthetic-tmp-remove-boom")}
		}
		return os.Remove(p)
	}
}

// recordRemoveFile wraps removeFile to record every path it is called
// with, delegating to the real os.Remove; restored via t.Cleanup.
func recordRemoveFile(t *testing.T, calls *[]string) {
	t.Helper()
	orig := removeFile
	removeFile = func(p string) error {
		*calls = append(*calls, p)
		return orig(p)
	}
	t.Cleanup(func() { removeFile = orig })
}

// TestSetStatus_TmpRemoveFails_RollsBackTarget: the first tmp removal
// fails after the link commit; SetStatus must surface the tmp-removal
// error, roll the committed target back (exactly one removeFile call for
// target), keep the old file with its old status and retry the tmp
// removal.
func TestSetStatus_TmpRemoveFails_RollsBackTarget(t *testing.T) {
	s, dir, oldPath := seedTicket(t)
	target := filepath.Join(dir, "T-0001-wip.md")

	tmpCalls := 0
	swapRemoveTmp(t, failTmpOnce(&tmpCalls))
	var removeFileCalls []string
	recordRemoveFile(t, &removeFileCalls)

	if _, err := s.SetStatus(1, domain.StatusWip, "tester", ""); err == nil {
		t.Fatal("expected tmp-removal error")
	} else if !strings.Contains(err.Error(), "synthetic-tmp-remove-boom") {
		t.Errorf("tmp-removal cause must be propagated; got %v", err)
	}
	if tmpCalls != 2 {
		t.Errorf("removeTmp calls = %d, want 2 (initial + best-effort retry)", tmpCalls)
	}
	if len(removeFileCalls) != 1 || removeFileCalls[0] != target {
		t.Errorf("removeFile calls = %v, want exactly [%s] (rollback)", removeFileCalls, target)
	}
	if _, serr := os.Stat(target); !errors.Is(serr, fs.ErrNotExist) {
		t.Errorf("target must be rolled back; stat err=%v", serr)
	}
	assertOpenBody(t, oldPath)
	assertNoTmpFiles(t, dir)
}

// TestArchive_TmpRemoveFails_RollsBackTarget pins the same rollback for
// commitArchive: a failed tmp removal after the archive commit must roll
// the archive target back and leave the done ticket in place.
func TestArchive_TmpRemoveFails_RollsBackTarget(t *testing.T) {
	s, dir := makeDone(t)
	archiveDir := filepath.Join(dir, "archive")
	target := filepath.Join(archiveDir, "T-0001-done.md")

	tmpCalls := 0
	swapRemoveTmp(t, failTmpOnce(&tmpCalls))
	var removeFileCalls []string
	recordRemoveFile(t, &removeFileCalls)

	if _, err := s.Archive(1, "tester"); err == nil {
		t.Fatal("expected tmp-removal error")
	} else if !strings.Contains(err.Error(), "synthetic-tmp-remove-boom") {
		t.Errorf("tmp-removal cause must be propagated; got %v", err)
	}
	if tmpCalls != 2 {
		t.Errorf("removeTmp calls = %d, want 2 (initial + best-effort retry)", tmpCalls)
	}
	if len(removeFileCalls) != 1 || removeFileCalls[0] != target {
		t.Errorf("removeFile calls = %v, want exactly [%s] (rollback)", removeFileCalls, target)
	}
	if _, serr := os.Stat(target); !errors.Is(serr, fs.ErrNotExist) {
		t.Errorf("archive target must be rolled back; stat err=%v", serr)
	}
	oldBody, rerr := os.ReadFile(filepath.Join(dir, "T-0001-done.md"))
	if rerr != nil {
		t.Fatalf("done file must survive rollback: %v", rerr)
	}
	if !strings.Contains(string(oldBody), "- Статус: done") {
		t.Errorf("done file was mutated:\n%s", oldBody)
	}
	assertNoTmpFiles(t, archiveDir)
}
