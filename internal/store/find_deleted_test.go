package store

// T-0189 W3-C1a: deleted-aware lookup seams (find_deleted.go). The seams
// widen ONLY the lookup: FindRaw/FindExact keep their W2 deleted-blindness
// (pinned here), ambiguity stays a before-body-read *AmbiguousNumberError
// with sorted paths (T-0157), and a deleted scan error refuses loudly.

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// acrossDeletedBody is a parseable deleted ticket body seeded straight
// into tickets/deleted/ (the filename status is authoritative, V19).
const acrossDeletedBody = `# T-0001 · BUG: seeded deleted

- Status (Статус): deleted
- Priority (Приоритет): normal
- Created (Создан): 2026-11-16 10:00 · by (кем): tester
- Project (Проект): tickets

## Summary (Кратко)
seeded body

## Details (Подробности)
details

## Journal (Журнал)
- 2026-11-16 10:00 — тикет создан (tester).
`

// seedDeletedFile creates tickets/deleted/ (when missing) and writes one
// entry, returning the deleted directory.
func seedDeletedFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	deletedDir := filepath.Join(dir, "deleted")
	if _, err := os.Lstat(deletedDir); errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(deletedDir, 0o755); err != nil {
			t.Fatalf("mkdir deleted: %v", err)
		}
	}
	makeFile(t, deletedDir, name, body)
	return deletedDir
}

// TestFindRawAcross_DeletedSingleMatch pins the happy path: a number that
// lives only under deleted/ resolves to that file with the same
// readLocated semantics as FindRaw — filename status, raw bytes, name.
func TestFindRawAcross_DeletedSingleMatch(t *testing.T) {
	s, dir := newStore(t)
	deletedDir := seedDeletedFile(t, dir, "T-0001-deleted.md", acrossDeletedBody)

	tk, name, raw, err := s.FindRawAcross(1)
	if err != nil {
		t.Fatalf("FindRawAcross: %v", err)
	}
	if tk.Number != 1 || tk.Status != domain.StatusDeleted {
		t.Errorf("ticket = %+v, want number 1 status deleted", tk)
	}
	if name != "T-0001-deleted.md" {
		t.Errorf("name = %q, want T-0001-deleted.md", name)
	}
	want, err := os.ReadFile(filepath.Join(deletedDir, name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, want) {
		t.Errorf("raw bytes differ from the deleted file")
	}
}

// TestFindExactAcross_DeletedSingleMatch pins the exact-name escape hatch
// across all three directories: the deleted file resolves by its exact
// base name.
func TestFindExactAcross_DeletedSingleMatch(t *testing.T) {
	s, dir := newStore(t)
	seedDeletedFile(t, dir, "T-0001-deleted.md", acrossDeletedBody)

	tk, name, _, err := s.FindExactAcross("T-0001-deleted.md")
	if err != nil {
		t.Fatalf("FindExactAcross: %v", err)
	}
	if tk.Number != 1 || tk.Status != domain.StatusDeleted || name != "T-0001-deleted.md" {
		t.Errorf("got %+v / %q, want ticket 1 deleted under its own name", tk, name)
	}
	if _, _, _, err := s.FindExactAcross("T-0002-deleted.md"); !errors.Is(err, ErrNotFound) {
		t.Errorf("absent exact name: err = %v, want ErrNotFound", err)
	}
}

// TestFindRawAcross_Ambiguity pins the T-0157 contract for every
// cross-directory pair: the ambiguity is detected BEFORE any body is
// read and carries every conflicting path, deduplicated and sorted.
func TestFindRawAcross_Ambiguity(t *testing.T) {
	cases := []struct {
		name string
		seed func(t *testing.T, s *Store, dir string) []string
	}{
		{"main+deleted", func(t *testing.T, s *Store, dir string) []string {
			if _, err := s.Create(fakeTicket(0)); err != nil {
				t.Fatal(err)
			}
			seedDeletedFile(t, dir, "T-0001-deleted.md", acrossDeletedBody)
			return []string{filepath.Join(dir, "T-0001-open.md"), filepath.Join(dir, "deleted", "T-0001-deleted.md")}
		}},
		{"archive+deleted", func(t *testing.T, _ *Store, dir string) []string {
			archiveDir := filepath.Join(dir, "archive")
			if err := os.Mkdir(archiveDir, 0o755); err != nil {
				t.Fatal(err)
			}
			makeFile(t, archiveDir, "T-0001-done.md", seededDoneBody)
			seedDeletedFile(t, dir, "T-0001-deleted.md", acrossDeletedBody)
			return []string{filepath.Join(dir, "archive", "T-0001-done.md"), filepath.Join(dir, "deleted", "T-0001-deleted.md")}
		}},
		{"main+archive+deleted", func(t *testing.T, s *Store, dir string) []string {
			if _, err := s.Create(fakeTicket(0)); err != nil {
				t.Fatal(err)
			}
			archiveDir := filepath.Join(dir, "archive")
			if err := os.Mkdir(archiveDir, 0o755); err != nil {
				t.Fatal(err)
			}
			makeFile(t, archiveDir, "T-0001-done.md", seededDoneBody)
			seedDeletedFile(t, dir, "T-0001-deleted.md", acrossDeletedBody)
			return []string{
				filepath.Join(dir, "T-0001-open.md"),
				filepath.Join(dir, "archive", "T-0001-done.md"),
				filepath.Join(dir, "deleted", "T-0001-deleted.md"),
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, dir := newStore(t)
			want := tc.seed(t, s, dir)
			_, _, _, err := s.FindRawAcross(1)
			assertAmbiguous(t, err, 1, want)
		})
	}
}

// TestFindExactAcross_IdenticalNameAcrossDirs pins that the SAME base
// name in more than one directory is an ambiguity too, deleted/
// included: the escape hatch names one file, never an arbitrary first
// match.
func TestFindExactAcross_IdenticalNameAcrossDirs(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil { // T-0001-open.md in main
		t.Fatal(err)
	}
	body := mustRead(t, dir, 1)
	deletedDir := seedDeletedFile(t, dir, "T-0001-open.md", string(body)) // same name, deleted/
	want := []string{filepath.Join(dir, "T-0001-open.md"), filepath.Join(deletedDir, "T-0001-open.md")}

	_, _, _, err := s.FindExactAcross("T-0001-open.md")
	assertAmbiguous(t, err, 1, want)
}

// TestFindRawAcross_AbsentNotFound pins the negative path for both seams:
// a number or name nobody carries is ErrNotFound, even with no deleted/
// at all.
func TestFindRawAcross_AbsentNotFound(t *testing.T) {
	s, _ := newStore(t)
	if _, _, _, err := s.FindRawAcross(7); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindRawAcross(7) = %v, want ErrNotFound", err)
	}
	if _, _, _, err := s.FindExactAcross("T-0007-open.md"); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindExactAcross = %v, want ErrNotFound", err)
	}
}

// TestFindRawAcross_InvalidDeletedPropagates pins that an invalid deleted
// path (a symlink or a regular file, T-0189) fails the lookup loudly on
// BOTH seams instead of being skipped — a lookup must not act on a
// number it could not fully read.
func TestFindRawAcross_InvalidDeletedPropagates(t *testing.T) {
	for _, tc := range []struct {
		name string
		make func(t *testing.T, dir string)
	}{
		{"regular file", func(t *testing.T, dir string) { makeFile(t, dir, "deleted", "not a dir") }},
		{"symlink", func(t *testing.T, dir string) {
			requireSymlinks(t)
			if err := os.Symlink(t.TempDir(), filepath.Join(dir, "deleted")); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, dir := newStore(t)
			if _, err := s.Create(fakeTicket(0)); err != nil {
				t.Fatal(err)
			}
			tc.make(t, dir)
			if _, _, _, err := s.FindRawAcross(1); !errors.Is(err, ErrDeletedInvalid) {
				t.Errorf("FindRawAcross = %v, want ErrDeletedInvalid", err)
			}
			if _, _, _, err := s.FindExactAcross("T-0001-open.md"); !errors.Is(err, ErrDeletedInvalid) {
				t.Errorf("FindExactAcross = %v, want ErrDeletedInvalid", err)
			}
		})
	}
}

// TestFindRawAcross_UnreadableDeletedPropagates pins the dir-read case:
// an unreadable deleted/ refuses both seams (parity with the archive
// scan error inside findAllNumber).
func TestFindRawAcross_UnreadableDeletedPropagates(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	deletedDir := seedDeletedFile(t, dir, "T-0009-deleted.md", acrossDeletedBody)
	orig := readDir
	swapReadDir(t, func(d string) ([]os.DirEntry, error) {
		if d == deletedDir {
			return nil, dirReadPathError(deletedDir)
		}
		return orig(d)
	})
	if _, _, _, err := s.FindRawAcross(9); err == nil {
		t.Errorf("FindRawAcross through an unreadable deleted/ = nil error")
	}
	if _, _, _, err := s.FindExactAcross("T-0009-deleted.md"); err == nil {
		t.Errorf("FindExactAcross through an unreadable deleted/ = nil error")
	}
}

// TestFindRawAcross_EmptyFileWindow pins the lockless window for the
// deleted seam: an existing but still-empty deleted file maps to
// ErrNotFound, exactly like FindRaw on the main directory.
func TestFindRawAcross_EmptyFileWindow(t *testing.T) {
	s, dir := newStore(t)
	seedDeletedFile(t, dir, "T-0001-deleted.md", "")
	if _, _, _, err := s.FindRawAcross(1); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindRawAcross = %v, want ErrNotFound (empty-file window)", err)
	}
	if _, _, _, err := s.FindExactAcross("T-0001-deleted.md"); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindExactAcross = %v, want ErrNotFound (empty-file window)", err)
	}
}

// TestFindExactAcross_RejectsNonTicketNames pins the traversal guard: a
// name that is not a separator-free ticket file name is refused outright
// on the escape-hatch seam, exactly like FindExact.
func TestFindExactAcross_RejectsNonTicketNames(t *testing.T) {
	s, _ := newStore(t)
	for _, name := range []string{"not-a-ticket.md", "sub/T-0001-open.md"} {
		if _, _, _, err := s.FindExactAcross(name); err == nil || strings.Contains(err.Error(), "ambiguous") {
			t.Errorf("FindExactAcross(%q) = %v, want an outright refusal", name, err)
		}
		if _, _, _, err := s.FindExact(name); err == nil {
			t.Errorf("FindExact(%q) = nil, want an outright refusal", name)
		}
	}
}

// TestFindRawAcross_FindRawStaysDeletedBlind pins invariant 1: the W2
// seams never see deleted/. A number that exists ONLY under deleted/ is
// ErrNotFound for FindRaw and FindExact, so set/edit/archive cannot gain
// an implicit undelete through them.
func TestFindRawAcross_FindRawStaysDeletedBlind(t *testing.T) {
	s, dir := newStore(t)
	seedDeletedFile(t, dir, "T-0001-deleted.md", acrossDeletedBody)
	if _, _, _, err := s.FindRaw(1); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindRaw = %v, want ErrNotFound (deleted-blind)", err)
	}
	if _, _, _, err := s.FindExact("T-0001-deleted.md"); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindExact = %v, want ErrNotFound (deleted-blind)", err)
	}
}
