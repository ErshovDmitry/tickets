package cli_test

// T-0157 duplicate-number contract via the public cli.Run surface (plan c5
// D5.2), fixture: healthy T-0001-open.md + foreign T-0001-done.md, so number 1
// is owned by two files. RU pinned byte-for-byte; EN: cmd_ambiguous_i18n_test.go.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/cli"
)

// ambiguousPair seeds dir with a healthy T-0001-open.md (via new) and a
// foreign T-0001-done.md (valid body), returning env and both paths (sorted: done < open).
func ambiguousPair(t *testing.T) (map[string]string, string, string, string) {
	t.Helper()
	dir := mustEval(t, t.TempDir())
	env := map[string]string{"TICKETS_DIR": dir}
	createTestTicket(t, dir) // T-0001-open.md
	// Same project as createTestTicket (the parent dir name — the store
	// derives the project from it) keeps list rows free of the project column.
	writeJSONTicket(t, dir, "T-0001-done.md", 1, "done", "Дубликат", filepath.Base(filepath.Dir(dir)), "2026-09-08 12:00")
	return env, dir,
		filepath.Join(dir, "T-0001-done.md"),
		filepath.Join(dir, "T-0001-open.md")
}

// runAmb runs cli.Run against env and returns code, stdout, stderr.
func runAmb(env map[string]string, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := cli.Run(args, env, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// ambiguousStderr is the pinned RU refusal report: header line + every path indented.
func ambiguousStderr(donePath, openPath string) string {
	return "ticket: номер тикета 1 неоднозначен: найдено несколько файлов\n" +
		"  " + donePath + "\n" +
		"  " + openPath + "\n"
}

// storeSnapshot captures the names and bytes of every file in dir.
func storeSnapshot(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	snap := make(map[string][]byte, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue // archive/ appears only after a successful archive
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		snap[e.Name()] = b
	}
	return snap
}

// assertStoreUnchanged fails when dir differs from snap (extra, missing or
// modified entries — journal, tmp and rename artifacts all show here).
func assertStoreUnchanged(t *testing.T, dir string, snap map[string][]byte) {
	t.Helper()
	after := storeSnapshot(t, dir)
	if len(after) != len(snap) {
		t.Fatalf("dir changed: %d entries, want %d", len(after), len(snap))
	}
	for name, before := range snap {
		got, ok := after[name]
		if !ok {
			t.Fatalf("entry %s disappeared", name)
		}
		if !bytes.Equal(got, before) {
			t.Errorf("entry %s modified:\nbefore: %q\nafter:  %q", name, before, got)
		}
	}
}

// TestAmbiguousShowNumber pins the refusal: show 1 exits 2, prints both
// conflicting paths on stderr, nothing on stdout.
func TestAmbiguousShowNumber(t *testing.T) {
	env, _, donePath, openPath := ambiguousPair(t)
	code, stdout, stderr := runAmb(env, "show", "1")
	if code != 2 {
		t.Fatalf("show 1 = %d, want 2; stderr %q", code, stderr)
	}
	if want := ambiguousStderr(donePath, openPath); stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

// TestAmbiguousShowByName pins the escape hatch: an exact file name opens
// that very file even though the number is ambiguous.
func TestAmbiguousShowByName(t *testing.T) {
	env, _, donePath, openPath := ambiguousPair(t)
	for _, path := range []string{donePath, openPath} {
		code, stdout, stderr := runAmb(env, "show", filepath.Base(path))
		if code != 0 {
			t.Fatalf("show %s = %d, want 0; stderr %q", path, code, stderr)
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if stdout != string(want) {
			t.Errorf("show %s stdout = %q, want file bytes %q", path, stdout, want)
		}
	}
}

// TestAmbiguousShowNameTwoMatches pins the name hatch's own ambiguity:
// the same base name in main and archive matches two files → both paths, exit 2.
func TestAmbiguousShowNameTwoMatches(t *testing.T) {
	env, dir, _, openPath := ambiguousPair(t)
	archiveDir := filepath.Join(dir, "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(openPath)
	if err != nil {
		t.Fatal(err)
	}
	archived := filepath.Join(archiveDir, "T-0001-open.md")
	if err := os.WriteFile(archived, body, 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runAmb(env, "show", "T-0001-open.md")
	if code != 2 {
		t.Fatalf("show T-0001-open.md = %d, want 2; stderr %q", code, stderr)
	}
	want := "ticket: номер тикета 1 неоднозначен: найдено несколько файлов\n" +
		"  " + openPath + "\n" +
		"  " + archived + "\n"
	if stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

// TestAmbiguousList pins the truthful inventory: exit 0, both rows, one warning line.
func TestAmbiguousList(t *testing.T) {
	env, _, donePath, openPath := ambiguousPair(t)
	code, stdout, stderr := runAmb(env, "list", "all")
	if code != 0 {
		t.Fatalf("list all = %d, want 0; stderr %q", code, stderr)
	}
	for _, want := range []string{"T-0001  done", "BUG: Дубликат", "T-0001  open", "BUG: Тестовый тикет"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout %q missing %q", stdout, want)
		}
	}
	wantWarn := "ticket: предупреждение: номер тикета 1 неоднозначен: " + donePath + ", " + openPath + "\n"
	if stderr != wantWarn {
		t.Errorf("stderr = %q, want %q", stderr, wantWarn)
	}
}

// TestAmbiguousListJSON pins the JSON contract under duplicates: exit 0, valid
// JSON, 6-field schema, both copies present, warning only on stderr.
func TestAmbiguousListJSON(t *testing.T) {
	env, _, donePath, openPath := ambiguousPair(t)
	code, stdout, stderr := runAmb(env, "list", "all", "--json")
	if code != 0 {
		t.Fatalf("list --json = %d, want 0; stderr %q", code, stderr)
	}
	var arr []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &arr); err != nil {
		t.Fatalf("json.Unmarshal: %v; stdout %q", err, stdout)
	}
	if len(arr) != 2 {
		t.Fatalf("got %d rows, want 2: %v", len(arr), arr)
	}
	statuses := map[string]bool{}
	for _, m := range arr {
		if len(m) != 6 {
			t.Errorf("row has %d fields, want 6: %v", len(m), m)
		}
		if n, ok := m["number"].(float64); !ok || int(n) != 1 {
			t.Errorf("number = %v, want 1", m["number"])
		}
		statuses[m["status"].(string)] = true
	}
	if !statuses["done"] || !statuses["open"] {
		t.Errorf("statuses = %v, want both done and open", statuses)
	}
	wantWarn := "ticket: предупреждение: номер тикета 1 неоднозначен: " + donePath + ", " + openPath + "\n"
	if stderr != wantWarn {
		t.Errorf("stderr = %q, want %q", stderr, wantWarn)
	}
}

// TestAmbiguousSet pins the refusal: set 1 exits 2, the directory is
// byte-for-byte unchanged (no journal, no tmp, no rename).
func TestAmbiguousSet(t *testing.T) {
	env, dir, donePath, openPath := ambiguousPair(t)
	snap := storeSnapshot(t, dir)
	code, stdout, stderr := runAmb(env, "set", "1", "wip")
	if code != 2 {
		t.Fatalf("set 1 wip = %d, want 2; stderr %q", code, stderr)
	}
	if want := ambiguousStderr(donePath, openPath); stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	assertStoreUnchanged(t, dir, snap)
}

// TestAmbiguousArchiveOne pins the refusal: archive 1 exits 2, the directory
// is byte-for-byte unchanged.
func TestAmbiguousArchiveOne(t *testing.T) {
	env, dir, donePath, openPath := ambiguousPair(t)
	snap := storeSnapshot(t, dir)
	code, stdout, stderr := runAmb(env, "archive", "1")
	if code != 2 {
		t.Fatalf("archive 1 = %d, want 2; stderr %q", code, stderr)
	}
	if want := ambiguousStderr(donePath, openPath); stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	assertStoreUnchanged(t, dir, snap)
}

// TestAmbiguousArchiveAll pins the LOCKED AC: archive-all archives unique closed
// numbers, skips the duplicated one with one warning naming its paths, exit 2.
func TestAmbiguousArchiveAll(t *testing.T) {
	env, dir, donePath, openPath := ambiguousPair(t)
	if code, _, stderr := runAmb(env, "new", "второй"); code != 0 {
		t.Fatalf("fixture new = %d; stderr %q", code, stderr)
	}
	if code, _, stderr := runAmb(env, "set", "2", "closed"); code != 0 {
		t.Fatalf("fixture set 2 = %d; stderr %q", code, stderr)
	}
	pair := map[string][]byte{
		"T-0001-done.md": mustRead(t, donePath),
		"T-0001-open.md": mustRead(t, openPath),
	}
	code, stdout, stderr := runAmb(env, "archive")
	if code != 2 {
		t.Fatalf("archive = %d, want 2; stderr %q", code, stderr)
	}
	wantOut := filepath.Join(dir, "archive", "T-0002-closed.md") + "\n"
	if stdout != wantOut {
		t.Errorf("stdout = %q, want %q", stdout, wantOut)
	}
	wantWarn := "ticket: предупреждение: номер тикета 1 неоднозначен, архивация пропущена: " + donePath + ", " + openPath + "\n"
	if stderr != wantWarn {
		t.Errorf("stderr = %q, want %q", stderr, wantWarn)
	}
	for name, before := range pair {
		// The skipped pair stays in place, both files intact.
		if got := mustRead(t, filepath.Join(dir, name)); !bytes.Equal(got, before) {
			t.Errorf("%s modified after skip:\nbefore: %q\nafter:  %q", name, before, got)
		}
	}
}

// TestArchiveAllNoDuplicates pins the LOCKED AC clean side: no duplicates →
// archive-all keeps its pre-T-0157 behaviour (exit 0).
func TestArchiveAllNoDuplicates(t *testing.T) {
	env, dir := newTicketDir(t)
	runSet(t, env, "1", "done")
	code, stdout, stderr := runAmb(env, "archive")
	if code != 0 {
		t.Fatalf("archive = %d, want 0; stderr %q", code, stderr)
	}
	if want := filepath.Join(dir, "archive", "T-0001-done.md") + "\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

// mustRead reads a file or fails.
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}
