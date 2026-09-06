package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCmdArchiveAllWithScanWarnings pins T-0051: `archive` without an
// argument moves the valid done ticket even when the scan reports
// warnings (a stray non-.md file): the moved path is echoed on stdout
// before the failure, the ScanWarningsError is rendered through the
// default archiveError branch («ticket: store: scan warnings: …»,
// single «ticket: » prefix) and the exit code is 1.
func TestCmdArchiveAllWithScanWarnings(t *testing.T) {
	env, dir := newTicketDir(t)
	runSet(t, env, "1", "done")
	stray := filepath.Join(dir, "stray.txt")
	if err := os.WriteFile(stray, []byte("not a ticket"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runArchive(env)
	if code != 1 {
		t.Fatalf("archive = %d, want 1", code)
	}
	want := filepath.Join(dir, "archive", "T-0001-done.md") + "\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	for _, frag := range []string{"ticket: store: scan warnings:", "stray.txt"} {
		if !strings.Contains(stderr, frag) {
			t.Errorf("stderr = %q, missing %q", stderr, frag)
		}
	}
	if strings.Contains(stderr, "ticket: ticket:") {
		t.Errorf("stderr = %q, doubled «ticket: » prefix", stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "archive", "T-0001-done.md")); err != nil {
		t.Errorf("archived file missing: %v", err)
	}
	if _, err := os.Stat(stray); err != nil {
		t.Errorf("stray file must remain in main dir: %v", err)
	}
}
