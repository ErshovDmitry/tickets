// Package paths resolves the tickets directory. It is the single source of
// truth for ticket storage location (AGENTS_ARCHITECTURE.md §5).
//
// Resolution order — first match wins:
//  1. $TICKETS_DIR (explicit override for tests/CI)
//  2. Upward scan from cwd for a child directory named "tickets" (git-style)
//  3. Exe-relative: <dir-of-exe>/.. — the bash layout tickets/bin/ticket
//
// All filesystem access is limited to os.Stat and filepath.EvalSymlinks.
package paths

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Sentinel errors returned by Resolve. Use errors.Is to discriminate.
var (
	// ErrEnvNotDir means $TICKETS_DIR is set but does not point to an
	// existing directory (missing path, regular file, or stat failure).
	ErrEnvNotDir = errors.New("invalid TICKETS_DIR")
	// ErrNoExePath means the exe-relative tier received an empty
	// executable path. It is user-facing, so it carries the bash die()
	// "ticket: " prefix like ErrNotResolved (wave-1 #2, ambiguity A4).
	ErrNoExePath = errors.New("ticket: empty executable path")
	// ErrNotResolved means no tier located a tickets directory.
	ErrNotResolved = errors.New("ticket: cannot locate tickets dir")
)

const (
	envTicketsDir = "TICKETS_DIR"
	dirName       = "tickets"
)

// Resolve locates the tickets directory using the precedence described in
// the package comment. env is the environment map (injected for tests/CI;
// pass os.Environ-derived map in production). cwd must be an absolute path.
// exePath is the value of os.Executable(), which may be a symlink.
func Resolve(env map[string]string, cwd, exePath string) (string, error) {
	if v := env[envTicketsDir]; v != "" {
		return resolveEnv(v)
	}
	if dir, ok := scanUpward(cwd); ok {
		return dir, nil
	}
	if exePath == "" {
		return "", ErrNoExePath
	}
	if dir := fromExe(exePath); dir != "" {
		return dir, nil
	}
	return "", notResolved(env, cwd, exePath)
}

// resolveEnv validates the $TICKETS_DIR override: Abs -> Stat -> EvalSymlinks
// -> Stat -> IsDir. The first Stat catches a missing path cheaply; the second
// Stat guards the symlink target after resolution.
func resolveEnv(value string) (string, error) {
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("%w: abs %s: %v", ErrEnvNotDir, value, err)
	}
	// Any fs.ErrNotExist at the Stat/EvalSymlinks/Stat sequence maps to the
	// same clear hint: "path does not exist: <abs>" (plan §4 step 2).
	if _, err := os.Stat(abs); errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%w: path does not exist: %s", ErrEnvNotDir, abs)
	} else if err != nil {
		return "", fmt.Errorf("%w: stat %s: %w", ErrEnvNotDir, abs, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%w: path does not exist: %s", ErrEnvNotDir, abs)
	} else if err != nil {
		return "", fmt.Errorf("%w: evalsymlinks %s: %w", ErrEnvNotDir, abs, err)
	}
	info, err := os.Stat(resolved)
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%w: path does not exist: %s", ErrEnvNotDir, abs)
	} else if err != nil {
		return "", fmt.Errorf("%w: stat %s: %w", ErrEnvNotDir, abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: not a directory: %s", ErrEnvNotDir, resolved)
	}
	return resolved, nil
}

// scanUpward walks from cwd toward the filesystem root looking for a child
// directory named "tickets". A regular file named "tickets" is rejected
// (!IsDir). The walk stops when Dir(dir) == dir (filesystem or volume root).
// The accepted candidate is resolved through EvalSymlinks so parent
// symlinks do not leak into the returned path.
func scanUpward(cwd string) (string, bool) {
	dir := cwd
	for {
		cand := filepath.Join(dir, dirName)
		if info, err := os.Stat(cand); err == nil && info.IsDir() {
			resolved, err := filepath.EvalSymlinks(cand)
			if err == nil {
				return resolved, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// fromExe resolves the exe-relative tier: EvalSymlinks(exePath) -> Dir -> Dir
// (bin/.. = tickets/). It only accepts the exact <dir-of-exe>/.. layout
// where exe lives in a "bin" subdirectory and the grandparent is named
// "tickets". Any other layout returns ""; the caller then reports
// ErrNotResolved with the exe path as a hint.
func fromExe(exePath string) string {
	resolved, err := filepath.EvalSymlinks(exePath)
	if err != nil {
		return ""
	}
	if filepath.Base(filepath.Dir(resolved)) != "bin" {
		return ""
	}
	candidate := filepath.Dir(filepath.Dir(resolved))
	if filepath.Base(candidate) != dirName {
		return ""
	}
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate
	}
	return ""
}

// notResolved builds the final "nothing found" error with diagnostic hints.
// Hints are unquoted (plan §4 step 5): "$TICKETS_DIR=, cwd=<abs>, exe=<abs>".
func notResolved(env map[string]string, cwd, exePath string) error {
	return fmt.Errorf("%w (set TICKETS_DIR or run from project tree); tried: $TICKETS_DIR=%s, cwd=%s, exe=%s",
		ErrNotResolved, env[envTicketsDir], cwd, exePath)
}
