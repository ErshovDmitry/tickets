package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"ticket/internal/store"
)

// cmdShow implements `show <номер|имя-файла>`: look the ticket up by
// number and print its file verbatim.
// Signature per wave-1 dispatch contract (wiki 8bd93a4e, A1).
func cmdShow(st *store.Store, args []string, who, project string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		usage(stdout)
		return 1
	}
	n, ok := parseTicketNumber(args[0])
	if !ok {
		return notFound(stderr, args[0])
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
		return notFound(stderr, args[0])
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
