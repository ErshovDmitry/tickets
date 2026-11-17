package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// editFlags holds the parsed options of the `edit` command: raw value
// pointers, nil meaning "not given". A non-nil pointer — even one pointing
// at the current value — means "set exactly this" (T-0187).
type editFlags struct {
	title   *string
	typ     *string
	prio    *string
	project *string
}

// empty reports whether no field flag was given at all.
func (f editFlags) empty() bool {
	return f.title == nil && f.typ == nil && f.prio == nil && f.project == nil
}

// cmdEdit implements `edit <номер> [--title S] [--type T] [--priority P] [--project S]` (T-0187).
// The number is mandatory and first; after it ONLY separate "--flag value"
// pairs are accepted ("--flag=value" and short flags are refused, repeat is
// last-wins by the parseNewFlags precedent). A flag value is consumed
// blindly — even one starting with '-' — and validated afterwards. An
// explicitly unchanged value refuses the WHOLE edit before any write
// (localized AlreadyField message, exit 1), mirroring the same-status
// `set` rule. Success is silent: exit 0 with both streams empty, apart
// from parse warnings on stderr by the `set` precedent.
func cmdEdit(st *store.Store, args []string, who, _ string, lang domain.Lang, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		printUsage(stdout, stderr, lang)
		return exitError
	}
	// C1: the number is validated before anything else, exactly like the
	// other positional-first commands; a digit-less argument is a
	// not-found report, not a usage error.
	n, ok := parseTicketNumber(args[0])
	if !ok {
		return notFound(stderr, lang, args[0])
	}
	f, ok := parseEditFlags(args[1:], lang, stderr)
	if !ok {
		return exitError
	}
	if f.empty() {
		printUsage(stdout, stderr, lang)
		return exitError
	}
	opts, ok := validateEditFlags(f, lang, stderr)
	if !ok {
		return exitError
	}
	// T-0189 W3: deleted is terminal and read-only — refuse a number
	// living in tickets/deleted/ (or duplicated across deleted/ and the
	// live dirs) BEFORE any mutation; the source file stays unchanged.
	if rc, done := deletedGuard(st, stderr, lang, n); done {
		return rc
	}
	warnings, err := st.Edit(n, opts, who)
	if err != nil {
		return editError(st, stderr, lang, args[0], err)
	}
	// Parse anomalies from the rewritten file (e.g. a merged duplicate
	// Journal section) are non-fatal notices on stderr; the command still
	// succeeds. Exact cmd_set.go precedent format.
	logParseWarnings(st, warnings)
	for _, w := range warnings {
		fmt.Fprintf(stderr, "ticket: %s\n", w.Error())
	}
	return exitOK
}

// parseEditFlags scans the post-number arguments: each token must be one of
// the four long flags, and its value is the NEXT token consumed blindly
// (even a '-'-prefixed one), then validated later. An unknown '-'-prefixed
// token (incl. the refused "--flag=value" glued forms and short flags) is
// the edit-specific unknown-flag message; a bare positional is the shared
// ErrUnknownArgument. Repeating a flag keeps the LAST value.
func parseEditFlags(args []string, lang domain.Lang, stderr io.Writer) (editFlags, bool) {
	var f editFlags
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--title", "--type", "--priority", "--project":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, domain.ErrFlagRequiresValue(lang, arg))
				return f, false
			}
			i++
			v := args[i]
			switch arg {
			case "--title":
				f.title = &v
			case "--type":
				f.typ = &v
			case "--priority":
				f.prio = &v
			case "--project":
				f.project = &v
			}
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintln(stderr, domain.ErrEditUnknownFlag(lang, arg))
				return f, false
			}
			fmt.Fprintln(stderr, domain.ErrUnknownArgument(lang, arg))
			return f, false
		}
	}
	return f, true
}

// validateEditFlags converts the parsed flags into store.EditOptions,
// rejecting bad values BEFORE any store I/O (C1 order: type, priority,
// title, project). Type/priority go through typeByName/priorityByName and
// the shared invalid messages — never the raw Parse* errors. Title follows
// the `new` rules (whitespace-only, CTL, dash-prefix); a whitespace-only
// project gets its own message; CR/LF in the project value would forge a
// journal line and gets the edit-specific newline error (title CR/LF is
// already covered by the CTL rule, as in `new`).
func validateEditFlags(f editFlags, lang domain.Lang, stderr io.Writer) (store.EditOptions, bool) {
	var opts store.EditOptions
	if f.typ != nil {
		t, ok := typeByName(*f.typ)
		if !ok {
			fmt.Fprintln(stderr, domain.ErrTypeInvalid(lang))
			return opts, false
		}
		opts.Type = &t
	}
	if f.prio != nil {
		p, ok := priorityByName(*f.prio)
		if !ok {
			fmt.Fprintln(stderr, domain.ErrPriorityInvalid(lang))
			return opts, false
		}
		opts.Priority = &p
	}
	if f.title != nil {
		title := *f.title
		if strings.TrimSpace(title) == "" {
			fmt.Fprintln(stderr, domain.ErrTitleEmpty(lang))
			return opts, false
		}
		if domain.ContainsCTL(title) {
			fmt.Fprintln(stderr, domain.ErrTitleCTL(lang))
			return opts, false
		}
		if strings.HasPrefix(title, "-") {
			fmt.Fprintln(stderr, domain.ErrTitleStartsWithDash(lang))
			return opts, false
		}
		opts.Title = &title
	}
	if f.project != nil {
		project := *f.project
		if strings.TrimSpace(project) == "" {
			fmt.Fprintln(stderr, domain.ErrProjectEmpty(lang))
			return opts, false
		}
		if strings.ContainsAny(project, "\r\n") {
			fmt.Fprintln(stderr, domain.ErrEditFieldNewline(lang))
			return opts, false
		}
		opts.Project = &project
	}
	return opts, true
}

// editError maps a failed Store.Edit to the pinned messages: the T-0157
// ambiguity report (exit 2, shared handlers), the localized AlreadyField
// refusal for an explicitly unchanged value (exit 1, no-op), the hinted
// not-found report, or the generic store line.
func editError(st *store.Store, stderr io.Writer, lang domain.Lang, numArg string, err error) int {
	var already *store.AlreadyFieldError
	var amb *store.AmbiguousNumberError
	switch {
	case errors.As(err, &amb):
		return reportAmbiguous(stderr, lang, amb)
	case errors.As(err, &already):
		fmt.Fprintln(stderr, domain.ErrEditAlreadyField(lang, already.Number, already.Field, already.Value))
	case errors.Is(err, store.ErrNotFound):
		return notFoundHinted(st, stderr, lang, numArg)
	case errors.Is(err, store.ErrReadOnly):
		fmt.Fprintln(stderr, domain.ErrReadOnly(lang))
	default:
		fmt.Fprintf(stderr, "ticket: %v\n", err)
	}
	return exitError
}
