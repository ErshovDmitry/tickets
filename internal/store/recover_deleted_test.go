package store

// T-0189 W2-A2: Store.Recover against crash pairs whose successor lives in
// tickets/deleted/ (an interrupted Delete, optionally force from archive).
// Reuses the recover_test.go helpers (recoverStateErr, unchangedAfterRefusal,
// seedTicketOne, swapRemove, snapshotTree). The deleted successor is always
// produced by the real renderDeleted path — never a hand-built journal line.

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// crashDeletePair leaves the real interrupted-Delete state on disk: the
// deleted successor is committed (honest renderDeleted output with one
// trailing old→deleted journal record) but the removal of the stale
// source fails together with its rollback, so the source and the
// deleted/ successor both remain — exactly what a crash between commit
// and removal leaves behind. setup optionally moves the ticket first
// (e.g. done+archive for the forced case).
func crashDeletePair(t *testing.T, s *Store, dir string, force bool, setup func()) (srcPath, deletedPath string) {
	t.Helper()
	seedTicketOne(t, s, dir)
	if setup != nil {
		setup()
	}
	// The removeFile swap is restored immediately (same idiom as
	// crashPair in recover_test.go): it must NOT stay active for the
	// Recover call itself, which legitimately removes the stale source.
	orig := removeFile
	removeFile = func(p string) error { return fmt.Errorf("boom(%s)", filepath.Base(p)) }
	restored := false
	t.Cleanup(func() {
		if !restored {
			removeFile = orig
		}
	})
	if _, err := s.Delete("1", force, "tester"); err == nil {
		t.Fatal("expected the failed source removal to surface an error")
	}
	removeFile = orig
	restored = true
	deletedPath = filepath.Join(dir, "deleted", "T-0001-deleted.md")
	if _, err := os.Lstat(deletedPath); err != nil {
		t.Fatalf("deleted successor must exist: %v", err)
	}
	files, err := s.findAllNumber(1)
	if err != nil {
		t.Fatalf("findAllNumber: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("want exactly the stale source left, got %v", files)
	}
	return filepath.Join(files[0].dir, files[0].Name), deletedPath
}

// assertDeletedCommitted asserts the committed recover result: the stale
// source is gone, the deleted file kept byte-identical and the terminal
// from→deleted transition present in its journal.
func assertDeletedCommitted(t *testing.T, srcPath, deletedPath string, deletedBody []byte, kept, from string) {
	t.Helper()
	if kept != deletedPath {
		t.Errorf("kept %q, want %q", kept, deletedPath)
	}
	if _, err := os.Stat(srcPath); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stale source must be removed, stat err: %v", err)
	}
	after, err := os.ReadFile(deletedPath)
	if err != nil {
		t.Fatalf("read kept deleted file: %v", err)
	}
	if !bytes.Equal(after, deletedBody) {
		t.Errorf("deleted file changed by recovery: %q", after)
	}
	// The terminal transition must be from→deleted: it is the LAST
	// " — статус: " line (the forced archive pair also carries the
	// earlier open→done record, which assertJournalTransition would
	// match first).
	var last string
	for line := range strings.SplitSeq(string(after), "\n") {
		if strings.Contains(line, " — статус: ") {
			last = line
		}
	}
	wantSuffix := " — статус: " + from + " → deleted (tester)"
	if !strings.HasSuffix(last, wantSuffix) {
		t.Errorf("terminal journal line = %q, want suffix %q", last, wantSuffix)
	}
	assertNoTmpFiles(t, filepath.Dir(deletedPath))
}

func TestRecover_DeletedPair_KeepsDeleted(t *testing.T) {
	s, dir := newStore(t)
	srcPath, deletedPath := crashDeletePair(t, s, dir, false, nil)
	deletedBody, err := os.ReadFile(deletedPath)
	if err != nil {
		t.Fatalf("read deleted: %v", err)
	}
	kept, err := s.Recover(1, "T-0001-deleted.md")
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	assertDeletedCommitted(t, srcPath, deletedPath, deletedBody, kept, "open")
	// Idempotent repeat: the deleted file alone remains — a no-op success.
	again, err := s.Recover(1, "T-0001-deleted.md")
	if err != nil || again != kept {
		t.Errorf("repeat Recover = (%q, %v), want (%q, nil)", again, err, kept)
	}
}

func TestRecover_DeletedPair_KeepOldRefused(t *testing.T) {
	s, dir := newStore(t)
	crashDeletePair(t, s, dir, false, nil)
	rse := unchangedAfterRefusal(t, s, dir, "T-0001-open.md", RecoverReasonKeepOld)
	if rse.Other != "T-0001-deleted.md" {
		t.Errorf("Other = %q, want the deleted successor", rse.Other)
	}
}

func TestRecover_ArchiveDeletedForcePair_KeepsDeleted(t *testing.T) {
	s, dir := newStore(t)
	setup := func() {
		if _, _, err := s.SetStatus(1, domain.StatusDone, "tester", "close"); err != nil {
			t.Fatalf("SetStatus done: %v", err)
		}
		if _, err := s.Archive(1, "tester"); err != nil {
			t.Fatalf("Archive: %v", err)
		}
	}
	srcPath, deletedPath := crashDeletePair(t, s, dir, true, setup)
	deletedBody, err := os.ReadFile(deletedPath)
	if err != nil {
		t.Fatalf("read deleted: %v", err)
	}
	kept, err := s.Recover(1, "T-0001-deleted.md")
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if srcPath != filepath.Join(dir, "archive", "T-0001-done.md") {
		t.Fatalf("stale source = %q, want the archived done file", srcPath)
	}
	assertDeletedCommitted(t, srcPath, deletedPath, deletedBody, kept, "done")
}

func TestRecover_DeletedPair_DivergentRefused(t *testing.T) {
	s, dir := newStore(t)
	_, deletedPath := crashDeletePair(t, s, dir, false, nil)
	body, err := os.ReadFile(deletedPath)
	if err != nil {
		t.Fatalf("read deleted: %v", err)
	}
	divergent := bytes.Replace(body, []byte("details for ticket 0"), []byte("details tampered"), 1)
	if bytes.Equal(divergent, body) {
		t.Fatal("tamper marker not found in deleted body")
	}
	if err := os.WriteFile(deletedPath, divergent, 0o644); err != nil {
		t.Fatalf("tamper deleted: %v", err)
	}
	unchangedAfterRefusal(t, s, dir, "T-0001-deleted.md", RecoverReasonDivergent)
}

func TestRecover_DeletedSameStatus_Refused(t *testing.T) {
	s, dir := newStore(t)
	_, deletedPath := crashDeletePair(t, s, dir, false, nil)
	body, err := os.ReadFile(deletedPath)
	if err != nil {
		t.Fatalf("read deleted: %v", err)
	}
	// The identical basename in two directories: same filename status in
	// main and deleted/ — no status transition to recover. The stale source
	// goes away first, otherwise its presence would trip the >2 ambiguity
	// gate before the status check.
	if err := os.Remove(filepath.Join(dir, "T-0001-open.md")); err != nil {
		t.Fatalf("remove stale source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "T-0001-deleted.md"), body, 0o644); err != nil {
		t.Fatalf("duplicate deleted basename into main: %v", err)
	}
	unchangedAfterRefusal(t, s, dir, "T-0001-deleted.md", RecoverReasonSameStatus)
}

func TestRecover_DeletedAmbiguous_MoreThanTwo(t *testing.T) {
	s, dir := newStore(t)
	srcPath, _ := crashDeletePair(t, s, dir, false, nil)
	srcBody, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	// A third file carrying the number turns the recovery into the T-0157
	// ambiguity (exit-2 material): nothing may be removed.
	cloneTicket(t, filepath.Join(dir, "T-0001-done.md"), srcBody)
	before := snapshotTree(t, dir)
	_, err = s.Recover(1, "T-0001-deleted.md")
	var amb *AmbiguousNumberError
	if !errors.As(err, &amb) {
		t.Fatalf("want *AmbiguousNumberError, got %v", err)
	}
	if len(amb.Paths) != 3 {
		t.Errorf("Paths = %v, want 3 entries", amb.Paths)
	}
	if after := snapshotTree(t, dir); !reflect.DeepEqual(before, after) {
		t.Error("ambiguous refusal must not touch the tree")
	}
}
