package cli_test

// T-0028 D5 integration: the -ldflags "-X ticket/internal/cli.version=X.Y.Z"
// injection path verified end-to-end on a purpose-built binary. The TestMain
// binary (ticketBin) is built without ldflags and stays "dev"; this test
// compiles its own binary with the release version baked in at link time.

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// requireGoToolchain skips when no Go toolchain is on PATH: this test
// compiles its own ldflags-injected binary, so TICKET_TEST_BIN (which only
// substitutes for the TestMain ticketBin) cannot substitute for it.
func requireGoToolchain(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("Go toolchain not found on PATH: %v", err)
	}
}

// moduleRoot derives the module root from this file's compile-time path:
// runtime.Caller(0) records the source path at build time. -trimpath
// builds yield a module-relative path (e.g.
// ticket/internal/cli/integration_test.go), and a binary moved off the
// build machine points at a nonexistent path — both fail the Stat check.
func moduleRoot() (string, bool) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", false
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return "", false
	}
	return root, true
}

// requireModuleRoot skips when the module root cannot be located: this
// test compiles its own ldflags-injected binary, so TICKET_TEST_BIN
// cannot substitute.
func requireModuleRoot(t *testing.T) string {
	t.Helper()
	root, ok := moduleRoot()
	if !ok {
		t.Skipf("cannot locate the ticket module root (binary moved off the build machine or built with -trimpath); this test compiles its own ldflags-injected binary, so TICKET_TEST_BIN cannot substitute")
	}
	return root
}

// TestVersionLdflagsInjection builds cmd/ticket with
// -ldflags "-X ticket/internal/cli.version=9.9.9" into a fresh temp dir and
// asserts that both `version` and `help` report the injected string: the
// version line as the exact stdout of `version` and as the FIRST line of
// `help` usage output. Both commands run with a minimal environment
// (os.Environ plus TICKETS_DIR pointing at an empty temp dir) — version and
// help must not need a real tickets dir (cli.Run skips dispatch entirely).
func TestVersionLdflagsInjection(t *testing.T) {
	requireGoToolchain(t)
	root := requireModuleRoot(t)
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "ticket"+exeSuffix())

	// Hermetic build: link-time injection of the release version (D1 path).
	build := exec.Command("go", "build",
		"-ldflags", "-X ticket/internal/cli.version=9.9.9",
		"-o", bin, "./cmd/ticket")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build with ldflags: %v\n%s", err, out)
	}

	tickets := t.TempDir() // exists, empty: version/help must never touch it
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Env = append(os.Environ(), "TICKETS_DIR="+tickets)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		code := 0
		if err != nil {
			var ee *exec.ExitError
			if !errors.As(err, &ee) {
				t.Fatalf("run %v: %v", args, err)
			}
			code = ee.ExitCode()
		}
		if code != 0 {
			t.Fatalf("%v: code=%d stderr=%q", args, code, stderr.String())
		}
		return stdout.String()
	}

	// `version`: the whole stdout is exactly the version line.
	if out := run("version"); out != "ticket version 9.9.9\n" {
		t.Fatalf("version stdout = %q, want %q", out, "ticket version 9.9.9\n")
	}

	// `help`: the version line is prepended to the usage text — it must be
	// the first line of stdout. strings.Split always yields >=1 element.
	out := run("help")
	if first := strings.Split(out, "\n")[0]; first != "ticket version 9.9.9" {
		t.Fatalf("help first line = %q (full output %d bytes), want %q",
			first, len(out), "ticket version 9.9.9")
	}
}
