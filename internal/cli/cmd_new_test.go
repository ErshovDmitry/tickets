package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"ticket/internal/cli"
)

// Golden reference ticket: byte copy of the bash-created tickets/T-0001-open.md
// (internal/cli/testdata/golden-T-0001-open.md). Title/type/priority/details
// must match the golden file exactly for the byte comparison to hold.
const (
	goldenTitle   = "Реализовать Go-версию ticket по AGENTS_ARCHITECTURE.md"
	goldenDetails = "Bootstrap завершён 2026-09-02: схема project_tickets, AGENTS.md, AGENTS_ARCHITECTURE.md, локальные тикеты. Реализация: cmd/ticket + internal/{domain,store,lock,paths,cli}, шаблоны через go:embed. Гейт: план в wiki -> план-ревью -> код."
	goldenWho     = "erdmitry"
)

// tsPattern matches creation timestamps like 2026-09-02 03:24 or
// 2026-09-02T03:24:05 (space or 'T' separator, seconds optional).
const tsPattern = `\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}(:\d{2})?`

var (
	// createdRe captures the volatile parts of the header line
	// "- Создан: <TS> · кем: <WHO>".
	createdRe = regexp.MustCompile(`(?m)^(- Создан: )(` + tsPattern + `)( · кем: )([^\n]+)$`)
	// journalRe captures the volatile parts of a journal entry
	// "- <TS> — тикет создан (<WHO>).".
	journalRe = regexp.MustCompile(`(?m)^(- )(` + tsPattern + `)( — тикет создан \()([^)\n]+)(\)\.)$`)
	// projectRe captures the value of the "- Проект: <PROJECT>" line.
	projectRe = regexp.MustCompile(`(?m)^(- Проект: )([^\n]+)$`)
)

// normalizeVolatile blanks out only the volatile substrings — creation
// timestamps, project name (derived from the temp dir) and who — inside
// their structural contexts, preserving every separator and literal
// ("· кем:", em-dash, dots). Degraded format no longer normalizes away:
// an unmatched line stays raw and fails the byte comparison.
func normalizeVolatile(b []byte) []byte {
	s := createdRe.ReplaceAllString(string(b), "${1}<TS>${4}<WHO>")
	s = journalRe.ReplaceAllString(s, "${1}<TS>${4}<WHO>${6}")
	s = projectRe.ReplaceAllString(s, "${1}<PROJECT>")
	return []byte(s)
}

// TestNewGoldenBytes creates a ticket end-to-end via Run and compares the
// created file bytes with the golden fixture (volatile fields normalized).
func TestNewGoldenBytes(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	var stdout, stderr bytes.Buffer
	args := []string{"new", goldenTitle, "-t", "ENH", "-p", "high", "-d", goldenDetails, "-w", goldenWho}
	if code := cli.Run(args, env, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(new) = %d, want 0; stderr: %q", code, stderr.String())
	}
	created, err := os.ReadFile(filepath.Join(dir, "T-0001-open.md"))
	if err != nil {
		t.Fatalf("created ticket is not readable: %v", err)
	}
	golden, err := os.ReadFile(filepath.Join("testdata", "golden-T-0001-open.md"))
	if err != nil {
		t.Fatalf("golden fixture is not readable: %v", err)
	}
	if !bytes.Equal(normalizeVolatile(created), normalizeVolatile(golden)) {
		t.Fatalf("created ticket differs from golden:\n--- created ---\n%s\n--- golden ---\n%s", created, golden)
	}
	// §6: on success Run prints the created file path to stdout (exit 0).
	if !strings.Contains(stdout.String(), "T-0001-open.md") {
		t.Fatalf("stdout %q does not mention the created file", stdout.String())
	}
	// §6: project = base name of the tickets dir parent (bash:10).
	wantProject := "Проект: " + filepath.Base(filepath.Dir(dir))
	if !strings.Contains(string(created), wantProject) {
		t.Fatalf("created ticket missing project line %q:\n%s", wantProject, created)
	}
	// §6: -w sets who, recorded in the header and journal lines.
	if !strings.Contains(string(created), "кем: "+goldenWho) {
		t.Fatalf("created ticket missing who %q:\n%s", goldenWho, created)
	}
}

// TestNewDefaults checks default type=BUG, priority=normal and the
// empty-details stub from the bash heredoc template.
func TestNewDefaults(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := cli.Run([]string{"new", "Проверка дефолтов"}, map[string]string{"TICKETS_DIR": dir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(new) = %d, want 0; stderr: %q", code, stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(dir, "T-0001-open.md"))
	if err != nil {
		t.Fatalf("created ticket is not readable: %v", err)
	}
	for _, want := range []string{
		"# T-0001 · BUG: Проверка дефолтов",
		"- Статус: open",
		"- Приоритет: normal",
		"## Подробности",
		"<!-- что найдено, где (файл:строка), логи/вывод, как воспроизвести, предложение по исправлению -->",
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("created ticket missing %q:\n%s", want, data)
		}
	}
}

// TestNewFlagErrors checks bash die() messages and exit code 1; no ticket
// file may be created when validation fails.
func TestNewFlagErrors(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantSub string
	}{
		{"invalid type", []string{"new", "X", "-t", "WRONG"}, "тип — один из: BUG OPS TD ENH"},
		{"invalid priority", []string{"new", "X", "-p", "urgent"}, "приоритет — один из: low normal high"},
		{"unknown flag", []string{"new", "X", "-z"}, "ticket: неизвестный аргумент: -z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			var stdout, stderr bytes.Buffer
			code := cli.Run(tc.args, map[string]string{"TICKETS_DIR": dir}, &stdout, &stderr)
			if code != 1 {
				t.Fatalf("Run(%q) = %d, want 1", tc.args, code)
			}
			if !strings.Contains(stderr.String(), tc.wantSub) {
				t.Fatalf("stderr %q does not contain %q", stderr.String(), tc.wantSub)
			}
			created, _ := filepath.Glob(filepath.Join(dir, "T-*.md"))
			if len(created) != 0 {
				t.Fatalf("ticket file created despite validation error: %v", created)
			}
		})
	}
}

// TestNewNoTitle checks the bash contract: `new` without a title prints the
// usage text to stdout and exits 1 (bash:60).
func TestNewNoTitle(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := cli.Run([]string{"new"}, testEnv(t), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(new) = %d, want 1", code)
	}
	if got := stdout.String(); got != string(usageFixture) {
		t.Fatalf("stdout is not the verbatim usage text (got %d bytes, want %d)", len(got), len(usageFixture))
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

// TestNewEmptyTitleRejected pins the V16 fix: empty and whitespace-only
// titles are rejected with exit 1 and the domain.Strict message before any
// ticket file is created. The no-arg path (usage, TestNewNoTitle) and valid
// titles are unaffected.
func TestNewEmptyTitleRejected(t *testing.T) {
	cases := []struct {
		name  string
		title string
	}{
		{"empty", ""},
		{"spaces", "   "},
		{"tabs and newlines", "\t\n "},
		{"unicode spaces", "\u00a0\u2003"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			var stdout, stderr bytes.Buffer
			code := cli.Run([]string{"new", tc.title}, map[string]string{"TICKETS_DIR": dir}, &stdout, &stderr)
			if code != 1 {
				t.Fatalf("Run(new %q) = %d, want 1; stderr: %q", tc.title, code, stderr.String())
			}
			if !strings.Contains(stderr.String(), "краткое описание не может быть пустым") {
				t.Fatalf("stderr %q does not contain the empty-title error", stderr.String())
			}
			created, _ := filepath.Glob(filepath.Join(dir, "T-*.md"))
			if len(created) != 0 {
				t.Fatalf("ticket file created despite empty title: %v", created)
			}
		})
	}
}

// TestNewTitlePreservedVerbatim guards against over-trimming: a title with
// surrounding whitespace is non-empty, so the ticket must be created and the
// title stored byte-for-byte (no normalization of valid input).
func TestNewTitlePreservedVerbatim(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	title := "  padded title  "
	if code := cli.Run([]string{"new", title}, map[string]string{"TICKETS_DIR": dir}, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(new) = %d, want 0; stderr: %q", code, stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(dir, "T-0001-open.md"))
	if err != nil {
		t.Fatalf("created ticket is not readable: %v", err)
	}
	if !strings.Contains(string(data), "# T-0001 · BUG: "+title) {
		t.Fatalf("title not preserved verbatim in H1:\n%s", data)
	}
}

// TestNewWhoResolution checks the bash who chain TICKET_WHO -> USER -> agent
// (bash:12) and that -w overrides the environment (bash:68).
func TestNewWhoResolution(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		args    []string
		wantWho string
	}{
		{"ticket who env", map[string]string{"TICKET_WHO": "who-env"}, nil, "who-env"},
		{"user fallback", map[string]string{"USER": "who-user"}, nil, "who-user"},
		{"agent fallback", map[string]string{}, nil, "agent"},
		{"flag overrides env", map[string]string{"TICKET_WHO": "who-env"}, []string{"-w", "who-flag"}, "who-flag"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.env["TICKETS_DIR"] = dir
			args := append([]string{"new", "who test"}, tc.args...)
			var stdout, stderr bytes.Buffer
			if code := cli.Run(args, tc.env, &stdout, &stderr); code != 0 {
				t.Fatalf("Run(%q) = %d, want 0; stderr: %q", args, code, stderr.String())
			}
			data, err := os.ReadFile(filepath.Join(dir, "T-0001-open.md"))
			if err != nil {
				t.Fatalf("created ticket is not readable: %v", err)
			}
			for _, want := range []string{"кем: " + tc.wantWho, "тикет создан (" + tc.wantWho + ")"} {
				if !strings.Contains(string(data), want) {
					t.Fatalf("created ticket missing %q:\n%s", want, data)
				}
			}
		})
	}
}
