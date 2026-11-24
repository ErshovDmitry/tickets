package cli

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// cmdShow implements `show <номер|имя-файла>`: look the ticket up by number
// and print its file verbatim — or select one of the T-0241 modes (--section,
// --json; see cmd_show_modes.go).
// lang selects the usage text language for argument errors.
// Signature per wave-1 dispatch contract (wiki 8bd93a4e, A1).
func cmdShow(st *store.Store, args []string, _, _ string, lang domain.Lang, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		printUsage(stdout, stderr, lang)
		return exitError
	}
	// T-0241: a bare --section/--json token selects a new mode with strict
	// validation. Without one the plain path stays exactly as before: any
	// extra argument (unknown flag included) is a warning on stderr and the
	// exit code stays 0.
	if hasShowModeFlag(args) {
		return runShowModes(st, args, lang, stdout, stderr)
	}
	// T-0076: extra arguments are ignored (as in the bash era), but the silent
	// truncation is surfaced as a warning; the exit code stays 0.
	if len(args) > 1 {
		fmt.Fprintln(stderr, domain.WarnExtraArgsIgnored(lang, strings.Join(args[1:], " ")))
	}
	// The lookup is TOCTOU-safe, and its parse warnings are DROPPED here: the
	// plain mode prints the file bytes on stdout and nothing else, so its
	// output stays byte-identical to the pre-T-0241 binary.
	_, _, raw, _, _, err := locateShow(st, args[0])
	if err != nil {
		return reportShowArgError(st, stderr, lang, args[0], err)
	}
	if _, err := stdout.Write(raw); err != nil {
		return reportShowWriteError(stderr, lang, err)
	}
	return exitOK
}

// reportShowArgError maps every locateShow failure to the pinned diagnostics:
// an ambiguous number is the ONLY exit-2 case and lists every conflicting
// path; an absent ticket gets the «не найден» report plus the numeric hint;
// an argument with no digits is named as such (the not-found report would
// mislead); read-only and any other store error are real errors, exit 1.
func reportShowArgError(st *store.Store, stderr io.Writer, lang domain.Lang, arg string, err error) int {
	if errors.Is(err, errShowArgNonNumeric) {
		fmt.Fprintln(stderr, domain.ErrTicketArgNonNumeric(lang, arg))
		return exitError
	}
	var amb *store.AmbiguousNumberError
	if errors.As(err, &amb) {
		return reportAmbiguous(stderr, lang, amb)
	}
	if errors.Is(err, store.ErrNotFound) {
		return notFoundHinted(st, stderr, lang, arg)
	}
	return reportShowWriteError(stderr, lang, err)
}

// reportShowWriteError renders a store/stdout failure of show: the read-only
// store message when the store refused, the raw error otherwise. It never
// masks the cause as «не найден».
func reportShowWriteError(stderr io.Writer, lang domain.Lang, err error) int {
	if errors.Is(err, store.ErrReadOnly) {
		fmt.Fprintln(stderr, domain.ErrReadOnly(lang))
	} else {
		fmt.Fprintf(stderr, "ticket: %v\n", err)
	}
	return exitError
}

// notFound prints the established «не найден» message; the quoted
// value is the user's original argument.
func notFound(stderr io.Writer, lang domain.Lang, arg string) int {
	fmt.Fprintln(stderr, domain.ErrTicketNotFound(lang, arg))
	return exitError
}

// notFoundHinted reports «не найден» and, for a numeric argument, adds
// ONE hint line chosen from the store's actual state (main + archive +
// deleted listings; the max is never hardcoded). Branch order is deliberate:
// n==0 → «номера начинаются с 1»; max==0 → «хранилище пусто», unless
// the scan rejected present files (warnings) — then the generic hint,
// so emptiness is never claimed for a store whose files were skipped;
// n>max → the real bound; in-range miss → the generic hint (show
// searches archive/ and deleted/ too). Scan warnings are not printed here.
func notFoundHinted(st *store.Store, stderr io.Writer, lang domain.Lang, arg string) int {
	rc := notFound(stderr, lang, arg)
	n, ok := parseTicketNumber(arg)
	if !ok {
		return rc
	}
	maxNum, warned := scanMax(st)
	switch {
	case n == 0:
		fmt.Fprintln(stderr, domain.HintNumbersStartAtOne(lang))
	case maxNum == 0 && warned:
		fmt.Fprintln(stderr, domain.HintUseList(lang))
	case maxNum == 0:
		fmt.Fprintln(stderr, domain.HintStoreEmpty(lang))
	case n > maxNum:
		fmt.Fprintln(stderr, domain.HintNumbersUpTo(lang, maxNum))
	default:
		fmt.Fprintln(stderr, domain.HintUseList(lang))
	}
	return rc
}

// scanMax returns the highest ticket number across the main, archive and
// deleted listings (numbering includes deleted/, T-0189) and whether any
// of the scans produced warnings. The deleted listing widens the
// reported bound and is itself readable by show (T-0189), so the bound
// and the set of numbers show can print stay in sync.
func scanMax(st *store.Store) (maxNum int, warned bool) {
	main, warns := st.List()
	archive, archiveWarns := st.ListArchive()
	deleted, deletedWarns := st.ListDeleted()
	for _, t := range main {
		if t.Number > maxNum {
			maxNum = t.Number
		}
	}
	for _, t := range archive {
		if t.Number > maxNum {
			maxNum = t.Number
		}
	}
	for _, t := range deleted {
		if t.Number > maxNum {
			maxNum = t.Number
		}
	}
	return maxNum, len(warns) > 0 || len(archiveWarns) > 0 || len(deletedWarns) > 0
}

// reportAmbiguous prints the pinned T-0157 ambiguity report — one
// localized header line naming the number, then every conflicting
// absolute path on its own indented line — and yields exitAmbiguous.
// Shared by show, set, archive and new; the store carries no UI text.
func reportAmbiguous(stderr io.Writer, lang domain.Lang, amb *store.AmbiguousNumberError) int {
	fmt.Fprintln(stderr, domain.ErrAmbiguousNumber(lang, amb.Number))
	for _, p := range amb.Paths {
		fmt.Fprintf(stderr, "  %s\n", p)
	}
	return exitAmbiguous
}

// parseTicketNumber strips every non-digit and parses the remainder
// base-10 (both inherited from the bash era). ok=false when no
// digits remain or the number overflows.
func parseTicketNumber(arg string) (int, bool) {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, arg)
	n, err := strconv.Atoi(digits)
	if err != nil {
		return 0, false
	}
	return n, true
}
