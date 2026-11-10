package store

// T-0157: a ticket number carried by more than one file (s.Dir or
// s.Dir/archive) must never resolve to an arbitrary first match. These
// tests pin the store contract for the read paths and the mutation gates:
// *AmbiguousNumberError with the conflicting paths, and no side effects at
// all on the directory when a mutation is refused.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"ticket/internal/domain"
)

// seedTicketOne creates ticket 1 (T-0001-open.md) and returns its body, so
// a test can clone it into a second file carrying the same number.
func seedTicketOne(t *testing.T, s *Store, dir string) []byte {
	t.Helper()
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "T-0001-open.md"))
	if err != nil {
		t.Fatalf("read seeded ticket: %v", err)
	}
	return body
}

// cloneTicket writes body to destPath, creating parent directories. The
// filename status is authoritative (V19), so the copy is a valid ticket at
// the status its name carries.
func cloneTicket(t *testing.T, destPath string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(destPath), err)
	}
	if err := os.WriteFile(destPath, body, 0o644); err != nil {
		t.Fatalf("write %s: %v", destPath, err)
	}
}

// assertAmbiguous requires err to be an *AmbiguousNumberError for number
// carrying exactly paths (absolute, sorted), and to unwrap to the
// ErrAmbiguousNumber sentinel.
func assertAmbiguous(t *testing.T, err error, number int, paths []string) *AmbiguousNumberError {
	t.Helper()
	var amb *AmbiguousNumberError
	if !errors.As(err, &amb) {
		t.Fatalf("expected *AmbiguousNumberError, got %v", err)
	}
	if !errors.Is(err, ErrAmbiguousNumber) {
		t.Errorf("errors.Is(err, ErrAmbiguousNumber) = false: %v", err)
	}
	if amb.Number != number {
		t.Errorf("Number = %d, want %d", amb.Number, number)
	}
	if !reflect.DeepEqual(amb.Paths, paths) {
		t.Errorf("Paths = %v, want %v", amb.Paths, paths)
	}
	return amb
}

// snapshotTree records every directory and hashes every file under dir, so
// a test can prove a refused mutation left the tree byte-identical.
func snapshotTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := make(map[string]string)
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		rel, rerr := filepath.Rel(dir, p)
		if rerr != nil {
			return rerr
		}
		if d.IsDir() {
			out[rel+"/"] = ""
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		sum := sha256.Sum256(data)
		out[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", dir, err)
	}
	return out
}

// TestAmbiguousNumber_FindRejectsTwoFilesOneDir pins the read paths: with
// T-0001-open.md and T-0001-done.md side by side, Find/FindNamed/FindRaw
// all refuse the number, while FindExact still resolves the named file
// (the escape hatch).
func TestAmbiguousNumber_FindRejectsTwoFilesOneDir(t *testing.T) {
	s, dir := newStore(t)
	body := seedTicketOne(t, s, dir)
	done := filepath.Join(dir, "T-0001-done.md")
	cloneTicket(t, done, body)
	want := []string{done, filepath.Join(dir, "T-0001-open.md")}

	_, err := s.Find(1)
	assertAmbiguous(t, err, 1, want)
	_, _, err = s.FindNamed(1)
	assertAmbiguous(t, err, 1, want)
	_, _, _, err = s.FindRaw(1)
	assertAmbiguous(t, err, 1, want)

	tk, name, _, err := s.FindExact("T-0001-open.md")
	if err != nil {
		t.Fatalf("FindExact must resolve an existing name: %v", err)
	}
	if name != "T-0001-open.md" || tk.Number != 1 || tk.Status != domain.StatusOpen {
		t.Errorf("FindExact = (%d, %q, %q), want ticket 1 open", tk.Number, name, tk.Status)
	}
}

// TestAmbiguousNumber_PairSpansDirAndArchive pins the cross-directory pair:
// the archive copy is a conflict too, and Paths is sorted by absolute path
// (main dir before archive/, because "T-" sorts before "archive/").
func TestAmbiguousNumber_PairSpansDirAndArchive(t *testing.T) {
	s, dir := newStore(t)
	body := seedTicketOne(t, s, dir)
	archived := filepath.Join(dir, "archive", "T-0001-done.md")
	cloneTicket(t, archived, body)
	want := []string{filepath.Join(dir, "T-0001-open.md"), archived}

	_, err := s.Find(1)
	assertAmbiguous(t, err, 1, want)
	_, _, err = s.FindNamed(1)
	assertAmbiguous(t, err, 1, want)
}

// TestAmbiguousNumber_SameBaseNameInArchive pins the same base name in both
// directories: FindExact cannot distinguish them either, so it reports the
// ambiguity instead of picking one.
func TestAmbiguousNumber_SameBaseNameInArchive(t *testing.T) {
	s, dir := newStore(t)
	body := seedTicketOne(t, s, dir)
	archived := filepath.Join(dir, "archive", "T-0001-open.md")
	cloneTicket(t, archived, body)
	want := []string{filepath.Join(dir, "T-0001-open.md"), archived}

	_, _, _, err := s.FindExact("T-0001-open.md")
	assertAmbiguous(t, err, 1, want)
	_, _, err = s.FindNamed(1)
	assertAmbiguous(t, err, 1, want)
}

// TestSetStatus_AmbiguousNumberLeavesDirUntouched pins the mutation gate:
// the ambiguity is detected before any read, journal append, tmp write or
// rename, so the directory is byte-identical afterwards.
func TestSetStatus_AmbiguousNumberLeavesDirUntouched(t *testing.T) {
	s, dir := newStore(t)
	body := seedTicketOne(t, s, dir)
	done := filepath.Join(dir, "T-0001-done.md")
	cloneTicket(t, done, body)
	before := snapshotTree(t, dir)

	_, _, err := s.SetStatus(1, domain.StatusWip, "tester", "note")
	assertAmbiguous(t, err, 1, []string{done, filepath.Join(dir, "T-0001-open.md")})
	if after := snapshotTree(t, dir); !reflect.DeepEqual(before, after) {
		t.Errorf("SetStatus mutated the tree on ambiguity\nbefore: %v\nafter:  %v", before, after)
	}
}

// TestArchive_AmbiguousNumberLeavesDirUntouched pins the same gate for
// Archive: nothing is moved and no archive/ directory is created.
func TestArchive_AmbiguousNumberLeavesDirUntouched(t *testing.T) {
	s, dir := newStore(t)
	body := seedTicketOne(t, s, dir)
	done := filepath.Join(dir, "T-0001-done.md")
	cloneTicket(t, done, body)
	before := snapshotTree(t, dir)

	_, err := s.Archive(1, "tester")
	assertAmbiguous(t, err, 1, []string{done, filepath.Join(dir, "T-0001-open.md")})
	if after := snapshotTree(t, dir); !reflect.DeepEqual(before, after) {
		t.Errorf("Archive mutated the tree on ambiguity\nbefore: %v\nafter:  %v", before, after)
	}
}

// TestDuplicates_Inventory pins the non-fatal inventory the cli builds its
// warnings and its archive skip-set from: one entry per conflicting number,
// sorted by number, each with sorted absolute paths.
func TestDuplicates_Inventory(t *testing.T) {
	s, dir := newStore(t)
	first := seedTicketOne(t, s, dir)
	cloneTicket(t, filepath.Join(dir, "T-0001-done.md"), first)
	if _, err := s.Create(fakeTicket(0)); err != nil { // T-0002-open.md
		t.Fatal(err)
	}
	secondOpen := filepath.Join(dir, "T-0002-open.md")
	secondDone := filepath.Join(dir, "T-0002-done.md")
	cloneTicket(t, secondDone, mustRead(t, dir, 2))

	got, err := s.Duplicates()
	if err != nil {
		t.Fatalf("Duplicates: %v", err)
	}
	want := []NumberConflict{
		{Number: 1, Paths: []string{filepath.Join(dir, "T-0001-done.md"), filepath.Join(dir, "T-0001-open.md")}},
		{Number: 2, Paths: []string{secondDone, secondOpen}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Duplicates = %v, want %v", got, want)
	}
}

// TestDuplicates_UniqueNumbersAreNotConflicts pins the negative: a store
// whose numbers are unique yields an empty, error-free inventory.
func TestDuplicates_UniqueNumbersAreNotConflicts(t *testing.T) {
	s, dir := newStore(t)
	seedTicketOne(t, s, dir)
	got, err := s.Duplicates()
	if err != nil {
		t.Fatalf("Duplicates: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Duplicates = %v, want none", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "archive")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Duplicates must not create archive/: %v", err)
	}
}
