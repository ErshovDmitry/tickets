package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// createDeleteRepetitions is how many times each concurrent-delete trial
// runs with a fresh store. The Create×Delete trial holds its contention
// deterministically, but the bounded block-proof window still depends on
// the racing Delete being scheduled inside it, and one lucky scheduling
// of the two-delete trial may never overlap the deletes; the repeats
// make a silent-scheduling escape vanishingly unlikely.
const createDeleteRepetitions = 50

// createDeleteBlockWindow is the bounded observation window proving that
// Delete blocks while Create holds the lock in afterCreateHook. Proving a
// negative ("Delete did not enter resolve/mutate") needs a finite window;
// it starts only after the Delete goroutine signalled readiness, so it
// covers the Delete call path alone, not goroutine startup. 100ms is
// orders of magnitude above scheduling jitter for an already-running
// goroutine, and the entry-order check catches a broken lock whenever it
// enters before release, so the window only needs to give it the chance.
const createDeleteBlockWindow = 100 * time.Millisecond

// TestConcurrent_CreateAndDelete_NoReuseNoLoss pins numbering under
// concurrent Create/Delete: the first Create is held inside the store
// lock while a Delete of the same ticket must block (deterministic
// in-flight contention, fix A8 cycle 2), then every ticket is created
// and deleted. Every assigned number is unique and contiguous (deleted
// numbers are never reissued), and each ticket ends up exactly once in
// deleted/ with its creation line and one byte-exact open→deleted
// transition intact. The trial repeats with a fresh store each time.
func TestConcurrent_CreateAndDelete_NoReuseNoLoss(t *testing.T) {
	setConcurrentLockBudget(t)
	for rep := 1; rep <= createDeleteRepetitions; rep++ {
		t.Run(fmt.Sprintf("rep-%02d", rep), func(t *testing.T) {
			runCreateDeleteTrial(t)
		})
	}
}

// runCreateDeleteTrial is one fresh-store repetition: deterministic
// Create×Delete contention on the first ticket, then the remaining
// tickets created and deleted one by one, then the final-state pins.
func runCreateDeleteTrial(t *testing.T) {
	t.Helper()
	s, dir := newStore(t)
	const n = 20
	h := newCreateDeleteHold(t, s)
	assigned := []int{h.proveDeleteBlocks(t, s)}
	for i := 2; i <= n; i++ {
		num, err := s.Create(fakeTicket(0))
		if err != nil {
			t.Fatalf("Create #%d: %v", i, err)
		}
		assigned = append(assigned, num)
		if _, err := s.Delete(fmt.Sprint(num), false, "tester"); err != nil {
			t.Fatalf("Delete(%d): %v", num, err)
		}
	}
	assertCreateDeleteFinalState(t, dir, assigned)
}

// createOutcome carries the first Create's result out of its goroutine.
type createOutcome struct {
	num int
	err error
}

// createDeleteHold wires the seams of the deterministic Create×Delete
// contention: the first Create pauses in afterCreateHook (inside the
// store lock) until release, and deleteEntryHook records any Delete
// entry into deleteOneLocked observed before release — with the lock
// intact no such entry can exist.
type createDeleteHold struct {
	inFlight         chan struct{}
	release          chan struct{}
	releaseCreate    func()
	deleteStarted    chan struct{}
	deleteDone       chan error
	createDone       chan createOutcome
	enteredWhileHeld chan string
}

// newCreateDeleteHold installs the hold seams and starts the first
// Create. Only the first Create pauses (holdOnce); later Creates pass
// through, so a numbering mutation that reissues 1 cannot deadlock the
// hold. Cleanup always releases the held Create, even on failure paths.
func newCreateDeleteHold(t *testing.T, s *Store) *createDeleteHold {
	t.Helper()
	h := &createDeleteHold{
		inFlight:         make(chan struct{}),
		release:          make(chan struct{}),
		deleteStarted:    make(chan struct{}),
		deleteDone:       make(chan error, 1),
		createDone:       make(chan createOutcome, 1),
		enteredWhileHeld: make(chan string, 1),
	}
	h.releaseCreate = sync.OnceFunc(func() { close(h.release) })
	t.Cleanup(h.releaseCreate)
	var holdOnce sync.Once
	swapAfterCreate(t, func(n int, path string) {
		holdOnce.Do(func() {
			close(h.inFlight)
			<-h.release
		})
	})
	swapDeleteEntry(t, func(ref string) {
		select {
		case <-h.release:
		default:
			select {
			case h.enteredWhileHeld <- ref:
			default:
			}
		}
	})
	go func() {
		num, err := s.Create(fakeTicket(0))
		h.createDone <- createOutcome{num, err}
	}()
	return h
}

// proveDeleteBlocks launches Delete("1") against the held Create and
// proves it stays outside deleteOneLocked until release: any pre-release
// entry is lock loss and fails explicitly. After release both operations
// must succeed and the first number must be 1.
func (h *createDeleteHold) proveDeleteBlocks(t *testing.T, s *Store) int {
	t.Helper()
	select {
	case <-h.inFlight:
	case res := <-h.createDone:
		t.Fatalf("first Create finished before the hold: num=%d err=%v", res.num, res.err)
	}
	go func() {
		close(h.deleteStarted)
		_, err := s.Delete("1", false, "tester")
		h.deleteDone <- err
	}()
	<-h.deleteStarted
	select {
	case ref := <-h.enteredWhileHeld:
		t.Fatalf("Delete(%s) entered deleteOneLocked while Create held the lock: lock lost", ref)
	case <-time.After(createDeleteBlockWindow):
		// No entry for the whole hold: the lock serialises Create/Delete.
	}
	h.releaseCreate()
	res := <-h.createDone
	if res.err != nil {
		t.Fatalf("Create(1): %v", res.err)
	}
	if err := <-h.deleteDone; err != nil {
		t.Fatalf("Delete(1): %v", err)
	}
	// Catch entries between window expiry and release: Delete completes
	// only after its entry hook ran, so every recorded entry is visible.
	select {
	case ref := <-h.enteredWhileHeld:
		t.Fatalf("Delete(%s) entered deleteOneLocked before release: lock lost", ref)
	default:
	}
	if res.num != 1 {
		t.Fatalf("first Create assigned %d, want 1", res.num)
	}
	return res.num
}

// assertCreateDeleteFinalState pins the trial outcome: numbers 1..n
// assigned exactly once (no reuse, no hole), no main file left, and each
// deleted body keeps exactly one creation line and one byte-exact
// open→deleted journal transition by "tester" (who/comment tamper fails).
func assertCreateDeleteFinalState(t *testing.T, dir string, assigned []int) {
	t.Helper()
	n := len(assigned)
	seen := make(map[int]bool, n)
	for _, num := range assigned {
		if seen[num] {
			t.Fatalf("number %d reused", num)
		}
		seen[num] = true
	}
	for i := 1; i <= n; i++ {
		if !seen[i] {
			t.Fatalf("number %d never assigned (hole)", i)
		}
		if _, err := os.Lstat(filepath.Join(dir, fmt.Sprintf("T-%04d-open.md", i))); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("ticket %d: main file must be gone, Lstat err=%v", i, err)
		}
		body := string(mustRead(t, filepath.Join(dir, "deleted"), i))
		if got := strings.Count(body, "— тикет создан"); got != 1 {
			t.Errorf("ticket %d: want exactly 1 creation journal line, got %d", i, got)
		}
		if got := strings.Count(body, "— статус: open → deleted"); got != 1 {
			t.Errorf("ticket %d: want exactly 1 delete transition, got %d", i, got)
		}
		assertJournalTransition(t, body, "open", "deleted", "", "tester")
	}
}

// swapDeleteEntry sets the deleteEntryHook seam for the duration of t
// (same idiom as swapAfterCreate).
func swapDeleteEntry(t *testing.T, fn func(ref string)) {
	t.Helper()
	orig := deleteEntryHook
	deleteEntryHook = fn
	t.Cleanup(func() { deleteEntryHook = orig })
}

// TestConcurrent_TwoDeletes_ExactlyOneSuccess pins that two racing
// deletes of the same ticket serialize under the lock: exactly one
// succeeds, the other is ErrAlreadyDeleted, and the deleted file
// carries exactly one transition (no double journal, no loss). Like
// the Create/Delete trial above it repeats with a fresh store per
// repetition: one lucky scheduling may never overlap the deletes.
func TestConcurrent_TwoDeletes_ExactlyOneSuccess(t *testing.T) {
	setConcurrentLockBudget(t)
	for rep := 1; rep <= createDeleteRepetitions; rep++ {
		t.Run(fmt.Sprintf("rep-%02d", rep), func(t *testing.T) {
			runTwoDeleteTrial(t)
		})
	}
}

// runTwoDeleteTrial is one fresh-store repetition of the two racing
// deletes of the same ticket.
func runTwoDeleteTrial(t *testing.T) {
	t.Helper()
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := s.Delete("1", false, "tester")
			results <- err
		}()
	}
	close(start)
	var successes, already int
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrAlreadyDeleted):
			already++
		default:
			t.Fatalf("unexpected delete error: %v", err)
		}
	}
	if successes != 1 || already != 1 {
		t.Fatalf("successes=%d already=%d, want 1/1", successes, already)
	}
	if _, err := os.Lstat(filepath.Join(dir, "T-0001-open.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("main file must be gone, Lstat err=%v", err)
	}
	body := string(mustRead(t, filepath.Join(dir, "deleted"), 1))
	if got := strings.Count(body, "— статус: open → deleted"); got != 1 {
		t.Fatalf("want exactly 1 delete transition, got %d", got)
	}
	assertJournalTransition(t, body, "open", "deleted", "", "tester")
}
