package store

// T-0074 regression: SetStatus must route on file names alone (collision
// gate before any body read), so a foreign same-number file at the target
// status — even one whose body is garbage (broken H1) — surfaces as a
// *CollisionError, never a parse error, and is never touched.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ticket/internal/domain"
)

// TestSetStatus_ForeignBrokenDoneCollision pins the main-dir case: a
// healthy T-0001-open.md plus a foreign T-0001-done.md whose body is
// "FOREIGN-ZERO-BODY" (H1 number 0) must yield *CollisionError{Target =
// dir/T-0001-done.md}, leave both files untouched, and leak no tmp file.
func TestSetStatus_ForeignBrokenDoneCollision(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil { // T-0001-open.md
		t.Fatal(err)
	}
	foreign := filepath.Join(dir, "T-0001-done.md")
	if err := os.WriteFile(foreign, []byte("FOREIGN-ZERO-BODY"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := s.SetStatus(1, domain.StatusDone, "tester", "")
	var coll *CollisionError
	if !errors.As(err, &coll) {
		t.Fatalf("expected *CollisionError, got %v", err)
	}
	if coll.Target != foreign {
		t.Errorf("CollisionError.Target = %q, want %q", coll.Target, foreign)
	}
	assertFileContent(t, foreign, "FOREIGN-ZERO-BODY")
	assertOpenBody(t, filepath.Join(dir, "T-0001-open.md"))
	assertNoTmpFiles(t, dir)
}

// TestSetStatus_ForeignBrokenDoneArchiveCollision pins the archive case:
// a real closed ticket archived under archive/ plus a foreign done file
// there must yield *CollisionError{Target = archive/T-0001-done.md} with
// both files and the archive dir left clean.
func TestSetStatus_ForeignBrokenDoneArchiveCollision(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetStatus(1, domain.StatusClosed, "tester", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Archive(1, "tester"); err != nil {
		t.Fatal(err)
	}
	archiveDir := filepath.Join(dir, "archive")
	foreign := filepath.Join(archiveDir, "T-0001-done.md")
	if err := os.WriteFile(foreign, []byte("FOREIGN-ZERO-BODY"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := s.SetStatus(1, domain.StatusDone, "tester", "")
	var coll *CollisionError
	if !errors.As(err, &coll) {
		t.Fatalf("expected *CollisionError, got %v", err)
	}
	if coll.Target != foreign {
		t.Errorf("CollisionError.Target = %q, want %q", coll.Target, foreign)
	}
	assertFileContent(t, foreign, "FOREIGN-ZERO-BODY")
	if _, serr := os.Stat(filepath.Join(archiveDir, "T-0001-closed.md")); serr != nil {
		t.Errorf("archived closed file must remain: %v", serr)
	}
	assertNoTmpFiles(t, dir)
	assertNoTmpFiles(t, archiveDir)
}

// assertFileContent fails the test if the file at path is not exactly want.
func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(data) != want {
		t.Errorf("%s was mutated: %q", path, data)
	}
}
