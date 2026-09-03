package cli_test

// T-0032 integration: the "## Комментарии" section and user text in it
// survive set cycles and are injected by archive on legacy tickets.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// commentsPlaceholder mirrors the domain commentsStub literal: unexported
// there, so the byte-exact contract is restated here (kept in sync by the
// pinned template/golden tests).
const commentsPlaceholder = `_Замечания пользователя: пишите сюда — агент прочитает эту секцию перед работой над тикетом._`

// TestCommentsSurviveSetCycle runs (i): new → placeholder → user text →
// set wip → set done → the section and the text are intact, and `show`
// prints the section. Everything happens in a TempDir sandbox.
func TestCommentsSurviveSetCycle(t *testing.T) {
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
	if !strings.Contains(string(data), "## Комментарии\n"+commentsPlaceholder+"\n") {
		t.Fatalf("placeholder not directly under the section header:\n%s", data)
	}
	// The user writes remarks in place of the placeholder.
	const remark = "Пользователь: сначала проверь регрессию в модуле X"
	edited := strings.Replace(string(data), commentsPlaceholder, remark, 1)
	if edited == string(data) {
		t.Fatalf("placeholder not found for replacement:\n%s", data)
	}
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatalf("write user remark: %v", err)
	}

	out, stderr, code = runBin(t, ticketBin, cwd, tickets, "set", "1", "wip", "в работу")
	if code != 0 {
		t.Fatalf("set wip: code=%d stderr=%q", code, stderr)
	}
	wipPath := filepath.Join(mustEval(t, tickets), "T-0001-wip.md")
	wipData, err := os.ReadFile(wipPath)
	if err != nil {
		t.Fatalf("read wip ticket: %v", err)
	}
	assertCommentsIntact(t, wipData, remark, "wip")

	out, stderr, code = runBin(t, ticketBin, cwd, tickets, "set", "1", "done", "готово")
	if code != 0 {
		t.Fatalf("set done: code=%d stderr=%q", code, stderr)
	}
	donePath := filepath.Join(mustEval(t, tickets), "T-0001-done.md")
	doneData, err := os.ReadFile(donePath)
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

// assertCommentsIntact fails unless body holds the section with the user
// remark and no placeholder residue.
func assertCommentsIntact(t *testing.T, body []byte, remark, label string) {
	t.Helper()
	if !strings.Contains(string(body), "## Комментарии\n"+remark+"\n") {
		t.Fatalf("%s: section or user remark lost:\n%s", label, body)
	}
	if strings.Contains(string(body), commentsPlaceholder) {
		t.Fatalf("%s: placeholder resurrected:\n%s", label, body)
	}
}

// TestArchiveInjectsCommentsSection runs (j): a legacy done ticket without
// the section, archived by `archive`, gains "## Комментарии" with the
// placeholder while the original body is preserved.
func TestArchiveInjectsCommentsSection(t *testing.T) {
	tickets := t.TempDir()
	legacy := "# T-0001 · BUG: легаси\n\n" +
		"- Статус: done\n" +
		"- Приоритет: normal\n" +
		"- Создан: 2026-01-01 10:00 · кем: erdmitry\n" +
		"- Проект: tickets\n\n" +
		"## Кратко\nлегаси\n\n" +
		"## Подробности\nтекст легаси\n\n" +
		"## Журнал\n" +
		"- 2026-01-01 10:00 — тикет создан (erdmitry).\n" +
		"- 2026-01-01 11:00 — статус: open → done (agent)\n"
	if err := os.WriteFile(filepath.Join(tickets, "T-0001-done.md"), []byte(legacy), 0o644); err != nil {
		t.Fatalf("seed legacy ticket: %v", err)
	}
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
	// The section is injected with the placeholder...
	if !strings.Contains(string(archived), "## Комментарии\n"+commentsPlaceholder+"\n\n## Журнал\n") {
		t.Fatalf("archived ticket missing the injected section:\n%s", archived)
	}
	// ...and the original body is preserved.
	for _, want := range []string{"текст легаси", "— тикет создан (erdmitry).", "— статус: open → done (agent)", "— перенесён в архив"} {
		if !strings.Contains(string(archived), want) {
			t.Fatalf("archived ticket lost %q:\n%s", want, archived)
		}
	}
}
