package store

// T-0189 W2-A2 (continued): single-file deleted states, the W2
// deleted-blindness regression of the shared scan, the read-only refusal
// and the not-silenced deleted-scan failure. Pair tests live in
// recover_deleted_test.go; helpers (crashDeletePair, recoverStateErr,
// unchangedAfterRefusal, seedTicketOne, snapshotTree) are shared across
// the package's test files.

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"ticket/internal/domain"
)

func TestRecover_LoneDeleted(t *testing.T) {
	s, dir := newStore(t)
	seedTicketOne(t, s, dir)
	deletedPath, err := s.Delete("1", false, "tester")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	before := snapshotTree(t, dir)
	// Single remaining deleted file + its own name: no-op success.
	kept, err := s.Recover(1, "T-0001-deleted.md")
	if err != nil || kept != deletedPath {
		t.Fatalf("Recover = (%q, %v), want (%q, nil)", kept, err, deletedPath)
	}
	// Lone deleted + a foreign name: single-mismatch refusal.
	if _, err := s.Recover(1, "T-0001-open.md"); err == nil {
		t.Fatal("want a refusal for a foreign keep name")
	} else {
		recoverStateErr(t, err, RecoverReasonSingleMismatch)
	}
	if after := snapshotTree(t, dir); !reflect.DeepEqual(before, after) {
		t.Error("single-file cases must not touch the tree")
	}
}

func TestRecover_SingleMain_KeepDeletedNameRefused(t *testing.T) {
	s, dir := newStore(t)
	seedTicketOne(t, s, dir)
	unchangedAfterRefusal(t, s, dir, "T-0001-deleted.md", RecoverReasonSingleMismatch)
}

func TestRecover_DeletedInvisibleToSharedScan(t *testing.T) {
	s, dir := newStore(t)
	srcBody := seedTicketOne(t, s, dir)
	if _, err := s.Delete("1", false, "tester"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// The interrupted-Delete state again, built without hooks: restore the
	// stale source the crash would have left behind.
	if err := os.WriteFile(filepath.Join(dir, "T-0001-open.md"), srcBody, 0o644); err != nil {
		t.Fatalf("restore source: %v", err)
	}
	// Regression (AC6): findAllNumber stays deleted-blind — set/show-style
	// resolution must not see the deleted/ file of the same number.
	files, err := s.findAllNumber(1)
	if err != nil || len(files) != 1 {
		t.Fatalf("findAllNumber = %v, %v; want exactly the main file", files, err)
	}
	target, _, err := s.SetStatus(1, domain.StatusDone, "tester", "close")
	if err != nil {
		t.Fatalf("SetStatus with deleted/ sibling: %v", err)
	}
	if filepath.Base(target) != "T-0001-done.md" {
		t.Errorf("target = %q, want main T-0001-done.md", target)
	}
	if tk, name, err := s.FindNamed(1); err != nil || filepath.Base(name) != "T-0001-done.md" || tk.Status != domain.StatusDone {
		t.Errorf("FindNamed = (%v, %q, %v)", tk.Status, name, err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "deleted", "T-0001-deleted.md")); err != nil {
		t.Errorf("deleted/ sibling must survive the shared-scan operations: %v", err)
	}
}

func TestRecover_ReadOnlyStoreRefused(t *testing.T) {
	s, dir := newStore(t)
	crashDeletePair(t, s, dir, false, nil)
	before := snapshotTree(t, dir)
	ro, err := NewReadOnly(dir)
	if err != nil {
		t.Fatalf("NewReadOnly: %v", err)
	}
	if _, err := ro.Recover(1, "T-0001-deleted.md"); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Recover on read-only store = %v, want ErrReadOnly", err)
	}
	if after := snapshotTree(t, dir); !reflect.DeepEqual(before, after) {
		t.Error("read-only refusal must not touch the tree")
	}
}

func TestRecover_DeletedScanFailureNotSilenced(t *testing.T) {
	s, dir := newStore(t)
	seedTicketOne(t, s, dir)
	// A deleted path that is not a real directory must fail the recovery
	// scan, exactly like an invalid archive path fails findAllNumber —
	// never silently skip the deleted half.
	if err := os.WriteFile(filepath.Join(dir, "deleted"), []byte("bogus"), 0o644); err != nil {
		t.Fatalf("plant bogus deleted path: %v", err)
	}
	before := snapshotTree(t, dir)
	_, err := s.Recover(1, "T-0001-open.md")
	if !errors.Is(err, ErrDeletedInvalid) {
		t.Fatalf("Recover = %v, want ErrDeletedInvalid", err)
	}
	if after := snapshotTree(t, dir); !reflect.DeepEqual(before, after) {
		t.Error("scan-failure refusal must not touch the tree")
	}
}
