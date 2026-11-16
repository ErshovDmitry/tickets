package store

// T-0189 W3-C1a: deleted-aware READ-ONLY lookup seams for show and the
// set/edit guards. They widen ONLY the lookup: the shared
// findAllNumber / findAllNumberStrict / findOneLocked scan (main+archive)
// is untouched, so SetStatus/Edit/Archive keep their W2 deleted-blindness
// and no implicit undelete is possible. Deleted entries join through the
// existing validated deletedScan() seam (validDeletedDir + scanDir), the
// same containment recover uses (recover_deleted.go). A file under
// tickets/deleted/ is terminal and read-only for these seams: nothing
// here renames, removes or rewrites it.
//
// Public API for the W3 CLI atom (C1b):
//   - FindRawAcross(n)       — show: ticket by number across main+archive+deleted
//   - FindExactAcross(name)  — show: ticket by exact basename across all three
//   - LocateNumber(n)        — set/edit guard: scan-only paths for n (main+archive)
//   - LocateNumberDeleted(n) — set/edit guard: scan-only paths for n (deleted/ only)
//
// Semantics shared with FindRaw/FindExact: ErrNotFound for an absent
// number or name (including the lockless empty-file window); a wrapped
// error for corrupt files, rejected opens and unreadable directories;
// *AmbiguousNumberError with every conflicting path (deduplicated,
// sorted) when more than one file carries the request — the ambiguity is
// detected BEFORE any body is read (T-0157). Deleted-scan errors
// propagate like archive-scan errors: the lookup refuses rather than
// silently ignoring a directory it could not read. No deleted/ yet is
// not an error: it contributes no matches.

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"ticket/internal/domain"
)

// findAllNumberAcross returns every file carrying n — the unchanged
// shared main+archive scan plus the deleted/ entries — each paired with
// its directory, in scan order (main, archive, deleted). Deleted scan
// errors propagate exactly like the archive scan error inside
// findAllNumber. Mirrors findRecoverFiles; kept as a separate helper so
// recover and the W3 lookups stay independently owned.
func (s *Store) findAllNumberAcross(n int) ([]locatedFile, error) {
	files, err := s.findAllNumber(n)
	if err != nil {
		return nil, err
	}
	deletedDir, entries, derr := s.deletedScan()
	if derr != nil {
		return nil, derr
	}
	for _, e := range entries {
		if e.Number == n {
			files = append(files, locatedFile{fileEntry: e, dir: deletedDir})
		}
	}
	return files, nil
}

// FindRawAcross is FindRaw widened with tickets/deleted/: it returns
// ticket n, the exact base name of the scanned file it was read from and
// the file's raw bytes, all three directories considered. The bytes come
// from the single validated handle (readLocated), so a symlink swapped
// in after the scan cannot leak outside content (the same TOCTOU
// containment as FindRaw); the name is the directory-scan match, never a
// path derived from the parsed body Status.
func (s *Store) FindRawAcross(n int) (domain.Ticket, string, []byte, error) {
	files, err := s.findAllNumberAcross(n)
	if err != nil {
		return domain.Ticket{}, "", nil, err
	}
	switch len(files) {
	case 0:
		return domain.Ticket{}, "", nil, ErrNotFound
	case 1:
		return readLocated(files[0])
	}
	return domain.Ticket{}, "", nil, ambiguousNumberError(n, files)
}

// FindExactAcross is FindExact widened with tickets/deleted/: it
// resolves a ticket by its exact base name (T-NNNN-<status>.md) across
// s.Dir, s.Dir/archive and s.Dir/deleted — the escape hatch for an
// ambiguous number (T-0157). The same base name present in more than one
// directory is *AmbiguousNumberError with every path. A name that is not
// a separator-free ticket file name is rejected outright, so path
// traversal never reaches the filesystem.
func (s *Store) FindExactAcross(baseName string) (domain.Ticket, string, []byte, error) {
	n, _, ferr := domain.ParseFilename(baseName)
	if ferr != nil || strings.ContainsAny(baseName, "/\\") {
		return domain.Ticket{}, "", nil, fmt.Errorf("store: FindExactAcross: not a ticket file name: %q", baseName)
	}
	entries, _, dirErr := s.scan()
	if dirErr != nil {
		return domain.Ticket{}, "", nil, fmt.Errorf("store: read dir %s: %w", s.Dir, dirErr)
	}
	matches := matchingEntries(baseName, s.Dir, entries, nil)
	archiveDir, archiveEntries, aerr := s.archiveScan()
	if aerr != nil {
		return domain.Ticket{}, "", nil, aerr
	}
	matches = matchingEntries(baseName, archiveDir, archiveEntries, matches)
	deletedDir, deletedEntries, derr := s.deletedScan()
	if derr != nil {
		return domain.Ticket{}, "", nil, derr
	}
	matches = matchingEntries(baseName, deletedDir, deletedEntries, matches)
	switch len(matches) {
	case 0:
		return domain.Ticket{}, "", nil, ErrNotFound
	case 1:
		return readLocated(matches[0])
	}
	return domain.Ticket{}, "", nil, ambiguousNumberError(n, matches)
}

// matchingEntries appends the entries named baseName to matches, each
// paired with dir. Extracted so FindExactAcross keeps one match loop per
// directory instead of three copies.
func matchingEntries(baseName, dir string, entries []fileEntry, matches []locatedFile) []locatedFile {
	for _, e := range entries {
		if e.Name == baseName {
			matches = append(matches, locatedFile{fileEntry: e, dir: dir})
		}
	}
	return matches
}

// LocateNumber returns the absolute paths of every ticket file carrying
// number n in s.Dir and s.Dir/archive. It is the scan-only half of the
// set/edit guard seam (T-0189 W3): no body is read and nothing is
// written, so the CLI can combine it with LocateNumberDeleted to refuse
// a deleted-only number (exit 1) or report a deleted+live ambiguity
// (exit 2, every path) BEFORE touching any file. The result is
// deduplicated and sorted so messages are stable. Error semantics mirror
// findAllNumber: an unreadable main directory or an invalid/unreadable
// archive propagates as an error.
func (s *Store) LocateNumber(n int) ([]string, error) {
	files, err := s.findAllNumber(n)
	if err != nil {
		return nil, err
	}
	return locatedPaths(files), nil
}

// LocateNumberDeleted returns the absolute paths of every ticket file
// carrying number n under tickets/deleted/ — the deleted half of the
// set/edit guard seam (see LocateNumber). Scan-only: no body is read and
// nothing is written, so a deleted file stays terminal and read-only.
// No deleted/ directory is not an error: the result is empty, so stores
// that predate W1 behave exactly as in W2. An invalid (symlink or file,
// T-0189) or unreadable deleted/ propagates as an error — the guard
// refuses rather than letting a set/edit act on a number it could not
// fully account for (deleted scan failures are never silenced).
func (s *Store) LocateNumberDeleted(n int) ([]string, error) {
	deletedDir, entries, err := s.deletedScan()
	if err != nil {
		return nil, err
	}
	var files []locatedFile
	for _, e := range entries {
		if e.Number == n {
			files = append(files, locatedFile{fileEntry: e, dir: deletedDir})
		}
	}
	return locatedPaths(files), nil
}

// locatedPaths renders locatedFiles as their absolute, deduplicated,
// sorted paths — the shape AmbiguousNumberError.Paths uses, shared by
// the Locate* guard seams so their output is directly composable into an
// ambiguity report.
func locatedPaths(files []locatedFile) []string {
	seen := make(map[string]struct{}, len(files))
	paths := make([]string, 0, len(files))
	for _, f := range files {
		p := filepath.Join(f.dir, f.Name)
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}
