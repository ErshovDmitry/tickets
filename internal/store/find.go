package store

import (
	"errors"
	"fmt"
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
	// Search archive/. T-0078: the archive path is validated first — a
	// symlinked or non-directory archive fails loudly and archive scan
	// errors are no longer discarded.
	archiveDir, archiveEntries, aerr := s.archiveScan()
	if aerr != nil {
		return domain.Ticket{}, "", nil, aerr
	}
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
	return domain.Ticket{}, "", nil, ErrNotFound
}

// FindNamed returns ticket n together with the exact base name of the
// scanned file it was read from. Delegates to FindRaw, discarding the
// raw bytes; signature and error semantics are unchanged.
func (s *Store) FindNamed(n int) (domain.Ticket, string, error) {
	tk, name, _, err := s.FindRaw(n)
	return tk, name, err
}

// findLocked scans s.Dir then s.Dir/archive for ticket n, returning the
// fileEntry and the directory where it was found. Used by SetStatus and
// Archive to handle archived tickets.
func (s *Store) findLocked(n int) (fileEntry, string, error) {
	entries, _, dirErr := s.scan()
	if dirErr != nil {
		return fileEntry{}, "", fmt.Errorf("store: read dir %s: %w", s.Dir, dirErr)
	}
	for _, e := range entries {
		if e.Number == n {
			return e, s.Dir, nil
		}
	}
	// Check archive/. T-0078: validated; archive errors fail loudly.
	archiveDir, archiveEntries, aerr := s.archiveScan()
	if aerr != nil {
		return fileEntry{}, "", aerr
	}
	for _, e := range archiveEntries {
		if e.Number == n {
			return e, archiveDir, nil
		}
	}
	return fileEntry{}, "", ErrNotFound
}

// locatedFile pairs a scanned file entry with the directory it lives in
// (s.Dir or s.Dir/archive).
type locatedFile struct {
	fileEntry
	dir string
}

// findAllNumber returns every scanned file whose number equals n, from
// both s.Dir and s.Dir/archive, each with its directory (main-dir entries
// first, then archive entries). Unlike findLocked it does not stop at the
// first match, so a caller can detect a same-number file at a different
// status (the T-0074 collision gate) without parsing either file. An
// unreadable main directory is a wrapped error, mirroring findLocked;
// archive errors (invalid path, T-0078, or an unreadable archive
// directory) propagate the same way.
func (s *Store) findAllNumber(n int) ([]locatedFile, error) {
	entries, _, dirErr := s.scan()
	if dirErr != nil {
		return nil, fmt.Errorf("store: read dir %s: %w", s.Dir, dirErr)
	}
	var out []locatedFile
	for _, e := range entries {
		if e.Number == n {
			out = append(out, locatedFile{fileEntry: e, dir: s.Dir})
		}
	}
	archiveDir, archiveEntries, aerr := s.archiveScan()
	if aerr != nil {
		return nil, aerr
	}
	for _, e := range archiveEntries {
		if e.Number == n {
			out = append(out, locatedFile{fileEntry: e, dir: archiveDir})
		}
	}
	return out, nil
}
