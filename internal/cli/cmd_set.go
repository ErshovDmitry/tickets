package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// cmdSet implements `set <номер> <статус> ["комментарий"]`. The comment
// is the whitespace-join of all remaining arguments (bash ${*:-}).
// lang selects the usage text language for argument errors.
// Signature per wave-1 dispatch contract (wiki 8bd93a4e, A1).
func cmdSet(st *store.Store, args []string, who, project, lang string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		usage(stdout, lang)
		return 1
	}
	numArg, stArg := args[0], args[1]
	next, ok := statusByName(stArg)
	if !ok {
		fmt.Fprintln(stderr, "ticket: статус — один из: open wip done closed")
		return 1
	}
	comment := strings.Join(args[2:], " ")
	n, ok := parseTicketNumber(numArg)
	if !ok {
		return notFound(stderr, numArg)
	}
	cur, err := st.Find(n)
	if err != nil {
		// setError distinguishes ErrNotFound («не найден») from real
		// lookup errors (unreadable/corrupt file), which are reported
		// verbatim instead of being masked as not-found.
		return setError(stderr, numArg, err)
	}
	if cur.Status == next {
		fmt.Fprintf(stderr, "ticket: тикет уже в статусе %s\n", stArg)
		return 1
	}
	// The store returns the real path: an archived ticket moved between
	// done and closed stays under archive/, so st.Dir would be wrong.
	target, err := st.SetStatus(n, next, who, comment)
	if err != nil {
		return setError(stderr, numArg, err)
	}
	fmt.Fprintln(stdout, target)
	return 0
}

// setError maps a failed store lookup (Find) or SetStatus to the
// pinned §6 messages. The collision target comes from the typed
// store error: an archived
// ticket's target lives under archive/, so it can never be derived
// from the store directory here.
func setError(stderr io.Writer, numArg string, err error) int {
	var collision *store.CollisionError
	switch {
	case errors.As(err, &collision):
		fmt.Fprintf(stderr, "ticket: файл %s уже существует\n", collision.Target)
	case errors.Is(err, store.ErrNotFound):
		return notFound(stderr, numArg)
	default:
		fmt.Fprintf(stderr, "ticket: %v\n", err)
	}
	return 1
}

// statusByName maps a status word to its enum constant.
func statusByName(s string) (domain.Status, bool) {
	switch s {
	case "open":
		return domain.StatusOpen, true
	case "wip":
		return domain.StatusWip, true
	case "done":
		return domain.StatusDone, true
	case "closed":
		return domain.StatusClosed, true
	}
	return domain.StatusOpen, false
}
