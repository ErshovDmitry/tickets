package store

// T-0074 regression, re-pinned for T-0157: SetStatus must still route on
// file names alone (the gate before any body read), but a second file with
// the same number is now an AMBIGUOUS number, not a target collision: the
// number no longer identifies exactly one ticket. Both files stay
// untouched, nothing is read, journaled, renamed or tmp-written, and a
// foreign file with a garbage body proves the body is never parsed.

import (
	"os"
	"path/filepath"
	"testing"

	"ticket/internal/domain"
)

// TestSetStatus_ForeignBrokenDoneAmbiguous pins the main-dir case: a
// healthy T-0001-open.md plus a foreign T-0001-done.md whose body is
// "FOREIGN-ZERO-BODY" (H1 number 0) must yield *AmbiguousNumberError for
// number 1 listing BOTH absolute paths, leave both files untouched, and
// leak no tmp file. A parse error here would mean the gate lost the
// T-0074 ordering.
func TestSetStatus_ForeignBrokenDoneAmbiguous(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil { // T-0001-open.md
		t.Fatal(err)
	}
	foreign := filepath.Join(dir, "T-0001-done.md")
	if err := os.WriteFile(foreign, []byte("FOREIGN-ZERO-BODY"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := s.SetStatus(1, domain.StatusDone, "tester", "")
	assertAmbiguous(t, err, 1, []string{foreign, filepath.Join(dir, "T-0001-open.md")})
	assertFileContent(t, foreign, "FOREIGN-ZERO-BODY")
	assertOpenBody(t, filepath.Join(dir, "T-0001-open.md"))
	assertNoTmpFiles(t, dir)
}

// TestSetStatus_ForeignBrokenDoneArchiveAmbiguous pins the archive case: a
// real closed ticket archived under archive/ plus a foreign done file there
// must yield *AmbiguousNumberError for number 1 listing both archive paths,
// with both files and the archive dir left clean.
func TestSetStatus_ForeignBrokenDoneArchiveAmbiguous(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SetStatus(1, domain.StatusClosed, "tester", ""); err != nil {
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

	_, _, err := s.SetStatus(1, domain.StatusDone, "tester", "")
	assertAmbiguous(t, err, 1, []string{filepath.Join(archiveDir, "T-0001-closed.md"), foreign})
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
