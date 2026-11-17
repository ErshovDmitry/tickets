package cli

// T-0191: a flag in the <number> or <new-file> positional slot of
// "ticket recover" is a usage error reported via the command-agnostic
// ErrUnknownFlagList (precedent T-0189 delete). recover must not answer
// with the set-specific "unknown flag for set command" text.

import (
	"strings"
	"testing"

	"ticket/internal/domain"
)

// TestRecoverUnknownFlagNumberSlot: a flag in the <number> slot is rejected
// before anything is created or parsed.
func TestRecoverUnknownFlagNumberSlot(t *testing.T) {
	dir := resolvedTempDir(t)
	before := treeState(t, dir)
	so, se, code := runRecoverCLI(t, dir, "recover", "--bogus", "x")
	if code != exitError {
		t.Fatalf("code=%d want %d (stderr=%q)", code, exitError, se)
	}
	if !strings.Contains(se, domain.ErrUnknownFlagList(domain.LangRU, "--bogus")) {
		t.Errorf("stderr=%q, want ErrUnknownFlagList for --bogus", se)
	}
	if strings.Contains(se, "set") {
		t.Errorf("stderr=%q must not mention the set command", se)
	}
	assertSameTree(t, dir, before)
	if so != "" {
		t.Errorf("stdout=%q, want empty", so)
	}
}

// TestRecoverUnknownFlagFileSlot: a flag in the <new-file> slot is rejected
// the same way, even when the <number> slot holds a valid number.
func TestRecoverUnknownFlagFileSlot(t *testing.T) {
	dir := resolvedTempDir(t)
	before := treeState(t, dir)
	_, se, code := runRecoverCLI(t, dir, "recover", "1", "--bogus")
	if code != exitError {
		t.Fatalf("code=%d want %d (stderr=%q)", code, exitError, se)
	}
	if !strings.Contains(se, domain.ErrUnknownFlagList(domain.LangRU, "--bogus")) {
		t.Errorf("stderr=%q, want ErrUnknownFlagList for --bogus", se)
	}
	assertSameTree(t, dir, before)
}

// TestRecoverUnknownFlagFirstOffender: with two flags, the report names the
// first one.
func TestRecoverUnknownFlagFirstOffender(t *testing.T) {
	dir := resolvedTempDir(t)
	_, se, code := runRecoverCLI(t, dir, "recover", "--a", "--b")
	if code != exitError {
		t.Fatalf("code=%d want %d (stderr=%q)", code, exitError, se)
	}
	if !strings.Contains(se, domain.ErrUnknownFlagList(domain.LangRU, "--a")) {
		t.Errorf("stderr=%q, want report of the first offender --a", se)
	}
	if strings.Contains(se, "--b") {
		t.Errorf("stderr=%q must report only the first offender", se)
	}
}

// TestRecoverUnknownFlagEN: the EN locale emits the English list form with
// no Cyrillic anywhere.
func TestRecoverUnknownFlagEN(t *testing.T) {
	dir := resolvedTempDir(t)
	_, se, code := runRecoverLangCLI(t, dir, "en", "recover", "1", "--bogus")
	if code != exitError {
		t.Fatalf("code=%d want %d (stderr=%q)", code, exitError, se)
	}
	want := domain.ErrUnknownFlagList(domain.LangEN, "--bogus")
	if se != want+"\n" {
		t.Errorf("stderr=%q, want %q", se, want+"\n")
	}
	if hasCyrillic(se) {
		t.Errorf("stderr=%q contains Cyrillic", se)
	}
}
