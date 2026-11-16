package store

// T-0189 W3-C1a: Normalize walks tickets/deleted/ after the archive, in
// place, under the same contracts as the other two directories:
// filename-authoritative status, journal preserved, guard enforced,
// invalid/unreadable deleted/ is one warning and the walk continues.

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// deletedFixture writes a legacy-shaped deleted ticket under
// tickets/deleted/ and returns its path. The body agrees with the
// deleted name; a body-status mismatch is a different test's fixture.
func deletedFixture(t *testing.T, dir string, tk *domain.Ticket) (string, string) {
	t.Helper()
	deletedDir := filepath.Join(dir, "deleted")
	if err := os.MkdirAll(deletedDir, 0o755); err != nil {
		t.Fatalf("mkdir deleted: %v", err)
	}
	tk.Status = domain.StatusDeleted
	return deletedDir, writeTicketFileBytes(t, deletedDir, domain.Filename(tk.Number, domain.StatusDeleted), delint(renderTicket(t, tk)))
}

// TestNormalize_DeletedWalkOrder pins the W3 traversal order: main, then
// archive, then deleted — a mid-run failure leaves the not-yet-walked
// directories untouched.
func TestNormalize_DeletedWalkOrder(t *testing.T) {
	s, dir := newStore(t)
	archiveDir := filepath.Join(dir, "archive")
	if err := os.Mkdir(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	main := normalizeTicket("tickets")
	main.Number = 1
	archived := normalizeTicket("tickets")
	archived.Number = 2
	deletedTk := normalizeTicket("tickets")
	deletedTk.Number = 3
	mainPath := legacyFixture(t, dir, main, domain.StatusOpen)
	archivePath := legacyFixture(t, archiveDir, archived, domain.StatusDone)
	deletedDir, deletedPath := deletedFixture(t, dir, deletedTk)

	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	wantChanged(t, res.Changed, mainPath, archivePath, deletedPath)
	for _, p := range []string{mainPath, archivePath, deletedPath} {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("walk moved or renamed %s: %v", p, err)
		}
	}
	assertNoTmpFiles(t, deletedDir)
}

// TestNormalize_DeletedInPlaceRewrite pins the migration on a deleted
// ticket: the legacy blank-line shape becomes the canonical bytes where
// the file lies — the name keeps its deleted status, the journal gains
// no line and no tmp file is left behind in deleted/.
func TestNormalize_DeletedInPlaceRewrite(t *testing.T) {
	s, dir := newStore(t)
	tk := normalizeTicket("tickets")
	tk.Number = 1
	// A non-empty journal: the rewrite must not append a line to it.
	// The creation-entry shape (From==To==open) is what renders the
	// "тикет создан" line the assertion below counts.
	tk.Journal = []domain.JournalEntry{{
		At: tk.Created, Who: tk.Who, From: domain.StatusOpen, To: domain.StatusOpen,
	}}
	deletedDir, path := deletedFixture(t, dir, tk)

	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	wantChanged(t, res.Changed, path)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("deleted ticket gone: %v", err)
	}
	if !bytes.Equal(got, renderTicket(t, tk)) {
		t.Errorf("deleted ticket not canonicalized in place:\n--- got ---\n%s", got)
	}
	if n := strings.Count(string(got), " — тикет создан "); n != 1 {
		t.Errorf("journal creation entries = %d, want 1 (no line may be appended)", n)
	}
	// In place means in place: still under deleted/, still the deleted
	// name, nothing moved into the main directory.
	if filepath.Dir(path) != deletedDir {
		t.Errorf("deleted ticket moved out of deleted/")
	}
	if filepath.Base(path) != "T-0001-deleted.md" {
		t.Errorf("name = %q, want T-0001-deleted.md", filepath.Base(path))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			t.Errorf("deleted ticket leaked into the main directory: %s", e.Name())
		}
	}
	assertNoTmpFiles(t, deletedDir)
}

// TestNormalize_DeletedBodyStatusMismatchWarns pins the V19 notice on a
// deleted file: a body claiming another status is reported once and the
// name wins — the file is rewritten with the deleted status.
func TestNormalize_DeletedBodyStatusMismatchWarns(t *testing.T) {
	s, dir := newStore(t)
	tk := normalizeTicket("tickets")
	tk.Number = 1
	_, path := deletedFixture(t, dir, tk)
	// tamperBodyStatus hardcodes the -open name; a deleted file needs the
	// same hand-edit against its own name.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(data), "- Status (Статус): deleted", "- Status (Статус): open", 1)
	if tampered == string(data) {
		t.Fatal("status line not found in deleted body")
	}
	writeTicketFileBytes(t, filepath.Dir(path), filepath.Base(path), []byte(tampered))

	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	wantChanged(t, res.Changed, path)
	if len(res.Warnings) != 1 || res.Warnings[0].Reason != reasonStatusFromName {
		t.Fatalf("Warnings = %v, want exactly one %q", res.Warnings, reasonStatusFromName)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, renderTicket(t, tk)) {
		t.Errorf("deleted body not canonicalized to the name's status")
	}
}

// TestNormalize_DeletedGuardSkips pins that the deliberately conservative
// guard applies inside deleted/ too: a bash-era file is skipped with the
// unrecognized-meta warning and left byte-identical.
func TestNormalize_DeletedGuardSkips(t *testing.T) {
	s, dir := newStore(t)
	deletedDir := filepath.Join(dir, "deleted")
	if err := os.Mkdir(deletedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte(`# T-0001 · ENH: Bash era deleted

- Статус: deleted
- Приоритет: normal
- Создан: 2026-01-02 03:04 · by (кем): tester
- Проект: tickets

## Краткое описание

Bash era body.
`)
	path := writeTicketFileBytes(t, deletedDir, "T-0001-deleted.md", body)

	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if len(res.Changed) != 0 {
		t.Errorf("Changed = %v, want none (guard must skip)", res.Changed)
	}
	if len(res.Warnings) != 1 || res.Warnings[0].Reason != reasonUnrecognizedMeta {
		t.Fatalf("Warnings = %v, want exactly one %q", res.Warnings, reasonUnrecognizedMeta)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, body) {
		t.Errorf("skipped deleted file was rewritten")
	}
}

// TestNormalize_InvalidDeletedWarnsAndContinues pins the T-0189
// containment on the normalize walk: a deleted path that is not a real
// directory is ONE deleted-dir warning, the main and archive results
// stand and the walk still returns no error.
func TestNormalize_InvalidDeletedWarnsAndContinues(t *testing.T) {
	s, dir := newStore(t)
	tk := normalizeTicket("tickets")
	tk.Number = 1
	mainPath := legacyFixture(t, dir, tk, domain.StatusOpen)
	if err := os.WriteFile(filepath.Join(dir, "deleted"), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	wantChanged(t, res.Changed, mainPath)
	found := 0
	for _, w := range res.Warnings {
		if w.Reason == reasonDeletedDir {
			found++
			if !errors.Is(w.Err, ErrDeletedInvalid) {
				t.Errorf("warning err = %v, want ErrDeletedInvalid", w.Err)
			}
		}
	}
	if found != 1 {
		t.Errorf("deleted-dir warnings = %d, want exactly 1 (warnings: %v)", found, res.Warnings)
	}
}

// TestNormalize_UnreadableDeletedWarnsAndContinues pins the dir-read
// parity: an unreadable deleted/ is one dir-read warning, results from
// the directories already walked stand, and no error is returned.
func TestNormalize_UnreadableDeletedWarnsAndContinues(t *testing.T) {
	s, dir := newStore(t)
	tk := normalizeTicket("tickets")
	tk.Number = 1
	mainPath := legacyFixture(t, dir, tk, domain.StatusOpen)
	deletedTk := normalizeTicket("tickets")
	deletedTk.Number = 2
	deletedDir, _ := deletedFixture(t, dir, deletedTk)

	orig := readDir
	swapReadDir(t, func(d string) ([]os.DirEntry, error) {
		if d == deletedDir {
			return nil, dirReadPathError(deletedDir)
		}
		return orig(d)
	})

	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	wantChanged(t, res.Changed, mainPath) // deleted pass skipped, main result stands
	found := 0
	for _, w := range res.Warnings {
		if w.Reason == reasonDirRead {
			found++
		}
	}
	if found != 1 {
		t.Errorf("dir-read warnings = %d, want exactly 1 (warnings: %v)", found, res.Warnings)
	}
}

// TestNormalize_ProjectFilterAppliesToDeleted pins that -P filters the
// deleted pass exactly like the other two directories: only the matching
// project is rewritten.
func TestNormalize_ProjectFilterAppliesToDeleted(t *testing.T) {
	s, dir := newStore(t)
	alpha := normalizeTicket("alpha")
	alpha.Number = 1
	beta := normalizeTicket("beta")
	beta.Number = 2
	_, alphaPath := deletedFixture(t, dir, alpha)
	_, betaPath := deletedFixture(t, dir, beta)

	res, err := s.Normalize(NormalizeOptions{ProjectFilter: "alpha"})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	wantChanged(t, res.Changed, alphaPath)
	betaRaw, err := os.ReadFile(betaPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(betaRaw, renderTicket(t, beta)) {
		t.Errorf("filtered-out deleted ticket was rewritten")
	}
}
