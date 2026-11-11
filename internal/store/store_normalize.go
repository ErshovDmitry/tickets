package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ticket/internal/domain"
)

// NormalizeOptions selects which ticket files a Normalize run touches.
// ProjectFilter ("" = every project) is compared case-sensitively with the
// ticket's own Project field, exactly as list's -P does; DryRun reports the
// files without writing a byte. T-0125.
type NormalizeOptions struct {
	ProjectFilter string
	DryRun        bool
}

// NormalizeResult is the outcome of one Normalize run: the absolute paths
// that were rewritten, in traversal order (with DryRun — the paths that
// WOULD be rewritten), plus the non-fatal parse and guard notices.
type NormalizeResult struct {
	Changed  []string
	Warnings []ParseWarning
}

// Normalize rewrites every ticket in s.Dir and s.Dir/archive whose bytes
// differ from the canonical domain.Render form, keeping the file name, its
// status (the file name is authoritative, V19) and the journal unchanged. It
// is the one-shot migration for files written before the markdownlint fix:
// no journal line is appended and no file is renamed, moved or archived.
//
// DryRun opens nothing writable and takes no lock, so it also works on a
// read-only tickets directory; a real run holds the per-store advisory lock
// once for the whole traversal. A write failure aborts the run, and
// NormalizeResult.Changed then holds the paths already rewritten.
func (s *Store) Normalize(o NormalizeOptions) (NormalizeResult, error) {
	if o.DryRun {
		return s.normalizeLocked(o)
	}
	var res NormalizeResult
	err := s.withLock(func() error {
		r, nerr := s.normalizeLocked(o)
		res = r
		return nerr
	})
	return res, err
}

// normalizeLocked walks the active directory and then the archive, in that
// order. The archive is optional: no archive/ is not an error, while an
// invalid (a symlink or a file, T-0078) or unreadable one is a single
// warning and the active directory's results still stand.
func (s *Store) normalizeLocked(o NormalizeOptions) (NormalizeResult, error) {
	var res NormalizeResult
	if err := s.normalizeDirLocked(s.Dir, o, &res); err != nil {
		return res, err
	}
	archiveDir, err := validArchiveDir(s.Dir)
	if err != nil {
		res.Warnings = append(res.Warnings, ParseWarning{Name: filepath.Join(s.Dir, "archive"), Err: err, Reason: archiveReason(err)})
		return res, nil
	}
	if archiveDir == "" {
		return res, nil
	}
	return res, s.normalizeDirLocked(archiveDir, o, &res)
}

// normalizeDirLocked normalizes every entry scanDir returns for dir; hidden
// files, .tmp artefacts and non-ticket names never reach it. An unreadable
// directory is a dir-read warning, not a failure: the caller keeps going.
func (s *Store) normalizeDirLocked(dir string, o NormalizeOptions, res *NormalizeResult) error {
	entries, warnings, err := scanDir(dir)
	res.Warnings = append(res.Warnings, warnings...)
	if err != nil {
		res.Warnings = append(res.Warnings, ParseWarning{Name: dir, Err: err, Reason: reasonDirRead})
		return nil
	}
	for _, e := range entries {
		if err := s.normalizeOneLocked(filepath.Join(dir, e.Name), e, o, res); err != nil {
			return err
		}
	}
	return nil
}

// normalizeOneLocked normalizes one ticket file in place. The order is fixed
// by the guard: read the raw bytes first, refuse to rewrite a file the guard
// flags (Render cannot reproduce those bytes), and only then render and
// compare. An unchanged file is a no-op in both modes.
func (s *Store) normalizeOneLocked(p string, e fileEntry, o NormalizeOptions, res *NormalizeResult) error {
	tk, raw, unknown, err := readTicketFile(p, e.Number)
	if err != nil {
		res.Warnings = append(res.Warnings, ParseWarning{Name: e.Name, Err: err, Reason: reasonOf(err)})
		return nil
	}
	if o.ProjectFilter != "" && string(tk.Project) != o.ProjectFilter {
		return nil
	}
	// The file name is authoritative (V19): a body status that disagrees is
	// replaced by the name below, never renamed. A body that STATES another
	// status is reported (a notice, not a skip), so the user learns that
	// normalize also fixes the body status instead of silently discarding it;
	// a body with no status line at all says nothing to contradict the name.
	if tk.Status != "" && tk.Status != e.Status {
		res.Warnings = append(res.Warnings, ParseWarning{
			Name:   e.Name,
			Err:    fmt.Errorf("body status %q differs from file name; the file name wins", tk.Status),
			Reason: reasonStatusFromName,
		})
	}
	tk.Status = e.Status
	// A file outside the selected project is not this run's business, so its
	// notices come after the filter — the same order as the guard warnings
	// below.
	res.Warnings = append(res.Warnings, journalDupWarnings(e.Name, tk)...)
	if reason := normalizeRenderable(tk, raw); reason != "" {
		res.Warnings = append(res.Warnings, ParseWarning{Name: e.Name, Err: normalizeGuardErr(reason), Reason: reason})
		return nil
	}
	rendered, rerr := domain.Render(tk, unknown)
	if rerr != nil {
		return fmt.Errorf("store: render %s: %w", e.Name, rerr)
	}
	if bytes.Equal(rendered, raw) {
		return nil
	}
	if o.DryRun {
		res.Changed = append(res.Changed, p)
		return nil
	}
	return s.commitNormalizedLocked(p, e, rendered, res)
}

// prepareReplace clears whatever would block a rename over path and returns
// a restore func that puts it back; the restore is never nil on a nil error,
// so callers may invoke it unconditionally. POSIX needs nothing here — a
// rename replaces a read-only target while the directory stays writable, and
// the mode bits travel with the new file — so the default is a no-op and
// normalize_windows.go installs the attribute-clearing implementation, the
// same "var default + platform override" shape as tmpfile.go and
// rename_windows.go in this package.
var prepareReplace = func(string) (func() error, error) {
	return func() error { return nil }, nil
}

// commitNormalizedLocked replaces p with rendered bytes: a same-directory tmp
// file (the commit must not cross a filesystem), the original permission
// bits, then an atomic rename over the target. A target whose read-only
// attribute cannot be cleared is a warning and a skip — the rest of the run
// continues — because a skip leaves the file exactly as it was.
func (s *Store) commitNormalizedLocked(p string, e fileEntry, rendered []byte, res *NormalizeResult) error {
	fi, err := os.Stat(p)
	if err != nil {
		return fmt.Errorf("store: stat %s: %w", e.Name, err)
	}
	perm := fi.Mode().Perm()
	tmpPath, err := s.writeTmp(e.Number, e.Status, rendered, filepath.Dir(p))
	if err != nil {
		return err
	}
	if cerr := os.Chmod(tmpPath, perm); cerr != nil {
		return fmt.Errorf("store: chmod %s: %w", e.Name, scrubFile(cerr, nil, tmpPath))
	}
	restore, perr := prepareReplace(p)
	if perr != nil {
		res.Warnings = append(res.Warnings, ParseWarning{
			Name:   e.Name,
			Err:    errors.Join(perr, removeTmp(tmpPath)),
			Reason: reasonReadOnlyTarget,
		})
		return nil
	}
	if rerr := renameFile(tmpPath, p); rerr != nil {
		return errors.Join(fmt.Errorf("store: rename %s: %w", e.Name, rerr), restore(), removeTmp(tmpPath))
	}
	res.Changed = append(res.Changed, p)
	s.Logger().Info("store.normalize.rewritten", "number", e.Number, "status", string(e.Status), "path", p)
	// The rename already happened, so a failed restore cannot fail the run:
	// the bytes are committed and only a platform attribute is left behind.
	// The file is still reported as changed and the warning names the reason;
	// the exit code stays 0 (a rewrite happened, the attribute did not).
	if rerr := restore(); rerr != nil {
		res.Warnings = append(res.Warnings, ParseWarning{
			Name:   e.Name,
			Err:    fmt.Errorf("%w: %v", errRestoreAttrs, rerr),
			Reason: reasonRestoreAttrs,
		})
	}
	return nil
}

// Warning errors behind the normalize reasons: the two guard skips (the file
// is left byte-identical) and the post-rename restore failure. They carry the
// user-facing sentence only; the reason code names the condition.
var (
	errUnrecognizedMeta = &parseErr{msg: "bash-era metadata labels, file left unchanged"}
	errIncompleteMeta   = &parseErr{msg: "incomplete metadata, file left unchanged"}
	// errRestoreAttrs labels a post-rename restore failure: the bytes ARE
	// committed, so it must never read like the read-only skip above.
	errRestoreAttrs = &parseErr{msg: "file rewritten, but its attributes could not be restored"}
)

// legacyMetaLabels are the bash-era metadata label names (docs_api/
// reference-init-tickets.md:107-110). matchMetaLine knows only the dict form
// ("- Status (Статус): open"), so these lines are neither captured as head
// metadata nor re-emitted by Render: a rewrite would drop them silently.
var legacyMetaLabels = []string{"Статус", "Приоритет", "Создан", "Проект"}

// normalizeRenderable reports why raw must not be rewritten, or "" when a
// Render round-trip is safe. Two independent checks, both on the raw bytes
// and both before any content decision:
//
//  1. a bash-era label line anywhere in the head region (the H1 line and the
//     metadata block, up to the first "## " header — the ParseHeader/Parse
//     secHead boundary) makes the render lossy even when all four dict meta
//     lines were parsed, because the unmatched line itself is not re-emitted;
//  2. an empty required field means the render would invent a value or lose
//     one, so the file is left for a human.
func normalizeRenderable(tk *domain.Ticket, raw []byte) string {
	if hasLegacyMetaLabels(raw) {
		return reasonUnrecognizedMeta
	}
	if tk.Type == "" || tk.Priority == "" || tk.Project == "" ||
		strings.TrimSpace(tk.Title) == "" || tk.Created.IsZero() {
		return reasonIncompleteMeta
	}
	return ""
}

// hasLegacyMetaLabels scans the head region of raw for a legacy label line.
// CRLF files are handled by trimming the CR of every line, exactly as the
// parser does, so a converted file cannot slip past the guard.
func hasLegacyMetaLabels(raw []byte) bool {
	for line := range bytes.SplitSeq(raw, []byte("\n")) {
		line = bytes.TrimSuffix(line, []byte("\r"))
		if bytes.HasPrefix(line, []byte("## ")) {
			return false
		}
		if isLegacyMetaLine(line) {
			return true
		}
	}
	return false
}

// isLegacyMetaLine reports whether line is a bash-era label bullet. The
// marker may be "-", "*" or "+", it may be indented, and a space may sit
// before the colon ("  - Статус : open"): all three are label lines a
// hand-edited or pre-tool file can carry, and Render cannot reproduce any of
// them. The label must be followed by the colon (optionally after spaces and
// tabs) so an ordinary word that merely starts with "Статус" is not matched.
func isLegacyMetaLine(line []byte) bool {
	line = bytes.TrimLeft(line, " \t")
	if len(line) == 0 || (line[0] != '-' && line[0] != '*' && line[0] != '+') {
		return false
	}
	line = bytes.TrimLeft(line[1:], " \t")
	for _, label := range legacyMetaLabels {
		if !bytes.HasPrefix(line, []byte(label)) {
			continue
		}
		rest := bytes.TrimLeft(line[len(label):], " \t")
		if len(rest) > 0 && rest[0] == ':' {
			return true
		}
	}
	return false
}

// normalizeGuardErr maps a guard reason to the warning error that carries it.
// Only the two reasons normalizeRenderable returns are known here, and an
// unknown one is refused by name instead of being reported as another
// guard's condition (that would hide a producer bug behind a wrong message).
func normalizeGuardErr(reason string) error {
	switch reason {
	case reasonUnrecognizedMeta:
		return errUnrecognizedMeta
	case reasonIncompleteMeta:
		return errIncompleteMeta
	default:
		return fmt.Errorf("store: unknown normalize guard reason %q", reason)
	}
}
