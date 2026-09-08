package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/cli"
)

// TestSetFlagLikeStatusRejected pins T-0068: a flag-like argument in the
// status position (`set 1 -P`) is rejected with the localized unknown-flag
// message naming -P, exit 1, before any store mutation.
func TestSetFlagLikeStatusRejected(t *testing.T) {
	env := map[string]string{"TICKETS_DIR": t.TempDir()}

	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"set", "1", "-P"}, env, &stdout, &stderr); code != 1 {
		t.Fatalf("Run(set 1 -P) = %d, want 1; stderr: %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	got := stderr.String()
	if !strings.Contains(got, "неизвестный флаг") {
		t.Errorf("stderr = %q, want «неизвестный флаг»", got)
	}
	if !strings.Contains(got, "-P") {
		t.Errorf("stderr = %q, want offending flag -P", got)
	}
}

// TestSetFlagInCommentJournaled pins T-0068 decision: a flag-like token
// in the comment position (args[2:]) is free text, not a flag — `set 1
// wip -P x` exits 0 and the whole «-P x» lands in the journal.
func TestSetFlagInCommentJournaled(t *testing.T) {
	dir := mustEval(t, t.TempDir())
	env := map[string]string{"TICKETS_DIR": dir}
	createTestTicket(t, dir)

	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"set", "1", "wip", "-P", "x"}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(set 1 wip -P x) = %d, want 0; stderr: %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
	body, err := os.ReadFile(filepath.Join(dir, "T-0001-wip.md"))
	if err != nil {
		t.Fatalf("read new file: %v", err)
	}
	if !bytes.Contains(body, []byte("-P x")) {
		t.Errorf("body missing comment «-P x»:\n%s", body)
	}
}

// TestSetGlobalFlagLikeCommentJournaled pins T-0068 decision: `set 1 wip
// c -C /path` — the -C appears after the subcommand, so it is part of the
// comment (extractGlobalDir only strips a LEADING -C), and the whole
// «c -C /path» lands in the journal.
func TestSetGlobalFlagLikeCommentJournaled(t *testing.T) {
	dir := mustEval(t, t.TempDir())
	env := map[string]string{"TICKETS_DIR": dir}
	createTestTicket(t, dir)

	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"set", "1", "wip", "c", "-C", "/path"}, env, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(set 1 wip c -C /path) = %d, want 0; stderr: %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
	body, err := os.ReadFile(filepath.Join(dir, "T-0001-wip.md"))
	if err != nil {
		t.Fatalf("read new file: %v", err)
	}
	if !bytes.Contains(body, []byte("c -C /path")) {
		t.Errorf("body missing comment «c -C /path»:\n%s", body)
	}
}
