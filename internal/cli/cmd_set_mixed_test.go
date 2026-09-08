package cli_test

// T-0070 CLI-level tests for `set` on anomalous files: a duplicate
// Journal section must be merged with a single stderr warning (the
// domain.ErrJournalDup text via the cmd_list.go precedent format) while
// the command still succeeds. Split from cmd_set_test.go (300-line cap).

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/cli"
)

// dupJournalSuffix is the second "## Journal" section appended to a
// seeded ticket (T-0070); the leading blank line separates it from the
// canonical journal at EOF. Entry text matches the transition grammar
// (no trailing period) so Parse merges it into the journal.
const dupJournalSuffix = "\n## Journal (Журнал)\n- 2099-01-01 10:00 — статус: open → wip · dup entry (someone)\n"

// mixedDupWip is a wip ticket with BOTH anomalies: the Journal section
// moved before Summary (mixed order) and a duplicate Journal section
// directly after it (the dup header hits the journal-tail state, where
// Parse flags JournalDup), so a same-status set must warn and normalize.
const mixedDupWip = `# T-0001 · BUG: Mixed wip ticket

- Status (Статус): wip
- Priority (Приоритет): high
- Created (Создан): 2026-09-10 10:00 · by (кем): tester
- Project (Проект): tickets

## Journal (Журнал)
- 2026-09-10 10:00 — тикет создан (tester).

## Journal (Журнал)
- 2099-01-01 10:00 — статус: open → wip · dup entry (someone)

## Summary (Кратко)
Mixed wip ticket

## Details (Подробности)
details body

## User comments (Комментарии от пользователя)

## Comments (Комментарии)
`

// countJournalHeaders counts body lines with the given section header
// prefix (e.g. "## Journal"), so stray occurrences inside prose do not
// inflate the count.
func countJournalHeaders(body, hdr string) int {
	n := 0
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, hdr) {
			n++
		}
	}
	return n
}

// TestSet_DupJournalMergesAndWarns pins T-0070 C4/C5: a cross-status
// `set` on a ticket whose file carries a second Journal section exits 0,
// prints the ErrJournalDup warning on stderr, and the new wip file holds
// a single merged Journal (created + dup + transition entries).
func TestSet_DupJournalMergesAndWarns(t *testing.T) {
	env, dir := newTicketDir(t)
	env["TICKET_WHO"] = "agent" // self-documenting who for journal entries
	path := filepath.Join(dir, "T-0001-open.md")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(before, []byte(dupJournalSuffix)...), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"set", "1", "wip", "x"}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(set) = %d, want 0; stderr: %q", code, stderr.String())
	}
	if got := stderr.String(); !strings.Contains(got, "дубль секции") {
		t.Errorf("stderr = %q, want ErrJournalDup warning («дубль секции»)", got)
	}
	body, err := os.ReadFile(filepath.Join(dir, "T-0001-wip.md"))
	if err != nil {
		t.Fatal(err)
	}
	if n := countJournalHeaders(string(body), "## Journal"); n != 1 {
		t.Errorf("## Journal occurs %d times, want 1:\n%s", n, body)
	}
	for _, want := range []string{"тикет создан", "dup entry (someone)", "статус: open → wip · x (agent)"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("body misses %q:\n%s", want, body)
		}
	}
}

// TestSet_SameStatusMixedFileWarns pins T-0070 C4/C5: a same-status
// journal-only `set` on a mixed-order dup-journal wip file exits 0,
// warns with ErrJournalDup, and normalizes the file on disk — a single
// Journal header, Summary and Details each exactly once.
func TestSet_SameStatusMixedFileWarns(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir, "TICKET_WHO": "agent"}
	if err := os.WriteFile(filepath.Join(dir, "T-0001-wip.md"), []byte(mixedDupWip), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"set", "1", "wip", "note"}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(set same-status) = %d, want 0; stderr: %q", code, stderr.String())
	}
	if got := stderr.String(); !strings.Contains(got, "дубль секции") {
		t.Errorf("stderr = %q, want ErrJournalDup warning («дубль секции»)", got)
	}
	body, err := os.ReadFile(filepath.Join(dir, "T-0001-wip.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, hdr := range []string{"## Journal", "## Summary", "## Details"} {
		if n := countJournalHeaders(string(body), hdr); n != 1 {
			t.Errorf("%q occurs %d times, want 1:\n%s", hdr, n, body)
		}
	}
}
