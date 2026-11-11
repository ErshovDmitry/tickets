package cli_test

// Argument and usage tests for `normalize` (T-0125): the flag parser is the
// only place that reports a bad invocation, and both usage templates document
// the command in the CLI's own block position.

import (
	"strings"
	"testing"
)

// TestCmdNormalize_Help pins the T-0041 interception: -h as the first
// argument prints the usage text and exits 0 without touching the store.
func TestCmdNormalize_Help(t *testing.T) {
	env := normalizeEnv(t)
	stdout, stderr, code := runNormalizeCLI(t, []string{"normalize", "-h"}, env)
	if code != 0 {
		t.Fatalf("normalize -h = %d, want 0", code)
	}
	if !strings.Contains(stdout, "ticket normalize") {
		t.Errorf("usage does not document normalize:\n%s", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

// TestCmdNormalize_UnknownFlag pins the unknown-flag rejection: exit 1, empty
// stdout, and the offending flag named in the localized message.
func TestCmdNormalize_UnknownFlag(t *testing.T) {
	env := normalizeEnv(t)
	stdout, stderr, code := runNormalizeCLI(t, []string{"normalize", "--json"}, env)
	if code != 1 {
		t.Fatalf("normalize --json = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "--json") {
		t.Errorf("stderr = %q, want the flag name", stderr)
	}
}

// TestCmdNormalize_ProjectFlagErrors pins both -P failure modes: a bare flag
// and an empty value are rejected before any store access.
func TestCmdNormalize_ProjectFlagErrors(t *testing.T) {
	env := normalizeEnv(t)
	for _, args := range [][]string{
		{"normalize", "-P"},
		{"normalize", "-P", ""},
	} {
		stdout, stderr, code := runNormalizeCLI(t, args, env)
		if code != 1 {
			t.Fatalf("%v = %d, want 1", args, code)
		}
		if stdout != "" {
			t.Errorf("%v stdout = %q, want empty", args, stdout)
		}
		if !strings.Contains(stderr, "-P") {
			t.Errorf("%v stderr = %q, want the flag name", args, stderr)
		}
	}
}

// TestCmdNormalize_PositionalArgument pins that normalize has no positional
// form: a ticket number is a usage error, not a filter.
func TestCmdNormalize_PositionalArgument(t *testing.T) {
	env := normalizeEnv(t)
	stdout, stderr, code := runNormalizeCLI(t, []string{"normalize", "7"}, env)
	if code != 1 {
		t.Fatalf("normalize 7 = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "7") {
		t.Errorf("stderr = %q, want the offending argument", stderr)
	}
}

// TestNormalizeUsage pins both embedded templates: the RU and the EN help text
// document the command with its flags and the dry-run-then-real-run sequence.
func TestNormalizeUsage(t *testing.T) {
	dir := t.TempDir()
	ru := map[string]string{"TICKETS_DIR": dir, "TICKET_LANG": "ru"}
	en := map[string]string{"TICKETS_DIR": dir, "TICKET_LANG": "en"}

	ruOut, _, code := runNormalizeCLI(t, []string{"normalize", "-h"}, ru)
	if code != 0 {
		t.Fatalf("normalize -h (ru) = %d, want 0", code)
	}
	if !strings.Contains(ruOut, "ticket normalize [-P <проект>] [--dry-run]") {
		t.Errorf("RU usage is missing the normalize line:\n%s", ruOut)
	}
	if !strings.Contains(ruOut, "--dry-run печатает пути") {
		t.Errorf("RU usage is missing the dry-run description")
	}
	enOut, _, code := runNormalizeCLI(t, []string{"normalize", "-h"}, en)
	if code != 0 {
		t.Fatalf("normalize -h (en) = %d, want 0", code)
	}
	if !strings.Contains(enOut, "ticket normalize [-P <project>] [--dry-run]") {
		t.Errorf("EN usage is missing the normalize line:\n%s", enOut)
	}
	if !strings.Contains(enOut, "--dry-run prints the paths") {
		t.Errorf("EN usage is missing the dry-run description")
	}
}
