package store

// findLocked — the ambiguity-safe lookup seam (T-0157). Split out of
// create_find_list_test.go to keep every test file inside the 300-line
// budget; the test names and semantics are unchanged.

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// ----------------------------------------------------------------------------
// findLocked — ambiguity-safe lookup (T-0157)
// ----------------------------------------------------------------------------

// TestFindLocked_AmbiguousNumber_ReturnsAmbiguousError pins that
// findLocked never returns an arbitrary first match: with two files
// carrying number 1 it reports *AmbiguousNumberError with both paths.
func TestFindLocked_AmbiguousNumber_ReturnsAmbiguousError(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	body := mustRead(t, dir, 1)
	done := filepath.Join(dir, "T-0001-done.md")
	if err := os.WriteFile(done, body, 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := s.findLocked(1)
	var amb *AmbiguousNumberError
	if !errors.As(err, &amb) {
		t.Fatalf("expected *AmbiguousNumberError, got %v", err)
	}
	want := []string{done, filepath.Join(dir, "T-0001-open.md")}
	if !reflect.DeepEqual(amb.Paths, want) {
		t.Errorf("Paths = %v, want %v", amb.Paths, want)
	}
}

// TestFindLocked_SingleMatchAndNotFound pins the non-ambiguous paths:
// exactly one match yields that fileEntry and its directory; an absent
// number yields ErrNotFound.
func TestFindLocked_SingleMatchAndNotFound(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	entry, where, err := s.findLocked(1)
	if err != nil {
		t.Fatalf("findLocked(1): %v", err)
	}
	if entry.Number != 1 || entry.Name != "T-0001-open.md" {
		t.Errorf("entry = %+v, want T-0001-open.md", entry)
	}
	if where != dir {
		t.Errorf("dir = %q, want %q", where, dir)
	}
	if _, _, err := s.findLocked(9999); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
