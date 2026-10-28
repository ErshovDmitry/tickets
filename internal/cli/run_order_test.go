package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"ticket/internal/domain"
)

// withRunSeams swaps the Run seams for the duration of one test. These tests
// must not run in parallel: the seams are package-level state.
func withRunSeams(t *testing.T, initFn func() error, langFn func(map[string]string) domain.Lang) {
	t.Helper()
	prevInit, prevLang := domainInitFn, langFromFn
	domainInitFn, langFromFn = initFn, langFn
	t.Cleanup(func() { domainInitFn, langFromFn = prevInit, prevLang })
}

// runFailWriter fails every write.
type runFailWriter struct{ err error }

func (w runFailWriter) Write([]byte) (int, error) { return 0, w.err }

// runShortWriter accepts everything but reports n-1 bytes written with no
// error, the classic short-write case io.WriteString cannot detect itself.
type runShortWriter struct{}

func (runShortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

// runNthFailWriter fails the n-th (1-based) Write call and accepts every
// other call in full, so a test can target the version-line write (n=1)
// separately from the template write (n=2).
type runNthFailWriter struct {
	n    int
	err  error
	call int
	buf  bytes.Buffer
}

func (w *runNthFailWriter) Write(p []byte) (int, error) {
	w.call++
	if w.call == w.n {
		return 0, w.err
	}
	return w.buf.Write(p)
}

// runNthShortWriter reports len(p)-1 bytes written with no error on the n-th
// Write call only, for stage-specific short-write coverage.
type runNthShortWriter struct {
	n    int
	call int
	buf  bytes.Buffer
}

func (w *runNthShortWriter) Write(p []byte) (int, error) {
	w.call++
	if w.call == w.n {
		if len(p) == 0 {
			return 0, nil
		}
		n := len(p) - 1
		if written, err := w.buf.Write(p[:n]); err != nil {
			return written, err
		}
		return n, nil
	}
	return w.buf.Write(p)
}

// runCountingWriter counts Write calls and keeps the bytes, proving that the
// version line and the template body are two independent writes.
type runCountingWriter struct {
	calls int
	buf   bytes.Buffer
}

func (w *runCountingWriter) Write(p []byte) (int, error) {
	w.calls++
	return w.buf.Write(p)
}

// runErrReadFS forces a non-ErrNotExist open failure for the named paths, so
// the fallback matrix can be exercised against a read error that is not a
// plain "missing" result.
type runErrReadFS struct {
	fsys fs.FS
	fail map[string]error
}

func (f runErrReadFS) Open(name string) (fs.File, error) {
	if err, ok := f.fail[name]; ok {
		return nil, err
	}
	return f.fsys.Open(name)
}

func TestRunVersionBypassesInitAndLang(t *testing.T) {
	initCalls, langCalls := 0, 0
	withRunSeams(t,
		func() error { initCalls++; return errors.New("init must not be called") },
		func(map[string]string) domain.Lang { langCalls++; return domain.LangEN },
	)

	for _, args := range [][]string{{"version"}, {"--version"}, {"-v"}} {
		var stdout, stderr bytes.Buffer
		code := Run(args, map[string]string{"TICKET_LANG": "en"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("Run(%q) = %d, want 0", args, code)
		}
		if got, want := stdout.String(), "ticket version dev\n"; got != want {
			t.Fatalf("Run(%q) stdout = %q, want %q", args, got, want)
		}
		if stderr.Len() != 0 {
			t.Fatalf("Run(%q) stderr = %q, want empty", args, stderr.String())
		}
	}
	if initCalls != 0 {
		t.Fatalf("domain init called %d times on version paths, want 0", initCalls)
	}
	if langCalls != 0 {
		t.Fatalf("langFrom called %d times on version paths, want 0", langCalls)
	}
}

func TestRunFatalInitStopsBeforeLangAndUsage(t *testing.T) {
	langCalls := 0
	initCalls := 0
	fatal := fmt.Errorf("%w: en.json broken", domain.ErrInitFatal)
	withRunSeams(t,
		func() error { initCalls++; return fatal },
		func(map[string]string) domain.Lang { langCalls++; return domain.LangRU },
	)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"help"}, map[string]string{"TICKET_LANG": "ru"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(help) = %d, want 1 on fatal init", code)
	}
	if initCalls != 1 {
		t.Fatalf("domain init called %d times, want 1", initCalls)
	}
	if langCalls != 0 {
		t.Fatalf("langFrom called %d times after fatal init, want 0", langCalls)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty (no usage on fatal init)", stdout.String())
	}
	if got, want := stderr.String(), fatal.Error()+"\n"; got != want {
		t.Fatalf("stderr = %q, want exactly one plain line %q", got, want)
	}
	if strings.Count(strings.TrimSuffix(stderr.String(), "\n"), "\n") != 0 {
		t.Fatalf("stderr must be a single line, got %q", stderr.String())
	}
}

func TestRunDegradedInitWarnsOnceAndContinues(t *testing.T) {
	degraded := fmt.Errorf("%w: ru.json broken", domain.ErrInitDegraded)
	withRunSeams(t,
		func() error { return degraded },
		func(map[string]string) domain.Lang { return domain.LangRU },
	)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"help"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(help) = %d, want 0 on degraded init", code)
	}
	if got, want := stderr.String(), degraded.Error()+"\n"; got != want {
		t.Fatalf("stderr = %q, want one warning %q", got, want)
	}
	if got, want := stdout.String(), versionLine()+usageTextRU; got != want {
		t.Fatalf("stdout = %d bytes, want version line + RU usage (%d bytes)", len(got), len(want))
	}
}

// TestLangFromDefaultValidatesRU covers the whole-diff fix: the no-locale
// default may be RU only when the RU dictionary actually registered. In a
// degraded initialisation (ru.json broken, EN usable) the default becomes EN,
// so the resolved language matches the dictionary domain.getDict would use
// instead of silently substituting EN behind a LangRU label.
func TestLangFromDefaultValidatesRU(t *testing.T) {
	prevKnown := isKnownLangFn
	t.Cleanup(func() { isKnownLangFn = prevKnown })

	withRunSeams(t,
		func() error { return fmt.Errorf("%w: ru.json broken", domain.ErrInitDegraded) },
		langFrom,
	)

	// Degraded registry: EN registered, RU not.
	isKnownLangFn = func(l domain.Lang) bool { return l == domain.LangEN }

	// No locale variables at all (nil and empty map) and all-empty strings
	// both take the default branch.
	for _, env := range []map[string]string{
		nil,
		{},
		{"TICKET_LANG": "", "LC_ALL": "", "LANG": ""},
	} {
		if got := langFrom(env); got != domain.LangEN {
			t.Fatalf("langFrom(%v) = %q, want EN with an unregistered RU", env, got)
		}
	}

	// End to end: `ticket help` with no locale env must render the EN
	// template while still surfacing the one degraded-init warning.
	var stdout, stderr bytes.Buffer
	code := Run([]string{"help"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(help) = %d, want 0; stderr: %q", code, stderr.String())
	}
	if got, want := stdout.String(), versionLine()+usageTextEN; got != want {
		t.Fatalf("stdout = %d bytes, want version line + EN usage (%d bytes)", len(got), len(want))
	}
	if !strings.Contains(stderr.String(), domain.ErrInitDegraded.Error()) {
		t.Fatalf("stderr = %q, want the degraded-init warning", stderr.String())
	}

	// A healthy registry still defaults to RU.
	withRunSeams(t, func() error { return nil }, langFrom)
	isKnownLangFn = func(l domain.Lang) bool { return l == domain.LangRU || l == domain.LangEN }

	if got := langFrom(nil); got != domain.LangRU {
		t.Fatalf("langFrom(nil) = %q, want RU when RU is registered", got)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"help"}, map[string]string{}, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(help) = %d, want 0; stderr: %q", code, stderr.String())
	}
	if got, want := stdout.String(), versionLine()+usageTextRU; got != want {
		t.Fatalf("stdout = %d bytes, want version line + RU usage (%d bytes)", len(got), len(want))
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty on a healthy init", stderr.String())
	}
}

func TestRunResolvesLangOnceForHelpAndDispatch(t *testing.T) {
	langCalls := 0
	withRunSeams(t,
		func() error { return nil },
		func(map[string]string) domain.Lang { langCalls++; return domain.LangRU },
	)

	cases := []struct {
		args     []string
		wantCode int
	}{
		{[]string{"help"}, 0},
		{[]string{"show", "-h"}, 0},
		{[]string{"unknown"}, 1},
	}
	for _, tc := range cases {
		langCalls = 0
		var stdout, stderr bytes.Buffer
		if code := Run(tc.args, nil, &stdout, &stderr); code != tc.wantCode {
			t.Fatalf("Run(%q) = %d, want %d", tc.args, code, tc.wantCode)
		}
		if langCalls != 1 {
			t.Fatalf("Run(%q) resolved lang %d times, want 1", tc.args, langCalls)
		}
	}
}

func TestRunUsageBytesAndCodes(t *testing.T) {
	withRunSeams(t, func() error { return nil }, langFrom)

	cases := []struct {
		name     string
		args     []string
		env      map[string]string
		wantCode int
		want     string
	}{
		{"no args ru", nil, nil, 0, versionLine() + usageTextRU},
		{"help ru", []string{"help"}, nil, 0, versionLine() + usageTextRU},
		{"-h ru", []string{"-h"}, nil, 0, versionLine() + usageTextRU},
		{"--help ru", []string{"--help"}, nil, 0, versionLine() + usageTextRU},
		{"unknown ru", []string{"nope"}, nil, 1, versionLine() + usageTextRU},
		{"subcommand help en", []string{"new", "-h"}, map[string]string{"TICKET_LANG": "en"}, 0, versionLine() + usageTextEN},
		{"show subcommand help ru", []string{"show", "-h"}, nil, 0, versionLine() + usageTextRU},
		{"set subcommand help ru", []string{"set", "-h"}, nil, 0, versionLine() + usageTextRU},
		{"init help en", []string{"init", "-h"}, map[string]string{"TICKET_LANG": "en"}, 0, versionLine() + usageTextEN},
		{"init extra args ru", []string{"init", "x"}, nil, 1, versionLine() + usageTextRU},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := Run(tc.args, tc.env, &stdout, &stderr); code != tc.wantCode {
				t.Fatalf("Run(%q) = %d, want %d; stderr: %q", tc.args, code, tc.wantCode, stderr.String())
			}
			if got := stdout.String(); got != tc.want {
				t.Fatalf("stdout is not the expected byte fixture (got %d bytes, want %d)", len(got), len(tc.want))
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func TestUsageFromFallbackMatrix(t *testing.T) {
	files := fstest.MapFS{
		"templates/usage.ru.txt": &fstest.MapFile{Data: []byte("RU-BODY")},
		"templates/usage.en.txt": &fstest.MapFile{Data: []byte("EN-BODY")},
	}

	t.Run("selected language", func(t *testing.T) {
		var b bytes.Buffer
		if err := usageFrom(&b, domain.LangRU, files, nil); err != nil {
			t.Fatalf("usageFrom = %v, want nil", err)
		}
		if got, want := b.String(), versionLine()+"RU-BODY"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})

	t.Run("selected missing falls back to EN with error naming language and path", func(t *testing.T) {
		var b bytes.Buffer
		err := usageFrom(&b, domain.Lang("zz"), files, nil)
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("usageFrom error = %v, want fs.ErrNotExist", err)
		}
		if !strings.Contains(err.Error(), "\"zz\"") {
			t.Fatalf("usageFrom error = %q, want it to name the requested language %q", err, "zz")
		}
		if !strings.Contains(err.Error(), "templates/usage.zz.txt") {
			t.Fatalf("usageFrom error = %q, want it to name the selected path", err)
		}
		if got, want := b.String(), versionLine()+"EN-BODY"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})

	t.Run("neither resource writes nothing", func(t *testing.T) {
		var b bytes.Buffer
		err := usageFrom(&b, domain.LangRU, fstest.MapFS{}, nil)
		if err == nil {
			t.Fatal("usageFrom = nil, want an error")
		}
		if b.Len() != 0 {
			t.Fatalf("stdout = %q, want empty", b.String())
		}
	})

	t.Run("valid selected render never reads EN", func(t *testing.T) {
		readErr := errors.New("EN unreadable")
		fsys := runErrReadFS{
			fsys: files,
			fail: map[string]error{"templates/usage.en.txt": readErr},
		}
		var b bytes.Buffer
		if err := usageFrom(&b, domain.LangRU, fsys, nil); err != nil {
			t.Fatalf("usageFrom = %v, want nil: EN must not be read when RU is readable", err)
		}
		if got, want := b.String(), versionLine()+"RU-BODY"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})

	t.Run("valid selected render ignores an unrelated init error", func(t *testing.T) {
		preload := errors.New("preload failed")
		var b bytes.Buffer
		if err := usageFrom(&b, domain.LangRU, files, preload); err != nil {
			t.Fatalf("usageFrom = %v, want nil: the selected template is readable", err)
		}
		if got, want := b.String(), versionLine()+"RU-BODY"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})

	t.Run("init error is joined on the EN fallback path", func(t *testing.T) {
		preload := errors.New("preload failed")
		var b bytes.Buffer
		err := usageFrom(&b, domain.Lang("zz"), files, preload)
		if !errors.Is(err, preload) {
			t.Fatalf("usageFrom error = %v, want the preload error", err)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("usageFrom error = %v, want fs.ErrNotExist for the selected template", err)
		}
		if got, want := b.String(), versionLine()+"EN-BODY"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})

	t.Run("non-ErrNotExist selected read error falls back to EN", func(t *testing.T) {
		readErr := fmt.Errorf("disk on fire: %w", fs.ErrPermission)
		fsys := runErrReadFS{
			fsys: files,
			fail: map[string]error{"templates/usage.zz.txt": readErr},
		}
		var b bytes.Buffer
		err := usageFrom(&b, domain.Lang("zz"), fsys, nil)
		if !errors.Is(err, fs.ErrPermission) {
			t.Fatalf("usageFrom error = %v, want fs.ErrPermission", err)
		}
		if errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("usageFrom error = %v, must not be reported as fs.ErrNotExist", err)
		}
		if !strings.Contains(err.Error(), "\"zz\"") {
			t.Fatalf("usageFrom error = %q, want it to name the requested language", err)
		}
		if got, want := b.String(), versionLine()+"EN-BODY"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})

	t.Run("write error is returned", func(t *testing.T) {
		writeErr := errors.New("write failed")
		err := usageFrom(runFailWriter{err: writeErr}, domain.LangRU, files, nil)
		if !errors.Is(err, writeErr) {
			t.Fatalf("usageFrom error = %v, want the write error", err)
		}
	})

	t.Run("version line and template are two writes", func(t *testing.T) {
		var w runCountingWriter
		if err := usageFrom(&w, domain.LangRU, files, nil); err != nil {
			t.Fatalf("usageFrom = %v, want nil", err)
		}
		if w.calls != 2 {
			t.Fatalf("Write called %d times, want 2 (version line + template)", w.calls)
		}
		if got, want := w.buf.String(), versionLine()+"RU-BODY"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})

	t.Run("first write (version line) error is returned", func(t *testing.T) {
		writeErr := errors.New("version write failed")
		w := &runNthFailWriter{n: 1, err: writeErr}
		err := usageFrom(w, domain.LangRU, files, nil)
		if !errors.Is(err, writeErr) {
			t.Fatalf("usageFrom error = %v, want the version-line write error", err)
		}
		if !strings.Contains(err.Error(), "version line") {
			t.Fatalf("usageFrom error = %q, want it to attribute the version line", err)
		}
		// Both stages are attempted: the body write is a separate call and
		// its own failure would be reported too.
		if w.call != 2 {
			t.Fatalf("Write called %d times, want 2", w.call)
		}
		if got, want := w.buf.String(), "RU-BODY"; got != want {
			t.Fatalf("output = %q, want only the body %q when the version line failed", got, want)
		}
	})

	t.Run("second write (template body) error is returned", func(t *testing.T) {
		writeErr := errors.New("body write failed")
		w := &runNthFailWriter{n: 2, err: writeErr}
		err := usageFrom(w, domain.LangRU, files, nil)
		if !errors.Is(err, writeErr) {
			t.Fatalf("usageFrom error = %v, want the body write error", err)
		}
		if !strings.Contains(err.Error(), "body") {
			t.Fatalf("usageFrom error = %q, want it to attribute the template body", err)
		}
		if got, want := w.buf.String(), versionLine(); got != want {
			t.Fatalf("output = %q, want only the version line %q", got, want)
		}
		if w.call != 2 {
			t.Fatalf("Write called %d times, want 2", w.call)
		}
	})

	t.Run("short write is an error", func(t *testing.T) {
		err := usageFrom(runShortWriter{}, domain.LangRU, files, nil)
		if !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("usageFrom error = %v, want io.ErrShortWrite", err)
		}
	})

	t.Run("second-write short write is an error", func(t *testing.T) {
		w := &runNthShortWriter{n: 2}
		err := usageFrom(w, domain.LangRU, files, nil)
		if !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("usageFrom error = %v, want io.ErrShortWrite for the body write", err)
		}
		if got, want := w.buf.String(), versionLine()+"RU-BOD"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})
}

// TestUsageProductionWrapper drives usage() itself with swapped usageFS and
// usageInitErr seams: the production wrapper must pass the ACTIVE filesystem
// and the stored init error to usageFrom. A readable selected template wins
// over an unrelated init error; the same error becomes visible only when the
// selected read falls back. No t.Parallel: the seams are package-level state.
func TestUsageProductionWrapper(t *testing.T) {
	prevFS, prevInitErr := usageFS, usageInitErr
	t.Cleanup(func() { usageFS, usageInitErr = prevFS, prevInitErr })

	usageFS = fstest.MapFS{
		"templates/usage.ru.txt": &fstest.MapFile{Data: []byte("RU-SEAM")},
		"templates/usage.en.txt": &fstest.MapFile{Data: []byte("EN-SEAM")},
	}

	t.Run("valid selected render ignores a stored init error", func(t *testing.T) {
		usageInitErr = errors.New("preload failed")
		var b bytes.Buffer
		if err := usage(&b, domain.LangRU); err != nil {
			t.Fatalf("usage = %v, want nil for a readable selected template", err)
		}
		if got, want := b.String(), versionLine()+"RU-SEAM"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})

	t.Run("fallback joins the stored init error", func(t *testing.T) {
		usageInitErr = errors.New("preload failed")
		var b bytes.Buffer
		err := usage(&b, domain.Lang("zz"))
		if !errors.Is(err, usageInitErr) {
			t.Fatalf("usage error = %v, want the stored init error", err)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("usage error = %v, want fs.ErrNotExist for the selected template", err)
		}
		if got, want := b.String(), versionLine()+"EN-SEAM"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})
}

// TestRunUsageWriteFailureIsOneDiagnostic drives a failing stdout writer
// through EVERY usage call route. There are eight printUsage call sites:
//
//  1. cli.go  init -h
//  2. cli.go  init with extra args
//  3. cli.go  subcommand -h (new/show/set/archive branch)
//  4. cli.go  help/-h/--help
//  5. cli.go  unknown command (default branch)
//  6. cmd_new.go  bare `new`
//  7. cmd_show.go bare `show`
//  8. cmd_set.go  bare `set`
//
// Each route must report the writer failure as exactly ONE stderr line and
// exit code 1, including the paths that normally exit 0 (help, -h). The
// normal-path stdout matrix is covered by TestRunUsageBytesAndCodes and
// TestRunBareCallerUsageErrors, which stay unchanged.
func TestRunUsageWriteFailureIsOneDiagnostic(t *testing.T) {
	withRunSeams(t, func() error { return nil }, langFrom)
	writeErr := errors.New("stdout broken")

	cases := []struct {
		name string
		args []string
	}{
		{"init -h (site 1)", []string{"init", "-h"}},
		{"init extra args (site 2)", []string{"init", "x"}},
		{"new -h (site 3)", []string{"new", "-h"}},
		{"show -h (site 3)", []string{"show", "-h"}},
		{"set -h (site 3)", []string{"set", "-h"}},
		{"help (site 4)", []string{"help"}},
		{"no args (site 4)", nil},
		{"unknown command (site 5)", []string{"nope"}},
		{"new without a title (site 6)", []string{"new"}},
		{"show without a number (site 7)", []string{"show"}},
		{"set without a number and status (site 8)", []string{"set"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			code := Run(tc.args, map[string]string{"TICKETS_DIR": t.TempDir()}, runFailWriter{err: writeErr}, &stderr)
			if code != 1 {
				t.Fatalf("Run(%q) = %d, want 1 on a failing usage writer", tc.args, code)
			}
			if !strings.Contains(stderr.String(), writeErr.Error()) {
				t.Fatalf("stderr = %q, want it to name the write error", stderr.String())
			}
			if strings.Count(stderr.String(), "\n") != 1 {
				t.Fatalf("stderr must be exactly one diagnostic line, got %q", stderr.String())
			}
		})
	}
}

// TestRunBareCallerUsageErrors drives the bare-argument usage branches of the
// three callers that print usage themselves (cmdNew, cmdShow, cmdSet): the
// usage text goes to stdout, stderr stays empty and the exit code is 1.
func TestRunBareCallerUsageErrors(t *testing.T) {
	withRunSeams(t, func() error { return nil }, langFrom)

	cases := []struct {
		name string
		args []string
	}{
		{"new without a title", []string{"new"}},
		{"show without a number", []string{"show"}},
		{"set without a number and status", []string{"set"}},
		{"set with only a number", []string{"set", "1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			var stdout, stderr bytes.Buffer
			code := Run(tc.args, map[string]string{"TICKETS_DIR": dir}, &stdout, &stderr)
			if code != 1 {
				t.Fatalf("Run(%q) = %d, want 1; stderr: %q", tc.args, code, stderr.String())
			}
			if got, want := stdout.String(), versionLine()+usageTextRU; got != want {
				t.Fatalf("stdout is not the version line + RU usage (got %d bytes, want %d)", len(got), len(want))
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}
