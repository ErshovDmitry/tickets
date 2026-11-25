package cli

// T-0240 A2 CLI tests: the shared write-input machinery (source selection,
// flag parsing, strict JSON) exercised through comment and section set.
// Command-specific success/rendering lives in cmd_comment_test.go and
// cmd_section_test.go; i18n validation of the new keys is covered by
// i18n_dict_validate_test.go (counter + stringFieldSetters).

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestDir returns a symlink-resolved temp dir (the binary prints
// EvalSymlinks-resolved paths).
func writeTestDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("evalsymlinks: %v", err)
	}
	return dir
}

// writeRun runs one command against dir with the given locale ("" = default
// RU) and returns both streams and the exit code.
func writeRun(t *testing.T, dir, lang string, args ...string) (string, string, int) {
	t.Helper()
	env := map[string]string{"TICKETS_DIR": dir}
	if lang != "" {
		env["TICKET_LANG"] = lang
	}
	var stdout, stderr bytes.Buffer
	code := Run(args, env, &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

// seedWriteTicket creates T-0001-open.md in dir.
func seedWriteTicket(t *testing.T, dir string) {
	t.Helper()
	if _, serr, code := writeRun(t, dir, "", "new", "Тестовый тикет", "-t", "BUG"); code != 0 {
		t.Fatalf("seed new = %d; stderr %q", code, serr)
	}
}

// readWriteTicket reads the current T-0001 file (status-agnostic).
func readWriteTicket(t *testing.T, dir string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "T-0001-*.md"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("glob T-0001: matches=%v err=%v", matches, err)
	}
	body, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	return string(body)
}

// TestWriteSourceSelection pins DS3: exactly one of - / --file; zero or both
// is a usage error (exit 1) and the terminal is never read (a failing stdin
// reader proves no read was attempted). The ticket stays byte-identical.
func TestWriteSourceSelection(t *testing.T) {
	dir := writeTestDir(t)
	seedWriteTicket(t, dir)
	before := readWriteTicket(t, dir)

	withStdin(t, failingReader{errors.New("terminal must not be read")})
	_, serr, code := writeRun(t, dir, "en", "comment", "1")
	if code != 1 || !strings.Contains(serr, "exactly one payload source") {
		t.Errorf("no source: code=%d stderr=%q", code, serr)
	}

	f := filepath.Join(dir, "p.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	withStdin(t, strings.NewReader("x"))
	_, serr, code = writeRun(t, dir, "en", "comment", "1", "-", "--file", f)
	if code != 1 || !strings.Contains(serr, "exactly one payload source") {
		t.Errorf("both sources: code=%d stderr=%q", code, serr)
	}
	if after := readWriteTicket(t, dir); after != before {
		t.Error("a source refusal mutated the ticket")
	}
}

// TestWriteFlagParsing pins the parseEditFlags style: only separate pairs,
// glued/short/-- separator refused, a bare positional is unknown argument, a
// missing --file value is reported. All exit 1 before any write.
func TestWriteFlagParsing(t *testing.T) {
	dir := writeTestDir(t)
	seedWriteTicket(t, dir)
	withStdin(t, strings.NewReader("payload"))
	cases := []struct {
		name    string
		args    []string
		wantSub string
	}{
		{"glued", []string{"comment", "1", "--json=true"}, "unknown flag for comment command: --json=true"},
		{"short", []string{"comment", "1", "-j"}, "unknown flag for comment command: -j"},
		{"separator", []string{"comment", "1", "--"}, "unknown flag for comment command: --"},
		{"unknown", []string{"comment", "1", "--bogus", "x"}, "unknown flag for comment command: --bogus"},
		{"extra positional", []string{"comment", "1", "-", "extra"}, "unknown argument: extra"},
		{"missing file value", []string{"comment", "1", "--file"}, "flag --file requires a value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, serr, code := writeRun(t, dir, "en", tc.args...)
			if code != 1 || !strings.Contains(serr, tc.wantSub) {
				t.Errorf("code=%d stderr=%q, want substring %q", code, serr, tc.wantSub)
			}
		})
	}
}

// TestWriteJSONStrict pins DS4/DS5: invalid JSON (single quotes, trailing
// comma, trailing garbage), a missing/non-string text, top-level null and a
// null text value all exit 1 with position + expected format + example and
// NO plain-text fallback; unknown fields are listed. The ticket stays
// byte-identical.
func TestWriteJSONStrict(t *testing.T) {
	dir := writeTestDir(t)
	seedWriteTicket(t, dir)
	before := readWriteTicket(t, dir)
	cases := []struct {
		name    string
		payload string
		wantSub string
	}{
		{"single quotes", `{'text':'x'}`, "invalid JSON payload at byte"},
		{"trailing comma", `{"text":"x",}`, "invalid JSON payload at byte"},
		{"trailing garbage", `{"text":"x"} junk`, "invalid JSON payload at byte"},
		{"empty object", `{}`, "invalid JSON payload"},
		{"null", `null`, "invalid JSON payload"},
		{"non-string text", `{"text":123}`, "invalid JSON payload"},
		{"text null", `{"text":null}`, "invalid JSON payload"},
		{"unknown fields", `{"text":"x","foo":1,"bar":2}`, "unknown JSON fields: bar, foo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withStdin(t, strings.NewReader(tc.payload))
			_, serr, code := writeRun(t, dir, "en", "comment", "1", "-", "--json")
			if code != 1 || !strings.Contains(serr, tc.wantSub) {
				t.Fatalf("code=%d stderr=%q, want substring %q", code, serr, tc.wantSub)
			}
			if strings.Contains(serr, "json payload") {
				t.Errorf("fell back to plain text: %q", serr)
			}
		})
	}
	// The invalid-JSON message carries the expected format and a ready example.
	withStdin(t, strings.NewReader(`{'text':'x'}`))
	_, serr, _ := writeRun(t, dir, "en", "comment", "1", "-", "--json")
	if !strings.Contains(serr, `{"text":"<payload>"}`) || !strings.Contains(serr, "example") {
		t.Errorf("invalid-JSON message lacks format/example: %q", serr)
	}
	if after := readWriteTicket(t, dir); after != before {
		t.Error("a JSON refusal mutated the ticket")
	}
}

// TestWriteJSONSuccess pins the happy path of the format contract, from stdin
// and from --file, and that a repeated --file keeps the last value.
func TestWriteJSONSuccess(t *testing.T) {
	dir := writeTestDir(t)
	seedWriteTicket(t, dir)
	withStdin(t, strings.NewReader(`{"text":"json payload"}`))
	if _, serr, code := writeRun(t, dir, "en", "comment", "1", "-", "--json"); code != 0 {
		t.Fatalf("json stdin: code=%d stderr=%q", code, serr)
	}
	f := filepath.Join(dir, "p.json")
	if err := os.WriteFile(f, []byte(`{"text":"from file"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, serr, code := writeRun(t, dir, "en", "comment", "1", "--file", f, "--json"); code != 0 {
		t.Fatalf("json file: code=%d stderr=%q", code, serr)
	}
	body := readWriteTicket(t, dir)
	if !strings.Contains(body, "json payload") || !strings.Contains(body, "from file") {
		t.Errorf("json payloads not appended: %q", body)
	}
}

// TestWriteFileRepeatLastWins pins repeat = last value for --file.
func TestWriteFileRepeatLastWins(t *testing.T) {
	dir := writeTestDir(t)
	seedWriteTicket(t, dir)
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(a, []byte("AAA"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("BBB"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, serr, code := writeRun(t, dir, "en", "comment", "1", "--file", a, "--file", b); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, serr)
	}
	body := readWriteTicket(t, dir)
	if !strings.Contains(body, "BBB") || strings.Contains(body, "AAA") {
		t.Errorf("repeat --file did not keep the last value: %q", body)
	}
}

// TestWriteReadErrorsBeforeLookup pins DS9/case 10: an unreadable --file with
// a NONEXISTENT number reports the read error, not not-found; a failed stdin
// read is localized. Both exit 1 before any lookup/write.
func TestWriteReadErrorsBeforeLookup(t *testing.T) {
	dir := writeTestDir(t)
	seedWriteTicket(t, dir)
	_, serr, code := writeRun(t, dir, "en", "comment", "999", "--file", filepath.Join(dir, "missing.txt"))
	if code != 1 || !strings.Contains(serr, "cannot read payload file") || strings.Contains(serr, "not found") {
		t.Errorf("file read error must win over not-found: code=%d stderr=%q", code, serr)
	}
	withStdin(t, failingReader{errors.New("boom")})
	_, serr, code = writeRun(t, dir, "en", "comment", "1", "-")
	if code != 1 || !strings.Contains(serr, "cannot read payload from stdin") {
		t.Errorf("stdin read: code=%d stderr=%q", code, serr)
	}
}

// TestWriteEmptyFilePayload pins that an empty file is a valid empty payload:
// section set clears the section (exit 0), while comment refuses it (exit 1).
func TestWriteEmptyFilePayload(t *testing.T) {
	dir := writeTestDir(t)
	seedWriteTicket(t, dir)
	withStdin(t, strings.NewReader("some details"))
	if _, serr, code := writeRun(t, dir, "", "section", "set", "1", "details", "-"); code != 0 {
		t.Fatalf("seed details: code=%d stderr=%q", code, serr)
	}
	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, serr, code := writeRun(t, dir, "en", "section", "set", "1", "details", "--file", empty); code != 0 {
		t.Fatalf("empty file clear: code=%d stderr=%q", code, serr)
	}
	if body := readWriteTicket(t, dir); strings.Contains(body, "some details") {
		t.Errorf("empty payload did not clear the section: %q", body)
	}
	if _, serr, code := writeRun(t, dir, "en", "comment", "1", "--file", empty); code != 1 || !strings.Contains(serr, "must not be empty") {
		t.Errorf("empty comment: code=%d stderr=%q", code, serr)
	}
}
