package cli_test

// §7.2 integration tests: the real binary as an OS process. TestMain
// builds the binary once, snapshots the live tickets/ tree, runs the
// suite, then asserts the tree is byte-identical — `go test ./...` must
// never mutate dogfood state.

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
	"testing"
)

// ticketBin is the production binary built once by TestMain.
var ticketBin string

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
	live := filepath.Join("..", "..", "tickets")
	before, liveExisted, err := snapshotLive(live)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: snapshot live tree: %v\n", err)
		return 1
	}
	bin, cleanup, err := buildTicketBinary()
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: %v\n", err)
		return 1
	}
	defer cleanup()
	ticketBin = bin
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

// exeSuffix returns the OS executable suffix (".exe" on windows, "" on
// other platforms): on Windows a built binary without it cannot be
// executed directly ("executable file not found").
func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// buildTicketBinary compiles cmd/ticket into a fresh temp dir and returns
// the binary path plus a cleanup func.
func buildTicketBinary() (string, func(), error) {
	tmp, err := os.MkdirTemp("", "tickets-it-*")
	if err != nil {
		return "", nil, err
	}
	bin := filepath.Join(tmp, "ticket"+exeSuffix())
	cmd := exec.Command("go", "build", "-o", bin, "../../cmd/ticket")
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(tmp)
		return "", nil, fmt.Errorf("go build ../../cmd/ticket: %v\n%s", err, out)
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
	cmd := exec.Command(bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	env := []string{}
	if ticketsDir != "" {
		env = []string{"TICKETS_DIR=" + ticketsDir}
	}
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
