//go:build unix

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
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
	if code := initProject(root, &stdout, &stderr); code != 1 {
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

// TestInitProjectBrokenSymlink verifies a broken tickets symlink fails
// cleanly: os.Stat reports fs.ErrNotExist, MkdirAll then fails (exact code
// is platform-dependent — assert exit 1, the RU prefix and that the
// symlink itself was not overwritten).
func TestInitProjectBrokenSymlink(t *testing.T) {
	root := t.TempDir()
	tickets := filepath.Join(root, "tickets")
	if err := os.Symlink(filepath.Join(root, "nonexistent-target"), tickets); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := initProject(root, &stdout, &stderr); code != 1 {
		t.Fatalf("broken symlink: code=%d want 1 stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "ticket: Ошибка инициализации:") {
		t.Errorf("stderr=%q missing init error prefix", stderr.String())
	}
	info, err := os.Lstat(tickets)
	if err != nil {
		t.Fatalf("Lstat after init: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("broken symlink was overwritten: mode=%v", info.Mode())
	}
}
