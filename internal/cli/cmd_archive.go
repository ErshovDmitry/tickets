package cli

import (
	"errors"
	"fmt"
	"io"

	"ticket/internal/store"
)

// cmdArchive implements `archive [<номер>]`: move closed tickets into
// tickets/archive/. Without an argument every done/closed ticket is
// archived; with an argument only the named one. Each moved file's path
// is echoed, mirroring bash cmd_archive (tickets/bin/ticket:161–188).
// Signature per wave-1 dispatch contract (wiki 8bd93a4e, A1).
func cmdArchive(st *store.Store, args []string, who, project string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return archiveAll(st, who, stdout, stderr)
	}
	n, ok := parseTicketNumber(args[0])
	if !ok {
		return notFound(stderr, args[0])
	}
	target, err := st.Archive(n, who)
	if err != nil {
		return archiveError(stderr, args[0], err)
	}
	fmt.Fprintln(stdout, target)
	return 0
}

// archiveAll archives every done/closed ticket. No closed tickets is a
// success (bash rc=0) with the pinned notice on stdout. Paths archived
// before a mid-run failure are still printed, as bash echoes each target
// as it goes.
func archiveAll(st *store.Store, who string, stdout, stderr io.Writer) int {
	moved, err := st.ArchiveClosed(who)
	for _, path := range moved {
		fmt.Fprintln(stdout, path)
	}
	if err != nil {
		return archiveError(stderr, "", err)
	}
	if len(moved) == 0 {
		fmt.Fprintln(stdout, "Нет закрытых тикетов для архивации.")
	}
	return 0
}

// archiveError maps a failed archive to the pinned bash messages
// (tickets/bin/ticket:166–171). The store never spells out UI text: it
// returns sentinels and typed errors that this function renders with the
// single «ticket: » prefix.
func archiveError(stderr io.Writer, numArg string, err error) int {
	var notClosed *store.NotClosedError
	var collision *store.CollisionError
	switch {
	case errors.Is(err, store.ErrNotFound):
		return notFound(stderr, numArg)
	case errors.Is(err, store.ErrAlreadyArchived):
		fmt.Fprintln(stderr, "ticket: тикет уже в архиве")
	case errors.As(err, &notClosed):
		fmt.Fprintf(stderr, "ticket: архивировать можно только done/closed (сейчас: %s)\n", notClosed.Status)
	case errors.As(err, &collision):
		fmt.Fprintf(stderr, "ticket: файл %s уже существует\n", collision.Target)
	default:
		fmt.Fprintf(stderr, "ticket: %v\n", err)
	}
	return 1
}
