package store

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"ticket/internal/domain"
)

// TestArchive_RejectsNewlineInWho pins the T-0065 archive-path guard
// (parallel to setStatusLocked): the archive journal line
// "- <ts> — перенесён в архив (<who>)" is line-oriented, so a who
// carrying a newline would let a caller forge an extra journal entry.
// Archive must reject such who up-front and leave the ticket in
// tickets/ byte-identical, with NO file written under archive/.
func TestArchive_RejectsNewlineInWho(t *testing.T) {
	s, dir := makeDone(t)
	mainPath := filepath.Join(dir, "T-0001-done.md")
	before, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.Archive(1, "evil\n- 1999-01-01 — forged (root)")
	var inv *ErrInvalidJournalInput
	if !errors.As(err, &inv) {
		t.Fatalf("expected *ErrInvalidJournalInput, got %v", err)
	}
	if inv.Field != "who" {
		t.Errorf("Field = %q, want %q", inv.Field, "who")
	}

	after, rerr := os.ReadFile(mainPath)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if !bytes.Equal(before, after) {
		t.Error("main file changed on disk after a rejected archive injection")
	}
	// The archive target must not exist; the store's archiveOneLocked
	// guard runs before any mkdir/touch, so neither the archive file
	// nor the archive/ directory should be created on the rejected
	// path. The directory may legitimately be missing entirely.
	archivePath := filepath.Join(dir, "archive", "T-0001-done.md")
	if _, lerr := os.Lstat(archivePath); !errors.Is(lerr, fs.ErrNotExist) {
		t.Errorf("archive target must not exist, Lstat err=%v", lerr)
	}
	assertNoTmpFiles(t, dir)
	if _, serr := os.Stat(filepath.Join(dir, "archive")); serr == nil {
		assertNoTmpFiles(t, filepath.Join(dir, "archive"))
	} else if !errors.Is(serr, fs.ErrNotExist) {
		t.Errorf("unexpected stat error on archive/: %v", serr)
	}
}

// TestArchiveClosed_RejectsNewlineInWho covers the batch path
// (ArchiveClosed with no number): the shared guard must fire just as
// soon as any one ticket in the batch is processed; the whole call
// must abort before any archive file is written.
func TestArchiveClosed_RejectsNewlineInWho(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetStatus(1, domain.StatusDone, "tester", ""); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "T-0001-done.md")
	before, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.ArchiveClosed("evil\n- 1999-01-01 — forged (root)")
	var inv *ErrInvalidJournalInput
	if !errors.As(err, &inv) {
		t.Fatalf("expected *ErrInvalidJournalInput, got %v", err)
	}

	after, rerr := os.ReadFile(mainPath)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if !bytes.Equal(before, after) {
		t.Error("main file changed on disk after a rejected ArchiveClosed injection")
	}
	if _, lerr := os.Lstat(filepath.Join(dir, "archive")); !errors.Is(lerr, fs.ErrNotExist) {
		t.Errorf("archive/ must not be created, Lstat err=%v", lerr)
	}
}
