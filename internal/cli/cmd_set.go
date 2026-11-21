package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// cmdSet implements `set <номер> <статус> ["комментарий"]`. The comment is
// built by setComment: with exactly one trailing token it is resolved as a
// details source (`-` stdin, `@file` file contents, else literal) and one
// trailing LF/CRLF is stripped (T-0225, issue #50); any other arity
// whitespace-joins the trailing arguments.
// A same-status set with a non-empty comment appends a journal-only
// entry (status and file name unchanged); without one it exits 1.
// lang selects the usage text language for argument errors.
// Signature per wave-1 dispatch contract (wiki 8bd93a4e, A1).
func cmdSet(st *store.Store, args []string, who, _ string, lang domain.Lang, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		printUsage(stdout, stderr, lang)
		return exitError
	}
	// T-0068: reject a flag-like arg in the <number> or <status> position.
	// Report the FIRST offending positional (number before status).
	if strings.HasPrefix(args[0], "-") || strings.HasPrefix(args[1], "-") {
		badArg := args[0]
		if !strings.HasPrefix(args[0], "-") {
			badArg = args[1]
		}
		fmt.Fprintln(stderr, domain.ErrUnknownFlag(lang, badArg))
		return exitError
	}
	numArg, stArg := args[0], args[1]
	next, ok := statusByName(stArg)
	if !ok {
		fmt.Fprintln(stderr, domain.ErrStatusInvalid(lang))
		return exitError
	}
	comment, ok := setComment(args, lang, stderr)
	if !ok {
		// T-0215: a failed source read (stdin/file) exits before the
		// CR/LF guard, the deleted guard and SetStatus, so the ticket
		// name, its bytes and the journal stay untouched.
		return exitError
	}
	// T-0065: reject CR/LF in the comment before touching the store
	// so a forged "c\nforged" can't append a fake journal entry. The
	// store also enforces this as defense-in-depth, but failing early
	// here gives a clean localized error and avoids any partial I/O.
	if strings.ContainsAny(comment, "\r\n") {
		fmt.Fprintln(stderr, domain.ErrJournalNewline(lang))
		return exitError
	}
	n, ok := parseTicketNumber(numArg)
	if !ok {
		return notFound(stderr, lang, numArg)
	}
	// T-0189 W3: deleted is terminal and read-only — refuse a number
	// living in tickets/deleted/ (or duplicated across deleted/ and the
	// live dirs) BEFORE any mutation. `set N deleted` itself never reaches
	// the store: statusByName above does not know the deleted status, so
	// the target is rejected with the pinned invalid-status message.
	if rc, done := deletedGuard(st, stderr, lang, n); done {
		return rc
	}
	// T-0074: no pre-parse via Find. SetStatus routes on file names alone SetStatus routes on file names alone
	// (collision gate before any read), so a foreign same-number file with
	// a broken H1 surfaces as «файл … уже существует» instead of a parse
	// error. The store returns the real path: an archived ticket moved
	// between done and closed stays under archive/, so st.Dir would be wrong.
	target, warnings, err := st.SetStatus(n, next, who, comment)
	if err != nil {
		// T-0065: the store may reject CR/LF in comment/who. Localize
		// the message via the i18n layer (parity with T-0044 title
		// check above) instead of surfacing the raw
		// "store: journal ..." line.
		if _, ok := errors.AsType[*store.ErrInvalidJournalInput](err); ok {
			fmt.Fprintln(stderr, domain.ErrJournalNewline(lang))
			return exitError
		}
		return setError(st, stderr, lang, numArg, err)
	}
	// T-0070: parse anomalies from the rewritten file (e.g. a duplicate
	// Journal section that was merged) are non-fatal notices on stderr;
	// the command still succeeds. Exact cmd_list.go precedent format.
	// NOTE(T-0080): plain RU diagnostic text; migrate to the i18n registry when T-0080 lands.
	logParseWarnings(st, warnings)
	for _, w := range warnings {
		fmt.Fprintf(stderr, "ticket: %s\n", w.Error())
	}
	fmt.Fprintln(stdout, target)
	return exitOK
}

// setComment builds the comment from the trailing arguments (T-0215).
// Exactly ONE comment token is a source resolved by the shared
// resolveDetails (`-` = stdin, `@path` = file, one "@" stripped) — the same
// rules, bytes and localized messages as `new -d`. Zero or two-plus
// tokens keep the legacy space-join: multi-arg and flag-like literals
// such as "-x" stay literal, and stdin is never read twice.
// T-0225 (github.com/ErshovDmitry/tickets/issues/50): a resolved source
// usually ends with exactly one newline written by echo or an editor, so
// ONE trailing LF is stripped here — together with the CR of a CRLF pair.
// The remaining text is still subject to the CR/LF guard below, so
// "x\n\n", "x\r" and multi-line payloads keep failing unchanged.
// ok is false after the localized read message was printed to stderr;
// the caller then exits 1 without touching the store.
func setComment(args []string, lang domain.Lang, stderr io.Writer) (string, bool) {
	if len(args) != 3 {
		return strings.Join(args[2:], " "), true
	}
	comment, ok := resolveDetails(args[2], lang, stderr)
	if !ok {
		return "", false
	}
	if strings.HasSuffix(comment, "\n") {
		comment = strings.TrimSuffix(comment[:len(comment)-1], "\r")
	}
	return comment, true
}

// setError maps a failed store lookup (Find) or SetStatus to the
// pinned §6 messages. The collision target comes from the typed
// store error: an archived
// ticket's target lives under archive/, so it can never be derived
// from the store directory here. st carries the store into the
// not-found branch so the message gains a hint line (T-0076).
func setError(st *store.Store, stderr io.Writer, lang domain.Lang, numArg string, err error) int {
	var collision *store.CollisionError
	var already *store.AlreadyStatusError
	var amb *store.AmbiguousNumberError
	switch {
	case errors.As(err, &amb):
		// T-0157: more than one file carries the number; the store
		// refused before any mutation, so nothing needs rolling back.
		return reportAmbiguous(stderr, lang, amb)
	case errors.As(err, &collision):
		fmt.Fprintln(stderr, domain.ErrFileAlreadyExists(lang, collision.Target))
	case errors.As(err, &already):
		fmt.Fprintln(stderr, domain.ErrTicketAlreadyStatus(lang, already.Number, string(already.Status)))
	case errors.Is(err, store.ErrNotFound):
		return notFoundHinted(st, stderr, lang, numArg)
	case errors.Is(err, store.ErrReadOnly):
		fmt.Fprintln(stderr, domain.ErrReadOnly(lang))
	default:
		fmt.Fprintf(stderr, "ticket: %v\n", err)
	}
	return exitError
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
