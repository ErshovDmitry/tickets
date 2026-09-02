package store

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"ticket/internal/domain"
)

// makeFile writes a file inside dir with the given name and body.
func makeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

func TestScan_EmptyDirectory(t *testing.T) {
	s, dir := newStore(t)
	entries, warns, err := s.scan()
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no entries, got %d", len(entries))
	}
	if len(warns) != 0 {
		t.Errorf("expected no warnings, got %d: %+v", len(warns), warns)
	}
	_ = dir
}

func TestScan_SkipsHiddenAndTmp(t *testing.T) {
	s, dir := newStore(t)
	makeFile(t, dir, ".lock", "")
	makeFile(t, dir, ".hidden", "x")
	makeFile(t, dir, ".T-9999-open.1-1.md.tmp", "partial")
	// A real ticket — must be picked up.
	makeFile(t, dir, "T-0001-open.md", "body")
	// Another stray hidden pattern.
	makeFile(t, dir, ".T-0002-wip.1-1.md.tmp", "")

	entries, warns, err := s.scan()
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d: %+v", len(entries), entries)
	}
	if entries[0].Number != 1 {
		t.Errorf("expected number=1, got %d", entries[0].Number)
	}
	if len(warns) != 0 {
		t.Errorf("hidden/tmp should NOT emit warnings; got %+v", warns)
	}
}

func TestScan_SkipsDirectories(t *testing.T) {
	s, dir := newStore(t)
	if err := os.Mkdir(filepath.Join(dir, "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	makeFile(t, dir, "T-0001-open.md", "body")
	entries, _, err := s.scan()
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestScan_BadFilenameProducesWarning(t *testing.T) {
	s, dir := newStore(t)
	makeFile(t, dir, "T-0001-open.md", "good")
	makeFile(t, dir, "T-0002-OPEN.md", "wrong case status")
	makeFile(t, dir, "T-NOTNUM-open.md", "non-numeric")
	makeFile(t, dir, "T-0001-open.txt", "wrong ext")
	makeFile(t, dir, "README.md", "non-ticket")
	entries, warns, err := s.scan()
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(entries) != 1 || entries[0].Number != 1 {
		t.Fatalf("expected only T-0001-open.md as entry; got %+v", entries)
	}
	// Exactly four files here are bad: two with malformed ticket names
	// and two non-md files; each must produce exactly one warning.
	if len(warns) != 4 {
		t.Fatalf("expected warnings for the 4 bad files, got %d: %+v", len(warns), warns)
	}
	names := make(map[string]bool)
	for _, w := range warns {
		names[w.Name] = true
	}
	for _, expect := range []string{"T-0002-OPEN.md", "T-NOTNUM-open.md", "T-0001-open.txt"} {
		if !names[expect] {
			t.Errorf("expected warning for %s; got %+v", expect, warns)
		}
	}
	// README.md gets the errBadExtension warning, not nil.
	if !names["README.md"] {
		t.Errorf("expected warning for README.md; got %+v", warns)
	}
}

func TestScan_SortsByNumber(t *testing.T) {
	s, dir := newStore(t)
	makeFile(t, dir, "T-0003-done.md", "c")
	makeFile(t, dir, "T-0001-open.md", "a")
	makeFile(t, dir, "T-0007-closed.md", "g")
	makeFile(t, dir, "T-0002-wip.md", "b")
	entries, _, err := s.scan()
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	want := []int{1, 2, 3, 7}
	if len(entries) != len(want) {
		t.Fatalf("expected %d entries, got %d", len(want), len(entries))
	}
	got := make([]int, len(entries))
	for i, e := range entries {
		got[i] = e.Number
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("at %d: want %d got %d (full %v)", i, w, got[i], got)
		}
	}
}

func TestMaxNumber_Empty(t *testing.T) {
	s, _ := newStore(t)
	if got := s.maxNumberLocked(); got != 0 {
		t.Errorf("empty dir: want 0, got %d", got)
	}
}

func TestMaxNumber_WithFiles(t *testing.T) {
	s, dir := newStore(t)
	makeFile(t, dir, "T-0001-open.md", "")
	makeFile(t, dir, "T-0002-wip.md", "")
	makeFile(t, dir, "T-0007-closed.md", "")
	makeFile(t, dir, "T-0003-done.md", "")
	if got := s.maxNumberLocked(); got != 7 {
		t.Errorf("max: want 7, got %d", got)
	}
}

func TestMaxNumber_IgnoresBadNames(t *testing.T) {
	s, dir := newStore(t)
	makeFile(t, dir, "T-0001-open.md", "")
	makeFile(t, dir, "T-NOTNUM-open.md", "")
	makeFile(t, dir, "T-99999-open.md", "out of range — 5 digits")
	if got := s.maxNumberLocked(); got != 1 {
		t.Errorf("max: want 1, got %d (bad names must not contribute)", got)
	}
}

func TestParseFilename_Roundtrip(t *testing.T) {
	// Smoke: ensure domain.ParseFilename round-trips for valid names.
	// (Plan §2 says reject 5+ digits; store never constructs those.)
	for _, st := range []domain.Status{domain.StatusOpen, domain.StatusWip, domain.StatusDone, domain.StatusClosed} {
		name := domain.Filename(42, st)
		n, gotSt, err := domain.ParseFilename(name)
		if err != nil {
			t.Fatalf("ParseFilename(%q): %v", name, err)
		}
		if n != 42 || gotSt != st {
			t.Errorf("round-trip %q: want (42, %s), got (%d, %s)", name, st, n, gotSt)
		}
	}
}

// TestScan_OrderedDirectory verifies that even if underlying entries come
// back in arbitrary order, scan re-sorts by number.
func TestScan_OrderedByNumberEvenWithUnsortedInput(t *testing.T) {
	s, dir := newStore(t)
	// os.ReadDir returns sorted entries already, but we don't trust that
	// — assert the post-condition regardless.
	files := []string{
		"T-0010-done.md",
		"T-0001-open.md",
		"T-0005-wip.md",
		"T-0003-closed.md",
	}
	for _, f := range files {
		makeFile(t, dir, f, "")
	}
	entries, _, err := s.scan()
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !sort.SliceIsSorted(entries, func(i, j int) bool { return entries[i].Number < entries[j].Number }) {
		t.Errorf("entries not sorted: %+v", entries)
	}
}

// TestScan_UnreadableDirectoryReturnsErr covers the rare path where the
// directory itself cannot be listed. Callers (List) wrap this in a warning
// rather than failing hard.
func TestScan_UnreadableDirectoryReturnsErr(t *testing.T) {
	// Create a Store with a valid handle, then remove the dir underneath.
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	_, _, err = s.scan()
	if err == nil {
		t.Fatal("expected error from scan on missing dir")
	}
}
