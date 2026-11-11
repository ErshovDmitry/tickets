package cli

// cmd_update_check.go implements `ticket update-check` (T-0173): the
// explicit check for a newer release on GitHub. The command never runs
// automatically and never blocks other commands; it needs no tickets dir
// and no store, so cli.go routes it beside init, outside runLogged/dispatch.

import (
	"fmt"
	"io"

	"ticket/internal/domain"
	"ticket/internal/releasecheck"
)

// Seams for white-box tests, cf. domainInitFn/langFromFn in cli.go: stubbing
// them keeps every update-check test hermetic (no network, no user cache dir).
var (
	defaultOptionsFn = releasecheck.DefaultOptions
	checkReleasesFn  = releasecheck.Check
)

// cmdUpdateCheck runs the T-0173 explicit release check and prints one
// localized line. Exit contract (see exitcodes.go): exitOK for up to date /
// no releases / dev build, exitUpdateAvailable when a newer release exists
// (version and URL on stdout), exitError when the check itself failed
// (cache dir unresolvable, network/API/cache failure — the cause goes to
// stderr via domain.ErrUpdateCheck). No store, no tickets dir, no logger.
func cmdUpdateCheck(lang domain.Lang, current string, stdout, stderr io.Writer) int {
	opts, err := defaultOptionsFn(current)
	if err != nil {
		fmt.Fprintln(stderr, domain.ErrUpdateCheck(lang, err))
		return exitError
	}
	res := checkReleasesFn(opts)
	switch res.State {
	case releasecheck.UpToDate:
		fmt.Fprintln(stdout, domain.UpdateCheckUpToDate(lang, current))
		return exitOK
	case releasecheck.NoReleases:
		fmt.Fprintln(stdout, domain.UpdateCheckNoReleases(lang))
		return exitOK
	case releasecheck.Dev:
		fmt.Fprintln(stdout, domain.UpdateCheckDev(lang, current))
		return exitOK
	case releasecheck.UpdateAvailable:
		fmt.Fprintln(stdout, domain.UpdateCheckAvailable(lang, res.Latest, res.URL))
		return exitUpdateAvailable
	case releasecheck.Failed: // Result.Err is always non-nil there (T-0173)
		fmt.Fprintln(stderr, domain.ErrUpdateCheck(lang, res.Err))
		return exitError
	default:
		// A State this binary does not know: name it honestly on stderr
		// instead of silently misreporting it as a check failure.
		fmt.Fprintln(stderr, domain.ErrUpdateCheck(lang, fmt.Errorf("unhandled release check state %d", int(res.State))))
		return exitError
	}
}
