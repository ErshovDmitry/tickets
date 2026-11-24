package store

// T-0241 code review (R2.3/R2.4): ONE scan of the read tree and ONE match rule
// per lookup axis, so the number/name matching cannot drift between FindRaw,
// FindExact and their deleted-aware (Across) and show (ShowAcross) variants.
//
// Before this file the match loop existed four times (findAllNumber,
// findAllNumberAcross, findAllNumberShowAcross, matchingEntries) and the
// per-directory warnings were flattened into one slice, which made a warning
// about archive/T-0001-done.md indistinguishable from one about the
// same-named file in s.Dir. Keeping the warnings next to the directory that
// reported them is what makes attribution by DIRECTORY + base name possible.

import "fmt"

// scannedDir is one directory of the read tree together with what its scan
// reported: the entries (each file paired with this directory) and the parse
// warnings, both in scan order.
type scannedDir struct {
	dir     string
	entries []fileEntry
	warns   []ParseWarning
}

// locatedScan is one scan of the read tree in scan order: s.Dir, then
// s.Dir/archive, then — when withDeleted was requested — s.Dir/deleted. An
// absent directory contributes an empty scannedDir, so a warning can never be
// attributed to a directory that was not walked.
type locatedScan struct {
	dirs []scannedDir
}

// scanReadTree walks s.Dir, s.Dir/archive and (optionally) s.Dir/deleted once,
// keeping every entry and every warning. Error semantics are the shared ones:
// an unreadable main directory is wrapped with its path, archive/deleted errors
// propagate unchanged (their seams already wrap), and a missing archive/ or
// deleted/ directory is not an error.
func (s *Store) scanReadTree(withDeleted bool) (locatedScan, error) {
	entries, warns, dirErr := s.scan()
	if dirErr != nil {
		return locatedScan{}, fmt.Errorf("store: read dir %s: %w", s.Dir, dirErr)
	}
	archiveDir, archiveEntries, archiveWarns, aerr := s.archiveScanWarned()
	if aerr != nil {
		return locatedScan{}, aerr
	}
	dirs := []scannedDir{
		{dir: s.Dir, entries: entries, warns: warns},
		{dir: archiveDir, entries: archiveEntries, warns: archiveWarns},
	}
	if !withDeleted {
		return locatedScan{dirs: dirs}, nil
	}
	deletedDir, deletedEntries, deletedWarns, derr := s.deletedScanWarned()
	if derr != nil {
		return locatedScan{}, derr
	}
	return locatedScan{dirs: append(dirs, scannedDir{dir: deletedDir, entries: deletedEntries, warns: deletedWarns})}, nil
}

// findNumberIn is the shared number lookup: one scan of the requested tree
// plus numberMatches, so findAllNumber and findAllNumberAcross differ ONLY in
// whether tickets/deleted/ is walked.
func (s *Store) findNumberIn(n int, withDeleted bool) ([]locatedFile, error) {
	sc, err := s.scanReadTree(withDeleted)
	if err != nil {
		return nil, err
	}
	return sc.numberMatches(n), nil
}

// numberMatches is the only "files carrying number n" rule of the read path:
// every entry of every walked directory whose number equals n, in scan order
// (main, archive, deleted). It never stops at the first match, so a caller can
// detect the T-0157 ambiguity without parsing a body.
func (sc locatedScan) numberMatches(n int) []locatedFile {
	return sc.matchingFiles(func(e fileEntry) bool { return e.Number == n })
}

// nameMatches is numberMatches for an exact base name (T-NNNN-<status>.md) —
// the escape hatch for an ambiguous number (T-0157).
func (sc locatedScan) nameMatches(baseName string) []locatedFile {
	return sc.matchingFiles(func(e fileEntry) bool { return e.Name == baseName })
}

// matchingFiles applies keep to every scanned entry, pairing the match with the
// directory it was found in.
func (sc locatedScan) matchingFiles(keep func(fileEntry) bool) []locatedFile {
	var out []locatedFile
	for _, d := range sc.dirs {
		for _, e := range d.entries {
			if keep(e) {
				out = append(out, locatedFile{fileEntry: e, dir: d.dir})
			}
		}
	}
	return out
}

// attributableWarnings keeps only the warnings that belong to loc: a warning is
// attributed only when it was reported by loc's OWN directory AND names loc's
// base name. A same-named file in another directory (archive/, deleted/) is a
// different file, so its warning must never be reported for this read. A
// located file was accepted by the very scan that produced the warnings, so
// this filter normally yields nothing.
func (sc locatedScan) attributableWarnings(loc locatedFile) []ParseWarning {
	var out []ParseWarning
	for _, d := range sc.dirs {
		if d.dir != loc.dir {
			continue
		}
		for _, w := range d.warns {
			if w.Name == loc.Name {
				out = append(out, w)
			}
		}
	}
	return out
}
