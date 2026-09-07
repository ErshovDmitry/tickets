package cli_test

// §7.2 binary-level integration scenarios: foreign-CWD full cycle and
// N=8 parallel OS-process numbering; `ticket init` scenarios (T-0057:
// init creates only tickets/ + tickets/archive/, no bin/, no symlink).

import (
	"bytes"
	"errors"
	"fmt"
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
	requireBin(t)
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

// parallelNew is the §7.2 concurrency width: N parallel OS processes.
const parallelNew = 8

// TestParallelNewUniqueContiguous launches N=8 parallel `ticket new` OS
// processes against one fresh empty tickets dir and asserts unique AND
// contiguous numbers 1..N (no gaps, no duplicates).
func TestParallelNewUniqueContiguous(t *testing.T) {
	requireBin(t)
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

// TestInitCreatesStructure verifies `ticket init` creates tickets/ and
// tickets/archive/ (no bin/, no symlink) and that a subsequent new/list
// cycle lands in the created tree via the upward scan. Cross-platform:
// no symlinks are involved anymore (T-0057 removed the deploy model).
func TestInitCreatesStructure(t *testing.T) {
	requireBin(t)
	root := t.TempDir()
	cwd := root

	out, stderr, code := runBin(t, ticketBin, cwd, "", "init")
	if code != 0 {
		t.Fatalf("init: code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(out, "Инициализировано:") {
		t.Errorf("init stdout=%q missing initialized message", out)
	}
	tickets := filepath.Join(root, "tickets")
	for _, dir := range []string{tickets, filepath.Join(tickets, "archive")} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("init did not create %s as a directory (err=%v)", dir, err)
		}
	}
	if _, err := os.Stat(filepath.Join(tickets, "bin")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("tickets/bin must not exist after init (err=%v)", err)
	}

	ticketsEval := mustEval(t, tickets)
	out, stderr, code = runBin(t, ticketBin, cwd, "", "new", "Via init")
	if code != 0 {
		t.Fatalf("new after init: code=%d stderr=%q", code, stderr)
	}
	if want := filepath.Join(ticketsEval, "T-0001-open.md") + "\n"; out != want {
		t.Fatalf("new stdout=%q want %q", out, want)
	}

	out, _, code = runBin(t, ticketBin, cwd, "", "list")
	if code != 0 {
		t.Fatalf("list after init: code=%d", code)
	}
	if !strings.Contains(out, "T-0001") {
		t.Errorf("list output=%q missing T-0001", out)
	}
}

// TestInitIdempotent verifies repeat init is no-op exit 0 (§B3: the same
// "Инициализировано:" line both times — no separate already-text).
func TestInitIdempotent(t *testing.T) {
	requireBin(t)
	root := t.TempDir()

	if _, stderr, code := runBin(t, ticketBin, root, "", "init"); code != 0 {
		t.Fatalf("first init: code=%d stderr=%q", code, stderr)
	}

	out, stderr, code := runBin(t, ticketBin, root, "", "init")
	if code != 0 {
		t.Fatalf("second init: code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(out, "Инициализировано:") {
		t.Errorf("second init stdout=%q missing initialized message", out)
	}
}

// TestInitConflictRealFile verifies init fails gracefully when tickets
// itself is a regular file: exit 1, empty stdout, bytes untouched.
func TestInitConflictRealFile(t *testing.T) {
	requireBin(t)
	root := t.TempDir()
	dst := filepath.Join(root, "tickets")
	origContent := []byte("existing-file-content")
	os.WriteFile(dst, origContent, 0644)

	out, stderr, code := runBin(t, ticketBin, root, "", "init")
	if code != 1 {
		t.Fatalf("init conflict regular file: code=%d want 1", code)
	}
	if !strings.Contains(stderr, "Конфликт:") {
		t.Errorf("stderr=%q missing Конфликт", stderr)
	}
	if out != "" {
		t.Errorf("stdout=%q want empty on conflict", out)
	}

	data, _ := os.ReadFile(dst)
	if !bytes.Equal(data, origContent) {
		t.Errorf("conflict overwrote file: got %q want %q", data, origContent)
	}
}
