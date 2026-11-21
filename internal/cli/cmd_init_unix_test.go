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

// TestInitProjectDeletedSymlink verifies a pre-planted tickets/deleted
// symlink is a conflict (T-0189): IsRealDir never follows a trailing
// symlink, so even a resolvable link is rejected. Exit 1 with the exact
// conflict line, the outside target is never created and the symlink
// itself survives untouched. archive/ is NOT pre-created (W1 review A2):
// the conflict must not leave a partial side effect, so archive/ must
// not exist afterwards.
func TestInitProjectDeletedSymlink(t *testing.T) {
	root := t.TempDir()
	tickets := filepath.Join(root, "tickets")
	if err := os.MkdirAll(tickets, 0o755); err != nil {
		t.Fatalf("MkdirAll tickets: %v", err)
	}
	deleted := filepath.Join(tickets, "deleted")
	target := filepath.Join(root, "outside-target")
	if err := os.Symlink(target, deleted); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := initProject(root, domain.LangRU, &stdout, &stderr); code != 1 {
		t.Fatalf("deleted symlink: code=%d want 1 stderr=%q", code, stderr.String())
	}
	wantStderr := "ticket: Конфликт: " + deleted + "\n"
	if stderr.String() != wantStderr {
		t.Errorf("stderr=%q want %q", stderr.String(), wantStderr)
	}
	if _, err := os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("outside target exists after conflict: err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(tickets, "archive")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("archive/ created despite deleted conflict: err=%v", err)
	}
	info, err := os.Lstat(deleted)
	if err != nil {
		t.Fatalf("Lstat after conflict: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("deleted symlink was overwritten: mode=%v", info.Mode())
	}
}

// TestInitProjectDeletedBrokenSymlink verifies a broken tickets/deleted
// symlink is a conflict (T-0189): Lstat sees the link itself, init exits 1
// with the exact conflict line and the broken link is not replaced by a
// real directory. archive/ is NOT pre-created (W1 review A2): the
// conflict must not leave a partial side effect, so archive/ must not
// exist afterwards.
func TestInitProjectDeletedBrokenSymlink(t *testing.T) {
	root := t.TempDir()
	tickets := filepath.Join(root, "tickets")
	if err := os.MkdirAll(tickets, 0o755); err != nil {
		t.Fatalf("MkdirAll tickets: %v", err)
	}
	deleted := filepath.Join(tickets, "deleted")
	if err := os.Symlink(filepath.Join(root, "nonexistent-target"), deleted); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := initProject(root, domain.LangRU, &stdout, &stderr); code != 1 {
		t.Fatalf("deleted broken symlink: code=%d want 1 stderr=%q", code, stderr.String())
	}
	wantStderr := "ticket: Конфликт: " + deleted + "\n"
	if stderr.String() != wantStderr {
		t.Errorf("stderr=%q want %q", stderr.String(), wantStderr)
	}
	if _, err := os.Lstat(filepath.Join(tickets, "archive")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("archive/ created despite deleted conflict: err=%v", err)
	}
	info, err := os.Lstat(deleted)
	if err != nil {
		t.Fatalf("Lstat after init: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("broken symlink was overwritten: mode=%v", info.Mode())
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

// TestInitExplicitSymlinkToDir verifies the explicit tier keeps the T-0062
// symlink semantics (plan D3): a -C target that is a symlink to a real
// directory proceeds — archive/ is created in the real target and stdout
// carries the Abs of the link path itself (no EvalSymlinks resolution).
func TestInitExplicitSymlinkToDir(t *testing.T) {
	root := t.TempDir()
	work := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	t.Chdir(work)
	code, out, errb := runCLI(t, []string{"-C", link, "init"}, nil)
	if code != 0 || errb != "" {
		t.Fatalf("Run(-C link) = %d, stderr: %q; want 0", code, errb)
	}
	if want := "Инициализировано: " + link + "\n"; out != want {
		t.Errorf("stdout = %q, want %q (no symlink resolution)", out, want)
	}
	if _, err := os.Stat(filepath.Join(real, "archive")); err != nil {
		t.Errorf("archive/ not created in link target: %v", err)
	}
}

// TestInitExplicitConflictSubdir verifies the explicit tier probes
// <target>/archive and <target>/deleted (T-0082/T-0189 logic, reached via
// -C): a regular file or a broken symlink there is a conflict — exit 1
// with the exact conflict line and no partial side effects.
func TestInitExplicitConflictSubdir(t *testing.T) {
	for _, tc := range []struct {
		name, sub string
		symlink   bool
	}{
		{"archive file", "archive", false},
		{"deleted file", "deleted", false},
		{"archive broken symlink", "archive", true},
		{"deleted broken symlink", "deleted", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			work := t.TempDir()
			target := filepath.Join(root, "t")
			if err := os.MkdirAll(target, 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			sub := filepath.Join(target, tc.sub)
			if tc.symlink {
				if err := os.Symlink(filepath.Join(root, "nonexistent-target"), sub); err != nil {
					t.Fatalf("Symlink: %v", err)
				}
			} else if err := os.WriteFile(sub, []byte("planted"), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			t.Chdir(work)
			code, out, errb := runCLI(t, []string{"-C", target, "init"}, nil)
			if code != 1 || out != "" {
				t.Fatalf("Run = %d stdout=%q, want 1 with empty stdout", code, out)
			}
			if want := "ticket: Конфликт: " + sub + "\n"; errb != want {
				t.Errorf("stderr = %q, want %q", errb, want)
			}
			other := "archive"
			if tc.sub == "archive" {
				other = "deleted"
			}
			if _, err := os.Lstat(filepath.Join(target, other)); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("%s/ created despite %s conflict: %v", other, tc.sub, err)
			}
		})
	}
}
