package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/cli"
)

// setupMultiProject creates a temp tickets dir with 3 tickets across 2 projects.
func setupMultiProject(t *testing.T) (dir string, env map[string]string) {
	t.Helper()
	dir = t.TempDir()
	env = map[string]string{"TICKETS_DIR": dir}
	// Create 2 tickets for projA, 1 for projB
	for i, proj := range []string{"projA", "projA", "projB"} {
		var stdout, stderr bytes.Buffer
		args := []string{"new", "Ticket " + string(rune('A'+i)), "-P", proj}
		if code := cli.Run(args, env, &stdout, &stderr); code != 0 {
			t.Fatalf("setup new #%d = %d, stderr: %q", i+1, code, stderr.String())
		}
	}
	return dir, env
}

// TestListProjectFilter verifies -P filters by exact project match (T-0040).
func TestListProjectFilter(t *testing.T) {
	_, env := setupMultiProject(t)
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"list", "-P", "projA"}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("list -P projA = %d, stderr: %q", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "T-0001") || !strings.Contains(out, "T-0002") {
		t.Errorf("list -P projA missing projA tickets: %q", out)
	}
	if strings.Contains(out, "T-0003") {
		t.Errorf("list -P projA shows projB ticket: %q", out)
	}
}

// TestListProjectFilterArchive verifies -P works with archive (T-0040).
func TestListProjectFilterArchive(t *testing.T) {
	dir, env := setupMultiProject(t)
	// Move T-0001 to done, archive it
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"set", "1", "done", "ok"}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("set 1 done = %d, stderr: %q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := cli.Run([]string{"archive"}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("archive = %d, stderr: %q", code, stderr.String())
	}
	// Verify archive/T-0001-done.md has projA
	data, err := os.ReadFile(filepath.Join(dir, "archive", "T-0001-done.md"))
	if err != nil {
		t.Fatalf("archived ticket not readable: %v", err)
	}
	if !strings.Contains(string(data), "projA") {
		t.Fatalf("archived T-0001 missing projA: %s", data)
	}
	// list archive -P projA should show T-0001
	stdout.Reset()
	stderr.Reset()
	if code := cli.Run([]string{"list", "archive", "-P", "projA"}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("list archive -P projA = %d, stderr: %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "T-0001") {
		t.Errorf("list archive -P projA missing T-0001: %q", stdout.String())
	}
	if strings.Contains(stdout.String(), "T-0002") || strings.Contains(stdout.String(), "T-0003") {
		t.Errorf("list archive -P projA shows non-archived tickets: %q", stdout.String())
	}
}

// TestListProjectColumn verifies project column appears when uniqueProjects > 1
// (T-0040).
func TestListProjectColumn(t *testing.T) {
	_, env := setupMultiProject(t)
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"list"}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("list = %d, stderr: %q", code, stderr.String())
	}
	out := stdout.String()
	// Project column format: T-%04d  %-10s  %-7s  %s
	// Should see "T-0001  projA" and "T-0003  projB"
	if !strings.Contains(out, "projA") || !strings.Contains(out, "projB") {
		t.Errorf("list output missing project column: %q", out)
	}
	// Verify format: each ticket line has project in field[1]
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "T-") {
			parts := strings.Fields(line)
			if len(parts) < 4 {
				t.Errorf("line has <4 fields (no project col): %q", line)
				continue
			}
			// parts[0]=T-NNNN, parts[1]=project, parts[2]=status, parts[3...]=type:title
			if parts[1] != "projA" && parts[1] != "projB" {
				t.Errorf("line field[1] is not a project name: %q", line)
			}
		}
	}
}

// TestListProjectColumnSingle verifies no project column when all tickets
// have same project (T-0040).
func TestListProjectColumnSingle(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	// Create 2 tickets with same project
	for i := 0; i < 2; i++ {
		var stdout, stderr bytes.Buffer
		if code := cli.Run([]string{"new", "Ticket", "-P", "same"}, env, &stdout, &stderr); code != 0 {
			t.Fatalf("new #%d = %d", i+1, code)
		}
	}
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"list"}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("list = %d, stderr: %q", code, stderr.String())
	}
	out := stdout.String()
	// Should NOT see project column (format T-%04d  %-7s  %s)
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "T-") {
			parts := strings.Fields(line)
			// Without project col: 3+ fields (number, status, type:title)
			// With project col: 4+ fields
			if len(parts) >= 4 && parts[1] == "same" {
				t.Errorf("line has project column when uniqueProjects=1: %q", line)
			}
		}
	}
}

// TestListProjectEmpty verifies empty -P value is rejected (T-0040).
func TestListProjectEmpty(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"list", "-P", ""}, env, &stdout, &stderr); code != 1 {
		t.Fatalf("list -P empty = %d, want 1; stderr: %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "непустое") {
		t.Errorf("stderr missing non-empty error: %q", stderr.String())
	}
}

// TestListProjectWhitespace verifies whitespace-only -P value is rejected (T-0040).
func TestListProjectWhitespace(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"list", "-P", "   "}, env, &stdout, &stderr); code != 1 {
		t.Fatalf("list -P whitespace = %d, want 1; stderr: %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "непустое") {
		t.Errorf("stderr missing non-empty error: %q", stderr.String())
	}
}

// TestListUnknownFlag verifies unknown flag is rejected (T-0040 deliberate
// deviation from bash parity).
func TestListUnknownFlag(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"list", "-z"}, env, &stdout, &stderr); code != 1 {
		t.Fatalf("list -z = %d, want 1; stderr: %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "неизвестный") && !strings.Contains(stderr.String(), "-z") {
		t.Errorf("stderr missing unknown flag error: %q", stderr.String())
	}
}

// TestListNoTicketsI18n verifies noTickets message uses i18n (T-0040).
func TestListNoTicketsI18n(t *testing.T) {
	tests := []struct {
		name   string
		env    map[string]string
		expect string
	}{
		{
			name:   "Russian",
			env:    map[string]string{"TICKETS_DIR": t.TempDir()},
			expect: "Нет тикетов (done)",
		},
		{
			name:   "English",
			env:    map[string]string{"TICKETS_DIR": t.TempDir(), "TICKET_LANG": "en"},
			expect: "No tickets (done)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := cli.Run([]string{"list", "done"}, tt.env, &stdout, &stderr); code != 0 {
				t.Fatalf("list done = %d, stderr: %q", code, stderr.String())
			}
			out := stdout.String()
			if !strings.Contains(out, tt.expect) {
				t.Errorf("list done output missing i18n noTickets %q: %q", tt.expect, out)
			}
		})
	}
}
