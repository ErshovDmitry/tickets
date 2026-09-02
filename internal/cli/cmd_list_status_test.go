package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/cli"
)

// writeTamperedTicket creates TICKETS_DIR under a fresh temp root holding
// T-0001-open.md whose body falsely claims the given status. The filename
// stays open — the divergence list filters must NOT trust.
func writeTamperedTicket(t *testing.T, claimed string) (dir string, body []byte) {
	t.Helper()
	root := t.TempDir()
	dir = filepath.Join(root, "tickets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body = []byte("# T-0001 · BUG: bait\n" +
		"\n" +
		"- Статус: " + claimed + "\n" +
		"- Приоритет: high\n" +
		"- Создан: 2026-09-02 10:00 · кем: тестер\n" +
		"- Проект: tickets\n" +
		"\n" +
		"## Кратко\nbait\n" +
		"\n## Подробности\nx\n" +
		"\n## Журнал\n" +
		"- 2026-09-02 10:00 — тикет создан (тестер).\n")
	p := filepath.Join(dir, "T-0001-open.md")
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, body
}

// TestList_BodyStatusCannotFakeFilter is the V19 regression: a crafted
// "- Статус: done" line inside T-0001-open.md must not move the ticket
// into the done bucket. The scan-matched filename status drives List.
func TestList_BodyStatusCannotFakeFilter(t *testing.T) {
	dir, _ := writeTamperedTicket(t, "done")
	env := map[string]string{"TICKETS_DIR": dir}

	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"list", "open"}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("list open = %d, stderr: %q", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "T-0001") || !strings.Contains(out, "open") {
		t.Errorf("list open must show T-0001 as open, got %q", out)
	}
	if strings.Contains(out, "done") {
		t.Errorf("list open leaked the tampered body status: %q", out)
	}

	stdout.Reset()
	stderr.Reset()
	if code := cli.Run([]string{"list", "done"}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("list done = %d, stderr: %q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "T-0001") {
		t.Errorf("list done showed the open ticket: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Нет тикетов (done)") {
		t.Errorf("list done output = %q, want the empty-bucket line", stdout.String())
	}
}
