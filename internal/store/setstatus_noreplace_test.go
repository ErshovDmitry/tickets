package store

// No-replace commit regression tests (T-0011): SetStatus commits via a
// hard link (os.Link), which is atomic no-replace on POSIX (EEXIST) and
// Windows (CreateHardLink ERROR_ALREADY_EXISTS, mapped by Go to
// fs.ErrExist). A target file that appears between the Lstat fast-path
// and the commit must surface as ErrCollision and must never be
// overwritten.

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// seedTicket creates ticket #1 (open) in a fresh store and returns the
// store, its directory and the old file path.
func seedTicket(t *testing.T) (*Store, string, string) {
	t.Helper()
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	return s, dir, filepath.Join(dir, "T-0001-open.md")
}

// assertOpenBody fails the test if the file at path is not the untouched
// open-status ticket body.
func assertOpenBody(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(data), "- Status (Статус): open") {
		t.Errorf("%s was mutated:\n%s", path, string(data))
	}
}

// TestSetStatus_NoReplace_WindowRace drives the real os.Link behind a
// hook that first creates the target with foreign content — simulating
// an out-of-lock agent winning the race window. The link must fail with
// fs.ErrExist; the foreign file must survive untouched.
func TestSetStatus_NoReplace_WindowRace(t *testing.T) {
	s, dir, oldPath := seedTicket(t)
	target := filepath.Join(dir, "T-0001-wip.md")
	const foreign = "# foreign agent committed first"
	swapLink(t, func(tmp, tgt string) error {
		if werr := os.WriteFile(tgt, []byte(foreign), 0o644); werr != nil {
			return werr
		}
		return os.Link(tmp, tgt)
	})
	_, err := s.SetStatus(1, domain.StatusWip, "tester", "")
	var coll *CollisionError
	if !errors.As(err, &coll) || !errors.Is(err, ErrCollision) {
		t.Fatalf("expected CollisionError/ErrCollision, got %v", err)
	}
	if coll.Target != target {
		t.Errorf("CollisionError.Target = %q, want %q", coll.Target, target)
	}
	data, rerr := os.ReadFile(target)
	if rerr != nil {
		t.Fatalf("read foreign target: %v", rerr)
	}
	if string(data) != foreign {
		t.Errorf("foreign target was overwritten:\n%s", string(data))
	}
	assertNoTmpFiles(t, dir)
	assertOpenBody(t, oldPath)
}

// TestSetStatus_NoReplace_InjectedErrExist injects fs.ErrExist without
// any I/O: the commit must map it to ErrCollision, clean the tmp file
// and leave both old and target untouched.
func TestSetStatus_NoReplace_InjectedErrExist(t *testing.T) {
	s, dir, oldPath := seedTicket(t)
	target := filepath.Join(dir, "T-0001-wip.md")
	swapLink(t, func(string, string) error { return fs.ErrExist })
	_, err := s.SetStatus(1, domain.StatusWip, "tester", "")
	if !errors.Is(err, ErrCollision) {
		t.Fatalf("expected ErrCollision, got %v", err)
	}
	assertNoTmpFiles(t, dir)
	assertOpenBody(t, oldPath)
	if _, serr := os.Stat(target); !errors.Is(serr, fs.ErrNotExist) {
		t.Fatalf("target must not exist; Lstat err=%v", serr)
	}
}

// TestSetStatus_NoReplace_InjectedOtherError injects an arbitrary link
// failure: the cause must be propagated (wrapped "store: link commit:"),
// the tmp removed and the old file left intact.
func TestSetStatus_NoReplace_InjectedOtherError(t *testing.T) {
	s, dir, oldPath := seedTicket(t)
	swapLink(t, func(string, string) error {
		return errors.New("synthetic-link-boom")
	})
	_, err := s.SetStatus(1, domain.StatusWip, "tester", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "synthetic-link-boom") {
		t.Errorf("cause must be propagated; got %v", err)
	}
	assertNoTmpFiles(t, dir)
	assertOpenBody(t, oldPath)
}
