package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// cmdArchive implements `archive [<номер>]`: move closed tickets into
// tickets/archive/. Without an argument every done/closed ticket is
// archived; with an argument only the named one. Each moved file's path
// is echoed, as the pre-Go implementation did.
// Signature per wave-1 dispatch contract (wiki 8bd93a4e, A1).
func cmdArchive(st *store.Store, args []string, who, project string, lang domain.Lang, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return archiveAll(st, who, lang, stdout, stderr)
	}
	n, ok := parseTicketNumber(args[0])
	if !ok {
		return notFound(stderr, lang, args[0])
	}
	target, err := st.Archive(n, who)
	if err != nil {
		return archiveError(stderr, lang, args[0], err)
	}
	fmt.Fprintln(stdout, target)
	return exitOK
}

// archiveAll archives every done/closed ticket. No closed tickets is a
// success (exit 0, as in the bash era) with the pinned notice on stdout.
// Paths archived before a mid-run failure are still printed, because each
// target is echoed as it goes.
func archiveAll(st *store.Store, who string, lang domain.Lang, stdout, stderr io.Writer) int {
	// T-0157: numbers owned by more than one file are skipped, one
	// warning per number; the rest of done/closed archives as usual.
	dups, derr := st.Duplicates()
	if derr != nil {
		return archiveError(stderr, lang, "", derr)
	}
	skip := make(map[int]struct{}, len(dups))
	for _, c := range dups {
		fmt.Fprintln(stderr, domain.WarnAmbiguousNumberSkipped(lang, c.Number, strings.Join(c.Paths, ", ")))
		skip[c.Number] = struct{}{}
	}
	moved, err := st.ArchiveClosedSkipping(who, skip)
	// Store scan warnings ride on the archive error, alone or joined with a
	// mid-run failure; log them before the moved paths and the human report.
	if scanWarnings, ok := errors.AsType[*store.ScanWarningsError](err); ok {
		logParseWarnings(st, scanWarnings.Warnings)
	}
	for _, path := range moved {
		fmt.Fprintln(stdout, path)
	}
	if err != nil {
		rc := archiveError(stderr, lang, "", err)
		// T-0157: the ambiguity code wins over any other failure code —
		// a skipped (duplicated) number is the fact automation must see,
		// even when a scan warning or a mid-run error rode along.
		if len(skip) != 0 {
			return exitAmbiguous
		}
		return rc
	}
	if len(moved) == 0 && len(skip) == 0 {
		// Skips are reported on stderr above; the «nothing to archive»
		// notice is pinned to the no-duplicates case (T-0157).
		fmt.Fprintln(stdout, domain.ErrNoClosedTicketsToArchive(lang))
		return exitOK
	}
	if len(skip) > 0 {
		return exitAmbiguous
	}
	return exitOK
}

// archiveError maps a failed archive to the pinned messages
// inherited from the pre-Go implementation. The store never spells out UI text: it
// returns sentinels and typed errors that this function renders with the
// single «ticket: » prefix.
func archiveError(stderr io.Writer, lang domain.Lang, numArg string, err error) int {
	var notClosed *store.NotClosedError
	var collision *store.CollisionError
	var amb *store.AmbiguousNumberError
	switch {
	case errors.As(err, &amb):
		// T-0157: more than one file carries the number; the store
		// refused before touching anything.
		return reportAmbiguous(stderr, lang, amb)
	case errors.Is(err, store.ErrNotFound):
		return notFound(stderr, lang, numArg)
	case errors.Is(err, store.ErrAlreadyArchived):
		fmt.Fprintln(stderr, domain.ErrAlreadyArchived(lang))
	case errors.As(err, &notClosed):
		fmt.Fprintln(stderr, domain.ErrArchiveOnlyDoneClosed(lang, string(notClosed.Status)))
	case errors.As(err, &collision):
		fmt.Fprintln(stderr, domain.ErrFileAlreadyExists(lang, collision.Target))
	default:
		fmt.Fprintf(stderr, "ticket: %v\n", err)
	}
	return exitError
}
