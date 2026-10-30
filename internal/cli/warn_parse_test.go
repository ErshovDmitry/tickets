package cli

// Structured logging of store parse warnings (T-0110, wave 6): the producer
// side — one "store.read.warning" record per warning a command surfaces. The
// negative cases (logging off, privacy, silent sites) live in
// warn_parse_silent_test.go, so the record assertions here stay apart from the
// assertions about what must not be logged.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"ticket/internal/store"
)

// dupJournalSuffix is the second "## Journal" section of the T-0070 fixture:
// the next set merges it and returns a journal-dup warning.
const dupJournalSuffix = "\n## Journal (Журнал)\n- 2099-01-01 10:00 — статус: open → wip · dup entry (someone)\n"

// warnLine renders the pinned log line of one warning. The handler quotes
// every string value (log.go formatValue -> strconv.Quote), so the bytes are
// path="…" reason="…" and not the unquoted shorthand.
func warnLine(name, reason string) string {
	return "ticket: warn: " + warnEvent + " path=" + strconv.Quote(name) + " reason=" + strconv.Quote(reason) + "\n"
}

// warnLines returns the warn records of logText, in order.
func warnLines(logText string) []string {
	var out []string
	for _, line := range strings.Split(logText, "\n") {
		if strings.HasPrefix(line, "ticket: warn: ") {
			out = append(out, line+"\n")
		}
	}
	return out
}

// wantWarnRecords fails unless the log holds exactly wants, in order.
func wantWarnRecords(t *testing.T, logText string, wants ...string) {
	t.Helper()
	got := warnLines(logText)
	if len(got) != len(wants) {
		t.Fatalf("warn records = %q, want %q (log %q)", got, wants, logText)
	}
	for i := range wants {
		if got[i] != wants[i] {
			t.Errorf("warn record %d = %q, want %q", i, got[i], wants[i])
		}
	}
}

// runWithLog runs args with TICKET_LOG in a fresh temp dir and returns the
// exit code, stdout, stderr and the log file content.
func runWithLog(t *testing.T, args []string, env map[string]string) (int, string, string, string) {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "ticket.log")
	logged := map[string]string{logFileEnv: logPath}
	for k, v := range env {
		logged[k] = v
	}
	code, out, errb := runCLI(t, args, logged)
	return code, out, errb, readFile(t, logPath)
}

// reasonsByName indexes the store-assigned reasons of warnings by name.
func reasonsByName(warnings []store.ParseWarning) map[string]string {
	m := make(map[string]string, len(warnings))
	for _, w := range warnings {
		m[w.Name] = w.Reason
	}
	return m
}

// listReasons derives the reasons list will log: the fixture directory is read
// through the store exactly as list does (read-only, so it stays untouched),
// either the main directory or archive/.
func listReasons(t *testing.T, dir string, archive bool) map[string]string {
	t.Helper()
	st, err := store.NewReadOnly(dir)
	if err != nil {
		t.Fatalf("store.NewReadOnly(%s): %v", dir, err)
	}
	if archive {
		_, warnings := st.ListArchive()
		return reasonsByName(warnings)
	}
	_, warnings := st.List()
	return reasonsByName(warnings)
}

// setReasons derives the reason set will log: the fixture's ticket 1 bytes are
// replayed into a scratch directory and SetStatus on them returns the warning
// the CLI must log.
func setReasons(t *testing.T, srcDir string) map[string]string {
	t.Helper()
	dir := t.TempDir()
	body := readFile(t, filepath.Join(srcDir, "T-0001-open.md"))
	if err := os.WriteFile(filepath.Join(dir, "T-0001-open.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("seed scratch ticket: %v", err)
	}
	st, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New(scratch): %v", err)
	}
	_, warnings, err := st.SetStatus(1, "wip", "tester", "")
	if err != nil {
		t.Fatalf("scratch SetStatus: %v", err)
	}
	return reasonsByName(warnings)
}

// dupJournalFixture returns a fixture whose ticket 1 file carries a second
// Journal section, plus the reason the store assigns to it.
func dupJournalFixture(t *testing.T) (string, string) {
	t.Helper()
	dir := verboseFixture(t, "")
	path := filepath.Join(dir, "T-0001-open.md")
	if err := os.WriteFile(path, []byte(readFile(t, path)+dupJournalSuffix), 0o644); err != nil {
		t.Fatalf("append dup journal: %v", err)
	}
	reason := setReasons(t, dir)["T-0001-open.md"]
	if reason == "" {
		t.Fatalf("the store reported no warning for T-0001-open.md")
	}
	return dir, reason
}

// TestWarnList pins the main-directory list site: one warn record, emitted
// before the human "ticket: …" line, with the store-assigned reason.
func TestWarnList(t *testing.T) {
	dir := verboseFixture(t, "")
	if err := os.WriteFile(filepath.Join(dir, "stray.txt"), []byte("stray"), 0o644); err != nil {
		t.Fatal(err)
	}
	reason := listReasons(t, dir, false)["stray.txt"]
	code, out, errb, logText := runWithLog(t, []string{"list"}, map[string]string{"TICKETS_DIR": dir})
	if code != 0 {
		t.Fatalf("list = %d, want 0; stderr %q", code, errb)
	}
	if !strings.Contains(out, "T-0001") {
		t.Errorf("stdout = %q, want the open ticket", out)
	}
	if !strings.Contains(errb, "ticket: stray.txt: ") {
		t.Errorf("human warning missing from stderr: %q", errb)
	}
	wantWarnRecords(t, logText, warnLine("stray.txt", reason))
}

// TestWarnListArchive pins the `list archive` site: the same structured record
// for a rejected name under archive/.
func TestWarnListArchive(t *testing.T) {
	dir := verboseFixture(t, "done")
	if code, _, stderr := runCLI(t, []string{"archive", "1"}, map[string]string{"TICKETS_DIR": dir}); code != 0 {
		t.Fatalf("fixture archive 1 = %d, want 0; stderr %q", code, stderr)
	}
	if err := os.WriteFile(filepath.Join(dir, "archive", "README.md"), []byte("notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	reason := listReasons(t, dir, true)["README.md"]
	code, out, errb, logText := runWithLog(t, []string{"list", "archive"}, map[string]string{"TICKETS_DIR": dir})
	if code != 0 {
		t.Fatalf("list archive = %d, want 0; stderr %q", code, errb)
	}
	if !strings.Contains(out, "T-0001") {
		t.Errorf("stdout = %q, want the archived ticket", out)
	}
	if !strings.Contains(errb, "ticket: README.md: ") {
		t.Errorf("human warning missing from stderr: %q", errb)
	}
	wantWarnRecords(t, logText, warnLine("README.md", reason))
}

// TestWarnNew pins both discovery sites of `new` in one run and their order:
// the main List warnings, then the ListArchive warnings.
func TestWarnNew(t *testing.T) {
	dir := verboseFixture(t, "")
	if err := os.WriteFile(filepath.Join(dir, "stray.txt"), []byte("stray"), 0o644); err != nil {
		t.Fatal(err)
	}
	archiveDir := filepath.Join(dir, "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archiveDir, "README.md"), []byte("notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainReason := listReasons(t, dir, false)["stray.txt"]
	archiveReason := listReasons(t, dir, true)["README.md"]
	code, out, errb, logText := runWithLog(t, []string{"new", "второй"}, map[string]string{"TICKETS_DIR": dir})
	if code != 0 {
		t.Fatalf("new = %d, want 0; stderr %q", code, errb)
	}
	if want := filepath.Join(dir, "T-0002-open.md") + "\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	wantWarnRecords(t, logText,
		warnLine("stray.txt", mainReason),
		warnLine("README.md", archiveReason))
}

// TestWarnSet pins the set site: the mutation-path journal-dup notice is
// logged with the reason SetStatus returned.
func TestWarnSet(t *testing.T) {
	dir, reason := dupJournalFixture(t)
	code, out, errb, logText := runWithLog(t, []string{"set", "1", "wip"}, map[string]string{"TICKETS_DIR": dir})
	if code != 0 {
		t.Fatalf("set = %d, want 0; stderr %q", code, errb)
	}
	if want := filepath.Join(dir, "T-0001-wip.md") + "\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if !strings.Contains(errb, "ticket: T-0001-open.md: ") {
		t.Errorf("human warning missing from stderr: %q", errb)
	}
	wantWarnRecords(t, logText, warnLine("T-0001-open.md", reason))
}

// TestWarnArchiveAll pins the archive-all site: the ScanWarningsError carried
// by the archive error is logged before the moved paths and the human report.
func TestWarnArchiveAll(t *testing.T) {
	dir := verboseFixture(t, "done")
	if err := os.WriteFile(filepath.Join(dir, "stray.txt"), []byte("stray"), 0o644); err != nil {
		t.Fatal(err)
	}
	reason := listReasons(t, dir, false)["stray.txt"]
	code, out, errb, logText := runWithLog(t, []string{"archive"}, map[string]string{"TICKETS_DIR": dir})
	if code != 1 {
		t.Fatalf("archive = %d, want 1; stderr %q", code, errb)
	}
	if want := filepath.Join(dir, "archive", "T-0001-done.md") + "\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if !strings.Contains(errb, "ticket: store: scan warnings:") {
		t.Errorf("human scan warning missing from stderr: %q", errb)
	}
	wantWarnRecords(t, logText, warnLine("stray.txt", reason))
}

// TestWarnArchiveMidRun pins the joined-error site: a mid-run collision plus a
// stray scan warning, reproduced through the public surface by pre-seeding the
// collision target (cf. store archive_closed_errors_test.go:145-165). Only the
// warnings are logged; the collision is not a parse warning.
func TestWarnArchiveMidRun(t *testing.T) {
	dir := verboseFixture(t, "done")
	env := map[string]string{"TICKETS_DIR": dir}
	if code, _, stderr := runCLI(t, []string{"new", "второй"}, env); code != 0 {
		t.Fatalf("fixture new = %d, want 0; stderr %q", code, stderr)
	}
	if code, _, stderr := runCLI(t, []string{"set", "2", "done"}, env); code != 0 {
		t.Fatalf("fixture set 2 = %d, want 0; stderr %q", code, stderr)
	}
	archiveDir := filepath.Join(dir, "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archiveDir, "T-0002-done.md"), []byte("FOREIGN"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stray.txt"), []byte("stray"), 0o644); err != nil {
		t.Fatal(err)
	}
	reason := listReasons(t, dir, false)["stray.txt"]
	code, out, errb, logText := runWithLog(t, []string{"archive"}, env)
	if code != 1 {
		t.Fatalf("archive = %d, want 1; stderr %q", code, errb)
	}
	if want := filepath.Join(archiveDir, "T-0001-done.md") + "\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if !strings.Contains(errb, "уже существует") {
		t.Errorf("human collision report missing from stderr: %q", errb)
	}
	wantWarnRecords(t, logText, warnLine("stray.txt", reason))
}

// TestWarnVerboseStderrRoute pins the stderr destination: with --verbose the
// record precedes the human line and appears exactly once.
func TestWarnVerboseStderrRoute(t *testing.T) {
	dir := verboseFixture(t, "")
	if err := os.WriteFile(filepath.Join(dir, "stray.txt"), []byte("stray"), 0o644); err != nil {
		t.Fatal(err)
	}
	reason := listReasons(t, dir, false)["stray.txt"]
	code, _, errb := runCLI(t, []string{"--verbose", "list"}, map[string]string{"TICKETS_DIR": dir})
	if code != 0 {
		t.Fatalf("--verbose list = %d, want 0", code)
	}
	want := warnLine("stray.txt", reason)
	if n := strings.Count(errb, want); n != 1 {
		t.Fatalf("stderr holds %d warn records, want 1: %q", n, errb)
	}
	if strings.Index(errb, want) > strings.Index(errb, "ticket: stray.txt: ") {
		t.Errorf("the record must precede the human line: %q", errb)
	}
}
