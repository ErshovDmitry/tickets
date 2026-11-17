package cli

// T-0189 W3: the terminal/read-only guard shared by set and edit. A file
// under tickets/deleted/ is terminal: the commands must refuse BEFORE any
// mutation, using only the store's scan-only Locate seams (no body reads),
// so no implicit undelete is ever possible. The guard runs after argument
// validation and adds three outcomes to the ordinary behaviour:
//
//   - number in deleted/ only          -> localized refusal, exit 1
//     (the source file is unchanged);
//   - number in deleted/ AND live dirs -> the T-0157 ambiguity report
//     (exit 2) listing EVERY conflicting path, deleted included;
//   - number absent from deleted/      -> no-op: the caller proceeds
//     exactly as before.
//
// A failed deleted scan (invalid or unreadable tickets/deleted) is a real
// error the user must see ("ticket: <error>", exit 1) — never masked as a
// not-found report: the guard refuses to act on a number it could not
// fully account for.

import (
	"errors"
	"fmt"
	"io"
	"sort"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// deletedGuard inspects number n through the scan-only Locate seams and
// reports (exitCode, true) when it refused; (0, false) means "no deleted
// involvement — proceed with the ordinary flow". Shared by cmdSet and
// cmdEdit so both refuse identically.
func deletedGuard(st *store.Store, stderr io.Writer, lang domain.Lang, n int) (int, bool) {
	deletedPaths, err := st.LocateNumberDeleted(n)
	if err != nil {
		if errors.Is(err, store.ErrReadOnly) {
			fmt.Fprintln(stderr, domain.ErrReadOnly(lang))
		} else {
			fmt.Fprintf(stderr, "ticket: %v\n", err)
		}
		return exitError, true
	}
	if len(deletedPaths) == 0 {
		return 0, false
	}
	livePaths, err := st.LocateNumber(n)
	if err != nil {
		if errors.Is(err, store.ErrReadOnly) {
			fmt.Fprintln(stderr, domain.ErrReadOnly(lang))
		} else {
			fmt.Fprintf(stderr, "ticket: %v\n", err)
		}
		return exitError, true
	}
	if len(livePaths) == 0 {
		fmt.Fprintln(stderr, domain.ErrTicketDeletedReadOnly(lang, n))
		return exitError, true
	}
	paths := guardPaths(livePaths, deletedPaths)
	fmt.Fprintln(stderr, domain.ErrAmbiguousNumber(lang, n))
	for _, p := range paths {
		fmt.Fprintf(stderr, "  %s\n", p)
	}
	return exitAmbiguous, true
}

// guardPaths merges the live and deleted Locate results into one
// deduplicated, sorted path list — the shape reportAmbiguous prints for
// the store-side AmbiguousNumberError.
func guardPaths(live, deleted []string) []string {
	seen := make(map[string]struct{}, len(live)+len(deleted))
	paths := make([]string, 0, len(live)+len(deleted))
	for _, p := range live {
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		paths = append(paths, p)
	}
	for _, p := range deleted {
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}
