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

// cmdShow implements `show <номер|имя-файла>`: look the ticket up by
// number and print its file verbatim.
// lang selects the usage text language for argument errors.
// Signature per wave-1 dispatch contract (wiki 8bd93a4e, A1).
func cmdShow(st *store.Store, args []string, _, _ string, lang domain.Lang, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		printUsage(stdout, stderr, lang)
		return 1
	}
	// T-0076: extra arguments are ignored (as in the bash era), but the silent
	// truncation is surfaced as a warning; the exit code stays 0.
	if len(args) > 1 {
		fmt.Fprintln(stderr, domain.WarnExtraArgsIgnored(lang, strings.Join(args[1:], " ")))
	}
	// T-0157 escape hatch: an argument that is an exact ticket file name
	// (no path separators, matches T-NNNN-<status>.md) resolves that very
	// file via FindExact, even when the number is ambiguous — the user
	// picked the file from a listing, so the number need not be unique.
	if !strings.ContainsAny(args[0], "/\\") {
		if _, _, perr := domain.ParseFilename(args[0]); perr == nil {
			return showByName(st, args[0], lang, stdout, stderr)
		}
	}
	n, ok := parseTicketNumber(args[0])
	if !ok {
		// T-0072: the argument has no digits at all — it is not a ticket
		// number, so the «не найден» report would mislead.
		fmt.Fprintln(stderr, domain.ErrTicketArgNonNumeric(lang, args[0]))
		return 1
	}
	// FindRaw returns the scan-matched file's exact raw bytes, read from
	// the single validated handle (openValidated): the file is never
	// re-opened by path after validation, so a symlink swapped into the
	// name between scan and read cannot leak outside content into show's
	// output. The body's Status is untrusted ("- Статус: " accepts any
	// bytes), so the path is NEVER rebuilt via domain.Filename(t.Number,
	// t.Status): a crafted body status like "../../../x" made the old code
	// read files outside TICKETS_DIR (filepath.Join cleans the inner ".."
	// elements out of the directory — T-0006).
	_, _, raw, err := st.FindRaw(n)
	if err != nil {
		// T-0157: more than one file carries n — refuse and list every
		// conflicting path instead of silently reading an arbitrary one.
		var amb *store.AmbiguousNumberError
		if errors.As(err, &amb) {
			return reportAmbiguous(stderr, lang, amb)
		}
		// An absent number is a not-found report; anything else (unreadable
		// or corrupt file, scan failure) is a real error the user must see,
		// never masked as «не найден».
		if errors.Is(err, store.ErrNotFound) {
			return notFoundHinted(st, stderr, lang, args[0])
		}
		fmt.Fprintf(stderr, "ticket: %v\n", err)
		return 1
	}
	if _, err := stdout.Write(raw); err != nil {
		fmt.Fprintf(stderr, "ticket: %v\n", err)
		return 1
	}
	return 0
}

// notFound prints the established «не найден» message; the quoted
// value is the user's original argument.
func notFound(stderr io.Writer, lang domain.Lang, arg string) int {
	fmt.Fprintln(stderr, domain.ErrTicketNotFound(lang, arg))
	return 1
}

// notFoundHinted reports «не найден» and, for a numeric argument, adds
// ONE hint line chosen from the store's actual state (main + archive
// listings; the max is never hardcoded). Branch order is deliberate:
// n==0 → «номера начинаются с 1»; max==0 → «хранилище пусто», unless
// the scan rejected present files (warnings) — then the generic hint,
// so emptiness is never claimed for a store whose files were skipped;
// n>max → the real bound; in-range miss → the generic hint (show
// searches archive/ too). Scan warnings are not printed here.
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

// scanMax returns the highest ticket number across the main and archive
// listings and whether either scan produced warnings.
func scanMax(st *store.Store) (maxNum int, warned bool) {
	main, warns := st.List()
	archive, archiveWarns := st.ListArchive()
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
	return maxNum, len(warns) > 0 || len(archiveWarns) > 0
}

// showByName resolves an exact ticket file name via the store's
// FindExact escape hatch (T-0157): one match prints the file's exact
// bytes; an ambiguous name (same base name in main and archive) lists
// every conflicting path; an absent name is a not-found report. As with
// the number path, the raw bytes come from the single validated handle.
func showByName(st *store.Store, name string, lang domain.Lang, stdout, stderr io.Writer) int {
	_, _, raw, err := st.FindExact(name)
	if err != nil {
		var amb *store.AmbiguousNumberError
		if errors.As(err, &amb) {
			return reportAmbiguous(stderr, lang, amb)
		}
		if errors.Is(err, store.ErrNotFound) {
			return notFoundHinted(st, stderr, lang, name)
		}
		fmt.Fprintf(stderr, "ticket: %v\n", err)
		return 1
	}
	if _, err := stdout.Write(raw); err != nil {
		fmt.Fprintf(stderr, "ticket: %v\n", err)
		return 1
	}
	return 0
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
