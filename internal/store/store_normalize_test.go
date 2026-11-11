package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ticket/internal/domain"
)

// Helpers for the normalize tests (T-0125). A canonical ticket is produced by
// Create, so it is by definition a Render fixed point; the legacy variant is
// derived from it mechanically (delint), never hand-written, so the two differ
// in exactly the markdownlint blank lines normalize has to restore.

// normalizeTicket builds an in-memory ticket with every metadata field set.
// It is deliberately not fakeTicket: that fixture leaves Created zero, which
// the normalize guard reports as incomplete metadata.
func normalizeTicket(project string) *domain.Ticket {
	return &domain.Ticket{
		Lang:     domain.LangRU,
		Type:     domain.TypeENH,
		Priority: domain.PriorityNormal,
		Title:    "normalize fixture",
		Details:  "normalize fixture details",
		Who:      "tester",
		Project:  project,
		Status:   domain.StatusOpen,
		Created:  time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC),
	}
}

// canonicalNormalize writes one ticket through Create and returns its path and
// bytes.
func canonicalNormalize(t *testing.T, s *Store, dir string) (string, []byte) {
	t.Helper()
	if _, err := s.Create(normalizeTicket("tickets")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	return filepath.Join(dir, "T-0001-open.md"), mustRead(t, dir, 1)
}

// delint turns a canonical body into the pre-fix (legacy) shape: the blank
// line Render emits before and after every "## " header is dropped. That is
// the whole difference the MD022 fix introduced, and it is reversible by
// Render.
func delint(body []byte) []byte {
	lines := strings.Split(string(body), "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if line == "" && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "## ") {
			continue // blank before a header: the MD022 fix added it
		}
		out = append(out, line)
		if strings.HasPrefix(line, "## ") && i+1 < len(lines) && lines[i+1] == "" {
			i++ // blank after a header: same fix
		}
	}
	return []byte(strings.Join(out, "\n"))
}

// writeTicketFileBytes writes body as dir/name and returns the path.
func writeTicketFileBytes(t *testing.T, dir, name string, body []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

// wantChanged asserts the exact changed-path list of a Normalize run.
func wantChanged(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("Changed = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Changed[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

// TestNormalize_CanonicalNoOp pins the primary contract: a file Create just
// wrote is already canonical, so nothing is rewritten and nothing is warned.
func TestNormalize_CanonicalNoOp(t *testing.T) {
	s, dir := newStore(t)
	_, body := canonicalNormalize(t, s, dir)

	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if len(res.Changed) != 0 {
		t.Errorf("Changed = %v, want none", res.Changed)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none", res.Warnings)
	}
	if got := mustRead(t, dir, 1); !bytes.Equal(got, body) {
		t.Errorf("body changed on a no-op run")
	}
}

// TestNormalize_NoWriteWithoutDiff pins the mtime guarantee: a no-op run must
// not touch the file at all (no tmp, no rename, no chmod).
func TestNormalize_NoWriteWithoutDiff(t *testing.T) {
	s, dir := newStore(t)
	path, _ := canonicalNormalize(t, s, dir)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.Normalize(NormalizeOptions{}); err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("mtime changed: %v -> %v", before.ModTime(), after.ModTime())
	}
	if after.Mode() != before.Mode() {
		t.Errorf("mode changed: %v -> %v", before.Mode(), after.Mode())
	}
}

// TestNormalize_LegacyRewrittenKeepsNameAndJournal pins the migration itself:
// the legacy blank-line shape is replaced by the canonical bytes, the file
// keeps its name and status, the journal gains no line and no tmp file is
// left behind.
func TestNormalize_LegacyRewrittenKeepsNameAndJournal(t *testing.T) {
	s, dir := newStore(t)
	_, canonical := canonicalNormalize(t, s, dir)
	legacy := delint(canonical)
	if bytes.Equal(legacy, canonical) {
		t.Fatal("fixture is not legacy-shaped: delint changed nothing")
	}
	path := writeTicketFileBytes(t, dir, "T-0001-open.md", legacy)

	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	wantChanged(t, res.Changed, path)
	if len(res.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none", res.Warnings)
	}
	got := mustRead(t, dir, 1)
	if !bytes.Equal(got, canonical) {
		t.Errorf("body not canonicalized:\n--- got ---\n%s\n--- want ---\n%s", got, canonical)
	}
	if n := strings.Count(string(got), " — тикет создан "); n != 1 {
		t.Errorf("journal creation entries = %d, want 1 (no line may be appended)", n)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("tmp file left behind: %s", e.Name())
		}
		if strings.HasSuffix(e.Name(), ".md") && e.Name() != "T-0001-open.md" {
			t.Errorf("unexpected ticket file %s: name or status changed", e.Name())
		}
	}
}

// TestNormalize_DryRunWritesNothing pins the preview contract: --dry-run
// reports the path that would change and leaves both bytes and mtime alone.
func TestNormalize_DryRunWritesNothing(t *testing.T) {
	s, dir := newStore(t)
	_, canonical := canonicalNormalize(t, s, dir)
	legacy := delint(canonical)
	path := writeTicketFileBytes(t, dir, "T-0001-open.md", legacy)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	res, err := s.Normalize(NormalizeOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	wantChanged(t, res.Changed, path)
	if got := mustRead(t, dir, 1); !bytes.Equal(got, legacy) {
		t.Errorf("dry run rewrote the file")
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("dry run changed mtime")
	}
}

// TestNormalize_CRLFFileConvertedOnceStable pins the line-ending guarantee: a
// CRLF legacy file is converted once and the next run is a no-op.
func TestNormalize_CRLFFileConvertedOnceStable(t *testing.T) {
	s, dir := newStore(t)
	_, canonical := canonicalNormalize(t, s, dir)
	crlf := bytes.ReplaceAll(delint(canonical), []byte("\n"), []byte("\r\n"))
	writeTicketFileBytes(t, dir, "T-0001-open.md", crlf)

	if _, err := s.Normalize(NormalizeOptions{}); err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	got := mustRead(t, dir, 1)
	if bytes.Contains(got, []byte("\r")) {
		t.Errorf("CR survived the rewrite")
	}
	if !bytes.Equal(got, canonical) {
		t.Errorf("CRLF file not canonicalized")
	}
	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("second Normalize: %v", err)
	}
	if len(res.Changed) != 0 {
		t.Errorf("second run changed %v, want none", res.Changed)
	}
}

// TestNormalize_SecondRunNoOpAfterRewrite pins idempotence: the canonical
// output of a rewrite is itself a fixed point.
func TestNormalize_SecondRunNoOpAfterRewrite(t *testing.T) {
	s, dir := newStore(t)
	_, canonical := canonicalNormalize(t, s, dir)
	writeTicketFileBytes(t, dir, "T-0001-open.md", delint(canonical))

	if _, err := s.Normalize(NormalizeOptions{}); err != nil {
		t.Fatalf("first Normalize: %v", err)
	}
	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("second Normalize: %v", err)
	}
	if len(res.Changed) != 0 {
		t.Errorf("second run changed %v, want none", res.Changed)
	}
}

// TestNormalize_RenameFailureLeavesFileIntact pins the atomicity contract: a
// failing commit returns the error, keeps the original bytes and removes the
// tmp file.
func TestNormalize_RenameFailureLeavesFileIntact(t *testing.T) {
	s, dir := newStore(t)
	_, canonical := canonicalNormalize(t, s, dir)
	legacy := delint(canonical)
	writeTicketFileBytes(t, dir, "T-0001-open.md", legacy)
	prev := renameFile
	renameFile = func(string, string) error { return errors.New("rename boom") }
	t.Cleanup(func() { renameFile = prev })

	if _, err := s.Normalize(NormalizeOptions{}); err == nil {
		t.Fatal("Normalize = nil error, want the rename failure")
	}
	if got := mustRead(t, dir, 1); !bytes.Equal(got, legacy) {
		t.Errorf("original bytes lost on a failed commit")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("tmp file left behind: %s", e.Name())
		}
	}
}

// TestNormalize_EmptyTicketsDir pins the empty-store contract: no tickets is
// a successful no-op, not an error.
func TestNormalize_EmptyTicketsDir(t *testing.T) {
	s, _ := newStore(t)
	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if len(res.Changed) != 0 || len(res.Warnings) != 0 {
		t.Errorf("Changed = %v, Warnings = %v, want both empty", res.Changed, res.Warnings)
	}
}
