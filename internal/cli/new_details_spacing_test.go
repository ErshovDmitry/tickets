package cli_test

// T-0230 / GitHub #56 CLI regressions: ticket new spaces MD022/MD032 blanks
// inside a dirty Details payload (new-only helper), while normalize and
// normalize --dry-run stay byte no-ops on both the spaced new output and a
// hand-written dirty body. Empty-Details stubs are untouched.

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// spacingEnv is normalizeEnv plus English headers so detailsRegion (the raw
// Details slicer from integration_new_details_test.go) applies.
func spacingEnv(t *testing.T) map[string]string {
	t.Helper()
	env := normalizeEnv(t)
	env["TICKET_LANG"] = "en"
	return env
}

// assertNormalizeNoOp pins dry-run silence and byte-identical normalize.
func assertNormalizeNoOp(t *testing.T, env map[string]string, path string) {
	t.Helper()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	stdout, stderr, code := runNormalizeCLI(t, []string{"normalize", "--dry-run"}, env)
	if code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("dry-run = %d, stdout %q, stderr %q; want 0/empty/empty", code, stdout, stderr)
	}
	stdout, stderr, code = runNormalizeCLI(t, []string{"normalize"}, env)
	if code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("normalize = %d, stdout %q, stderr %q; want 0/empty/empty", code, stdout, stderr)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("re-read %s: %v", path, err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("normalize changed bytes:\n%s\n---\n%s", before, after)
	}
}

// TestNewDirtyDetailsSpaced covers the issue repro payloads: a list after
// text (MD032) and a sub-heading between text (MD022), plus the combined
// heading+list case that must get exactly one blank line, not two.
func TestNewDirtyDetailsSpaced(t *testing.T) {
	tests := []struct{ name, payload, want string }{
		{"list after text", "Факт:\n- первый\n- второй", "Факт:\n\n- первый\n- второй"},
		{"heading between text", "Тело:\n## Подзаголовок\nтекст", "Тело:\n\n## Подзаголовок\n\nтекст"},
		{"heading then list one blank", "## H\n- a", "## H\n\n- a"},
		{"ordered list after text", "text\n1. a\n2. b", "text\n\n1. a\n2. b"},
		{"crlf list", "text\r\n- a\r\n", "text\r\n\r\n- a"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := spacingEnv(t)
			path := seedNormalizeTicket(t, env, "Spacing", "-d", tc.payload)
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			if strings.Contains(tc.payload, "\r\n") {
				// detailsRegion's LF slicer cannot trim a CRLF separator;
				// assert the spaced payload bytes directly instead.
				if !bytes.Contains(raw, []byte(tc.want+"\r\n")) {
					t.Fatalf("CRLF spacing missing from:\n%q", raw)
				}
				return // CRLF normalize behavior is out of scope
			}
			if got := detailsRegion(t, raw); got != tc.want {
				t.Fatalf("Details region =\n%q\nwant\n%q", got, tc.want)
			}
			assertNormalizeNoOp(t, env, path)
		})
	}
}

// TestNewCleanDetailsUnchanged: an already-clean payload keeps its bytes.
func TestNewCleanDetailsUnchanged(t *testing.T) {
	env := spacingEnv(t)
	payload := "text\n\n- a\n\n## Sub\n\ntext"
	path := seedNormalizeTicket(t, env, "Clean", "-d", payload)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := detailsRegion(t, raw); got != payload {
		t.Fatalf("Details region = %q, want %q", got, payload)
	}
	assertNormalizeNoOp(t, env, path)
}

// TestNewEmptyDetailsStubUntouched: the localized empty-Details format block
// is emitted as before and normalize leaves it alone.
func TestNewEmptyDetailsStubUntouched(t *testing.T) {
	env := spacingEnv(t)
	path := seedNormalizeTicket(t, env, "Stub")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(raw), "## Details\n\n") {
		t.Fatalf("Details header block missing:\n%s", raw)
	}
	assertNormalizeNoOp(t, env, path)
}

// TestNormalizeDirtyBodyNoOp pins the unchanged normalize contract: a
// hand-written dirty body (blank lines removed inside Details) is NOT fixed
// by normalize — dry-run prints nothing and the bytes stay identical.
func TestNormalizeDirtyBodyNoOp(t *testing.T) {
	env := spacingEnv(t)
	path := seedNormalizeTicket(t, env, "Dirty body", "-d", "text\n\n- a\n\n## Sub\n\ntext")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	dirty := strings.Replace(string(raw), "text\n\n- a\n\n## Sub\n\ntext", "text\n- a\n## Sub\ntext", 1)
	if dirty == string(raw) {
		t.Fatal("fixture did not produce a dirty body")
	}
	if err := os.WriteFile(path, []byte(dirty), 0o644); err != nil {
		t.Fatalf("write dirty: %v", err)
	}
	assertNormalizeNoOp(t, env, path)
}
