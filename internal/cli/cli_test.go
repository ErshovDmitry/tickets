package cli_test

import (
	"bytes"
	_ "embed"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/cli"
)

// usageFixture is the byte-exact expected usage text. Production usage.go
// embeds the same file via //go:embed; tests treat templates/usage.ru.txt
// as a byte fixture and assert stdout equal to wantUsage(usageFixture):
// the version line followed by this fixture verbatim (tests run with a
// lang-less env, so usage falls back to the Russian text).
//
//go:embed templates/usage.ru.txt
var usageFixture []byte

// wantUsage returns the expected stdout for usage-printing paths in the
// default (dev) build: the version line followed by the embedded fixture
// verbatim (usage() prepends versionLine() at runtime, T-0028). Black-box
// tests cannot call the unexported versionLine, hence the literal.
//
// 🔴 The literal pins the DEFAULT "dev" build. Running the tests with a
// release version override, e.g.
//
//	go test -ldflags "-X ticket/internal/cli.version=1.2.3" ./internal/cli/
//
// will FALSELY FAIL these tests (expected stdout still says "dev" while
// the binary prints "1.2.3"). That is a known limitation, not a bug: run
// the suite without the override, or re-pin the literal for such runs.
func wantUsage(fixture []byte) string {
	return "ticket version dev\n" + string(fixture)
}

// testEnv returns a Run env map pointing TICKETS_DIR at a fresh temp dir.
func testEnv(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{"TICKETS_DIR": t.TempDir()}
}

// TestRunHelpRouting checks the bash contract: no args and help variants
// print the version line followed by the usage text verbatim to stdout and
// exit 0 (bash:147,153).
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
			want := wantUsage(usageFixture)
			if got := stdout.String(); got != want {
				t.Fatalf("stdout is not the version line + verbatim usage text (got %d bytes, want %d)", len(got), len(want))
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

// TestSubcommandHelp pins the T-0041 central interception: -h/--help as
// the first argument of any subcommand prints the usage text byte-for-byte
// to stdout, exits 0 and creates no ticket. Boundary cases: `new --help
// extra` still intercepts (only args[1] matters), while `new -` is NOT a
// help flag — it falls through to the dash-title rejection (T-0041).
func TestSubcommandHelp(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantCode  int
		wantUsage bool
	}{
		{"new short", []string{"new", "-h"}, 0, true},
		{"new long", []string{"new", "--help"}, 0, true},
		{"list short", []string{"list", "-h"}, 0, true},
		{"list long", []string{"list", "--help"}, 0, true},
		{"show short", []string{"show", "-h"}, 0, true},
		{"show long", []string{"show", "--help"}, 0, true},
		{"set short", []string{"set", "-h"}, 0, true},
		{"set long", []string{"set", "--help"}, 0, true},
		{"archive short", []string{"archive", "-h"}, 0, true},
		{"archive long", []string{"archive", "--help"}, 0, true},
		{"help extra arg", []string{"new", "--help", "extra"}, 0, true},
		{"dash is not help", []string{"new", "-"}, 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			var stdout, stderr bytes.Buffer
			code := cli.Run(tc.args, map[string]string{"TICKETS_DIR": dir}, &stdout, &stderr)
			if code != tc.wantCode {
				t.Fatalf("Run(%q) = %d, want %d; stderr: %q", tc.args, code, tc.wantCode, stderr.String())
			}
			if tc.wantUsage {
				if got := stdout.String(); got != wantUsage(usageFixture) {
					t.Fatalf("stdout is not the version line + verbatim usage text (got %d bytes)", len(got))
				}
				if stderr.Len() != 0 {
					t.Fatalf("stderr = %q, want empty", stderr.String())
				}
			} else if !strings.Contains(stderr.String(), "не может начинаться с '-'") {
				t.Fatalf("stderr %q missing the dash-title error", stderr.String())
			}
			if created, _ := filepath.Glob(filepath.Join(dir, "T-*.md")); len(created) != 0 {
				t.Fatalf("ticket file created: %v", created)
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
// version line followed by the usage text to stdout and exits 1 without
// writing to stderr (bash:154).
func TestRunUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := cli.Run([]string{"frobnicate"}, testEnv(t), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(unknown) = %d, want 1", code)
	}
	want := wantUsage(usageFixture)
	if got := stdout.String(); got != want {
		t.Fatalf("stdout is not the version line + verbatim usage text (got %d bytes, want %d)", len(got), len(want))
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}
