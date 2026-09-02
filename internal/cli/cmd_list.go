package cli

import (
	"fmt"
	"io"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// cmdList implements `list [active|open|wip|done|closed|archive|all]`
// with the default filter active. The archive filter lists tickets under
// tickets/archive/ instead of the main directory. An empty first argument
// falls back to the default (bash `want="${1:-active}"` treats an empty
// $1 as unset). Extra arguments are ignored for bash parity.
// Signature per wave-1 dispatch contract (wiki 8bd93a4e, A1).
func cmdList(st *store.Store, args []string, who, project string, stdout, stderr io.Writer) int {
	want := "active"
	if len(args) >= 1 && args[0] != "" {
		want = args[0]
	}
	if !isListFilter(want) {
		fmt.Fprintln(stderr, "ticket: фильтр — один из: active open wip done closed archive all")
		return 1
	}

	var tickets []domain.Ticket
	var warnings []store.ParseWarning

	if want == "archive" {
		tickets, warnings = st.ListArchive()
	} else {
		tickets, warnings = st.List()
	}

	for _, w := range warnings {
		fmt.Fprintf(stderr, "ticket: %s\n", w.Error())
	}
	found := false
	for i := range tickets {
		t := tickets[i]
		if want != "archive" && !matchFilter(t.Status, want) {
			continue
		}
		fmt.Fprintf(stdout, "T-%04d  %-7s  %s\n", t.Number, t.Status, string(t.Type)+": "+t.Title)
		found = true
	}
	if !found {
		fmt.Fprintf(stdout, "Нет тикетов (%s).\n", want)
	}
	return 0
}

// isListFilter reports whether want is a valid list filter.
func isListFilter(want string) bool {
	switch want {
	case "active", "open", "wip", "done", "closed", "archive", "all":
		return true
	}
	return false
}

// matchFilter applies the list filter to a ticket status.
func matchFilter(st domain.Status, want string) bool {
	switch want {
	case "all":
		return true
	case "active":
		return st == domain.StatusOpen || st == domain.StatusWip
	case "open":
		return st == domain.StatusOpen
	case "wip":
		return st == domain.StatusWip
	case "done":
		return st == domain.StatusDone
	case "closed":
		return st == domain.StatusClosed
	}
	return false
}
