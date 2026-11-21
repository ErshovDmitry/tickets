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

// cmdInit initializes the tickets store at the target resolved from the
// global -C/--tickets-dir flag, then $TICKETS_DIR, then <cwd>/tickets
// (T-0223, GitHub issue #49). Init stays outside runLogged: no logger.
func cmdInit(env map[string]string, flagDir string, lang domain.Lang, stdout, stderr io.Writer) int {
	cwd, err := os.Getwd()
	if err != nil {
		return initError(lang, stderr, err)
	}
	target, err := paths.ResolveInitTarget(env, cwd, flagDir)
	if err != nil {
		return initError(lang, stderr, err)
	}
	return initStore(target, lang, stdout, stderr)
}

// initProject is the scan-tier wrapper kept for the existing white-box
// tests: initialize <cwd>/tickets. cmdInit resolves the real target via
// paths.ResolveInitTarget and calls initStore directly.
func initProject(cwd string, lang domain.Lang, stdout, stderr io.Writer) int {
	return initStore(filepath.Join(cwd, "tickets"), lang, stdout, stderr)
}

// initStore creates archive/ and deleted/ inside ticketsDir — the FINAL
// store directory. The conflict/symlink probing below is tier-agnostic
// (T-0062/T-0082/T-0189): it must behave identically for a scan-tier
// <cwd>/tickets and an explicit -C/$TICKETS_DIR target.
func initStore(ticketsDir string, lang domain.Lang, stdout, stderr io.Writer) int {
	tickets := ticketsDir
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
