package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

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
// Error semantics: ErrNotFound for absent numbers (including the
// lockless empty-file window); *AmbiguousNumberError when more than one
// file in s.Dir or s.Dir/archive carries n — the ambiguity is detected
// BEFORE any body is read, so no arbitrary first match is ever returned
// (T-0157); a wrapped error for corrupt files, for entries rejected by
// the validated open and for an unreadable directory or archive.
//
// Scan order: the main directory (s.Dir) first, then the archive
// subdirectory (s.Dir/archive).
func (s *Store) FindRaw(n int) (domain.Ticket, string, []byte, error) {
	files, err := s.findAllNumber(n)
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

// FindNamed returns ticket n together with the exact base name of the
// scanned file it was read from. Delegates to FindRaw, discarding the
// raw bytes; signature and error semantics are unchanged.
func (s *Store) FindNamed(n int) (domain.Ticket, string, error) {
	tk, name, _, err := s.FindRaw(n)
	return tk, name, err
}

// readLocated reads the located file's body, mapping an empty file to
// ErrNotFound (the lockless window: the file exists but its body has not
// landed yet) and any other read failure to an error carrying the base
// name. Shared by FindRaw and FindExact so both keep identical
// semantics.
func readLocated(loc locatedFile) (domain.Ticket, string, []byte, error) {
	tk, raw, _, err := readTicketFile(filepath.Join(loc.dir, loc.Name), loc.Number)
	if err != nil {
		if errors.Is(err, errEmptyTicket) {
			return domain.Ticket{}, "", nil, ErrNotFound
		}
		return domain.Ticket{}, "", nil, fmt.Errorf("store: read %s: %w", loc.Name, err)
	}
	// The scan-matched filename status is authoritative (V19); the parsed
	// body status is untrusted.
	tk.Status = loc.Status
	return *tk, loc.Name, raw, nil
}

// FindExact resolves a ticket by its exact base name (T-NNNN-<status>.md)
// across s.Dir and s.Dir/archive — the escape hatch for an ambiguous
// number (T-0157): the caller passes a name it took from a listing and
// gets exactly that file. ErrNotFound for an absent name; the same base
// name present in both directories is *AmbiguousNumberError. A name that
// is not a separator-free ticket file name is rejected outright, so path
// traversal never reaches the filesystem.
func (s *Store) FindExact(baseName string) (domain.Ticket, string, []byte, error) {
	n, _, ferr := domain.ParseFilename(baseName)
	if ferr != nil || strings.ContainsAny(baseName, "/\\") {
		return domain.Ticket{}, "", nil, fmt.Errorf("store: FindExact: not a ticket file name: %q", baseName)
	}
	entries, _, dirErr := s.scan()
	if dirErr != nil {
		return domain.Ticket{}, "", nil, fmt.Errorf("store: read dir %s: %w", s.Dir, dirErr)
	}
	var matches []locatedFile
	for _, e := range entries {
		if e.Name == baseName {
			matches = append(matches, locatedFile{fileEntry: e, dir: s.Dir})
		}
	}
	archiveDir, archiveEntries, aerr := s.archiveScan()
	if aerr != nil {
		return domain.Ticket{}, "", nil, aerr
	}
	for _, e := range archiveEntries {
		if e.Name == baseName {
			matches = append(matches, locatedFile{fileEntry: e, dir: archiveDir})
		}
	}
	switch len(matches) {
	case 0:
		return domain.Ticket{}, "", nil, ErrNotFound
	case 1:
		return readLocated(matches[0])
	}
	return domain.Ticket{}, "", nil, ambiguousNumberError(n, matches)
}

// findLocked scans s.Dir then s.Dir/archive for ticket n, returning the
// fileEntry and the directory where it was found. It is ambiguity-safe
// (T-0157): when more than one file carries n — in s.Dir, in
// s.Dir/archive, or one in each — it returns *AmbiguousNumberError
// instead of an arbitrary first match, so no caller can ever act on the
// wrong file. ErrNotFound for an absent number; a wrapped error for an
// unreadable main directory; archive errors (invalid path, T-0078, or an
// unreadable archive directory) propagate as they do elsewhere.
//
// It is not used by SetStatus or Archive: those go through
// findAllNumberStrict / findOneLocked.
func (s *Store) findLocked(n int) (fileEntry, string, error) {
	files, err := s.findAllNumberStrict(n, "store.find.failure")
	if err != nil {
		return fileEntry{}, "", err
	}
	if len(files) == 0 {
		return fileEntry{}, "", ErrNotFound
	}
	return files[0].fileEntry, files[0].dir, nil
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

// ambiguousNumberError builds the T-0157 ambiguity error for number n
// from the located files carrying it: their absolute paths, deduplicated
// and sorted so the message is stable. Callers exclude their own file.
func ambiguousNumberError(n int, files []locatedFile) *AmbiguousNumberError {
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
	return &AmbiguousNumberError{Number: n, Paths: paths}
}

// findAllNumberStrict is findAllNumber plus the T-0157 ambiguity gate: a
// number carried by more than one file yields *AmbiguousNumberError and
// no files, so a mutation never acts on an arbitrary first match. event
// is the caller's failure log event; stage=ambiguous is added here.
func (s *Store) findAllNumberStrict(n int, event string) ([]locatedFile, error) {
	files, err := s.findAllNumber(n)
	if err != nil || len(files) <= 1 {
		return files, err
	}
	s.Logger().Error(event, "number", n, "stage", "ambiguous")
	return nil, ambiguousNumberError(n, files)
}

// findOneLocked is findAllNumberStrict narrowed to exactly one file:
// ErrNotFound for an absent number, *AmbiguousNumberError for a
// duplicated one.
func (s *Store) findOneLocked(n int, event string) (locatedFile, error) {
	files, err := s.findAllNumberStrict(n, event)
	if err != nil {
		return locatedFile{}, err
	}
	if len(files) == 0 {
		return locatedFile{}, ErrNotFound
	}
	return files[0], nil
}
