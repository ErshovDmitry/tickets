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

// findAllNumberAcross is findAllNumber widened with tickets/deleted/: the one
// shared main+archive+deleted scan (scanReadTree) plus the one match rule
// (numberMatches), so the deleted-aware lookup cannot disagree with the
// deleted-blind one. Deleted scan errors propagate exactly like the archive
// scan error.
func (s *Store) findAllNumberAcross(n int) ([]locatedFile, error) {
	return s.findNumberIn(n, true)
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
	matches, n, err := s.matchExactName(baseName, "FindExactAcross", true)
	if err != nil {
		return domain.Ticket{}, "", nil, err
	}
	switch len(matches) {
	case 0:
		return domain.Ticket{}, "", nil, ErrNotFound
	case 1:
		return readLocated(matches[0])
	}
	return domain.Ticket{}, "", nil, ambiguousNumberError(n, matches)
}

// readShowLocated reads one located file for the show read API: the ticket,
// its exact bytes, the unknown trailing bytes and the warnings attributable to
// it — the scan warnings naming exactly this file plus the journal-dup notice
// of a ticket whose second "## Journal" was merged by Parse.
func readShowLocated(loc locatedFile, sc locatedScan) (domain.Ticket, string, []byte, []byte, []ParseWarning, error) {
	tk, raw, unknown, err := readLocatedShow(loc)
	if err != nil {
		return domain.Ticket{}, "", nil, nil, nil, err
	}
	attributed := sc.attributableWarnings(loc)
	attributed = append(attributed, journalDupWarnings(loc.Name, &tk)...)
	return tk, loc.Name, raw, unknown, attributed, nil
}

// FindShowAcross is FindRawAcross widened for the show read path (T-0241): it
// adds the unknown trailing bytes and the parse warnings attributable to the
// ticket, so neither a "## ..." tail nor a merged duplicate journal is lost.
// Everything else matches FindRawAcross: main+archive+deleted, the scan-matched
// name (never a path derived from the body status), the ambiguity detected
// before any body is read, ErrNotFound for an absent number.
func (s *Store) FindShowAcross(n int) (domain.Ticket, string, []byte, []byte, []ParseWarning, error) {
	sc, err := s.scanReadTree(true)
	if err != nil {
		return domain.Ticket{}, "", nil, nil, nil, err
	}
	files := sc.numberMatches(n)
	switch len(files) {
	case 0:
		return domain.Ticket{}, "", nil, nil, nil, ErrNotFound
	case 1:
		return readShowLocated(files[0], sc)
	}
	return domain.Ticket{}, "", nil, nil, nil, ambiguousNumberError(n, files)
}

// FindExactShowAcross is FindExactAcross widened the same way: the exact-name
// escape hatch (T-0157) across main, archive and deleted, carrying the unknown
// tail and the attributable warnings.
func (s *Store) FindExactShowAcross(baseName string) (domain.Ticket, string, []byte, []byte, []ParseWarning, error) {
	sc, err := s.scanReadTree(true)
	if err != nil {
		return domain.Ticket{}, "", nil, nil, nil, err
	}
	matches, n, err := scanExactName(sc, baseName, "FindExactShowAcross")
	if err != nil {
		return domain.Ticket{}, "", nil, nil, nil, err
	}
	switch len(matches) {
	case 0:
		return domain.Ticket{}, "", nil, nil, nil, ErrNotFound
	case 1:
		return readShowLocated(matches[0], sc)
	}
	return domain.Ticket{}, "", nil, nil, nil, ambiguousNumberError(n, matches)
}

// matchExactName is the exact-name lookup for the byte-only variants
// (FindExact, FindExactAcross): it scans the requested tree, applies the shared
// name rule and rejects a non-ticket file name outright, so path traversal
// never reaches the filesystem and every exact-name caller shares one
// validation.
func (s *Store) matchExactName(baseName, caller string, withDeleted bool) ([]locatedFile, int, error) {
	sc, err := s.scanReadTree(withDeleted)
	if err != nil {
		return nil, 0, err
	}
	return scanExactName(sc, baseName, caller)
}

// scanExactName validates baseName and resolves it inside an already performed
// scan. It is the single name-match rule behind FindExact, FindExactAcross and
// FindExactShowAcross.
func scanExactName(sc locatedScan, baseName, caller string) ([]locatedFile, int, error) {
	n, _, ferr := domain.ParseFilename(baseName)
	if ferr != nil || strings.ContainsAny(baseName, "/\\") {
		return nil, 0, fmt.Errorf("store: %s: not a ticket file name: %q", caller, baseName)
	}
	return sc.nameMatches(baseName), n, nil
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
