package store

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ticket/internal/domain"
)

// fileEntry is a single ticket file's parsed identifier.
type fileEntry struct {
	Number int
	Status domain.Status
	Name   string // basename only, e.g. "T-0001-open.md"
}

// scan reads s.Dir and returns parsed file entries (sorted by number)
// plus warnings for files the codec rejected. The function returns a
// non-nil error only when the directory itself is unreadable; List
// converts that error into a warning so callers never get a hard
// failure on a partially broken directory.
func (s *Store) scan() ([]fileEntry, []ParseWarning, error) {
	return scanDir(s.Dir)
}

// scanDir reads dirPath and returns parsed file entries (sorted by number)
// plus warnings. Extracted for archive/ support.
func scanDir(dirPath string) ([]fileEntry, []ParseWarning, error) {
	raw, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, nil, err
	}
	var (
		out  []fileEntry
		warn []ParseWarning
	)
	for _, de := range raw {
		name := de.Name()
		// Skip hidden files (incl. .lock) and unfinished .tmp artefacts.
		if strings.HasPrefix(name, ".") {
			continue
		}
		if strings.HasSuffix(name, ".tmp") {
			continue
		}
		// Skip directories unconditionally (e.g. archive/).
		if de.IsDir() {
			continue
		}
		// Reject every non-regular entry (symlinks, FIFOs, devices): a
		// ticket-named symlink makes ReadFile follow its target outside
		// TICKETS_DIR, so it must never be scanned as a ticket. Loud
		// warning; the entry is skipped (V19 containment). DirEntry.Type()
		// is accurate even where the raw dirent type is unknown (the os
		// package falls back to lstat), and IsRegular is portable to
		// Windows (no type bits set for regular files).
		if !de.Type().IsRegular() {
			warn = append(warn, ParseWarning{Name: name, Err: errNotRegularFile})
			continue
		}
		if !strings.HasSuffix(name, ".md") {
			// Loudly record non-md files as warnings (helps spot typos
			// like a stray README); the plan keeps List never-failing.
			// Hidden and .tmp names never reach this branch: they were
			// skipped above.
			warn = append(warn, ParseWarning{Name: name, Err: errBadExtension})
			continue
		}
		n, st, perr := domain.ParseFilename(name)
		if perr != nil {
			warn = append(warn, ParseWarning{Name: name, Err: perr})
			continue
		}
		out = append(out, fileEntry{Number: n, Status: st, Name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out, warn, nil
}

// errBadExtension is reported when a non-md file slips past the dot/tmp
// filters. It's a sentinel of convenience so tests can match without
// reaching into the package internals.
var errBadExtension = &parseErr{msg: "unexpected non-.md extension"}

// errNotRegularFile is reported for ticket-named entries that are not
// regular files (symlinks, FIFOs, devices). They are skipped: reading
// through them would follow a symlink target outside TICKETS_DIR.
var errNotRegularFile = &parseErr{msg: "not a regular file"}

// parseErr is a lightweight error type so ParseWarning.Err stays opaque
// to callers while remaining comparable in tests.
type parseErr struct{ msg string }

func (e *parseErr) Error() string { return e.msg }

// maxNumberLocked returns the highest ticket number present in s.Dir
// and s.Dir/archive, or 0 when no parseable files exist. Archive numbers
// are included to prevent reuse after archival.
func (s *Store) maxNumberLocked() int {
	entries, _, err := s.scan()
	if err != nil {
		return 0
	}
	var m int
	for _, e := range entries {
		if e.Number > m {
			m = e.Number
		}
	}
	// Check archive/ for max number.
	archiveDir := filepath.Join(s.Dir, "archive")
	if _, serr := os.Stat(archiveDir); serr == nil {
		archiveEntries, _, _ := scanDir(archiveDir)
		for _, e := range archiveEntries {
			if e.Number > m {
				m = e.Number
			}
		}
	}
	return m
}
