package store

// Load variant of TestConcurrent_CreateAndSetStatus_NoJournalLoss with
// N = 50, adopted under T-0084 (originated in an uncommitted session
// 2026-09-08, T-0063). Same invariants as the N=5 original at higher
// contention; the lock budget and the vanish deadline come from the shared
// consts in setstatus_commit_test.go, so this test does not depend on the
// ambient TICKET_LOCK_TIMEOUT either.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"ticket/internal/domain"
)

// TestConcurrent_CreateAndSetStatus_NoJournalLoss_N50 pins the same
// invariants as the N=5 original (creation journal line kept, exactly
// one open → wip transition line per ticket) at higher contention.
func TestConcurrent_CreateAndSetStatus_NoJournalLoss_N50(t *testing.T) {
	s, dir := newStore(t)
	setConcurrentLockBudget(t)
	const n = 50
	start := time.Now()
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
				time.Sleep(concurrentFindPollBackoff)
			}
			if _, _, err := s.SetStatus(i, domain.StatusWip, "tester", fmt.Sprintf("go %d", i)); err != nil {
				t.Errorf("SetStatus(%d): %v", i, err)
			}
		}()
	}
	wg.Wait()
	t.Logf("N50: %d tickets in %s", n, time.Since(start))
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
