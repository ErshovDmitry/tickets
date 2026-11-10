package cli_test

// T-0157 concurrent CLI-process regression (plan c5 D5.1, GitHub #19):
// 32 real `ticket -C <sandbox> new` OS processes per round across 10
// rounds against ONE tickets dir; every process must exit 0 (the default
// TICKET_LOCK_TIMEOUT stays in force — a lock timeout is a failure) and
// the numbers in <sandbox> + <sandbox>/archive must have zero duplicates.
// No sleeps: processes are reaped with cmd.Wait only.

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// parallelNewProcs is the D5.1 concurrency width per round.
const parallelNewProcs = 32

// parallelNewRounds is the D5.1 round count (320 creates total).
const parallelNewRounds = 10

// TestParallelNewSetNoDuplicates launches parallelNewProcs parallel
// `ticket -C <tickets> new` OS processes for parallelNewRounds rounds and
// asserts: every process exits 0 and the number set across the main dir
// and archive/ has zero duplicates. The binary is built once by TestMain
// (integration_test.go:32); TICKET_LOCK_TIMEOUT is never overridden.
func TestParallelNewSetNoDuplicates(t *testing.T) {
	requireBin(t)
	tickets := t.TempDir()
	cwd := t.TempDir()
	seen := map[int]int{} // number → round that created it

	for round := 0; round < parallelNewRounds; round++ {
		cmds := make([]*exec.Cmd, parallelNewProcs)
		bufs := make([]bytes.Buffer, parallelNewProcs)
		for i := range cmds {
			cmds[i] = exec.Command(ticketBin, "-C", tickets, "new", fmt.Sprintf("гонка %d/%d", round, i))
			cmds[i].Dir = cwd
			// Hermetic env: no TICKETS_DIR (the -C flag pins the store),
			// no TICKET_LOCK_TIMEOUT override — a lock timeout must fail.
			cmds[i].Env = childEnv()
			cmds[i].Stdout = &bufs[i]
			if err := cmds[i].Start(); err != nil {
				t.Fatalf("round %d: start process %d: %v", round, i, err)
			}
		}
		// Buffered results channel: one receive per process is the
		// synchronization — no sleeps, no shared-slice writes.
		type procResult struct {
			i   int
			err error
		}
		results := make(chan procResult, parallelNewProcs)
		for i, c := range cmds {
			go func(i int, c *exec.Cmd) {
				results <- procResult{i, c.Wait()}
			}(i, c)
		}
		for range cmds {
			r := <-results
			if r.err != nil {
				t.Fatalf("round %d process %d: %v (stdout=%q)", round, r.i, r.err, bufs[r.i].String())
			}
			n := parseCreatedNumber(t, round, r.i, bufs[r.i].String())
			if prev, dup := seen[n]; dup {
				t.Fatalf("number %d created twice: round %d and round %d", n, prev, round)
			}
			seen[n] = round
		}
	}

	// Ground truth: scan the sandbox files, not the collected stdout.
	total := parallelNewProcs * parallelNewRounds
	if len(seen) != total {
		t.Fatalf("created %d unique numbers, want %d", len(seen), total)
	}
	assertNoDuplicateFiles(t, tickets, total)
}

// childEnv returns the hermetic child environment: only the temp-dir pins
// runBinEnv uses (integration_test.go:221), nothing else.
func childEnv() []string {
	env := []string{}
	if tmp := os.TempDir(); tmp != "" {
		if abs, err := filepath.Abs(tmp); err == nil {
			tmp = filepath.Clean(abs)
			env = append(env, "TMPDIR="+tmp, "TMP="+tmp, "TEMP="+tmp)
		}
	}
	return env
}

// parseCreatedNumber extracts the ticket number from `new` stdout
// ("<abs path>/T-NNNN-open.md\n").
func parseCreatedNumber(t *testing.T, round, i int, stdout string) int {
	t.Helper()
	base := filepath.Base(strings.TrimSpace(stdout))
	n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(base, "T-"), "-open.md"))
	if err != nil {
		t.Fatalf("round %d process %d stdout %q: %v", round, i, stdout, err)
	}
	return n
}

// assertNoDuplicateFiles scans dir and dir/archive for ticket files and
// fails on any duplicate number, any leftover tmp artifact or a count
// other than want.
func assertNoDuplicateFiles(t *testing.T, dir string, want int) {
	t.Helper()
	nums := map[int]string{}
	count := 0
	for _, d := range []string{dir, filepath.Join(dir, "archive")} {
		entries, err := os.ReadDir(d)
		if err != nil {
			if d != dir && errors.Is(err, fs.ErrNotExist) {
				continue // archive/ may not exist yet
			}
			t.Fatalf("ReadDir(%s): %v", d, err)
		}
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue // .lock
			}
			if strings.HasSuffix(name, ".tmp") {
				t.Fatalf("leftover tmp artifact: %s", name)
			}
			stem := strings.TrimSuffix(strings.TrimPrefix(name, "T-"), ".md")
			n, err := strconv.Atoi(strings.SplitN(stem, "-", 2)[0])
			if err != nil {
				t.Fatalf("unexpected entry %s: %v", name, err)
			}
			if prev, dup := nums[n]; dup {
				t.Fatalf("number %d in both %s and %s", n, prev, name)
			}
			nums[n] = filepath.Join(d, name)
			count++
		}
	}
	if count != want {
		t.Fatalf("sandbox holds %d ticket files, want %d", count, want)
	}
}
