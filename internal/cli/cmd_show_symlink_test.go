package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"ticket/internal/cli"
)

// TestShow_SymlinkedTicketNameCannotLeakOutside is the V19 containment
// regression on the CLI surface: a ticket-named symlink pointing outside
// TICKETS_DIR must make `show` report «не найден» (the scan rejects the
// entry), never print the link target's bytes. os.ReadFile follows
// symlinks, so before the scan fix this leaked the outside file verbatim.
func TestShow_SymlinkedTicketNameCannotLeakOutside(t *testing.T) {
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
	// The outside file is a FULLY VALID ticket body (H1 T-0001 matches
	// the symlink's filename) with the sentinel embedded — the exact
	// escape shape: readTicketFile's H1-number check only blocks outside
	// content that does not parse as the expected ticket.
	body := append([]byte("# T-0001 · BUG: bait\n\n"+
		"- Статус: open\n"+
		"- Приоритет: high\n"+
		"- Создан: 2026-09-02 10:00 · кем: тестер\n"+
		"- Проект: tickets\n\n"+
		"## Кратко\n"), sentinel...)
	body = append(body, []byte("\n\n## Подробности\nx\n\n## Журнал\n"+
		"- 2026-09-02 10:00 — тикет создан (тестер).\n")...)
	if err := os.WriteFile(secret, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(dir, "T-0001-open.md")); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}

	var stdout, stderr bytes.Buffer
	env := map[string]string{"TICKETS_DIR": dir}
	if code := cli.Run([]string{"show", "1"}, env, &stdout, &stderr); code != 1 {
		t.Fatalf("Run(show) = %d, want 1 (ticket-named symlink is not a ticket)", code)
	}
	if bytes.Contains(stdout.Bytes(), sentinel) {
		t.Errorf("show leaked outside file contents %q in stdout", sentinel)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if got, err := os.ReadFile(secret); err != nil || !bytes.Equal(got, body) {
		t.Fatalf("outside sentinel changed (err=%v, data=%q)", err, got)
	}
}
