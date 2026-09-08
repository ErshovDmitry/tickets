//go:build unix

package store

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"ticket/internal/domain"
)

// runWithFifoDeadline bounds every FIFO test: if the open path still
// blocks on a writer-less FIFO, the test fails after 2s instead of
// hanging the suite. fn runs in a goroutine; it must report through its
// returned error — t.Fatal/Skip are not allowed inside fn (wrong
// goroutine), use errors.
func runWithFifoDeadline(t *testing.T, label string, fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatalf("%s did not complete within 2s — blocking FIFO open suspected", label)
		return nil
	}
}

// mkfifoOrSkip creates a FIFO at path; skips the test where Mkfifo is
// unavailable. Test-goroutine use only.
func mkfifoOrSkip(t *testing.T, path string) {
	t.Helper()
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("Mkfifo unavailable: %v", err)
	}
}

// TestReadTicketFile_FIFOInPlaceOfTicketDoesNotHang is T-0079 (a): a
// FIFO placed at a ticket path must be rejected by the pre-open Lstat
// (errNotRegularFile) with zero blocking — pre-fix, os.Open(RDONLY) on
// a writer-less FIFO hung until a writer connected. FindRaw must report
// the number as absent (scan skips non-regular entries with a warning),
// also without blocking.
func TestReadTicketFile_FIFOInPlaceOfTicketDoesNotHang(t *testing.T) {
	s, dir := newStore(t)
	path := filepath.Join(dir, "T-0001-open.md")
	mkfifoOrSkip(t, path)

	err := runWithFifoDeadline(t, "readTicketFile on FIFO", func() error {
		_, _, _, err := readTicketFile(path, 1)
		return err
	})
	if !errors.Is(err, errNotRegularFile) {
		t.Errorf("readTicketFile err = %v, want errNotRegularFile", err)
	}

	err = runWithFifoDeadline(t, "FindRaw on FIFO", func() error {
		_, _, _, err := s.FindRaw(1)
		return err
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("FindRaw err = %v, want ErrNotFound (FIFO skipped by scan)", err)
	}
}

// TestOpenValidated_FIFOSwapInRaceWindowRejectedInstantly is T-0079 (b):
// the regular→FIFO swap lands inside the TOCTOU window (between Lstat
// and open, via the hookAfterValidate seam). O_NONBLOCK lets the FIFO
// open return immediately; the opened-handle Stat then sees a
// non-regular file and the identity guard rejects with errFileSwapped
// instantly, never waiting for a writer.
func TestOpenValidated_FIFOSwapInRaceWindowRejectedInstantly(t *testing.T) {
	_, dir := newStore(t)
	path := filepath.Join(dir, "T-0001-open.md")
	makeFile(t, dir, "T-0001-open.md", realTicketBody)
	fired := 0
	hookAfterValidate = func(string) {
		fired++
		if fired > 1 {
			return
		}
		if err := os.Rename(path, filepath.Join(dir, ".T-0001-open.md.swapped")); err != nil {
			t.Errorf("swap rename: %v", err)
			return
		}
		// Mkfifo error via t.Errorf (hook runs in the fn goroutine, no
		// Skip allowed): a failure surfaces as an open error below.
		if err := syscall.Mkfifo(path, 0o600); err != nil {
			t.Errorf("swap mkfifo: %v", err)
		}
	}
	t.Cleanup(func() { hookAfterValidate = nil })

	err := runWithFifoDeadline(t, "openValidated on swapped-in FIFO", func() error {
		_, err := openValidated(path)
		return err
	})
	if !errors.Is(err, errFileSwapped) {
		t.Errorf("openValidated err = %v, want errFileSwapped (FIFO rejected by identity guard)", err)
	}
	if fired != 1 {
		t.Errorf("hook fired %d times, want 1", fired)
	}
}

// TestFIFOFix_RegularFileStillReads is T-0079 (c): the O_NOFOLLOW |
// O_NONBLOCK open must not change regular-file behavior — a valid
// ticket still reads completely through FindRaw.
func TestFIFOFix_RegularFileStillReads(t *testing.T) {
	s, dir := newStore(t)
	makeFile(t, dir, "T-0001-open.md", realTicketBody)
	tk, name, raw, err := s.FindRaw(1)
	if err != nil {
		t.Fatalf("FindRaw on regular file: %v", err)
	}
	if name != "T-0001-open.md" {
		t.Errorf("name = %q, want T-0001-open.md", name)
	}
	if tk.Number != 1 || tk.Status != domain.StatusOpen {
		t.Errorf("tk = %+v, want number 1 status open", tk)
	}
	if len(raw) == 0 {
		t.Errorf("FindRaw returned empty raw bytes for a regular ticket")
	}
}
