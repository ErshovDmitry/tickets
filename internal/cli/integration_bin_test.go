package cli_test

// §7.2 binary-level integration scenarios: foreign-CWD full cycle,
// symlink invocation, and N=8 parallel OS-process numbering.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// TestForeignCWDNewListShowSet runs the full new→list→show→set cycle from
// a foreign working directory: artifacts land under TICKETS_DIR, never in
// the CWD (§7.2). Fresh empty TempDir per mutating test.
func TestForeignCWDNewListShowSet(t *testing.T) {
	tickets := t.TempDir()
	cwd := t.TempDir()
	ticketsEval := mustEval(t, tickets)

	out, stderr, code := runBin(t, ticketBin, cwd, tickets, "new", "Интеграционный тикет", "-d", "детали")
	if code != 0 {
		t.Fatalf("new: code=%d stderr=%q", code, stderr)
	}
	wantPath := filepath.Join(ticketsEval, "T-0001-open.md") + "\n"
	if out != wantPath {
		t.Fatalf("new stdout = %q, want %q", out, wantPath)
	}
	assertEmptyDir(t, cwd, "foreign cwd")

	out, _, code = runBin(t, ticketBin, cwd, tickets, "list")
	if code != 0 {
		t.Fatalf("list: code=%d", code)
	}
	// Row shape: "T-%04d  %-7s  <TYPE>: <title>"; assert the pieces
	// instead of hand-counting %-7s padding spaces.
	for _, want := range []string{"T-0001", "open", "BUG: Интеграционный тикет"} {
		if !strings.Contains(out, want) {
			t.Fatalf("list output %q missing %q", out, want)
		}
	}

	data, err := os.ReadFile(strings.TrimSuffix(wantPath, "\n"))
	if err != nil {
		t.Fatalf("read created ticket: %v", err)
	}
	out, _, code = runBin(t, ticketBin, cwd, tickets, "show", "1")
	if code != 0 {
		t.Fatalf("show: code=%d", code)
	}
	if out != string(data) {
		t.Fatalf("show output differs from file bytes")
	}

	out, stderr, code = runBin(t, ticketBin, cwd, tickets, "set", "1", "wip", "в работу")
	if code != 0 {
		t.Fatalf("set: code=%d stderr=%q", code, stderr)
	}
	if want := filepath.Join(ticketsEval, "T-0001-wip.md") + "\n"; out != want {
		t.Fatalf("set stdout = %q, want %q", out, want)
	}
	if _, err := os.Stat(filepath.Join(ticketsEval, "T-0001-open.md")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("old T-0001-open.md still present: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ticketsEval, "T-0001-wip.md")); err != nil {
		t.Fatalf("T-0001-wip.md missing: %v", err)
	}
}

// installCopy copies src to dst with mode 0o755.
func installCopy(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("open %s: %v", src, err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatalf("create %s: %v", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close %s: %v", dst, err)
	}
}

// TestSymlinkInvocation installs the binary as tickets/bin/ticket and runs
// it through a symlink: exe resolution must follow the real executable to
// the bin/.. layout (§7.2).
func TestSymlinkInvocation(t *testing.T) {
	root := t.TempDir()
	tickets := filepath.Join(root, "tickets")
	binDir := filepath.Join(tickets, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Both the installed name and the symlink name carry the OS exe
	// suffix: on Windows the launcher only executes "*.exe" files.
	installed := filepath.Join(binDir, "ticket"+exeSuffix())
	installCopy(t, ticketBin, installed)
	link := filepath.Join(root, "ticket-link"+exeSuffix())
	if err := os.Symlink(installed, link); err != nil {
		t.Skipf("symlink unavailable on this host: %v", err)
	}
	cwd := t.TempDir()
	ticketsEval := mustEval(t, tickets)

	out, stderr, code := runBin(t, link, cwd, "", "new", "Симлинк тикет")
	if code != 0 {
		t.Fatalf("new via symlink: code=%d stderr=%q", code, stderr)
	}
	wantPath := filepath.Join(ticketsEval, "T-0001-open.md") + "\n"
	if out != wantPath {
		t.Fatalf("new stdout = %q, want %q", out, wantPath)
	}
	if _, err := os.Stat(strings.TrimSuffix(wantPath, "\n")); err != nil {
		t.Fatalf("ticket not created in exe-relative dir: %v", err)
	}
	assertEmptyDir(t, cwd, "foreign cwd")
}

// parallelNew is the §7.2 concurrency width: N parallel OS processes.
const parallelNew = 8

// TestParallelNewUniqueContiguous launches N=8 parallel `ticket new` OS
// processes against one fresh empty tickets dir and asserts unique AND
// contiguous numbers 1..N (no gaps, no duplicates).
func TestParallelNewUniqueContiguous(t *testing.T) {
	tickets := t.TempDir()
	cwd := t.TempDir()

	cmds := make([]*exec.Cmd, parallelNew)
	bufs := make([]bytes.Buffer, parallelNew)
	for i := range cmds {
		cmds[i] = exec.Command(ticketBin, "new", fmt.Sprintf("Параллельный тикет %d", i))
		cmds[i].Dir = cwd
		cmds[i].Env = []string{"TICKETS_DIR=" + tickets}
		cmds[i].Stdout = &bufs[i]
		if err := cmds[i].Start(); err != nil {
			t.Fatalf("start process %d: %v", i, err)
		}
	}
	var wg sync.WaitGroup
	errs := make([]error, parallelNew)
	for i, c := range cmds {
		wg.Add(1)
		go func(i int, c *exec.Cmd) {
			defer wg.Done()
			errs[i] = c.Wait()
		}(i, c)
	}
	wg.Wait()

	nums := make([]int, 0, parallelNew)
	for i := range cmds {
		if err := errs[i]; err != nil {
			t.Fatalf("process %d failed: %v (stdout=%q)", i, err, bufs[i].String())
		}
		base := filepath.Base(strings.TrimSpace(bufs[i].String()))
		n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(base, "T-"), "-open.md"))
		if err != nil {
			t.Fatalf("process %d stdout %q: %v", i, bufs[i].String(), err)
		}
		nums = append(nums, n)
	}
	sort.Ints(nums)
	for i, n := range nums {
		if n != i+1 {
			t.Fatalf("numbers not contiguous 1..%d: got %v", parallelNew, nums)
		}
	}
	assertEmptyDir(t, cwd, "foreign cwd")

	entries, err := os.ReadDir(tickets)
	if err != nil {
		t.Fatalf("ReadDir(tickets): %v", err)
	}
	files := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue // .lock
		}
		files++
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("leftover tmp artifact: %s", e.Name())
		}
	}
	if files != parallelNew {
		t.Fatalf("tickets dir holds %d files, want %d", files, parallelNew)
	}
}
