package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ticket/internal/domain"
)

// seededDoneBody is a parseable done-status ticket body used to seed the
// main directory directly: with a hostile archive path, Create/SetStatus
// themselves fail (T-0078), so the fixture bypasses the store API.
const seededDoneBody = "# T-0001 · BUG: seeded done\n" +
	"\n" +
	"- Status (Статус): done\n" +
	"- Priority (Приоритет): normal\n" +
	"- Created (Создан): 2026-09-08 10:00 · by (кем): tester\n" +
	"- Project (Проект): tickets\n" +
	"\n" +
	"## Summary (Кратко)\nseeded body\n" +
	"\n## Details (Подробности)\ndetails\n" +
	"\n## Journal (Журнал)\n" +
	"- 2026-09-08 10:00 — тикет создан (tester).\n"

// symlinkedArchiveStore builds a store holding a done T-0001 ticket in
// the main dir and tickets/archive as a symlink to an empty OUTSIDE
// directory (separate t.TempDir). Skips when symlinks are unavailable.
func symlinkedArchiveStore(t *testing.T) (s *Store, dir, outside string) {
	t.Helper()
	requireSymlinks(t)
	dir = t.TempDir()
	outside = t.TempDir()
	makeFile(t, dir, "T-0001-done.md", seededDoneBody)
	if err := os.Symlink(outside, filepath.Join(dir, "archive")); err != nil {
		t.Fatalf("symlink archive: %v", err)
	}
	s, err := New(dir)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	return s, dir, outside
}

// assertDirEmpty fails when dir contains any entry.
func assertDirEmpty(t *testing.T, dir string) {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	if len(ents) != 0 {
		t.Fatalf("expected empty %s, got %d entries", dir, len(ents))
	}
}

// TestArchiveDir_SymlinkArchive_BlocksWrites pins the write-path escape
// fix: with tickets/archive symlinked outside, every mutation must fail
// loudly and nothing may land in the linked outside directory (T-0078).
func TestArchiveDir_SymlinkArchive_BlocksWrites(t *testing.T) {
	s, dir, outside := symlinkedArchiveStore(t)

	if _, err := s.Archive(1, "tester"); !errors.Is(err, ErrArchiveInvalid) {
		t.Fatalf("Archive err = %v, want ErrArchiveInvalid", err)
	}
	if moved, err := s.ArchiveClosed("tester"); err == nil || !errors.Is(err, ErrArchiveInvalid) {
		t.Fatalf("ArchiveClosed err = %v (moved %v), want ErrArchiveInvalid", err, moved)
	}
	if _, err := s.SetStatus(1, domain.StatusWip, "tester", "x"); !errors.Is(err, ErrArchiveInvalid) {
		t.Fatalf("SetStatus err = %v, want ErrArchiveInvalid", err)
	}
	if _, err := s.Create(fakeTicket(0)); !errors.Is(err, ErrArchiveInvalid) {
		t.Fatalf("Create err = %v, want ErrArchiveInvalid", err)
	}

	assertDirEmpty(t, outside)
	if _, serr := os.Stat(filepath.Join(dir, "T-0001-done.md")); serr != nil {
		t.Errorf("main ticket must remain untouched: %v", serr)
	}
}

// TestArchiveDir_SymlinkArchive_BlocksReads pins the read-path escape
// fix: a bait ticket inside the linked outside directory must never be
// returned by FindRaw or ListArchive — the invalid archive fails loudly
// instead of being scanned (T-0078).
func TestArchiveDir_SymlinkArchive_BlocksReads(t *testing.T) {
	s, _, outside := symlinkedArchiveStore(t)

	baitPath := filepath.Join(outside, "T-0009-open.md")
	baitBody := []byte("# T-0009 · BUG: outside bait\n\n- Status (Статус): open\nTOPSECRET-SENTINEL\n")
	if werr := os.WriteFile(baitPath, baitBody, 0o600); werr != nil {
		t.Fatalf("write bait: %v", werr)
	}

	tk, name, raw, ferr := s.FindRaw(9)
	if ferr == nil {
		t.Fatalf("FindRaw(9) returned %+v/%s through a symlinked archive", tk, name)
	}
	if errors.Is(ferr, ErrNotFound) {
		t.Fatalf("FindRaw(9) err = %v, want fail-loud invalid-archive error", ferr)
	}
	if raw != nil || name != "" || bytes.Contains(raw, []byte("TOPSECRET-SENTINEL")) {
		t.Fatalf("outside content leaked: name=%q raw=%q", name, raw)
	}

	tickets, warns := s.ListArchive()
	if len(tickets) != 0 {
		t.Fatalf("ListArchive returned tickets: %+v", tickets)
	}
	if len(warns) != 1 || !errors.Is(warns[0].Err, ErrArchiveInvalid) {
		t.Fatalf("ListArchive warns = %+v, want 1 warning with ErrArchiveInvalid", warns)
	}

	got, rerr := os.ReadFile(baitPath)
	if rerr != nil || !bytes.Equal(got, baitBody) {
		t.Fatalf("bait mutated: err=%v data=%q", rerr, got)
	}
}

// TestArchiveDir_RegularFileArchive_Rejected pins the non-directory
// branch: tickets/archive as a REGULAR FILE must trigger the same
// rejections and stay byte-identical (T-0078).
func TestArchiveDir_RegularFileArchive_Rejected(t *testing.T) {
	s, dir := newStore(t)
	makeFile(t, dir, "T-0001-done.md", seededDoneBody)
	archivePath := filepath.Join(dir, "archive")
	if werr := os.WriteFile(archivePath, []byte("not a dir"), 0o644); werr != nil {
		t.Fatal(werr)
	}

	if _, err := s.Archive(1, "tester"); !errors.Is(err, ErrArchiveInvalid) {
		t.Fatalf("Archive err = %v, want ErrArchiveInvalid", err)
	}
	if _, err := s.Create(fakeTicket(0)); !errors.Is(err, ErrArchiveInvalid) {
		t.Fatalf("Create err = %v, want ErrArchiveInvalid", err)
	}
	if _, err := s.SetStatus(1, domain.StatusWip, "tester", "x"); !errors.Is(err, ErrArchiveInvalid) {
		t.Fatalf("SetStatus err = %v, want ErrArchiveInvalid", err)
	}
	if _, _, _, err := s.FindRaw(9); err == nil {
		t.Fatal("FindRaw(9) must fail loudly on a regular-file archive")
	}
	if _, warns := s.ListArchive(); len(warns) != 1 || !errors.Is(warns[0].Err, ErrArchiveInvalid) {
		t.Fatalf("ListArchive warns = %+v, want ErrArchiveInvalid warning", warns)
	}

	if got, rerr := os.ReadFile(archivePath); rerr != nil || string(got) != "not a dir" {
		t.Fatalf("archive file mutated: err=%v data=%q", rerr, got)
	}
}

// TestArchiveDir_ValidArchiveRegression guards against over-blocking:
// with a normal (absent-then-created) archive/ every operation works as
// before (T-0078).
func TestArchiveDir_ValidArchiveRegression(t *testing.T) {
	s, dir := newStore(t)

	if n, err := s.Create(fakeTicket(0)); err != nil || n != 1 {
		t.Fatalf("Create = %d, %v", n, err)
	}
	if _, err := s.SetStatus(1, domain.StatusDone, "tester", ""); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	target, err := s.Archive(1, "tester")
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if target != filepath.Join(dir, "archive", "T-0001-done.md") {
		t.Fatalf("target = %q", target)
	}
	tk, err := s.Find(1)
	if err != nil || tk.Status != domain.StatusDone {
		t.Fatalf("Find(1) from archive = %+v, %v", tk, err)
	}
	if n, err := s.Create(fakeTicket(0)); err != nil || n != 2 {
		t.Fatalf("Create after archive = %d, %v", n, err)
	}
	if _, err := s.SetStatus(2, domain.StatusWip, "tester", ""); err != nil {
		t.Fatalf("SetStatus(2): %v", err)
	}
	arch, warns := s.ListArchive()
	if len(arch) != 1 || arch[0].Number != 1 || len(warns) != 0 {
		t.Fatalf("ListArchive = %+v/%+v, want exactly ticket 1", arch, warns)
	}
}
