package store

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// Scope and traversal tests for Normalize (T-0125): which files are touched
// (project filter, archive), which are skipped with a warning, and in which
// order the results arrive.

// renderTicket returns the canonical bytes of tk — the exact fixed point
// Normalize compares a file against.
func renderTicket(t *testing.T, tk *domain.Ticket) []byte {
	t.Helper()
	body, err := domain.Render(tk, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return body
}

// legacyFixture writes a legacy-shaped ticket for tk under dir and returns its
// path.
func legacyFixture(t *testing.T, dir string, tk *domain.Ticket, status domain.Status) string {
	t.Helper()
	// The file name carries the status (V19), and the body must agree with it:
	// a mismatch is a semantic repair, not the formatting migration under test.
	tk.Status = status
	name := domain.Filename(tk.Number, status)
	return writeTicketFileBytes(t, dir, name, delint(renderTicket(t, tk)))
}

// TestNormalize_ProjectFilter pins -P: only tickets whose own Project matches
// exactly are rewritten, and a case-different name does not match.
func TestNormalize_ProjectFilter(t *testing.T) {
	s, dir := newStore(t)
	alpha := normalizeTicket("alpha")
	alpha.Number = 1
	beta := normalizeTicket("beta")
	beta.Number = 2
	alphaPath := legacyFixture(t, dir, alpha, domain.StatusOpen)
	legacyFixture(t, dir, beta, domain.StatusOpen)

	res, err := s.Normalize(NormalizeOptions{ProjectFilter: "alpha"})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	wantChanged(t, res.Changed, alphaPath)
	betaRaw, err := os.ReadFile(filepath.Join(dir, "T-0002-open.md"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(betaRaw, renderTicket(t, beta)) {
		t.Errorf("filtered-out ticket was rewritten")
	}
}

// TestNormalize_ProjectFilterAppliesToArchive pins that -P filters the archive
// pass too, not only the active directory.
func TestNormalize_ProjectFilterAppliesToArchive(t *testing.T) {
	s, dir := newStore(t)
	archiveDir := filepath.Join(dir, "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	alpha := normalizeTicket("alpha")
	alpha.Number = 3
	beta := normalizeTicket("beta")
	beta.Number = 4
	legacyFixture(t, archiveDir, alpha, domain.StatusDone)
	legacyFixture(t, archiveDir, beta, domain.StatusDone)

	res, err := s.Normalize(NormalizeOptions{ProjectFilter: "alpha"})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	wantChanged(t, res.Changed, filepath.Join(archiveDir, "T-0003-done.md"))
}

// TestNormalize_ArchiveInPlace pins that an archived ticket is rewritten where
// it lies: same directory, same file name, never moved back into work.
func TestNormalize_ArchiveInPlace(t *testing.T) {
	s, dir := newStore(t)
	archiveDir := filepath.Join(dir, "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tk := normalizeTicket("tickets")
	tk.Number = 7
	path := legacyFixture(t, archiveDir, tk, domain.StatusClosed)

	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	wantChanged(t, res.Changed, path)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("archived ticket gone: %v", err)
	}
	if !bytes.Equal(got, renderTicket(t, tk)) {
		t.Errorf("archived ticket not canonicalized in place")
	}
}

// TestNormalize_SkipsBrokenFiles pins the never-fatal read contract: a broken
// H1, an empty file and a non-.md stray are warnings, the good ticket is still
// normalized and Normalize returns no error.
func TestNormalize_SkipsBrokenFiles(t *testing.T) {
	s, dir := newStore(t)
	tk := normalizeTicket("tickets")
	tk.Number = 1
	good := legacyFixture(t, dir, tk, domain.StatusOpen)

	writeTicketFileBytes(t, dir, "T-0002-open.md", []byte("# T-0009 · ENH: wrong number\n"))
	writeTicketFileBytes(t, dir, "T-0003-open.md", nil)
	writeTicketFileBytes(t, dir, "notes.txt", []byte("stray"))

	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	wantChanged(t, res.Changed, good)
	if len(res.Warnings) < 3 {
		t.Errorf("Warnings = %v, want at least the three broken entries", res.Warnings)
	}
	reasons := make(map[string]bool, len(res.Warnings))
	for _, w := range res.Warnings {
		reasons[w.Reason] = true
	}
	for _, want := range []string{reasonH1Mismatch, reasonEmptyFile, reasonBadExtension} {
		if !reasons[want] {
			t.Errorf("missing warning reason %q in %v", want, res.Warnings)
		}
	}
}

// TestNormalize_UnknownTailPreserved pins that the manual tail after the
// journal survives byte for byte and stays a fixed point.
func TestNormalize_UnknownTailPreserved(t *testing.T) {
	s, dir := newStore(t)
	tk := normalizeTicket("tickets")
	tk.Number = 1
	const tail = "<!-- analyst note: keep me -->\n"
	legacy := append(delint(renderTicket(t, tk)), []byte("\n"+tail)...)
	path := writeTicketFileBytes(t, dir, "T-0001-open.md", legacy)

	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	wantChanged(t, res.Changed, path)
	got := mustRead(t, dir, 1)
	if !strings.Contains(string(got), tail) {
		t.Errorf("unknown tail lost:\n%s", got)
	}
	again, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("second Normalize: %v", err)
	}
	if len(again.Changed) != 0 {
		t.Errorf("second run changed %v, want none", again.Changed)
	}
}

// TestNormalize_OrderActiveThenArchive pins the traversal order: the active
// directory is fully processed before the archive, so a mid-run failure leaves
// the archive untouched.
func TestNormalize_OrderActiveThenArchive(t *testing.T) {
	s, dir := newStore(t)
	archiveDir := filepath.Join(dir, "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	active := normalizeTicket("tickets")
	active.Number = 1
	archived := normalizeTicket("tickets")
	archived.Number = 2
	activePath := legacyFixture(t, dir, active, domain.StatusOpen)
	archivePath := legacyFixture(t, archiveDir, archived, domain.StatusDone)

	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	wantChanged(t, res.Changed, activePath, archivePath)
}

// TestNormalize_OrderByNumberName pins numeric ordering: T-0010 sorts after
// T-0002 because the number is compared as a number, not as a name.
func TestNormalize_OrderByNumberName(t *testing.T) {
	s, dir := newStore(t)
	ten := normalizeTicket("tickets")
	ten.Number = 10
	two := normalizeTicket("tickets")
	two.Number = 2
	// Ten is written first so a directory-order or name-order implementation
	// would report it first.
	tenPath := legacyFixture(t, dir, ten, domain.StatusOpen)
	twoPath := legacyFixture(t, dir, two, domain.StatusOpen)

	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	wantChanged(t, res.Changed, twoPath, tenPath)
}

// TestNormalize_DuplicateNumbersWarnExit0 pins that one number owned by two
// files is not fatal here: both are rewritten independently and Normalize
// returns no error (the CLI prints the ambiguity warning and still exits 0).
//
// The order is compared as a SET: scanDir sorts only by Number with the
// unstable sort.Slice, so two files sharing a number keep directory order and
// a fixed sequence would pin an accident of readdir.
func TestNormalize_DuplicateNumbersWarnExit0(t *testing.T) {
	s, dir := newStore(t)
	tk := normalizeTicket("tickets")
	tk.Number = 1
	openPath := legacyFixture(t, dir, tk, domain.StatusOpen)
	wipPath := legacyFixture(t, dir, tk, domain.StatusWip)

	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	got := slices.Clone(res.Changed)
	slices.Sort(got)
	want := []string{openPath, wipPath}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("Changed = %v, want the set %v", res.Changed, want)
	}
}

// TestNormalize_InvalidArchiveDirWarnsAndContinues pins the T-0078 containment:
// an archive path that is not a real directory is one warning (archive-dir),
// and the active directory is still normalized.
func TestNormalize_InvalidArchiveDirWarnsAndContinues(t *testing.T) {
	s, dir := newStore(t)
	tk := normalizeTicket("tickets")
	tk.Number = 1
	activePath := legacyFixture(t, dir, tk, domain.StatusOpen)
	if err := os.WriteFile(filepath.Join(dir, "archive"), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	wantChanged(t, res.Changed, activePath)
	// The regular file named archive is also a stray non-.md entry for the
	// active pass, so the archive warning is not necessarily the only one.
	found := false
	for _, w := range res.Warnings {
		if w.Reason == reasonArchiveDir {
			found = true
		}
	}
	if !found {
		t.Errorf("Warnings = %v, want one with reason %q", res.Warnings, reasonArchiveDir)
	}
}
