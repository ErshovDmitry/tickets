package store

// Store logger seam (T-0110 W2): WithLogger injects a command logger; Logger
// always returns a usable logger, falling back to a discarding one for a nil
// receiver, a zero-value Store, no injection or WithLogger(nil). The seam adds
// no log call of its own, so store operations stay silent.

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"testing"
)

// levels is the level set the CLI logger can render; the fallback logger must
// admit none of them.
var levels = []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError}

// bufferLogger returns a debug-threshold text logger plus the buffer it writes
// to, so tests can prove that nothing was written.
func bufferLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

// openers covers both constructors so every logger assertion is pinned for
// each of them.
var openers = []struct {
	name string
	open func(dir string, opts ...Option) (*Store, error)
}{
	{"New", New},
	{"NewReadOnly", NewReadOnly},
}

// loggerCase is one store whose Logger must fall back to a discarding logger.
type loggerCase struct {
	name   string
	logger func(*testing.T) *slog.Logger
}

// assertFallback fails unless l is non-nil and discards at every CLI level.
func assertFallback(t *testing.T, l *slog.Logger) {
	t.Helper()
	if l == nil {
		t.Fatal("Logger() returned nil; want a discarding logger")
	}
	ctx := t.Context()
	for _, lvl := range levels {
		if l.Enabled(ctx, lvl) {
			t.Fatalf("fallback logger enabled at %v; want disabled", lvl)
		}
	}
}

// TestLogger_FallbackIsNonNilAndDiscards pins every fallback case: nil
// receiver, zero value, unset logger and nil injection, for both constructors.
func TestLogger_FallbackIsNonNilAndDiscards(t *testing.T) {
	cases := []loggerCase{
		{"nil receiver", func(*testing.T) *slog.Logger {
			var s *Store
			return s.Logger()
		}},
		{"zero value", func(*testing.T) *slog.Logger {
			var s Store
			return s.Logger()
		}},
	}
	for _, op := range openers {
		unset, err := op.open(t.TempDir())
		if err != nil {
			t.Fatalf("%s: %v", op.name, err)
		}
		nilInjected, err := op.open(t.TempDir(), WithLogger(nil))
		if err != nil {
			t.Fatalf("%s WithLogger(nil): %v", op.name, err)
		}
		cases = append(cases,
			loggerCase{op.name + " without option", func(*testing.T) *slog.Logger { return unset.Logger() }},
			loggerCase{op.name + " with WithLogger(nil)", func(*testing.T) *slog.Logger { return nilInjected.Logger() }},
		)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertFallback(t, tc.logger(t))
		})
	}
}

// TestLogger_InjectsExactInstance pins that both constructors honour
// WithLogger: Logger returns the very instance handed in, so the injected
// logger keeps its own Enabled result instead of a discarding fallback.
func TestLogger_InjectsExactInstance(t *testing.T) {
	ctx := t.Context()
	for _, op := range openers {
		t.Run(op.name, func(t *testing.T) {
			injected, injectedBuf := bufferLogger()
			st, err := op.open(t.TempDir(), WithLogger(injected))
			if err != nil {
				t.Fatalf("%s: %v", op.name, err)
			}
			got := st.Logger()
			if got != injected {
				t.Fatalf("Logger() = %p; want the injected logger %p", got, injected)
			}
			if !got.Enabled(ctx, slog.LevelError) {
				t.Fatal("injected logger lost its Enabled result at LevelError")
			}
			if got.Enabled(ctx, slog.LevelInfo) != injected.Enabled(ctx, slog.LevelInfo) {
				t.Fatal("Logger() Enabled differs from the injected logger")
			}
			if injectedBuf.Len() != 0 {
				t.Fatalf("seam emitted %q; want no log line", injectedBuf.String())
			}

			explicitDiscard := slog.New(slog.DiscardHandler)
			dt, err := op.open(t.TempDir(), WithLogger(explicitDiscard))
			if err != nil {
				t.Fatalf("%s: %v", op.name, err)
			}
			if dt.Logger() != explicitDiscard {
				t.Fatal("an explicitly injected discarding logger was replaced")
			}
			if dt.Logger().Enabled(ctx, slog.LevelError) {
				t.Fatal("explicitly injected discarding logger admitted a record")
			}
		})
	}
}

// TestLogger_OptionsRunAfterValidation pins that a rejected directory applies
// no option: an invalid path fails validateDir, and a valid directory whose
// write probe fails also stops before any option runs.
func TestLogger_OptionsRunAfterValidation(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")
	t.Run("invalid directory", func(t *testing.T) {
		for _, op := range openers {
			t.Run(op.name, func(t *testing.T) {
				assertOptionsNotApplied(t, op.name, op.open, missing)
			})
		}
	})
	// Only New probes writability; NewReadOnly deliberately skips the probe,
	// so a read-only directory rejects New alone.
	t.Run("write probe rejected", func(t *testing.T) {
		skipRootOrWindows(t)
		assertOptionsNotApplied(t, "New", New, makeReadOnlyDir(t))
	})
}

// assertOptionsNotApplied fails unless open rejects dir before running opts.
func assertOptionsNotApplied(t *testing.T, name string, open func(string, ...Option) (*Store, error), dir string) {
	t.Helper()
	applied := false
	st, err := open(dir, func(*Store) { applied = true })
	if err == nil {
		t.Fatalf("%s on %q returned no error", name, dir)
	}
	if st != nil {
		t.Fatalf("%s returned a Store alongside its error", name)
	}
	if applied {
		t.Fatalf("%s applied an option to the rejected directory %q", name, dir)
	}
}

// TestLogger_OptionsApplyInOrder pins that options run in the given order, so
// the last WithLogger wins and Logger returns that exact instance.
func TestLogger_OptionsApplyInOrder(t *testing.T) {
	for _, op := range openers {
		t.Run(op.name, func(t *testing.T) {
			first, firstBuf := bufferLogger()
			last, lastBuf := bufferLogger()
			st, err := op.open(t.TempDir(), WithLogger(first), WithLogger(last))
			if err != nil {
				t.Fatalf("%s: %v", op.name, err)
			}
			if got := st.Logger(); got != last {
				t.Fatalf("Logger() = %p; want the last WithLogger instance %p", got, last)
			}
			if firstBuf.Len() != 0 || lastBuf.Len() != 0 {
				t.Fatalf("options emitted %q %q; want no log line", firstBuf.String(), lastBuf.String())
			}

			reverse, err := op.open(t.TempDir(), WithLogger(last), WithLogger(first))
			if err != nil {
				t.Fatalf("%s reversed: %v", op.name, err)
			}
			if got := reverse.Logger(); got != first {
				t.Fatalf("reversed Logger() = %p; want the last WithLogger instance %p", got, first)
			}
		})
	}
}

// TestLogger_WritesNoLogLine pins that the seam itself emits nothing: a
// debug-threshold logger stays empty across construction and a read.
func TestLogger_WritesNoLogLine(t *testing.T) {
	for _, op := range openers {
		t.Run(op.name, func(t *testing.T) {
			l, buf := bufferLogger()
			st, err := op.open(t.TempDir(), WithLogger(l))
			if err != nil {
				t.Fatalf("%s: %v", op.name, err)
			}
			tickets, warnings := st.List()
			if len(tickets) != 0 || len(warnings) != 0 {
				t.Fatalf("empty directory yielded %d tickets, %d warnings", len(tickets), len(warnings))
			}
			if buf.Len() != 0 {
				t.Fatalf("store emitted %q; want no log line", buf.String())
			}
		})
	}
}
