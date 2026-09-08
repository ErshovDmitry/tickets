package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// mixedOrderT0001 mirrors domain's ticketMixedOrder fixture (T-0070)
// renumbered to T-0001: a canonical ticket body with the Journal section
// moved before Summary.
const mixedOrderT0001 = `# T-0001 · BUG: Mixed section order

- Status (Статус): open
- Priority (Приоритет): high
- Created (Создан): 2026-09-10 10:00 · by (кем): tester
- Project (Проект): tickets

## Journal (Журнал)
- 2026-09-10 10:00 — тикет создан (tester).

## Summary (Кратко)
Mixed section order

## Details (Подробности)
details body

## User comments (Комментарии от пользователя)

## Comments (Комментарии)
`

// dupJournalSuffix is the second "## Journal" section appended by the
// dup-journal fixtures (T-0070); the leading blank line separates it
// from the canonical journal at EOF. Entry text is aligned with the
// cli cmd_set_mixed_test.go fixture of the same name.
const dupJournalSuffix = "\n## Journal (Журнал)\n- 2099-01-01 10:00 — статус: open → wip · dup entry (someone)\n"

// countHeaderLines counts body lines starting with the section header
// prefix hdr (e.g. "## Summary"), so stray occurrences inside prose do
// not inflate the count.
func countHeaderLines(body, hdr string) int {
	n := 0
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, hdr) {
			n++
		}
	}
	return n
}

// TestSetStatusMixedOrderNoDuplicates pins T-0070 C5: a cross-status
// SetStatus on a mixed-order file (Journal before Summary) rewrites it
// canonically — exactly one Summary and one Details header, the Details
// body preserved, no stub placeholder — and reports no warnings.
func TestSetStatusMixedOrderNoDuplicates(t *testing.T) {
	s, dir := newStore(t)
	if err := os.WriteFile(filepath.Join(dir, "T-0001-open.md"), []byte(mixedOrderT0001), 0o644); err != nil {
		t.Fatal(err)
	}

	target, warnings, err := s.SetStatus(1, domain.StatusWip, "tester", "go")
	if err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if want := filepath.Join(dir, "T-0001-wip.md"); target != want {
		t.Fatalf("target = %q, want %q", target, want)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want empty", warnings)
	}
	body := string(mustRead(t, dir, 1))
	for _, hdr := range []string{"## Summary", "## Details"} {
		if n := countHeaderLines(body, hdr); n != 1 {
			t.Errorf("%q occurs %d times, want 1:\n%s", hdr, n, body)
		}
	}
	if !strings.Contains(body, "details body") {
		t.Errorf("Details body lost:\n%s", body)
	}
	if strings.Contains(body, "<!--") {
		t.Errorf("stub placeholder leaked:\n%s", body)
	}
	assertJournalTransition(t, body, "open", "wip", "go", "tester")
}

// TestSetStatusDuplicateJournalMergesAndWarns pins T-0070 C5: a
// cross-status SetStatus on a dup-journal file merges both journal entry
// lines into the single canonical Journal of the new file and returns
// exactly one ErrJournalDup warning for the read source file.
func TestSetStatusDuplicateJournalMergesAndWarns(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "T-0001-open.md")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(before, []byte(dupJournalSuffix)...), 0o644); err != nil {
		t.Fatal(err)
	}

	_, warnings, err := s.SetStatus(1, domain.StatusWip, "tester", "go")
	if err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly 1", warnings)
	}
	if !errors.Is(warnings[0].Err, domain.ErrJournalDup) {
		t.Errorf("w.Err = %v, want errors.Is ErrJournalDup", warnings[0].Err)
	}
	if want := "T-0001-open.md"; warnings[0].Name != want {
		t.Errorf("w.Name = %q, want %q", warnings[0].Name, want)
	}
	body := string(mustRead(t, dir, 1))
	if n := countHeaderLines(body, "## Journal"); n != 1 {
		t.Errorf("## Journal occurs %d times, want 1:\n%s", n, body)
	}
	for _, want := range []string{"тикет создан", "dup entry (someone)", "статус: open → wip · go (tester)"} {
		if !strings.Contains(body, want) {
			t.Errorf("body misses %q:\n%s", want, body)
		}
	}
}

// TestSetStatusSameStatusDuplicateJournalWarns pins T-0070 C5: a
// same-status SetStatus (journal-only path) on a dup-journal wip file
// also surfaces the ErrJournalDup warning and rewrites the file with a
// single merged Journal.
func TestSetStatusSameStatusDuplicateJournalWarns(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SetStatus(1, domain.StatusWip, "tester", "start"); err != nil {
		t.Fatalf("SetStatus(open->wip): %v", err)
	}
	path := filepath.Join(dir, "T-0001-wip.md")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(before, []byte(dupJournalSuffix)...), 0o644); err != nil {
		t.Fatal(err)
	}

	target, warnings, err := s.SetStatus(1, domain.StatusWip, "tester", "note")
	if err != nil {
		t.Fatalf("SetStatus(same-status): %v", err)
	}
	if target != path {
		t.Errorf("target = %q, want unchanged %q", target, path)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly 1", warnings)
	}
	if !errors.Is(warnings[0].Err, domain.ErrJournalDup) {
		t.Errorf("w.Err = %v, want errors.Is ErrJournalDup", warnings[0].Err)
	}
	if want := "T-0001-wip.md"; warnings[0].Name != want {
		t.Errorf("w.Name = %q, want %q", warnings[0].Name, want)
	}
	body := string(mustRead(t, dir, 1))
	if n := countHeaderLines(body, "## Journal"); n != 1 {
		t.Errorf("## Journal occurs %d times, want 1:\n%s", n, body)
	}
	for _, want := range []string{"тикет создан", "dup entry (someone)", "статус: wip → wip · note (tester)"} {
		if !strings.Contains(body, want) {
			t.Errorf("body misses %q:\n%s", want, body)
		}
	}
}
