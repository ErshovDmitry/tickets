package store

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// Guard and file-mode tests for Normalize (T-0125): files whose bytes Render
// cannot reproduce are skipped with a warning, and a rewrite keeps the
// original permission bits (POSIX) or the read-only attribute (Windows).

// skipWindowsPOSIXMode skips tests whose premise is POSIX permission bits:
// Windows does not store them. os.Chmod there only toggles the read-only
// attribute and os.Stat reports 0444/0666 for every file, so there is no mode
// to preserve or to lose.
func skipWindowsPOSIXMode(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not store POSIX permission bits: os.Chmod only toggles the read-only attribute")
	}
}

// skipRootNormalize skips chmod-dependent tests where root bypasses the
// permission checks the premise relies on. os.Geteuid is -1 on Windows, so it
// never fires there.
func skipRootNormalize(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permission checks")
	}
}

// TestNormalize_BashEraLabelsSkipped pins the lossy-render guard: the bash-era
// label form ("- Статус: open") is not part of the dictionary grammar, so
// Render would drop the line. The file must be left byte-identical and
// reported with the unrecognized-meta reason.
func TestNormalize_BashEraLabelsSkipped(t *testing.T) {
	s, dir := newStore(t)
	body := []byte("# T-0001 · ENH: Bash era\n\n" +
		"- Статус: open\n- Приоритет: normal\n" +
		"- Создан: 2026-01-02 03:04 · by (кем): tester\n- Проект: tickets\n\n" +
		"## Краткое описание\n\nBash era body.\n")
	writeTicketFileBytes(t, dir, "T-0001-open.md", body)

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
	if got := mustRead(t, dir, 1); !bytes.Equal(got, body) {
		t.Errorf("skipped file was rewritten")
	}
}

// TestNormalize_MixedLabelsSkipped pins the other half of the guard: a file
// with all four dictionary meta lines PLUS a bash-era label line is still
// skipped, because the unmatched line itself would be lost.
func TestNormalize_MixedLabelsSkipped(t *testing.T) {
	s, dir := newStore(t)
	tk := normalizeTicket("tickets")
	tk.Number = 1
	legacy := delint(renderTicket(t, tk))
	mixed := bytes.Replace(legacy, []byte("- Project (Проект): tickets\n"),
		[]byte("- Project (Проект): tickets\n- Статус: open\n"), 1)
	if bytes.Equal(mixed, legacy) {
		t.Fatal("fixture not built: the project line was not found")
	}
	writeTicketFileBytes(t, dir, "T-0001-open.md", mixed)

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
	if got := mustRead(t, dir, 1); !bytes.Equal(got, mixed) {
		t.Errorf("mixed-label file was rewritten")
	}
}

// withLabelAfterProject inserts label as a standalone line right after the
// project metadata line of a freshly rendered ticket body, so the fixture
// carries a valid ticket plus one line Render cannot reproduce.
func withLabelAfterProject(t *testing.T, body []byte, label string) []byte {
	t.Helper()
	lines := strings.Split(string(body), "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "- Project") {
			continue
		}
		out := make([]string, 0, len(lines)+1)
		out = append(out, lines[:i+1]...)
		out = append(out, label)
		return []byte(strings.Join(append(out, lines[i+1:]...), "\n"))
	}
	t.Fatal("fixture has no project line")
	return nil
}

// TestNormalize_LegacyLabelVariantsSkipped pins the guard's width: an
// indented marker, a "*" bullet and a space before the colon are still
// bash-era label lines that Render cannot reproduce, so each file must be
// left byte-identical and reported as unrecognized-meta (a rewrite would
// silently drop the line).
func TestNormalize_LegacyLabelVariantsSkipped(t *testing.T) {
	for _, tc := range []struct{ name, line string }{
		{"leading-space", " - Статус: open"},
		{"star-bullet", "* Статус: open"},
		{"space-before-colon", "- Статус : open"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, dir := newStore(t)
			_, body := canonicalNormalize(t, s, dir)
			fixture := withLabelAfterProject(t, body, tc.line)
			writeTicketFileBytes(t, dir, "T-0001-open.md", fixture)

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
			if got := mustRead(t, dir, 1); !bytes.Equal(got, fixture) {
				t.Errorf("guarded file was rewritten")
			}
		})
	}
}

// TestNormalize_ForeignLabelsNotTranslated pins label fidelity: the language is
// decided by the FIRST dictionary signal, so a single foreign-dictionary meta
// line inside an RU ticket is re-emitted from its raw segment, never translated
// into the surrounding language.
func TestNormalize_ForeignLabelsNotTranslated(t *testing.T) {
	s, dir := newStore(t)
	tk := normalizeTicket("tickets")
	tk.Number = 1
	legacy := delint(renderTicket(t, tk))
	foreign := bytes.Replace(legacy, []byte("- Priority (Приоритет): normal"),
		[]byte("- Priority: normal"), 1)
	if bytes.Equal(foreign, legacy) {
		t.Fatal("fixture not built: the RU priority line was not found")
	}
	writeTicketFileBytes(t, dir, "T-0001-open.md", foreign)

	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	wantChanged(t, res.Changed, filepath.Join(dir, "T-0001-open.md"))
	got := string(mustRead(t, dir, 1))
	if !strings.Contains(got, "- Priority: normal") {
		t.Errorf("foreign label was translated:\n%s", got)
	}
	if strings.Contains(got, "- Priority (Приоритет): normal") {
		t.Errorf("foreign label was rewritten into the RU form:\n%s", got)
	}
}

// TestNormalize_PreservesFileMode pins that the rewrite keeps the original
// permission bits instead of the 0644 the tmp file is created with. POSIX
// only: the read-only attribute case (what Windows can express at all) is
// TestNormalize_ReadOnlySourcePreserved.
func TestNormalize_PreservesFileMode(t *testing.T) {
	skipWindowsPOSIXMode(t)
	s, dir := newStore(t)
	tk := normalizeTicket("tickets")
	tk.Number = 1
	path := legacyFixture(t, dir, tk, domain.StatusOpen)
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Normalize(NormalizeOptions{}); err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o640 {
		t.Errorf("mode = %o, want 640", got)
	}
}

// TestNormalize_ReadOnlySourcePreserved pins the read-only source case: a
// read-only ticket is rewritten (the rename only needs a writable directory)
// and comes out read-only again. On POSIX that is the 0444 permission bits; on
// Windows it is FILE_ATTRIBUTE_READONLY, which os.Chmod sets and os.Stat
// reports as 0444, and which prepareReplace clears before the rename and
// restores after it.
func TestNormalize_ReadOnlySourcePreserved(t *testing.T) {
	skipRootNormalize(t)
	s, dir := newStore(t)
	tk := normalizeTicket("tickets")
	tk.Number = 1
	// A non-empty journal: the rewrite must not append a line to it.
	tk.Journal = []domain.JournalEntry{{
		At: tk.Created, Who: tk.Who, From: domain.StatusOpen, To: domain.StatusOpen,
	}}
	path := legacyFixture(t, dir, tk, domain.StatusOpen)
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cerr := os.Chmod(path, 0o644); cerr != nil {
			t.Errorf("restore mode: %v", cerr)
		}
	})

	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	wantChanged(t, res.Changed, path)
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o444 {
		t.Errorf("mode = %o, want 444", got)
	}
	got := mustRead(t, dir, 1)
	if !bytes.Equal(got, renderTicket(t, tk)) {
		t.Errorf("read-only ticket not canonicalized")
	}
	if n := strings.Count(string(got), " — тикет создан "); n != 1 {
		t.Errorf("journal creation entries = %d, want 1 (no line may be appended)", n)
	}
}

// TestNormalize_TmpOpenFailure pins the write-failure contract: when the tmp
// file cannot be created (a read-only archive directory, the store itself is
// writable so the lock succeeds), Normalize returns the error and the ticket
// stays byte-identical. The premise is POSIX-only: on Windows os.Chmod sets
// the read-only attribute but a 0555 directory still accepts new files, so the
// tmp file would be created and the write would succeed.
func TestNormalize_TmpOpenFailure(t *testing.T) {
	skipRootNormalize(t)
	if runtime.GOOS == "windows" {
		t.Skip("a 0555 directory is not write-protected on Windows: the tmp file can still be created")
	}
	s, dir := newStore(t)
	archiveDir := filepath.Join(dir, "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tk := normalizeTicket("tickets")
	tk.Number = 1
	path := legacyFixture(t, archiveDir, tk, domain.StatusDone)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(archiveDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cerr := os.Chmod(archiveDir, 0o755); cerr != nil {
			t.Errorf("restore archive mode: %v", cerr)
		}
	})

	if _, err := s.Normalize(NormalizeOptions{}); err == nil {
		t.Fatal("Normalize = nil error, want the tmp open failure")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("ticket changed despite the write failure")
	}
}
