package store

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"ticket/internal/domain"
)

// DeleteCandidate identifies the single ticket file a delete ref
// resolves to. Status is the filename-derived status (authoritative,
// V19); Path is absolute. It carries no body: resolution works on
// scanned names alone, so a corrupt body never hides a candidate.
type DeleteCandidate struct {
	Number int
	Status domain.Status
	Path   string
}

// ResolveDelete resolves ref — a ticket number or an exact ticket
// base name (T-NNNN-<status>.md) — across s.Dir, s.Dir/archive and
// s.Dir/deleted (T-0189). Read-only and lockless, like Find/FindExact;
// Delete re-runs the same resolution under the lock before mutating.
//
// Errors: ErrNotFound for an absent ref; *AmbiguousNumberError when
// several files carry the number, or the identical base name exists
// in more than one directory — Paths holds every conflicting absolute
// path, sorted; a wrapped error for an unreadable main directory or
// an invalid/unreadable archive or deleted directory (never
// silenced); an invalid ref (path separators, traversal, non-digit
// non-filename input, or a number outside 1..maxTicketNumber) is
// rejected before any scan.
func (s *Store) ResolveDelete(ref string) (DeleteCandidate, error) {
	loc, err := s.resolveDeleteRef(ref)
	if err != nil {
		return DeleteCandidate{}, err
	}
	return DeleteCandidate{Number: loc.Number, Status: loc.Status, Path: filepath.Join(loc.dir, loc.Name)}, nil
}

// resolveDeleteRef is the shared resolve step of ResolveDelete and
// Delete: one scan pass over main+archive+deleted through the same
// seams numbering and Duplicates use (scan/archiveScan/deletedScan).
// It never reads a body and never mutates.
func (s *Store) resolveDeleteRef(ref string) (locatedFile, error) {
	byName := false
	var n int
	if !strings.ContainsAny(ref, "/\\") {
		if fn, _, perr := domain.ParseFilename(ref); perr == nil {
			byName, n = true, fn
		}
	}
	if !byName {
		parsed, err := deleteNumber(ref)
		if err != nil {
			return locatedFile{}, err
		}
		n = parsed
	}
	entries, _, dirErr := s.scan()
	if dirErr != nil {
		return locatedFile{}, fmt.Errorf("store: read dir %s: %w", s.Dir, dirErr)
	}
	var matches []locatedFile
	collect := func(entries []fileEntry, dir string) {
		for _, e := range entries {
			if (byName && e.Name == ref) || (!byName && e.Number == n) {
				matches = append(matches, locatedFile{fileEntry: e, dir: dir})
			}
		}
	}
	collect(entries, s.Dir)
	archiveDir, archiveEntries, aerr := s.archiveScan()
	if aerr != nil {
		return locatedFile{}, aerr
	}
	collect(archiveEntries, archiveDir)
	deletedDir, deletedEntries, derr := s.deletedScan()
	if derr != nil {
		return locatedFile{}, derr
	}
	collect(deletedEntries, deletedDir)
	switch len(matches) {
	case 0:
		return locatedFile{}, ErrNotFound
	case 1:
		return matches[0], nil
	}
	return locatedFile{}, ambiguousNumberError(n, matches)
}

// deleteNumber validates a numeric delete ref: strict digits within
// 1..maxTicketNumber. Atoi overflow and out-of-range numbers are
// invalid refs on every architecture — rejected before any scan,
// never ErrNotFound (W1 A4).
func deleteNumber(ref string) (int, error) {
	if !isDigits(ref) {
		return 0, fmt.Errorf("store: ResolveDelete: not a ticket number or file name: %q", ref)
	}
	n, err := strconv.Atoi(ref)
	if err != nil || n < 1 || n > maxTicketNumber {
		return 0, fmt.Errorf("store: ResolveDelete: not a ticket number or file name: %q", ref)
	}
	return n, nil
}

// isDigits reports whether s is a non-empty run of decimal digits.
// A delete number ref must be strict digits: signs, spaces and
// punctuation are invalid input, not a number to normalize.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
