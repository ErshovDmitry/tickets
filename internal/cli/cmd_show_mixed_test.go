package cli_test

// T-0070: `show` must emit the raw file bytes (FindRaw contract) even
// for a mixed-order file (Journal before Summary) — no canonicalization
// on the display path. Split from cmd_show_test.go (300-line cap).

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"ticket/internal/cli"
)

// mixedOrderOpen is an open T-0001 ticket whose Journal section sits
// before Summary — parseable, but not canonically ordered.
const mixedOrderOpen = `# T-0001 · BUG: Mixed order

- Status (Статус): open
- Priority (Приоритет): high
- Created (Создан): 2026-09-10 10:00 · by (кем): tester
- Project (Проект): tickets

## Journal (Журнал)
- 2026-09-10 10:00 — тикет создан (tester).

## Summary (Кратко)
Mixed order

## Details (Подробности)
details body

## User comments (Комментарии от пользователя)

## Comments (Комментарии)
`

// TestShowMixedFileRawBytes pins the FindRaw display contract: show
// streams the on-disk bytes verbatim for a mixed-order file — stdout
// bytes equal the file bytes exactly, no normalization, exit 0.
func TestShowMixedFileRawBytes(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	path := filepath.Join(dir, "T-0001-open.md")
	if err := os.WriteFile(path, []byte(mixedOrderOpen), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"show", "1"}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(show) = %d, want 0; stderr: %q", code, stderr.String())
	}
	if !bytes.Equal(stdout.Bytes(), raw) {
		t.Errorf("show stdout != raw file bytes:\n got %q\nwant %q", stdout.Bytes(), raw)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}
