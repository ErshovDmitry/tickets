package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"ticket/internal/domain"
)

// FindRaw returns ticket n together with the exact base name of the
// scanned file it was read from AND the file's exact raw bytes. The
// bytes come from the single validated handle (openValidated): the file
// is never re-opened by path after the scan matched it, so a symlink
// swapped into the name between scan and read cannot leak outside
// content into the returned bytes (TOCTOU containment).
//
// The name is the directory-scan match — NEVER a path derived from the
// parsed body Status, which is untrusted ("- Статус: " accepts any
// bytes) and must not reach the filesystem (T-0006). The name is
// guaranteed separator-free because scan only accepts basenames matching
// T-NNNN-<lowercase-status>.md.
//
// Error semantics match FindNamed: ErrNotFound for absent numbers
// (including the lockless empty-file window), a wrapped error for
// corrupt files and for entries rejected by the validated open.
// Searches main dir then archive/ subdirectory.
func (s *Store) FindRaw(n int) (domain.Ticket, string, []byte, error) {
	// Search main dir.
	entries, _, dirErr := s.scan()
	if dirErr != nil {
		return domain.Ticket{}, "", nil, fmt.Errorf("store: read dir %s: %w", s.Dir, dirErr)
	}
	for _, e := range entries {
		if e.Number != n {
			continue
		}
		tk, raw, _, err := readTicketFile(filepath.Join(s.Dir, e.Name), e.Number)
		if err != nil {
			if errors.Is(err, errEmptyTicket) {
				// Lockless reader saw the O_EXCL-created file before its
				// body landed: the number is not there yet (§5).
				return domain.Ticket{}, "", nil, ErrNotFound
			}
			return domain.Ticket{}, "", nil, fmt.Errorf("store: read %s: %w", e.Name, err)
		}
		// The scan-matched filename status is authoritative (V19); the
		// parsed body status is untrusted and must not reach Find/show
		// consumers.
		tk.Status = e.Status
		return *tk, e.Name, raw, nil
	}
	// Search archive/.
	archiveDir := filepath.Join(s.Dir, "archive")
	if _, err := os.Stat(archiveDir); err == nil {
		archiveEntries, _, _ := scanDir(archiveDir)
		for _, e := range archiveEntries {
			if e.Number != n {
				continue
			}
			tk, raw, _, err := readTicketFile(filepath.Join(archiveDir, e.Name), e.Number)
			if err != nil {
				if errors.Is(err, errEmptyTicket) {
					return domain.Ticket{}, "", nil, ErrNotFound
				}
				return domain.Ticket{}, "", nil, fmt.Errorf("store: read %s: %w", e.Name, err)
			}
			tk.Status = e.Status
			return *tk, e.Name, raw, nil
		}
	}
	return domain.Ticket{}, "", nil, ErrNotFound
}

// FindNamed returns ticket n together with the exact base name of the
// scanned file it was read from. Delegates to FindRaw, discarding the
// raw bytes; signature and error semantics are unchanged.
func (s *Store) FindNamed(n int) (domain.Ticket, string, error) {
	tk, name, _, err := s.FindRaw(n)
	return tk, name, err
}
