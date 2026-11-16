package store

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"ticket/internal/domain"
)

// TestMaxNumber_IncludesDeleted pins that deleted numbers raise the
// numbering high-water mark exactly like archive numbers.
func TestMaxNumber_IncludesDeleted(t *testing.T) {
	s, dir := newStore(t)
	deletedDir := mkDeletedDir(t, dir)
	seedNamed(t, dir, 1, domain.StatusOpen)
	seedNamed(t, deletedDir, 7, domain.StatusDeleted)
	got, err := s.maxNumberLocked()
	if err != nil || got != 7 {
		t.Fatalf("maxNumberLocked = %d, %v, want 7", got, err)
	}
	n, err := s.Create(fakeTicket(0))
	if err != nil || n != 8 {
		t.Fatalf("Create = %d, %v, want 8 (no reuse of deleted 7)", n, err)
	}
}

// TestCreate_AfterDelete_NoNumberReuse pins the end-to-end contract:
// deleting the highest ticket never lets the next Create reuse it.
func TestCreate_AfterDelete_NoNumberReuse(t *testing.T) {
	s, dir := newStore(t)
	for range 3 {
		if _, err := s.Create(fakeTicket(0)); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	if _, err := s.Delete("3", false, "tester"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	n, err := s.Create(fakeTicket(0))
	if err != nil || n != 4 {
		t.Fatalf("Create after delete = %d, %v, want 4", n, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "deleted", "T-0003-deleted.md")); err != nil {
		t.Fatalf("deleted ticket must survive: %v", err)
	}
}

// TestCreate_DeletedOnlyStore pins numbering when only deleted/
// holds numbers (main empty).
func TestCreate_DeletedOnlyStore(t *testing.T) {
	s, dir := newStore(t)
	deletedDir := mkDeletedDir(t, dir)
	seedNamed(t, deletedDir, 5, domain.StatusDeleted)
	n, err := s.Create(fakeTicket(0))
	if err != nil || n != 6 {
		t.Fatalf("Create = %d, %v, want 6", n, err)
	}
}

// TestCreate_UnreadableDeleted_Fails pins that an unreadable deleted
// directory blocks numbering (no blind reuse, mirrors archive).
func TestCreate_UnreadableDeleted_Fails(t *testing.T) {
	s, dir := newStore(t)
	deletedDir := mkDeletedDir(t, dir)
	seedNamed(t, deletedDir, 9, domain.StatusDeleted)
	swapReadDir(t, func(p string) ([]os.DirEntry, error) {
		if p == deletedDir {
			return nil, dirReadPathError(deletedDir)
		}
		return os.ReadDir(p)
	})
	if _, err := s.Create(fakeTicket(0)); err == nil {
		t.Fatal("Create must fail on unreadable deleted/")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".md" {
			t.Fatalf("no ticket may be created on a half-read store, found %s", e.Name())
		}
	}
}

// TestCreate_InvalidDeleted_Fails pins that a hostile deleted/ path
// (regular file or symlink) blocks numbering without reuse.
func TestCreate_InvalidDeleted_Fails(t *testing.T) {
	t.Run("regular-file", func(t *testing.T) {
		s, dir := newStore(t)
		makeFile(t, dir, "deleted", "not a directory")
		if _, err := s.Create(fakeTicket(0)); !errors.Is(err, ErrDeletedInvalid) {
			t.Fatalf("err = %v, want ErrDeletedInvalid", err)
		}
	})
	t.Run("symlink", func(t *testing.T) {
		requireSymlinks(t)
		s, dir := newStore(t)
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(dir, "deleted")); err != nil {
			t.Fatalf("symlink deleted: %v", err)
		}
		if _, err := s.Create(fakeTicket(0)); !errors.Is(err, ErrDeletedInvalid) {
			t.Fatalf("err = %v, want ErrDeletedInvalid", err)
		}
		assertDirEmpty(t, outside)
	})
}

// TestDuplicates_DeletedAware pins that Duplicates reports numbers
// shared with deleted/ through the same seam, with sorted absolute
// paths.
func TestDuplicates_DeletedAware(t *testing.T) {
	s, dir := newStore(t)
	deletedDir := mkDeletedDir(t, dir)
	seedNamed(t, dir, 1, domain.StatusOpen)
	seedNamed(t, deletedDir, 1, domain.StatusDeleted)
	seedNamed(t, deletedDir, 2, domain.StatusDeleted)
	seedNamed(t, dir, 2, domain.StatusWip)
	seedNamed(t, dir, 3, domain.StatusOpen)
	conflicts, err := s.Duplicates()
	if err != nil {
		t.Fatalf("Duplicates: %v", err)
	}
	if len(conflicts) != 2 || conflicts[0].Number != 1 || conflicts[1].Number != 2 {
		t.Fatalf("conflicts = %+v, want numbers [1 2]", conflicts)
	}
	want := []string{filepath.Join(dir, "T-0001-open.md"), filepath.Join(deletedDir, "T-0001-deleted.md")}
	if !reflect.DeepEqual(conflicts[0].Paths, want) {
		t.Fatalf("paths = %v, want %v", conflicts[0].Paths, want)
	}
}

// TestDuplicates_DeletedErrors pins that an invalid or unreadable
// deleted directory fails Duplicates loudly (never silenced).
func TestDuplicates_DeletedErrors(t *testing.T) {
	s, dir := newStore(t)
	makeFile(t, dir, "deleted", "not a directory")
	if _, err := s.Duplicates(); !errors.Is(err, ErrDeletedInvalid) {
		t.Fatalf("invalid: err = %v, want ErrDeletedInvalid", err)
	}
	if err := os.Remove(filepath.Join(dir, "deleted")); err != nil {
		t.Fatal(err)
	}
	deletedDir := mkDeletedDir(t, dir)
	swapReadDir(t, func(p string) ([]os.DirEntry, error) {
		if p == deletedDir {
			return nil, dirReadPathError(deletedDir)
		}
		return os.ReadDir(p)
	})
	if _, err := s.Duplicates(); err == nil {
		t.Fatal("unreadable deleted: want error, got nil")
	}
}
