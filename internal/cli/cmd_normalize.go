package cli

import (
	"fmt"
	"io"
	"strings"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// normalizeFlags is the parsed argument set of "normalize" (T-0125).
type normalizeFlags struct {
	project string
	dryRun  bool
}

// parseNormalizeFlags parses "normalize [-P <project>] [--dry-run]" without
// any I/O and returns the first error message instead of printing it, so the
// dispatch site keeps the only Parse and the only print (-h handling and the
// store selection depend on the result). -P takes the next argument, the last
// -P wins, and an empty or whitespace-only project is rejected like list's;
// --dry-run is a bare idempotent switch. A positional argument and any other
// flag are rejected: normalize has no positional form and no --json.
func parseNormalizeFlags(args []string, lang domain.Lang) (normalizeFlags, string) {
	var f normalizeFlags
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-P":
			if i+1 >= len(args) {
				return f, domain.ErrFlagPRequiresValue(lang)
			}
			i++
			if strings.TrimSpace(args[i]) == "" {
				return f, domain.ErrFlagPRequiresNonEmpty(lang)
			}
			f.project = args[i]
		case a == "--dry-run":
			f.dryRun = true
		case strings.HasPrefix(a, "-"):
			return f, domain.ErrUnknownFlagList(lang, a)
		default:
			return f, domain.ErrUnknownArgument(lang, a)
		}
	}
	return f, ""
}

// cmdNormalize implements "normalize [-P <project>] [--dry-run]": rewrite
// every ticket whose bytes differ from the canonical domain.Render form,
// keeping the file name, its status and the journal unchanged (T-0125 — the
// one-shot migration that a status change used to do lazily, at the price of
// a journal line per file).
//
// Output follows the repository convention: changed paths go to stdout, one
// per line (the set/archive path shape; with --dry-run they are the paths
// that WOULD change); warnings go to stderr as "ticket: <text>" and never
// change the exit code; a store failure is "ticket: <error>" and exit 1.
func cmdNormalize(st *store.Store, f normalizeFlags, lang domain.Lang, stdout, stderr io.Writer) int {
	res, err := st.Normalize(store.NormalizeOptions{ProjectFilter: f.project, DryRun: f.dryRun})
	logParseWarnings(st, res.Warnings)
	for _, w := range res.Warnings {
		fmt.Fprintf(stderr, "ticket: %s\n", w.Error())
	}
	reportDuplicates(st, lang, stderr)
	// Paths first: a mid-run write failure still reports what it rewrote.
	for _, path := range res.Changed {
		fmt.Fprintln(stdout, path)
	}
	if err != nil {
		fmt.Fprintf(stderr, "ticket: %v\n", err)
		return exitError
	}
	return exitOK
}
