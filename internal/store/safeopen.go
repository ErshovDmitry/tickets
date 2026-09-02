package store

import (
	"errors"
	"fmt"
	"io"
	"os"

	"ticket/internal/domain"
)

// hookAfterValidate is a test-only seam invoked by openValidated between
// the pre-open identity check and os.Open. Production code leaves it nil
// (nil call = no-op). The TOCTOU regression test installs a hook that
// swaps the validated regular ticket file for a symlink to an outside
// sentinel, exactly reproducing the race window between scan-time
// validation and open. Unexported, set/cleared synchronously in the
// calling goroutine (race-detector clean).
var hookAfterValidate func(path string)

// errFileSwapped reports that the file identity changed between the
// pre-open Lstat and the opened-handle Stat (TOCTOU guard): the entry
// the scan matched is no longer the file that got opened.
var errFileSwapped = &parseErr{msg: "file identity changed during open"}

// openValidated opens path read-only only after proving it is still the
// same regular file the scan matched:
//
//  1. os.Lstat resolves the entry itself, never the symlink target, so
//     a symlink/FIFO/device present at read time is rejected before any
//     open (Context7: Lstat "without following symbolic links").
//  2. The test hook (nil in production) runs between check and open.
//  3. os.Open is the only open; f.Stat re-reads the identity through the
//     opened handle and os.SameFile compares it with step 1. Any swap in
//     the window (file→symlink, file→another regular file, removal)
//     mismatches and is rejected BEFORE a single byte is read.
//
// The caller must Close the returned file. Content is read from this
// validated handle — never re-opened by path.
func openValidated(path string) (*os.File, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, errNotRegularFile
	}
	if hookAfterValidate != nil {
		hookAfterValidate(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	sfi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !sfi.Mode().IsRegular() || !os.SameFile(fi, sfi) {
		f.Close()
		return nil, errFileSwapped
	}
	return f, nil
}

// readTicketFile reads and parses one ticket file. domain.Parse is
// tolerant by contract (it never fails), so corruption is detected as a
// mismatch between the H1 number and wantNum derived from the filename:
// a file whose body does not name its own ticket number is corrupt.
// An EMPTY file is not corruption: it is the visible window between the
// O_EXCL create and the body write of a concurrent Create, reported via
// errEmptyTicket so lockless readers can treat the number as absent.
// It returns the parsed ticket, the exact raw bytes read (raw, for
// verbatim consumers like show) and the unknown trailing bytes
// (unknown, preserved for Render). The bytes come from a validated
// handle (openValidated); the file is never re-opened by path after
// validation.
func readTicketFile(path string, wantNum int) (*domain.Ticket, []byte, []byte, error) {
	f, err := openValidated(path)
	if err != nil {
		return nil, nil, nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(data) == 0 {
		return nil, nil, nil, errEmptyTicket
	}
	tk, unknown, perr := domain.Parse(data)
	if perr != nil {
		return nil, nil, nil, perr
	}
	if tk.Number != wantNum {
		return nil, nil, nil, fmt.Errorf("H1 number %d does not match filename T-%04d", tk.Number, wantNum)
	}
	return tk, data, unknown, nil
}

// errEmptyTicket marks a zero-length T-*.md file: create in progress.
var errEmptyTicket = errors.New("ticket file is empty (create in progress)")
