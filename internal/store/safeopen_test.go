package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ticket/internal/domain"
)

// installSwapHook is the deterministic TOCTOU regression hook. It runs
// exactly once, synchronously, inside openValidated between the pre-open
// Lstat check and os.Open — the exact race window — and swaps the
// validated regular ticket file for a symlink to the outside sentinel.
// Hidden rename keeps the original bytes in the store dir invisible to
// scan (dotfile), so post-swap assertions stay exact. Race-detector
// clean: the hook variable is written before and cleared after the
// synchronous Find/List calls, all in the test goroutine.
func installSwapHook(t *testing.T, dir, origName, sentinel string) {
	t.Helper()
	path := filepath.Join(dir, origName)
	calls := 0
	hookAfterValidate = func(p string) {
		calls++
		if calls > 1 {
			t.Errorf("swap hook fired %d times, want exactly 1", calls)
			return
		}
		if p != path {
			t.Errorf("swap hook fired for %q, want %q", p, path)
			return
		}
		hidden := filepath.Join(dir, "."+origName+".swapped")
		if err := os.Rename(path, hidden); err != nil {
			t.Errorf("swap hook rename: %v", err)
			return
		}
		if err := os.Symlink(sentinel, path); err != nil {
			t.Errorf("swap hook symlink: %v", err)
		}
	}
	t.Cleanup(func() { hookAfterValidate = nil })
}

// requireSymlinks skips the test when the platform cannot create
// symlinks at all (e.g. unprivileged Windows), mirroring the skips in
// TestOpenValidated_RejectsSymlinkAtReadTime and scan_symlink_test.go.
// Needed by tests whose installSwapHook performs os.Symlink: without
// the probe the hook's t.Errorf would fail instead of skip there.
func requireSymlinks(t *testing.T) {
	t.Helper()
	probe := filepath.Join(t.TempDir(), "probe-target")
	if err := os.WriteFile(probe, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(probe, probe+".link"); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}
}

// realTicketBody is a fully valid T-0001 open ticket body stored INSIDE
// the store directory.
const realTicketBody = "# T-0001 · BUG: real\n" +
	"\n" +
	"- Статус: open\n" +
	"- Приоритет: normal\n" +
	"- Создан: 2026-09-02 10:00 · кем: tester\n" +
	"- Проект: tickets\n" +
	"\n" +
	"## Кратко\ninside body\n" +
	"\n## Подробности\nx\n" +
	"\n## Журнал\n" +
	"- 2026-09-02 10:00 — тикет создан (tester).\n"

// newStoreWithRealTicket builds a store dir holding exactly one real,
// regular T-0001-open.md ticket plus an outside sentinel dir laid out by
// makeOutsideTicket (a fully valid T-0001 body with TOPSECRET-SENTINEL —
// indistinguishable from a real ticket by content alone).
func newStoreWithRealTicket(t *testing.T) (s *Store, dir, sentinel string, sentinelBody []byte) {
	t.Helper()
	s, dir = newStore(t)
	makeFile(t, dir, "T-0001-open.md", realTicketBody)
	root := filepath.Dir(dir)
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel, sentinelBody = makeOutsideTicket(t, outside)
	return s, dir, sentinel, sentinelBody
}

// TestFindRaw_TOCTOUSymlinkSwapCannotLeakOutside is the FINAL-F3
// regression: a regular ticket validated by scan is swapped for a
// symlink to the outside sentinel between validation and open. The
// opened-handle Stat must mismatch the pre-open Lstat identity
// (os.SameFile) and reject BEFORE any byte is read, so the sentinel
// content can never be returned through FindRaw, List or SetStatus.
// Pre-fix this leaked the sentinel bytes verbatim.
func TestFindRaw_TOCTOUSymlinkSwapCannotLeakOutside(t *testing.T) {
	requireSymlinks(t) // installSwapHook creates a symlink; skip (not fail) where unavailable
	s, dir, sentinel, sentinelBody := newStoreWithRealTicket(t)
	installSwapHook(t, dir, "T-0001-open.md", sentinel)

	tk, name, raw, err := s.FindRaw(1)
	if err == nil {
		t.Fatalf("FindRaw succeeded through a mid-open symlink swap: %+v", tk)
	}
	if !errors.Is(err, errFileSwapped) {
		t.Errorf("FindRaw err = %v, want errFileSwapped (identity check)", err)
	}
	if raw != nil {
		t.Errorf("FindRaw returned raw bytes on rejected open: %q", raw)
	}
	if bytes.Contains(raw, sentinelBody) {
		t.Errorf("outside sentinel bytes leaked through FindRaw")
	}
	if name != "" {
		t.Errorf("FindRaw name = %q on rejected open, want empty", name)
	}

	// List: the swapped name is no longer a valid ticket file — no
	// tickets, exactly one warning for the (now non-regular) entry; the
	// hidden renamed original stays invisible to scan (dotfile).
	tickets, warns := s.List()
	if len(tickets) != 0 {
		t.Errorf("List returned tickets through the swapped entry: %+v", tickets)
	}
	if len(warns) != 1 || warns[0].Name != "T-0001-open.md" {
		t.Errorf("List warnings = %+v, want exactly 1 for T-0001-open.md", warns)
	}
	for _, tk := range tickets {
		if bytes.Contains([]byte(tk.Title), []byte("TOPSECRET")) {
			t.Errorf("List leaked outside sentinel title: %+v", tk)
		}
	}

	// Find/FindNamed: absent, not a pass-through to the sentinel.
	if _, err := s.Find(1); !errors.Is(err, ErrNotFound) {
		t.Errorf("Find(1) err = %v, want ErrNotFound", err)
	}
	if _, _, err := s.FindNamed(1); !errors.Is(err, ErrNotFound) {
		t.Errorf("FindNamed(1) err = %v, want ErrNotFound", err)
	}

	// SetStatus must not mutate anything through the swapped entry.
	if _, err := s.SetStatus(1, domain.StatusWip, "tester", "TOCTOU attempt"); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetStatus err = %v, want ErrNotFound", err)
	}
	assertOutsideUntouched(t, sentinel, sentinelBody)
}

// TestOpenValidated_RejectsSymlinkAtReadTime pins the first guard layer:
// a symlink already present at read time is rejected by the pre-open
// Lstat (non-regular) before os.Open runs at all — the test hook is
// never reached.
func TestOpenValidated_RejectsSymlinkAtReadTime(t *testing.T) {
	_, dir, sentinel, sentinelBody := newStoreWithRealTicket(t)
	if err := os.Rename(filepath.Join(dir, "T-0001-open.md"), filepath.Join(dir, ".stashed")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sentinel, filepath.Join(dir, "T-0001-open.md")); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}
	fired := false
	hookAfterValidate = func(string) { fired = true }
	t.Cleanup(func() { hookAfterValidate = nil })

	f, err := openValidated(filepath.Join(dir, "T-0001-open.md"))
	if err == nil {
		f.Close()
		t.Fatalf("openValidated opened through a symlink")
	}
	if !errors.Is(err, errNotRegularFile) {
		t.Errorf("openValidated err = %v, want errNotRegularFile", err)
	}
	if fired {
		t.Errorf("hook fired although Lstat should reject before open")
	}
	assertOutsideUntouched(t, sentinel, sentinelBody)
}

// TestFindRaw_ReturnsExactRawBytes guards the raw-bytes contract: the
// bytes FindRaw returns must be byte-for-byte the file's content, and
// the scan-matched filename status stays authoritative.
func TestFindRaw_ReturnsExactRawBytes(t *testing.T) {
	s, dir := newStore(t)
	makeFile(t, dir, "T-0001-open.md", realTicketBody)

	want, err := os.ReadFile(filepath.Join(dir, "T-0001-open.md"))
	if err != nil {
		t.Fatal(err)
	}
	tk, name, raw, err := s.FindRaw(1)
	if err != nil {
		t.Fatalf("FindRaw: %v", err)
	}
	if name != "T-0001-open.md" {
		t.Errorf("name = %q, want T-0001-open.md", name)
	}
	if !bytes.Equal(raw, want) {
		t.Errorf("raw bytes differ from file content: got %q, want %q", raw, want)
	}
	if tk.Number != 1 || tk.Status != domain.StatusOpen {
		t.Errorf("tk = %+v, want number 1 status open", tk)
	}
}
