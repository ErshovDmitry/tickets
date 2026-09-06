package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"ticket/internal/domain"
)

// errInjected is the failure the readDir hook returns in dirErr tests.
var errInjected = errors.New("injected readDir failure")

// swapReadDir sets the readDir hook for the duration of t (same swap +
// t.Cleanup idiom as swapLink/swapRemove in store_test.go).
func swapReadDir(t *testing.T, fn func(string) ([]os.DirEntry, error)) {
	t.Helper()
	orig := readDir
	readDir = fn
	t.Cleanup(func() { readDir = orig })
}

// TestArchiveClosed_ScanDirError pins the dirErr contract: an unreadable
// ticket directory fails loudly (no silent empty success), nothing is
// moved and the injected cause survives errors.Is.
func TestArchiveClosed_ScanDirError(t *testing.T) {
	s, dir := makeDone(t)
	swapReadDir(t, func(string) ([]os.DirEntry, error) { return nil, errInjected })
	moved, err := s.ArchiveClosed("tester")
	if err == nil {
		t.Fatal("expected scan error, got nil")
	}
	if !errors.Is(err, errInjected) {
		t.Fatalf("expected errInjected cause, got %v", err)
	}
	if len(moved) != 0 {
		t.Fatalf("moved = %v, want empty on scan failure", moved)
	}
	if _, serr := os.Stat(filepath.Join(dir, "T-0001-done.md")); serr != nil {
		t.Errorf("done ticket must remain in main: %v", serr)
	}
	if _, serr := os.Stat(filepath.Join(dir, "archive")); !errors.Is(serr, fs.ErrNotExist) {
		t.Errorf("archive/ must not be created on scan failure: %v", serr)
	}
	assertNoTmpFiles(t, dir)
}

// TestArchiveClosed_WarningsArchiveValidTickets pins the warning
// contract: a stray non-.md file surfaces as *ScanWarningsError payload,
// while the valid done ticket still archives.
func TestArchiveClosed_WarningsArchiveValidTickets(t *testing.T) {
	s, dir := makeDone(t)
	stray := "README"
	if err := os.WriteFile(filepath.Join(dir, stray), []byte("stray"), 0o644); err != nil {
		t.Fatal(err)
	}
	moved, err := s.ArchiveClosed("tester")
	var swe *ScanWarningsError
	if !errors.As(err, &swe) {
		t.Fatalf("expected *ScanWarningsError, got %v", err)
	}
	want := filepath.Join(dir, "archive", "T-0001-done.md")
	if len(moved) != 1 || moved[0] != want {
		t.Fatalf("moved = %v, want [%s]", moved, want)
	}
	if len(swe.Warnings) != 1 {
		t.Fatalf("warnings = %d, want 1: %v", len(swe.Warnings), swe.Warnings)
	}
	if swe.Warnings[0].Name != stray {
		t.Fatalf("warning name = %q, want %q", swe.Warnings[0].Name, stray)
	}
	// The stray file is skipped by the scan, never deleted.
	if _, serr := os.Stat(filepath.Join(dir, stray)); serr != nil {
		t.Errorf("stray file must remain: %v", serr)
	}
	assertNoTmpFiles(t, dir)
	assertNoTmpFiles(t, filepath.Join(dir, "archive"))
}

// TestArchiveClosed_WarningsOnlyNoClosed pins the exception to the bash
// rc=0 contract: warnings with zero done/closed tickets yield an empty
// move list plus a *ScanWarningsError.
func TestArchiveClosed_WarningsOnlyNoClosed(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil { // T-0001-open.md
		t.Fatal(err)
	}
	stray := "NOTES.TXT"
	if err := os.WriteFile(filepath.Join(dir, stray), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	moved, err := s.ArchiveClosed("tester")
	var swe *ScanWarningsError
	if !errors.As(err, &swe) {
		t.Fatalf("expected *ScanWarningsError, got %v", err)
	}
	if len(moved) != 0 {
		t.Fatalf("moved = %v, want empty", moved)
	}
	if len(swe.Warnings) != 1 || swe.Warnings[0].Name != stray {
		t.Fatalf("warnings = %v, want exactly one for %q", swe.Warnings, stray)
	}
	if _, serr := os.Stat(filepath.Join(dir, "T-0001-open.md")); serr != nil {
		t.Errorf("open ticket must remain: %v", serr)
	}
	assertNoTmpFiles(t, dir)
}

// TestArchiveClosed_MidRunCollisionReturnsMoved pins the partial-failure
// contract: the first ticket's path is still returned when the second
// move collides, and the error carries *CollisionError for the blocked
// ticket.
func TestArchiveClosed_MidRunCollisionReturnsMoved(t *testing.T) {
	s, dir := newStore(t)
	for i := 0; i < 2; i++ {
		if _, err := s.Create(fakeTicket(0)); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []int{1, 2} {
		if _, err := s.SetStatus(n, domain.StatusDone, "tester", ""); err != nil {
			t.Fatal(err)
		}
	}
	archiveDir := filepath.Join(dir, "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(archiveDir, "T-0002-done.md")
	if err := os.WriteFile(blocked, []byte("FOREIGN"), 0o644); err != nil {
		t.Fatal(err)
	}
	moved, err := s.ArchiveClosed("tester")
	want := filepath.Join(archiveDir, "T-0001-done.md")
	if len(moved) != 1 || moved[0] != want {
		t.Fatalf("moved = %v, want [%s]", moved, want)
	}
	var coll *CollisionError
	if !errors.As(err, &coll) {
		t.Fatalf("expected *CollisionError, got %v", err)
	}
	if coll.Target != blocked {
		t.Errorf("CollisionError.Target = %q, want %q", coll.Target, blocked)
	}
	if _, serr := os.Stat(filepath.Join(dir, "T-0002-done.md")); serr != nil {
		t.Errorf("blocked ticket must remain in main: %v", serr)
	}
	assertNoTmpFiles(t, dir)
	assertNoTmpFiles(t, archiveDir)
}

// TestArchiveClosed_MidRunCollisionKeepsWarnings pins the errors.Join
// contract for the mid-run collision path: when a move collides after
// some tickets have landed and the scan also produced warnings, the
// returned error carries BOTH payloads — the *CollisionError for the
// blocked ticket and the *ScanWarningsError whose warnings survive the
// join (nothing is silently dropped).
func TestArchiveClosed_MidRunCollisionKeepsWarnings(t *testing.T) {
	s, dir := newStore(t)
	for i := 0; i < 2; i++ {
		if _, err := s.Create(fakeTicket(0)); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []int{1, 2} {
		if _, err := s.SetStatus(n, domain.StatusDone, "tester", ""); err != nil {
			t.Fatal(err)
		}
	}
	stray := "README"
	if err := os.WriteFile(filepath.Join(dir, stray), []byte("stray"), 0o644); err != nil {
		t.Fatal(err)
	}
	archiveDir := filepath.Join(dir, "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(archiveDir, "T-0002-done.md")
	if err := os.WriteFile(blocked, []byte("FOREIGN"), 0o644); err != nil {
		t.Fatal(err)
	}
	moved, err := s.ArchiveClosed("tester")
	want := filepath.Join(archiveDir, "T-0001-done.md")
	if len(moved) != 1 || moved[0] != want {
		t.Fatalf("moved = %v, want [%s]", moved, want)
	}
	var coll *CollisionError
	if !errors.As(err, &coll) {
		t.Fatalf("expected *CollisionError in joined error, got %v", err)
	}
	if coll.Target != blocked {
		t.Errorf("CollisionError.Target = %q, want %q", coll.Target, blocked)
	}
	var swe *ScanWarningsError
	if !errors.As(err, &swe) {
		t.Fatalf("expected *ScanWarningsError in joined error, got %v", err)
	}
	if len(swe.Warnings) != 1 || swe.Warnings[0].Name != stray {
		t.Fatalf("warnings = %v, want exactly one for %q", swe.Warnings, stray)
	}
	assertNoTmpFiles(t, dir)
	assertNoTmpFiles(t, archiveDir)
}
