package cli_test

// T-0035 integration: the user-remarks section ("## Комментарии от
// пользователя", stub placeholder) and the free-form "## Комментарии"
// section survive set cycles and are injected by set/archive on legacy
// tickets.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// userCommentsPlaceholder mirrors the domain userCommentsStub literal:
// unexported there, so the byte-exact contract is restated here (kept in
// sync by the pinned template/golden tests).
const userCommentsPlaceholder = `_Замечания пользователя: пишите сюда — агент прочитает эту секцию перед работой над тикетом. Агент сюда не пишет._`

// seedTicket writes body as the named ticket file inside the tickets dir.
func seedTicket(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("seed %s: %v", name, err)
	}
}

// legacyBody builds a bash-style legacy ticket (number 1, the given
// status) without comment sections; comments, when non-empty, is spliced
// verbatim before "## Журнал". A done status carries the transition
// journal entry like real bash-created tickets.
func legacyBody(status, comments string) string {
	body := "# T-0001 · BUG: легаси\n\n" +
		"- Статус: " + status + "\n" +
		"- Приоритет: normal\n" +
		"- Создан: 2026-01-01 10:00 · кем: erdmitry\n" +
		"- Проект: tickets\n\n" +
		"## Кратко\nлегаси\n\n" +
		"## Подробности\nтекст легаси\n\n" +
		comments +
		"## Журнал\n" +
		"- 2026-01-01 10:00 — тикет создан (erdmitry).\n"
	if status == "done" {
		body += "- 2026-01-01 11:00 — статус: open → done (agent)\n"
	}
	return body
}

// assertCommentsIntact fails unless body holds the user section with the
// remark, the bare free-form header, and no stub residue.
func assertCommentsIntact(t *testing.T, body []byte, remark, label string) {
	t.Helper()
	if !strings.Contains(string(body), "## Комментарии от пользователя\n"+remark+"\n") {
		t.Fatalf("%s: user section or remark lost:\n%s", label, body)
	}
	if !strings.Contains(string(body), "## Комментарии\n") {
		t.Fatalf("%s: free comments header missing:\n%s", label, body)
	}
	if strings.Contains(string(body), userCommentsPlaceholder) {
		t.Fatalf("%s: placeholder resurrected:\n%s", label, body)
	}
}

// TestUserCommentsSurviveSetCycle runs: new → stub under the user section
// → a user remark replaces the stub in place → set wip → set done → the
// remark survives with no stub residue, and `show` prints the done file
// bytes. Everything happens in a TempDir sandbox.
func TestUserCommentsSurviveSetCycle(t *testing.T) {
	tickets := t.TempDir()
	cwd := t.TempDir()
	out, stderr, code := runBin(t, ticketBin, cwd, tickets, "new", "Комментарии цикл", "-d", "детали цикла")
	if code != 0 {
		t.Fatalf("new: code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(out, "T-0001-open.md") {
		t.Fatalf("new stdout %q does not mention the created file", out)
	}
	path := filepath.Join(mustEval(t, tickets), "T-0001-open.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read created ticket: %v", err)
	}
	if !strings.Contains(string(data), "## Комментарии от пользователя\n"+userCommentsPlaceholder+"\n\n## Комментарии\n\n## Журнал\n") {
		t.Fatalf("new ticket missing the stubbed user section before the free one:\n%s", data)
	}
	// The user writes a remark in place of the stub inside the user section.
	const remark = "Пользователь: сначала проверь регрессию в модуле X"
	edited := strings.Replace(string(data), userCommentsPlaceholder, remark, 1)
	if edited == string(data) {
		t.Fatalf("stub not found for replacement:\n%s", data)
	}
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatalf("write user remark: %v", err)
	}

	_, stderr, code = runBin(t, ticketBin, cwd, tickets, "set", "1", "wip", "в работу")
	if code != 0 {
		t.Fatalf("set wip: code=%d stderr=%q", code, stderr)
	}
	wipData, err := os.ReadFile(filepath.Join(mustEval(t, tickets), "T-0001-wip.md"))
	if err != nil {
		t.Fatalf("read wip ticket: %v", err)
	}
	assertCommentsIntact(t, wipData, remark, "wip")

	_, stderr, code = runBin(t, ticketBin, cwd, tickets, "set", "1", "done", "готово")
	if code != 0 {
		t.Fatalf("set done: code=%d stderr=%q", code, stderr)
	}
	doneData, err := os.ReadFile(filepath.Join(mustEval(t, tickets), "T-0001-done.md"))
	if err != nil {
		t.Fatalf("read done ticket: %v", err)
	}
	assertCommentsIntact(t, doneData, remark, "done")

	showOut, stderr, code := runBin(t, ticketBin, cwd, tickets, "show", "1")
	if code != 0 {
		t.Fatalf("show: code=%d stderr=%q", code, stderr)
	}
	if showOut != string(doneData) {
		t.Fatalf("show output differs from the done file bytes")
	}
}

// assertFreeCommentsIntact fails unless body keeps the free-form text
// verbatim under "## Комментарии" and exactly one user stub.
func assertFreeCommentsIntact(t *testing.T, body []byte, freeText, label string) {
	t.Helper()
	if !strings.Contains(string(body), "## Комментарии\n"+freeText+"\n") {
		t.Fatalf("%s: free comments text lost:\n%s", label, body)
	}
	if n := strings.Count(string(body), userCommentsPlaceholder); n != 1 {
		t.Fatalf("%s: user stub count = %d, want 1:\n%s", label, n, body)
	}
}

// TestFreeCommentsSurviveSetCycle: free-form text written directly into
// the bare "## Комментарии" section of a seeded open ticket survives
// set wip → set done; the user stub is not duplicated and `show` matches
// the done file bytes.
func TestFreeCommentsSurviveSetCycle(t *testing.T) {
	tickets := t.TempDir()
	cwd := t.TempDir()
	const freeText = "заметки агента: воспроизведение в песочнице"
	seedTicket(t, tickets, "T-0001-open.md", legacyBody("open",
		"## Комментарии от пользователя\n"+userCommentsPlaceholder+"\n\n## Комментарии\n"+freeText+"\n\n"))

	_, stderr, code := runBin(t, ticketBin, cwd, tickets, "set", "1", "wip", "в работу")
	if code != 0 {
		t.Fatalf("set wip: code=%d stderr=%q", code, stderr)
	}
	wipData, err := os.ReadFile(filepath.Join(mustEval(t, tickets), "T-0001-wip.md"))
	if err != nil {
		t.Fatalf("read wip ticket: %v", err)
	}
	assertFreeCommentsIntact(t, wipData, freeText, "wip")

	_, stderr, code = runBin(t, ticketBin, cwd, tickets, "set", "1", "done", "готово")
	if code != 0 {
		t.Fatalf("set done: code=%d stderr=%q", code, stderr)
	}
	doneData, err := os.ReadFile(filepath.Join(mustEval(t, tickets), "T-0001-done.md"))
	if err != nil {
		t.Fatalf("read done ticket: %v", err)
	}
	assertFreeCommentsIntact(t, doneData, freeText, "done")

	showOut, stderr, code := runBin(t, ticketBin, cwd, tickets, "show", "1")
	if code != 0 {
		t.Fatalf("show: code=%d stderr=%q", code, stderr)
	}
	if showOut != string(doneData) {
		t.Fatalf("show output differs from the done file bytes")
	}
}

// TestSetInjectsUserSectionIntoLegacyWithComments: a bash-style legacy
// ticket with text in the bare "## Комментарии" section gains the stubbed
// "## Комментарии от пользователя" BEFORE the free section on the first
// set; the free text stays verbatim and `show` matches the file bytes.
func TestSetInjectsUserSectionIntoLegacyWithComments(t *testing.T) {
	tickets := t.TempDir()
	cwd := t.TempDir()
	const freeText = "заметки агента"
	seedTicket(t, tickets, "T-0001-open.md", legacyBody("open", "## Комментарии\n"+freeText+"\n\n"))

	_, stderr, code := runBin(t, ticketBin, cwd, tickets, "set", "1", "wip", "миграция")
	if code != 0 {
		t.Fatalf("set wip: code=%d stderr=%q", code, stderr)
	}
	wipData, err := os.ReadFile(filepath.Join(mustEval(t, tickets), "T-0001-wip.md"))
	if err != nil {
		t.Fatalf("read wip ticket: %v", err)
	}
	body := string(wipData)
	userIdx := strings.Index(body, "## Комментарии от пользователя\n"+userCommentsPlaceholder+"\n")
	freeIdx := strings.Index(body, "## Комментарии\n"+freeText+"\n")
	if userIdx < 0 || freeIdx < 0 {
		t.Fatalf("wip ticket missing the injected user section or the free text:\n%s", body)
	}
	if userIdx > freeIdx {
		t.Fatalf("user section must precede the free one:\n%s", body)
	}
	if n := strings.Count(body, userCommentsPlaceholder); n != 1 {
		t.Fatalf("stub duplicated (count=%d):\n%s", n, body)
	}
	showOut, stderr, code := runBin(t, ticketBin, cwd, tickets, "show", "1")
	if code != 0 {
		t.Fatalf("show: code=%d stderr=%q", code, stderr)
	}
	if showOut != body {
		t.Fatalf("show output differs from the wip file bytes")
	}
}

// TestArchiveInjectsBothSections: a legacy done ticket without any comment
// sections, archived by `archive`, gains BOTH sections — the stubbed user
// one before the bare free one — while the original body is preserved.
func TestArchiveInjectsBothSections(t *testing.T) {
	tickets := t.TempDir()
	seedTicket(t, tickets, "T-0001-done.md", legacyBody("done", ""))
	out, stderr, code := runBin(t, ticketBin, t.TempDir(), tickets, "archive")
	if code != 0 {
		t.Fatalf("archive: code=%d stderr=%q", code, stderr)
	}
	wantPath := filepath.Join(mustEval(t, tickets), "archive", "T-0001-done.md")
	if !strings.Contains(out, wantPath) {
		t.Fatalf("archive stdout %q does not mention %q", out, wantPath)
	}
	archived, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read archived ticket: %v", err)
	}
	body := string(archived)
	userIdx := strings.Index(body, "## Комментарии от пользователя\n"+userCommentsPlaceholder+"\n")
	freeIdx := strings.Index(body, "## Комментарии\n\n## Журнал\n")
	if userIdx < 0 || freeIdx < 0 {
		t.Fatalf("archived ticket missing the injected sections:\n%s", archived)
	}
	if userIdx > freeIdx {
		t.Fatalf("user section must precede the free one:\n%s", archived)
	}
	// The original body is preserved.
	for _, want := range []string{"текст легаси", "— тикет создан (erdmitry).", "— статус: open → done (agent)", "— перенесён в архив"} {
		if !strings.Contains(body, want) {
			t.Fatalf("archived ticket lost %q:\n%s", want, archived)
		}
	}
}
