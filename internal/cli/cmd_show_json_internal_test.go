package cli

// T-0241 A5 and code review R3.7: package-internal unit tests for the parts of
// the --json document that cannot be reached end-to-end. Every warning
// PRODUCER sets an error (scan.go, archivedir.go, deleteddir.go, store.go), so
// the error-less warning shape is unreachable from a real store; and a stdout
// that fails or truncates a write cannot be built from cli.Run, which writes
// into a strings.Builder.

import (
	"errors"
	"strings"
	"testing"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// TestShowJSONWarningWithoutError pins the ParseWarning.Error() semantics the
// DTO relies on: a warning carrying no error renders as the bare name, and the
// service reason code never enters the text.
func TestShowJSONWarningWithoutError(t *testing.T) {
	got := showJSONWarnings([]store.ParseWarning{{Name: "T-0001-open.md", Reason: "probe"}})
	if len(got) != 1 || got[0] != "T-0001-open.md" {
		t.Fatalf("showJSONWarnings = %v, want [\"T-0001-open.md\"]", got)
	}
	if got := showJSONWarnings(nil); len(got) != 0 {
		t.Errorf("showJSONWarnings(nil) = %v, want an empty slice", got)
	}
}

// failWriter fails every write — the shape of a closed pipe.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("probe: write failed") }

// shortWriter keeps only the first keep bytes of EVERY write and reports no
// error — the shape of a truncated stream. It records the bytes and the number
// of calls so a test can prove the CLI never patches a short write with a
// second one.
type shortWriter struct {
	keep  int
	got   []byte
	calls int
}

func (w *shortWriter) Write(p []byte) (int, error) {
	w.calls++
	n := min(w.keep, len(p))
	w.got = append(w.got, p[:n]...)
	return n, nil
}

// showJSONProbeTicket is the minimal ticket whose every DTO field is filled, so
// the document under test is a real one.
func showJSONProbeTicket() domain.Ticket {
	return domain.Ticket{
		Number:   1,
		Status:   "open",
		Type:     "BUG",
		Priority: "normal",
		Project:  "tickets",
		Title:    "probe title",
		Brief:    []string{"probe summary"},
	}
}

// TestRunShowJSONWriteError pins the failed-write path: exit 1 with one
// localized error line on stderr and nothing on stdout.
func TestRunShowJSONWriteError(t *testing.T) {
	var stderr strings.Builder
	code := runShowJSON(failWriter{}, &stderr, domain.LangEN, showJSONProbeTicket(), nil, nil)
	if code != exitError {
		t.Fatalf("code = %d, want %d", code, exitError)
	}
	got := stderr.String()
	if !strings.Contains(got, "ticket: ") {
		t.Errorf("stderr = %q, want it to name the failure", got)
	}
	if !strings.HasSuffix(got, "\n") || strings.Count(got, "\n") != 1 {
		t.Errorf("stderr = %q, want exactly one line", got)
	}
}

// TestRunShowJSONShortWrite pins the truncated-write path: a short write is an
// error (exit 1), never patched up with a second write, and the shortfall is
// reported with both byte counts.
func TestRunShowJSONShortWrite(t *testing.T) {
	w := &shortWriter{keep: 10}
	var stderr strings.Builder
	code := runShowJSON(w, &stderr, domain.LangEN, showJSONProbeTicket(), nil, nil)
	if code != exitError {
		t.Fatalf("code = %d, want %d (stderr: %s)", code, exitError, stderr.String())
	}
	if w.calls != 1 {
		t.Errorf("write calls = %d, want 1: a short write must not be retried", w.calls)
	}
	if len(w.got) != 10 {
		t.Errorf("bytes accepted = %d, want 10", len(w.got))
	}
	got := stderr.String()
	if !strings.Contains(got, "10 of ") {
		t.Errorf("stderr = %q, want the written and expected byte counts", got)
	}
	if strings.HasSuffix(strings.TrimSuffix(got, "\n"), "\n") || strings.Count(got, "\n") != 1 {
		t.Errorf("stderr = %q, want exactly one line", got)
	}
}

// TestRunShowJSONShortWriteRussian pins the RU rendering of the same path: the
// two JSON diagnostics are dictionary messages, not hard-coded English.
func TestRunShowJSONShortWriteRussian(t *testing.T) {
	var stderr strings.Builder
	code := runShowJSON(&shortWriter{keep: 1}, &stderr, domain.LangRU, showJSONProbeTicket(), nil, nil)
	if code != exitError {
		t.Fatalf("code = %d, want %d", code, exitError)
	}
	if got := stderr.String(); !strings.Contains(got, "байт") {
		t.Errorf("stderr = %q, want the Russian short-write wording", got)
	}
}
