package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCreate_NumberBeyond4DigitsRejected pins the filename-codec edge:
// with T-9999 present, next = 10000 would produce "T-10000-open.md", a
// name ParseFilename rejects (exactly four digits). Create must fail
// with a distinct error instead of creating an invisible ticket.
func TestCreate_NumberBeyond4DigitsRejected(t *testing.T) {
	s, dir := newStore(t)
	seed := filepath.Join(dir, "T-9999-open.md")
	body := "# T-9999 · ENH: max\n\n- Status (Статус): open\n- Priority (Приоритет): normal\n- Created (Создан): 2026-09-02 00:00 · by (кем): seed\n- Project (Проект): tickets\n\n## Summary (Кратко)\nmax\n\n## Details (Подробности)\n\n## Journal (Журнал)\n- 2026-09-02 00:00 — тикет создан (seed).\n"
	if err := os.WriteFile(seed, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	n, err := s.Create(fakeTicket(0))
	if err == nil {
		t.Fatalf("Create after T-9999 = (%d, nil), want error", n)
	}
	if strings.Contains(err.Error(), "collision") {
		t.Errorf("error should be a range failure, not a collision: %v", err)
	}
	if !strings.Contains(err.Error(), "4-digit filename limit") {
		t.Errorf("error should mention the 4-digit filename limit; got %v", err)
	}
	if _, serr := os.Stat(filepath.Join(dir, "T-10000-open.md")); !os.IsNotExist(serr) {
		t.Errorf("invisible T-10000 file must not be created; stat err=%v", serr)
	}
}
