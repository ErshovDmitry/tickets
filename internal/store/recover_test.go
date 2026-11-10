package store

// T-0159 A1: focused tests for Store.Recover — the store half of recovery
// from an interrupted status commit. Reuses the existing test helpers
// (newStore/seedTicketOne/snapshotTree/assertAmbiguous/swapRemove hooks).

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"ticket/internal/domain"
)

// crashPair creates the real interrupted-commit state: a normal SetStatus
// whose old-file removal fails, leaving old+new behind with the honest
// From=open→To=next journal in the new file.
func crashPair(t *testing.T, s *Store, dir string, next domain.Status) (oldPath, newPath string) {
	t.Helper()
	seedTicketOne(t, s, dir)
	oldPath = filepath.Join(dir, "T-0001-open.md")
	newPath = filepath.Join(dir, domain.Filename(1, next))
	orig := removeFile
	removeFile = func(p string) error { return fmt.Errorf("boom(%s)", filepath.Base(p)) }
	// t.Cleanup is the safety net for a t.Fatal between swap and restore;
	// the restored flag keeps the two restore paths from double-restoring.
	restored := false
	t.Cleanup(func() {
		if !restored {
			removeFile = orig
		}
	})
	_, _, err := s.SetStatus(1, next, "tester", "transition")
	removeFile = orig
	restored = true
	if err == nil {
		t.Fatal("expected the failed old-file removal to surface an error")
	}
	return oldPath, newPath
}

func recoverStateErr(t *testing.T, err error, wantReason string) *RecoveryStateError {
	t.Helper()
	var rse *RecoveryStateError
	if !errors.As(err, &rse) {
		t.Fatalf("want *RecoveryStateError, got %v", err)
	}
	if rse.Reason != wantReason {
		t.Fatalf("reason %q, want %q (err: %v)", rse.Reason, wantReason, err)
	}
	return rse
}

func unchangedAfterRefusal(t *testing.T, s *Store, dir, keep, wantReason string) *RecoveryStateError {
	t.Helper()
	before := snapshotTree(t, dir)
	_, err := s.Recover(1, keep)
	rse := recoverStateErr(t, err, wantReason)
	if after := snapshotTree(t, dir); !reflect.DeepEqual(before, after) {
		t.Errorf("recover(%q) mutated the tree\nbefore: %v\nafter:  %v", keep, before, after)
	}
	return rse
}

func TestRecover_CrashPair_KeepsNew(t *testing.T) {
	s, dir := newStore(t)
	oldPath, newPath := crashPair(t, s, dir, domain.StatusDone)
	newBody, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read new: %v", err)
	}
	kept, err := s.Recover(1, "T-0001-done.md")
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if kept != newPath {
		t.Errorf("kept %q, want %q", kept, newPath)
	}
	if _, err := os.Stat(oldPath); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("old file must be removed, stat err: %v", err)
	}
	assertTicketFileNames(t, dir, "T-0001-done.md")
	after := mustRead(t, dir, 1)
	if !bytes.Equal(after, newBody) {
		t.Errorf("kept file changed by recovery: %q", after)
	}
	assertJournalTransition(t, string(after), "open", "done", "transition", "tester")
	assertNoTmpFiles(t, dir)
	// Idempotent repeat: exactly the kept file remains — a no-op success.
	again, err := s.Recover(1, "T-0001-done.md")
	if err != nil || again != kept {
		t.Errorf("repeat Recover = (%q, %v), want (%q, nil)", again, err, kept)
	}
}

func TestRecover_CrossDir_ReopenPair(t *testing.T) {
	s, dir := newStore(t)
	seedTicketOne(t, s, dir)
	if _, _, err := s.SetStatus(1, domain.StatusDone, "tester", "close"); err != nil {
		t.Fatalf("SetStatus done: %v", err)
	}
	if _, err := s.Archive(1, "tester"); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	// Reopen writes the new file into main; the crash leaves the archived
	// old file behind. Old/new roles come from the journal, not the path.
	orig := removeFile
	removeFile = func(p string) error { return fmt.Errorf("boom(%s)", filepath.Base(p)) }
	_, _, rerr := s.SetStatus(1, domain.StatusOpen, "tester", "reopen")
	removeFile = orig
	if rerr == nil {
		t.Fatal("expected the failed old-file removal to surface an error")
	}
	newPath := filepath.Join(dir, "T-0001-open.md")
	oldPath := filepath.Join(dir, "archive", "T-0001-done.md")
	newBody, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read new: %v", err)
	}
	kept, err := s.Recover(1, "T-0001-open.md")
	if err != nil || kept != newPath {
		t.Fatalf("Recover = (%q, %v), want (%q, nil)", kept, err, newPath)
	}
	if _, err := os.Stat(oldPath); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("archived old must be removed, stat err: %v", err)
	}
	assertTicketFileNames(t, dir, "T-0001-open.md")
	if matches, _ := filepath.Glob(filepath.Join(dir, "archive", "T-*.md")); len(matches) != 0 {
		t.Errorf("archive must hold no ticket files, got %v", matches)
	}
	after := mustRead(t, dir, 1)
	if !bytes.Equal(after, newBody) {
		t.Errorf("kept file changed by recovery")
	}
	if !strings.Contains(string(after), "статус: done → open · reopen (tester)") {
		t.Errorf("kept journal must hold the reopen transition:\n%s", after)
	}
}

func TestRecover_RefusalsLeaveTreeUnchanged(t *testing.T) {
	t.Run("keep-old", func(t *testing.T) {
		s, dir := newStore(t)
		crashPair(t, s, dir, domain.StatusDone)
		rse := unchangedAfterRefusal(t, s, dir, "T-0001-open.md", RecoverReasonKeepOld)
		if !strings.Contains(rse.Error(), "T-0001-done.md") {
			t.Errorf("keep-old hint must name the new file: %v", rse)
		}
	})
	t.Run("divergent-unknown-bytes", func(t *testing.T) {
		s, dir := newStore(t)
		_, newPath := crashPair(t, s, dir, domain.StatusDone)
		body, _ := os.ReadFile(newPath)
		cloneTicket(t, newPath, append(body, []byte("\n<!-- foreign -->\n")...))
		unchangedAfterRefusal(t, s, dir, "T-0001-done.md", RecoverReasonDivergent)
	})
	t.Run("same-status", func(t *testing.T) {
		s, dir := newStore(t)
		body := seedTicketOne(t, s, dir)
		cloneTicket(t, filepath.Join(dir, "archive", "T-0001-open.md"), body)
		unchangedAfterRefusal(t, s, dir, "T-0001-open.md", RecoverReasonSameStatus)
	})
	t.Run("more-than-two", func(t *testing.T) {
		s, dir := newStore(t)
		_, newPath := crashPair(t, s, dir, domain.StatusDone)
		body, _ := os.ReadFile(newPath)
		wip := filepath.Join(dir, "T-0001-wip.md")
		cloneTicket(t, wip, body)
		before := snapshotTree(t, dir)
		_, err := s.Recover(1, "T-0001-done.md")
		assertAmbiguous(t, err, 1, []string{newPath, filepath.Join(dir, "T-0001-open.md"), wip})
		if after := snapshotTree(t, dir); !reflect.DeepEqual(before, after) {
			t.Errorf("recover mutated the tree on >2 files")
		}
	})
	t.Run("single-mismatch", func(t *testing.T) {
		s, dir := newStore(t)
		_, newPath := crashPair(t, s, dir, domain.StatusDone)
		if err := os.Remove(newPath); err != nil {
			t.Fatalf("remove new: %v", err)
		}
		unchangedAfterRefusal(t, s, dir, "T-0001-done.md", RecoverReasonSingleMismatch)
	})
	t.Run("symlink-new", func(t *testing.T) {
		// The scanner never lists symlinks, so the new name is invisible:
		// the single visible file is not the selected new one.
		s, dir := newStore(t)
		oldPath, newPath := crashPair(t, s, dir, domain.StatusDone)
		if err := os.Remove(newPath); err != nil {
			t.Fatalf("remove new: %v", err)
		}
		if err := os.Symlink(oldPath, newPath); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		unchangedAfterRefusal(t, s, dir, "T-0001-done.md", RecoverReasonSingleMismatch)
	})
}

func TestRecover_NotFound(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.Recover(42, "T-0042-done.md"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Recover on empty store = %v, want ErrNotFound", err)
	}
}

func TestRecover_RemoveFault_ThenRetry(t *testing.T) {
	s, dir := newStore(t)
	crashPair(t, s, dir, domain.StatusDone)
	orig := removeFile
	calls := 0
	removeFile = func(p string) error {
		calls++
		return fmt.Errorf("denied(%s)", filepath.Base(p))
	}
	t.Cleanup(func() { removeFile = orig })
	_, err := s.Recover(1, "T-0001-done.md")
	if err == nil {
		t.Fatal("expected the remove fault to surface")
	}
	for _, want := range []string{"denied(T-0001-open.md)", "T-0001-done.md"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q must mention %q", err, want)
		}
	}
	if calls != 1 {
		t.Errorf("removeFile called %d times, want exactly 1 (only the old file)", calls)
	}
	assertTicketFileNames(t, dir, "T-0001-open.md", "T-0001-done.md")
	assertNoTmpFiles(t, dir)
	// Fault healed: the retry recovers the same pair.
	removeFile = orig
	kept, err := s.Recover(1, "T-0001-done.md")
	if err != nil {
		t.Fatalf("retry Recover: %v", err)
	}
	assertTicketFileNames(t, dir, "T-0001-done.md")
	if filepath.Base(kept) != "T-0001-done.md" {
		t.Errorf("kept %q", kept)
	}
}

func TestRecover_ConcurrentRecovers_Idempotent(t *testing.T) {
	s, dir := newStore(t)
	_, newPath := crashPair(t, s, dir, domain.StatusDone)
	newBody, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read new: %v", err)
	}
	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Go(func() {
			_, rerr := s.Recover(1, "T-0001-done.md")
			errs <- rerr
		})
	}
	wg.Wait()
	close(errs)
	for rerr := range errs {
		if rerr != nil {
			t.Errorf("concurrent Recover: %v", rerr)
		}
	}
	assertTicketFileNames(t, dir, "T-0001-done.md")
	if !bytes.Equal(mustRead(t, dir, 1), newBody) {
		t.Errorf("kept file changed under concurrent recovery")
	}
	// No arbitrary picks afterwards: a normal set works on the survivor.
	if _, _, err := s.SetStatus(1, domain.StatusWip, "tester", "resume"); err != nil {
		t.Fatalf("SetStatus after recovery: %v", err)
	}
	assertTicketFileNames(t, dir, "T-0001-wip.md")
}
