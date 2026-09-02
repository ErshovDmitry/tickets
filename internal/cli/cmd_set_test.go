package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"ticket/internal/cli"
)

// createTestTicket runs `new` with fixed args in dir, failing t on any
// non-zero exit. The ticket is T-0001-open.md with who=тестер.
func createTestTicket(t *testing.T, dir string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	args := []string{"new", "Тестовый тикет", "-t", "BUG", "-w", "тестер"}
	if code := cli.Run(args, map[string]string{"TICKETS_DIR": dir}, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(new) = %d, want 0; stderr: %q", code, stderr.String())
	}
}

// setJournalRe matches the transition line `set` must append:
// "- <ts> — статус: open → wip · комментарий (agent)" — the env map
// carries no TICKET_WHO/USER, so whoFrom resolves to "agent".
var setJournalRe = regexp.MustCompile(`(?m)^- ` + tsPattern + ` — статус: open → wip · комментарий \(agent\)$`)

// TestSetHappyPath pins plan §6 item 7(e): set rewrites the status line,
// appends the bash-shaped journal entry, prints the NEW path to stdout,
// and removes the old-status file.
func TestSetHappyPath(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	createTestTicket(t, dir)

	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"set", "1", "wip", "комментарий"}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(set) = %d, want 0; stderr: %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
	wantPath := filepath.Join(dir, "T-0001-wip.md") + "\n"
	if got := stdout.String(); got != wantPath {
		t.Errorf("stdout = %q, want %q", got, wantPath)
	}
	if _, err := os.Stat(filepath.Join(dir, "T-0001-open.md")); !os.IsNotExist(err) {
		t.Errorf("old file still present (stat err=%v), want removed", err)
	}
	body, err := os.ReadFile(wantPath[:len(wantPath)-1])
	if err != nil {
		t.Fatalf("read new file: %v", err)
	}
	if !bytes.Contains(body, []byte("- Статус: wip")) {
		t.Errorf("body missing updated status line:\n%s", body)
	}
	if !setJournalRe.Match(body) {
		t.Errorf("body missing open → wip transition journal line:\n%s", body)
	}
}

// TestSetSameStatusFails pins plan §6 item 7(e): setting the status the
// ticket already has exits 1 with the pinned message and keeps the file.
func TestSetSameStatusFails(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	createTestTicket(t, dir)

	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"set", "1", "open"}, env, &stdout, &stderr); code != 1 {
		t.Fatalf("Run(set same) = %d, want 1", code)
	}
	if got, want := stderr.String(), "ticket: тикет уже в статусе open\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "T-0001-open.md")); err != nil {
		t.Errorf("file should remain untouched: %v", err)
	}
}

// TestSetCollisionOnArchivedNamesArchivePath pins the collision message
// for an archived ticket's done↔closed transition (T-0022): the reported
// file is the REAL target under archive/, the foreign target and the
// archived original survive untouched (no-replace).
//
// Direction: the real ticket is closed and the foreign target is done.
// findLocked resolves equal numbers in scanDir order (T-0001-closed.md
// sorts before T-0001-done.md), so the REAL file is always found first
// and the transition proceeds to the Lstat collision fast-path.
func TestSetCollisionOnArchivedNamesArchivePath(t *testing.T) {
	env, dir := newTicketDir(t)
	runSet(t, env, "1", "closed")
	code, _, _ := runArchive(env, "1")
	if code != 0 {
		t.Fatalf("archive 1 = %d, want 0", code)
	}
	archiveDir := filepath.Join(dir, "archive")
	target := filepath.Join(archiveDir, "T-0001-done.md")
	if err := os.WriteFile(target, []byte("FOREIGN-CONTENT-KEEP"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"set", "1", "done"}, env, &stdout, &stderr); code != 1 {
		t.Fatalf("Run(set archived collision) = %d, want 1", code)
	}
	if want := "ticket: файл " + target + " уже существует\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "FOREIGN-CONTENT-KEEP" {
		t.Errorf("foreign target was modified: %q", got)
	}
	if _, err := os.Stat(filepath.Join(archiveDir, "T-0001-closed.md")); err != nil {
		t.Errorf("archived original must remain: %v", err)
	}
}

// TestSetInvalidStatusFails pins plan §6 item 7(e): an unknown status word
// exits 1 with the pinned message before touching any file.
func TestSetInvalidStatusFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := map[string]string{"TICKETS_DIR": t.TempDir()}
	if code := cli.Run([]string{"set", "1", "bogus"}, env, &stdout, &stderr); code != 1 {
		t.Fatalf("Run(set bogus) = %d, want 1", code)
	}
	if got, want := stderr.String(), "ticket: статус — один из: open wip done closed\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}
