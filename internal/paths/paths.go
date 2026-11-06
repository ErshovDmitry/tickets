// Package paths resolves tickets directories in three levels: an explicit
// --tickets-dir/-C flag, $TICKETS_DIR, or an upward scan from cwd.
// All filesystem access is limited to os.Stat, os.Lstat, os.ReadDir
// and filepath.EvalSymlinks.
package paths

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"ticket/internal/domain"
)

var (
	ErrInvalidDir  = errors.New("invalid tickets dir")
	ErrNotResolved = errors.New("ticket: cannot locate tickets dir")

	// ErrNotRealDir reports a path that exists but is not a real directory:
	// a symlink (any target) or a non-directory. Uses Lstat semantics —
	// the trailing symlink is never followed (T-0082).
	ErrNotRealDir = errors.New("not a real directory")
)

const (
	envTicketsDir = "TICKETS_DIR"
	dirName       = "tickets"
	srcEnv        = envTicketsDir
	srcFlag       = "--tickets-dir"
	srcScan       = "scan"
)

// IsRealDir reports via os.Lstat (never follows a trailing symlink):
// (true, nil) — a real directory; (false, nil) — path absent;
// (false, wrapped ErrNotRealDir) — exists but not a real dir;
// (false, other error) — Lstat failure. T-0082.
func IsRealDir(path string) (bool, error) {
	li, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if !li.IsDir() {
		// Lstat of a symlink yields ModeSymlink, so IsDir is false for both
		// a symlink (any target) and a non-directory — same discrimination
		// as the store's validArchiveDir (2726abb).
		return false, fmt.Errorf("%s: %w", path, ErrNotRealDir)
	}
	return true, nil
}

// tier is the selected resolution tier: the value that selected it and the
// private source label used in diagnostics.
type tier struct {
	value string
	label string
}

// sourceTier picks the tier from the flag value and the env map only — no
// filesystem access and no process-environment reads. Precedence:
// --tickets-dir > $TICKETS_DIR > upward scan (T-0057 hub decision #2).
func sourceTier(env map[string]string, flagDir string) tier {
	switch {
	case flagDir != "":
		return tier{value: flagDir, label: srcFlag}
	case env[envTicketsDir] != "":
		return tier{value: env[envTicketsDir], label: srcEnv}
	default:
		return tier{label: srcScan}
	}
}

// Source reports which tier selected the tickets directory: the
// --tickets-dir flag label, the TICKETS_DIR env label or the scan label.
// It reads no process environment and touches no filesystem — only the
// given map and the flag value. The scan tier is reported even when an
// upward scan would find nothing: that is not detectable without
// filesystem access.
func Source(env map[string]string, flagDir string) string {
	return sourceTier(env, flagDir).label
}

func Resolve(env map[string]string, cwd, flagDir string) (string, error) {
	t := sourceTier(env, flagDir)
	// A tier with a nonempty value is an explicit tier (flag or env); the
	// scan tier carries an empty value. The label is only diagnostics text
	// and never decides the branch.
	if t.value != "" {
		return resolveExplicit(t.value, t.label)
	}
	if d, ok := scanUpward(cwd); ok {
		return d, nil
	}
	return "", notResolved(env, cwd, flagDir)
}
func resolveExplicit(value, source string) (string, error) {
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("%w (%s): abs %s: %v", ErrInvalidDir, source, value, err)
	}
	if _, err = os.Stat(abs); errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%w (%s): path does not exist: %s", ErrInvalidDir, source, abs)
	} else if err != nil {
		return "", fmt.Errorf("%w (%s): %w", ErrInvalidDir, source, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%w (%s): path does not exist: %s", ErrInvalidDir, source, abs)
	} else if err != nil {
		return "", fmt.Errorf("%w (%s): evalsymlinks %s: %w", ErrInvalidDir, source, abs, err)
	}
	info, err := os.Stat(resolved)
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%w (%s): path does not exist: %s", ErrInvalidDir, source, abs)
	} else if err != nil {
		return "", fmt.Errorf("%w (%s): %w", ErrInvalidDir, source, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w (%s): not a directory: %s", ErrInvalidDir, source, resolved)
	}
	if !isTicketsDir(resolved) {
		return "", fmt.Errorf("%w (%s): not a tickets directory (no .lock, archive/ or ticket files): %s — did you mean %s/tickets?", ErrInvalidDir, source, resolved, resolved)
	}
	return resolved, nil
}

// isTicketsDir reports whether dir looks like a tickets directory: it holds
// the .lock file, an archive/ subdirectory or at least one parseable ticket
// file. Unreadable and empty directories pass (the store reports real read
// errors; empty dirs allow bootstrap via `new`). Hidden (dot-prefixed) and
// *.tmp entries — transient write-probe and lock artifacts from parallel
// processes — do not count as foreign files.
func isTicketsDir(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return true // let the store report the real read error
	}
	for _, e := range entries {
		name := e.Name()
		if name == ".lock" {
			return true
		}
		if name == "archive" && e.IsDir() {
			return true
		}
		if _, _, perr := domain.ParseFilename(name); perr == nil {
			return true
		}
		if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".tmp") {
			continue // hidden/transient: not a foreign file
		}
		return false // a visible non-marker entry → not a tickets dir
	}
	return true // empty or only hidden/transient entries → bootstrap
}

// sameDir reports whether a and b denote the same directory. The comparison
// is best-effort: both paths are made absolute and cleaned, and when both
// exist they are compared through os.Stat/os.SameFile — the only check that
// also catches a relative TMPDIR, a symlinked temp dir (/tmp ->
// /private/tmp) and Windows case/8.3 aliases. The Abs+Clean arm is required
// because os.TempDir does not guarantee that the directory exists.
func sameDir(a, b string) bool {
	if abs, err := filepath.Abs(a); err == nil {
		a = abs
	}
	if abs, err := filepath.Abs(b); err == nil {
		b = abs
	}
	a, b = filepath.Clean(a), filepath.Clean(b)
	if a == b {
		return true
	}
	infoA, errA := os.Stat(a)
	infoB, errB := os.Stat(b)
	if errA != nil || errB != nil {
		return false
	}
	return os.SameFile(infoA, infoB)
}

// scanUpward walks from cwd to the filesystem root looking for a tickets
// subdirectory. The walk stops at the system temp root (os.TempDir) BEFORE
// that root is inspected: a temp dir is shared by every process on the host
// and is not a project root, so a leftover <temp-root>/tickets from an
// earlier smoke run must not hijack resolution from an unrelated cwd
// (T-0127). Projects INSIDE the temp root still self-locate. The scan stops
// before the temp root itself, so neither <temp-root>/tickets nor its
// ancestors are inspected.
func scanUpward(cwd string) (string, bool) {
	tempRoot := os.TempDir()
	for {
		if sameDir(cwd, tempRoot) {
			return "", false
		}
		c := filepath.Join(cwd, dirName)
		if i, e := os.Stat(c); e == nil && i.IsDir() {
			if r, e := filepath.EvalSymlinks(c); e == nil {
				return r, true
			}
		}
		p := filepath.Dir(cwd)
		if p == cwd {
			return "", false
		}
		cwd = p
	}
}
func notResolved(env map[string]string, cwd, flagDir string) error {
	return fmt.Errorf("%w (pass --tickets-dir/-C <path>, set TICKETS_DIR, or run from the project tree); tried: --tickets-dir=%s, $TICKETS_DIR=%s, cwd=%s", ErrNotResolved, flagDir, env[envTicketsDir], cwd)
}
