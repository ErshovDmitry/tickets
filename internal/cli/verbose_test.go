package cli

// Verbose logging wiring of the CLI (T-0110, wave 3).

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// lockFileName is the store lock file (internal/store lockPath): the dir
// assertions count it, so no unexpected file can hide behind it.
const lockFileName = ".lock"

// wantEq fails unless got and want are equal.
func wantEq(t *testing.T, what, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %q, want %q", what, got, want)
	}
}

// wantNoRecords fails when the text carries a log record.
func wantNoRecords(t *testing.T, s string) {
	t.Helper()
	for _, p := range []string{"ticket: debug: ", "ticket: info: ", "ticket: warn: ", "ticket: error: "} {
		if strings.Contains(s, p) {
			t.Errorf("stderr carries a log record (%q): %q", p, s)
		}
	}
}

// runCLI runs Run and returns the exit code, stdout and stderr.
func runCLI(t *testing.T, args []string, env map[string]string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Run(args, env, &out, &errb)
	return code, out.String(), errb.String()
}

// verboseFixture returns a fresh dir with ticket 1 in status ("" = open).
func verboseFixture(t *testing.T, status string) string {
	t.Helper()
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	if code, _, stderr := runCLI(t, []string{"new", "тест"}, env); code != 0 {
		t.Fatalf("fixture new: code %d, stderr %q", code, stderr)
	}
	if status != "" {
		if code, _, stderr := runCLI(t, []string{"set", "1", status}, env); code != 0 {
			t.Fatalf("fixture set %s: code %d, stderr %q", status, code, stderr)
		}
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("evalsymlinks %s: %v", dir, err)
	}
	return resolved
}

// readFile returns the content of path.
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// dirState returns a stable description of the files of dir.
func dirState(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	var parts []string
	for _, e := range entries {
		if !e.IsDir() {
			parts = append(parts, e.Name()+"="+readFile(t, filepath.Join(dir, e.Name())))
		}
	}
	return strings.Join(parts, "\x00")
}

// wantListEvents returns the three lifecycle records of a list run.
func wantListEvents(dir string) string {
	return "ticket: info: command.start command=\"list\"\n" +
		"ticket: debug: dir.resolve source=\"TICKETS_DIR\" dir=" + strconv.Quote(dir) + "\n" +
		"ticket: info: command.done command=\"list\" code=0\n"
}

// TestVerboseActivation pins the activation, the event order and the stdout isolation.
func TestVerboseActivation(t *testing.T) {
	dir := verboseFixture(t, "")
	logPath := filepath.Join(t.TempDir(), "ticket.log")
	env := map[string]string{"TICKETS_DIR": dir}
	fileEnv := map[string]string{"TICKETS_DIR": dir, logFileEnv: logPath}
	code, offOut, offErr := runCLI(t, []string{"list"}, env)
	if code != 0 || offErr != "" {
		t.Fatalf("off-mode Run(list) = %d, stderr %q", code, offErr)
	}
	cases := []struct {
		args []string
		env  map[string]string
		want string
	}{
		{[]string{"list"}, env, ""},
		{[]string{"--verbose", "list"}, env, wantListEvents(dir)},
		{[]string{"--verbose", "--verbose", "list"}, env, wantListEvents(dir)},
		{[]string{"list"}, map[string]string{"TICKETS_DIR": dir, logFileEnv: ""}, ""},
		{[]string{"list"}, fileEnv, ""},
		{[]string{"--verbose", "list"}, fileEnv, ""},
	}
	for _, tc := range cases {
		code, out, errb := runCLI(t, tc.args, tc.env)
		if code != 0 {
			t.Fatalf("Run(%q) = %d, want 0", tc.args, code)
		}
		wantEq(t, "stdout", out, offOut)
		wantEq(t, "stderr", errb, tc.want)
	}
	// The file destination holds the records of both file runs.
	wantEq(t, "log file", readFile(t, logPath), wantListEvents(dir)+wantListEvents(dir))
	if runtime.GOOS != "windows" {
		assertPerm(t, logPath, 0o600)
	}
	// stdout isolation: show prints the raw ticket bytes.
	raw := readFile(t, filepath.Join(dir, "T-0001-open.md"))
	code, out, errb := runCLI(t, []string{"--verbose", "show", "1"}, env)
	if code != 0 {
		t.Fatalf("Run(--verbose show 1) = %d, want 0", code)
	}
	wantEq(t, "stdout", out, raw)
	if strings.Contains(errb, "тест") {
		t.Errorf("the log leaks the ticket title: %q", errb)
	}
	// -C and --verbose are consumed in any order: the flag wins over the env dir.
	wantFlag := "ticket: info: command.start command=\"list\"\n" +
		"ticket: debug: dir.resolve source=\"--tickets-dir\" dir=" + strconv.Quote(dir) + "\n" +
		"ticket: info: command.done command=\"list\" code=0\n"
	for _, args := range [][]string{
		{"-C", dir, "--verbose", "list"},
		{"--verbose", "-C", dir, "list"},
	} {
		code, out, errb := runCLI(t, args, env)
		if code != 0 {
			t.Fatalf("Run(%q) = %d, want 0", args, code)
		}
		wantEq(t, "stdout", out, offOut)
		wantEq(t, "stderr", errb, wantFlag)
	}
	// A repeated --verbose with a file destination: one logger, one file, one stream.
	repDir := t.TempDir()
	repPath := filepath.Join(repDir, "ticket.log")
	code, out, errb = runCLI(t, []string{"--verbose", "--verbose", "list"},
		map[string]string{"TICKETS_DIR": dir, logFileEnv: repPath})
	if code != 0 {
		t.Fatalf("Run(--verbose --verbose list) = %d, want 0", code)
	}
	wantEq(t, "stdout", out, offOut)
	wantEq(t, "stderr", errb, "")
	wantEq(t, "log file", readFile(t, repPath), wantListEvents(dir))
	entries, err := os.ReadDir(repDir)
	if err != nil {
		t.Fatalf("read dir %s: %v", repDir, err)
	}
	if len(entries) != 1 || entries[0].Name() != "ticket.log" {
		t.Errorf("the log dir holds %v, want exactly one ticket.log", entries)
	}
}

// TestVerboseLogOpenFailure pins the open failure of the log file.
func TestVerboseLogOpenFailure(t *testing.T) {
	dir := verboseFixture(t, "")
	bad := filepath.Join(t.TempDir(), "missing", "ticket.log")
	// The expected line is the OS error of the same open call: exact everywhere.
	_, openErr := os.OpenFile(bad, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if openErr == nil {
		t.Fatalf("the log path must not be creatable: %s", bad)
	}
	wantErr := "ticket: open log file " + bad + ": " + openErr.Error() + "\n"
	// The command is not executed: stdout empty, the error the only stderr line.
	code, out, stderr := runCLI(t, []string{"--verbose", "list"},
		map[string]string{"TICKETS_DIR": dir, logFileEnv: bad})
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	wantEq(t, "stdout", out, "")
	wantEq(t, "stderr", stderr, wantErr)
	// A creating command proves the side effect: no ticket written on open failure.
	before := dirState(t, dir)
	code, out, stderr = runCLI(t, []string{"--verbose", "new", "second"},
		map[string]string{"TICKETS_DIR": dir, logFileEnv: bad})
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	wantEq(t, "stdout", out, "")
	wantEq(t, "stderr", stderr, wantErr)
	if after := dirState(t, dir); after != before {
		t.Errorf("the open failure changed the tickets dir: before %q, after %q", before, after)
	}
}

// TestVerboseHelpAndNoCommand pins that the help interception precedes the logger.
func TestVerboseHelpAndNoCommand(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "ticket.log")
	env := map[string]string{"TICKETS_DIR": verboseFixture(t, ""), logFileEnv: logPath}
	_, usage, _ := runCLI(t, []string{"help"}, env)
	cases := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
	}{
		{"no command", []string{"--verbose"}, 0, usage},
		{"help", []string{"--verbose", "help"}, 0, usage},
		{"unknown command", []string{"--verbose=x"}, 1, usage},
		{"subcommand help", []string{"--verbose", "list", "-h"}, 0, usage},
		{"version", []string{"--verbose", "version"}, 0, versionLine()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out, stderr := runCLI(t, tc.args, env)
			if code != tc.wantCode {
				t.Fatalf("Run(%q) = %d, want %d", tc.args, code, tc.wantCode)
			}
			wantEq(t, "stdout", out, tc.wantOut)
			wantEq(t, "stderr", stderr, "")
			wantNoRecords(t, stderr)
		})
	}
	// A non-empty TICKET_LOG does not change that: help precedes logger creation.
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Errorf("the help path created the log file %s (err %v)", logPath, err)
	}
	// init ignores the switch: it runs normally and writes no record.
	work := t.TempDir()
	t.Chdir(work)
	code, out, stderr := runCLI(t, []string{"--verbose", "init"}, map[string]string{logFileEnv: logPath})
	if code != 0 {
		t.Fatalf("Run(--verbose init) = %d, want 0; stderr: %q", code, stderr)
	}
	wantNoRecords(t, stderr)
	if tickets := filepath.Join(work, "tickets"); !strings.Contains(out, tickets) {
		t.Errorf("stdout = %q, want the initialized dir %q", out, tickets)
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Errorf("the init path created the log file %s (err %v)", logPath, err)
	}
}

// TestVerboseLoggerCloseError pins the deferred close of a file destination:
// the actual *os.File Close error reaches stderr exactly once, after the
// command output, and leaves the exit code unchanged — zero or not. The seam
// is restored via t.Cleanup; the package-level override forbids t.Parallel.
func TestVerboseLoggerCloseError(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "log")
	if err != nil {
		t.Fatalf("create temp log: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close temp log: %v", err)
	}
	cerr := f.Close() // a real *os.File Close error: the file is already closed
	if cerr == nil {
		t.Fatalf("closing an already closed file must fail")
	}
	prev := newLoggerFn
	newLoggerFn = func(w io.Writer, e map[string]string, verbose bool) (*slog.Logger, func() error, error) {
		if verbose {
			return slog.New(slog.DiscardHandler), f.Close, nil
		}
		return newLogger(w, e, verbose) // the untouched off-mode logger of the probe
	}
	t.Cleanup(func() { newLoggerFn = prev })
	env := map[string]string{"TICKETS_DIR": t.TempDir()}
	for _, args := range [][]string{{"list"}, {"show", "x"}} {
		var probe, buf bytes.Buffer // tagged writers pin the stream and the write order
		wantCode := Run(args, env,
			&streamWriter{tag: "OUT:", buf: &probe}, &streamWriter{tag: "ERR:", buf: &probe})
		if wantNonzero := args[0] != "list"; (wantCode != 0) != wantNonzero {
			t.Fatalf("probe Run(%q) = %d: the post-command failure must stay nonzero", args, wantCode)
		}
		code := Run(append([]string{"--verbose"}, args...), env,
			&streamWriter{tag: "OUT:", buf: &buf}, &streamWriter{tag: "ERR:", buf: &buf})
		if code != wantCode {
			t.Fatalf("Run(%q) = %d, want %d (the exit code must not change)", args, code, wantCode)
		}
		wantEq(t, "output", buf.String(), probe.String()+"ERR:"+cerr.Error()+"\n")
	}
}
