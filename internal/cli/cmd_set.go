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
// A same-status set with a non-empty comment appends a journal-only
// entry (status and file name unchanged); without one it exits 1.
// lang selects the usage text language for argument errors.
// Signature per wave-1 dispatch contract (wiki 8bd93a4e, A1).
func cmdSet(st *store.Store, args []string, who, project string, lang domain.Lang, stdout, stderr io.Writer) int {
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
	// T-0065: reject CR/LF in the comment before touching the store
	// so a forged "c\nforged" can't append a fake journal entry. The
	// store also enforces this as defense-in-depth, but failing early
	// here gives a clean localized error and avoids any partial I/O.
	if strings.ContainsAny(comment, "\r\n") {
		fmt.Fprintln(stderr, domain.ErrJournalNewline(lang))
		return 1
	}
	n, ok := parseTicketNumber(numArg)
	if !ok {
		return notFound(stderr, numArg)
	}
	// T-0074: no pre-parse via Find. SetStatus routes on file names alone
	// (collision gate before any read), so a foreign same-number file with
	// a broken H1 surfaces as «файл … уже существует» instead of a parse
	// error. The store returns the real path: an archived ticket moved
	// between done and closed stays under archive/, so st.Dir would be wrong.
	target, err := st.SetStatus(n, next, who, comment)
	if err != nil {
		// T-0065: the store may reject CR/LF in comment/who. Localize
		// the message via the i18n layer (parity with T-0044 title
		// check above) instead of surfacing the raw
		// "store: journal ..." line.
		var inv *store.ErrInvalidJournalInput
		if errors.As(err, &inv) {
			fmt.Fprintln(stderr, domain.ErrJournalNewline(lang))
			return 1
		}
		return setError(st, stderr, numArg, err)
	}
	fmt.Fprintln(stdout, target)
	return 0
}

// setError maps a failed store lookup (Find) or SetStatus to the
// pinned §6 messages. The collision target comes from the typed
// store error: an archived
// ticket's target lives under archive/, so it can never be derived
// from the store directory here. st carries the store into the
// not-found branch so the message gains a hint line (T-0076).
func setError(st *store.Store, stderr io.Writer, numArg string, err error) int {
	var collision *store.CollisionError
	var already *store.AlreadyStatusError
	switch {
	case errors.As(err, &collision):
		fmt.Fprintf(stderr, "ticket: файл %s уже существует\n", collision.Target)
	case errors.As(err, &already):
		fmt.Fprintf(stderr, "ticket: тикет %d уже в статусе %s\n", already.Number, already.Status)
	case errors.Is(err, store.ErrNotFound):
		return notFoundHinted(st, stderr, numArg)
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
