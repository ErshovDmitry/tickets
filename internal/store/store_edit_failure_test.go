package store

// Store.Edit failure-injection coverage (T-0187, C3/3): the rename-over
// commit seam (renameFile) and the tmp cleanup seam (removeTmp) reuse the
// swappable primitives; the old file must survive any commit failure
// byte-identical, the tmp must be removed (or its removal error joined),
// the terminal store.edit.failure log carries only number+stage, a pinned
// tmp name drives the practical tmp-open collision through the existing
// nanoNow seam, and a lock timeout refuses the edit before any read/write.

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
	"ticket/internal/lock"
)

// strPtr returns a pointer to s: the Go 1.26 new(expr) form is not
// understood by every toolchain in the pipeline, so the helper keeps the
// file portable.
func strPtr(s string) *string { return &s }

// assertEditFailureLog fails unless rec holds exactly one
// store.edit.failure record with number=1 and the given stage.
func assertEditFailureLog(t *testing.T, rec *logRecorder, stage string) {
	t.Helper()
	fails := rec.find("store.edit.failure")
	if len(fails) != 1 {
		t.Fatalf("store.edit.failure records = %v, want exactly 1", fails)
	}
	if fails[0].level != slog.LevelError {
		t.Errorf("failure level = %v, want error", fails[0].level)
	}
	want := kv("number", "1", "stage", stage)
	if len(fails[0].attrs) != len(want) {
		t.Errorf("failure attrs = %v, want exactly %v", fails[0].attrs, want)
	}
	if fails[0].attrs["stage"] != stage || fails[0].attrs["number"] != "1" {
		t.Errorf("failure attrs = %v, want %v", fails[0].attrs, want)
	}
}

// TestEdit_RenameFails_OldFileIntactTmpCleaned: a rename-over failure
// aborts the commit with the cause, leaves the old file byte-identical,
// removes the tmp and logs one stage=rename failure.
func TestEdit_RenameFails_OldFileIntactTmpCleaned(t *testing.T) {
	s, dir, rec := newLoggingStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	oldPath := filepath.Join(dir, "T-0001-open.md")
	before, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	rec.reset()

	swapRename(t, func(string, string) error { return errors.New("synthetic-rename-boom") })

	if _, err := s.Edit(1, EditOptions{Title: strPtr("New name")}, "alice"); err == nil {
		t.Fatal("expected rename failure")
	} else if !strings.Contains(err.Error(), "synthetic-rename-boom") {
		t.Errorf("rename cause must be propagated; got %v", err)
	}
	after, rerr := os.ReadFile(oldPath)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(after) != string(before) {
		t.Fatal("old file changed on failed rename commit")
	}
	assertNoTmpFiles(t, dir)
	assertEditFailureLog(t, rec, "rename")
}

// TestEdit_RenameFails_TmpCleanupFails_BothCausesJoined: when both the
// rename and the best-effort tmp removal fail, the returned error must
// carry both causes and the old file stays intact.
func TestEdit_RenameFails_TmpCleanupFails_BothCausesJoined(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	oldPath := filepath.Join(dir, "T-0001-open.md")
	before, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// The injected cleanup failure leaks the tmp; remove it for real.
		matches, _ := filepath.Glob(filepath.Join(dir, "*.tmp"))
		for _, m := range matches {
			_ = os.Remove(m)
		}
	})

	swapRename(t, func(string, string) error { return errors.New("synthetic-rename-boom") })
	swapRemoveTmp(t, func(p string) error {
		if strings.HasSuffix(p, ".md.tmp") {
			return errors.New("synthetic-tmp-remove-boom")
		}
		return os.Remove(p)
	})

	err = nil
	if _, e := s.Edit(1, EditOptions{Title: strPtr("New name")}, "alice"); e != nil {
		err = e
	}
	if err == nil {
		t.Fatal("expected joined rename+cleanup failure")
	}
	for _, cause := range []string{"synthetic-rename-boom", "synthetic-tmp-remove-boom"} {
		if !strings.Contains(err.Error(), cause) {
			t.Errorf("error %q missing cause %q", err, cause)
		}
	}
	after, rerr := os.ReadFile(oldPath)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(after) != string(before) {
		t.Fatal("old file changed when rename and cleanup both failed")
	}
}

// TestEdit_TmpNameCollision_ErrCollisionOldIntact: planting writeTmp's
// exact pid-based tmp name (via the nanoNow seam, same fixture as the
// set/archive failure tests) makes every O_EXCL attempt collide: the edit
// fails with ErrCollision, the old file stays byte-identical, no new tmp
// appears and the failure logs stage=tmp.
func TestEdit_TmpNameCollision_ErrCollisionOldIntact(t *testing.T) {
	s, dir, rec := newLoggingStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	oldPath := filepath.Join(dir, "T-0001-open.md")
	before, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	rec.reset()

	prepTmpWrite("", domain.StatusOpen)(t, s, dir)
	planted := filepath.Join(dir, ".T-0001-open."+itoa(os.Getpid())+"-7.md.tmp")
	t.Cleanup(func() { _ = os.Remove(planted) })

	if _, err := s.Edit(1, EditOptions{Title: strPtr("New name")}, "alice"); !errors.Is(err, ErrCollision) {
		t.Fatalf("err = %v, want ErrCollision", err)
	}
	after, rerr := os.ReadFile(oldPath)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(after) != string(before) {
		t.Fatal("old file changed on tmp-name collision")
	}
	// Exactly the one planted tmp; writeTmp must not leave partial files.
	tmp, gerr := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if gerr != nil {
		t.Fatal(gerr)
	}
	if len(tmp) != 1 || tmp[0] != planted {
		t.Errorf("tmp files = %v, want exactly the planted %s", tmp, planted)
	}
	assertEditFailureLog(t, rec, "tmp")
}

// itoa is a tiny int->string to keep the planted tmp name literal.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// assertOnlyLockTimeoutLog requires exactly one store.lock.error record
// with reason=timeout and no other record at all: the edit was refused at
// the lock, so it never read, wrote or logged an edit event.
func assertOnlyLockTimeoutLog(t *testing.T, rec *logRecorder) {
	t.Helper()
	lockErrs := rec.find("store.lock.error")
	if len(lockErrs) != 1 || lockErrs[0].attrs["reason"] != "timeout" {
		t.Errorf("store.lock.error records = %v, want one with reason=timeout", lockErrs)
	}
	for _, e := range rec.all() {
		if e.event != "store.lock.error" {
			t.Errorf("unexpected log record %v; want only store.lock.error", e)
		}
	}
}

// TestEdit_LockTimeout_ErrLockedFileUntouched: while another holder keeps
// the per-store lock and the wait budget is zero, Edit fails with
// lock.ErrLocked before reading or writing anything.
func TestEdit_LockTimeout_ErrLockedFileUntouched(t *testing.T) {
	s, dir, rec := newLoggingStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	oldPath := filepath.Join(dir, "T-0001-open.md")
	before, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	lk, err := lock.AcquireLocked(s.lockPath(), 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lk.Release() })
	t.Setenv("TICKET_LOCK_TIMEOUT", "0")
	rec.reset()

	if _, err := s.Edit(1, EditOptions{Title: strPtr("New name")}, "alice"); !errors.Is(err, lock.ErrLocked) {
		t.Fatalf("err = %v, want lock.ErrLocked", err)
	}
	// The refusal happens at the lock, before the lookup: no failure
	// event, no success event and no other event at all — in particular
	// no read or write of the ticket file.
	if fails := rec.find("store.edit.failure"); len(fails) != 0 {
		t.Errorf("store.edit.failure records = %v, want none", fails)
	}
	if oks := rec.find("store.edit"); len(oks) != 0 {
		t.Errorf("store.edit records = %v, want none", oks)
	}
	assertOnlyLockTimeoutLog(t, rec)
	after, rerr := os.ReadFile(oldPath)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(after) != string(before) {
		t.Fatal("file changed on lock-timeout refusal")
	}
	assertNoTmpFiles(t, dir)
}

// TestEdit_AmbiguousNumber_StageAmbiguousLog: a number carried by two
// files is refused before any read or write; the terminal
// store.edit.failure log carries stage=ambiguous and nothing else, and
// both files stay byte-identical.
func TestEdit_AmbiguousNumber_StageAmbiguousLog(t *testing.T) {
	s, dir, rec := newLoggingStore(t)
	body := seedTicketOne(t, s, dir)
	dup := filepath.Join(dir, "T-0001-done.md")
	cloneTicket(t, dup, body)
	rec.reset()

	_, err := s.Edit(1, EditOptions{Title: strPtr("New name")}, "alice")
	if err == nil {
		t.Fatal("expected an ambiguity refusal")
	}
	assertAmbiguous(t, err, 1, []string{dup, filepath.Join(dir, "T-0001-open.md")})
	assertEditFailureLog(t, rec, "ambiguous")
	for _, p := range []string{dup, filepath.Join(dir, "T-0001-open.md")} {
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			t.Fatal(rerr)
		}
		if string(data) != string(body) {
			t.Errorf("%s changed on the ambiguity refusal", p)
		}
	}
	assertNoTmpFiles(t, dir)
}
