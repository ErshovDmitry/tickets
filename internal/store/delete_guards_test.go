package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ticket/internal/domain"
)

// TestDelete_AlreadyDeleted pins the terminal contract: a repeat
// (with or without force, by number or by name) is ErrAlreadyDeleted
// and changes nothing.
func TestDelete_AlreadyDeleted(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := s.Delete("1", false, "tester"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	deletedFile := filepath.Join(dir, "deleted", "T-0001-deleted.md")
	before, err := os.ReadFile(deletedFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		ref   string
		force bool
	}{
		{"1", false}, {"1", true}, {"T-0001-deleted.md", false}, {"T-0001-deleted.md", true},
	} {
		if _, err := s.Delete(tc.ref, tc.force, "tester"); !errors.Is(err, ErrAlreadyDeleted) {
			t.Errorf("Delete(%q, force=%v) = %v, want ErrAlreadyDeleted", tc.ref, tc.force, err)
		}
	}
	after, err := os.ReadFile(deletedFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("repeat delete changed the deleted file")
	}
}

// TestDelete_TargetCollision pins that an existing target refuses the
// delete with *CollisionError and changes neither source nor target.
// The ref is the exact base name: a foreign same-number file in
// deleted/ makes the bare number ambiguous before the collision gate
// (pinned separately in TestResolveDelete_DuplicateNumber).
func TestDelete_TargetCollision(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	deletedDir := mkDeletedDir(t, dir)
	foreign := []byte("# T-0001 · BUG: foreign\n")
	makeFile(t, deletedDir, "T-0001-deleted.md", string(foreign))
	source := mustRead(t, dir, 1)
	_, err := s.Delete("T-0001-open.md", false, "tester")
	var col *CollisionError
	if !errors.As(err, &col) || !errors.Is(err, ErrCollision) {
		t.Fatalf("err = %v, want *CollisionError", err)
	}
	if want := filepath.Join(deletedDir, "T-0001-deleted.md"); col.Target != want {
		t.Fatalf("Target = %q, want %q", col.Target, want)
	}
	if after := mustRead(t, dir, 1); !bytes.Equal(source, after) {
		t.Fatal("collision changed the source")
	}
	if after, rerr := os.ReadFile(filepath.Join(deletedDir, "T-0001-deleted.md")); rerr != nil || !bytes.Equal(foreign, after) {
		t.Fatalf("collision changed the target (err=%v)", rerr)
	}
}

// TestDelete_WhoCRLFGuard pins the T-0065 injection guard: who with
// CR/LF is rejected before any scan or mkdir, source unchanged.
func TestDelete_WhoCRLFGuard(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	source := mustRead(t, dir, 1)
	for _, who := range []string{"bad\rwho", "bad\nwho", "bad\r\nwho"} {
		_, err := s.Delete("1", false, who)
		var inv *ErrInvalidJournalInput
		if !errors.As(err, &inv) || inv.Field != "who" {
			t.Fatalf("who=%q: err = %v, want *ErrInvalidJournalInput{who}", who, err)
		}
	}
	if after := mustRead(t, dir, 1); !bytes.Equal(source, after) {
		t.Fatal("CRLF guard changed the source")
	}
	if _, err := os.Lstat(filepath.Join(dir, "deleted")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("CRLF guard must not create deleted/, Lstat err=%v", err)
	}
}

// TestDelete_ReadOnly pins ErrReadOnly for a NewReadOnly store.
func TestDelete_ReadOnly(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	ro, err := NewReadOnly(dir)
	if err != nil {
		t.Fatalf("NewReadOnly: %v", err)
	}
	if _, err := ro.Delete("1", true, "tester"); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("err = %v, want ErrReadOnly", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "T-0001-open.md")); err != nil {
		t.Fatalf("read-only delete changed the store: %v", err)
	}
}

// TestDelete_NotFoundAndAmbiguous pins ErrNotFound for absent refs
// and the typed ambiguity for a crash pair, with both files intact.
func TestDelete_NotFoundAndAmbiguous(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Delete("42", true, "tester"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("absent: err = %v, want ErrNotFound", err)
	}
	seedNamed(t, dir, 1, domain.StatusOpen)
	seedNamed(t, dir, 1, domain.StatusWip)
	_, err := s.Delete("1", true, "tester")
	var amb *AmbiguousNumberError
	if !errors.As(err, &amb) || !errors.Is(err, ErrAmbiguousNumber) || len(amb.Paths) != 2 {
		t.Fatalf("crash pair: err = %v, want *AmbiguousNumberError with 2 paths", err)
	}
	for _, name := range []string{"T-0001-open.md", "T-0001-wip.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("ambiguous delete removed %s: %v", name, err)
		}
	}
}

// TestDelete_InvalidDeletedPath pins that a hostile deleted/ path
// (regular file or symlink) blocks the delete without touching the
// source or anything outside the store.
func TestDelete_InvalidDeletedPath(t *testing.T) {
	t.Run("regular-file", func(t *testing.T) {
		s, dir := newStore(t)
		if _, err := s.Create(fakeTicket(0)); err != nil {
			t.Fatalf("Create: %v", err)
		}
		makeFile(t, dir, "deleted", "not a directory")
		source := mustRead(t, dir, 1)
		if _, err := s.Delete("1", true, "tester"); !errors.Is(err, ErrDeletedInvalid) {
			t.Fatalf("err = %v, want ErrDeletedInvalid", err)
		}
		if after := mustRead(t, dir, 1); !bytes.Equal(source, after) {
			t.Fatal("invalid deleted path changed the source")
		}
	})
	t.Run("symlink", func(t *testing.T) {
		requireSymlinks(t)
		s, dir := newStore(t)
		if _, err := s.Create(fakeTicket(0)); err != nil {
			t.Fatalf("Create: %v", err)
		}
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(dir, "deleted")); err != nil {
			t.Fatalf("symlink deleted: %v", err)
		}
		if _, err := s.Delete("1", true, "tester"); !errors.Is(err, ErrDeletedInvalid) {
			t.Fatalf("err = %v, want ErrDeletedInvalid", err)
		}
		assertDirEmpty(t, outside)
		if _, err := os.Stat(filepath.Join(dir, "T-0001-open.md")); err != nil {
			t.Fatalf("symlink guard changed the source: %v", err)
		}
	})
}

// TestDelete_InvalidRef pins that Delete rejects traversal and
// malformed refs before any mutation.
func TestDelete_InvalidRef(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, ref := range []string{"../1", "deleted/../1", "abc", "-1", ""} {
		if _, err := s.Delete(ref, true, "tester"); err == nil || errors.Is(err, ErrNotFound) {
			t.Errorf("Delete(%q) = %v, want invalid-ref rejection", ref, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "T-0001-open.md")); err != nil {
		t.Fatalf("invalid ref changed the store: %v", err)
	}
}
