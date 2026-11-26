package cli

// T-0241: the two new show modes — --section and --json.
//
// The plain mode in cmd_show.go must stay byte-identical, so the mode detection
// is a pure token scan and parseShowFlags runs ONLY once a bare mode token is
// present: a --section=x glued form or an unknown flag alone still belongs to
// the plain path's «extra arguments are ignored» warning.

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// errShowArgNonNumeric marks a show argument that carries no digits at all. It
// is not a store error: the caller reports it with the ticket-specific message,
// because a «не найден» report would mislead (T-0072 precedent).
var errShowArgNonNumeric = errors.New("cli: show argument has no digits")

// hasShowModeFlag reports whether args select a new mode: an exact --section or
// --json token. Glued forms never select a mode (the plan refuses them inside
// the modes and leaves them alone outside).
func hasShowModeFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--section" || arg == "--json" {
			return true
		}
	}
	return false
}

// runShowModes validates the flags and prints the selected section body or the
// JSON document. Nothing is read from the store before the flags are valid.
func runShowModes(st *store.Store, args []string, lang domain.Lang, stdout, stderr io.Writer) int {
	arg, sec, jsonMode, ok := parseShowFlags(args, lang, stderr)
	if !ok {
		return exitError
	}
	tk, _, _, unknown, warnings, err := locateShow(st, arg)
	if err != nil {
		return reportShowArgError(st, stderr, lang, arg, err)
	}
	if jsonMode {
		return runShowJSON(stdout, stderr, lang, tk, unknown, warnings)
	}
	return writeShowSection(stdout, stderr, lang, tk, *sec)
}

// writeShowSection prints the section body followed by exactly one LF. An empty
// or absent section is a success that prints nothing at all (0 bytes), so the
// payload survives a pipe unchanged and never gains a second newline.
func writeShowSection(stdout, stderr io.Writer, lang domain.Lang, tk domain.Ticket, sec domain.Section) int {
	body := tk.SectionBody(sec)
	if body == "" {
		return exitOK
	}
	if _, err := io.WriteString(stdout, body+"\n"); err != nil {
		return reportShowWriteError(stderr, lang, err)
	}
	return exitOK
}

// parseShowFlags scans the arguments of a new-mode show. Only the split form is
// accepted: --section takes the NEXT token as its value, --json is bare. A
// positional argument is the ticket to show and exactly one is allowed.
// Repeated flags, a missing flag value, an unknown or glued flag, extra
// positionals and the --section+--json combination are all refusals: the
// message goes to stderr and stdout stays untouched.
func parseShowFlags(args []string, lang domain.Lang, stderr io.Writer) (string, *domain.Section, bool, bool) {
	var sec *domain.Section
	var jsonMode bool
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--section":
			if sec != nil {
				fmt.Fprintln(stderr, domain.ErrRepeatedFlagSection(lang))
				return "", nil, false, false
			}
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, domain.ErrFlagRequiresValue(lang, "--section"))
				return "", nil, false, false
			}
			i++
			parsed, ok := domain.ParseSectionName(args[i])
			if !ok {
				fmt.Fprintln(stderr, domain.ErrUnknownSection(lang, args[i],
					domain.CanonicalSectionNames(), domain.SectionAliasNames()))
				return "", nil, false, false
			}
			sec = &parsed
		case "--json":
			if jsonMode {
				fmt.Fprintln(stderr, domain.ErrRepeatedFlagJson(lang))
				return "", nil, false, false
			}
			jsonMode = true
		default:
			if strings.HasPrefix(args[i], "-") {
				// Unknown flag, including the refused glued --section=<value>
				// and --json=<value> forms.
				fmt.Fprintln(stderr, domain.ErrUnknownFlagList(lang, args[i]))
				return "", nil, false, false
			}
			positional = append(positional, args[i])
		}
	}
	if sec != nil && jsonMode {
		fmt.Fprintln(stderr, domain.ErrFlagsConflictShow(lang))
		return "", nil, false, false
	}
	if len(positional) == 0 {
		// No ticket to show: the same usage block plain show prints for a
		// missing argument, but on stderr — a new mode never writes to stdout
		// when it refuses.
		printUsage(stderr, stderr, lang)
		return "", nil, false, false
	}
	if len(positional) > 1 {
		fmt.Fprintln(stderr, domain.ErrShowExtraArgsModes(lang, positional[1:]))
		return "", nil, false, false
	}
	return positional[0], sec, jsonMode, true
}

// locateShow resolves the ticket argument through the same TOCTOU-safe lookups
// the plain path always used, reading the file once for both modes: an exact
// ticket file name (T-0157 escape hatch, no path separators, T-NNNN-<status>.md)
// resolves that very file even when the number is ambiguous; otherwise the
// argument must carry digits and is looked up across the main, archive and
// deleted listings (T-0189). The store never rebuilds a path from the parsed
// body status, which is untrusted (T-0006).
func locateShow(st *store.Store, arg string) (domain.Ticket, string, []byte, []byte, []store.ParseWarning, error) {
	if !strings.ContainsAny(arg, "/\\") {
		if _, _, perr := domain.ParseFilename(arg); perr == nil {
			return st.FindExactShowAcross(arg)
		}
	}
	n, ok := parseTicketNumber(arg)
	if !ok {
		return domain.Ticket{}, "", nil, nil, nil, errShowArgNonNumeric
	}
	return st.FindShowAcross(n)
}
