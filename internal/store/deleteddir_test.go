package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// seededDeletedBody is a parseable ticket body used to seed deleted/
// directly. Its body status line deliberately disagrees with the file
// name: the filename-derived status must win (V19).
const seededDeletedBody = "# T-0002 · BUG: seeded deleted\n" +
	"\n" +
	"- Status (Статус): open\n" +
	"- Priority (Приоритет): normal\n" +
	"- Created (Создан): 2026-11-16 10:00 · by (кем): tester\n" +
	"- Project (Проект): tickets\n" +
	"\n" +
	"## Summary (Кратко)\nseeded body\n" +
	"\n## Details (Подробности)\ndetails\n" +
	"\n## Journal (Журнал)\n" +
	"- 2026-11-16 10:00 — тикет создан (tester).\n"

// mkDeletedDir creates tickets/deleted under dir.
func mkDeletedDir(t *testing.T, dir string) string {
	t.Helper()
	deletedDir := filepath.Join(dir, "deleted")
	if err := os.Mkdir(deletedDir, 0o755); err != nil {
		t.Fatalf("mkdir deleted: %v", err)
	}
	return deletedDir
}

// TestListDeleted_ReturnsSortedFilenameStatus pins the happy path:
// parseable deleted tickets come back sorted by number with the
// filename-derived status, and List (main) never sees them.
func TestListDeleted_ReturnsSortedFilenameStatus(t *testing.T) {
	s, dir := newStore(t)
	deletedDir := mkDeletedDir(t, dir)
	makeFile(t, deletedDir, "T-0002-deleted.md", seededDeletedBody)
	makeFile(t, deletedDir, "T-0001-deleted.md", strings.ReplaceAll(seededDeletedBody, "T-0002", "T-0001"))
	makeFile(t, dir, "T-0003-open.md", strings.ReplaceAll(seededDoneBody, "T-0001", "T-0003"))
	tickets, warns := s.ListDeleted()
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %+v", warns)
	}
	if len(tickets) != 2 || tickets[0].Number != 1 || tickets[1].Number != 2 {
		t.Fatalf("ListDeleted = %+v, want numbers [1 2]", tickets)
	}
	for _, tk := range tickets {
		if tk.Status != domain.StatusDeleted {
			t.Errorf("ticket %d: status = %q, want deleted (filename authoritative)", tk.Number, tk.Status)
		}
	}
	main, _ := s.List()
	if len(main) != 1 || main[0].Number != 3 {
		t.Fatalf("List must not see deleted tickets, got %+v", main)
	}
}

// TestListDeleted_MissingDirEmptyNoWarnings: no deleted/ yet is not an
// error and yields no warnings (mirrors ListArchive).
func TestListDeleted_MissingDirEmptyNoWarnings(t *testing.T) {
	s, _ := newStore(t)
	tickets, warns := s.ListDeleted()
	if len(tickets) != 0 || len(warns) != 0 {
		t.Fatalf("ListDeleted = %+v / %+v, want empty/empty", tickets, warns)
	}
}

// TestListDeleted_RegularFileRejected: a regular file at the deleted
// path is a single deleted-dir warning, never a hard failure.
func TestListDeleted_RegularFileRejected(t *testing.T) {
	s, dir := newStore(t)
	makeFile(t, dir, "deleted", "not a directory")
	tickets, warns := s.ListDeleted()
	if len(tickets) != 0 || len(warns) != 1 {
		t.Fatalf("ListDeleted = %+v / %+v, want one warning", tickets, warns)
	}
	if warns[0].Reason != reasonDeletedDir || !errors.Is(warns[0].Err, ErrDeletedInvalid) {
		t.Fatalf("warn = %+v, want reason %q wrapping ErrDeletedInvalid", warns[0], reasonDeletedDir)
	}
}

// TestListDeleted_SymlinkRejected: a symlinked deleted/ is rejected
// without reading through it (T-0078 containment, mirrors archive).
func TestListDeleted_SymlinkRejected(t *testing.T) {
	requireSymlinks(t)
	s, dir := newStore(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "deleted")); err != nil {
		t.Fatalf("symlink deleted: %v", err)
	}
	tickets, warns := s.ListDeleted()
	if len(tickets) != 0 || len(warns) != 1 || warns[0].Reason != reasonDeletedDir {
		t.Fatalf("ListDeleted = %+v / %+v, want one %q warning", tickets, warns, reasonDeletedDir)
	}
	assertDirEmpty(t, outside) // nothing was written through the link
}

// TestListDeletedWarningReasons pins the per-file and dir-read warning
// producers: an empty ticket file is empty-file, an unreadable deleted
// directory is dir-read (never io-error).
func TestListDeletedWarningReasons(t *testing.T) {
	s, dir := newStore(t)
	deletedDir := mkDeletedDir(t, dir)
	makeFile(t, deletedDir, "T-0002-deleted.md", "")
	tickets, warns := s.ListDeleted()
	if len(tickets) != 0 || len(warns) != 1 || warns[0].Reason != reasonEmptyFile {
		t.Fatalf("ListDeleted = %+v / %+v, want one %q warning", tickets, warns, reasonEmptyFile)
	}
	swapReadDir(t, func(string) ([]os.DirEntry, error) { return nil, dirReadPathError(deletedDir) })
	if _, warns = s.ListDeleted(); len(warns) != 1 || warns[0].Reason != reasonDirRead {
		t.Fatalf("warns = %+v, want one %q warning (never %q)", warns, reasonDirRead, reasonIOError)
	}
}

// TestDeletedScan_Contract pins the internal seam: missing yields
// ("", nil, nil); a real dir yields its entries; an invalid path is a
// wrapped ErrDeletedInvalid, never silenced.
func TestDeletedScan_Contract(t *testing.T) {
	s, dir := newStore(t)
	if d, entries, err := s.deletedScan(); d != "" || entries != nil || err != nil {
		t.Fatalf("missing: got (%q, %+v, %v), want empty/nil/nil", d, entries, err)
	}
	deletedDir := mkDeletedDir(t, dir)
	makeFile(t, deletedDir, "T-0005-deleted.md", seededDeletedBody)
	d, entries, err := s.deletedScan()
	if err != nil || d != deletedDir || len(entries) != 1 || entries[0].Number != 5 {
		t.Fatalf("valid: got (%q, %+v, %v), want dir + entry 5", d, entries, err)
	}
	if err := os.Remove(filepath.Join(deletedDir, "T-0005-deleted.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(deletedDir); err != nil {
		t.Fatal(err)
	}
	makeFile(t, dir, "deleted", "file again")
	if _, _, err = s.deletedScan(); !errors.Is(err, ErrDeletedInvalid) {
		t.Fatalf("invalid: err = %v, want ErrDeletedInvalid", err)
	}
}
