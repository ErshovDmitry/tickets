package store

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"ticket/internal/domain"
)

// assertJournalArchived finds the archive journal line in body and asserts
// it byte-equals the bash shape "- <ts> — перенесён в архив (<who>)" with
// no trailing dot (bash cmd_archive printf, tickets/bin/ticket:177).
func assertJournalArchived(t *testing.T, body, who string) {
	t.Helper()
	const marker = " — перенесён в архив ("
	for _, line := range strings.Split(body, "\n") {
		if !strings.Contains(line, marker) {
			continue
		}
		if !strings.HasPrefix(line, "- ") {
			t.Fatalf("journal line must start with \"- \": %q", line)
		}
		rest := line[2:]
		if i := strings.Index(rest, marker); i != len(tsLayout) {
			t.Fatalf("timestamp must be %d chars: %q", len(tsLayout), line)
		}
		ts := rest[:len(tsLayout)]
		if _, err := time.Parse(tsLayout, ts); err != nil {
			t.Fatalf("timestamp %q does not parse as %q: %v", ts, tsLayout, err)
		}
		want := "- " + ts + marker + who + ")"
		if line != want {
			t.Fatalf("journal line byte mismatch\n got: %q\nwant: %q", line, want)
		}
		return
	}
	t.Fatalf("no archive journal line in body:\n%s", body)
}

// makeDone seeds a store with ticket 1 already transitioned to done.
func makeDone(t *testing.T) (*Store, string) {
	t.Helper()
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := s.SetStatus(1, domain.StatusDone, "tester", ""); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	return s, dir
}

// TestArchive_MovesDoneTicket pins the happy path: Archive returns the
// real archive path (bash echo "$target"), removes the main file and
// appends the byte-exact archive journal line.
func TestArchive_MovesDoneTicket(t *testing.T) {
	s, dir := makeDone(t)
	target, err := s.Archive(1, "tester")
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	want := filepath.Join(dir, "archive", "T-0001-done.md")
	if target != want {
		t.Fatalf("target = %q, want %q", target, want)
	}
	if _, err := os.Lstat(filepath.Join(dir, "T-0001-done.md")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("main file must be gone, Lstat err=%v", err)
	}
	body, rerr := os.ReadFile(target)
	if rerr != nil {
		t.Fatalf("read archived: %v", rerr)
	}
	assertJournalArchived(t, string(body), "tester")
	assertNoTmpFiles(t, dir)
	assertNoTmpFiles(t, filepath.Join(dir, "archive"))
}

// TestArchive_NotFound pins the sentinel for an absent number.
func TestArchive_NotFound(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.Archive(42, "tester"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// TestArchive_AlreadyArchived pins the sentinel for a second archive of
// the same ticket: it already lives under archive/.
func TestArchive_AlreadyArchived(t *testing.T) {
	s, _ := makeDone(t)
	if _, err := s.Archive(1, "tester"); err != nil {
		t.Fatalf("first Archive: %v", err)
	}
	if _, err := s.Archive(1, "tester"); !errors.Is(err, ErrAlreadyArchived) {
		t.Fatalf("expected ErrAlreadyArchived, got %v", err)
	}
}

// TestArchive_RejectsOpenAndWip pins the NotClosedError contract: only
// done/closed tickets archive, the error carries the current status, and
// the file stays untouched in the main directory.
func TestArchive_RejectsOpenAndWip(t *testing.T) {
	for _, st := range []domain.Status{domain.StatusOpen, domain.StatusWip} {
		t.Run(string(st), func(t *testing.T) {
			s, dir := newStore(t)
			if _, err := s.Create(fakeTicket(0)); err != nil {
				t.Fatal(err)
			}
			if st == domain.StatusWip {
				if _, err := s.SetStatus(1, domain.StatusWip, "tester", ""); err != nil {
					t.Fatal(err)
				}
			}
			_, err := s.Archive(1, "tester")
			var nc *NotClosedError
			if !errors.As(err, &nc) {
				t.Fatalf("expected *NotClosedError, got %v", err)
			}
			if nc.Status != st {
				t.Fatalf("NotClosedError.Status = %q, want %q", nc.Status, st)
			}
			if _, serr := os.Stat(filepath.Join(dir, "T-0001-"+string(st)+".md")); serr != nil {
				t.Fatalf("file must remain in main: %v", serr)
			}
		})
	}
}

// TestArchive_TargetCollisionKeepsForeignContent pins the no-replace
// commit: an existing archive target blocks the move (ErrCollision via
// CollisionError), its content is NOT modified and the original ticket
// stays in the main directory.
func TestArchive_TargetCollisionKeepsForeignContent(t *testing.T) {
	s, dir := makeDone(t)
	archiveDir := filepath.Join(dir, "archive")
	target := filepath.Join(archiveDir, "T-0001-done.md")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := []byte("FOREIGN-CONTENT-KEEP")
	if err := os.WriteFile(target, foreign, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := s.Archive(1, "tester")
	var coll *CollisionError
	if !errors.As(err, &coll) || !errors.Is(err, ErrCollision) {
		t.Fatalf("expected CollisionError/ErrCollision, got %v", err)
	}
	if coll.Target != target {
		t.Errorf("CollisionError.Target = %q, want %q", coll.Target, target)
	}
	got, rerr := os.ReadFile(target)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if !bytes.Equal(got, foreign) {
		t.Errorf("existing target content was modified: %q", got)
	}
	if _, serr := os.Stat(filepath.Join(dir, "T-0001-done.md")); serr != nil {
		t.Errorf("original must remain in main: %v", serr)
	}
	assertNoTmpFiles(t, dir)
	assertNoTmpFiles(t, archiveDir)
}

// TestArchive_NumberingAccountsArchive is the §8a regression: a number
// already moved to archive/ must never be reused by Create (N+1).
func TestArchive_NumberingAccountsArchive(t *testing.T) {
	s, _ := makeDone(t)
	if _, err := s.Archive(1, "tester"); err != nil {
		t.Fatal(err)
	}
	n, err := s.Create(fakeTicket(0))
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("Create after archive = %d, want 2 (archived numbers must not be reused)", n)
	}
}

// TestArchiveClosed_DoneGroupBeforeClosedGroup pins the bash output
// order: all done files first, then all closed files, each group in
// ticket-number order (bash iterates the two globs back to back). A
// closed ticket with a LOWER number than a done one proves the grouping:
// numeric order alone would emit T-0001-closed before T-0002-done.
func TestArchiveClosed_DoneGroupBeforeClosedGroup(t *testing.T) {
	s, dir := newStore(t)
	for i := 0; i < 3; i++ {
		if _, err := s.Create(fakeTicket(0)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.SetStatus(1, domain.StatusClosed, "tester", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetStatus(2, domain.StatusDone, "tester", ""); err != nil {
		t.Fatal(err)
	}
	moved, err := s.ArchiveClosed("tester")
	if err != nil {
		t.Fatalf("ArchiveClosed: %v", err)
	}
	want := []string{
		filepath.Join(dir, "archive", "T-0002-done.md"),
		filepath.Join(dir, "archive", "T-0001-closed.md"),
	}
	if !reflect.DeepEqual(moved, want) {
		t.Fatalf("moved = %v, want %v", moved, want)
	}
	if _, serr := os.Stat(filepath.Join(dir, "T-0003-open.md")); serr != nil {
		t.Errorf("open ticket must remain: %v", serr)
	}
}

// TestArchiveClosed_NoClosedIsEmptyNoError pins the bash rc=0 contract:
// zero closed tickets is an empty result with a nil error.
func TestArchiveClosed_NoClosedIsEmptyNoError(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	moved, err := s.ArchiveClosed("tester")
	if err != nil {
		t.Fatalf("ArchiveClosed must succeed with zero closed tickets, got %v", err)
	}
	if len(moved) != 0 {
		t.Fatalf("moved = %v, want empty", moved)
	}
	if _, serr := os.Stat(filepath.Join(dir, "T-0001-open.md")); serr != nil {
		t.Errorf("file must remain: %v", serr)
	}
}

// TestListArchive_ExcludesFromList pins the directory separation: archived
// tickets leave the regular List and appear only in ListArchive (with the
// filename-authoritative status); Find resolves them two-stage.
func TestListArchive_ExcludesFromList(t *testing.T) {
	s, _ := makeDone(t)
	if _, err := s.Archive(1, "tester"); err != nil {
		t.Fatal(err)
	}
	ts, warns := s.List()
	if len(ts) != 0 || len(warns) != 0 {
		t.Fatalf("List = %v/%v, archived tickets must not appear", ts, warns)
	}
	arch, warns := s.ListArchive()
	if len(arch) != 1 || len(warns) != 0 {
		t.Fatalf("ListArchive = %v/%v, want exactly ticket 1", arch, warns)
	}
	if arch[0].Number != 1 || arch[0].Status != domain.StatusDone {
		t.Errorf("archived ticket = #%d %s, want #1 done", arch[0].Number, arch[0].Status)
	}
	tk, ferr := s.Find(1)
	if ferr != nil || tk.Number != 1 {
		t.Fatalf("Find(1) = %v, %v; want the archived ticket", tk, ferr)
	}
}

// TestListArchive_MissingDirEmptyNoWarnings pins that an absent archive/
// directory is an empty result, not an error.
func TestListArchive_MissingDirEmptyNoWarnings(t *testing.T) {
	s, _ := newStore(t)
	arch, warns := s.ListArchive()
	if len(arch) != 0 || len(warns) != 0 {
		t.Fatalf("ListArchive on missing dir = %v/%v, want empty/empty", arch, warns)
	}
}
