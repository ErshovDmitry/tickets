package store

// Store.Edit concurrency coverage (T-0187, C3/3): N=50 tickets each
// mutated by two goroutines through the same per-store advisory lock.
// Disjoint-field edits must both land with both journal lines (no lost
// update), and a concurrent edit+set-status must leave the renamed,
// transitioned file with both journal entries. The lock budget is pinned
// by setConcurrentLockBudget like the existing N50 test, so the run does
// not depend on the ambient TICKET_LOCK_TIMEOUT.
//
// Journal assertions are byte-exact: every status-journal line is checked
// against the inherited "- <ts> — статус: <from> → <to>[ · <comment>] (<who>)"
// shape by assertJournalTransition, and the creation line against
// "- <ts> — тикет создан (<who>)." — no substring or count checks.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"ticket/internal/domain"
)

// concJournalLines returns the status-journal lines of body in file order
// (creation and archive lines do not match).
func concJournalLines(body string) []string {
	var out []string
	for line := range strings.SplitSeq(body, "\n") {
		if strings.Contains(line, " — статус: ") {
			out = append(out, line)
		}
	}
	return out
}

// concJournalFind returns the index of the status-journal line written by
// who with the given comment, or -1 when there is no such line.
func concJournalFind(lines []string, comment, who string) int {
	needle := " · " + comment + " (" + who + ")"
	for i, line := range lines {
		if strings.Contains(line, needle) {
			return i
		}
	}
	return -1
}

// assertConcJournalEntry asserts that the status-journal line at index i
// byte-equals the transition from → to written by who with comment.
func assertConcJournalEntry(t *testing.T, lines []string, i int, from, to, comment, who string) {
	t.Helper()
	if i < 0 || i >= len(lines) {
		t.Fatalf("no journal entry %s → %s · %s (%s) among %q", from, to, comment, who, lines)
	}
	assertJournalTransition(t, lines[i], from, to, comment, who)
}

// assertConcCreationEntry asserts that body carries the creation journal
// line "- <ts> — тикет создан (<who>)." byte-exactly.
func assertConcCreationEntry(t *testing.T, body, who string) {
	t.Helper()
	const marker = " — тикет создан ("
	for line := range strings.SplitSeq(body, "\n") {
		if !strings.Contains(line, marker) {
			continue
		}
		if !strings.HasPrefix(line, "- ") {
			t.Fatalf("creation line must start with \"- \": %q", line)
		}
		at := strings.Index(line, marker)
		if at != 2+len(tsLayout) {
			t.Fatalf("creation timestamp must be %d chars: %q", len(tsLayout), line)
		}
		ts := line[2:at]
		if _, err := time.Parse(tsLayout, ts); err != nil {
			t.Fatalf("creation timestamp %q does not parse as %q: %v", ts, tsLayout, err)
		}
		want := "- " + ts + " — тикет создан (" + who + ")."
		if line != want {
			t.Fatalf("creation line byte mismatch\n got: %q\nwant: %q", line, want)
		}
		return
	}
	t.Fatalf("no creation journal line in body:\n%s", body)
}

// TestConcurrent_Edit_DisjointFields_N50: for every ticket one goroutine
// edits the title and another edits the priority. The edits are disjoint,
// so both must succeed; the serialized commit must leave the new title,
// the new priority and both journal lines in the still-open file.
func TestConcurrent_Edit_DisjointFields_N50(t *testing.T) {
	s, dir := newStore(t)
	setConcurrentLockBudget(t)
	const n = 50
	for i := 1; i <= n; i++ {
		if _, err := s.Create(fakeTicket(0)); err != nil {
			t.Fatalf("Create #%d: %v", i, err)
		}
	}
	var wg sync.WaitGroup
	for i := 1; i <= n; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := s.Edit(i, EditOptions{Title: new(fmt.Sprintf("renamed-%d", i))}, "alice"); err != nil {
				t.Errorf("title Edit(%d): %v", i, err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := s.Edit(i, EditOptions{Priority: new(domain.PriorityHigh)}, "bob"); err != nil {
				t.Errorf("priority Edit(%d): %v", i, err)
			}
		}()
	}
	wg.Wait()

	for i := 1; i <= n; i++ {
		name := filepath.Join(dir, fmt.Sprintf("T-%04d-open.md", i))
		if _, err := os.Stat(name); err != nil {
			t.Errorf("ticket %d: open file must survive edits: %v", i, err)
			continue
		}
		raw := mustRead(t, dir, i)
		body := string(raw)
		for _, sub := range []string{
			fmt.Sprintf("# T-%04d · ENH: renamed-%d", i, i),
			"- Priority (Приоритет): high",
		} {
			if !strings.Contains(body, sub) {
				t.Errorf("ticket %d: body missing %q", i, sub)
			}
		}
		assertConcCreationEntry(t, body, "tester")
		lines := concJournalLines(body)
		if len(lines) != 2 {
			t.Fatalf("ticket %d: want 2 status journal lines, got %d: %q", i, len(lines), lines)
		}
		titleComment := fmt.Sprintf("title: ticket-0 → renamed-%d", i)
		assertConcJournalEntry(t, lines, concJournalFind(lines, titleComment, "alice"),
			"open", "open", titleComment, "alice")
		assertConcJournalEntry(t, lines, concJournalFind(lines, "priority: normal → high", "bob"),
			"open", "open", "priority: normal → high", "bob")
		tk, _, err := domain.Parse(raw)
		if err != nil {
			t.Fatalf("ticket %d: Parse: %v", i, err)
		}
		if tk.Title != fmt.Sprintf("renamed-%d", i) || tk.Priority != domain.PriorityHigh {
			t.Errorf("ticket %d: parsed title=%q priority=%q", i, tk.Title, tk.Priority)
		}
		if tk.Status != domain.StatusOpen {
			t.Errorf("ticket %d: status = %q, want open", i, tk.Status)
		}
	}
	assertNoTmpFiles(t, dir)
}

// TestConcurrent_EditAndSetStatus_NoLostUpdate_N50: for every ticket one
// goroutine edits the title while another transitions open → wip with a
// comment. The final wip file must carry the renamed title and both the
// edit line and the transition line — neither operation may silently
// overwrite the other.
func TestConcurrent_EditAndSetStatus_NoLostUpdate_N50(t *testing.T) {
	s, dir := newStore(t)
	setConcurrentLockBudget(t)
	const n = 50
	for i := 1; i <= n; i++ {
		if _, err := s.Create(fakeTicket(0)); err != nil {
			t.Fatalf("Create #%d: %v", i, err)
		}
	}
	var wg sync.WaitGroup
	for i := 1; i <= n; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := s.Edit(i, EditOptions{Title: new(fmt.Sprintf("renamed-%d", i))}, "alice"); err != nil {
				t.Errorf("Edit(%d): %v", i, err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, _, err := s.SetStatus(i, domain.StatusWip, "tester", fmt.Sprintf("go %d", i)); err != nil {
				t.Errorf("SetStatus(%d): %v", i, err)
			}
		}()
	}
	wg.Wait()

	for i := 1; i <= n; i++ {
		wip := filepath.Join(dir, fmt.Sprintf("T-%04d-wip.md", i))
		if _, err := os.Stat(wip); err != nil {
			t.Errorf("ticket %d: wip file missing: %v", i, err)
			continue
		}
		if _, oerr := os.Stat(filepath.Join(dir, fmt.Sprintf("T-%04d-open.md", i))); oerr == nil {
			t.Errorf("ticket %d: open file must be gone", i)
		}
		bodyBytes, rerr := os.ReadFile(wip)
		if rerr != nil {
			t.Fatal(rerr)
		}
		body := string(bodyBytes)
		if !strings.Contains(body, fmt.Sprintf("# T-%04d · ENH: renamed-%d", i, i)) {
			t.Errorf("ticket %d: body missing renamed title", i)
		}
		assertConcCreationEntry(t, body, "tester")
		lines := concJournalLines(body)
		if len(lines) != 2 {
			t.Fatalf("ticket %d: want 2 status journal lines, got %d: %q", i, len(lines), lines)
		}
		titleComment := fmt.Sprintf("title: ticket-0 → renamed-%d", i)
		setComment := fmt.Sprintf("go %d", i)
		editAt := concJournalFind(lines, titleComment, "alice")
		setAt := concJournalFind(lines, setComment, "tester")
		if editAt < 0 || setAt < 0 {
			t.Fatalf("ticket %d: missing journal entries (edit=%d set=%d): %q", i, editAt, setAt, lines)
		}
		// The status change is always open → wip.
		assertConcJournalEntry(t, lines, setAt, "open", "wip", setComment, "tester")
		// The edit entry records the status the ticket carried when the
		// edit committed: open when the edit won the race, wip when the
		// set-status did.
		editStatus := "open"
		if editAt > setAt {
			editStatus = "wip"
		}
		assertConcJournalEntry(t, lines, editAt, editStatus, editStatus, titleComment, "alice")
		tk, _, err := domain.Parse(bodyBytes)
		if err != nil {
			t.Fatalf("ticket %d: Parse: %v", i, err)
		}
		if tk.Status != domain.StatusWip || tk.Title != fmt.Sprintf("renamed-%d", i) {
			t.Errorf("ticket %d: parsed status=%q title=%q", i, tk.Status, tk.Title)
		}
	}
	assertNoTmpFiles(t, dir)
}
