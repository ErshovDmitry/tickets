package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"ticket/internal/domain"
	"ticket/internal/paths"
)

func cmdInit(lang domain.Lang, stdout, stderr io.Writer) int {
	cwd, err := os.Getwd()
	if err != nil {
		return initError(lang, stderr, err)
	}
	return initProject(cwd, lang, stdout, stderr)
}
func initProject(cwd string, lang domain.Lang, stdout, stderr io.Writer) int {
	tickets := filepath.Join(cwd, "tickets")
	if li, err := os.Lstat(tickets); err == nil {
		if li.Mode()&os.ModeSymlink != 0 {
			// Symlink: follow once. Broken link, unresolvable target (ELOOP)
			// or a non-directory target is a conflict; a symlink to a real
			// directory proceeds (behavior preserved, T-0062).
			// target is a real dir: proceed
			if info, serr := os.Stat(tickets); serr != nil || !info.IsDir() {
				return conflict(lang, stderr, tickets)
			}
		} else if !li.IsDir() {
			return conflict(lang, stderr, tickets)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return initError(lang, stderr, err)
	}
	archive := filepath.Join(tickets, "archive")
	deleted := filepath.Join(tickets, "deleted")
	// Probe both subdirs before creating anything (W1 review A2): a
	// conflict on one must not leave a partial side effect — creating
	// archive/ and then bailing out on deleted/ would be one. A
	// pre-planted symlink, regular file or broken symlink is a conflict,
	// never overwritten (T-0082, T-0189).
	for _, sub := range []string{archive, deleted} {
		if _, err := paths.IsRealDir(sub); err != nil {
			if errors.Is(err, paths.ErrNotRealDir) {
				return conflict(lang, stderr, sub)
			}
			return initError(lang, stderr, err)
		}
	}
	if err := os.MkdirAll(archive, 0o755); err != nil {
		return initError(lang, stderr, err)
	}
	if err := os.MkdirAll(deleted, 0o755); err != nil {
		return initError(lang, stderr, err)
	}
	fmt.Fprintln(stdout, domain.MsgInitialized(lang, tickets))
	return exitOK
}
func initError(lang domain.Lang, w io.Writer, e error) int {
	fmt.Fprintln(w, domain.ErrInitError(lang, e))
	return exitError
}
func conflict(lang domain.Lang, w io.Writer, p string) int {
	fmt.Fprintln(w, domain.ErrConflict(lang, p))
	return exitError
}
