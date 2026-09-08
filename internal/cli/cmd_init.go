package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"ticket/internal/paths"
)

func cmdInit(stdout, stderr io.Writer) int {
	cwd, err := os.Getwd()
	if err != nil {
		return initError(stderr, err)
	}
	return initProject(cwd, stdout, stderr)
}
func initProject(cwd string, stdout, stderr io.Writer) int {
	tickets := filepath.Join(cwd, "tickets")
	if li, err := os.Lstat(tickets); err == nil {
		if li.Mode()&os.ModeSymlink != 0 {
			// Symlink: follow once. Broken link, unresolvable target (ELOOP)
			// or a non-directory target is a conflict; a symlink to a real
			// directory proceeds (behavior preserved, T-0062).
			// target is a real dir: proceed
			if info, serr := os.Stat(tickets); serr != nil || !info.IsDir() {
				return conflict(stderr, tickets)
			}
		} else if !li.IsDir() {
			return conflict(stderr, tickets)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return initError(stderr, err)
	}
	archive := filepath.Join(tickets, "archive")
	if _, err := paths.IsRealDir(archive); err != nil {
		if errors.Is(err, paths.ErrNotRealDir) {
			return conflict(stderr, archive) // pre-planted symlink/file (T-0082)
		}
		return initError(stderr, err)
	}
	if err := os.MkdirAll(archive, 0o755); err != nil {
		return initError(stderr, err)
	}
	fmt.Fprintf(stdout, "Инициализировано: %s\n", tickets)
	return 0
}
func initError(w io.Writer, e error) int {
	fmt.Fprintf(w, "ticket: Ошибка инициализации: %v\n", e)
	return 1
}
func conflict(w io.Writer, p string) int {
	fmt.Fprintf(w, "ticket: Конфликт: %s\n", p)
	return 1
}
