package cli

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"ticket/internal/domain"
)

// TestInitProjectCreatesStructure verifies clean initialization: tickets/
// and tickets/archive/ directories, nothing else, and the RU stdout line
// (plan T-0057 §B1: no bin/, no symlink, no starter files).
func TestInitProjectCreatesStructure(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer

	if code := initProject(root, domain.LangRU, &stdout, &stderr); code != 0 {
		t.Fatalf("initProject: code=%d stderr=%q", code, stderr.String())
	}
	tickets := filepath.Join(root, "tickets")
	for _, dir := range []string{tickets, filepath.Join(tickets, "archive"), filepath.Join(tickets, "deleted")} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("Stat %s: %v", dir, err)
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", dir)
		}
	}
	entries, err := os.ReadDir(tickets)
	if err != nil {
		t.Fatalf("ReadDir tickets: %v", err)
	}
	if len(entries) != 2 || entries[0].Name() != "archive" || entries[1].Name() != "deleted" {
		t.Errorf("tickets/ holds %d entries, want exactly [archive deleted]", len(entries))
	}
	wantStdout := "Инициализировано: " + tickets + "\n"
	if stdout.String() != wantStdout {
		t.Errorf("stdout=%q want %q", stdout.String(), wantStdout)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr=%q want empty", stderr.String())
	}
}

// TestInitProjectIdempotent verifies repeat init is no-op exit 0 with the
// same stdout (§B3: no separate "already initialized" text).
func TestInitProjectIdempotent(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer

	if code := initProject(root, domain.LangRU, &stdout, &stderr); code != 0 {
		t.Fatalf("first init: code=%d stderr=%q", code, stderr.String())
	}
	first := stdout.String()

	stdout.Reset()
	stderr.Reset()
	if code := initProject(root, domain.LangRU, &stdout, &stderr); code != 0 {
		t.Fatalf("repeat init: code=%d stderr=%q", code, stderr.String())
	}
	if stdout.String() != first {
		t.Errorf("repeat stdout=%q want %q", stdout.String(), first)
	}
	entries, err := os.ReadDir(filepath.Join(root, "tickets"))
	if err != nil {
		t.Fatalf("ReadDir tickets: %v", err)
	}
	if len(entries) != 2 || entries[0].Name() != "archive" || entries[1].Name() != "deleted" {
		t.Errorf("repeat init changed the structure: %d entries", len(entries))
	}
}

// TestInitProjectConflictFile verifies the conflict branch when tickets
// exists as a regular file: exit 1, empty stdout, bytes preserved.
func TestInitProjectConflictFile(t *testing.T) {
	root := t.TempDir()
	tickets := filepath.Join(root, "tickets")
	origContent := []byte("blocker")
	if err := os.WriteFile(tickets, origContent, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := initProject(root, domain.LangRU, &stdout, &stderr); code != 1 {
		t.Fatalf("conflict file: code=%d want 1", code)
	}
	wantStderr := "ticket: Конфликт: " + tickets + "\n"
	if stderr.String() != wantStderr {
		t.Errorf("stderr=%q want %q", stderr.String(), wantStderr)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout=%q want empty on conflict", stdout.String())
	}
	data, err := os.ReadFile(tickets)
	if err != nil {
		t.Fatalf("ReadFile after conflict: %v", err)
	}
	if !bytes.Equal(data, origContent) {
		t.Errorf("conflict overwrote the file: got %q want %q", data, origContent)
	}
}

// TestInitProjectArchiveRegularFile verifies a pre-planted regular file
// at tickets/archive is a conflict (T-0082, cross-platform): exit 1 with
// the exact conflict line and the file's bytes preserved.
func TestInitProjectArchiveRegularFile(t *testing.T) {
	root := t.TempDir()
	tickets := filepath.Join(root, "tickets")
	if err := os.MkdirAll(tickets, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	archive := filepath.Join(tickets, "archive")
	origContent := []byte("planted file")
	if err := os.WriteFile(archive, origContent, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := initProject(root, domain.LangRU, &stdout, &stderr); code != 1 {
		t.Fatalf("archive file: code=%d want 1 stderr=%q", code, stderr.String())
	}
	wantStderr := "ticket: Конфликт: " + archive + "\n"
	if stderr.String() != wantStderr {
		t.Errorf("stderr=%q want %q", stderr.String(), wantStderr)
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatalf("ReadFile after conflict: %v", err)
	}
	if !bytes.Equal(data, origContent) {
		t.Errorf("conflict overwrote the file: got %q want %q", data, origContent)
	}
}

// TestInitProjectDeletedRegularFile verifies a pre-planted regular file at
// tickets/deleted is a conflict (T-0189, cross-platform): exit 1 with the
// exact conflict line and the file's bytes preserved. archive/ is NOT
// pre-created (W1 review A2): the conflict must not leave a partial
// side effect, so archive/ must not exist afterwards.
func TestInitProjectDeletedRegularFile(t *testing.T) {
	root := t.TempDir()
	tickets := filepath.Join(root, "tickets")
	if err := os.MkdirAll(tickets, 0o755); err != nil {
		t.Fatalf("MkdirAll tickets: %v", err)
	}
	deleted := filepath.Join(tickets, "deleted")
	origContent := []byte("planted file")
	if err := os.WriteFile(deleted, origContent, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := initProject(root, domain.LangRU, &stdout, &stderr); code != 1 {
		t.Fatalf("deleted file: code=%d want 1 stderr=%q", code, stderr.String())
	}
	wantStderr := "ticket: Конфликт: " + deleted + "\n"
	if stderr.String() != wantStderr {
		t.Errorf("stderr=%q want %q", stderr.String(), wantStderr)
	}
	data, err := os.ReadFile(deleted)
	if err != nil {
		t.Fatalf("ReadFile after conflict: %v", err)
	}
	if !bytes.Equal(data, origContent) {
		t.Errorf("conflict overwrote the file: got %q want %q", data, origContent)
	}
	if _, err := os.Lstat(filepath.Join(tickets, "archive")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("archive/ created despite deleted conflict: err=%v", err)
	}
}

// TestInitProjectPreservesExistingTickets verifies existing ticket files
// stay byte-identical while archive/ is added.
func TestInitProjectPreservesExistingTickets(t *testing.T) {
	root := t.TempDir()
	tickets := filepath.Join(root, "tickets")
	if err := os.MkdirAll(tickets, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	origContent := []byte("title: preserved\n")
	existing := filepath.Join(tickets, "T-0001-open.md")
	if err := os.WriteFile(existing, origContent, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := initProject(root, domain.LangRU, &stdout, &stderr); code != 0 {
		t.Fatalf("init with existing tickets: code=%d stderr=%q", code, stderr.String())
	}
	data, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("ReadFile existing: %v", err)
	}
	if !bytes.Equal(data, origContent) {
		t.Errorf("existing ticket modified: got %q want %q", data, origContent)
	}
	if info, err := os.Stat(filepath.Join(tickets, "archive")); err != nil || !info.IsDir() {
		t.Errorf("archive/ not added by init (err=%v)", err)
	}
}

// runInitIn runs Run init from work: exit 0, exact stdout (Abs target,
// no symlink resolution), target/{archive,deleted} created, cwd clean.
func runInitIn(t *testing.T, work string, args []string, env map[string]string, target string) {
	t.Helper()
	t.Chdir(work)
	code, out, errb := runCLI(t, args, env)
	if want := "Инициализировано: " + target + "\n"; code != 0 || errb != "" || out != want {
		t.Fatalf("Run(%v) = %d, stdout=%q stderr=%q; want 0 with empty stderr and stdout %q", args, code, out, errb, want)
	}
	for _, sub := range []string{"archive", "deleted"} {
		if info, err := os.Stat(filepath.Join(target, sub)); err != nil || !info.IsDir() {
			t.Errorf("target/%s missing or not a dir: %v", sub, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(work, "tickets")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stray tickets/ in cwd: %v", err)
	}
}

// TestInitResolvesTarget covers plan matrix items 1-5, 7-8 via Run: -C,
// $TICKETS_DIR, flag>env priority, nested target, --tickets-dir long form,
// relative -C, repeat-run idempotency.
func TestInitResolvesTarget(t *testing.T) {
	cases := []struct {
		name, flag, flagSeg, envSeg string
		repeat                      bool
	}{
		{"flag -C", "-C", "t", "", true},
		{"env TICKETS_DIR", "", "", "e", false},
		{"flag beats env", "-C", "f", "e", false},
		{"nested nonexistent", "-C", "a/b/tickets", "", false},
		{"long --tickets-dir", "--tickets-dir", "l", "", false},
		{"relative -C", "-C", "rel", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			work := t.TempDir()
			args := []string{"init"}
			if tc.flag != "" {
				args = []string{tc.flag, tc.flagSeg, "init"}
			}
			env := map[string]string{"TICKETS_DIR": filepath.Join(work, tc.envSeg)}
			seg := tc.flagSeg
			if seg == "" {
				seg = tc.envSeg
			}
			runInitIn(t, work, args, env, filepath.Join(work, seg))
			if _, err := os.Lstat(filepath.Join(work, tc.envSeg)); tc.envSeg != "" && tc.flag != "" && !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("env target created despite flag priority: %v", err)
			}
			if tc.repeat {
				t.Chdir(work)
				code, out, errb := runCLI(t, args, nil)
				want := "Инициализировано: " + filepath.Join(work, tc.flagSeg) + "\n"
				if code != 0 || errb != "" || out != want {
					t.Fatalf("repeat Run = %d, stdout=%q stderr=%q; want 0 with stdout %q", code, out, errb, want)
				}
			}
		})
	}
}

// TestInitConflictExplicitFile covers plan matrix item 6: an explicit
// target that is a regular file (via -C and via $TICKETS_DIR) is a
// conflict: exit 1, exact conflict line, empty stdout, clean cwd.
func TestInitConflictExplicitFile(t *testing.T) {
	for _, tc := range []struct {
		name   string
		useEnv bool
	}{
		{"flag", false},
		{"env", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			work := t.TempDir()
			target := filepath.Join(work, "blocker")
			if err := os.WriteFile(target, []byte("blocker"), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			t.Chdir(work)
			args, env := []string{"-C", target, "init"}, map[string]string(nil)
			if tc.useEnv {
				args, env = []string{"init"}, map[string]string{"TICKETS_DIR": target}
			}
			code, out, errb := runCLI(t, args, env)
			if want := "ticket: Конфликт: " + target + "\n"; code != 1 || out != "" || errb != want {
				t.Fatalf("Run = %d stdout=%q stderr=%q, want 1 with empty stdout and conflict line %q", code, out, errb, want)
			}
			if _, err := os.Lstat(filepath.Join(work, "tickets")); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("stray tickets/ in cwd: %v", err)
			}
		})
	}
}
