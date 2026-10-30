package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// dirReadPathError returns the real *fs.PathError a directory read fails
// with. The dir-read producer tests inject it, so they prove the explicit
// dir-read reason wins over the io-error that reasonOf would derive from
// a *fs.PathError — a plain errors.New proves nothing here.
func dirReadPathError(dir string) error {
	return &fs.PathError{Op: "readdir", Path: dir, Err: fs.ErrPermission}
}

// TestReasonOf_Mapping pins the derived reason for every error shape the
// read path produces. Wrapped causes must map too: reasonOf walks the
// chain, so %w-wrapped errors (not %v) stay classifiable. LOCK W5 (plan
// page 86530021-f099-4c4f-8a49-07711e056582): this mapping test keeps
// its own verdict, with no clustering of unrelated assertions.
func TestReasonOf_Mapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"file swapped", errFileSwapped, reasonFileSwapped},
		{"empty file", errEmptyTicket, reasonEmptyFile},
		{"not regular", errNotRegularFile, reasonNotRegularFile},
		{"bad extension", errBadExtension, reasonBadExtension},
		{"h1 mismatch", &h1MismatchError{H1: 2, WantNum: 1}, reasonH1Mismatch},
		{"wrapped h1 mismatch", fmt.Errorf("read: %w", &h1MismatchError{H1: 2, WantNum: 1}), reasonH1Mismatch},
		{"path error", &fs.PathError{Op: "open", Path: "x", Err: fs.ErrPermission}, reasonIOError},
		{"wrapped sentinel", fmt.Errorf("read: %w", errEmptyTicket), reasonEmptyFile},
		{"unknown", errors.New("something else"), reasonParse},
	}
	for _, tc := range cases {
		if got := reasonOf(tc.err); got != tc.want {
			t.Errorf("reasonOf(%s) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestArchiveReason_Mapping pins the archive-specific additions: an
// invalid archive path keeps archive-dir (also when wrapped), an OS
// failure is io-error, and anything else defers to reasonOf. LOCK W5 (plan
// page 86530021-f099-4c4f-8a49-07711e056582): this mapping test keeps its
// own verdict, with no clustering of unrelated assertions.
func TestArchiveReason_Mapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"invalid archive", fmt.Errorf("store: x: %w", ErrArchiveInvalid), reasonArchiveDir},
		{"path error", &fs.PathError{Op: "open", Path: "x", Err: fs.ErrNotExist}, reasonIOError},
		{"empty file", errEmptyTicket, reasonEmptyFile},
		{"unknown", errors.New("something else"), reasonParse},
	}
	for _, tc := range cases {
		if got := archiveReason(tc.err); got != tc.want {
			t.Errorf("archiveReason(%s) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestH1MismatchError_BytesAndType pins the typed H1 error: the same
// message bytes as the former bare fmt.Errorf and reachable through
// errors.As from readTicketHeader.
func TestH1MismatchError_BytesAndType(t *testing.T) {
	err := error(&h1MismatchError{H1: 2, WantNum: 1})
	if got, want := err.Error(), "H1 number 2 does not match filename T-0001"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	dir := t.TempDir()
	makeFile(t, dir, "T-0001-open.md", "# T-0002 · BUG: wrong\n\n- Status (Статус): open\n")
	_, rerr := readTicketHeader(filepath.Join(dir, "T-0001-open.md"), 1)
	var h1 *h1MismatchError
	if !errors.As(rerr, &h1) {
		t.Fatalf("readTicketHeader err = %v, want *h1MismatchError", rerr)
	}
	if h1.H1 != 2 || h1.WantNum != 1 {
		t.Errorf("h1 = %+v, want H1=2 WantNum=1", h1)
	}
}

// TestParseWarning_ReasonIsAdditive pins byte compatibility: setting a
// Reason never changes Error(), so warning text and ScanWarningsError
// text stay exactly as before.
func TestParseWarning_ReasonIsAdditive(t *testing.T) {
	w := ParseWarning{Name: "T-0001-open.md", Err: errEmptyTicket, Reason: reasonEmptyFile}
	if got, want := w.Error(), "T-0001-open.md: "+errEmptyTicket.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	swe := &ScanWarningsError{Warnings: []ParseWarning{w}}
	if got, want := swe.Error(), "store: scan warnings: "+w.Error(); got != want {
		t.Fatalf("ScanWarningsError = %q, want %q", got, want)
	}
}

// TestSafeOpenReason_EmptyAndSwappedStayDistinct pins that the two
// safe-open sentinels keep separate reasons instead of collapsing into
// one parse failure.
func TestSafeOpenReason_EmptyAndSwappedStayDistinct(t *testing.T) {
	dir := t.TempDir()
	makeFile(t, dir, "T-0001-open.md", "")
	_, err := readTicketHeader(filepath.Join(dir, "T-0001-open.md"), 1)
	if !errors.Is(err, errEmptyTicket) {
		t.Fatalf("err = %v, want errEmptyTicket", err)
	}
	if got := reasonOf(err); got != reasonEmptyFile {
		t.Errorf("empty file reason = %q, want %q", got, reasonEmptyFile)
	}
	if got := reasonOf(errFileSwapped); got == reasonEmptyFile || got != reasonFileSwapped {
		t.Errorf("swapped file reason = %q, want %q", got, reasonFileSwapped)
	}
	if err := os.WriteFile(filepath.Join(dir, "T-0002-open.md"), []byte("# T-0002 · BUG: ok\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
