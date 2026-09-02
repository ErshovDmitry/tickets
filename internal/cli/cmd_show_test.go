package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"ticket/internal/cli"
)

// TestShow_TraversalStatusCannotEscapeTicketsDir pins the T-0006 fix:
// the body's Status line is user-controlled ("^- Статус: (.*)$" parses
// any bytes), so show must read the file the directory scan actually
// matched — never a path rebuilt from domain.Filename(t.Number, t.Status).
// A body status "../../outside/secret" made the old code read
// <parent-of-TICKETS_DIR>/outside/secret.md: filepath.Join cleans the
// inner ".." elements of "T-0001-../../outside/secret.md" out of the dir.
func TestShow_TraversalStatusCannotEscapeTicketsDir(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "tickets")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := []byte("TOPSECRET-OUTSIDE")
	secret := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(secret, sentinel, 0o600); err != nil {
		t.Fatal(err)
	}

	// Legitimate, scannable ticket (T-0001-open.md matches
	// ^T-\d{4}-[a-z]+\.md$) whose BODY status is the traversal payload.
	// Join(dir, Filename(1, "../../../outside/secret")) cleans to
	// <root>/outside/secret.md: the first ".." eats the "T-0001-.."
	// element, the second pops the tickets dir itself.
	body := []byte("# T-0001 · BUG: bait\n" +
		"\n" +
		"- Статус: ../../../outside/secret\n" +
		"- Приоритет: high\n" +
		"- Создан: 2026-09-02 10:00 · кем: тестер\n" +
		"- Проект: tickets\n" +
		"\n" +
		"## Кратко\nbait\n" +
		"\n## Подробности\nx\n" +
		"\n## Журнал\n" +
		"- 2026-09-02 10:00 — тикет создан (тестер).\n")
	ticket := filepath.Join(dir, "T-0001-open.md")
	if err := os.WriteFile(ticket, body, 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	env := map[string]string{"TICKETS_DIR": dir}
	if code := cli.Run([]string{"show", "1"}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(show) = %d, want 0; stderr: %q", code, stderr.String())
	}
	if bytes.Contains(stdout.Bytes(), sentinel) {
		t.Errorf("show leaked outside file contents %q in stdout", sentinel)
	}
	if got := stdout.Bytes(); !bytes.Equal(got, body) {
		t.Errorf("show stdout != raw ticket bytes:\n got %q\nwant %q", got, body)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
	// The outside sentinel file is untouched evidence, never consumed.
	got, err := os.ReadFile(secret)
	if err != nil || !bytes.Equal(got, sentinel) {
		t.Fatalf("outside sentinel changed (err=%v, data=%q)", err, got)
	}
}
