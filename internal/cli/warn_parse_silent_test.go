package cli

// Structured logging of store parse warnings (T-0110, wave 6): the negative
// side — logging off stays byte-identical, the log never carries ticket
// content, and the deliberately silent sites stay silent. The producer tests
// live in warn_parse_test.go.

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/store"
)

// TestWarnLoggingOff pins the off-mode contract: stdout and stderr are
// byte-identical to the TICKET_LOG run and no warn record reaches stderr.
func TestWarnLoggingOff(t *testing.T) {
	dir := verboseFixture(t, "")
	if err := os.WriteFile(filepath.Join(dir, "stray.txt"), []byte("stray"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"TICKETS_DIR": dir}
	codeOff, offOut, offErr := runCLI(t, []string{"list"}, env)
	codeOn, onOut, onErr, logText := runWithLog(t, []string{"list"}, env)
	if codeOff != codeOn || offOut != onOut {
		t.Fatalf("logging changed the output: off %d %q, on %d %q", codeOff, offOut, codeOn, onOut)
	}
	wantEq(t, "stderr", onErr, offErr)
	if len(warnLines(logText)) != 1 {
		t.Fatalf("log = %q, want exactly one warn record", logText)
	}
	wantNoRecords(t, offErr)
}

// TestWarnPrivacy pins the no-content rule: the sentinel title, details,
// comment and author never appear in the log, and neither does the TICKET_LOG
// path taken from the environment, while the log itself is not empty.
func TestWarnPrivacy(t *testing.T) {
	const (
		titleSentinel   = "SENTINEL-TITLE"
		detailsSentinel = "SENTINEL-DETAILS"
		commentSentinel = "SENTINEL-COMMENT"
		whoSentinel     = "SENTINEL-WHO"
	)
	dir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "ticket.log")
	env := map[string]string{"TICKETS_DIR": dir, "TICKET_WHO": whoSentinel, logFileEnv: logPath}
	if code, _, stderr := runCLI(t, []string{"new", titleSentinel, "-d", detailsSentinel}, env); code != 0 {
		t.Fatalf("fixture new = %d, want 0; stderr %q", code, stderr)
	}
	if code, _, stderr := runCLI(t, []string{"set", "1", "wip", commentSentinel}, env); code != 0 {
		t.Fatalf("fixture set = %d, want 0; stderr %q", code, stderr)
	}
	// The fixture really carries every sentinel, so the absence checks below
	// cannot pass vacuously.
	body := readFile(t, filepath.Join(dir, "T-0001-wip.md"))
	for _, sentinel := range []string{titleSentinel, detailsSentinel, commentSentinel, whoSentinel} {
		if !strings.Contains(body, sentinel) {
			t.Fatalf("ticket body does not carry %q: %q", sentinel, body)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "stray.txt"), []byte("stray"), 0o644); err != nil {
		t.Fatal(err)
	}
	// wantCleanLog reads the accumulated log file and fails on any leak in it.
	wantCleanLog := func() string {
		t.Helper()
		logText := readFile(t, logPath)
		if !strings.Contains(logText, "ticket: info: command.start") {
			t.Errorf("the runs were not logged: %q", logText)
		}
		for _, sentinel := range []string{titleSentinel, detailsSentinel, commentSentinel, whoSentinel, logPath} {
			if strings.Contains(logText, sentinel) {
				t.Errorf("the log leaks %q: %q", sentinel, logText)
			}
		}
		return logText
	}
	if code, _, stderr := runCLI(t, []string{"list"}, env); code != 0 {
		t.Fatalf("list = %d, want 0; stderr %q", code, stderr)
	}
	if listLog := wantCleanLog(); len(warnLines(listLog)) != 1 {
		t.Fatalf("log = %q, want one warn record", listLog)
	}
	if code, _, stderr := runCLI(t, []string{"set", "1", "closed", commentSentinel}, env); code != 0 {
		t.Fatalf("set = %d, want 0; stderr %q", code, stderr)
	}
	wantCleanLog()
}

// TestWarnSilentSites pins the sites that deliberately log nothing even when
// their store reads report warnings.
func TestWarnSilentSites(t *testing.T) {
	t.Run("show scanMax", func(t *testing.T) {
		dir := verboseFixture(t, "")
		if err := os.WriteFile(filepath.Join(dir, "stray.txt"), []byte("stray"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, _, errb, logText := runWithLog(t, []string{"show", "99"}, map[string]string{"TICKETS_DIR": dir})
		if code != 1 {
			t.Fatalf("show 99 = %d, want 1; stderr %q", code, errb)
		}
		if !strings.Contains(errb, "не найден") {
			t.Errorf("human not-found report missing: %q", errb)
		}
		wantWarnRecords(t, logText)
	})
	t.Run("new collisionPath", func(t *testing.T) {
		// ErrCollision needs a racing creator, so the silent producer is probed
		// at its own boundary with the same store/logger wiring the command has.
		dir := verboseFixture(t, "")
		if err := os.WriteFile(filepath.Join(dir, "stray.txt"), []byte("stray"), 0o644); err != nil {
			t.Fatal(err)
		}
		logPath := filepath.Join(t.TempDir(), "ticket.log")
		lg, closeLog, err := newLogger(io.Discard, map[string]string{logFileEnv: logPath}, false)
		if err != nil {
			t.Fatalf("newLogger: %v", err)
		}
		defer func() { _ = closeLog() }()
		st, err := store.New(dir, store.WithLogger(lg))
		if err != nil {
			t.Fatalf("store.New: %v", err)
		}
		if got, want := collisionPath(st), filepath.Join(dir, "T-0002-open.md"); got != want {
			t.Errorf("collisionPath = %q, want %q", got, want)
		}
		wantEq(t, "log", readFile(t, logPath), "")
	})
	t.Run("single-ticket archive", func(t *testing.T) {
		dir := verboseFixture(t, "done")
		if err := os.WriteFile(filepath.Join(dir, "stray.txt"), []byte("stray"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, out, errb, logText := runWithLog(t, []string{"archive", "1"}, map[string]string{"TICKETS_DIR": dir})
		if code != 0 {
			t.Fatalf("archive 1 = %d, want 0; stderr %q", code, errb)
		}
		if want := filepath.Join(dir, "archive", "T-0001-done.md") + "\n"; out != want {
			t.Errorf("stdout = %q, want %q", out, want)
		}
		wantWarnRecords(t, logText)
	})
}
