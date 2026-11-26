package cli_test

// T-0241 A5: table-driven tests for the show --json mode — the frozen byte
// contract, the zero-timestamp policy, structured journal records, the two
// warning and unknown-tail policies, and every refusal of the mode. Everything
// runs in a TICKETS_DIR sandbox (t.TempDir).

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// showJSONStandard is the byte-exact expected document of the standard fixture
// written by newShowSandbox (T-0001-open.md, showTicketSrc).
const showJSONStandard = `{"number":1,"status":"open","type":"BUG","priority":"normal","project":"tickets","title":"x","summary":"summary body","created":"2026-09-08T12:00:00Z","who":"tester","details":"details body","user_comments":"user comments body","comments":"comments body","journal":[{"at":"2026-09-08T12:00:00Z","from":"open","to":"open","who":"tester","comment":"","archived":false}],"unknown":"","unknown_encoding":"utf-8","warnings":[]}` + "\n"

// TestShowJSON_Contract pins the frozen document of the standard fixture: key
// order, the empty journal/unknown/warnings policies, and exactly one trailing
// LF.
func TestShowJSON_Contract(t *testing.T) {
	dir := newShowSandbox(t)
	code, out, errOut := runShow([]string{"show", "1", "--json"}, dir)
	if code != 0 {
		t.Fatalf("show --json = %d, stderr: %s", code, errOut)
	}
	if out != showJSONStandard {
		t.Errorf("stdout mismatch:\n got: %q\nwant: %q", out, showJSONStandard)
	}
	if strings.Count(out, "\n") != 1 || !strings.HasSuffix(out, "\n") {
		t.Errorf("document must carry exactly one trailing LF, got %q", out)
	}
}

// TestShowJSONHTMLEscaping pins the default encoding/json HTML escaping (the
// same policy as list --json): <, > and & never reach stdout unescaped.
func TestShowJSONHTMLEscaping(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tickets")
	writeShowFile(t, dir, "T-0001-open.md", "# T-0001 · BUG: a<b>&c\n\n- Status (Статус): open\n- Priority (Приоритет): normal\n"+
		"- Created (Создан): 2026-09-08 12:00 · by (кем): tester\n- Project (Проект): tickets\n\n## Summary (Кратко)\n\ns\n")
	code, out, errOut := runShow([]string{"show", "1", "--json"}, dir)
	if code != 0 {
		t.Fatalf("show --json = %d, stderr: %s", code, errOut)
	}
	want := `"title":"a\u003cb\u003e\u0026c"`
	if !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want it to contain %s", out, want)
	}
}

// TestShowJSONZeroTimestamps pins the documented policy: an unparsable created
// value and an unparsable journal timestamp both render as "" — never as a
// fabricated instant.
func TestShowJSONZeroTimestamps(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tickets")
	src := "# T-0002 · BUG: x\n\n- Status (Статус): open\n- Priority (Приоритет): normal\n" +
		"- Created (Создан): garbage · by (кем): tester\n- Project (Проект): tickets\n\n" +
		"## Summary (Кратко)\n\ns\n\n## Journal (Журнал)\n\n- garbage — ticket created (tester).\n"
	writeShowFile(t, dir, "T-0002-open.md", src)
	code, out, errOut := runShow([]string{"show", "2", "--json"}, dir)
	if code != 0 {
		t.Fatalf("show --json = %d, stderr: %s", code, errOut)
	}
	doc := decodeShowJSON(t, out)
	if doc["created"] != "" {
		t.Errorf("created = %v, want \"\"", doc["created"])
	}
	entries, ok := doc["journal"].([]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("journal = %v, want one entry", doc["journal"])
	}
	if at := entries[0].(map[string]any)["at"]; at != "" {
		t.Errorf("journal[0].at = %v, want \"\"", at)
	}
}

// TestShowJSONJournalEntries pins the structured journal: creation, transition
// and archive records become objects with their own fields, and the raw
// markdown line is NOT exposed.
func TestShowJSONJournalEntries(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tickets")
	src := "# T-0003 · BUG: x\n\n- Status (Статус): done\n- Priority (Приоритет): normal\n" +
		"- Created (Создан): 2026-09-08 12:00 · by (кем): tester\n- Project (Проект): tickets\n\n" +
		"## Summary (Кратко)\n\ns\n\n## Journal (Журнал)\n\n" +
		"- 2026-09-08 12:00 — ticket created (tester).\n\n" +
		"- 2026-09-08 13:00 — status: open → wip · took it (tester)\n\n" +
		"- 2026-09-08 14:00 — moved to archive (tester)\n"
	writeShowFile(t, dir, "T-0003-done.md", src)
	code, out, errOut := runShow([]string{"show", "3", "--json"}, dir)
	if code != 0 {
		t.Fatalf("show --json = %d, stderr: %s", code, errOut)
	}
	want := `[{"at":"2026-09-08T12:00:00Z","from":"open","to":"open","who":"tester","comment":"","archived":false},` +
		`{"at":"2026-09-08T13:00:00Z","from":"open","to":"wip","who":"tester","comment":"took it","archived":false},` +
		`{"at":"2026-09-08T14:00:00Z","from":"","to":"","who":"tester","comment":"","archived":true}]`
	if !strings.Contains(out, `"journal":`+want) {
		t.Errorf("stdout = %q, want journal %s", out, want)
	}
	if strings.Contains(out, "moved to archive") {
		t.Errorf("the raw journal line must not be exposed, got %q", out)
	}
}

// TestShowJSONWarnings pins the warning policy: a ticket whose duplicate
// Journal section was merged reports one "<name>: <err>" line inside the
// document, and stdout stays pure JSON.
func TestShowJSONWarnings(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tickets")
	src := "# T-0001 · BUG: x\n\n- Status (Статус): open\n- Priority (Приоритет): normal\n" +
		"- Created (Создан): 2026-09-08 12:00 · by (кем): tester\n- Project (Проект): tickets\n\n" +
		"## Summary (Кратко)\n\ns\n\n## Journal (Журнал)\n\n- 2026-09-08 12:00 — ticket created (tester).\n\n" +
		"## Journal (Журнал)\n\n- 2026-09-08 13:00 — moved to archive (tester)\n"
	writeShowFile(t, dir, "T-0001-open.md", src)
	code, out, errOut := runShow([]string{"show", "1", "--json"}, dir)
	if code != 0 {
		t.Fatalf("show --json = %d, stderr: %s", code, errOut)
	}
	doc := decodeShowJSON(t, out)
	warns, ok := doc["warnings"].([]any)
	if !ok || len(warns) != 1 {
		t.Fatalf("warnings = %v, want exactly one", doc["warnings"])
	}
	want := fmt.Sprintf("T-0001-open.md: %v", domain.ErrJournalDup)
	if warns[0] != want {
		t.Errorf("warnings[0] = %v, want %q", warns[0], want)
	}
	if len(doc["journal"].([]any)) != 2 {
		t.Errorf("journal = %v, want both entries", doc["journal"])
	}
}

// TestShowJSONUnknownTail pins the lossless unknown policy: a UTF-8 tail is
// emitted as text, an invalid-UTF-8 tail as base64 — never mangled into U+FFFD.
func TestShowJSONUnknownTail(t *testing.T) {
	utf8Tail := "\nmanual tail line\n"
	base := "# T-0001 · BUG: x\n\n- Status (Статус): open\n- Priority (Приоритет): normal\n" +
		"- Created (Создан): 2026-09-08 12:00 · by (кем): tester\n- Project (Проект): tickets\n\n" +
		"## Summary (Кратко)\n\ns\n\n## Journal (Журнал)\n\n- 2026-09-08 12:00 — ticket created (tester).\n"
	cases := []struct {
		name     string
		tail     string
		wantText string
		wantEnc  string
	}{
		{"utf-8 tail", utf8Tail, utf8Tail, "utf-8"},
		{"invalid utf-8 tail", "\nbad \xff\xfe bytes\n", "CmJhZCD//iBieXRlcwo=", "base64"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "tickets")
			writeShowFile(t, dir, "T-0001-open.md", base+tc.tail)
			code, out, errOut := runShow([]string{"show", "1", "--json"}, dir)
			if code != 0 {
				t.Fatalf("show --json = %d, stderr: %s", code, errOut)
			}
			doc := decodeShowJSON(t, out)
			if doc["unknown_encoding"] != tc.wantEnc {
				t.Errorf("unknown_encoding = %v, want %q", doc["unknown_encoding"], tc.wantEnc)
			}
			if doc["unknown"] != tc.wantText {
				t.Errorf("unknown = %v, want %q", doc["unknown"], tc.wantText)
			}
		})
	}
}

// TestShowJSONFileNameStatus pins the authority of the file name: a forged
// "- Статус:" line in the body never reaches the document.
func TestShowJSONFileNameStatus(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tickets")
	src := "# T-0004 · BUG: x\n\n- Status (Статус): fake\n- Priority (Приоритет): normal\n" +
		"- Created (Создан): 2026-09-08 12:00 · by (кем): tester\n- Project (Проект): tickets\n\n" +
		"## Summary (Кратко)\n\ns\n"
	writeShowFile(t, dir, "T-0004-open.md", src)
	code, out, errOut := runShow([]string{"show", "4", "--json"}, dir)
	if code != 0 {
		t.Fatalf("show --json = %d, stderr: %s", code, errOut)
	}
	if doc := decodeShowJSON(t, out); doc["status"] != "open" {
		t.Errorf("status = %v, want \"open\" (from the file name)", doc["status"])
	}
}

// TestShowJSONRefusals pins the refusals and the plain-mode compatibility rule
// of the --json token.
func TestShowJSONRefusals(t *testing.T) {
	dir := newShowSandbox(t)
	refusals := []struct {
		name       string
		args       []string
		wantErrSub string
	}{
		{"repeated flag", []string{"show", "1", "--json", "--json"}, "--json"},
		{"glued flag with a mode token", []string{"show", "1", "--json=x", "--json"}, "--json=x"},
		{"no ticket argument", []string{"show", "--json"}, ""},
		{"extra positional", []string{"show", "1", "extra", "--json"}, "extra"},
	}
	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			code, out, errOut := runShow(tc.args, dir)
			if code != 1 {
				t.Fatalf("code = %d, want 1 (stderr: %s)", code, errOut)
			}
			if out != "" {
				t.Errorf("stdout = %q, want empty", out)
			}
			if !strings.Contains(errOut, tc.wantErrSub) {
				t.Errorf("stderr = %q, want it to contain %q", errOut, tc.wantErrSub)
			}
		})
	}
	t.Run("plain mode keeps its warning", func(t *testing.T) {
		code, out, errOut := runShow([]string{"show", "1", "--json=x"}, dir)
		if code != 0 {
			t.Fatalf("plain show = %d, want 0 (stderr: %s)", code, errOut)
		}
		if !strings.Contains(out, "## Summary") {
			t.Errorf("plain show must print the whole file, got %q", out)
		}
		if !strings.Contains(errOut, "--json=x") {
			t.Errorf("stderr = %q, want the extra-args warning naming the argument", errOut)
		}
	})
}

// TestShowJSONAcrossDirectories pins the shared lookup rules: archive/ and
// deleted/ tickets are readable in the new mode, and an exact file name still
// resolves its own file.
func TestShowJSONAcrossDirectories(t *testing.T) {
	dir := newShowSandbox(t)
	writeShowFile(t, dir, "archive/T-0007-done.md", showTicketSrc(7, "done", "## Summary", "## Details", "", ""))
	writeShowFile(t, dir, "deleted/T-0008-deleted.md", showTicketSrc(8, "deleted", "## Summary", "## Details", "", ""))
	cases := []struct {
		name string
		args []string
		num  string
	}{
		{"archive number", []string{"show", "7", "--json"}, "7"},
		{"deleted number", []string{"show", "8", "--json"}, "8"},
		{"exact file name", []string{"show", "T-0001-open.md", "--json"}, "1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out, errOut := runShow(tc.args, dir)
			if code != 0 {
				t.Fatalf("code = %d, stderr: %s", code, errOut)
			}
			want := fmt.Sprintf(`"number":%s,`, tc.num)
			if !strings.Contains(out, want) {
				t.Errorf("stdout = %q, want it to contain %s", out, want)
			}
		})
	}
}

// decodeShowJSON decodes one show --json document or fails the test.
func decodeShowJSON(t *testing.T, out string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("json.Unmarshal: %v (stdout %q)", err, out)
	}
	return doc
}
