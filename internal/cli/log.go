package cli

// Debug logging for the ticket CLI (T-0110). The logger is built once per
// command from the TICKET_LOG destination and the --verbose flag; records are
// written as one line each, never to stdout.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"ticket/internal/store"
)

// logFileEnv names the environment variable holding the log destination path.
// The value is used only as a path by newLogger and is never logged itself.
const logFileEnv = "TICKET_LOG"

// logFilePerm is the permission mode for a log file created by newLogger.
// It is the numeric source of the documented "owner-only" contract.
const logFilePerm = 0o600

// warnEvent is the single structured-log event name for a store parse warning
// surfaced by a command.
const warnEvent = "store.read.warning"

// logParseWarnings emits one warn record per store parse warning. It is the
// first CLI use of the st.Logger() seam: command handlers have no logger
// parameter, so the store built by dispatch with store.WithLogger is the only
// route. Logger() is called once (a store without an injected logger allocates
// a fresh discarding logger per call) and no Enabled pre-check is done:
// (*slog.Logger).Warn (log/slog/logger.go:218) calls the unexported
// (*slog.Logger).log (:240), which evaluates Enabled at :244; it never calls
// the exported (*slog.Logger).Log (:188).
func logParseWarnings(st *store.Store, warnings []store.ParseWarning) {
	if len(warnings) == 0 {
		return
	}
	lg := st.Logger()
	for _, w := range warnings {
		lg.Warn(warnEvent, "path", w.Name, "reason", w.Reason)
	}
}

// newLogger builds the command logger. When neither verbose nor a non-empty
// TICKET_LOG is set it returns a discarding logger and opens no file. A
// non-empty TICKET_LOG wins over verbose and makes the logger append to that
// file (created owner-only, existing mode preserved, parent directories never
// created); otherwise records go to stderr.
//
// Only after a successful return may close be deferred unconditionally: it
// surfaces the first write error and, for a file destination, the file-close
// error, and never returns nil. A failed open returns a nil logger and a nil
// close function, so close must not be deferred before err is checked.
func newLogger(stderr io.Writer, env map[string]string, verbose bool) (*slog.Logger, func() error, error) {
	path := env[logFileEnv]
	if !verbose && path == "" {
		return slog.New(slog.DiscardHandler), func() error { return nil }, nil
	}
	st := &logState{}
	if path == "" {
		return slog.New(&logHandler{st: st, w: stderr}), st.close, nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, logFilePerm)
	if err != nil {
		return nil, nil, fmt.Errorf("ticket: open log file %s: %w", path, err)
	}
	lf := &logFile{st: st, path: path, f: f}
	return slog.New(&logHandler{st: st, w: f}), lf.close, nil
}

// newLoggerFn is the seam through which runLogged builds its logger. Production
// code always takes newLogger; tests replace it to inject a deterministic
// close failure and restore the previous value in t.Cleanup.
var newLoggerFn = newLogger

// logState is the write state shared by every handler copy derived from one
// logger: the mutex serialises whole lines and the first write error is kept
// until the deferred close surfaces it.
type logState struct {
	mu  sync.Mutex
	err error
}

// write emits line as a single locked write so concurrent records never
// interleave. Logger discards Handler errors, so failures are recorded here
// and reported by close instead.
func (st *logState) write(w io.Writer, line string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if _, err := io.WriteString(w, line); err != nil && st.err == nil {
		st.err = err
	}
}

// writeErr returns the first write error seen by any handler copy.
func (st *logState) writeErr() error {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.err
}

// close surfaces the first write error of a stderr destination.
func (st *logState) close() error {
	if err := st.writeErr(); err != nil {
		return fmt.Errorf("ticket: log write: %w", err)
	}
	return nil
}

// logFile is the close state of a file destination.
type logFile struct {
	st   *logState
	path string
	f    *os.File
}

// close joins the first write error with the file-close error.
func (lf *logFile) close() error {
	var errs []error
	if err := lf.st.writeErr(); err != nil {
		errs = append(errs, fmt.Errorf("ticket: log write: %w", err))
	}
	if err := lf.f.Close(); err != nil {
		errs = append(errs, fmt.Errorf("ticket: close log file %s: %w", lf.path, err))
	}
	return errors.Join(errs...)
}

// logHandler renders records as "ticket: <level>: <event>[ key=value...]" with
// no timestamp and no caller. Copies share st (write mutex and first error)
// but own their attribute slice.
type logHandler struct {
	st     *logState
	w      io.Writer
	prefix string   // group path, e.g. "g.", applied to later attributes
	kvs    []string // already rendered "key=value" pairs from With-attrs
}

// Enabled admits exactly the levels levelName can render, so the admitted and
// rendered level sets cannot drift apart.
func (h *logHandler) Enabled(_ context.Context, l slog.Level) bool {
	return levelName(l) != ""
}

// Handle renders one record with a single locked write.
func (h *logHandler) Handle(_ context.Context, r slog.Record) error {
	level := levelName(r.Level)
	if level == "" {
		return nil
	}
	kvs := make([]string, 0, len(h.kvs)+r.NumAttrs())
	kvs = append(kvs, h.kvs...)
	r.Attrs(func(a slog.Attr) bool {
		kvs = appendAttr(kvs, h.prefix, a)
		return true
	})
	line := "ticket: " + level + ": " + renderToken(r.Message)
	if len(kvs) > 0 {
		line += " " + strings.Join(kvs, " ")
	}
	h.st.write(h.w, line+"\n")
	return nil
}

// WithAttrs returns a copy owning a fresh attribute slice; the input slice is
// copied, never retained. With no attributes the receiver is returned as is.
func (h *logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	nh := *h
	nh.kvs = make([]string, 0, len(h.kvs)+len(attrs))
	nh.kvs = append(nh.kvs, h.kvs...)
	for _, a := range attrs {
		nh.kvs = appendAttr(nh.kvs, h.prefix, a)
	}
	return &nh
}

// WithGroup returns a copy whose later attributes carry the group prefix;
// attributes added earlier stay unprefixed.
func (h *logHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	nh := *h
	nh.prefix = h.prefix + name + "."
	return &nh
}

// levelName maps a supported level to its line token; unsupported levels get
// an empty name.
func levelName(l slog.Level) string {
	switch l {
	case slog.LevelDebug:
		return "debug"
	case slog.LevelInfo:
		return "info"
	case slog.LevelWarn:
		return "warn"
	case slog.LevelError:
		return "error"
	}
	return ""
}

// appendAttr renders a resolved attribute at the given group prefix. A zero
// attribute and an attribute-less group are dropped; an empty group key inlines
// its attributes at the same prefix; leaves are appended as key=value.
func appendAttr(dst []string, prefix string, a slog.Attr) []string {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return dst
	}
	if a.Value.Kind() == slog.KindGroup {
		attrs := a.Value.Group()
		if len(attrs) == 0 {
			return dst
		}
		groupPrefix := prefix
		if a.Key != "" {
			groupPrefix = prefix + a.Key + "."
		}
		for _, ga := range attrs {
			dst = appendAttr(dst, groupPrefix, ga)
		}
		return dst
	}
	return append(dst, renderToken(prefix+a.Key)+"="+formatValue(a.Value))
}

// formatValue renders numbers, booleans and durations bare and everything else
// as a quoted string.
func formatValue(v slog.Value) string {
	switch v.Kind() {
	case slog.KindInt64, slog.KindUint64, slog.KindFloat64, slog.KindBool, slog.KindDuration:
		return v.String()
	default:
		return strconv.Quote(v.String())
	}
}

// renderToken returns s bare when it is a plain token and quoted otherwise, so
// an event name or a key holding spaces, '=', quotes, backslashes, control
// bytes or non-printable runes cannot break the one-record-per-line format.
func renderToken(s string) string {
	if !needsQuote(s) {
		return s
	}
	return strconv.Quote(s)
}

// needsQuote reports whether s would break the "key=value" line grammar or
// hold invalid UTF-8, which would make the written line invalid UTF-8.
func needsQuote(s string) bool {
	if s == "" || !utf8.ValidString(s) {
		return true
	}
	for _, r := range s {
		if r < utf8.RuneSelf {
			if r == ' ' || r == '=' || r == '"' || r == '\\' || r < 0x20 || r == 0x7f {
				return true
			}
			continue
		}
		if !unicode.IsPrint(r) || unicode.IsSpace(r) {
			return true
		}
	}
	return false
}
