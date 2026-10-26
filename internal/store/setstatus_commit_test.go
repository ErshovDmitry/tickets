package store

// SetStatus/Archive commit edge cases: injected failures around the
// no-replace commit (link, remove-old, rollback) and the concurrent
// Create+SetStatus race that must not lose journal lines.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"ticket/internal/domain"
)

// ----------------------------------------------------------------------------
// SetStatus — failure paths (e, i, j)
// ----------------------------------------------------------------------------

func TestSetStatus_LinkFailsJoinsErr(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	// link always fails; cleanup (os.Remove on tmp) still runs and
	// succeeds, so errors.Join produces a one-cause error.
	swapLink(t, func(string, string) error {
		return errors.New("synthetic-link-fail")
	})
	_, _, err := s.SetStatus(1, domain.StatusWip, "tester", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "synthetic-link-fail") {
		t.Errorf("err should mention synthetic fail; got %v", err)
	}
	assertNoTmpFiles(t, dir)
	if _, err := os.Stat(filepath.Join(dir, "T-0001-open.md")); err != nil {
		t.Fatalf("original file disappeared: %v", err)
	}
}

func TestSetStatus_RemoveOldFailsRollbackOK(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(dir, "T-0001-open.md")
	target := filepath.Join(dir, "T-0001-wip.md")
	// linkFile stays the default os.Link, so the no-replace commit
	// succeeds and the tmp name is removed directly (os.Remove, not via
	// removeFile). removeFile is therefore invoked exactly twice:
	// remove-old (must fail) and the rollback removal of the target
	// (must pass through to os.Remove).
	calls := 0
	swapRemove(t, func(p string) error {
		calls++
		if p == oldPath {
			return errors.New("synthetic-remove-old-fail")
		}
		return os.Remove(p)
	})
	_, _, err := s.SetStatus(1, domain.StatusWip, "tester", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "synthetic-remove-old-fail") {
		t.Errorf("err should mention synthetic remove-old fail; got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected exactly two removeFile calls, got %d", calls)
	}
	// Old file must remain.
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("old file should still be present after rollback: %v", err)
	}
	// Target (created by link commit) must be removed by rollback.
	if _, err := os.Stat(target); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("target should have been removed by rollback; Lstat err=%v", err)
	}
}

func TestSetStatus_RollbackAlsoFails_BreachText(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(dir, "T-0001-open.md")
	target := filepath.Join(dir, "T-0001-wip.md")
	swapRemove(t, func(p string) error {
		return fmt.Errorf("synthetic-remove(%s)", filepath.Base(p))
	})
	_, _, err := s.SetStatus(1, domain.StatusWip, "tester", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invariant breach: old+new coexist") {
		t.Errorf("err missing breach text; got %v", err)
	}
	// Both files must still exist on disk.
	for _, p := range []string{oldPath, target} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to exist after breach: %v", p, err)
		}
	}
}

// ----------------------------------------------------------------------------
// Concurrent Create + SetStatus racing each other (h)
// ----------------------------------------------------------------------------

// concurrentLockBudget is the per-acquisition lock wait budget for the
// concurrent Create+SetStatus tests. Each ticket costs one Create plus one
// SetStatus behind a single store lock, and every write is fsynced (Create
// commits in place, SetStatus via link or rename), so the load variant
// serialises 2N such operations behind that lock. The shipped default
// (defaultLockTimeout in internal/store/store.go) is one interactive
// operation's budget, not a load test's, and the load variant has been
// observed to exhaust it on CI; both tests therefore pin the budget
// explicitly instead of depending on the ambient TICKET_LOCK_TIMEOUT.
const concurrentLockBudget = 60 * time.Second

// concurrentAppearDeadline bounds how long a worker waits for its ticket to
// appear before reporting it as vanished. It must exceed
// concurrentLockBudget because creating that ticket needs the same lock.
const concurrentAppearDeadline = 2 * concurrentLockBudget

// setConcurrentLockBudget pins TICKET_LOCK_TIMEOUT for the concurrent tests.
// withLock re-reads the variable on every call (store.parseLockTimeout), so
// goroutines started after this call observe the value. The tests must not
// use t.Parallel: testing forbids Setenv there.
func setConcurrentLockBudget(t *testing.T) {
	t.Helper()
	t.Setenv("TICKET_LOCK_TIMEOUT", concurrentLockBudget.String())
}

// TestConcurrent_CreateAndSetStatus_NoJournalLoss runs Create and SetStatus
// genuinely in parallel: one producer goroutine creates n tickets while n
// waiter goroutines each poll for their number and transition it as soon as
// it appears. Every ticket must end as a wip file whose journal keeps the
// creation line and exactly one byte-exact open → wip transition line.
func TestConcurrent_CreateAndSetStatus_NoJournalLoss(t *testing.T) {
	s, dir := newStore(t)
	setConcurrentLockBudget(t)
	const n = 5
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			if _, err := s.Create(fakeTicket(0)); err != nil {
				t.Errorf("Create #%d: %v", i+1, err)
			}
		}
	}()
	for i := 1; i <= n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			deadline := time.Now().Add(concurrentAppearDeadline)
			for {
				_, err := s.Find(i)
				if err == nil {
					break
				}
				if !errors.Is(err, ErrNotFound) {
					t.Errorf("Find(%d): %v", i, err)
					return
				}
				if time.Now().After(deadline) {
					t.Errorf("ticket %d never appeared", i)
					return
				}
				time.Sleep(time.Millisecond)
			}
			if _, _, err := s.SetStatus(i, domain.StatusWip, "tester", fmt.Sprintf("go %d", i)); err != nil {
				t.Errorf("SetStatus(%d): %v", i, err)
			}
		}()
	}
	wg.Wait()
	for i := 1; i <= n; i++ {
		body := string(mustRead(t, dir, i))
		wipName := filepath.Join(dir, fmt.Sprintf("T-%04d-wip.md", i))
		if _, err := os.Stat(wipName); err != nil {
			t.Errorf("ticket %d: expected wip file: %v", i, err)
			continue
		}
		if !strings.Contains(body, "— тикет создан") {
			t.Errorf("ticket %d: creation journal line lost", i)
		}
		if got := strings.Count(body, "— статус: open → wip"); got != 1 {
			t.Errorf("ticket %d: want exactly 1 transition line, got %d", i, got)
		}
		assertJournalTransition(t, body, "open", "wip", fmt.Sprintf("go %d", i), "tester")
	}
}
