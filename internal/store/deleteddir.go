package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"ticket/internal/domain"
)

// ErrDeletedInvalid reports a tickets/deleted path that is not a real
// directory (a symlink or a regular file). Scanning through a symlinked
// deleted/ reads foreign files, and writing through one lands ticket
// files outside TICKETS_DIR (os.MkdirAll silently passes on a symlink
// pointing at a directory). The path is rejected outright; use errors.Is
// to test. T-0189.
var ErrDeletedInvalid = errors.New("deleted is not a directory")

// validDeletedDir resolves the store's deleted directory without ever
// following a symlink. It Lstats <dir>/deleted (never Stat, which would
// resolve the link) and returns:
//   - ("", nil) — no deleted/ yet: the caller keeps its current skip
//     semantics (Delete creates it under the lock, like archive);
//   - ("", wrapped ErrDeletedInvalid) — a symlink or a regular file at
//     the deleted path (Lstat of a symlink yields ModeSymlink, so IsDir
//     is false for both cases);
//   - ("", wrapped stat error) — any other Lstat failure;
//   - (deletedDir, nil) — a real directory.
//
// Accepted residual: the TOCTOU window between this Lstat and the
// caller's later use of deletedDir — the same containment rationale as
// validArchiveDir (T-0078): an attacker able to plant that symlink can
// already corrupt tickets in dir itself, and every write through the
// deleted path runs under the store flock.
func validDeletedDir(dir string) (string, error) {
	deletedDir := filepath.Join(dir, "deleted")
	fi, err := os.Lstat(deletedDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("store: %w", err)
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("store: %s: %w", deletedDir, ErrDeletedInvalid)
	}
	return deletedDir, nil
}

// deletedScan validates the store's deleted directory (validDeletedDir)
// and returns it together with its scanned entries, so callers never
// touch the deleted path unvalidated (T-0189, mirrors archiveScan). No
// deleted/ yet yields ("", nil, nil); an invalid or unreadable deleted
// directory yields an error — deleted scan failures are never silenced,
// so numbering can never reuse a number it could not read. Single
// read-only seam shared by numbering, ResolveDelete and Duplicates.
func (s *Store) deletedScan() (string, []fileEntry, error) {
	deletedDir, err := validDeletedDir(s.Dir)
	if err != nil || deletedDir == "" {
		return "", nil, err
	}
	entries, _, err := scanDir(deletedDir)
	if err != nil {
		return "", nil, fmt.Errorf("store: read dir %s: %w", deletedDir, err)
	}
	return deletedDir, entries, nil
}

// ListDeleted returns the parseable tickets under tickets/deleted/,
// sorted by number. A missing deleted/ directory is not an error: the
// result is empty. Warning semantics match ListArchive: a deleted path
// that is not a real directory (symlink or regular file) or an
// unreadable deleted directory is reported as a single ParseWarning,
// never as a hard failure. Read-only seam for the W2 delete CLI.
func (s *Store) ListDeleted() ([]domain.Ticket, []ParseWarning) {
	resolved, derr := validDeletedDir(s.Dir)
	if derr != nil {
		return nil, []ParseWarning{{Name: filepath.Join(s.Dir, "deleted"), Err: derr, Reason: deletedReason(derr)}}
	}
	if resolved == "" {
		return nil, nil
	}
	entries, warnings, dirErr := scanDir(resolved)
	if dirErr != nil {
		// The deleted directory itself could not be read: dir-read is
		// assigned here, never derived.
		return nil, []ParseWarning{{Name: resolved, Err: dirErr, Reason: reasonDirRead}}
	}
	var tickets []domain.Ticket
	for _, e := range entries {
		tk, err := readTicketHeader(filepath.Join(resolved, e.Name), e.Number)
		if err != nil {
			warnings = append(warnings, ParseWarning{Name: e.Name, Err: err, Reason: deletedReason(err)})
			continue
		}
		// The scan-matched filename status is authoritative (V19).
		tk.Status = e.Status
		tickets = append(tickets, *tk)
	}
	return tickets, warnings
}
