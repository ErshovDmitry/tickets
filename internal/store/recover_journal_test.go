package store

// T-0159 A2: follow-up focused tests for Store.Recover. They live in their
// own file because recover_test.go already uses the full 300-line budget.
// Helpers are package-shared (crashPair, snapshotTree, unchangedAfterRefusal,
// recoverStateErr, mustRead, assertTicketFileNames, ...).

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"ticket/internal/domain"
)

// extraJournalRecord is a well-formed journal line that a normal SetStatus
// would have appended for a further transition.
const extraJournalRecord = "- 2026-01-01 00:00 — статус: done → closed (tester)\n"

// TestRecover_NewSortsAfterOld_JournalWins pins the direction rule: the
// open→wip pair leaves T-0001-wip.md sorting AFTER T-0001-open.md, so a
// positional heuristic ("first/last file wins") would pick the wrong side.
// Only the journal can decide which file is the crash-pair successor.
func TestRecover_NewSortsAfterOld_JournalWins(t *testing.T) {
	s, dir := newStore(t)
	oldPath, newPath := crashPair(t, s, dir, domain.StatusWip)
	if !(filepath.Base(newPath) > filepath.Base(oldPath)) {
		t.Fatalf("pair must sort new-after-old, got %q and %q", newPath, oldPath)
	}
	newBody, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read new: %v", err)
	}
	// Selecting the stale old file first is a typed refusal, and the hint
	// names the journal-proven successor.
	rse := unchangedAfterRefusal(t, s, dir, "T-0001-open.md", RecoverReasonKeepOld)
	if !strings.Contains(rse.Error(), "T-0001-wip.md") {
		t.Errorf("keep-old hint must name the new file: %v", rse)
	}
	kept, err := s.Recover(1, "T-0001-wip.md")
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if kept != newPath {
		t.Errorf("kept %q, want %q", kept, newPath)
	}
	assertTicketFileNames(t, dir, "T-0001-wip.md")
	if after := mustRead(t, dir, 1); !bytes.Equal(after, newBody) {
		t.Errorf("kept file changed by recovery: %q", after)
	}
	assertJournalTransition(t, string(mustRead(t, dir, 1)), "open", "wip", "transition", "tester")
	assertNoTmpFiles(t, dir)
}

// TestRecover_ExtraJournalEntry_Divergent proves the signature is exact:
// one transition too many means divergent contents, and the refusal leaves
// a byte-identical tree (snapshotTree hashes every file and directory).
func TestRecover_ExtraJournalEntry_Divergent(t *testing.T) {
	s, dir := newStore(t)
	_, newPath := crashPair(t, s, dir, domain.StatusDone)
	body, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read new: %v", err)
	}
	grown := append(bytes.TrimRight(body, "\n"), '\n')
	grown = append(grown, []byte(extraJournalRecord)...)
	if werr := os.WriteFile(newPath, grown, 0o644); werr != nil {
		t.Fatalf("write grown new: %v", werr)
	}
	unchangedAfterRefusal(t, s, dir, "T-0001-done.md", RecoverReasonDivergent)
}

// TestRecover_RemoveFault_NoIntermediateWrites re-runs the fault path with
// a full-tree snapshot on both sides: a failing old-file removal must not
// leave tmp files, rewritten bodies or partially moved entries behind.
func TestRecover_RemoveFault_NoIntermediateWrites(t *testing.T) {
	s, dir := newStore(t)
	crashPair(t, s, dir, domain.StatusDone)
	before := snapshotTree(t, dir)
	orig := removeFile
	removeFile = func(p string) error { return fmt.Errorf("denied(%s)", filepath.Base(p)) }
	t.Cleanup(func() { removeFile = orig })
	_, err := s.Recover(1, "T-0001-done.md")
	if err == nil {
		t.Fatal("expected the remove fault to surface")
	}
	if !strings.Contains(err.Error(), "denied(T-0001-open.md)") {
		t.Errorf("error %q must mention the failed old-file removal", err)
	}
	if after := snapshotTree(t, dir); !reflect.DeepEqual(before, after) {
		t.Errorf("failed removal wrote intermediate state\nbefore: %v\nafter:  %v", before, after)
	}
	assertTicketFileNames(t, dir, "T-0001-open.md", "T-0001-done.md")
}

// TestRecover_BrokenBody_RefusedUnchanged covers unparseable bodies on
// either side of the pair. The crash signature cannot be proven, so the
// read error surfaces as a plain refusal (never as a typed state guess),
// and the snapshot proves nothing on disk moved.
func TestRecover_BrokenBody_RefusedUnchanged(t *testing.T) {
	t.Run("foreign-new-body", func(t *testing.T) {
		s, dir := newStore(t)
		_, newPath := crashPair(t, s, dir, domain.StatusDone)
		if err := os.WriteFile(newPath, []byte("FOREIGN-ZERO-BODY"), 0o644); err != nil {
			t.Fatalf("write foreign new: %v", err)
		}
		refusedUnchanged(t, s, dir, "T-0001-done.md")
		assertFileContent(t, newPath, "FOREIGN-ZERO-BODY")
	})
	t.Run("truncated-old-body", func(t *testing.T) {
		s, dir := newStore(t)
		oldPath, _ := crashPair(t, s, dir, domain.StatusDone)
		body, err := os.ReadFile(oldPath)
		if err != nil {
			t.Fatalf("read old: %v", err)
		}
		if werr := os.WriteFile(oldPath, body[:len(body)/2], 0o644); werr != nil {
			t.Fatalf("truncate old: %v", werr)
		}
		// A truncated body still parses, so the signature simply does not
		// match: that is the typed divergent refusal.
		unchangedAfterRefusal(t, s, dir, "T-0001-done.md", RecoverReasonDivergent)
	})
}

// TestRecover_OldEndsWithSameTransition_Divergent covers the replay case:
// the old file's journal already ends with the very From=old→To=new record
// the interrupted commit was about to append (a completed transition
// rolled back and replayed). Length, prefix and render checks all pass,
// but the duplicate terminal record marks the pair divergent, and the
// refusal leaves a byte-identical tree.
func TestRecover_OldEndsWithSameTransition_Divergent(t *testing.T) {
	s, dir := newStore(t)
	_, newPath := crashPair(t, s, dir, domain.StatusDone)
	doneBody, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read new: %v", err)
	}
	// Old was already moved to done once and rolled back, so its journal
	// ends with open→done; "new" replays that same record a second time.
	cloneTicket(t, filepath.Join(dir, "T-0001-open.md"), doneBody)
	dup := append(bytes.TrimRight(doneBody, "\n"), '\n')
	dup = append(dup, []byte("- 2026-01-01 00:00 — статус: open → done (tester)\n")...)
	cloneTicket(t, newPath, dup)
	unchangedAfterRefusal(t, s, dir, "T-0001-done.md", RecoverReasonDivergent)
}

func TestRecover_InvalidSelection(t *testing.T) {
	for _, tc := range []struct {
		name string
		n    int
		keep string
	}{
		{"separator", 1, "archive/T-0001-done.md"},
		{"number-mismatch", 1, "T-0002-done.md"},
		{"unknown-status", 1, "T-0001-bogus.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, dir := newStore(t)
			crashPair(t, s, dir, domain.StatusDone)
			before := snapshotTree(t, dir)
			if _, err := s.Recover(tc.n, tc.keep); err == nil {
				t.Fatal("expected a selection error")
			} else {
				if rse, ok := errors.AsType[*RecoveryStateError](err); ok {
					t.Errorf("selection misuse must not be a typed state refusal: %v", rse)
				}
			}
			if after := snapshotTree(t, dir); !reflect.DeepEqual(before, after) {
				t.Errorf("invalid selection mutated the tree")
			}
		})
	}
}

// refusedUnchanged requires a plain refusal that names wantFile and leaves
// the tree byte-identical.
func refusedUnchanged(t *testing.T, s *Store, dir, wantFile string) {
	t.Helper()
	before := snapshotTree(t, dir)
	_, err := s.Recover(1, "T-0001-done.md")
	if err == nil {
		t.Fatal("expected a refusal on an unparseable body")
	}
	if rse, ok := errors.AsType[*RecoveryStateError](err); ok {
		t.Errorf("an unparseable body must not be reported as a typed state refusal: %v", rse)
	}
	if !strings.Contains(err.Error(), wantFile) {
		t.Errorf("error %q must name %s", err, wantFile)
	}
	if after := snapshotTree(t, dir); !reflect.DeepEqual(before, after) {
		t.Errorf("refusal on a broken body changed the tree")
	}
}

// TestRecover_KeepsNewInodeAndModTime pins the "never rewrites the new
// file" contract. Instead of sleeping for a fresh timestamp tick (flaky on
// coarse-mtime filesystems) the new file is stamped with a fixed past time
// via os.Chtimes: any rewrite would install mtime "now", years away.
func TestRecover_KeepsNewInodeAndModTime(t *testing.T) {
	s, dir := newStore(t)
	_, newPath := crashPair(t, s, dir, domain.StatusDone)
	stamp := time.Unix(1_000_000_000, 0)
	if err := os.Chtimes(newPath, stamp, stamp); err != nil {
		t.Fatalf("stamp new file: %v", err)
	}
	before, err := os.Stat(newPath)
	if err != nil {
		t.Fatalf("stat before: %v", err)
	}
	if _, err := s.Recover(1, "T-0001-done.md"); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	after, err := os.Stat(newPath)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if !os.SameFile(before, after) {
		t.Errorf("recovery replaced the new file (inode changed): %q", newPath)
	}
	if !after.ModTime().Equal(stamp) {
		t.Errorf("recovery rewrote the new file: mtime %v, want %v", after.ModTime(), stamp)
	}
}
