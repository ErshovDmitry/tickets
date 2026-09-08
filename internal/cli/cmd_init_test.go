package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestInitProjectCreatesStructure verifies clean initialization: tickets/
// and tickets/archive/ directories, nothing else, and the RU stdout line
// (plan T-0057 §B1: no bin/, no symlink, no starter files).
func TestInitProjectCreatesStructure(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer

	if code := initProject(root, &stdout, &stderr); code != 0 {
		t.Fatalf("initProject: code=%d stderr=%q", code, stderr.String())
	}
	tickets := filepath.Join(root, "tickets")
	for _, dir := range []string{tickets, filepath.Join(tickets, "archive")} {
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
	if len(entries) != 1 || entries[0].Name() != "archive" {
		t.Errorf("tickets/ holds %d entries, want exactly [archive]", len(entries))
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

	if code := initProject(root, &stdout, &stderr); code != 0 {
		t.Fatalf("first init: code=%d stderr=%q", code, stderr.String())
	}
	first := stdout.String()

	stdout.Reset()
	stderr.Reset()
	if code := initProject(root, &stdout, &stderr); code != 0 {
		t.Fatalf("repeat init: code=%d stderr=%q", code, stderr.String())
	}
	if stdout.String() != first {
		t.Errorf("repeat stdout=%q want %q", stdout.String(), first)
	}
	entries, err := os.ReadDir(filepath.Join(root, "tickets"))
	if err != nil {
		t.Fatalf("ReadDir tickets: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "archive" {
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
	if code := initProject(root, &stdout, &stderr); code != 1 {
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
	if code := initProject(root, &stdout, &stderr); code != 1 {
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
	if code := initProject(root, &stdout, &stderr); code != 0 {
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
