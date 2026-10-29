package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

// logRecorder captures the emitted log bytes and can fail on write.
type logRecorder struct {
	buf bytes.Buffer
	err error
}

func (r *logRecorder) Write(p []byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	return r.buf.Write(p)
}

// seqWriter fails with successive errors from errs, then succeeds.
type seqWriter struct {
	errs []error
}

func (w *seqWriter) Write(p []byte) (int, error) {
	if len(w.errs) == 0 {
		return len(p), nil
	}
	err := w.errs[0]
	w.errs = w.errs[1:]
	return 0, err
}
func testLogger(t *testing.T) (*slog.Logger, *logRecorder, func() error) {
	t.Helper()
	rec := &logRecorder{}
	lg, closeLog, err := newLogger(rec, map[string]string{}, true)
	expectOK(t, err)
	return lg, rec, closeLog
}
func assertLog(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("output mismatch\ngot:  %q\nwant: %q", got, want)
	}
}

// expectOK fails the test when err is non-nil.
func expectOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestLogHandlerOutput pins prefixes, attr rendering, groups and escaping.
func TestLogHandlerOutput(t *testing.T) {
	lg, rec, closeLog := testLogger(t)
	ctx := context.Background()
	lg.Debug("d")
	lg.Info("i", "s", "x", "n", 7, "b", true)
	lg.Warn("w", "f", 1.5)
	lg.Error("e")
	lg.LogAttrs(ctx, slog.LevelInfo, "z", slog.Attr{}, slog.Int("", 7), slog.Any("any", []int{1}))
	lg.Info("esc", "key", "a\"b\nc")
	lg.Info("dur", slog.Duration("timeout", 5*time.Second))
	lg.WithGroup("g").With("timeout", 5*time.Second).Info("gdur")
	lg.With("a", 1).WithGroup("g").With("b", 2).Info("ord", "c", 3)
	lg.Info("grp", slog.Group("", slog.Int("x", 1)), slog.Group("k"), slog.Group("g", slog.Int("y", 2)))
	lg.LogAttrs(ctx, slog.LevelInfo, "", slog.String("e", ""), slog.Time("t", time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)))
	lg.Info("a\nb")
	lg.With("k\nv", 1).Info("e")
	lg.WithGroup("g h").With("x", 2).Info("e")
	expectOK(t, closeLog())
	assertLog(t, rec.buf.String(), "ticket: debug: d\n"+
		"ticket: info: i s=\"x\" n=7 b=true\n"+
		"ticket: warn: w f=1.5\n"+
		"ticket: error: e\n"+
		"ticket: info: z \"\"=7 any=\"[1]\"\n"+
		"ticket: info: esc key=\"a\\\"b\\nc\"\n"+
		"ticket: info: dur timeout=5s\n"+
		"ticket: info: gdur g.timeout=5s\n"+
		"ticket: info: ord a=1 g.b=2 g.c=3\n"+
		"ticket: info: grp x=1 g.y=2\n"+
		"ticket: info: \"\" e=\"\" t=\"2026-10-29 12:00:00 +0000 UTC\"\n"+
		"ticket: info: \"a\\nb\"\n"+
		"ticket: info: e \"k\\nv\"=1\n"+
		"ticket: info: e \"g h.x\"=2\n")
}

// TestLogHandlerLevels pins admitted levels, boundary offsets and the no-op.
func TestLogHandlerLevels(t *testing.T) {
	lg, rec, _ := testLogger(t)
	h, ctx := lg.Handler(), context.Background()
	for _, l := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
		if !h.Enabled(ctx, l) {
			t.Fatalf("level %v must be enabled", l)
		}
	}
	for _, l := range []slog.Level{slog.LevelDebug - 1, slog.LevelInfo + 1, slog.LevelWarn + 1, slog.LevelError + 1, slog.Level(9)} {
		if h.Enabled(ctx, l) {
			t.Fatalf("level %v must be disabled", l)
		}
		lg.Log(ctx, l, "x")
	}
	assertLog(t, rec.buf.String(), "")
	if h.WithGroup("") != h {
		t.Fatal("WithGroup(\"\") must return the receiver")
	}
	// An unset or explicitly empty TICKET_LOG stays silent without verbose.
	for _, env := range []map[string]string{{}, {logFileEnv: ""}} {
		dlg, dclose, err := newLogger(rec, env, false)
		expectOK(t, err)
		dlg.Debug("d")
		dlg.Error("e")
		expectOK(t, dclose())
		assertLog(t, rec.buf.String(), "")
	}
	elg, eclose, err := newLogger(rec, map[string]string{logFileEnv: ""}, true)
	expectOK(t, err)
	elg.Info("stderr")
	expectOK(t, eclose())
	assertLog(t, rec.buf.String(), "ticket: info: stderr\n")
}

// TestLogHandlerWithAttrsOwnership pins the receiver return, a new handler for
// non-empty attrs and ownership of the caller's attribute slice.
func TestLogHandlerWithAttrsOwnership(t *testing.T) {
	lg, rec, _ := testLogger(t)
	h := lg.Handler()
	if h.WithAttrs(nil) != h || h.WithAttrs([]slog.Attr{}) != h {
		t.Fatal("WithAttrs with no attributes must return the receiver")
	}
	attrs := []slog.Attr{slog.String("k", "v")}
	derived := h.WithAttrs(attrs)
	if derived == h {
		t.Fatal("WithAttrs must return a new handler")
	}
	attrs[0] = slog.String("k", "mutated")
	slog.New(derived).Info("d")
	lg.Info("p")
	assertLog(t, rec.buf.String(), "ticket: info: d k=\"v\"\nticket: info: p\n")
}

// TestNewLoggerFileLifecycle pins failed open, owner-only creation, appended
// content, mode preservation and the exact close/write error wraps.
func TestNewLoggerFileLifecycle(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing", "ticket.log")
	lg, closeLog, err := newLogger(&logRecorder{}, map[string]string{logFileEnv: missing}, true)
	if err == nil || lg != nil || closeLog != nil {
		t.Fatalf("want only an open error, got err=%v logger=%v closeLog set=%v", err, lg, closeLog != nil)
	}
	if !strings.Contains(err.Error(), "ticket: open log file "+missing+": ") {
		t.Fatalf("error must name the path, got %q", err)
	}
	if _, statErr := os.Stat(missing); statErr == nil {
		t.Fatal("log file must not be created")
	}
	path := filepath.Join(t.TempDir(), "ticket.log")
	env := map[string]string{logFileEnv: path}
	lg, closeLog, err = newLogger(io.Discard, env, false)
	expectOK(t, err)
	lg.Info("first")
	expectOK(t, closeLog())
	if runtime.GOOS != "windows" {
		assertPerm(t, path, 0o600)
		if err := os.Chmod(path, 0o640); err != nil {
			t.Fatalf("chmod: %v", err)
		}
	}
	lg, closeLog, err = newLogger(io.Discard, env, true)
	expectOK(t, err)
	lg.Info("second")
	expectOK(t, closeLog())
	data, err := os.ReadFile(path)
	expectOK(t, err)
	if want := "ticket: info: first\nticket: info: second\n"; string(data) != want {
		t.Fatalf("log content = %q, want %q", data, want)
	}
	if runtime.GOOS != "windows" {
		assertPerm(t, path, 0o640)
	}
	boom := errors.New("boom")
	lg, closeLog, err = newLogger(&logRecorder{err: boom}, map[string]string{}, true)
	expectOK(t, err)
	lg.Info("e")
	err = closeLog()
	if err == nil || !strings.Contains(err.Error(), "ticket: log write: ") || !errors.Is(err, boom) {
		t.Fatalf("close error = %v, want wrapped ticket: log write: boom", err)
	}
	first, second := errors.New("first"), errors.New("second")
	lg, closeLog, err = newLogger(&seqWriter{errs: []error{first, second}}, map[string]string{}, true)
	expectOK(t, err)
	lg.With("k", 1).Info("e")
	lg.Info("e")
	if err := closeLog(); !errors.Is(err, first) {
		t.Fatalf("close error = %v, want the first write error", err)
	}
	_, closeLog, f, path := fileLogger(t)
	expectOK(t, f.Close())
	want := "ticket: close log file " + path + ": "
	err = closeLog()
	if err == nil || !strings.Contains(err.Error(), want) || !errors.Is(err, os.ErrClosed) {
		t.Fatalf("close error = %v, want %q wrapping ErrClosed", err, want)
	}
	if strings.Contains(err.Error(), "ticket: log write: ") {
		t.Fatalf("close-only path must not report a write error: %v", err)
	}
	lg, closeLog, f, path = fileLogger(t)
	expectOK(t, f.Close())
	lg.Info("e")
	err = closeLog()
	if err == nil {
		t.Fatal("want a joined error")
	}
	for _, want := range []string{"ticket: log write: ", "ticket: close log file " + path + ": "} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %v must contain %q", err, want)
		}
	}
	if !errors.Is(err, os.ErrClosed) {
		t.Fatalf("error %v must wrap ErrClosed", err)
	}
}
func assertPerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	fi, err := os.Stat(path)
	expectOK(t, err)
	if mode := fi.Mode().Perm(); mode != want {
		t.Fatalf("log mode = %o, want %o", mode, want)
	}
}

// fileLogger exposes the *os.File of a file destination to force failures.
func fileLogger(t *testing.T) (*slog.Logger, func() error, *os.File, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ticket.log")
	lg, closeLog, err := newLogger(io.Discard, map[string]string{logFileEnv: path}, true)
	expectOK(t, err)
	h := lg.Handler().(*logHandler)
	f := h.w.(*os.File)
	return lg, closeLog, f, path
}

// TestNewLoggerConcurrentWrites pins whole, non-interleaved lines.
func TestNewLoggerConcurrentWrites(t *testing.T) {
	const workers = 16
	lg, rec, closeLog := testLogger(t)
	derived := lg.With("shared", true)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			derived.Info("e", "worker", i)
		}(i)
	}
	wg.Wait()
	expectOK(t, closeLog())
	lines := strings.Split(strings.TrimSuffix(rec.buf.String(), "\n"), "\n")
	want := make([]string, 0, workers)
	for i := range workers {
		want = append(want, "ticket: info: e shared=true worker="+strconv.Itoa(i))
	}
	slices.Sort(lines)
	slices.Sort(want)
	if !slices.Equal(lines, want) {
		t.Fatalf("lines = %q, want %q (interleaved?)", lines, want)
	}
}

// TestRenderTokenUnicode pins quoting of Unicode and invalid UTF-8 bytes.
func TestRenderTokenUnicode(t *testing.T) {
	for in, want := range map[string]string{
		"ключ": "ключ", "a\u00a0b": `"a\u00a0b"`, "z\u200b": `"z\u200b"`, "bad\xff": `"bad\xff"`,
	} {
		if got := renderToken(in); got != want {
			t.Fatalf("renderToken(%q) = %q, want %q", in, got, want)
		}
	}
	lg, rec, closeLog := testLogger(t)
	lg.Info("e\xff", "k\xff", 1)
	expectOK(t, closeLog())
	if !utf8.ValidString(rec.buf.String()) {
		t.Fatalf("output must be valid UTF-8: %q", rec.buf.Bytes())
	}
}
