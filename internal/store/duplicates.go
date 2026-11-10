package store

import (
	"fmt"
	"sort"
)

// NumberConflict reports a ticket number carried by more than one file
// across s.Dir and s.Dir/archive (T-0157). Paths are the canonical
// ambiguity set: absolute, deduplicated, sorted — the same shape
// AmbiguousNumberError carries.
type NumberConflict struct {
	Number int
	Paths  []string
}

// Duplicates scans the main directory and the archive and returns every
// number owned by more than one file, sorted by number. It is the
// inventory source for the CLI's non-fatal duplicate reports (list prints
// a warning per conflict; archive-all skips conflicting numbers): unlike
// the strict find gates it never fails on ambiguity, only on unreadable
// directories.
func (s *Store) Duplicates() ([]NumberConflict, error) {
	entries, _, dirErr := s.scan()
	if dirErr != nil {
		return nil, fmt.Errorf("store: read dir %s: %w", s.Dir, dirErr)
	}
	archiveDir, archiveEntries, aerr := s.archiveScan()
	if aerr != nil {
		return nil, aerr
	}
	byNumber := make(map[int][]locatedFile)
	for _, e := range entries {
		byNumber[e.Number] = append(byNumber[e.Number], locatedFile{fileEntry: e, dir: s.Dir})
	}
	for _, e := range archiveEntries {
		byNumber[e.Number] = append(byNumber[e.Number], locatedFile{fileEntry: e, dir: archiveDir})
	}
	numbers := make([]int, 0, len(byNumber))
	for n, files := range byNumber {
		if len(files) > 1 {
			numbers = append(numbers, n)
		}
	}
	sort.Ints(numbers)
	out := make([]NumberConflict, 0, len(numbers))
	for _, n := range numbers {
		out = append(out, NumberConflict{Number: n, Paths: ambiguousNumberError(n, byNumber[n]).Paths})
	}
	return out, nil
}
