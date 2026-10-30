package cli

// Verbose logging wiring of the CLI (T-0110, wave 3): the post-command
// rows, the lifecycle error events and the deferred close of the logger.

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// streamWriter appends tagged writes to a shared buffer and can fail the
// n-th write, so a test can prove which stream a line went to and the
// order of the lines.
type streamWriter struct {
	tag    string
	buf    *bytes.Buffer
	failAt int
	err    error
	calls  int
}

func (w *streamWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls == w.failAt {
		return 0, w.err
	}
	return w.buf.WriteString(w.tag + string(p))
}

// TestVerbosePostCommandRows covers the ten post-command rows, each in a
// fresh isolated fixture with its own ticket 1.
func TestVerbosePostCommandRows(t *testing.T) {
	cases := []struct {
		name     string
		status   string
		args     []string
		wantCode int
		wantOut  func(t *testing.T, dir string) string
		wantErr  string
		check    func(t *testing.T, dir string)
	}{
		{
			name: "show extra args", status: "", args: []string{"show", "1", "--verbose"}, wantCode: 0,
			wantOut: func(t *testing.T, dir string) string {
				return readFile(t, filepath.Join(dir, "T-0001-open.md"))
			},
			wantErr: "ticket: предупреждение: лишние аргументы игнорируются: --verbose\n",
		},
		{
			name: "set comment", status: "", args: []string{"set", "1", "wip", "--verbose"}, wantCode: 0,
			wantOut: func(t *testing.T, dir string) string {
				return filepath.Join(dir, "T-0001-wip.md") + "\n"
			},
			check: func(t *testing.T, dir string) {
				if _, err := os.Stat(filepath.Join(dir, "T-0001-open.md")); !os.IsNotExist(err) {
					t.Errorf("the old ticket file is still there (err %v)", err)
				}
				if !strings.Contains(readFile(t, filepath.Join(dir, "T-0001-wip.md")), "--verbose") {
					t.Errorf("the journal does not carry the --verbose comment")
				}
				// No other file: a log must never land in the tickets dir.
				wantEq(t, "tickets dir", dirState(t, dir), lockFileName+"=\x00T-0001-wip.md="+readFile(t, filepath.Join(dir, "T-0001-wip.md")))
			},
		},
		{
			name: "archive done", status: "done", args: []string{"archive", "1", "--verbose"}, wantCode: 0,
			wantOut: func(t *testing.T, dir string) string {
				return filepath.Join(dir, "archive", "T-0001-done.md") + "\n"
			},
			check: func(t *testing.T, dir string) { wantArchived(t, dir, "T-0001-done.md") },
		},
		{
			name: "archive closed", status: "closed", args: []string{"archive", "1", "--verbose"}, wantCode: 0,
			wantOut: func(t *testing.T, dir string) string {
				return filepath.Join(dir, "archive", "T-0001-closed.md") + "\n"
			},
			check: func(t *testing.T, dir string) { wantArchived(t, dir, "T-0001-closed.md") },
		},
		{
			name: "show non numeric", status: "", args: []string{"show", "--verbose"}, wantCode: 1,
			wantErr: "ticket: «--verbose» не является номером тикета (нужно целое число)\n",
		},
		{
			name: "set unknown flag", status: "", args: []string{"set", "1", "--verbose"}, wantCode: 1,
			wantErr: "ticket: неизвестный флаг для команды set: --verbose\n",
		},
		{
			name: "archive not found", status: "", args: []string{"archive", "--verbose"}, wantCode: 1,
			wantErr: "ticket: тикет «--verbose» не найден\n",
		},
		{
			name: "list unknown flag", status: "", args: []string{"list", "--verbose"}, wantCode: 1,
			wantErr: "ticket: неизвестный флаг: --verbose\n",
		},
		{
			name: "new unknown argument", status: "", args: []string{"new", "t", "--verbose"}, wantCode: 1,
			wantErr: "ticket: неизвестный аргумент: --verbose\n",
		},
		{
			name: "new title dash", status: "", args: []string{"new", "--verbose", "t"}, wantCode: 1,
			wantErr: "ticket: краткое описание не может начинаться с '-'\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := verboseFixture(t, tc.status)
			env := map[string]string{"TICKETS_DIR": dir}
			before := "" // check rows verify the dir themselves: no snapshot
			if tc.check == nil {
				before = dirState(t, dir)
			}
			code, out, stderr := runCLI(t, tc.args, env)
			if code != tc.wantCode {
				t.Fatalf("Run(%q) = %d, want %d; stderr: %q", tc.args, code, tc.wantCode, stderr)
			}
			wantNoRecords(t, stderr)
			wantEq(t, "stderr", stderr, tc.wantErr)
			if tc.wantOut != nil {
				wantEq(t, "stdout", out, tc.wantOut(t, dir))
			} else {
				wantEq(t, "stdout", out, "")
			}
			if tc.check != nil {
				tc.check(t, dir)
				return
			}
			if after := dirState(t, dir); after != before {
				t.Errorf("the command changed the tickets dir: before %q, after %q", before, after)
			}
		})
	}
}

// wantArchived pins the archive move of name: the source file is gone, the
// archive copy is there, and no other file — a log included — appeared.
func wantArchived(t *testing.T, dir, name string) {
	t.Helper()
	src, dst := filepath.Join(dir, name), filepath.Join(dir, "archive", name)
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("the source ticket file is still there (err %v)", err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Errorf("the archived ticket file is missing: %v", err)
	}
	wantEq(t, "tickets dir", dirState(t, dir), lockFileName+"=")
	wantEq(t, "archive dir", dirState(t, filepath.Join(dir, "archive")), name+"="+readFile(t, dst))
}

// TestVerboseErrorEvents pins the failure events and their order: a failed
// dir resolve emits dir.resolve.fail and no dir.resolve, a failed store open
// emits store.open, and neither of them emits command.done.
func TestVerboseErrorEvents(t *testing.T) {
	// A missing tickets dir: the resolve fails before the store is opened.
	missing := filepath.Join(t.TempDir(), "missing")
	code, out, stderr := runCLI(t, []string{"--verbose", "list"}, map[string]string{"TICKETS_DIR": missing})
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	wantEq(t, "stdout", out, "")
	abs, err := filepath.Abs(missing)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	wantEq(t, "stderr", stderr,
		"ticket: info: command.start command=\"list\"\n"+
			"ticket: error: dir.resolve.fail source=\"TICKETS_DIR\"\n"+
			"invalid tickets dir (TICKETS_DIR): path does not exist: "+abs+"\n")
	// A tickets dir that cannot be written: the mutable store's write probe
	// fails, so the command never runs. POSIX keeps this store.open probe
	// coverage; Windows lacks it, because chmod cannot make a dir unwritable.
	if runtime.GOOS == "windows" {
		t.Skip("chmod cannot make a directory unwritable on Windows: no store.open probe coverage")
	}
	dir := verboseFixture(t, "")
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod %s: %v", dir, err)
	}
	defer func() { _ = os.Chmod(dir, 0o755) }()
	before := dirState(t, dir)
	code, out, stderr = runCLI(t, []string{"--verbose", "new", "x"}, map[string]string{"TICKETS_DIR": dir})
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	wantEq(t, "stdout", out, "")
	wantEvents := "ticket: info: command.start command=\"new\"\n" +
		"ticket: debug: dir.resolve source=\"TICKETS_DIR\" dir=" + strconv.Quote(dir) + "\n" +
		"ticket: error: store.open dir=" + strconv.Quote(dir) + "\n"
	// Only the stable probe-error prefix is pinned; command.done must be absent.
	if rest := strings.TrimPrefix(stderr, wantEvents); !strings.HasPrefix(stderr, wantEvents) ||
		!strings.HasPrefix(rest, "store: probe write ") || strings.Contains(stderr, "command.done") {
		t.Errorf("stderr = %q, want the events %q, the store probe error and no command.done", stderr, wantEvents)
	}
	if after := dirState(t, dir); after != before {
		t.Errorf("the store failure changed the tickets dir: before %q, after %q", before, after)
	}
}

// TestVerboseWhoNewline pins the early exit on a CR/LF in TICKET_WHO: no
// command.done, but command.start and dir.resolve are still emitted.
func TestVerboseWhoNewline(t *testing.T) {
	for _, who := range []string{"bad\nname", "bad\rname"} {
		t.Run(who, func(t *testing.T) {
			dir := verboseFixture(t, "")
			env := map[string]string{"TICKETS_DIR": dir, "TICKET_WHO": who}
			before := dirState(t, dir)
			code, out, stderr := runCLI(t, []string{"--verbose", "new", "x"}, env)
			if code != 1 {
				t.Fatalf("Run = %d, want 1", code)
			}
			wantEq(t, "stdout", out, "")
			wantEq(t, "stderr", stderr,
				"ticket: info: command.start command=\"new\"\n"+
					"ticket: debug: dir.resolve source=\"TICKETS_DIR\" dir="+strconv.Quote(dir)+"\n"+
					"ticket: комментарий/автор не может содержать перевод строки\n")
			if after := dirState(t, dir); after != before {
				t.Errorf("the early exit changed the tickets dir: before %q, after %q", before, after)
			}
		})
	}
}

// TestVerboseDeferredWriteError pins a write error surfaced by the deferred
// close: it goes to stderr AFTER the command output and the exit code of the
// command does not change. The actual file-close error is pinned separately by
// TestVerboseLoggerCloseError.
func TestVerboseDeferredWriteError(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir()) // production logs the resolved path
	if err != nil {
		t.Fatalf("evalsymlinks: %v", err)
	}
	env := map[string]string{"TICKETS_DIR": dir}
	var buf bytes.Buffer
	out := &streamWriter{tag: "OUT:", buf: &buf}
	errw := &streamWriter{tag: "ERR:", buf: &buf, failAt: 3, err: errors.New("write failed")}
	if code := Run([]string{"--verbose", "list"}, env, out, errw); code != 0 {
		t.Fatalf("Run = %d, want 0 (the exit code must not change)", code)
	}
	want := "ERR:ticket: info: command.start command=\"list\"\n" +
		"ERR:ticket: debug: dir.resolve source=\"TICKETS_DIR\" dir=" + strconv.Quote(dir) + "\n" +
		"OUT:Нет тикетов (active).\n" +
		"ERR:ticket: log write: write failed\n"
	wantEq(t, "output", buf.String(), want)
}

// TestVerboseBlindDirValue pins the blind -C value: the next token is taken
// as the DIR value, so the switch is not consumed and the logger stays off.
func TestVerboseBlindDirValue(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	_, usage, _ := runCLI(t, []string{"help"}, env)
	logPath := filepath.Join(t.TempDir(), "ticket.log")
	logEnv := map[string]string{"TICKETS_DIR": dir, logFileEnv: logPath}

	// No command: the help interception precedes the logger, so even a
	// non-empty TICKET_LOG leaves no log file behind.
	code, out, stderr := runCLI(t, []string{"-C", "--verbose"}, logEnv)
	if code != 0 {
		t.Fatalf("Run(-C --verbose) = %d, want 0; stderr: %q", code, stderr)
	}
	wantEq(t, "stdout", out, usage)
	wantEq(t, "stderr", stderr, "")
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Errorf("the help path created the log file %s (err %v)", logPath, err)
	}

	// With a command the blind value is resolved as a dir and fails.
	code, out, stderr = runCLI(t, []string{"-C", "--verbose", "list"}, logEnv)
	if code != 1 {
		t.Fatalf("Run(-C --verbose list) = %d, want 1", code)
	}
	wantEq(t, "stdout", out, "")
	abs, err := filepath.Abs("--verbose")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	wantErr := "invalid tickets dir (--tickets-dir): path does not exist: " + abs + "\n"
	wantEq(t, "stderr", stderr, wantErr)
	// The logger is on because TICKET_LOG asks for it, but the switch was
	// not consumed: only the lifecycle of the failed resolve is written.
	wantEq(t, "log file", readFile(t, logPath),
		"ticket: info: command.start command=\"list\"\n"+
			"ticket: error: dir.resolve.fail source=\"--tickets-dir\"\n")

	// Without TICKET_LOG the blind value leaves no logger and no file.
	code, out, stderr = runCLI(t, []string{"-C", "--verbose", "list"}, env)
	if code != 1 {
		t.Fatalf("Run(-C --verbose list) = %d, want 1", code)
	}
	wantEq(t, "stdout", out, "")
	wantEq(t, "stderr", stderr, wantErr)
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Errorf("the tickets dir holds %v (err %v), want none", entries, err)
	}
}
