package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"ticket/internal/cli"
)

// newJSONTicketsDir creates a fresh temp TICKETS_DIR sandbox.
func newJSONTicketsDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "tickets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// writeJSONTicket writes a valid ticket file name into dir with the given
// fields (v2 markdown format). Pass created="garbage" to exercise the
// zero-timestamp fallback of markdown.parseTS.
func writeJSONTicket(t *testing.T, dir, name string, num int, status, title, project, created string) {
	t.Helper()
	body := fmt.Sprintf(`# T-%04d · BUG: %s

- Status (Статус): %s
- Priority (Приоритет): normal
- Created (Создан): %s · by (кем): tester
- Project (Проект): %s

## Summary (Кратко)
%s

## Details (Подробности)
x

## User comments (Комментарии от пользователя)

## Comments (Комментарии)

## Journal (Журнал)
- 2026-09-08 12:00 — ticket created (tester).
`, num, title, status, created, project, title)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// runJSONList runs cli.Run on args with TICKETS_DIR=dir and returns the
// exit code plus stdout/stderr as strings.
func runJSONList(args []string, dir string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := cli.Run(args, map[string]string{"TICKETS_DIR": dir}, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// jsonNumbers decodes a list --json stdout array and returns the ticket
// numbers in output order.
func jsonNumbers(t *testing.T, out string) []int {
	t.Helper()
	var arr []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	nums := make([]int, len(arr))
	for i, m := range arr {
		n, ok := m["number"].(float64)
		if !ok {
			t.Fatalf("ticket %d: number missing or not a number: %T", i, m["number"])
		}
		nums[i] = int(n)
	}
	return nums
}

// TestListJSON_Contract verifies the frozen v1 JSON schema: exactly 6
// fields in order number,status,type,priority,title,created (raw string
// check, NOT a map); RFC3339 created; default HTML escaping of <>&;
// exactly one trailing newline; no extra fields.
func TestListJSON_Contract(t *testing.T) {
	dir := newJSONTicketsDir(t)
	writeJSONTicket(t, dir, "T-0001-open.md", 1, "open", `test <script>&"alert"</script>`, "tickets", "2026-09-08 12:00")

	code, out, errOut := runJSONList([]string{"list", "--json"}, dir)
	if code != 0 {
		t.Fatalf("list --json = %d, stderr: %s", code, errOut)
	}
	// Byte-exact contract (writeJSONTicket emits type=BUG, priority=normal,
	// created parsed as UTC): key order number,status,type,priority,title,
	// created; default HTML escaping of <>& (\u003c \u003e \u0026, SetEscapeHTML
	// untouched); escaped quotes in title; exactly one trailing newline
	// (json.Encoder.Encode).
	want := `[{"number":1,"status":"open","type":"BUG","priority":"normal","title":"test \u003cscript\u003e\u0026\"alert\"\u003c/script\u003e","created":"2026-09-08T12:00:00Z"}]` + "\n"
	if out != want {
		t.Errorf("stdout mismatch:\n got: %q\nwant: %q", out, want)
	}
	// Structure: exactly 6 keys, no extras.
	var arr []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("expected 1 ticket, got %d", len(arr))
	}
	if len(arr[0]) != 6 {
		t.Errorf("expected exactly 6 fields, got %d: %v", len(arr[0]), arr[0])
	}
	for _, k := range []string{"number", "status", "type", "priority", "title", "created"} {
		if _, ok := arr[0][k]; !ok {
			t.Errorf("missing field %q", k)
		}
	}
	for _, k := range []string{"details", "who", "project", "comments", "lang"} {
		if _, ok := arr[0][k]; ok {
			t.Errorf("unexpected field %q present", k)
		}
	}
	created, ok := arr[0]["created"].(string)
	if !ok {
		t.Fatalf("created is not a string: %T", arr[0]["created"])
	}
	if _, err := time.Parse(time.RFC3339, created); err != nil {
		t.Errorf("created not RFC3339: %q (%v)", created, err)
	}
}

// TestListJSON_ZeroCreated pins the sentinel: a garbage timestamp parses to
// the zero time and renders as "0001-01-01T00:00:00Z" (type-stable RFC3339,
// not null/omitted).
func TestListJSON_ZeroCreated(t *testing.T) {
	dir := newJSONTicketsDir(t)
	writeJSONTicket(t, dir, "T-0002-open.md", 2, "open", "zero", "tickets", "garbage")

	code, out, errOut := runJSONList([]string{"list", "--json"}, dir)
	if code != 0 {
		t.Fatalf("list --json = %d, stderr: %s", code, errOut)
	}
	var arr []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("expected 1 ticket, got %d", len(arr))
	}
	if got := arr[0]["created"]; got != "0001-01-01T00:00:00Z" {
		t.Errorf("zero created = %v, want 0001-01-01T00:00:00Z", got)
	}
}

// TestListJSON_Filters covers status filters in --json mode: open/wip/done/
// closed, active (=open+wip), the bare default (active) and the
// flag-before-filter position.
func TestListJSON_Filters(t *testing.T) {
	dir := newJSONTicketsDir(t)
	for i, st := range []string{"open", "wip", "done", "closed"} {
		writeJSONTicket(t, dir, fmt.Sprintf("T-%04d-%s.md", i+1, st), i+1, st, st, "tickets", "2026-09-08 12:00")
	}
	tests := []struct {
		args []string
		want []int
	}{
		{[]string{"list", "--json", "open"}, []int{1}},
		{[]string{"list", "--json", "wip"}, []int{2}},
		{[]string{"list", "--json", "done"}, []int{3}},
		{[]string{"list", "--json", "closed"}, []int{4}},
		{[]string{"list", "active", "--json"}, []int{1, 2}},
		{[]string{"list", "--json"}, []int{1, 2}},
		{[]string{"list", "all", "--json"}, []int{1, 2, 3, 4}},
	}
	for _, tt := range tests {
		code, out, errOut := runJSONList(tt.args, dir)
		if code != 0 {
			t.Errorf("%v: exit %d, stderr: %s", tt.args, code, errOut)
			continue
		}
		if got := jsonNumbers(t, out); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%v: numbers = %v, want %v", tt.args, got, tt.want)
		}
	}
}

// TestListJSON_Sorting pins Number ASC ordering (store scan sort) in JSON
// mode: files created as 9, 10, 1 must list as 1, 9, 10.
func TestListJSON_Sorting(t *testing.T) {
	dir := newJSONTicketsDir(t)
	for _, tc := range []struct {
		num   int
		title string
	}{{9, "nine"}, {10, "ten"}, {1, "one"}} {
		writeJSONTicket(t, dir, fmt.Sprintf("T-%04d-open.md", tc.num), tc.num, "open", tc.title, "tickets", "2026-09-08 12:00")
	}
	code, out, errOut := runJSONList([]string{"list", "--json"}, dir)
	if code != 0 {
		t.Fatalf("list --json = %d, stderr: %s", code, errOut)
	}
	if got := jsonNumbers(t, out); !reflect.DeepEqual(got, []int{1, 9, 10}) {
		t.Errorf("numbers = %v, want [1 9 10]", got)
	}
}

// TestListJSON_Archive verifies that the archive filter finds archived
// tickets in --json mode.
func TestListJSON_Archive(t *testing.T) {
	dir := newJSONTicketsDir(t)
	archiveDir := filepath.Join(dir, "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSONTicket(t, archiveDir, "T-0005-done.md", 5, "done", "archived", "tickets", "2026-09-08 12:00")

	code, out, errOut := runJSONList([]string{"list", "archive", "--json"}, dir)
	if code != 0 {
		t.Fatalf("list archive --json = %d, stderr: %s", code, errOut)
	}
	if got := jsonNumbers(t, out); !reflect.DeepEqual(got, []int{5}) {
		t.Errorf("numbers = %v, want [5]", got)
	}
}

// TestListJSON_ProjectFilter verifies that -P filters by exact project
// match in --json mode.
func TestListJSON_ProjectFilter(t *testing.T) {
	dir := newJSONTicketsDir(t)
	for i, proj := range []string{"foo", "bar"} {
		writeJSONTicket(t, dir, fmt.Sprintf("T-%04d-open.md", i+1), i+1, "open", "test", proj, "2026-09-08 12:00")
	}
	code, out, errOut := runJSONList([]string{"list", "-P", "foo", "--json"}, dir)
	if code != 0 {
		t.Fatalf("list -P foo --json = %d, stderr: %s", code, errOut)
	}
	if got := jsonNumbers(t, out); !reflect.DeepEqual(got, []int{1}) {
		t.Errorf("numbers = %v, want [1]", got)
	}
}
