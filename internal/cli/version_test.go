package cli

// White-box tests for the version command and the version-prefixed usage
// (T-0028). package cli (not cli_test) so versionLine and the unexported
// version/usageText* identifiers are reachable; coexists with the
// black-box cli_test package in this directory.

import (
	"bytes"
	"strings"
	"testing"
)

// TestVersionLine pins the default-build version line: the version variable
// defaults to "dev" (release builds override it via -ldflags, version.go).
func TestVersionLine(t *testing.T) {
	if got, want := versionLine(), "ticket version dev\n"; got != want {
		t.Fatalf("versionLine() = %q, want %q", got, want)
	}
}

// TestRunVersionRouting checks the bash contract for `version`, `--version`
// and `-v`: all print the version line to stdout, exit 0, write nothing to
// stderr (bash:155; cli.go routes them before dispatch).
func TestRunVersionRouting(t *testing.T) {
	for _, arg := range []string{"version", "--version", "-v"} {
		var stdout, stderr bytes.Buffer
		code := Run([]string{arg}, map[string]string{}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("Run(%q) = %d, want 0", arg, code)
		}
		// Pinned literal (see TestVersionLine): comparing against
		// versionLine() here would be a tautology.
		if got, want := stdout.String(), "ticket version dev\n"; got != want {
			t.Fatalf("Run(%q) stdout = %q, want %q", arg, got, want)
		}
		if stderr.Len() != 0 {
			t.Fatalf("Run(%q) stderr = %q, want empty", arg, stderr.String())
		}
	}
}

// TestRunVersionIgnoresExtraArgs pins the Go-port decision for arguments
// after a version flag: the bash reference has NO version command, so this
// behavior is ours to define — extra args are ignored by design, the
// version line is printed and the exit code stays 0 (cli.go routes on
// args[0] only).
func TestRunVersionIgnoresExtraArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"version with positional arg", []string{"version", "extra"}},
		{"-v with junk arg", []string{"-v", "junk"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(tc.args, map[string]string{}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("Run(%q) = %d, want 0", tc.args, code)
			}
			if got, want := stdout.String(), "ticket version dev\n"; got != want {
				t.Fatalf("Run(%q) stdout = %q, want %q", tc.args, got, want)
			}
			if stderr.Len() != 0 {
				t.Fatalf("Run(%q) stderr = %q, want empty", tc.args, stderr.String())
			}
		})
	}
}

// TestUsageStartsWithVersionLine pins the T-0028 change: usage() prepends
// the version line to the embedded usage text (the templates themselves
// stay byte-identical to the bash reference and contain no version line).
func TestUsageStartsWithVersionLine(t *testing.T) {
	cases := []struct {
		name string
		lang string
		text string
	}{
		{"ru", "ru", usageTextRU},
		{"en", "en", usageTextEN},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			usage(&buf, tc.lang)
			got := buf.String()
			// Pinned literal (see TestVersionLine): expecting versionLine()
			// here would make the assertion tautological.
			if !strings.HasPrefix(got, "ticket version dev\n") {
				t.Fatalf("usage(%q) output %q does not start with version line %q", tc.lang, got, "ticket version dev\n")
			}
			if rest := strings.TrimPrefix(got, "ticket version dev\n"); rest != tc.text {
				t.Fatalf("usage(%q) remainder is not the %s usage text (got %d bytes, want %d)", tc.lang, tc.lang, len(rest), len(tc.text))
			}
		})
	}
}

// TestRunUnknownCommandExit1 pins the bash contract for an unknown command:
// usage (version line + Russian text, the lang-less default) on stdout and
// exit 1 without touching stderr (bash:154).
func TestRunUnknownCommandExit1(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"frobnicate"}, map[string]string{}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(unknown) = %d, want 1", code)
	}
	// Pinned literal (see TestVersionLine), not versionLine(): the
	// expected output must be independent of the code under test.
	if got, want := stdout.String(), "ticket version dev\n"+usageTextRU; got != want {
		t.Fatalf("stdout is not version line + usage text (got %d bytes, want %d)", len(got), len(want))
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}
