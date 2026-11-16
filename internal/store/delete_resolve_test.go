package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"ticket/internal/domain"
)

// seedNamed writes an empty ticket-named file (resolve works on names
// alone, no body is read).
func seedNamed(t *testing.T, dir string, n int, st domain.Status) {
	t.Helper()
	makeFile(t, dir, domain.Filename(n, st), "")
}

// TestResolveDelete_NumberAcrossDirs pins number resolution in each
// of the three directories, with the filename-derived status.
func TestResolveDelete_NumberAcrossDirs(t *testing.T) {
	s, dir := newStore(t)
	archiveDir := filepath.Join(dir, "archive")
	deletedDir := mkDeletedDir(t, dir)
	if err := os.Mkdir(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	seedNamed(t, dir, 1, domain.StatusOpen)
	seedNamed(t, archiveDir, 2, domain.StatusDone)
	seedNamed(t, deletedDir, 3, domain.StatusDeleted)
	for _, tc := range []struct {
		ref string
		n   int
		st  domain.Status
		dir string
	}{
		{"1", 1, domain.StatusOpen, dir},
		{"2", 2, domain.StatusDone, archiveDir},
		{"3", 3, domain.StatusDeleted, deletedDir},
		{"0001", 1, domain.StatusOpen, dir},
	} {
		got, err := s.ResolveDelete(tc.ref)
		if err != nil {
			t.Fatalf("ResolveDelete(%q): %v", tc.ref, err)
		}
		want := DeleteCandidate{Number: tc.n, Status: tc.st, Path: filepath.Join(tc.dir, domain.Filename(tc.n, tc.st))}
		if got != want {
			t.Errorf("ResolveDelete(%q) = %+v, want %+v", tc.ref, got, want)
		}
	}
}

// TestResolveDelete_ExactBasename pins the filename selector in each
// directory, including a deleted-status name.
func TestResolveDelete_ExactBasename(t *testing.T) {
	s, dir := newStore(t)
	deletedDir := mkDeletedDir(t, dir)
	seedNamed(t, dir, 1, domain.StatusWip)
	seedNamed(t, deletedDir, 2, domain.StatusDeleted)
	for _, tc := range []struct {
		name string
		dir  string
	}{
		{"T-0001-wip.md", dir},
		{"T-0002-deleted.md", deletedDir},
	} {
		got, err := s.ResolveDelete(tc.name)
		if err != nil {
			t.Fatalf("ResolveDelete(%q): %v", tc.name, err)
		}
		if got.Path != filepath.Join(tc.dir, tc.name) {
			t.Errorf("ResolveDelete(%q).Path = %q, want %q", tc.name, got.Path, filepath.Join(tc.dir, tc.name))
		}
	}
}

// TestResolveDelete_DuplicateNumber pins the typed ambiguity for a
// number carried by several files across any of the three dirs:
// every conflicting absolute path, sorted.
func TestResolveDelete_DuplicateNumber(t *testing.T) {
	s, dir := newStore(t)
	archiveDir := filepath.Join(dir, "archive")
	deletedDir := mkDeletedDir(t, dir)
	if err := os.Mkdir(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	seedNamed(t, dir, 1, domain.StatusOpen)
	seedNamed(t, dir, 1, domain.StatusWip) // crash pair in main
	seedNamed(t, archiveDir, 1, domain.StatusDone)
	seedNamed(t, deletedDir, 1, domain.StatusDeleted)
	_, err := s.ResolveDelete("1")
	var amb *AmbiguousNumberError
	if !errors.As(err, &amb) || !errors.Is(err, ErrAmbiguousNumber) {
		t.Fatalf("err = %v, want *AmbiguousNumberError", err)
	}
	// sort.Strings is bytewise: "T-…" < "archive" < "deleted".
	want := []string{
		filepath.Join(dir, "T-0001-open.md"),
		filepath.Join(dir, "T-0001-wip.md"),
		filepath.Join(archiveDir, "T-0001-done.md"),
		filepath.Join(deletedDir, "T-0001-deleted.md"),
	}
	if amb.Number != 1 || !reflect.DeepEqual(amb.Paths, want) {
		t.Fatalf("amb = %+v, want number 1 paths %v", amb, want)
	}
}

// TestResolveDelete_IdenticalBasenameAcrossDirs pins that the exact
// filename selector is ambiguous when the same base name exists in
// more than one directory (main+archive and main+deleted).
func TestResolveDelete_IdenticalBasenameAcrossDirs(t *testing.T) {
	for _, sub := range []string{"archive", "deleted"} {
		t.Run(sub, func(t *testing.T) {
			s, dir := newStore(t)
			other := filepath.Join(dir, sub)
			if err := os.Mkdir(other, 0o755); err != nil {
				t.Fatal(err)
			}
			seedNamed(t, dir, 1, domain.StatusOpen)
			seedNamed(t, other, 1, domain.StatusOpen)
			_, err := s.ResolveDelete("T-0001-open.md")
			var amb *AmbiguousNumberError
			if !errors.As(err, &amb) || !errors.Is(err, ErrAmbiguousNumber) {
				t.Fatalf("err = %v, want *AmbiguousNumberError", err)
			}
			want := []string{filepath.Join(dir, "T-0001-open.md"), filepath.Join(other, "T-0001-open.md")}
			if !reflect.DeepEqual(amb.Paths, want) {
				t.Fatalf("paths = %v, want %v", amb.Paths, want)
			}
		})
	}
}

// TestResolveDelete_InvalidRefs pins the reject-before-scan contract
// for traversal, separators and non-number/non-filename input.
func TestResolveDelete_InvalidRefs(t *testing.T) {
	s, dir := newStore(t)
	seedNamed(t, dir, 1, domain.StatusOpen)
	for _, ref := range []string{
		"", "../1", "..\\1", "1/..", "T-0001-open.md/../T-0002-open.md",
		"abc", "-1", "+1", "1.5", "12x", " 1", "1 ",
		"T-0001-open.txt", "T-00001-open.md", "T-0001-OPEN.md",
		"99999999999999999999",
	} {
		if _, err := s.ResolveDelete(ref); err == nil || errors.Is(err, ErrNotFound) {
			t.Errorf("ResolveDelete(%q): err = %v, want invalid-ref rejection", ref, err)
		}
	}
}

// TestResolveDelete_NumberOutOfRange pins the numeric range contract:
// 0, numbers above maxTicketNumber and Atoi overflow are invalid refs
// before any scan (never ErrNotFound), on every architecture; the
// upper boundary 9999 still resolves (W1 A4).
func TestResolveDelete_NumberOutOfRange(t *testing.T) {
	s, dir := newStore(t)
	seedNamed(t, dir, 9999, domain.StatusOpen)
	for _, ref := range []string{"0", "0000", "10000", "99999", "99999999999999999999"} {
		if _, err := s.ResolveDelete(ref); err == nil || errors.Is(err, ErrNotFound) {
			t.Errorf("ResolveDelete(%q): err = %v, want invalid-ref rejection", ref, err)
		}
	}
	if got, err := s.ResolveDelete("9999"); err != nil || got.Number != 9999 {
		t.Errorf("ResolveDelete(\"9999\") = %+v, %v, want number 9999", got, err)
	}
}

// TestResolveDelete_NotFound pins ErrNotFound for absent numbers and
// names.
func TestResolveDelete_NotFound(t *testing.T) {
	s, _ := newStore(t)
	for _, ref := range []string{"42", "T-0042-open.md"} {
		if _, err := s.ResolveDelete(ref); !errors.Is(err, ErrNotFound) {
			t.Errorf("ResolveDelete(%q): err = %v, want ErrNotFound", ref, err)
		}
	}
}

// TestResolveDelete_DeletedDirErrors pins that an invalid or
// unreadable deleted directory is never silenced by resolution.
func TestResolveDelete_DeletedDirErrors(t *testing.T) {
	s, dir := newStore(t)
	seedNamed(t, dir, 1, domain.StatusOpen)
	makeFile(t, dir, "deleted", "not a directory")
	if _, err := s.ResolveDelete("1"); !errors.Is(err, ErrDeletedInvalid) {
		t.Fatalf("invalid deleted: err = %v, want ErrDeletedInvalid", err)
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
	if _, err := s.ResolveDelete("1"); err == nil {
		t.Fatal("unreadable deleted: want error, got nil")
	}
}

// TestResolveDelete_MainDirError pins that an unreadable main
// directory is never silenced by resolution: the injected readdir
// cause survives the wrap.
func TestResolveDelete_MainDirError(t *testing.T) {
	s, dir := newStore(t)
	seedNamed(t, dir, 1, domain.StatusOpen)
	swapReadDir(t, func(p string) ([]os.DirEntry, error) {
		if p == dir {
			return nil, dirReadPathError(dir)
		}
		return os.ReadDir(p)
	})
	if _, err := s.ResolveDelete("1"); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("unreadable main: err = %v, want wrapped readdir failure", err)
	}
}

// TestResolveDelete_ArchiveDirErrors pins that an invalid or
// unreadable archive directory is never silenced by resolution
// (same contract as TestResolveDelete_DeletedDirErrors).
func TestResolveDelete_ArchiveDirErrors(t *testing.T) {
	s, dir := newStore(t)
	seedNamed(t, dir, 1, domain.StatusOpen)
	makeFile(t, dir, "archive", "not a directory")
	if _, err := s.ResolveDelete("1"); !errors.Is(err, ErrArchiveInvalid) {
		t.Fatalf("invalid archive: err = %v, want ErrArchiveInvalid", err)
	}
	if err := os.Remove(filepath.Join(dir, "archive")); err != nil {
		t.Fatal(err)
	}
	archiveDir := filepath.Join(dir, "archive")
	if err := os.Mkdir(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	swapReadDir(t, func(p string) ([]os.DirEntry, error) {
		if p == archiveDir {
			return nil, dirReadPathError(archiveDir)
		}
		return os.ReadDir(p)
	})
	if _, err := s.ResolveDelete("1"); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("unreadable archive: err = %v, want wrapped readdir failure", err)
	}
}
