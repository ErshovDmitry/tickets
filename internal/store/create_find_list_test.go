package store

// Create/Find/List behaviours outside the SetStatus commit path:
// sequential and concurrent numbering, the written body of a fresh
// Create, and Find/List robustness (corrupt files, hidden entries).

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"ticket/internal/domain"
)

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
		Lang:     domain.LangRU,
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
		"- Status (Статус): open",
		"- Priority (Приоритет): high",
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
