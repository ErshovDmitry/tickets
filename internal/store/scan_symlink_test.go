package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ticket/internal/domain"
)

// makeOutsideTicket writes a fully valid T-0001 ticket body OUTSIDE the
// store directory and returns its path plus the exact bytes. Linked from
// inside the store by a ticket-named symlink it is indistinguishable from
// a real ticket by content alone — the scan must reject the link by entry
// TYPE, not by content.
func makeOutsideTicket(t *testing.T, outsideDir string) (string, []byte) {
	t.Helper()
	body := []byte("# T-0001 · BUG: outside bait\n" +
		"\n" +
		"- Статус: open\n" +
		"- Приоритет: normal\n" +
		"- Создан: 2026-09-02 10:00 · кем: attacker\n" +
		"- Проект: tickets\n" +
		"\n" +
		"## Кратко\nTOPSECRET-SENTINEL\n" +
		"\n## Подробности\noutside body\n" +
		"\n## Журнал\n" +
		"- 2026-09-02 10:00 — тикет создан (attacker).\n")
	p := filepath.Join(outsideDir, "T-0001-open.md")
	if err := os.WriteFile(p, body, 0o600); err != nil {
		t.Fatalf("write outside ticket: %v", err)
	}
	return p, body
}

// symlinkOutsideTicket builds the store layout used by the containment
// regressions: <root>/tickets (store dir) with a ticket-named symlink to
// a valid-looking ticket body stored in <root>/outside. Skips the test
// when the platform cannot create symlinks (e.g. unprivileged Windows).
func symlinkOutsideTicket(t *testing.T) (s *Store, target string, body []byte) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "tickets")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	target, body = makeOutsideTicket(t, outside)
	link := filepath.Join(dir, "T-0001-open.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}
	s, err := New(dir)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	return s, target, body
}

// assertOutsideUntouched verifies the outside sentinel still holds its
// original bytes and that no extra .md file appeared next to it.
func assertOutsideUntouched(t *testing.T, target string, body []byte) {
	t.Helper()
	got, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("outside sentinel changed (err=%v, data=%q)", err, got)
	}
	matches, gerr := filepath.Glob(filepath.Join(filepath.Dir(target), "*.md"))
	if gerr != nil || len(matches) != 1 {
		t.Fatalf("outside dir mutated: files=%v globerr=%v", matches, gerr)
	}
}

// TestScan_RejectsSymlinkedTicketName is the V19 containment regression:
// os.ReadFile follows symlinks, so a ticket-named symlink inside
// TICKETS_DIR made scan admit an entry whose content lives outside the
// storage directory. The scan must reject the entry by TYPE (symlink),
// loudly, while regular files keep scanning.
func TestScan_RejectsSymlinkedTicketName(t *testing.T) {
	s, target, body := symlinkOutsideTicket(t)

	entries, warns, err := s.scan()
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("symlink admitted as ticket entry: %+v", entries)
	}
	if len(warns) != 1 || warns[0].Name != "T-0001-open.md" {
		t.Errorf("want exactly 1 warning for T-0001-open.md, got %+v", warns)
	}
	assertOutsideUntouched(t, target, body)
}

// TestListFindSet_CannotEscapeViaSymlinkedTicketName pins the consumer
// side of the containment: List, Find, FindNamed and SetStatus must all
// treat the symlinked name as absent and never read or mutate anything
// outside TICKETS_DIR.
func TestListFindSet_CannotEscapeViaSymlinkedTicketName(t *testing.T) {
	s, target, body := symlinkOutsideTicket(t)

	tickets, warns := s.List()
	if len(tickets) != 0 {
		t.Errorf("List returned tickets through a symlink: %+v", tickets)
	}
	if len(warns) == 0 {
		t.Errorf("List dropped the symlink warning")
	}

	if _, err := s.Find(1); !errors.Is(err, ErrNotFound) {
		t.Errorf("Find(1) err = %v, want ErrNotFound", err)
	}
	if _, _, err := s.FindNamed(1); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindNamed(1) err = %v, want ErrNotFound", err)
	}
	if _, err := s.SetStatus(1, domain.StatusWip, "tester", "escape attempt"); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetStatus err = %v, want ErrNotFound (no outside mutation)", err)
	}
	assertOutsideUntouched(t, target, body)
}

// TestScan_RegularFilesStillScanned guards against over-rejection: after
// the non-regular filter, an ordinary ticket file must scan exactly as
// before.
func TestScan_RegularFilesStillScanned(t *testing.T) {
	s, dir := newStore(t)
	makeFile(t, dir, "T-0001-open.md", "regular body")
	entries, warns, err := s.scan()
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(entries) != 1 || entries[0].Number != 1 || entries[0].Status != domain.StatusOpen {
		t.Fatalf("regular file mis-scanned: entries=%+v warns=%+v", entries, warns)
	}
	if len(warns) != 0 {
		t.Errorf("regular file produced warnings: %+v", warns)
	}
}
