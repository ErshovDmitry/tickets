package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/cli"
)

// TestNewWhoResolution checks the who chain TICKET_WHO -> USER ->
// USERNAME -> agent (USERNAME covers Windows hosts) and that -w overrides
// the environment.
func TestNewWhoResolution(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		args    []string
		wantWho string
	}{
		{"ticket who env", map[string]string{"TICKET_WHO": "who-env"}, nil, "who-env"},
		{"user fallback", map[string]string{"USER": "who-user"}, nil, "who-user"},
		{"username fallback", map[string]string{"USERNAME": "who-win"}, nil, "who-win"},
		{"user beats username", map[string]string{"USER": "who-user", "USERNAME": "who-win"}, nil, "who-user"},
		{"empty username falls to agent", map[string]string{"USERNAME": ""}, nil, "agent"},
		{"agent fallback", map[string]string{}, nil, "agent"},
		{"flag overrides env", map[string]string{"TICKET_WHO": "who-env"}, []string{"-w", "who-flag"}, "who-flag"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.env["TICKETS_DIR"] = dir
			args := append([]string{"new", "who test"}, tc.args...)
			var stdout, stderr bytes.Buffer
			if code := cli.Run(args, tc.env, &stdout, &stderr); code != 0 {
				t.Fatalf("Run(%q) = %d, want 0; stderr: %q", args, code, stderr.String())
			}
			data, err := os.ReadFile(filepath.Join(dir, "T-0001-open.md"))
			if err != nil {
				t.Fatalf("created ticket is not readable: %v", err)
			}
			for _, want := range []string{"by (кем): " + tc.wantWho, "тикет создан (" + tc.wantWho + ")"} {
				if !strings.Contains(string(data), want) {
					t.Fatalf("created ticket missing %q:\n%s", want, data)
				}
			}
		})
	}
}

// firstLine returns the first line of path with its trailing LF removed.
func firstLine(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("created ticket is not readable: %v", err)
	}
	line, _, _ := bytes.Cut(data, []byte("\n"))
	return line
}

// TestNewTypePrefixNormalized pins T-0124: a title that already starts with
// the effective type prefix is stripped in a loop (case-blind, plain
// "<type>:" form) so the H1 never doubles it; only ASCII space and TAB
// directly after the colon are removed, the rest is kept byte-for-byte.
func TestNewTypePrefixNormalized(t *testing.T) {
	cases := []struct {
		name  string
		title string
		flags []string
		want  string
	}{
		{"plain title", "x", []string{"-t", "TD"}, "# T-0001 · TD: x"},
		{"same type prefix", "TD: x", []string{"-t", "TD"}, "# T-0001 · TD: x"},
		{"lowercase prefix", "td: x", []string{"-t", "TD"}, "# T-0001 · TD: x"},
		{"no space after colon", "TD:x", []string{"-t", "TD"}, "# T-0001 · TD: x"},
		{"repeated prefix", "TD: TD: x", []string{"-t", "TD"}, "# T-0001 · TD: x"},
		{"repeated mixed case", "td: Td: TD: x", []string{"-t", "TD"}, "# T-0001 · TD: x"},
		{"tab after colon", "TD:\tx", []string{"-t", "TD"}, "# T-0001 · TD: x"},
		{"tab after repeated prefix", "TD:\tTD:\tx", []string{"-t", "TD"}, "# T-0001 · TD: x"},
		{"nbsp after colon kept", "TD:\u00a0x", []string{"-t", "TD"}, "# T-0001 · TD: \u00a0x"},
		{"mismatched type kept", "BUG: x", []string{"-t", "TD"}, "# T-0001 · TD: BUG: x"},
		{"trailing whitespace kept", "TD: x  ", []string{"-t", "TD"}, "# T-0001 · TD: x  "},
		{"default type stripped", "BUG: x", nil, "# T-0001 · BUG: x"},
		{"default type lowercase", "bug: x", nil, "# T-0001 · BUG: x"},
		{"default type other prefix kept", "TD: x", nil, "# T-0001 · BUG: TD: x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			var stdout, stderr bytes.Buffer
			args := append([]string{"new", tc.title}, tc.flags...)
			if code := cli.Run(args, map[string]string{"TICKETS_DIR": dir}, &stdout, &stderr); code != 0 {
				t.Fatalf("Run(%q) = %d, want 0; stderr: %q", args, code, stderr.String())
			}
			if got := firstLine(t, filepath.Join(dir, "T-0001-open.md")); !bytes.Equal(got, []byte(tc.want)) {
				t.Fatalf("H1 = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestNewTypePrefixOnlyRejected pins T-0124: when stripping the prefix leaves
// an empty title the command exits 1 with the localized empty-title error and
// creates no ticket file.
func TestNewTypePrefixOnlyRejected(t *testing.T) {
	for _, title := range []string{"TD:", "TD:   ", "TD:\t", "td: TD:\t  "} {
		t.Run(title, func(t *testing.T) {
			dir := t.TempDir()
			var stdout, stderr bytes.Buffer
			code := cli.Run([]string{"new", title, "-t", "TD"}, map[string]string{"TICKETS_DIR": dir}, &stdout, &stderr)
			if code != 1 {
				t.Fatalf("Run(new %q -t TD) = %d, want 1; stderr: %q", title, code, stderr.String())
			}
			if !strings.Contains(stderr.String(), "краткое описание не может быть пустым") {
				t.Fatalf("stderr %q does not contain the empty-title error", stderr.String())
			}
			created, err := filepath.Glob(filepath.Join(dir, "T-*.md"))
			if err != nil {
				t.Fatalf("glob: %v", err)
			}
			if len(created) != 0 {
				t.Fatalf("ticket file created for a prefix-only title: %v", created)
			}
		})
	}
}
