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
func cmdShow(st *store.Store, args []string, who, project string, lang domain.Lang, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		usage(stdout, lang)
		return 1
	}
	// T-0076: extra arguments are ignored bash-compatibly, but the silent
	// truncation is surfaced as a warning; the exit code stays 0.
	if len(args) > 1 {
		fmt.Fprintf(stderr, "ticket: предупреждение: лишние аргументы игнорируются: %s\n", strings.Join(args[1:], " "))
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
		// An absent number is a not-found report; anything else (unreadable
		// or corrupt file, scan failure) is a real error the user must see,
		// never masked as «не найден».
		if errors.Is(err, store.ErrNotFound) {
			return notFoundHinted(st, stderr, args[0])
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

// notFound prints the bash-compatible «не найден» message; the quoted
// value is the user's original argument.
func notFound(stderr io.Writer, arg string) int {
	fmt.Fprintf(stderr, "ticket: тикет «%s» не найден\n", arg)
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
func notFoundHinted(st *store.Store, stderr io.Writer, arg string) int {
	rc := notFound(stderr, arg)
	n, ok := parseTicketNumber(arg)
	if !ok {
		return rc
	}
	const generic = "ticket: подсказка: используйте ticket list (show ищет и в archive/)"
	maxNum, warned := scanMax(st)
	switch {
	case n == 0:
		fmt.Fprintln(stderr, "ticket: подсказка: номера тикетов начинаются с 1; используйте ticket list")
	case maxNum == 0 && warned:
		fmt.Fprintln(stderr, generic)
	case maxNum == 0:
		fmt.Fprintln(stderr, "ticket: подсказка: хранилище пусто; используйте ticket list")
	case n > maxNum:
		fmt.Fprintf(stderr, "ticket: подсказка: в этом хранилище номера до %d; используйте ticket list\n", maxNum)
	default:
		fmt.Fprintln(stderr, generic)
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

// parseTicketNumber strips every non-digit (bash ${1//[^0-9]/}) and
// parses the remainder base-10 (bash $((10#...))). ok=false when no
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
