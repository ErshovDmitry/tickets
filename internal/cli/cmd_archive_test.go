package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/cli"
)

// newTicketDir returns a Run env plus dir pre-seeded with ticket 1 (open).
func newTicketDir(t *testing.T) (map[string]string, string) {
	t.Helper()
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	createTestTicket(t, dir)
	return env, dir
}

// runSet transitions ticket n via `set`, failing t on any non-zero exit.
func runSet(t *testing.T, env map[string]string, n, st string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"set", n, st}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(set %s %s) = %d, want 0; stderr: %q", n, st, code, stderr.String())
	}
}

// runArchive executes `archive [args...]` and returns the exit code,
// stdout and stderr.
func runArchive(env map[string]string, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := cli.Run(append([]string{"archive"}, args...), env, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// TestCmdArchiveOnePrintsTargetPath pins A24: archiving a done ticket
// prints the REAL archive target path (bash echo "$target").
func TestCmdArchiveOnePrintsTargetPath(t *testing.T) {
	env, dir := newTicketDir(t)
	runSet(t, env, "1", "done")
	code, stdout, stderr := runArchive(env, "1")
	if code != 0 {
		t.Fatalf("archive 1 = %d, want 0; stderr: %q", code, stderr)
	}
	want := filepath.Join(dir, "archive", "T-0001-done.md") + "\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "archive", "T-0001-done.md")); err != nil {
		t.Errorf("archived file missing: %v", err)
	}
}

// TestCmdArchiveErrorsBashContract pins A26: the exact bash error texts
// and rc=1. Exact string equality enforces the single «ticket: » prefix —
// a doubled "ticket: ticket:" cannot match (cycle-1 regression).
func TestCmdArchiveErrorsBashContract(t *testing.T) {
	t.Run("open rejected", func(t *testing.T) {
		env, _ := newTicketDir(t)
		code, stdout, stderr := runArchive(env, "1")
		if code != 1 {
			t.Fatalf("archive open = %d, want 1", code)
		}
		if want := "ticket: архивировать можно только done/closed (сейчас: open)\n"; stderr != want {
			t.Errorf("stderr = %q, want %q", stderr, want)
		}
		if stdout != "" {
			t.Errorf("stdout = %q, want empty", stdout)
		}
	})
	t.Run("not found", func(t *testing.T) {
		env, _ := newTicketDir(t)
		code, _, stderr := runArchive(env, "99")
		if code != 1 {
			t.Fatalf("archive 99 = %d, want 1", code)
		}
		if want := "ticket: тикет «99» не найден\n"; stderr != want {
			t.Errorf("stderr = %q, want %q", stderr, want)
		}
	})
	t.Run("already archived", func(t *testing.T) {
		env, _ := newTicketDir(t)
		runSet(t, env, "1", "done")
		if code, _, _ := runArchive(env, "1"); code != 0 {
			t.Fatalf("first archive = %d, want 0", code)
		}
		code, _, stderr := runArchive(env, "1")
		if code != 1 {
			t.Fatalf("second archive = %d, want 1", code)
		}
		if want := "ticket: тикет уже в архиве\n"; stderr != want {
			t.Errorf("stderr = %q, want %q", stderr, want)
		}
	})
}

// TestCmdArchiveCollisionKeepsTarget pins A26: archiving over an existing
// archive file fails with rc=1, the bash wording naming the real target;
// the foreign file's content survives untouched (FOREIGN-CONTENT-KEEP)
// and the original stays in the main directory (no-replace).
func TestCmdArchiveCollisionKeepsTarget(t *testing.T) {
	env, dir := newTicketDir(t)
	runSet(t, env, "1", "done")
	archiveDir := filepath.Join(dir, "archive")
	target := filepath.Join(archiveDir, "T-0001-done.md")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := []byte("FOREIGN-CONTENT-KEEP")
	if err := os.WriteFile(target, foreign, 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runArchive(env, "1")
	if code != 1 {
		t.Fatalf("archive over collision = %d, want 1", code)
	}
	if want := "ticket: файл " + target + " уже существует\n"; stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if got, rerr := os.ReadFile(target); rerr != nil || !bytes.Equal(got, foreign) {
		t.Errorf("foreign target content modified: %q (err=%v)", got, rerr)
	}
	if _, err := os.Stat(filepath.Join(dir, "T-0001-done.md")); err != nil {
		t.Errorf("original must remain in main: %v", err)
	}
}

// TestCmdArchiveAll pins A25: without an argument every done/closed
// ticket moves (paths echoed in number order, open ones stay); zero
// closed tickets is a success (rc=0) printing the pinned notice.
func TestCmdArchiveAll(t *testing.T) {
	t.Run("moves closed in number order", func(t *testing.T) {
		env, dir := newTicketDir(t)
		var newOut, newErr bytes.Buffer
		for i := 0; i < 2; i++ {
			args := []string{"new", "Ещё тикет", "-t", "BUG", "-w", "тестер"}
			if code := cli.Run(args, env, &newOut, &newErr); code != 0 {
				t.Fatalf("Run(new #%d) = %d; stderr: %q", i+2, code, newErr.String())
			}
		}
		runSet(t, env, "1", "done")
		runSet(t, env, "2", "closed")
		code, out, stderr := runArchive(env)
		if code != 0 {
			t.Fatalf("archive = %d, want 0; stderr: %q", code, stderr)
		}
		want := filepath.Join(dir, "archive", "T-0001-done.md") + "\n" +
			filepath.Join(dir, "archive", "T-0002-closed.md") + "\n"
		if out != want {
			t.Errorf("stdout = %q, want %q", out, want)
		}
		if _, err := os.Stat(filepath.Join(dir, "T-0003-open.md")); err != nil {
			t.Errorf("open ticket must remain in main: %v", err)
		}
	})
	t.Run("no closed tickets is success", func(t *testing.T) {
		env, _ := newTicketDir(t) // ticket 1 stays open
		code, stdout, stderr := runArchive(env)
		if code != 0 {
			t.Fatalf("archive = %d, want 0; stderr: %q", code, stderr)
		}
		if want := "Нет закрытых тикетов для архивации.\n"; stdout != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
	})
}

// TestCmdSetArchivedStaysInArchive pins A30 via the CLI: `set <n> closed`
// on an archived done ticket rewrites the file in place under archive/
// and prints the archive path (not a path derived from st.Dir).
func TestCmdSetArchivedStaysInArchive(t *testing.T) {
	env, dir := newTicketDir(t)
	runSet(t, env, "1", "done")
	if code, _, stderr := runArchive(env, "1"); code != 0 {
		t.Fatalf("archive = %d; stderr: %q", code, stderr)
	}
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"set", "1", "closed"}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("set closed = %d, want 0; stderr: %q", code, stderr.String())
	}
	want := filepath.Join(dir, "archive", "T-0001-closed.md") + "\n"
	if got := stdout.String(); got != want {
		t.Errorf("stdout = %q, want %q (archive path, not st.Dir)", got, want)
	}
	if _, err := os.Stat(want[:len(want)-1]); err != nil {
		t.Errorf("archived closed file missing: %v", err)
	}
}

// TestCmdListArchiveFilter pins A27: archived tickets appear only under
// the archive filter — list/all/done buckets must not show them; an empty
// archive prints the pinned empty line naming the filter.
func TestCmdListArchiveFilter(t *testing.T) {
	env, _ := newTicketDir(t)
	runSet(t, env, "1", "done")
	if code, _, _ := runArchive(env, "1"); code != 0 {
		t.Fatal("archive setup failed")
	}
	for _, filter := range []string{"active", "all", "done"} {
		var stdout, stderr bytes.Buffer
		if code := cli.Run([]string{"list", filter}, env, &stdout, &stderr); code != 0 {
			t.Fatalf("list %s = %d; stderr: %q", filter, code, stderr.String())
		}
		if out := stdout.String(); strings.Contains(out, "T-0001") {
			t.Errorf("list %s leaked the archived ticket: %q", filter, out)
		}
	}
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"list", "archive"}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("list archive = %d; stderr: %q", code, stderr.String())
	}
	if out := stdout.String(); !strings.Contains(out, "T-0001") || !strings.Contains(out, "done") {
		t.Errorf("list archive output missing archived ticket: %q", out)
	}
	stdout.Reset()
	if code := cli.Run([]string{"list", "archive"}, testEnv(t), &stdout, &stderr); code != 0 {
		t.Fatalf("list archive empty = %d", code)
	}
	if want := "Нет тикетов (archive).\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

// TestUsageArchiveKeyLines compares the embedded usage fixture with the
// bash usage() contract (tickets/bin/ticket:14–37) on the lines the
// archive feature added or touched: the list/show/set/archive blocks.
func TestUsageArchiveKeyLines(t *testing.T) {
	usage := string(usageFixture)
	for _, want := range []string{
		"ticket list [active|open|wip|done|closed|archive|all]",
		"archive — закрытые тикеты, унесённые в архив",
		"ticket show <номер|имя-файла>",
		"показать тикет (ищет и в архиве)",
		"ticket set <номер> <статус> [\"комментарий\"]",
		"перевод архивного тикета в open/wip возвращает его из архива в работу",
		"ticket archive [<номер>]",
		"перенести закрытые тикеты (done/closed) в archive/;",
		"без номера — все закрытые, с номером — один указанный",
	} {
		if !strings.Contains(usage, want) {
			t.Errorf("usage fixture missing bash usage() line: %q", want)
		}
	}
}
