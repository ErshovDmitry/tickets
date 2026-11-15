package cli_test

// T-0188 / GitHub #21: `ticket list --type` and `--priority` value flags.
// Everything runs through the black-box cli.Run entry against t.TempDir()
// stores — never the real tickets/ directory.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"ticket/internal/cli"
)

// setupTypePriorityStore seeds five tickets with distinct type/priority/
// project/status combinations and archives the TD/normal one, so the fixture
// covers the main directory and tickets/archive/:
//
//	T-0001  open  BUG  high    projA  (main)
//	T-0002  open  OPS  low     projB  (main)
//	T-0003  done  TD   normal  projA  (archive)
//	T-0004  open  ENH  high    projB  (main)
//	T-0005  wip   BUG  high    projA  (main; differs from T-0001 by status only)
func setupTypePriorityStore(t *testing.T) (string, map[string]string) {
	t.Helper()
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	seed := []struct {
		title, typ, prio, project string
	}{
		{"bug high", "BUG", "high", "projA"},
		{"ops low", "OPS", "low", "projB"},
		{"td normal", "TD", "normal", "projA"},
		{"enh high", "ENH", "high", "projB"},
		{"bug high wip", "BUG", "high", "projA"},
	}
	for i, s := range seed {
		var stdout, stderr bytes.Buffer
		args := []string{"new", s.title, "-t", s.typ, "-p", s.prio, "-P", s.project}
		if code := cli.Run(args, env, &stdout, &stderr); code != 0 {
			t.Fatalf("seed new #%d = %d, stderr: %q", i+1, code, stderr.String())
		}
	}
	// T-0003 goes to archive/; T-0005 becomes the status twin of T-0001.
	for _, fix := range []struct{ num, state string }{{"3", "done"}, {"5", "wip"}} {
		var stdout, stderr bytes.Buffer
		if code := cli.Run([]string{"set", fix.num, fix.state, "fixture"}, env, &stdout, &stderr); code != 0 {
			t.Fatalf("set %s %s = %d, stderr: %q", fix.num, fix.state, code, stderr.String())
		}
	}
	var stdout, stderr bytes.Buffer
	stdout.Reset()
	stderr.Reset()
	if code := cli.Run([]string{"archive"}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("archive = %d, stderr: %q", code, stderr.String())
	}
	return dir, env
}

// runList invokes `list` with args and returns stdout, stderr, exit code.
func runList(t *testing.T, env map[string]string, args ...string) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run(append([]string{"list"}, args...), env, &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

// TestListTypeFilter checks --type alone (AND default status active).
func TestListTypeFilter(t *testing.T) {
	_, env := setupTypePriorityStore(t)
	out, errOut, code := runList(t, env, "--type", "BUG")
	if code != 0 {
		t.Fatalf("list --type BUG = %d, stderr: %q", code, errOut)
	}
	if !strings.Contains(out, "T-0001") {
		t.Errorf("list --type BUG missing T-0001: %q", out)
	}
	for _, banned := range []string{"T-0002", "T-0003", "T-0004"} {
		if strings.Contains(out, banned) {
			t.Errorf("list --type BUG leaked %s: %q", banned, out)
		}
	}
}

// TestListPriorityFilter checks --priority alone.
func TestListPriorityFilter(t *testing.T) {
	_, env := setupTypePriorityStore(t)
	out, errOut, code := runList(t, env, "--priority", "high")
	if code != 0 {
		t.Fatalf("list --priority high = %d, stderr: %q", code, errOut)
	}
	if !strings.Contains(out, "T-0001") || !strings.Contains(out, "T-0004") {
		t.Errorf("list --priority high missing high-priority tickets: %q", out)
	}
	if strings.Contains(out, "T-0002") || strings.Contains(out, "T-0003") {
		t.Errorf("list --priority high leaked non-high tickets: %q", out)
	}
}

// TestListTypePriorityAndStatusAndProject checks the full AND combination.
func TestListTypePriorityAndStatusAndProject(t *testing.T) {
	_, env := setupTypePriorityStore(t)
	out, errOut, code := runList(t, env, "open", "-P", "projA", "--type", "BUG", "--priority", "high")
	if code != 0 {
		t.Fatalf("combined list = %d, stderr: %q", code, errOut)
	}
	if !strings.Contains(out, "T-0001") {
		t.Errorf("combined list missing T-0001: %q", out)
	}
	for _, banned := range []string{"T-0002", "T-0003", "T-0004"} {
		if strings.Contains(out, banned) {
			t.Errorf("combined list leaked %s: %q", banned, out)
		}
	}
	// T-0005 is BUG/high/projA exactly like T-0001, so it survives the
	// type+priority+-P legs; only the `open` status leg can exclude it.
	if strings.Contains(out, "T-0005") {
		t.Errorf("combined list (open) leaked wip T-0005: the status leg is not applied: %q", out)
	}
	// Proof that the exclusion above is caused by status alone: the very
	// same type/priority/-P query with status wip returns T-0005, not T-0001.
	out, errOut, code = runList(t, env, "wip", "-P", "projA", "--type", "BUG", "--priority", "high")
	if code != 0 {
		t.Fatalf("combined list (wip) = %d, stderr: %q", code, errOut)
	}
	if !strings.Contains(out, "T-0005") {
		t.Errorf("combined list (wip) missing T-0005: %q", out)
	}
	if strings.Contains(out, "T-0001") {
		t.Errorf("combined list (wip) leaked open T-0001: %q", out)
	}
	// One negative leg proves the priority leg is really applied: same
	// status/project/type but the wrong priority must be empty.
	out, errOut, code = runList(t, env, "open", "-P", "projA", "--type", "BUG", "--priority", "low")
	if code != 0 {
		t.Fatalf("combined list (low) = %d, stderr: %q", code, errOut)
	}
	if strings.Contains(out, "T-0001") {
		t.Errorf("combined list with --priority low showed a high ticket: %q", out)
	}
}

// TestListTypePriorityArchive checks that both filters apply to archive,
// where the status leg is intentionally not evaluated.
func TestListTypePriorityArchive(t *testing.T) {
	_, env := setupTypePriorityStore(t)
	out, errOut, code := runList(t, env, "archive", "--type", "TD", "--priority", "normal")
	if code != 0 {
		t.Fatalf("list archive --type TD --priority normal = %d, stderr: %q", code, errOut)
	}
	if !strings.Contains(out, "T-0003") {
		t.Errorf("list archive filters missing T-0003: %q", out)
	}
	if strings.Contains(out, "T-0001") || strings.Contains(out, "T-0004") {
		t.Errorf("list archive filters leaked main-dir tickets: %q", out)
	}
	// Negative leg inside the archive source: nothing there is BUG/high
	// (only T-0003 TD/normal), so the AND of both value flags must be empty
	// while exit stays 0 and the pinned notice names every active filter.
	out, errOut, code = runList(t, env, "archive", "--type", "BUG", "--priority", "high")
	if code != 0 {
		t.Fatalf("list archive --type BUG --priority high = %d, stderr: %q", code, errOut)
	}
	if strings.Contains(out, "T-0003") {
		t.Errorf("archive negative leg leaked T-0003: %q", out)
	}
	wantEmpty := "Нет тикетов (archive --type BUG --priority high)."
	if !strings.Contains(out, wantEmpty) {
		t.Errorf("archive negative leg stdout = %q, want %q", out, wantEmpty)
	}
}

// TestListTypePriorityRepeatLastWins: repeating --type/--priority is not an
// error; the last value wins (an invalid earlier value is overwritten too).
func TestListTypePriorityRepeatLastWins(t *testing.T) {
	_, env := setupTypePriorityStore(t)
	out, errOut, code := runList(t, env, "all", "--type", "OPS", "--type", "BUG", "--priority", "low", "--priority", "high")
	if code != 0 {
		t.Fatalf("repeated filters = %d, stderr: %q", code, errOut)
	}
	// Repeating a value flag is silent: no warning, no ambiguity note.
	if errOut != "" {
		t.Errorf("repeated filters stderr = %q, want empty", errOut)
	}
	if !strings.Contains(out, "T-0001") {
		t.Errorf("repeated filters (last wins) missing T-0001: %q", out)
	}
	if strings.Contains(out, "T-0002") {
		t.Errorf("repeated filters kept the overwritten OPS/low value: %q", out)
	}

	// An earlier invalid value is fully overwritten by the last one and is
	// never validated (BOGUS would be rejected on its own).
	out, errOut, code = runList(t, env, "all", "--type", "BOGUS", "--type", "BUG", "--priority", "nope", "--priority", "high")
	if code != 0 {
		t.Fatalf("repeated flags with invalid first = %d, stderr: %q", code, errOut)
	}
	if errOut != "" {
		t.Errorf("repeated flags stderr = %q, want empty (overwritten values must not be validated)", errOut)
	}
	if !strings.Contains(out, "T-0001") || !strings.Contains(out, "T-0005") {
		t.Errorf("repeated flags last-wins missing BUG/high rows: %q", out)
	}
	if strings.Contains(out, "T-0002") || strings.Contains(out, "T-0004") {
		t.Errorf("repeated flags kept an overwritten value: %q", out)
	}
}

// TestListRepeatJsonStillError: --json stays a bare flag and keeps its
// duplicate rejection.
func TestListRepeatJsonStillError(t *testing.T) {
	_, env := setupTypePriorityStore(t)
	_, errOut, code := runList(t, env, "--json", "--type", "BUG", "--json")
	if code != 1 {
		t.Fatalf("list --json ... --json = %d, want 1", code)
	}
	if !strings.Contains(errOut, "повторный флаг --json") {
		t.Errorf("stderr = %q, want repeated --json rejection", errOut)
	}
}

// TestListTypePriorityEmptyPlain: the plain "no tickets" line names every
// active filter, exit stays 0.
func TestListTypePriorityEmptyPlain(t *testing.T) {
	_, env := setupTypePriorityStore(t)
	out, errOut, code := runList(t, env, "done", "-P", "projA", "--type", "BUG", "--priority", "low")
	if code != 0 {
		t.Fatalf("empty plain list = %d, stderr: %q", code, errOut)
	}
	want := "Нет тикетов (done -P projA --type BUG --priority low)."
	if !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

// TestListTypePriorityEmptyJSON: filtered JSON is `[]`, not null.
func TestListTypePriorityEmptyJSON(t *testing.T) {
	_, env := setupTypePriorityStore(t)
	out, errOut, code := runList(t, env, "done", "--type", "BUG", "--json")
	if code != 0 {
		t.Fatalf("empty JSON list = %d, stderr: %q", code, errOut)
	}
	if got := strings.TrimSpace(out); got != "[]" {
		t.Errorf("filtered empty JSON = %q, want []", got)
	}
}

// fixtureCreatedLayout is how the Created meta line stores the timestamp.
const fixtureCreatedLayout = "2006-01-02 15:04"

// fixtureCreatedRegexp captures the timestamp of the Created meta line.
var fixtureCreatedRegexp = regexp.MustCompile(`(?m)^- Created \(Создан\): (\d{4}-\d{2}-\d{2} \d{2}:\d{2})`)

// fixtureCreated returns the Created value of ticket num as written on disk,
// so a JSON assertion can compare against the fixture itself instead of an
// arbitrary time.
func fixtureCreated(t *testing.T, dir string, num int) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, fmt.Sprintf("T-%04d-*.md", num)))
	if err != nil {
		t.Fatalf("glob T-%04d: %v", num, err)
	}
	if len(matches) != 1 {
		t.Fatalf("T-%04d files = %v, want exactly 1", num, matches)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read %s: %v", matches[0], err)
	}
	m := fixtureCreatedRegexp.FindStringSubmatch(string(data))
	if m == nil {
		t.Fatalf("no Created meta line in %s", matches[0])
	}
	return m[1]
}

// TestListTypePriorityJSONShape pins the DTO: exactly the 6 documented field
// names and the real fixture values (number/status/type/priority/title and a
// created timestamp that matches the ticket file), not merely a field count.
func TestListTypePriorityJSONShape(t *testing.T) {
	dir, env := setupTypePriorityStore(t)
	out, errOut, code := runList(t, env, "all", "--type", "BUG", "--priority", "high", "--json")
	if code != 0 {
		t.Fatalf("JSON list = %d, stderr: %q", code, errOut)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("JSON decode: %v (raw %q)", err, out)
	}
	// The AND of type+priority must return exactly the two BUG/high tickets,
	// each with its own status: T-0001 open and T-0005 wip.
	want := map[int]struct{ status, title string }{
		1: {"open", "bug high"},
		5: {"wip", "bug high wip"},
	}
	if len(rows) != len(want) {
		t.Fatalf("JSON rows = %d, want %d (raw %q)", len(rows), len(want), out)
	}
	seen := make(map[int]bool, len(rows))
	for _, row := range rows {
		if len(row) != 6 {
			t.Errorf("JSON row has %d fields, want 6: %v", len(row), row)
		}
		for _, k := range []string{"number", "status", "type", "priority", "title", "created"} {
			if _, ok := row[k]; !ok {
				t.Errorf("JSON row missing field %q: %v", k, row)
			}
		}
		for _, k := range []string{"details", "who", "project", "comments", "lang"} {
			if _, ok := row[k]; ok {
				t.Errorf("JSON row has unexpected field %q: %v", k, row)
			}
		}
		numF, ok := row["number"].(float64)
		if !ok {
			t.Fatalf("JSON number is not a JSON number: %T (%v)", row["number"], row)
		}
		num := int(numF)
		exp, ok := want[num]
		if !ok {
			t.Fatalf("unexpected ticket in JSON: T-%04d (%v)", num, row)
		}
		seen[num] = true
		if row["type"] != "BUG" || row["priority"] != "high" {
			t.Errorf("T-%04d type/priority = %v/%v, want BUG/high", num, row["type"], row["priority"])
		}
		if row["status"] != exp.status {
			t.Errorf("T-%04d status = %v, want %q", num, row["status"], exp.status)
		}
		if row["title"] != exp.title {
			t.Errorf("T-%04d title = %v, want %q", num, row["title"], exp.title)
		}
		created, ok := row["created"].(string)
		if !ok {
			t.Fatalf("T-%04d created is not a string: %T", num, row["created"])
		}
		ts, err := time.Parse(time.RFC3339, created)
		if err != nil {
			t.Errorf("T-%04d created not RFC3339: %q (%v)", num, created, err)
			continue
		}
		if got, wantCreated := ts.Format(fixtureCreatedLayout), fixtureCreated(t, dir, num); got != wantCreated {
			t.Errorf("T-%04d created = %q, want the fixture value %q", num, got, wantCreated)
		}
	}
	for num := range want {
		if !seen[num] {
			t.Errorf("JSON output missing T-%04d: %q", num, out)
		}
	}
}

// TestListTypePriorityErrors pins every rejection, in both locales, and the
// error precedence (flags → positional → enum → store).
func TestListTypePriorityErrors(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantRU   string
		wantEN   string
		wantCode int
	}{
		{"bad type", []string{"--type", "BUGX"}, "тип — один из: BUG OPS TD ENH", "type must be one of: BUG OPS TD ENH", 1},
		{"bad priority", []string{"--priority", "urgent"}, "приоритет — один из: low normal high", "priority must be one of: low normal high", 1},
		{"type without value", []string{"--type"}, "флаг --type требует значение", "flag --type requires a value", 1},
		{"priority without value", []string{"--priority"}, "флаг --priority требует значение", "flag --priority requires a value", 1},
		{"type equals form", []string{"--type=BUG"}, "неизвестный флаг: --type=BUG", "unknown flag: --type=BUG", 1},
		{"priority equals form", []string{"--priority=high"}, "неизвестный флаг: --priority=high", "unknown flag: --priority=high", 1},
		{"empty type", []string{"--type", ""}, "тип — один из: BUG OPS TD ENH", "type must be one of: BUG OPS TD ENH", 1},
		{"empty priority", []string{"--priority", ""}, "приоритет — один из: low normal high", "priority must be one of: low normal high", 1},
		{"status as type", []string{"--type", "open"}, "тип — один из: BUG OPS TD ENH", "type must be one of: BUG OPS TD ENH", 1},
		{"flag as type value", []string{"--type", "--json"}, "тип — один из: BUG OPS TD ENH", "type must be one of: BUG OPS TD ENH", 1},
		{"status beats type", []string{"bogus", "--type", "BUGX"}, "фильтр — один из: active open wip done closed archive all", "filter must be one of: active open wip done closed archive all", 1},
		{"positional count beats type", []string{"open", "extra", "--type", "BUGX"}, "список принимает не более одного аргумента (фильтр)", "list accepts at most one argument (filter)", 1},
		// Parse-time flag errors beat every later validation: a dangling
		// --type after a bad status is reported as a missing value, not as a
		// bad status (flags → positional → enum → store).
		{"missing type value beats bad status", []string{"bogus", "--type"}, "флаг --type требует значение", "flag --type requires a value", 1},
		{"missing priority value beats bad status", []string{"bogus", "--priority"}, "флаг --priority требует значение", "flag --priority requires a value", 1},
		{"unknown flag beats bad type", []string{"--nope", "--type", "BUGX"}, "неизвестный флаг: --nope", "unknown flag: --nope", 1},
		{"unknown flag beats bad status", []string{"bogus", "--nope"}, "неизвестный флаг: --nope", "unknown flag: --nope", 1},
		// Enum order: type is validated before priority, so a run with both
		// values bad reports the type.
		{"type enum beats priority enum", []string{"--priority", "urgent", "--type", "BUGX"}, "тип — один из: BUG OPS TD ENH", "type must be one of: BUG OPS TD ENH", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/ru", func(t *testing.T) {
			_, env := setupTypePriorityStore(t)
			out, errOut, code := runList(t, env, tc.args...)
			if code != tc.wantCode {
				t.Fatalf("code = %d, want %d (stderr %q)", code, tc.wantCode, errOut)
			}
			if out != "" {
				t.Errorf("stdout = %q, want empty on an error path", out)
			}
			if !strings.Contains(errOut, tc.wantRU) {
				t.Errorf("stderr = %q, want substring %q", errOut, tc.wantRU)
			}
		})
		t.Run(tc.name+"/en", func(t *testing.T) {
			_, env := setupTypePriorityStore(t)
			env["TICKET_LANG"] = "en"
			out, errOut, code := runList(t, env, tc.args...)
			if code != tc.wantCode {
				t.Fatalf("code = %d, want %d (stderr %q)", code, tc.wantCode, errOut)
			}
			if out != "" {
				t.Errorf("stdout = %q, want empty on an error path", out)
			}
			if !strings.Contains(errOut, tc.wantEN) {
				t.Errorf("stderr = %q, want substring %q", errOut, tc.wantEN)
			}
			assertNoCyrillic(t, "en stderr", errOut)
			assertNoCyrillic(t, "en stdout", out)
		})
	}
}
