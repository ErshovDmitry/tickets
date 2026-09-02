package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"ticket/internal/domain"
)

// TestSetStatus_RemoveOldENOENT_IsSuccess pins the invariant: if the old
// file vanishes after the target rename succeeded (removed by a lockless
// external agent), remove-old fails with ENOENT — yet the success state
// "ONLY the new file remains" already holds. SetStatus must return nil
// and must NOT roll back (a rollback would delete the only remaining
// ticket file).
func TestSetStatus_RemoveOldENOENT_IsSuccess(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(dir, "T-0001-open.md")
	target := filepath.Join(dir, "T-0001-wip.md")

	calls := 0
	swapRemove(t, func(p string) error {
		calls++
		if p == oldPath {
			// Simulate an out-of-lock agent having removed the old file
			// before our remove ran: really delete it, then report
			// os.Remove's ENOENT shape (*PathError wrapping
			// fs.ErrNotExist).
			if rerr := os.Remove(p); rerr != nil {
				return rerr
			}
			return &fs.PathError{Op: "remove", Path: p, Err: fs.ErrNotExist}
		}
		return os.Remove(p)
	})

	if _, err := s.SetStatus(1, domain.StatusWip, "tester", ""); err != nil {
		t.Fatalf("SetStatus with ENOENT remove-old = %v, want nil (no rollback)", err)
	}
	if calls != 1 {
		t.Fatalf("removeFile calls = %d, want 1 (no rollback attempt)", calls)
	}
	if _, err := os.Stat(oldPath); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("old file should be absent; stat err=%v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("target must survive (no rollback): %v", err)
	}
	assertJournalTransition(t, string(mustRead(t, dir, 1)), "open", "wip", "", "tester")
	assertNoTmpFiles(t, dir)
}
