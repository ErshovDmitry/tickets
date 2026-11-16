package cli_test

// T-0187 F10: post-edit `list --json` contract (the edited fields are
// reflected in the frozen 6-field schema) and the canonical MD022/MD032
// layout of the edited ticket file — blank lines around every "## "
// heading and around the journal list. The external markdownlint smoke
// run stays a coordinator-level check; these unit tests encode the same
// rules in-process so the suite has no dependency on an installed tool.

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"
)

// createdLineRe captures the naive "YYYY-MM-DD HH:MM" timestamp of the
// file metadata Created line; parseTS reads it as UTC (list JSON contract).
var createdLineRe = regexp.MustCompile(`- Created \([^)]*\): (\d{4}-\d{2}-\d{2})[ T](\d{2}:\d{2})`)

// TestEditListJSONReflectsChanges pins F10: after a successful multi-field
// edit, `list --json` reports the NEW values under exactly the frozen
// 6-field schema (number,status,type,priority,title,created) and the
// created timestamp renders the same instant as the file metadata.
func TestEditListJSONReflectsChanges(t *testing.T) {
	dir := t.TempDir()
	createTestTicket(t, dir)
	_, stderr, code := editRun(t, dir,
		"edit", "1", "--title", "Новый заголовок", "--type", "OPS", "--priority", "high", "--project", "beta")
	if code != 0 {
		t.Fatalf("Run(edit) = %d, want 0; stderr: %q", code, stderr)
	}

	stdout, stderr, code := editRun(t, dir, "list", "--json")
	if code != 0 {
		t.Fatalf("Run(list --json) = %d, want 0; stderr: %q", code, stderr)
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(stdout), &arr); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("list --json returned %d tickets, want 1", len(arr))
	}
	got := arr[0]
	for k, v := range map[string]any{
		"number":   float64(1),
		"status":   "open",
		"type":     "OPS",
		"priority": "high",
		"title":    "Новый заголовок",
	} {
		if got[k] != v {
			t.Errorf("json field %q = %v, want %v", k, got[k], v)
		}
	}
	if len(got) != 6 {
		t.Errorf("json object has %d fields, want the frozen 6: %v", len(got), got)
	}
	created, ok := got["created"].(string)
	if !ok || created == "" {
		t.Fatalf("json created missing or not a string: %v", got)
	}
	if _, err := time.Parse(time.RFC3339, created); err != nil {
		t.Errorf("json created %q is not RFC3339: %v", created, err)
	}
	// The file metadata and the JSON must render the same instant.
	m := createdLineRe.FindStringSubmatch(readTicket(t, dir))
	if m == nil {
		t.Fatalf("file metadata has no Created line")
	}
	if naive := m[1] + "T" + m[2]; !strings.HasPrefix(created, naive) {
		t.Errorf("json created %q does not start with the file timestamp %q", created, naive)
	}
}

// TestEditLayoutCanonicalBlankLines pins MD022/MD032 on the edited file:
// every heading sits between blank lines, every list item opens a list
// only after a blank line, list items stay consecutive (one journal list,
// not several), no double blank lines accumulate (MD012) and the file
// ends with exactly one newline after the last journal entry.
func TestEditLayoutCanonicalBlankLines(t *testing.T) {
	dir := t.TempDir()
	createTestTicket(t, dir)
	if _, stderr, code := editRun(t, dir,
		"edit", "1", "--title", "Новый заголовок", "--priority", "high"); code != 0 {
		t.Fatalf("Run(edit) = %d, want 0; stderr: %q", code, stderr)
	}
	assertCanonicalMarkdownLayout(t, readTicket(t, dir))
}

// assertCanonicalMarkdownLayout encodes the markdownlint rules the
// renderer must satisfy for a freshly edited ticket: MD022 (blanks around
// headings), MD032 (blanks around lists) and the MD012 end shape.
func assertCanonicalMarkdownLayout(t *testing.T, body string) {
	t.Helper()
	assertSingleTrailingNewline(t, body)
	lines := strings.Split(strings.TrimSuffix(body, "\n"), "\n")
	for i, line := range lines {
		assertHeadingBlanks(t, lines, i, line)
		assertListItemBlanks(t, lines, i, line)
		if isBlankLine(lines, i) && isBlankLine(lines, i+1) {
			t.Errorf("line %d: consecutive blank lines (MD012)", i+1)
		}
	}
}

// isBlankLine reports whether line i is outside the file or whitespace-only.
func isBlankLine(lines []string, i int) bool {
	return i < 0 || i >= len(lines) || strings.TrimSpace(lines[i]) == ""
}

// assertSingleTrailingNewline pins the MD012 end shape: exactly one "\n"
// after the last journal entry.
func assertSingleTrailingNewline(t *testing.T, body string) {
	t.Helper()
	if !strings.HasSuffix(body, "\n") || strings.HasSuffix(body, "\n\n") {
		t.Fatalf("file must end with exactly one newline (MD012), tail: %q", tail(body))
	}
}

// assertHeadingBlanks pins MD022: a heading line sits between blank lines.
func assertHeadingBlanks(t *testing.T, lines []string, i int, line string) {
	t.Helper()
	if !strings.HasPrefix(line, "## ") && !strings.HasPrefix(line, "# ") {
		return
	}
	if !isBlankLine(lines, i-1) || !isBlankLine(lines, i+1) {
		t.Errorf("line %d: heading %q not surrounded by blank lines (MD022)", i+1, line)
	}
}

// assertListItemBlanks pins MD032: a list item that opens a list (the
// previous line is not an item) must be preceded by a blank line.
func assertListItemBlanks(t *testing.T, lines []string, i int, line string) {
	t.Helper()
	if i == 0 || !strings.HasPrefix(line, "- ") ||
		strings.HasPrefix(lines[i-1], "- ") || isBlankLine(lines, i-1) {
		return
	}
	t.Errorf("line %d: list item %q not preceded by a blank line (MD032)", i+1, line)
}

// tail returns the last min(len(body), 40) bytes for failure messages.
func tail(body string) string {
	if len(body) > 40 {
		return body[len(body)-40:]
	}
	return body
}
