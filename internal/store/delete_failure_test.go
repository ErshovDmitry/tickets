package store

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// failRemovePaths records every removeFile call and fails the listed
// paths with a synthetic PathError (same swap idiom as swapRemove).
func failRemovePaths(t *testing.T, calls *[]string, fail map[string]bool) {
	t.Helper()
	orig := removeFile
	removeFile = func(p string) error {
		*calls = append(*calls, p)
		if fail[p] {
			return &fs.PathError{Op: "remove", Path: p, Err: errors.New("synthetic-remove-boom")}
		}
		return orig(p)
	}
	t.Cleanup(func() { removeFile = orig })
}

// TestDelete_LinkFails_SourceUntouched pins the crash-before-commit
// invariant: a failed link commit leaves the source byte-identical,
// creates no target, cleans the tmp name and never calls removeFile.
func TestDelete_LinkFails_SourceUntouched(t *testing.T) {
	s, dir, oldPath := seedTicket(t)
	before := mustRead(t, dir, 1)
	swapLink(t, func(string, string) error {
		return &fs.PathError{Op: "link", Path: "injected", Err: errors.New("synthetic-link-boom")}
	})
	var removeFileCalls []string
	recordRemoveFile(t, &removeFileCalls)
	_, err := s.Delete("1", false, "tester")
	if err == nil || !strings.Contains(err.Error(), "synthetic-link-boom") {
		t.Fatalf("err = %v, want propagated link failure", err)
	}
	if after := mustRead(t, dir, 1); !bytes.Equal(before, after) {
		t.Fatal("failed commit changed the source")
	}
	if _, serr := os.Lstat(filepath.Join(dir, "deleted", "T-0001-deleted.md")); !errors.Is(serr, fs.ErrNotExist) {
		t.Fatalf("target must not exist, Lstat err=%v", serr)
	}
	if len(removeFileCalls) != 0 {
		t.Fatalf("removeFile calls = %v, want none before commit", removeFileCalls)
	}
	assertNoTmpFiles(t, filepath.Join(dir, "deleted"))
	if _, serr := os.Stat(oldPath); serr != nil {
		t.Fatalf("source must survive: %v", serr)
	}
}

// TestDelete_LinkExist_Collision pins the no-replace race gate: a
// target appearing between the Lstat fast-path and the link is a
// *CollisionError, source unchanged, tmp cleaned.
func TestDelete_LinkExist_Collision(t *testing.T) {
	s, dir, _ := seedTicket(t)
	before := mustRead(t, dir, 1)
	swapLink(t, func(string, string) error { return fs.ErrExist })
	_, err := s.Delete("1", false, "tester")
	var col *CollisionError
	if !errors.As(err, &col) || !errors.Is(err, ErrCollision) {
		t.Fatalf("err = %v, want *CollisionError", err)
	}
	if after := mustRead(t, dir, 1); !bytes.Equal(before, after) {
		t.Fatal("link collision changed the source")
	}
	assertNoTmpFiles(t, filepath.Join(dir, "deleted"))
}

// TestDelete_RemoveOldFails_RollsBackTarget pins the post-commit
// rollback: a failed old removal removes the committed target and
// keeps the source; removeFile sees exactly [old, target].
func TestDelete_RemoveOldFails_RollsBackTarget(t *testing.T) {
	s, dir, oldPath := seedTicket(t)
	before := mustRead(t, dir, 1)
	target := filepath.Join(dir, "deleted", "T-0001-deleted.md")
	var calls []string
	failRemovePaths(t, &calls, map[string]bool{oldPath: true})
	_, err := s.Delete("1", false, "tester")
	if err == nil || !strings.Contains(err.Error(), "remove old") {
		t.Fatalf("err = %v, want remove-old failure", err)
	}
	if len(calls) != 2 || calls[0] != oldPath || calls[1] != target {
		t.Fatalf("removeFile calls = %v, want [old target]", calls)
	}
	if _, serr := os.Lstat(target); !errors.Is(serr, fs.ErrNotExist) {
		t.Fatalf("target must be rolled back, Lstat err=%v", serr)
	}
	if after := mustRead(t, dir, 1); !bytes.Equal(before, after) {
		t.Fatal("rollback changed the source")
	}
	assertNoTmpFiles(t, filepath.Join(dir, "deleted"))
}

// TestDelete_RemoveOldAndRollbackFail_Breach pins that a double
// removal failure surfaces the invariant breach (old+new coexist)
// instead of silently losing either file.
func TestDelete_RemoveOldAndRollbackFail_Breach(t *testing.T) {
	s, dir, oldPath := seedTicket(t)
	target := filepath.Join(dir, "deleted", "T-0001-deleted.md")
	var calls []string
	failRemovePaths(t, &calls, map[string]bool{oldPath: true, target: true})
	_, err := s.Delete("1", false, "tester")
	if err == nil || !strings.Contains(err.Error(), "invariant breach") {
		t.Fatalf("err = %v, want invariant breach", err)
	}
	for _, p := range []string{oldPath, target} {
		if _, serr := os.Stat(p); serr != nil {
			t.Fatalf("breach must leave both files visible, %s: %v", p, serr)
		}
	}
}

// TestDelete_TmpRemoveFails_RollsBackTarget pins the shared
// commitMove tmp-removal rollback for delete: failed tmp removal
// after the link commit rolls the target back, keeps the source and
// retries the tmp removal (removeTmp twice, removeFile=[target]).
func TestDelete_TmpRemoveFails_RollsBackTarget(t *testing.T) {
	s, dir, _ := seedTicket(t)
	before := mustRead(t, dir, 1)
	target := filepath.Join(dir, "deleted", "T-0001-deleted.md")
	tmpCalls := 0
	swapRemoveTmp(t, failTmpOnce(&tmpCalls))
	var removeFileCalls []string
	recordRemoveFile(t, &removeFileCalls)
	_, err := s.Delete("1", false, "tester")
	if err == nil || !strings.Contains(err.Error(), "synthetic-tmp-remove-boom") {
		t.Fatalf("err = %v, want propagated tmp-removal failure", err)
	}
	if tmpCalls != 2 {
		t.Fatalf("removeTmp calls = %d, want 2 (initial + retry)", tmpCalls)
	}
	if len(removeFileCalls) != 1 || removeFileCalls[0] != target {
		t.Fatalf("removeFile calls = %v, want exactly [%s]", removeFileCalls, target)
	}
	if _, serr := os.Lstat(target); !errors.Is(serr, fs.ErrNotExist) {
		t.Fatalf("target must be rolled back, Lstat err=%v", serr)
	}
	if after := mustRead(t, dir, 1); !bytes.Equal(before, after) {
		t.Fatal("tmp-removal rollback changed the source")
	}
	assertNoTmpFiles(t, filepath.Join(dir, "deleted"))
}

// TestDelete_TmpRemoveAndRollbackFail_Breach pins the symmetric
// invariant marker of the tmp-removal branch: when the post-commit
// tmp removal AND the target rollback both fail, old+new coexist and
// the joined error carries the breach marker (W1 A6).
func TestDelete_TmpRemoveAndRollbackFail_Breach(t *testing.T) {
	s, dir, oldPath := seedTicket(t)
	target := filepath.Join(dir, "deleted", "T-0001-deleted.md")
	swapRemoveTmp(t, func(string) error { return errors.New("synthetic-tmp-remove-boom") })
	var calls []string
	failRemovePaths(t, &calls, map[string]bool{target: true})
	_, err := s.Delete("1", false, "tester")
	if err == nil || !strings.Contains(err.Error(), "invariant breach: old+new coexist") {
		t.Fatalf("err = %v, want invariant breach", err)
	}
	for _, want := range []string{"synthetic-tmp-remove-boom", "rollback remove"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %v, want %q", err, want)
		}
	}
	if len(calls) != 1 || calls[0] != target {
		t.Fatalf("removeFile calls = %v, want exactly [%s]", calls, target)
	}
	for _, p := range []string{oldPath, target} {
		if _, serr := os.Stat(p); serr != nil {
			t.Fatalf("breach must leave both files visible, %s: %v", p, serr)
		}
	}
}
