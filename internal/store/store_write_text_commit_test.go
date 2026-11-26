package store

// T-0240 A1 commit-path tests: DS8 surroundings preservation, concurrent
// appends, lookup/lock gates (edit policy) and EN journal events. Split from
// store_write_text_test.go to keep each file within the 300-line budget.

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"ticket/internal/domain"
)

const preservedSrc = "# T-0001 · ENH: ticket-0\n\n" +
	"- Status (Статус): open\n- Priority (Приоритет): normal\n" +
	"- Created (Создан): 2026-01-01 10:00 · by (кем): tester\n- Project (Проект): tickets\n\n" +
	"## Summary (Мой итог)\n\nbrief line\n\n" +
	"## Details (Мои подробности)\n\ndetails line\n\n" +
	"## User comments (Комментарии от пользователя)\n\nuser line\n\n" +
	"## Comments (Комментарии)\n\ncomment line\n\n" +
	"## Journal (Журнал)\n\n- 2026-01-01 10:00 — тикет создан (tester).\n\nmanual tail bytes\n"

// TestWriteText_PreservesSurroundings is the mandatory DS8 check: neighbours,
// unknown tail and original header templates survive the re-render byte-for-byte.
func TestWriteText_PreservesSurroundings(t *testing.T) {
	ops := []struct {
		name string
		run  func(s *Store) error
		keep []string
		gone string
	}{
		{"comment", func(s *Store) error { _, err := s.AddComment(1, "new note", "alice"); return err },
			[]string{"details line", "comment line", "new note"}, ""},
		{"section", func(s *Store) error {
			_, err := s.SetSection(1, domain.SectionDetails, "new details", "alice")
			return err
		},
			[]string{"comment line", "new details"}, "details line"},
	}
	for _, op := range ops {
		t.Run(op.name, func(t *testing.T) {
			s, dir := newStore(t)
			writeTicketFileBytes(t, dir, "T-0001-open.md", []byte(preservedSrc))
			before, beforeTail, err := domain.Parse([]byte(preservedSrc))
			if err != nil {
				t.Fatal(err)
			}
			if err := op.run(s); err != nil {
				t.Fatalf("mutation: %v", err)
			}
			afterBytes := mustRead(t, dir, 1)
			after, afterTail, err := domain.Parse(afterBytes)
			if err != nil {
				t.Fatal(err)
			}
			if !maps.Equal(before.RawTemplates.Headers, after.RawTemplates.Headers) {
				t.Fatalf("header templates changed: %v -> %v", before.RawTemplates.Headers, after.RawTemplates.Headers)
			}
			if !bytes.Equal(afterTail, beforeTail) || !bytes.HasSuffix(afterBytes, afterTail) {
				t.Fatalf("unknown tail changed: %q -> %q", beforeTail, afterTail)
			}
			body := string(afterBytes)
			keep := append([]string{"brief line", "user line",
				"- Created (Создан): 2026-01-01 10:00 · by (кем): tester",
				"- 2026-01-01 10:00 — тикет создан (tester)."}, op.keep...)
			for _, k := range keep {
				if !strings.Contains(body, k) {
					t.Fatalf("lost %q:\n%s", k, body)
				}
			}
			if op.gone != "" && strings.Contains(body, op.gone) {
				t.Fatalf("replaced body %q survived:\n%s", op.gone, body)
			}
			if tr := journalTransitions(body); len(tr) != 1 {
				t.Fatalf("want exactly one new journal line, got %v", tr)
			}
		})
	}
}

func TestWriteText_NotFound(t *testing.T) {
	s, _ := newStore(t)
	_, errC := s.AddComment(99, "x", "alice")
	_, errS := s.SetSection(99, domain.SectionSummary, "x", "alice")
	if !errors.Is(errC, ErrNotFound) || !errors.Is(errS, ErrNotFound) {
		t.Fatalf("err = %v / %v, want ErrNotFound", errC, errS)
	}
}

func TestWriteText_Ambiguous(t *testing.T) {
	s, dir := seededStore(t)
	body := mustRead(t, dir, 1)
	writeTicketFileBytes(t, dir, "T-0001-done.md", body)
	if _, err := s.AddComment(1, "x", "alice"); !errors.Is(err, ErrAmbiguousNumber) {
		t.Fatalf("err = %v, want ErrAmbiguousNumber", err)
	}
	after, rerr := os.ReadFile(filepath.Join(dir, "T-0001-open.md"))
	if rerr != nil || !bytes.Equal(after, body) {
		t.Fatal("ambiguous refusal changed the file")
	}
}

func TestWriteText_DeletedReadOnly(t *testing.T) {
	s, dir := seededStore(t)
	body := mustRead(t, dir, 1)
	if err := os.MkdirAll(filepath.Join(dir, "deleted"), 0o755); err != nil {
		t.Fatal(err)
	}
	deleted := writeTicketFileBytes(t, dir, filepath.Join("deleted", "T-0001-deleted.md"), body)
	if err := os.Remove(filepath.Join(dir, "T-0001-open.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddComment(1, "x", "alice"); err == nil {
		t.Fatal("deleted ticket accepted a write")
	}
	if after, rerr := os.ReadFile(deleted); rerr != nil || !bytes.Equal(after, body) {
		t.Fatalf("deleted file changed: %v", rerr)
	}
}

func TestWriteText_ReadOnly(t *testing.T) {
	_, dir := seededStore(t)
	rs, err := NewReadOnly(dir)
	if err != nil {
		t.Fatal(err)
	}
	before := mustRead(t, dir, 1)
	_, err = rs.AddComment(1, "x", "alice")
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("err = %v, want ErrReadOnly", err)
	}
	assertWriteRefused(t, dir, 1, before, err)
	_, err = rs.SetSection(1, domain.SectionSummary, "x", "alice")
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("err = %v, want ErrReadOnly", err)
	}
	assertWriteRefused(t, dir, 1, before, err)
}

func TestWriteText_ArchiveInPlace(t *testing.T) {
	s, dir := makeDone(t)
	if _, err := s.Archive(1, "tester"); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if _, err := s.AddComment(1, "archived note", "alice"); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	if total := countTicketFiles(t, dir, 1); total != 1 {
		t.Fatalf("ticket files = %d, want 1 (archive only)", total)
	}
	body, err := os.ReadFile(filepath.Join(dir, "archive", "T-0001-done.md"))
	if err != nil {
		t.Fatal(err)
	}
	tr := journalTransitions(string(body))
	if !strings.Contains(string(body), "archived note") || len(tr) == 0 ||
		!strings.Contains(tr[len(tr)-1], "статус: done → done · comment: +1 строк (alice)") {
		t.Fatalf("archive append wrong:\n%s", body)
	}
}

func TestAddComment_Concurrent(t *testing.T) {
	s, dir := seededStore(t)
	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := s.AddComment(1, fmt.Sprintf("note-%d", i), "alice"); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent AddComment: %v", err)
	}
	body := string(mustRead(t, dir, 1))
	for i := range n {
		if !strings.Contains(body, fmt.Sprintf("note-%d", i)) {
			t.Fatalf("lost note-%d:\n%s", i, body)
		}
	}
	if tr := journalTransitions(body); len(tr) != n {
		t.Fatalf("journal events = %d, want %d", len(tr), n)
	}
}

// enSrc: plain-EN ticket — detectLang yields LangEN, journal events take EN forms.
const enSrc = "# T-0001 · ENH: ticket-0\n\n" +
	"- Status: open\n- Priority: normal\n- Created: 2026-01-01 10:00 · by: tester\n- Project: tickets\n\n" +
	"## Summary\n\nticket-0\n\n## Details\n\nd\n\n## User comments\n\n## Comments\n\n" +
	"## Journal\n\n- 2026-01-01 10:00 — ticket created (tester).\n"

func TestWriteText_JournalEN(t *testing.T) {
	s, dir := newStore(t)
	writeTicketFileBytes(t, dir, "T-0001-open.md", []byte(enSrc))
	if _, err := s.AddComment(1, "note", "alice"); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	if body := string(mustRead(t, dir, 1)); !strings.Contains(body, "comment: +1 lines (alice)") {
		t.Fatalf("EN event missing:\n%s", body)
	}
	if _, err := s.SetSection(1, domain.SectionSummary, "brief", "alice"); err != nil {
		t.Fatalf("SetSection: %v", err)
	}
	if body := string(mustRead(t, dir, 1)); !strings.Contains(body, "section summary: replaced (alice)") {
		t.Fatalf("EN event missing:\n%s", body)
	}
}
