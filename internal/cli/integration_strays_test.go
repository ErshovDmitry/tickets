package cli_test

// T-0166 end-to-end: stray lines (unrecognized head content and an
// extended Summary body) survive a 'ticket set' rewrite and a 'ticket
// normalize' run, and a second normalize is a no-op. Everything runs in a
// TempDir sandbox — the production tickets/ directory is never touched.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// injectStrays inserts head-strays (between the metadata block and the
// Summary header), a stray right after the Created meta line, and a stray
// line into the Summary body. Each anchor must match exactly once.
func injectStrays(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	step1 := strings.Replace(string(data),
		"\n\n## Summary (Кратко)\n",
		"\n\nstray alpha\n## Orphan header\norphan body\nstray beta\n\n## Summary (Кратко)\n", 1)
	if step1 == string(data) {
		t.Fatal("anchor 1 (Summary header) not found")
	}
	step2 := strings.Replace(step1,
		"\n- Project (Проект):",
		"\nstray after created\n- Project (Проект):", 1)
	if step2 == step1 {
		t.Fatal("anchor 2 (Project meta line) not found")
	}
	edited := strings.Replace(step2,
		"\n\n## Details (Подробности)",
		"\nsummary stray line\n\n## Details (Подробности)", 1)
	if edited == step2 {
		t.Fatal("anchor 3 (Details header) not found")
	}
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatalf("write strays: %v", err)
	}
	return []byte(edited)
}

func newStrayTicket(t *testing.T) (tickets, cwd, path string) {
	t.Helper()
	requireBin(t)
	tickets = t.TempDir()
	cwd = t.TempDir()
	out, stderr, code := runBin(t, ticketBin, cwd, tickets, "new", "Strays cycle", "-d", "детали цикла")
	if code != 0 {
		t.Fatalf("new: code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(out, "T-0001-open.md") {
		t.Fatalf("new stdout %q does not mention the created file", out)
	}
	path = filepath.Join(mustEval(t, tickets), "T-0001-open.md")
	return tickets, cwd, path
}

// TestStraysSurviveSetCycle pins T-0166 AC1: every stray line survives a
// 'ticket set' rewrite with its relative order inside the HeadStray block.
func TestStraysSurviveSetCycle(t *testing.T) {
	tickets, cwd, path := newStrayTicket(t)
	injectStrays(t, path)

	_, stderr, code := runBin(t, ticketBin, cwd, tickets, "set", "1", "wip", "в работу")
	if code != 0 {
		t.Fatalf("set wip: code=%d stderr=%q", code, stderr)
	}
	wipPath := filepath.Join(mustEval(t, tickets), "T-0001-wip.md")
	wip, err := os.ReadFile(wipPath)
	if err != nil {
		t.Fatalf("read wip ticket: %v", err)
	}
	out := string(wip)
	for _, stray := range []string{"stray after created", "stray alpha", "## Orphan header", "orphan body", "stray beta", "summary stray line"} {
		if !strings.Contains(out, stray) {
			t.Errorf("stray %q lost after set:\n%s", stray, out)
		}
	}
	// Relative order inside the HeadStray block is preserved, and the
	// block stays anchored between the metadata block and the Summary
	// header (the absolute order against the meta lines is canonicalized
	// by Render and is not asserted).
	order := []string{"stray after created", "stray alpha", "## Orphan header", "orphan body", "stray beta", "## Summary (Кратко)"}
	last := -1
	for _, s := range order {
		at := strings.Index(out, s)
		if at < 0 || at < last {
			t.Errorf("stray block order broken at %q (index %d, previous %d):\n%s", s, at, last, out)
		}
		last = at
	}
	// The Summary stray stays inside the Summary block.
	summaryAt := strings.Index(out, "## Summary (Кратко)")
	detailsAt := strings.Index(out, "## Details (Подробности)")
	strayAt := strings.Index(out, "summary stray line")
	if !(summaryAt >= 0 && summaryAt < strayAt && strayAt < detailsAt) {
		t.Errorf("summary stray not anchored between Summary and Details:\n%s", out)
	}
}

// TestStraysSurviveNormalizeIdempotent pins T-0166 AC2: after a set cycle
// the file is canonical, so normalize is a no-op — the second run prints
// nothing and the bytes are identical.
func TestStraysSurviveNormalizeIdempotent(t *testing.T) {
	tickets, cwd, path := newStrayTicket(t)
	injectStrays(t, path)
	if _, _, code := runBin(t, ticketBin, cwd, tickets, "set", "1", "wip", "в работу"); code != 0 {
		t.Fatalf("set wip: code=%d", code)
	}

	stdout, stderr, code := runBin(t, ticketBin, cwd, tickets, "normalize")
	if code != 0 {
		t.Fatalf("first normalize: code=%d stderr=%q", code, stderr)
	}
	_ = stdout // may list the file or be empty; only the second run is pinned
	first, err := os.ReadFile(filepath.Join(mustEval(t, tickets), "T-0001-wip.md"))
	if err != nil {
		t.Fatalf("read after first normalize: %v", err)
	}
	for _, stray := range []string{"stray after created", "stray alpha", "## Orphan header", "orphan body", "stray beta", "summary stray line"} {
		if !strings.Contains(string(first), stray) {
			t.Errorf("stray %q lost during normalize:\n%s", stray, first)
		}
	}

	stdout, stderr, code = runBin(t, ticketBin, cwd, tickets, "normalize")
	if code != 0 {
		t.Fatalf("second normalize: code=%d stderr=%q", code, stderr)
	}
	if stdout != "" {
		t.Errorf("second normalize stdout = %q, want empty (no-op)", stdout)
	}
	second, err := os.ReadFile(filepath.Join(mustEval(t, tickets), "T-0001-wip.md"))
	if err != nil {
		t.Fatalf("read after second normalize: %v", err)
	}
	if !bytes.Equal(second, first) {
		t.Errorf("second normalize rewrote the file:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}
