package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ticket/internal/domain"
)

// assertLastJournalTransition asserts the LAST transition line in body
// byte-equals "- <ts> — статус: <from> → <to>[ · <comment>] (<who>)" —
// delete bodies carry the ticket's earlier transitions too.
func assertLastJournalTransition(t *testing.T, body, from, to, comment, who string) {
	t.Helper()
	const marker = " — статус: "
	var last string
	for line := range strings.SplitSeq(body, "\n") {
		if strings.HasPrefix(line, "- ") && strings.Contains(line, marker) {
			last = line
		}
	}
	if last == "" {
		t.Fatalf("no journal transition line in body:\n%s", body)
	}
	rest := last[2:]
	if i := strings.Index(rest, marker); i != len(tsLayout) {
		t.Fatalf("timestamp must be %d chars: %q", len(tsLayout), last)
	}
	ts := rest[:len(tsLayout)]
	if _, err := time.Parse(tsLayout, ts); err != nil {
		t.Fatalf("timestamp %q does not parse: %v", ts, err)
	}
	want := fmt.Sprintf("- %s — статус: %s → %s", ts, from, to)
	if comment != "" {
		want += " · " + comment
	}
	want += " (" + who + ")"
	if last != want {
		t.Fatalf("journal line byte mismatch\n got: %q\nwant: %q", last, want)
	}
}

// TestDelete_OpenMainHappyPath pins the happy path: Delete returns
// the real deleted path, creates deleted/ when absent, removes the
// main file and appends exactly one open→deleted transition.
func TestDelete_OpenMainHappyPath(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	target, err := s.Delete("1", false, "tester")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	want := filepath.Join(dir, "deleted", "T-0001-deleted.md")
	if target != want {
		t.Fatalf("target = %q, want %q", target, want)
	}
	if _, err := os.Lstat(filepath.Join(dir, "T-0001-open.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("main file must be gone, Lstat err=%v", err)
	}
	body := string(mustRead(t, filepath.Join(dir, "deleted"), 1))
	if !strings.Contains(body, "- Status (Статус): deleted") {
		t.Fatalf("status line must be deleted:\n%s", body)
	}
	if got := strings.Count(body, "— статус: open → deleted"); got != 1 {
		t.Fatalf("want exactly 1 delete transition, got %d", got)
	}
	if !strings.Contains(body, "— тикет создан") {
		t.Fatal("creation journal line lost")
	}
	assertJournalTransition(t, body, "open", "deleted", "", "tester")
	assertNoTmpFiles(t, dir)
	assertNoTmpFiles(t, filepath.Join(dir, "deleted"))
}

// TestDelete_ForceStatusMatrix pins the force guard: without force
// every non-open main status is *NotOpenError (nothing written);
// with force each transitions old→deleted.
func TestDelete_ForceStatusMatrix(t *testing.T) {
	for _, st := range []domain.Status{domain.StatusWip, domain.StatusDone, domain.StatusClosed} {
		t.Run(string(st), func(t *testing.T) {
			s, dir := newStore(t)
			if _, err := s.Create(fakeTicket(0)); err != nil {
				t.Fatalf("Create: %v", err)
			}
			if _, _, err := s.SetStatus(1, st, "tester", ""); err != nil {
				t.Fatalf("SetStatus(%s): %v", st, err)
			}
			before := mustRead(t, dir, 1)
			_, err := s.Delete("1", false, "tester")
			var notOpen *NotOpenError
			if !errors.As(err, &notOpen) || notOpen.Status != st {
				t.Fatalf("no-force err = %v, want *NotOpenError{%s}", err, st)
			}
			if after := mustRead(t, dir, 1); !bytes.Equal(before, after) {
				t.Fatal("refused delete changed the source")
			}
			target, err := s.Delete("1", true, "eraser")
			if err != nil {
				t.Fatalf("force Delete: %v", err)
			}
			if want := filepath.Join(dir, "deleted", "T-0001-deleted.md"); target != want {
				t.Fatalf("target = %q, want %q", target, want)
			}
			body := string(mustRead(t, filepath.Join(dir, "deleted"), 1))
			assertLastJournalTransition(t, body, string(st), "deleted", "", "eraser")
		})
	}
}

// TestDelete_ArchiveRequiresForce pins that an archived ticket is
// refused without force and moved from archive/ with force.
func TestDelete_ArchiveRequiresForce(t *testing.T) {
	s, dir := makeDone(t)
	if _, err := s.Archive(1, "tester"); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	archiveFile := filepath.Join(dir, "archive", "T-0001-done.md")
	before, err := os.ReadFile(archiveFile)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Delete("1", false, "tester")
	var notOpen *NotOpenError
	if !errors.As(err, &notOpen) || notOpen.Status != domain.StatusDone {
		t.Fatalf("no-force err = %v, want *NotOpenError{done}", err)
	}
	after, rerr := os.ReadFile(archiveFile)
	if rerr != nil {
		t.Fatalf("re-read archived source: %v", rerr)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("refused delete changed the archived source")
	}
	target, err := s.Delete("1", true, "eraser")
	if err != nil {
		t.Fatalf("force Delete: %v", err)
	}
	if want := filepath.Join(dir, "deleted", "T-0001-deleted.md"); target != want {
		t.Fatalf("target = %q, want %q", target, want)
	}
	if _, err := os.Lstat(archiveFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("archive file must be gone, Lstat err=%v", err)
	}
	body := string(mustRead(t, filepath.Join(dir, "deleted"), 1))
	assertLastJournalTransition(t, body, "done", "deleted", "", "eraser")
	if !strings.Contains(body, "— перенесён в архив") {
		t.Fatal("archive journal line lost")
	}
}

// TestDelete_FilenameSelectorDisambiguates pins that an exact base
// name selects one file of a duplicate-number pair, leaving the
// other untouched, while the bare number stays ambiguous.
func TestDelete_FilenameSelectorDisambiguates(t *testing.T) {
	s, dir := makeDone(t)
	if _, err := s.Archive(1, "tester"); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	makeFile(t, dir, "T-0001-open.md", strings.ReplaceAll(seededDoneBody, "T-0002", "T-0001"))
	if _, err := s.Delete("1", true, "tester"); !errors.Is(err, ErrAmbiguousNumber) {
		t.Fatalf("bare number err = %v, want ErrAmbiguousNumber", err)
	}
	target, err := s.Delete("T-0001-open.md", false, "tester")
	if err != nil {
		t.Fatalf("basename Delete: %v", err)
	}
	if want := filepath.Join(dir, "deleted", "T-0001-deleted.md"); target != want {
		t.Fatalf("target = %q, want %q", target, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "archive", "T-0001-done.md")); err != nil {
		t.Fatalf("archive twin must survive: %v", err)
	}
}

// TestDelete_FullPreservation pins that metadata, sections, Unknown,
// raw templates and the prior journal survive byte-exactly: only
// the status and one appended transition differ.
func TestDelete_FullPreservation(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, _, err := s.SetStatus(1, domain.StatusWip, "tester", "рабочая заметка"); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	// Manual unknown section after the journal (Unknown bytes).
	wipPath := filepath.Join(dir, "T-0001-wip.md")
	orig, err := os.ReadFile(wipPath)
	if err != nil {
		t.Fatal(err)
	}
	orig = append(orig, []byte("\n## Заметки\nручная секция\n")...)
	if err := os.WriteFile(wipPath, orig, 0o644); err != nil {
		t.Fatal(err)
	}
	before, unknownBefore, err := domain.Parse(orig)
	if err != nil {
		t.Fatalf("parse before: %v", err)
	}
	if _, err := s.Delete("1", true, "eraser"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	deletedRaw := mustRead(t, filepath.Join(dir, "deleted"), 1)
	after, unknownAfter, err := domain.Parse(deletedRaw)
	if err != nil {
		t.Fatalf("parse after: %v", err)
	}
	if after.Status != domain.StatusDeleted {
		t.Fatalf("status = %q, want deleted", after.Status)
	}
	if !bytes.Equal(unknownBefore, unknownAfter) {
		t.Fatalf("Unknown changed: %q vs %q", unknownBefore, unknownAfter)
	}
	if len(after.Journal) != len(before.Journal)+1 {
		t.Fatalf("journal len = %d, want %d", len(after.Journal), len(before.Journal)+1)
	}
	last := after.Journal[len(after.Journal)-1]
	if last.From != domain.StatusWip || last.To != domain.StatusDeleted || last.Who != "eraser" || last.Comment != "" {
		t.Fatalf("delete entry = %+v, want wip→deleted by eraser", last)
	}
	// Reverting status+journal must reproduce the original bytes —
	// pins RawTemplates/sections/metadata byte-stability.
	revert := *after
	revert.Status = before.Status
	revert.Journal = before.Journal
	rebuilt, err := domain.Render(&revert, unknownAfter)
	if err != nil {
		t.Fatalf("render revert: %v", err)
	}
	if !bytes.Equal(orig, rebuilt) {
		t.Fatalf("preservation breach:\norig:\n%s\nrebuilt:\n%s", orig, rebuilt)
	}
}
