package store

// T-0189 W3-C1a: scan-only guard seams for the set/edit commands
// (LocateNumber / LocateNumberDeleted, find_deleted.go). The CLI combines
// the two to refuse a deleted-only number (exit 1) or report a
// deleted+live ambiguity (exit 2) BEFORE touching any file. No seam reads
// a body or writes anything; deleted scan errors refuse loudly.

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestLocateNumber_DeletedOnly pins guard case (a): the live half is
// empty, the deleted half names the file — the CLI refuses with exit 1.
func TestLocateNumber_DeletedOnly(t *testing.T) {
	s, dir := newStore(t)
	deletedDir := seedDeletedFile(t, dir, "T-0001-deleted.md", acrossDeletedBody)

	live, err := s.LocateNumber(1)
	if err != nil {
		t.Fatalf("LocateNumber: %v", err)
	}
	if len(live) != 0 {
		t.Errorf("LocateNumber = %v, want empty (nothing live)", live)
	}
	deleted, err := s.LocateNumberDeleted(1)
	if err != nil {
		t.Fatalf("LocateNumberDeleted: %v", err)
	}
	want := []string{filepath.Join(deletedDir, "T-0001-deleted.md")}
	if !reflect.DeepEqual(deleted, want) {
		t.Errorf("LocateNumberDeleted = %v, want %v", deleted, want)
	}
}

// TestLocateNumber_DeletedAndMain pins guard case (b) for the main
// directory: both halves report their file, and the combined sorted set
// is exactly what an exit-2 ambiguity message prints.
func TestLocateNumber_DeletedAndMain(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	deletedDir := seedDeletedFile(t, dir, "T-0001-deleted.md", acrossDeletedBody)

	live, err := s.LocateNumber(1)
	if err != nil {
		t.Fatalf("LocateNumber: %v", err)
	}
	deleted, err := s.LocateNumberDeleted(1)
	if err != nil {
		t.Fatalf("LocateNumberDeleted: %v", err)
	}
	combined := append(live, deleted...)
	want := []string{filepath.Join(dir, "T-0001-open.md"), filepath.Join(deletedDir, "T-0001-deleted.md")}
	if !reflect.DeepEqual(combined, want) {
		t.Errorf("combined = %v, want %v", combined, want)
	}
}

// TestLocateNumber_DeletedAndArchive pins guard case (b) for the archive:
// an archived same-number file plus its deleted successor is an exit-2
// ambiguity as well, not a silent archive hit.
func TestLocateNumber_DeletedAndArchive(t *testing.T) {
	s, dir := newStore(t)
	archiveDir := filepath.Join(dir, "archive")
	if err := os.Mkdir(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	makeFile(t, archiveDir, "T-0001-done.md", seededDoneBody)
	deletedDir := seedDeletedFile(t, dir, "T-0001-deleted.md", acrossDeletedBody)

	live, err := s.LocateNumber(1)
	if err != nil {
		t.Fatalf("LocateNumber: %v", err)
	}
	deleted, err := s.LocateNumberDeleted(1)
	if err != nil {
		t.Fatalf("LocateNumberDeleted: %v", err)
	}
	combined := append(live, deleted...)
	want := []string{filepath.Join(archiveDir, "T-0001-done.md"), filepath.Join(deletedDir, "T-0001-deleted.md")}
	if !reflect.DeepEqual(combined, want) {
		t.Errorf("combined = %v, want %v", combined, want)
	}
}

// TestLocateNumber_AbsentAndNoDeletedDir pins the W2-compatible negative:
// with no deleted/ at all both halves come back empty without an error,
// so a store that predates W1 behaves exactly as before.
func TestLocateNumber_AbsentAndNoDeletedDir(t *testing.T) {
	s, _ := newStore(t)
	live, err := s.LocateNumber(1)
	if err != nil || len(live) != 0 {
		t.Errorf("LocateNumber = %v, %v; want empty, nil", live, err)
	}
	deleted, err := s.LocateNumberDeleted(1)
	if err != nil || len(deleted) != 0 {
		t.Errorf("LocateNumberDeleted = %v, %v; want empty, nil", deleted, err)
	}
}

// TestLocateNumber_DeletedBlindnessOfLiveHalf pins that the live half
// never consults deleted/: with only a deleted file, LocateNumber stays
// empty even though LocateNumberDeleted sees it.
func TestLocateNumber_DeletedBlindnessOfLiveHalf(t *testing.T) {
	s, dir := newStore(t)
	seedDeletedFile(t, dir, "T-0001-deleted.md", acrossDeletedBody)
	live, err := s.LocateNumber(1)
	if err != nil || len(live) != 0 {
		t.Errorf("LocateNumber = %v, %v; want empty, nil (deleted-blind)", live, err)
	}
}

// TestLocateNumber_InvalidDeletedPropagates pins that an invalid deleted
// path refuses the deleted half loudly while the live half — which never
// touches deleted/ — still answers.
func TestLocateNumber_InvalidDeletedPropagates(t *testing.T) {
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
			if _, err := s.LocateNumberDeleted(1); !errors.Is(err, ErrDeletedInvalid) {
				t.Errorf("LocateNumberDeleted = %v, want ErrDeletedInvalid", err)
			}
			live, err := s.LocateNumber(1)
			if err != nil || len(live) != 1 {
				t.Errorf("LocateNumber = %v, %v; want the live file, nil", live, err)
			}
		})
	}
}

// TestLocateNumber_UnreadableDeletedPropagates pins the dir-read case for
// the guard seam: the deleted half refuses, the live half still answers.
func TestLocateNumber_UnreadableDeletedPropagates(t *testing.T) {
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
	if _, err := s.LocateNumberDeleted(9); err == nil {
		t.Errorf("LocateNumberDeleted through an unreadable deleted/ = nil error")
	}
	if live, err := s.LocateNumber(1); err != nil || len(live) != 1 {
		t.Errorf("LocateNumber = %v, %v; want the live file, nil", live, err)
	}
}

// TestLocateNumber_ScanOnlyDoesNotReadBody pins the scan-only contract:
// a deleted file whose body is empty (unreadable as a ticket) is still
// located — the guard never opens the file, so it cannot fail on body
// content and cannot mutate anything.
func TestLocateNumber_ScanOnlyDoesNotReadBody(t *testing.T) {
	s, dir := newStore(t)
	deletedDir := seedDeletedFile(t, dir, "T-0001-deleted.md", "") // empty: body read would fail
	got, err := s.LocateNumberDeleted(1)
	if err != nil {
		t.Fatalf("LocateNumberDeleted: %v", err)
	}
	want := []string{filepath.Join(deletedDir, "T-0001-deleted.md")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LocateNumberDeleted = %v, want %v", got, want)
	}
}

// TestLocateNumber_SortedAndDeduplicated pins the output shape: several
// same-number live files come back as one sorted path list, the exact
// shape an ambiguity message composes from.
func TestLocateNumber_SortedAndDeduplicated(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	body := mustRead(t, dir, 1)
	makeFile(t, dir, "T-0001-wip.md", string(body))

	got, err := s.LocateNumber(1)
	if err != nil {
		t.Fatalf("LocateNumber: %v", err)
	}
	want := []string{filepath.Join(dir, "T-0001-open.md"), filepath.Join(dir, "T-0001-wip.md")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LocateNumber = %v, want sorted %v", got, want)
	}
}
