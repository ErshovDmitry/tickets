//go:build unix

package cli

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"ticket/internal/domain"
)

// TestInitProjectConflictFIFO verifies a FIFO at tickets is a conflict
// (the !IsDir branch on a non-regular file: os.Stat on a FIFO succeeds
// without blocking — only open would block).
func TestInitProjectConflictFIFO(t *testing.T) {
	root := t.TempDir()
	tickets := filepath.Join(root, "tickets")
	if err := syscall.Mkfifo(tickets, 0o600); err != nil {
		t.Fatalf("Mkfifo: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := initProject(root, domain.LangRU, &stdout, &stderr); code != 1 {
		t.Fatalf("FIFO conflict: code=%d want 1", code)
	}
	wantStderr := "ticket: Конфликт: " + tickets + "\n"
	if stderr.String() != wantStderr {
		t.Errorf("stderr=%q want %q", stderr.String(), wantStderr)
	}

	info, err := os.Lstat(tickets)
	if err != nil {
		t.Fatalf("Lstat after conflict: %v", err)
	}
	if info.Mode()&os.ModeNamedPipe == 0 {
		t.Errorf("FIFO was modified: mode=%v", info.Mode())
	}
}

// TestInitProjectBrokenSymlink verifies a broken tickets symlink is a
// clean conflict (T-0062): the single Stat-follow on the link fails, init
// exits 1 with the exact conflict line and the symlink itself was not
// overwritten.
func TestInitProjectBrokenSymlink(t *testing.T) {
	root := t.TempDir()
	tickets := filepath.Join(root, "tickets")
	if err := os.Symlink(filepath.Join(root, "nonexistent-target"), tickets); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := initProject(root, domain.LangRU, &stdout, &stderr); code != 1 {
		t.Fatalf("broken symlink: code=%d want 1 stderr=%q", code, stderr.String())
	}
	wantStderr := "ticket: Конфликт: " + tickets + "\n"
	if stderr.String() != wantStderr {
		t.Errorf("stderr=%q want %q", stderr.String(), wantStderr)
	}
	info, err := os.Lstat(tickets)
	if err != nil {
		t.Fatalf("Lstat after init: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("broken symlink was overwritten: mode=%v", info.Mode())
	}
}

// TestInitProjectArchiveSymlink verifies a pre-planted tickets/archive
// symlink is a conflict, not a silent creation outside tickets/ (T-0082):
// exit 1 with the exact conflict line, the outside target is never
// created and the symlink itself survives untouched.
func TestInitProjectArchiveSymlink(t *testing.T) {
	root := t.TempDir()
	tickets := filepath.Join(root, "tickets")
	if err := os.MkdirAll(tickets, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	archive := filepath.Join(tickets, "archive")
	target := filepath.Join(root, "outside-target")
	if err := os.Symlink(target, archive); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := initProject(root, domain.LangRU, &stdout, &stderr); code != 1 {
		t.Fatalf("archive symlink: code=%d want 1 stderr=%q", code, stderr.String())
	}
	wantStderr := "ticket: Конфликт: " + archive + "\n"
	if stderr.String() != wantStderr {
		t.Errorf("stderr=%q want %q", stderr.String(), wantStderr)
	}
	if _, err := os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("outside target exists after conflict: err=%v", err)
	}
	info, err := os.Lstat(archive)
	if err != nil {
		t.Fatalf("Lstat after conflict: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("archive symlink was overwritten: mode=%v", info.Mode())
	}
}

// TestInitProjectSymlinkToDirTickets verifies the preserved behavior
// (T-0062): tickets is a symlink to a real directory — init proceeds with
// exit 0 and creates archive/ in the symlink's target.
func TestInitProjectSymlinkToDirTickets(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real-tickets")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	tickets := filepath.Join(root, "tickets")
	if err := os.Symlink(real, tickets); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := initProject(root, domain.LangRU, &stdout, &stderr); code != 0 {
		t.Fatalf("symlink to dir: code=%d stderr=%q", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(real, "archive")); err != nil {
		t.Errorf("archive/ not created in target: %v", err)
	}
}
