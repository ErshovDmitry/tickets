package cli_test

// §7.2 integration tests: the real binary as an OS process. TestMain
// provides the binary — built once via the Go toolchain, or taken from
// TICKET_TEST_BIN on hosts without Go — snapshots the live tickets/ tree,
// runs the suite, then asserts the tree is byte-identical —
// `go test ./...` must never mutate dogfood state.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ticketBin is the production binary provided by TestMain (built via the
// Go toolchain, or taken from TICKET_TEST_BIN); empty when unavailable.
var ticketBin string

// skipReason explains why ticketBin is empty; it is empty whenever
// ticketBin is ready to use. Tests skip via requireBin.
var skipReason string

func TestMain(m *testing.M) {
	os.Exit(runMain(m))
}

// snapshotLive hashes the live tickets/ tree. ok=false means the tree does
// not exist: tickets/ is gitignored, so a clean checkout has none and the
// guard has nothing to protect — it must not demand one.
func snapshotLive(root string) (snap map[string]string, ok bool, err error) {
	_, statErr := os.Stat(root)
	if errors.Is(statErr, fs.ErrNotExist) {
		return nil, false, nil
	}
	if statErr != nil {
		return nil, false, statErr
	}
	snap, err = hashTree(root)
	if err != nil {
		return nil, false, err
	}
	return snap, true, nil
}

// runMain wires the binary build and the live-tree guard around the suite.
func runMain(m *testing.M) int {
	// The guard hashes (read-only) the live dogfood tree; the module root
	// keeps it working at CWD != internal/cli. Without one the relative
	// default usually does not exist — vacuous guard (skip-mode).
	live := filepath.Join("..", "..", "tickets")
	if root, ok := moduleRoot(); ok {
		live = filepath.Join(root, "tickets")
	}
	before, liveExisted, err := snapshotLive(live)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: snapshot live tree: %v\n", err)
		return 1
	}
	bin, reason, cleanup, err := resolveBin()
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: %v\n", err)
		return 1
	}
	defer cleanup()
	ticketBin, skipReason = bin, reason
	code := m.Run()
	after, liveExists, err := snapshotLive(live)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: re-snapshot live tree: %v\n", err)
		return 1
	}
	switch {
	case liveExisted != liveExists:
		fmt.Fprintf(os.Stderr, "integration: live tickets/ tree appeared or vanished during the test run\n")
		return 1
	case liveExisted && !equalTrees(before, after):
		fmt.Fprintf(os.Stderr, "integration: live tickets/ tree changed during the test run\n")
		return 1
	}
	return code
}

// resolveBin picks the integration binary and its cleanup. TICKET_TEST_BIN
// wins when set: an existing file is used as-is (Go-less hosts test a
// prebuilt binary), a missing file is a configuration error — not a skip.
// The override is normalized to an absolute path: tests exec the binary
// with cmd.Dir set to temp dirs, and exec re-resolves relative paths
// against cmd.Dir, which would fail with "executable file not found".
// No PATH dispatch happens: a bare name that is not an existing file in
// the working directory is a configuration error.
// Without it, `go` from PATH builds the binary from the module root (see
// moduleRoot); a missing `go` or module root skips gracefully (reason
// remembered, ticketBin empty), while a failing `go build` is a hard error.
func resolveBin() (string, string, func(), error) {
	if p := os.Getenv("TICKET_TEST_BIN"); p != "" {
		p = absOverride(p)
		if _, statErr := os.Stat(p); statErr != nil {
			return "", "", func() {}, fmt.Errorf("TICKET_TEST_BIN=%q: %v", p, statErr)
		}
		return p, "", func() {}, nil
	}
	if _, lookErr := exec.LookPath("go"); lookErr != nil {
		return "", fmt.Sprintf("no Go toolchain on PATH (%v); set TICKET_TEST_BIN to a prebuilt ticket binary to run integration tests", lookErr), func() {}, nil
	}
	root, ok := moduleRoot()
	if !ok {
		return "", "cannot locate the ticket module root (binary moved off the build machine or built with -trimpath); set TICKET_TEST_BIN to a prebuilt ticket binary to run integration tests", func() {}, nil
	}
	bin, cleanup, err := buildTicketBinary(root)
	return bin, "", cleanup, err
}

// absOverride normalizes p to an absolute path against TestMain's working
// directory so a later cmd.Dir on the exec'd process cannot invalidate it.
// If Abs fails, p is returned unchanged.
func absOverride(p string) string {
	if abs, absErr := filepath.Abs(p); absErr == nil {
		return abs
	}
	return p
}

// requireBin skips a test when TestMain could not provide the binary.
func requireBin(t *testing.T) {
	t.Helper()
	if ticketBin == "" {
		t.Skip(skipReason)
	}
}

// exeSuffix returns the OS executable suffix (".exe" on windows, "" on
// other platforms): on Windows a built binary without it cannot be
// executed directly ("executable file not found").
func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// buildTicketBinary compiles ./cmd/ticket (cmd.Dir=root: the package path
// resolves from the module root regardless of the process CWD) into a
// fresh temp dir and returns the binary path plus a cleanup func.
func buildTicketBinary(root string) (string, func(), error) {
	tmp, err := os.MkdirTemp("", "tickets-it-*")
	if err != nil {
		return "", nil, err
	}
	bin := filepath.Join(tmp, "ticket"+exeSuffix())
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/ticket")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(tmp)
		return "", nil, fmt.Errorf("go build ./cmd/ticket: %v\n%s", err, out)
	}
	return bin, func() { os.RemoveAll(tmp) }, nil
}

// hashTree maps every file under root (repo-relative) to its sha256.
func hashTree(root string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// equalTrees reports whether two hash snapshots are identical.
func equalTrees(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// runBin executes bin with the given working directory and tickets dir.
// An empty ticketsDir means NO TICKETS_DIR in the child environment at all
// (hermetic: no ambient TICKET_WHO/USER leakage either).
func runBin(t *testing.T, bin, dir, ticketsDir string, args ...string) (string, string, int) {
	t.Helper()
	return runBinEnv(t, bin, dir, ticketsDir, nil, args...)
}

// runBinEnv is runBin with extra key=value pairs appended to the child
// environment (e.g. TICKET_LANG=en for language-pinned scenarios).
// extraEnv must NOT contain TICKETS_DIR: a duplicate key would silently
// override the ticketsDir sandbox, since the last duplicate wins in
// exec.Cmd.Env.
func runBinEnv(t *testing.T, bin, dir, ticketsDir string, extraEnv []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	env := []string{}
	if ticketsDir != "" {
		env = []string{"TICKETS_DIR=" + ticketsDir}
	}
	env = append(env, extraEnv...)
	cmd.Env = env
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
	return stdout.String(), stderr.String(), code
}

// mustEval resolves symlinks so expected paths match what the binary
// prints (paths.Resolve returns EvalSymlinks-resolved directories).
func mustEval(t *testing.T, p string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", p, err)
	}
	return resolved
}

// assertEmptyDir fails when dir contains any entry.
func assertEmptyDir(t *testing.T, dir, label string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("%s is not empty: %v", label, names)
	}
}

// TestNewHelpDoesNotCreateTicket runs the real binary (T-0041 regression:
// `ticket new --help` used to create a ticket titled "--help"): in a fresh
// temp TICKETS_DIR it must print usage to stdout, exit 0 and leave the dir
// empty.
func TestNewHelpDoesNotCreateTicket(t *testing.T) {
	requireBin(t)
	dir := t.TempDir()
	stdout, stderr, code := runBin(t, ticketBin, dir, dir, "new", "--help")
	if code != 0 {
		t.Fatalf("new --help exit = %d, want 0; stderr: %q", code, stderr)
	}
	if !strings.HasPrefix(stdout, "ticket version dev\n") {
		t.Fatalf("stdout %q does not start with the usage text", stdout)
	}
	assertEmptyDir(t, dir, "tickets dir")
}
