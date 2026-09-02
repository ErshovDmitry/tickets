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
	"strings"
	"testing"
)

// TestVersionLdflagsInjection builds cmd/ticket with
// -ldflags "-X ticket/internal/cli.version=9.9.9" into a fresh temp dir and
// asserts that both `version` and `help` report the injected string: the
// version line as the exact stdout of `version` and as the FIRST line of
// `help` usage output. Both commands run with a minimal environment
// (os.Environ plus TICKETS_DIR pointing at an empty temp dir) — version and
// help must not need a real tickets dir (cli.Run skips dispatch entirely).
func TestVersionLdflagsInjection(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "ticket")

	// Hermetic build: link-time injection of the release version (D1 path).
	build := exec.Command("go", "build",
		"-ldflags", "-X ticket/internal/cli.version=9.9.9",
		"-o", bin, "../../cmd/ticket")
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
