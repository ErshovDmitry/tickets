// Package paths resolves tickets directories in three levels: an explicit
// --tickets-dir/-C flag, $TICKETS_DIR, or an upward scan from cwd.
// All filesystem access is limited to os.Stat and filepath.EvalSymlinks.
package paths

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

var (
	ErrInvalidDir  = errors.New("invalid tickets dir")
	ErrNotResolved = errors.New("ticket: cannot locate tickets dir")
)

const (
	envTicketsDir = "TICKETS_DIR"
	dirName       = "tickets"
	srcEnv        = "TICKETS_DIR"
	srcFlag       = "--tickets-dir"
)

func Resolve(env map[string]string, cwd, flagDir string) (string, error) {
	if flagDir != "" {
		return resolveExplicit(flagDir, srcFlag)
	}
	if v := env[envTicketsDir]; v != "" {
		return resolveExplicit(v, srcEnv)
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
		return "", fmt.Errorf("%w (%s): stat %s: %w", ErrInvalidDir, source, abs, err)
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
		return "", fmt.Errorf("%w (%s): stat %s: %w", ErrInvalidDir, source, resolved, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w (%s): not a directory: %s", ErrInvalidDir, source, resolved)
	}
	return resolved, nil
}
func scanUpward(cwd string) (string, bool) {
	for {
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
