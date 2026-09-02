package cli_test

import (
	"bytes"
	_ "embed"
	"strings"
	"testing"

	"ticket/internal/cli"
)

// usageFixture is the byte-exact expected usage text. Production usage.go
// embeds the same file via //go:embed; tests treat templates/usage.txt as a
// byte fixture and assert verbatim stdout output.
//
//go:embed templates/usage.txt
var usageFixture []byte

// testEnv returns a Run env map pointing TICKETS_DIR at a fresh temp dir.
func testEnv(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{"TICKETS_DIR": t.TempDir()}
}

// TestRunHelpRouting checks the bash contract: no args and help variants
// print the usage text verbatim to stdout and exit 0 (bash:147,153).
func TestRunHelpRouting(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no args", nil},
		{"empty args", []string{}},
		{"help", []string{"help"}},
		{"short flag", []string{"-h"}},
		{"long flag", []string{"--help"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := cli.Run(tc.args, testEnv(t), &stdout, &stderr)
			if code != 0 {
				t.Fatalf("Run(%q) = %d, want 0", tc.args, code)
			}
			if got := stdout.String(); got != string(usageFixture) {
				t.Fatalf("stdout is not the verbatim usage text (got %d bytes, want %d)", len(got), len(usageFixture))
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

// TestListEmptyFilterDefaultsActive pins the bash parity fix: `list ""`
// must behave as the default active filter (bash want="${1:-active}")
// instead of failing as an invalid filter.
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

// TestRunUnknownCommand checks the bash contract: unknown command prints the
// usage text to stdout and exits 1 without writing to stderr (bash:154).
func TestRunUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := cli.Run([]string{"frobnicate"}, testEnv(t), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(unknown) = %d, want 1", code)
	}
	if got := stdout.String(); got != string(usageFixture) {
		t.Fatalf("stdout is not the verbatim usage text (got %d bytes, want %d)", len(got), len(usageFixture))
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}
