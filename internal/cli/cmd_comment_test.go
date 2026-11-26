package cli

// T-0240 A2 CLI tests for ticket comment: the --schema pre-mode, append +
// one-line journal, the empty/ATX refusals, not-found/ambiguous/deleted exit
// codes, byte-identical refusal and EN localization.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCommentSchema pins case 11: --schema prints exactly the JSON Schema to
// stdout (exit 0, no lookup, no write — it works without any ticket), and a
// payload source alongside it is a conflict (exit 1).
func TestCommentSchema(t *testing.T) {
	dir := writeTestDir(t) // deliberately no ticket seeded
	stdout, stderr, code := writeRun(t, dir, "en", "comment", "--schema")
	if code != 0 || stderr != "" {
		t.Fatalf("schema: code=%d stderr=%q", code, stderr)
	}
	if stdout != writeJSONSchema+"\n" {
		t.Errorf("schema stdout = %q, want the exact schema + LF", stdout)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("--schema wrote into the tickets dir: %d entries", len(entries))
	}
	for _, args := range [][]string{
		{"comment", "--schema", "-"},
		{"comment", "--schema", "--file", "whatever"},
	} {
		_, serr, code := writeRun(t, dir, "en", args...)
		if code != 1 || !strings.Contains(serr, "--schema cannot be combined") {
			t.Errorf("schema+source %q: code=%d stderr=%q", args, code, serr)
		}
	}
	// A leading number does not block the schema mode (no lookup happens).
	if sout, _, code := writeRun(t, dir, "en", "comment", "1", "--schema"); code != 0 || sout != writeJSONSchema+"\n" {
		t.Errorf("comment 1 --schema: code=%d stdout=%q", code, sout)
	}
}

// TestDetectCommentSchema pins the shared flag grammar (walkWriteFlags):
// source flags set hasSource, --schema sets schema, a leading positional is
// skipped, and a --file value is consumed blindly so a "--schema" used as a
// file path does not trigger the schema mode.
func TestDetectCommentSchema(t *testing.T) {
	cases := []struct {
		name              string
		args              []string
		schema, hasSource bool
	}{
		{"schema only", []string{"--schema"}, true, false},
		{"schema and stdin", []string{"--schema", "-"}, true, true},
		{"schema and file", []string{"--schema", "--file", "p"}, true, true},
		{"file value looks like schema", []string{"--file", "--schema"}, false, true},
		{"leading number skipped", []string{"1", "--schema"}, true, false},
		{"no schema", []string{"1", "-"}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			schema, hasSource := detectCommentSchema(tc.args)
			if schema != tc.schema || hasSource != tc.hasSource {
				t.Fatalf("detectCommentSchema(%q) = (%v,%v), want (%v,%v)", tc.args, schema, hasSource, tc.schema, tc.hasSource)
			}
		})
	}
}

// TestCommentAppendAndJournal pins DS1/DS7/DS12: two comments append as two
// blocks separated by exactly one blank line, each with a one-line journal
// event; success is silent (exit 0, empty stderr).
func TestCommentAppendAndJournal(t *testing.T) {
	dir := writeTestDir(t)
	seedWriteTicket(t, dir)
	withStdin(t, strings.NewReader("first comment"))
	if _, serr, code := writeRun(t, dir, "", "comment", "1", "-"); code != 0 || serr != "" {
		t.Fatalf("comment 1: code=%d stderr=%q", code, serr)
	}
	withStdin(t, strings.NewReader("second comment"))
	if _, serr, code := writeRun(t, dir, "", "comment", "1", "-"); code != 0 || serr != "" {
		t.Fatalf("comment 2: code=%d stderr=%q", code, serr)
	}
	body := readWriteTicket(t, dir)
	if !strings.Contains(body, "first comment\n\nsecond comment") {
		t.Errorf("comments not separated by exactly one blank line: %q", body)
	}
	if n := strings.Count(body, "comment: +1 строк"); n != 2 {
		t.Errorf("journal comment events = %d, want 2: %q", n, body)
	}
}

// TestCommentRefusalsByteIdentical pins the empty-payload and ATX-heading
// refusals (DS6/DS9): exit 1, the localized reason (with the offending line
// number for a heading), and the file+journal left byte-identical.
func TestCommentRefusalsByteIdentical(t *testing.T) {
	dir := writeTestDir(t)
	seedWriteTicket(t, dir)
	before := readWriteTicket(t, dir)
	cases := []struct {
		name    string
		payload string
		wantSub string
	}{
		{"empty", "", "must not be empty"},
		{"heading line 1", "## Journal", "payload line 1"},
		{"heading line 2", "ok text\n### heading", "payload line 2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withStdin(t, strings.NewReader(tc.payload))
			_, serr, code := writeRun(t, dir, "en", "comment", "1", "-")
			if code != 1 || !strings.Contains(serr, tc.wantSub) {
				t.Fatalf("code=%d stderr=%q, want %q", code, serr, tc.wantSub)
			}
			if after := readWriteTicket(t, dir); after != before {
				t.Error("refusal mutated the ticket")
			}
		})
	}
	// A '#' inside a line is NOT a heading and is accepted.
	withStdin(t, strings.NewReader("see issue #42"))
	if _, serr, code := writeRun(t, dir, "en", "comment", "1", "-"); code != 0 {
		t.Errorf("mid-line '#' refused: code=%d stderr=%q", code, serr)
	}
}

// TestCommentNotFound pins the hinted not-found report (exit 1).
func TestCommentNotFound(t *testing.T) {
	dir := writeTestDir(t)
	seedWriteTicket(t, dir)
	withStdin(t, strings.NewReader("x"))
	_, serr, code := writeRun(t, dir, "en", "comment", "42", "-")
	if code != 1 || !strings.Contains(serr, "not found") {
		t.Errorf("code=%d stderr=%q", code, serr)
	}
}

// TestCommentAmbiguous pins exit 2 with every conflicting path (T-0157).
func TestCommentAmbiguous(t *testing.T) {
	dir := writeTestDir(t)
	seedWriteTicket(t, dir)
	src := filepath.Join(dir, "T-0001-open.md")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "T-0001-wip.md"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	withStdin(t, strings.NewReader("x"))
	_, serr, code := writeRun(t, dir, "en", "comment", "1", "-")
	if code != 2 {
		t.Fatalf("ambiguous code=%d, want 2; stderr=%q", code, serr)
	}
	if !strings.Contains(serr, "T-0001-open.md") || !strings.Contains(serr, "T-0001-wip.md") {
		t.Errorf("ambiguous report must list every conflicting path: %q", serr)
	}
}

// TestCommentDeleted pins the deleted/ read-only refusal (exit 1, file kept).
func TestCommentDeleted(t *testing.T) {
	dir := writeTestDir(t)
	seedWriteTicket(t, dir)
	if _, serr, code := writeRun(t, dir, "en", "delete", "1", "--yes"); code != 0 {
		t.Fatalf("delete: code=%d stderr=%q", code, serr)
	}
	withStdin(t, strings.NewReader("x"))
	_, serr, code := writeRun(t, dir, "en", "comment", "1", "-")
	if code != 1 || !strings.Contains(serr, "read-only") {
		t.Errorf("deleted comment: code=%d stderr=%q", code, serr)
	}
}

// TestCommentErrorsLocalizeToEN runs the error paths under TICKET_LANG=en and
// asserts the EN substring with no Cyrillic leak (case 14).
func TestCommentErrorsLocalizeToEN(t *testing.T) {
	dir := writeTestDir(t)
	seedWriteTicket(t, dir)
	withStdin(t, strings.NewReader(""))
	_, serr, code := writeRun(t, dir, "en", "comment", "1", "-")
	if code != 1 || !strings.Contains(serr, "comment payload must not be empty") {
		t.Fatalf("EN empty: code=%d stderr=%q", code, serr)
	}
	for _, r := range serr {
		if r >= 0x0400 && r <= 0x04FF {
			t.Fatalf("Cyrillic leak in EN message: %q", serr)
		}
	}
}
