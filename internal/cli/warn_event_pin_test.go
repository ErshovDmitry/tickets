package cli

// Byte-level pin of the warn event name (T-0123). warn_parse_test.go renders
// its expectations through warnLine, which is built from the warnEvent
// constant (log.go), so renaming the constant silently keeps those tests
// green. This test spells the event name out as literal bytes, so any change
// to the constant that is not mirrored here fails.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestWarnEventNamePinnedBytes asserts that the warn record for a rejected
// ticket file name is exactly the documented bytes: a literal event name, the
// quoted path and the store-assigned reason.
func TestWarnEventNamePinnedBytes(t *testing.T) {
	dir := verboseFixture(t, "")
	if err := os.WriteFile(filepath.Join(dir, "stray.txt"), []byte("stray"), 0o644); err != nil {
		t.Fatal(err)
	}
	reason := listReasons(t, dir, false)["stray.txt"]
	if reason == "" {
		t.Fatalf("the store reported no warning for stray.txt")
	}

	code, out, errb, logText := runWithLog(t, []string{"list"}, map[string]string{"TICKETS_DIR": dir})
	if code != 0 {
		t.Fatalf("list = %d, want 0; stderr %q", code, errb)
	}
	if !strings.Contains(out, "T-0001") {
		t.Errorf("stdout = %q, want the open ticket", out)
	}

	got := warnLines(logText)
	if len(got) != 1 {
		t.Fatalf("warn records = %q, want exactly one (log %q)", got, logText)
	}
	if want := "ticket: warn: store.read.warning path=\"stray.txt\" reason=" + strconv.Quote(reason) + "\n"; got[0] != want {
		t.Errorf("warn record = %q, want %q", got[0], want)
	}
}
