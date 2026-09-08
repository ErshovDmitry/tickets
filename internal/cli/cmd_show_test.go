package cli_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// TestShow_UnreadableTicketReportsRealError pins the error-contract fix:
// a present-but-unreadable ticket file (chmod 000) is NOT «не найден» —
// show must exit 1 with the wrapped store error («ticket: store: read
// T-0001-open.md: …»), never the not-found message. Skips where chmod
// cannot produce the condition (Windows, root).
func TestShow_UnreadableTicketReportsRealError(t *testing.T) {
	skipRootOrWindows(t)
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	createTestTicket(t, dir)
	ticket := filepath.Join(dir, "T-0001-open.md")
	if err := os.Chmod(ticket, 0o000); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"show", "1"}, env, &stdout, &stderr); code != 1 {
		t.Fatalf("Run(show unreadable) = %d, want 1; stderr: %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if got := stderr.String(); !strings.HasPrefix(got, "ticket: store: read T-0001-open.md:") {
		t.Errorf("stderr = %q, want prefix %q", got, "ticket: store: read T-0001-open.md:")
	}
	if got := stderr.String(); strings.Contains(got, "не найден") {
		t.Errorf("stderr = %q masks a real error as not-found", got)
	}
}

// TestShowAbsentNumberNotFound pins the absent-number contract: with no
// matching file, `show` exits 1 with the bash-compatible «не найден»
// message quoting the user's argument. In an empty store the hint names
// the emptiness (T-0076 branch 2).
func TestShowAbsentNumberNotFound(t *testing.T) {
	env := map[string]string{"TICKETS_DIR": t.TempDir()}

	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"show", "9999"}, env, &stdout, &stderr); code != 1 {
		t.Fatalf("Run(show 9999) = %d, want 1; stderr: %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if got, want := stderr.String(),
		"ticket: тикет «9999» не найден\nticket: подсказка: хранилище пусто; используйте ticket list\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

// TestShowHintEmptyButWarned pins the T-0076 guard branch: when the
// store has no parseable tickets (max==0) yet the scan rejected present
// files (warnings), «хранилище пусто» would be a false claim — the hint
// is the generic list hint (show also searches archive/). The warning
// source mirrors the store-level precedent
// TestList_PartialCorruptionReturnsWarnings (a .md file whose name
// fails ParseFilename, portable — no chmod, no root/Windows skip),
// placed inside archive/: a lone foreign file in the main dir is
// rejected earlier by the paths layer as «not a tickets directory»,
// while the archive/ marker passes validation and ListArchive still
// yields the ParseWarning.
func TestShowHintEmptyButWarned(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	if err := os.MkdirAll(filepath.Join(dir, "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "archive", "T-NOT-A-TICKET.md")
	if err := os.WriteFile(bad, []byte("# garbage"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"show", "9999"}, env, &stdout, &stderr); code != 1 {
		t.Fatalf("Run(show 9999) = %d, want 1; stderr: %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if got := stderr.String(); strings.Contains(got, "хранилище пусто") {
		t.Errorf("stderr = %q claims an empty store while scan warnings were present", got)
	}
	if want := "ticket: тикет «9999» не найден\nticket: подсказка: используйте ticket list (show ищет и в archive/)\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
}

// writeTicketFile writes a minimal parseable ticket file with the given
// number (H1 matches the file name) for tests that need number gaps.
func writeTicketFile(t *testing.T, dir string, n int) {
	t.Helper()
	body := fmt.Sprintf("# T-%04d · BUG: bait\n\n"+
		"- Статус: open\n"+
		"- Приоритет: high\n"+
		"- Создан: 2026-09-02 10:00 · кем: тестер\n"+
		"- Проект: tickets\n\n"+
		"## Кратко\nbait\n\n## Подробности\nx\n\n## Журнал\n"+
		"- 2026-09-02 10:00 — тикет создан (тестер).\n", n)
	name := filepath.Join(dir, fmt.Sprintf("T-%04d-open.md", n))
	if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestShowExtraArgumentsWarned pins T-0076: arguments after the first
// are ignored bash-compatibly with a one-line warning on stderr; exit
// code and stdout (the ticket bytes) stay unchanged.
func TestShowExtraArgumentsWarned(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	createTestTicket(t, dir)
	raw, err := os.ReadFile(filepath.Join(dir, "T-0001-open.md"))
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"show", "1", "лишний", "ещё"}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(show extra args) = %d, want 0; stderr: %q", code, stderr.String())
	}
	if want := "ticket: предупреждение: лишние аргументы игнорируются: лишний ещё\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	if got := stdout.String(); got != string(raw) {
		t.Errorf("stdout = %q, want ticket bytes %q", got, raw)
	}
}

// TestShowHintZero pins T-0076 branch 1: `show 0` gets the
// numbers-start-at-1 hint, ahead of any other hint.
func TestShowHintZero(t *testing.T) {
	env := map[string]string{"TICKETS_DIR": t.TempDir()}

	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"show", "0"}, env, &stdout, &stderr); code != 1 {
		t.Fatalf("Run(show 0) = %d, want 1; stderr: %q", code, stderr.String())
	}
	if want := "ticket: тикет «0» не найден\nticket: подсказка: номера тикетов начинаются с 1; используйте ticket list\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

// TestShowHintAboveMax pins T-0076 branch 3: a number above the store's
// actual maximum (1 here) reports the real bound, never hardcoded 9999.
func TestShowHintAboveMax(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	createTestTicket(t, dir)

	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"show", "9999"}, env, &stdout, &stderr); code != 1 {
		t.Fatalf("Run(show 9999) = %d, want 1; stderr: %q", code, stderr.String())
	}
	if want := "ticket: тикет «9999» не найден\nticket: подсказка: в этом хранилище номера до 1; используйте ticket list\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

// TestShowHintInRangeGap pins T-0076 branch 4: a number inside [1..max]
// with no file (gap: 1 and 3 exist) gets the generic list hint — show
// also searches archive/, so absence in main/ is not conclusive.
func TestShowHintInRangeGap(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	createTestTicket(t, dir)
	writeTicketFile(t, dir, 3)

	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"show", "2"}, env, &stdout, &stderr); code != 1 {
		t.Fatalf("Run(show 2) = %d, want 1; stderr: %q", code, stderr.String())
	}
	if want := "ticket: тикет «2» не найден\nticket: подсказка: используйте ticket list (show ищет и в archive/)\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

// TestShowNonNumericNoHint pins the T-0076 boundary: a non-numeric
// argument gets the bare not-found message without any hint (the hint
// logic applies to numeric arguments only; T-0072 owns this branch).
func TestShowNonNumericNoHint(t *testing.T) {
	env := map[string]string{"TICKETS_DIR": t.TempDir()}

	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"show", "abc"}, env, &stdout, &stderr); code != 1 {
		t.Fatalf("Run(show abc) = %d, want 1; stderr: %q", code, stderr.String())
	}
	if want := "ticket: тикет «abc» не найден\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}
