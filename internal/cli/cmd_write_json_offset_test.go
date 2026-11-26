package cli

// T-0244 C1: document-relative offsets for the strict --json contract. The
// locator walks the original document, so a duplicate "text" (map last-wins),
// an escaped key spelling, whitespace and escapes around the value point at
// the SELECTED value. Expected positions are computed from the input bytes
// (a unique marker in the document), never hard-coded.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// valueStartOffset returns the one-based, document-relative byte position
// of the first byte of the selected value, derived from the original
// document and the value marker alone: null and non-string values share the
// same anchor (T-0246). It deliberately does not reuse the locator's
// arithmetic, so a wrong base position is caught.
func valueStartOffset(t *testing.T, doc, marker string, last bool) int64 {
	t.Helper()
	return markerOffset(t, doc, marker, last) + 1
}

// markerOffset finds the marker in doc, honoring last for duplicate values.
func markerOffset(t *testing.T, doc, marker string, last bool) int64 {
	t.Helper()
	data := []byte(doc)
	idx := bytes.Index(data, []byte(marker))
	if last {
		idx = bytes.LastIndex(data, []byte(marker))
	}
	if idx < 0 {
		t.Fatalf("marker %q not found in %q", marker, doc)
	}
	return int64(idx)
}

// TestLocateJSONField pins the locator: the start of the SELECTED value —
// last top-level occurrence, decoded (escaped) key comparison, escapes and
// whitespace inside or around preceding values are consumed wholesale.
func TestLocateJSONField(t *testing.T) {
	cases := []struct {
		name   string
		doc    string
		marker string
		last   bool
	}{
		{"simple", `{"text":123}`, "123", false},
		{"leading whitespace", "  {\n \t \"text\" : \t 123 }", "123", false},
		{"escaped key", `{"t\u0065xt":true}`, "true", false},
		{"escape in preceding value", `{"a":"x\"y","text":[1]}`, "[1]", false},
		{"preceding other fields", `{"a":1,"b":{"c":[1,2]},"text":false}`, "false", false},
		{"nested key ignored", `{"a":{"text":1},"text":123}`, "123", false},
		{"duplicate identical raw", `{"text":123,"text":123}`, "123", true},
		{"duplicate different raw", `{"text":"ok","text":123}`, "123", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, err := locateJSONField([]byte(tc.doc), "text")
			if err != nil {
				t.Fatalf("locateJSONField: %v", err)
			}
			if want := markerOffset(t, tc.doc, tc.marker, tc.last); start != want {
				t.Errorf("start=%d, want %d", start, want)
			}
		})
	}
	for _, doc := range []string{`{"a":1}`, `[1,2]`, `null`, `{"text":`} {
		if _, err := locateJSONField([]byte(doc), "text"); err == nil {
			t.Errorf("doc %q: want error", doc)
		}
	}
}

// TestDecodeWriteJSONOffset pins the document-relative refusal offsets for a
// non-string and a null selected value, including duplicate last-wins.
func TestDecodeWriteJSONOffset(t *testing.T) {
	typeCases := []struct {
		name   string
		doc    string
		marker string
		last   bool
	}{
		{"number", `{"text":123}`, "123", false},
		{"number whitespace", "  {\n \"text\" : \t 123 }", "123", false},
		{"object", `{"text":{"a":1}}`, `{"a":1}`, false},
		{"array", `{"text":[1,2]}`, "[1,2]", false},
		{"bool", `{"text":true}`, "true", false},
		{"escaped key", `{"t\u0065xt":123}`, "123", false},
		{"escape in preceding value", `{"text":"x\"y","text":123}`, "123", false},
		{"duplicate identical raw", `{"text":123,"text":123}`, "123", true},
		{"duplicate different raw", `{"text":"ok","text":123}`, "123", false},
	}
	for _, tc := range typeCases {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if _, ok := decodeWriteJSON([]byte(tc.doc), domain.LangEN, &stderr); ok {
				t.Fatal("decodeWriteJSON accepted a non-string text")
			}
			want := valueStartOffset(t, tc.doc, tc.marker, tc.last)
			if !strings.Contains(stderr.String(), fmt.Sprintf("at byte %d: ", want)) {
				t.Errorf("stderr=%q, want offset %d", stderr.String(), want)
			}
		})
	}
	nullCases := []struct {
		name string
		doc  string
		last bool
	}{
		{"null", `{"text":null}`, false},
		{"duplicate null", `{"text":null,"text":null}`, true},
	}
	for _, tc := range nullCases {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if _, ok := decodeWriteJSON([]byte(tc.doc), domain.LangEN, &stderr); ok {
				t.Fatal("decodeWriteJSON accepted a null text")
			}
			// A selected null is reported at the same anchor as a
			// non-string value: the first byte of the value itself.
			want := valueStartOffset(t, tc.doc, "null", tc.last)
			if !strings.Contains(stderr.String(), fmt.Sprintf("at byte %d: ", want)) {
				t.Errorf("stderr=%q, want offset %d", stderr.String(), want)
			}
		})
	}
}

// TestDecodeWriteJSONSyntaxBeforeLookup pins that a malformed value is a JSON
// parse refusal, never a lookup report: the primary Unmarshal fails before the
// locator runs, so the offset is the offending byte of the original document.
func TestDecodeWriteJSONSyntaxBeforeLookup(t *testing.T) {
	cases := []struct {
		doc    string
		marker string
		delta  int64
	}{
		{`{"text":}`, "}", 1},    // value missing entirely
		{`{"text":1x}`, "1x", 2}, // number missing its terminator
	}
	for _, tc := range cases {
		t.Run(tc.doc, func(t *testing.T) {
			var stderr bytes.Buffer
			if _, ok := decodeWriteJSON([]byte(tc.doc), domain.LangEN, &stderr); ok {
				t.Fatal("decodeWriteJSON accepted malformed JSON")
			}
			msg := stderr.String()
			want := markerOffset(t, tc.doc, tc.marker, true) + tc.delta
			if !strings.Contains(msg, fmt.Sprintf("at byte %d: ", want)) {
				t.Errorf("stderr=%q, want offset %d", msg, want)
			}
			if !strings.Contains(msg, "invalid character") {
				t.Errorf("stderr=%q, want the JSON parse cause", msg)
			}
			for _, unwanted := range []string{"missing required field", "must be a string"} {
				if strings.Contains(msg, unwanted) {
					t.Errorf("stderr=%q, want no lookup report %q", msg, unwanted)
				}
			}
		})
	}
}

// dirFileNames snapshots the directory content to prove no temp file survives.
func dirFileNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// TestWriteJSONOffsetCLI pins the public refusal through comment and
// section set, stdin and --file: exit 1, empty stdout, the document-relative
// offset, the ticket byte-identical and no temp files left behind.
func TestWriteJSONOffsetCLI(t *testing.T) {
	doc := "  {\n \"text\" : \t 12345 }"
	want := valueStartOffset(t, doc, "12345", false)
	nullDoc := `{"text":null}`
	nullWant := valueStartOffset(t, nullDoc, "null", false)
	jsonFile := func(t *testing.T, dir, payload string) string {
		t.Helper()
		f := filepath.Join(dir, "payload.json")
		if err := os.WriteFile(f, []byte(payload), 0o644); err != nil {
			t.Fatal(err)
		}
		return f
	}
	for _, tc := range []struct {
		name string
		args []string
		want int64
	}{
		{"comment stdin", []string{"comment", "1", "-", "--json"}, want},
		{"comment file", []string{"comment", "1", "--file", "FILE", "--json"}, want},
		{"section stdin", []string{"section", "set", "1", "details", "-", "--json"}, want},
		{"section file", []string{"section", "set", "1", "details", "--file", "FILE", "--json"}, want},
		{"null comment", []string{"comment", "1", "-", "--json"}, nullWant},
		{"null section", []string{"section", "set", "1", "details", "-", "--json"}, nullWant},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeTestDir(t)
			seedWriteTicket(t, dir)
			before := readWriteTicket(t, dir)
			payload := doc
			if tc.want == nullWant {
				payload = nullDoc
			}
			args := slices.Clone(tc.args)
			for i, a := range args {
				if a == "FILE" {
					args[i] = jsonFile(t, dir, payload)
				}
			}
			// Snapshot AFTER the payload file exists: the file-list check
			// targets temp files leaked by the refusal, not test fixtures.
			filesBefore := dirFileNames(t, dir)
			if tc.name != "comment file" && tc.name != "section file" {
				withStdin(t, strings.NewReader(payload))
			} else {
				withStdin(t, strings.NewReader("unused"))
			}
			stdout, serr, code := writeRun(t, dir, "en", args...)
			if code != 1 {
				t.Fatalf("code=%d stderr=%q", code, serr)
			}
			if stdout != "" {
				t.Errorf("stdout=%q, want empty", stdout)
			}
			if !strings.Contains(serr, fmt.Sprintf("at byte %d: ", tc.want)) {
				t.Errorf("stderr=%q, want offset %d", serr, tc.want)
			}
			if after := readWriteTicket(t, dir); after != before {
				t.Error("a JSON refusal mutated the ticket")
			}
			if !slices.Equal(dirFileNames(t, dir), filesBefore) {
				t.Error("refusal left extra files behind")
			}
		})
	}
}

// TestWriteJSONEscapedAccepted pins that an accepted escaped payload decodes
// to the expected Unicode string via stdin and --file.
func TestWriteJSONEscapedAccepted(t *testing.T) {
	doc := `{"text":"line1\nline2 \u0041\u00e9"}`
	want := "line1\nline2 Aé"
	t.Run("stdin", func(t *testing.T) {
		dir := writeTestDir(t)
		seedWriteTicket(t, dir)
		withStdin(t, strings.NewReader(doc))
		if _, serr, code := writeRun(t, dir, "en", "comment", "1", "-", "--json"); code != 0 || serr != "" {
			t.Fatalf("code=%d stderr=%q", code, serr)
		}
		if body := readWriteTicket(t, dir); !strings.Contains(body, want) {
			t.Errorf("decoded payload missing: %q", body)
		}
	})
	t.Run("file", func(t *testing.T) {
		dir := writeTestDir(t)
		seedWriteTicket(t, dir)
		f := filepath.Join(dir, "payload.json")
		if err := os.WriteFile(f, []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
		withStdin(t, strings.NewReader("unused"))
		if _, serr, code := writeRun(t, dir, "en", "section", "set", "1", "details", "--file", f, "--json"); code != 0 || serr != "" {
			t.Fatalf("code=%d stderr=%q", code, serr)
		}
		if body := readWriteTicket(t, dir); !strings.Contains(body, want) {
			t.Errorf("decoded payload missing: %q", body)
		}
	})
}
