package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"ticket/internal/domain"
)

// ErrArchiveInvalid reports a tickets/archive path that is not a real
// directory (a symlink or a regular file). Scanning through a symlinked
// archive reads foreign files, and writing through one lands ticket
// files outside TICKETS_DIR (os.MkdirAll silently passes on a symlink
// pointing at a directory). The path is rejected outright; use errors.Is
// to test. T-0078.
var ErrArchiveInvalid = errors.New("archive is not a directory")

// validArchiveDir resolves the store's archive directory without ever
// following a symlink. It Lstats <dir>/archive (never Stat, which would
// resolve the link) and returns:
//   - ("", nil) — no archive yet: the caller keeps its current
//     skip semantics;
//   - ("", wrapped ErrArchiveInvalid) — a symlink or a regular file at
//     the archive path (Lstat of a symlink yields ModeSymlink, so
//     IsDir is false for both cases);
//   - ("", wrapped stat error) — any other Lstat failure;
//   - (archiveDir, nil) — a real directory.
//
// Accepted residual: the TOCTOU window between this Lstat and the
// caller's later use of archiveDir. An attacker able to plant that
// symlink can already corrupt tickets in dir itself, and every write
// through the archive path runs under the store flock — this check is
// containment, not a boundary against the directory's owner.
func validArchiveDir(dir string) (string, error) {
	archiveDir := filepath.Join(dir, "archive")
	fi, err := os.Lstat(archiveDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("store: lstat %s: %w", archiveDir, err)
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("store: %s: %w", archiveDir, ErrArchiveInvalid)
	}
	return archiveDir, nil
}

// archiveScan validates the store's archive directory (validArchiveDir)
// and returns it together with its scanned entries, so callers never
// touch the archive path unvalidated (T-0078). No archive yet yields
// ("", nil, nil); an invalid or unreadable archive yields an error —
// archive scan failures are never silenced.
func (s *Store) archiveScan() (string, []fileEntry, error) {
	archiveDir, err := validArchiveDir(s.Dir)
	if err != nil || archiveDir == "" {
		return "", nil, err
	}
	entries, _, err := scanDir(archiveDir)
	if err != nil {
		return "", nil, fmt.Errorf("store: read dir %s: %w", archiveDir, err)
	}
	return archiveDir, entries, nil
}

// ListArchive returns the parseable tickets under tickets/archive/,
// sorted by number. A missing archive/ directory is not an error: the
// result is empty. Warning semantics match List: an archive path that
// is not a real directory (symlink or regular file, T-0078) or an
// unreadable archive directory is reported as a single ParseWarning,
// never as a hard failure.
func (s *Store) ListArchive() ([]domain.Ticket, []ParseWarning) {
	resolved, aerr := validArchiveDir(s.Dir)
	if aerr != nil {
		return nil, []ParseWarning{{Name: filepath.Join(s.Dir, "archive"), Err: aerr}}
	}
	if resolved == "" {
		return nil, nil
	}
	entries, warnings, dirErr := scanDir(resolved)
	if dirErr != nil {
		return nil, []ParseWarning{{Name: resolved, Err: dirErr}}
	}
	var tickets []domain.Ticket
	for _, e := range entries {
		tk, _, _, err := readTicketFile(filepath.Join(resolved, e.Name), e.Number)
		if err != nil {
			warnings = append(warnings, ParseWarning{Name: e.Name, Err: err})
			continue
		}
		tk.Status = e.Status
		tickets = append(tickets, *tk)
	}
	return tickets, warnings
}
