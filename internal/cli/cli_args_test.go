package cli_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/cli"
)

// TestListEmptyFilterDefaultsActive pins the empty-filter contract:
// `list ""` must behave as the default active filter (an empty argument
// counts as unset) instead of failing as an invalid filter.
func TestListEmptyFilterDefaultsActive(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}

	// Empty store: the empty-result message must name the ACTIVE filter —
	// an invalid-filter rejection or a literal "" filter would fail here.
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"list", ""}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(list, \"\") = %d, want 0; stderr: %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
	if got, want := stdout.String(), "Нет тикетов (active).\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}

	// With one open ticket the empty filter lists it.
	stdout.Reset()
	args := []string{"new", "Тикет для list", "-t", "BUG", "-w", "тестер"}
	if code := cli.Run(args, env, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(new) = %d, want 0; stderr: %q", code, stderr.String())
	}
	stdout.Reset()
	if code := cli.Run([]string{"list", ""}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(list, \"\") = %d, want 0; stderr: %q", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "T-0001") || !strings.Contains(out, "BUG: Тикет для list") {
		t.Errorf("list output %q missing the open ticket", out)
	}
}

// TestGlobalFlagRequiresValue pins T-0067: a leading -C/--tickets-dir flag
// with no value, or with an empty value, must fail fast BEFORE command
// routing (exit 1, one-line stderr, EMPTY stdout) instead of silently
// degrading to help / falling through to scanUpward. Error cases assert
// exit code 1, EMPTY stdout, and stderr contains BOTH the required
// substring AND the actual flag spelling the user typed. Positive
// controls exercise every valid flag form (-Cpath glued, -C path,
// --tickets-dir path, --tickets-dir=path) against a real temp dir with
// TICKETS_DIR explicitly absent, proving the flag path itself works.
func TestGlobalFlagRequiresValue(t *testing.T) {
	tmp := t.TempDir()
	// Empty env — no TICKETS_DIR — forces the flag to be the only source
	// of truth, so a positive control that exits 0 proves the flag was
	// honored end-to-end (and not rescued by env or scanUpward).
	env := map[string]string{}

	cases := []struct {
		name         string
		args         []string
		wantSpelling string // flag spelling that MUST appear in stderr (error cases only)
		wantSubstr   string // message substring that MUST appear in stderr
	}{
		// Error cases: flag with no value or empty value.
		{"trailing short", []string{"-C"}, "-C", "requires a value"},
		{"trailing long", []string{"--tickets-dir"}, "--tickets-dir", "requires a value"},
		{"equals empty", []string{"--tickets-dir="}, "--tickets-dir", "non-empty value"},
		{"short with empty", []string{"-C", ""}, "-C", "non-empty value"},
		{"long with empty", []string{"--tickets-dir", ""}, "--tickets-dir", "non-empty value"},
		// Positive controls: every valid form must still resolve the dir.
		{"glued path", []string{"-C" + tmp, "list"}, "", ""},
		{"short space", []string{"-C", tmp, "list"}, "", ""},
		{"long space", []string{"--tickets-dir", tmp, "list"}, "", ""},
		{"long equals", []string{"--tickets-dir=" + tmp, "list"}, "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := cli.Run(tc.args, env, &stdout, &stderr)
			if tc.wantSubstr == "" {
				// Positive control: flag worked, command (list) ran cleanly.
				if code != 0 {
					t.Fatalf("Run(%q) = %d, want 0; stderr: %q", tc.args, code, stderr.String())
				}
				if stderr.Len() != 0 {
					t.Fatalf("Run(%q) stderr = %q, want empty", tc.args, stderr.String())
				}
				return
			}
			// Error case: One-line stderr, EMPTY stdout, exit 1, and the
			// message must name the offending flag spelling verbatim.
			if code != 1 {
				t.Fatalf("Run(%q) = %d, want 1; stderr: %q", tc.args, code, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("Run(%q) stdout = %q, want EMPTY (no usage dump on flag error)", tc.args, stdout.String())
			}
			serr := stderr.String()
			if !strings.Contains(serr, tc.wantSubstr) {
				t.Errorf("Run(%q) stderr %q missing substring %q", tc.args, serr, tc.wantSubstr)
			}
			if !strings.Contains(serr, tc.wantSpelling) {
				t.Errorf("Run(%q) stderr %q missing flag spelling %q", tc.args, serr, tc.wantSpelling)
			}
		})
	}
}

// TestRunRejectsNewlineInWho pins T-0065: a TICKET_WHO env value
// containing CR or LF must be rejected BEFORE any command is
// dispatched (defense-in-depth — the store also guards). The
// rejection uses the localized "comment/who must not contain a line
// break" message (parity with the T-0044 title check), exits 1, and
// creates no ticket file. We exercise every command that takes `who`
// (new / set / archive) via a single `new` call (the cheapest path
// that flows through dispatch + whoFrom), and also confirm the
// `set` path: who reaches cmdSet via dispatch, not via flag parsing.
func TestRunRejectsNewlineInWho(t *testing.T) {
	cases := []struct {
		name string
		who  string
		args []string
	}{
		// who with bare LF — newline injection into the journal line.
		{"new LF", "evil\n- forged", []string{"new", "x"}},
		{"set LF", "evil\n- forged", []string{"set", "1", "wip", "go"}},
		// who with bare CR.
		{"new CR", "evil\rforged", []string{"new", "x"}},
		// who with CRLF.
		{"new CRLF", "evil\r\n- forged", []string{"new", "x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			env := map[string]string{
				"TICKETS_DIR": dir,
				"TICKET_WHO":  tc.who,
			}
			var stdout, stderr bytes.Buffer
			code := cli.Run(tc.args, env, &stdout, &stderr)
			if code != 1 {
				t.Fatalf("Run(%v, who=%q) = %d, want 1; stderr: %q", tc.args, tc.who, code, stderr.String())
			}
			serr := stderr.String()
			// Localized RU message (default langFrom falls back to RU).
			if !strings.Contains(serr, "комментарий/автор не может содержать перевод строки") {
				t.Errorf("Run(... who=%q) stderr %q missing the localized RU rejection", tc.who, serr)
			}
			// No ticket file may exist for the `new` case (others would
			// also fail before any write because whoFrom validation
			// runs in dispatch before the command handler).
			matches, err := filepath.Glob(filepath.Join(dir, "T-*.md"))
			if err != nil {
				t.Fatalf("glob: %v", err)
			}
			if len(matches) != 0 {
				t.Errorf("Run(... who=%q) created ticket files: %v", tc.who, matches)
			}
		})
	}
}

// TestRunRejectsNewlineInWhoEN pins the T-0065 EN-locale path: a
// TICKET_LANG=en env yields the English message, both for parity
// with the EN help text and so the test is hermetic under any
// default-locale assumption.
func TestRunRejectsNewlineInWhoEN(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{
		"TICKETS_DIR": dir,
		"TICKET_LANG": "en",
		"TICKET_WHO":  "evil\n- forged",
	}
	var stdout, stderr bytes.Buffer
	code := cli.Run([]string{"new", "x"}, env, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(new, who=LF) = %d, want 1; stderr: %q", code, stderr.String())
	}
	if got, want := stderr.String(), "ticket: comment/who must not contain a line break\n"; got != want {
		t.Errorf("EN stderr = %q, want %q", got, want)
	}
}
