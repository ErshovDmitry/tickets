package cli_test

// T-0187 success-path tests: silent success, per-field journal lines in
// the fixed order with one timestamp, the H1 rewrite, default Summary
// carried to the new title, custom Brief preserved, an archived ticket
// edited in place, and the shared not-found / ambiguous handlers.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// editJournalRe matches one edit journal line: status stays open → open,
// the comment is "field: old → new", who defaults to "agent". The first
// group captures the FULL timestamp (tsPattern's own optional-seconds
// group lands in m[2]), so callers can pin the one-shared-timestamp rule.
var editJournalRe = regexp.MustCompile(
	`(?m)^- (` + tsPattern + `) — статус: open → open · ([a-z]+): (.+) → (.+) \(agent\)$`)

// journalEntries extracts the timestamp and the "field: old → new" triple
// from each edit line of the Journal section of a rendered ticket body,
// in file order. Lines that are not edit entries (creation, archive,
// plain status transitions) are skipped.
func journalEntries(t *testing.T, body string) [][4]string {
	t.Helper()
	sec := strings.SplitN(body, "## Journal (Журнал)\n\n", 2)
	if len(sec) != 2 {
		t.Fatalf("body has no Journal section:\n%s", body)
	}
	var out [][4]string
	for line := range strings.SplitSeq(strings.TrimRight(sec[1], "\n"), "\n") {
		if m := editJournalRe.FindStringSubmatch(line); m != nil {
			// m[1] is the full timestamp; m[2] is tsPattern's own
			// optional-seconds group; field/old/new live in m[3..5].
			out = append(out, [4]string{m[1], m[3], m[4], m[5]})
		}
	}
	return out
}

// TestEditTitleSuccess pins: exit 0, both streams empty, the H1 rewritten,
// the title journal line appended, the default Summary carried to the new
// title (renderer default, T-0166 semantics via the store).
func TestEditTitleSuccess(t *testing.T) {
	dir := t.TempDir()
	createTestTicket(t, dir)

	stdout, stderr, code := editRun(t, dir, "edit", "1", "--title", "Новый заголовок")
	if code != 0 {
		t.Fatalf("Run = %d, want 0; stderr: %q", code, stderr)
	}
	if stdout != "" || stderr != "" {
		t.Errorf("streams = %q/%q, want both empty (silent success)", stdout, stderr)
	}
	body := readTicket(t, dir)
	if !strings.Contains(body, "# T-0001 · BUG: Новый заголовок") {
		t.Errorf("H1 not rewritten:\n%s", body)
	}
	// The old title legitimately survives inside the journal comment
	// ("title: old → new"); the H1 and the carried Summary must not keep it.
	if strings.Contains(body, "# T-0001 · BUG: Тестовый тикет") {
		t.Errorf("H1 kept the old title:\n%s", body)
	}
	if !strings.Contains(body, "## Summary (Кратко)\n\nНовый заголовок") {
		t.Errorf("default Summary not carried to the new title:\n%s", body)
	}
	entries := journalEntries(t, body)
	if len(entries) != 1 ||
		entries[0][1] != "title" || entries[0][2] != "Тестовый тикет" || entries[0][3] != "Новый заголовок" {
		t.Errorf("journal entries = %v, want exactly the title change", entries)
	}
}

// TestEditAllFieldsSuccess pins the fixed journal order (title, type,
// priority, project), the ONE shared timestamp captured from and compared
// across all four entries, and the metadata rewrite for a single
// all-fields edit.
func TestEditAllFieldsSuccess(t *testing.T) {
	dir := t.TempDir()
	createTestTicket(t, dir)

	_, stderr, code := editRun(t, dir,
		"edit", "1", "--project", "beta", "--priority", "high", "--type", "ENH", "--title", "Всё новое")
	if code != 0 {
		t.Fatalf("Run = %d, want 0; stderr: %q", code, stderr)
	}
	body := readTicket(t, dir)
	for _, want := range []string{
		"# T-0001 · ENH: Всё новое",
		"- Priority (Приоритет): high",
		"- Project (Проект): beta",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	entries := journalEntries(t, body)
	if len(entries) != 4 {
		t.Fatalf("journal entries = %v, want 4", entries)
	}
	assertSharedTimestamp(t, entries)
	for i, field := range []string{"title", "type", "priority", "project"} {
		if entries[i][1] != field {
			t.Errorf("entry %d field = %q, want %q (fixed order)", i, entries[i][1], field)
		}
	}
	if entries[0][2] != "Тестовый тикет" || entries[0][3] != "Всё новое" ||
		entries[1][2] != "BUG" || entries[1][3] != "ENH" ||
		entries[2][2] != "normal" || entries[2][3] != "high" ||
		entries[3][3] != "beta" {
		t.Errorf("journal old/new values wrong: %v", entries)
	}
}

// assertSharedTimestamp pins the one-shared-timestamp rule (R8): every
// edit journal entry of a single edit carries the SAME captured timestamp.
func assertSharedTimestamp(t *testing.T, entries [][4]string) {
	t.Helper()
	ts := entries[0][0]
	if ts == "" {
		t.Fatalf("journal entries carry no timestamp: %v", entries)
	}
	for i, e := range entries {
		if e[0] != ts {
			t.Errorf("entry %d timestamp = %q, want the shared %q", i, e[0], ts)
		}
	}
}

// TestEditCustomBriefPreserved pins: a ticket whose Summary holds a custom
// Brief (anything other than the title) keeps it across a title edit.
func TestEditCustomBriefPreserved(t *testing.T) {
	dir := t.TempDir()
	createTestTicket(t, dir)
	path := filepath.Join(dir, "T-0001-open.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	custom := strings.Replace(string(body),
		"## Summary (Кратко)\n\nТестовый тикет", "## Summary (Кратко)\n\nСвой бриф", 1)
	if err := os.WriteFile(path, []byte(custom), 0o644); err != nil {
		t.Fatalf("write custom Brief: %v", err)
	}

	if _, stderr, code := editRun(t, dir, "edit", "1", "--title", "Новый заголовок"); code != 0 {
		t.Fatalf("Run = %d, want 0; stderr: %q", code, stderr)
	}
	after := readTicket(t, dir)
	if !strings.Contains(after, "## Summary (Кратко)\n\nСвой бриф") {
		t.Errorf("custom Brief not preserved:\n%s", after)
	}
	if !strings.Contains(after, "# T-0001 · BUG: Новый заголовок") {
		t.Errorf("H1 not rewritten:\n%s", after)
	}
}

// TestEditArchivedInPlace pins the C2/architecture decision: an archived
// ticket is edited inside archive/ — no reopen, no move back, the file
// name keeps its done status.
func TestEditArchivedInPlace(t *testing.T) {
	dir := t.TempDir()
	createTestTicket(t, dir)
	if _, stderr, code := editRun(t, dir, "set", "1", "done", "готово"); code != 0 {
		t.Fatalf("set done = %d; stderr: %q", code, stderr)
	}
	if _, stderr, code := editRun(t, dir, "archive", "1"); code != 0 {
		t.Fatalf("archive = %d; stderr: %q", code, stderr)
	}
	archived := filepath.Join(dir, "archive", "T-0001-done.md")
	if _, err := os.Stat(archived); err != nil {
		t.Fatalf("archived ticket missing: %v", err)
	}

	stdout, stderr, code := editRun(t, dir, "edit", "1", "--title", "Архивный заголовок")
	if code != 0 {
		t.Fatalf("Run = %d, want 0; stderr: %q", code, stderr)
	}
	if stdout != "" || stderr != "" {
		t.Errorf("streams = %q/%q, want both empty", stdout, stderr)
	}
	body, err := os.ReadFile(archived)
	if err != nil {
		t.Fatalf("read archived ticket: %v", err)
	}
	if !strings.Contains(string(body), "# T-0001 · BUG: Архивный заголовок") {
		t.Errorf("archived H1 not rewritten:\n%s", body)
	}
	main, _ := filepath.Glob(filepath.Join(dir, "T-0001-*.md"))
	if len(main) != 0 {
		t.Errorf("ticket leaked back from the archive: %v", main)
	}
}

// TestEditNotFound pins the shared not-found handler (hinted) and the
// digit-less number argument path.
func TestEditNotFound(t *testing.T) {
	dir := t.TempDir()
	createTestTicket(t, dir)

	_, stderr, code := editRun(t, dir, "edit", "42", "--title", "x")
	if code != 1 {
		t.Fatalf("Run(edit 42) = %d, want 1; stderr: %q", code, stderr)
	}
	if !strings.Contains(stderr, "тикет «42» не найден") {
		t.Errorf("stderr = %q, want not-found report", stderr)
	}

	_, stderr, code = editRun(t, dir, "edit", "x", "--title", "y")
	if code != 1 {
		t.Fatalf("Run(edit x) = %d, want 1; stderr: %q", code, stderr)
	}
	if !strings.Contains(stderr, "тикет «x» не найден") {
		t.Errorf("stderr = %q, want not-found report for «x»", stderr)
	}
}

// TestEditAmbiguous pins the shared T-0157 report: exit 2, the localized
// header plus every conflicting path, no mutation.
func TestEditAmbiguous(t *testing.T) {
	// mustEval mirrors paths.Resolve's EvalSymlinks: Windows CI temp dirs
	// may carry 8.3 short components (RUNNER~1) that Resolve expands.
	dir := mustEval(t, t.TempDir())
	createTestTicket(t, dir)
	writeJSONTicket(t, dir, "T-0001-done.md", 1, "done", "Duplicate", "tickets", "2026-09-08 12:00")
	openPath := filepath.Join(dir, "T-0001-open.md")
	donePath := filepath.Join(dir, "T-0001-done.md")
	beforeOpen, _ := os.ReadFile(openPath)
	beforeDone, _ := os.ReadFile(donePath)

	_, stderr, code := editRun(t, dir, "edit", "1", "--title", "x")
	if code != 2 {
		t.Fatalf("Run(edit 1) = %d, want 2; stderr: %q", code, stderr)
	}
	if !strings.Contains(stderr, "номер тикета 1 неоднозначен") ||
		!strings.Contains(stderr, openPath) || !strings.Contains(stderr, donePath) {
		t.Errorf("stderr = %q, want ambiguous header plus both paths (open=%q done=%q)", stderr, openPath, donePath)
	}
	afterOpen, _ := os.ReadFile(openPath)
	afterDone, _ := os.ReadFile(donePath)
	if string(afterOpen) != string(beforeOpen) || string(afterDone) != string(beforeDone) {
		t.Errorf("ambiguous edit mutated files")
	}
}
