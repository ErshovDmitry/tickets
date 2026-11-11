package store

// T-0164: the recover journal must distinguish the destructive pair commit
// from the single-file idempotent no-op. These tests assert on the parsed
// slog records (not on lenient substrings), filtered to the store.recover.*
// prefix so the lock and set records sharing the buffer cannot interfere.

import (
	"bytes"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// slogRecord is one parsed slog text-handler record.
type slogRecord map[string]string

// newLoggedStore returns a store whose slog records are captured in the
// returned buffer.
func newLoggedStore(t *testing.T) (*Store, string, *bytes.Buffer) {
	t.Helper()
	logger, buf := bufferLogger()
	dir := t.TempDir()
	s, err := New(dir, WithLogger(logger))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	return s, dir, buf
}

// splitSlogFields splits one slog text line into key=value fields, honoring
// the handler's quoting so a value may contain spaces.
func splitSlogFields(line string) []string {
	var fields []string
	var cur strings.Builder
	quoted, escaped := false, false
	for _, r := range line {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case quoted && r == '\\':
			cur.WriteRune(r)
			escaped = true
		case r == '"':
			cur.WriteRune(r)
			quoted = !quoted
		case r == ' ' && !quoted:
			if cur.Len() > 0 {
				fields = append(fields, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		fields = append(fields, cur.String())
	}
	return fields
}

// parseSlogLine parses one slog text line into a record.
func parseSlogLine(t *testing.T, line string) slogRecord {
	t.Helper()
	rec := slogRecord{}
	for _, field := range splitSlogFields(line) {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			t.Fatalf("malformed slog field %q in line %q", field, line)
		}
		if strings.HasPrefix(value, "\x22") {
			unquoted, err := strconv.Unquote(value)
			if err != nil {
				t.Fatalf("unquote %q in line %q: %v", value, line, err)
			}
			value = unquoted
		}
		rec[key] = value
	}
	return rec
}

// recoverRecords returns the parsed store.recover.* records of the buffer.
func recoverRecords(t *testing.T, buf *bytes.Buffer) []slogRecord {
	t.Helper()
	var out []slogRecord
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		rec := parseSlogLine(t, line)
		if strings.HasPrefix(rec["msg"], "store.recover.") {
			out = append(out, rec)
		}
	}
	return out
}

// countEvents counts the records carrying msg.
func countEvents(records []slogRecord, msg string) int {
	n := 0
	for _, rec := range records {
		if rec["msg"] == msg {
			n++
		}
	}
	return n
}

// singleEvent requires exactly one record carrying msg and returns it.
func singleEvent(t *testing.T, records []slogRecord, msg string) slogRecord {
	t.Helper()
	if n := countEvents(records, msg); n != 1 {
		t.Fatalf("%s records = %d, want exactly 1 (records: %v)", msg, n, records)
	}
	for _, rec := range records {
		if rec["msg"] == msg {
			return rec
		}
	}
	return nil
}

// assertEventCount fails unless exactly want records carry msg.
func assertEventCount(t *testing.T, records []slogRecord, msg string, want int) {
	t.Helper()
	if got := countEvents(records, msg); got != want {
		t.Errorf("%s records = %d, want %d (records: %v)", msg, got, want, records)
	}
}

// TestRecoverLog_PairCommitEmitsCommitOnly pins that a real pair recovery
// emits one store.recover.commit carrying both paths and no noop.
func TestRecoverLog_PairCommitEmitsCommitOnly(t *testing.T) {
	s, dir, buf := newLoggedStore(t)
	oldPath, newPath := crashPair(t, s, dir, domain.StatusDone)
	if _, err := s.Recover(1, "T-0001-done.md"); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	records := recoverRecords(t, buf)
	rec := singleEvent(t, records, "store.recover.commit")
	if rec["number"] != "1" {
		t.Errorf("commit number = %q, want %q", rec["number"], "1")
	}
	if rec["keep"] != newPath {
		t.Errorf("commit keep = %q, want %q", rec["keep"], newPath)
	}
	if rec["discard"] != oldPath {
		t.Errorf("commit discard = %q, want %q", rec["discard"], oldPath)
	}
	assertEventCount(t, records, "store.recover.noop", 0)
	assertEventCount(t, records, "store.recover.failure", 0)
}

// TestRecoverLog_SingleNoopHasNoDiscard pins that the lone-file no-op emits
// store.recover.noop without a discard attribute (an empty discard is what
// used to make the no-op look destructive).
func TestRecoverLog_SingleNoopHasNoDiscard(t *testing.T) {
	s, dir, buf := newLoggedStore(t)
	seedTicketOne(t, s, dir)
	path := filepath.Join(dir, "T-0001-open.md")
	if _, err := s.Recover(1, "T-0001-open.md"); err != nil {
		t.Fatalf("Recover on a lone ticket: %v", err)
	}
	records := recoverRecords(t, buf)
	rec := singleEvent(t, records, "store.recover.noop")
	if rec["number"] != "1" {
		t.Errorf("noop number = %q, want %q", rec["number"], "1")
	}
	if rec["keep"] != path {
		t.Errorf("noop keep = %q, want %q", rec["keep"], path)
	}
	if discard, ok := rec["discard"]; ok {
		t.Errorf("noop record must carry no discard attribute, got %q", discard)
	}
	assertEventCount(t, records, "store.recover.commit", 0)
	assertEventCount(t, records, "store.recover.failure", 0)
}

// TestRecoverLog_CommitThenNoop pins the transition: a pair recovery
// followed by a repeated Recover yields exactly one commit and one noop.
func TestRecoverLog_CommitThenNoop(t *testing.T) {
	s, dir, buf := newLoggedStore(t)
	crashPair(t, s, dir, domain.StatusDone)
	if _, err := s.Recover(1, "T-0001-done.md"); err != nil {
		t.Fatalf("first Recover: %v", err)
	}
	if _, err := s.Recover(1, "T-0001-done.md"); err != nil {
		t.Fatalf("second Recover: %v", err)
	}
	records := recoverRecords(t, buf)
	assertEventCount(t, records, "store.recover.commit", 1)
	assertEventCount(t, records, "store.recover.noop", 1)
	assertEventCount(t, records, "store.recover.failure", 0)
}

// TestRecoverLog_RefusalEmitsFailureOnly pins that a refused recovery emits
// store.recover.failure and neither commit nor noop.
func TestRecoverLog_RefusalEmitsFailureOnly(t *testing.T) {
	s, dir, buf := newLoggedStore(t)
	body := seedTicketOne(t, s, dir)
	cloneTicket(t, filepath.Join(dir, "T-0001-done.md"), body)
	unchangedAfterRefusal(t, s, dir, "T-0001-done.md", RecoverReasonDivergent)
	records := recoverRecords(t, buf)
	rec := singleEvent(t, records, "store.recover.failure")
	if rec["number"] != "1" {
		t.Errorf("failure number = %q, want %q", rec["number"], "1")
	}
	if rec["stage"] != "state" {
		t.Errorf("failure stage = %q, want %q", rec["stage"], "state")
	}
	assertEventCount(t, records, "store.recover.commit", 0)
	assertEventCount(t, records, "store.recover.noop", 0)
}
