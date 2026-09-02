package store

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
// helpers
// ----------------------------------------------------------------------------

// newStore creates a per-test temp directory and a Store backed by it.
func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	return s, dir
}

// mustRead reads the ticket file whose number equals n inside dir, dying
// on any error.
func mustRead(t *testing.T, dir string, n int) []byte {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, fmt.Sprintf("T-%04d-*.md", n)))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly one ticket %d, got %d files: %v", n, len(matches), matches)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read %s: %v", matches[0], err)
	}
	return data
}

// fakeTicket builds a minimal in-memory ticket that domain.RenderNewTicket
// can render.
func fakeTicket(n int) *domain.Ticket {
	return &domain.Ticket{
		Number:   n,
		Type:     domain.TypeENH,
		Priority: domain.PriorityNormal,
		Title:    fmt.Sprintf("ticket-%d", n),
		Details:  "details for ticket " + fmt.Sprint(n),
		Who:      "tester",
		Project:  "tickets",
		Status:   domain.StatusOpen,
	}
}

// tsLayout is the bash-compatible journal timestamp layout (§2).
const tsLayout = "2006-01-02 15:04"

// assertJournalTransition finds the transition journal line in body and
// asserts it byte-equals the bash reference shape
// "- <ts> — статус: <from> → <to>[ · <comment>] (<who>)", where <ts> must
// be exactly len(tsLayout) chars parsing as tsLayout. No regex involved.
func assertJournalTransition(t *testing.T, body, from, to, comment, who string) {
	t.Helper()
	const marker = " — статус: "
	for _, line := range strings.Split(body, "\n") {
		if !strings.Contains(line, marker) {
			continue
		}
		if !strings.HasPrefix(line, "- ") {
			t.Fatalf("journal line must start with \"- \": %q", line)
		}
		rest := line[2:]
		if i := strings.Index(rest, marker); i != len(tsLayout) {
			t.Fatalf("timestamp must be %d chars: %q", len(tsLayout), line)
		}
		ts := rest[:len(tsLayout)]
		if _, err := time.Parse(tsLayout, ts); err != nil {
			t.Fatalf("timestamp %q does not parse as %q: %v", ts, tsLayout, err)
		}
		want := fmt.Sprintf("- %s — статус: %s → %s", ts, from, to)
		if comment != "" {
			want += " · " + comment
		}
		want += " (" + who + ")"
		if line != want {
			t.Fatalf("journal line byte mismatch\n got: %q\nwant: %q", line, want)
		}
		return
	}
	t.Fatalf("no journal transition line in body:\n%s", body)
}

// swapLink sets linkFile for the duration of t.
func swapLink(t *testing.T, fn func(string, string) error) {
	t.Helper()
	orig := linkFile
	linkFile = fn
	t.Cleanup(func() { linkFile = orig })
}

// swapRemove sets removeFile for the duration of t.
func swapRemove(t *testing.T, fn func(string) error) {
	t.Helper()
	orig := removeFile
	removeFile = fn
	t.Cleanup(func() { removeFile = orig })
}

// assertNoTmpFiles fails the test if dir contains *.tmp entries.
func assertNoTmpFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leaked tmp file: %s", e.Name())
		}
	}
}

// ----------------------------------------------------------------------------
// New — directory validation
// ----------------------------------------------------------------------------

func TestNew_OK(t *testing.T) {
	s, _ := newStore(t)
	if s == nil {
		t.Fatal("expected non-nil Store")
	}
}

func TestNew_NotADirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(file); err == nil {
		t.Fatal("expected error for non-directory")
	}
}

func TestNew_MissingDirectory(t *testing.T) {
	if _, err := New(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected error for missing dir")
	}
}

// ----------------------------------------------------------------------------
// Create — basic numbering (a, b)
// ----------------------------------------------------------------------------

func TestCreate_AssignsSequentialNumbers(t *testing.T) {
	s, _ := newStore(t)
	for _, want := range []int{1, 2, 3} {
		got, err := s.Create(fakeTicket(0))
		if err != nil {
			t.Fatalf("Create #%d: %v", want, err)
		}
		if got != want {
			t.Fatalf("Create: want %d, got %d", want, got)
		}
	}
}

func TestCreate_PicksNextNumberAfterExistingFile(t *testing.T) {
	s, dir := newStore(t)
	if err := os.WriteFile(filepath.Join(dir, "T-0001-open.md"), []byte("# T-0001 · ENH: seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := s.Create(fakeTicket(0))
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Fatalf("want 2, got %d", got)
	}
}

func TestCreate_WritesOpenTicketOnly(t *testing.T) {
	s, dir := newStore(t)
	n, err := s.Create(&domain.Ticket{
		Number:   0,
		Type:     domain.TypeBUG,
		Priority: domain.PriorityHigh,
		Title:    "My first ticket",
		Details:  "Some details.",
		Who:      "tester",
		Project:  "tickets",
		Status:   "", // intentionally blank → defaults to open
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1, got %d", n)
	}
	body := string(mustRead(t, dir, 1))
	for _, want := range []string{
		"# T-0001 · BUG: My first ticket",
		"- Статус: open",
		"- Приоритет: high",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q\n---\n%s\n---", want, body)
		}
	}
}

func TestCreate_RejectsNonOpen(t *testing.T) {
	s, _ := newStore(t)
	tk := fakeTicket(0)
	tk.Status = domain.StatusWip
	if _, err := s.Create(tk); err == nil {
		t.Fatal("expected error when Create is called with non-open status")
	}
}

func TestCreate_NilTicket(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.Create(nil); err == nil {
		t.Fatal("expected error for nil ticket")
	}
}

// ----------------------------------------------------------------------------
// Create — concurrent numbering (c)
// ----------------------------------------------------------------------------

func TestCreate_ConcurrentAssignsDistinctNumbers(t *testing.T) {
	s, _ := newStore(t)
	const n = 10
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		seen = make(map[int]bool)
	)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			got, err := s.Create(fakeTicket(0))
			if err != nil {
				t.Errorf("concurrent Create: %v", err)
				return
			}
			mu.Lock()
			seen[got] = true
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(seen) != n {
		t.Fatalf("expected %d distinct numbers, got %d: %v", n, len(seen), seen)
	}
	for i := 1; i <= n; i++ {
		if !seen[i] {
			t.Errorf("missing number %d", i)
		}
	}
}

// ----------------------------------------------------------------------------
// Find
// ----------------------------------------------------------------------------

func TestFind_OK(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	tk, err := s.Find(1)
	if err != nil {
		t.Fatalf("Find(1): %v", err)
	}
	if tk.Number != 1 {
		t.Fatalf("Find: want Number=1, got %d", tk.Number)
	}
}

func TestFind_NotFound(t *testing.T) {
	s, _ := newStore(t)
	_, err := s.Find(9999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFind_CorruptFile_ReturnsWrappedErr(t *testing.T) {
	s, dir := newStore(t)
	if err := os.WriteFile(filepath.Join(dir, "T-0001-open.md"), []byte("garbage — no markdown structure"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := s.Find(1)
	if err == nil {
		t.Fatal("expected error from corrupt file")
	}
}

// ----------------------------------------------------------------------------
// List — mixed good/bad (f)
// ----------------------------------------------------------------------------

func TestList_PartialCorruptionReturnsWarnings(t *testing.T) {
	s, dir := newStore(t)
	// T-0001-open.md valid.
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	// T-0002-open.md structurally malformed.
	if err := os.WriteFile(filepath.Join(dir, "T-0002-open.md"), []byte("definitely not a ticket"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A file with a wrong name pattern.
	if err := os.WriteFile(filepath.Join(dir, "T-NOT-A-TICKET.md"), []byte("# garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	// .tmp file left over — must not appear.
	if err := os.WriteFile(filepath.Join(dir, ".T-9999-open.1234-5.md.tmp"), []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	// .lock file — must not appear.
	if err := os.WriteFile(filepath.Join(dir, ".lock"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	ts, warns := s.List()
	if len(ts) != 1 || ts[0].Number != 1 {
		t.Fatalf("expected exactly ticket #1, got %d tickets: %+v", len(ts), ts)
	}
	if len(warns) < 1 {
		t.Fatalf("expected at least one ParseWarning for the bad files, got 0")
	}
	// The hidden and tmp files must not have leaked into either slice.
	for _, tk := range ts {
		if tk.Number == 9999 {
			t.Errorf("tmp file leaked into ticket list")
		}
	}
	for _, w := range warns {
		if w.Name == ".lock" {
			t.Errorf(".lock should not produce a warning: %v", w)
		}
		if w.Name == ".T-9999-open.1234-5.md.tmp" {
			t.Errorf(".tmp file should not produce a warning: %v", w)
		}
	}
}

// ----------------------------------------------------------------------------
// SetStatus — journal byte-shape (d)
// ----------------------------------------------------------------------------

func TestSetStatus_UpdatesNameAndAppendsJournal(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetStatus(1, domain.StatusWip, "tester", "starting"); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	// Old filename must be gone.
	if _, err := os.Lstat(filepath.Join(dir, "T-0001-open.md")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected old filename gone, Lstat err=%v", err)
	}
	// New filename must exist.
	data, err := os.ReadFile(filepath.Join(dir, "T-0001-wip.md"))
	if err != nil {
		t.Fatalf("read new file: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "- Статус: wip") {
		t.Errorf("body missing '- Статус: wip' line\n%s", body)
	}
	assertJournalTransition(t, body, "open", "wip", "starting", "tester")
}

func TestSetStatus_NoComment(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetStatus(1, domain.StatusWip, "tester", ""); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	assertJournalTransition(t, string(mustRead(t, dir, 1)), "open", "wip", "", "tester")
}

func TestSetStatus_PreservesUnknownBytes(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	extra := "\n\n<!-- manual analyst note: do not touch -->\n"
	if err := os.WriteFile(filepath.Join(dir, "T-0001-open.md"), append(mustRead(t, dir, 1), []byte(extra)...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetStatus(1, domain.StatusWip, "tester", "go"); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	body := string(mustRead(t, dir, 1))
	if !strings.Contains(body, "manual analyst note") {
		t.Fatalf("unknown bytes lost after SetStatus; body:\n%s", body)
	}
}

func TestSetStatus_SameStatusRefused(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	_, err := s.SetStatus(1, domain.StatusOpen, "tester", "")
	if err == nil {
		t.Fatal("expected error for no-op status change")
	}
}

func TestSetStatus_CollisionOnExistingTarget(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil { // T-0001-open.md
		t.Fatal(err)
	}
	// Pre-create the target filename to force the collision pre-check.
	if err := os.WriteFile(filepath.Join(dir, "T-0001-wip.md"), []byte("# pre-existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := s.SetStatus(1, domain.StatusWip, "tester", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrCollision) {
		t.Fatalf("expected ErrCollision, got %v", err)
	}
}

func TestSetStatus_NotFound(t *testing.T) {
	s, _ := newStore(t)
	_, err := s.SetStatus(4242, domain.StatusWip, "tester", "ghost")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

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
	_, err := s.SetStatus(1, domain.StatusWip, "tester", "")
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
	_, err := s.SetStatus(1, domain.StatusWip, "tester", "")
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
	_, err := s.SetStatus(1, domain.StatusWip, "tester", "")
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

// TestConcurrent_CreateAndSetStatus_NoJournalLoss runs Create and SetStatus
// genuinely in parallel: one producer goroutine creates n tickets while n
// waiter goroutines each poll for their number and transition it as soon as
// it appears. Every ticket must end as a wip file whose journal keeps the
// creation line and exactly one byte-exact open → wip transition line.
func TestConcurrent_CreateAndSetStatus_NoJournalLoss(t *testing.T) {
	s, dir := newStore(t)
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
			deadline := time.Now().Add(5 * time.Second)
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
			if _, err := s.SetStatus(i, domain.StatusWip, "tester", fmt.Sprintf("go %d", i)); err != nil {
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
