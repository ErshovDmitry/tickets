package cli

// T-0240 A2: `ticket section set` — replace the body of an editable
// section through Store.SetSection (A1). Section protection, the ATX guard
// and payload normalization stay in the store; the CLI only parses the
// section token (domain.ParseSectionName) and refuses an unknown one.

import (
	"fmt"
	"io"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// cmdSection implements `section set <number> <section> [-] [--file <path>] [--json]`.
// The number and the canonical section token are the first two positionals;
// flags follow in any order as separate pairs. An unknown token is refused
// with the editable allowlist; a known-but-protected token is refused by the
// store (SectionNotEditableError). Reading/decoding the source precedes the
// lookup so a refused input changes nothing (DS9). An empty payload clears the
// section. Success is silent (exit 0) apart from parse warnings on stderr.
func cmdSection(st *store.Store, args []string, who, _ string, lang domain.Lang, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "set" {
		printUsage(stdout, stderr, lang)
		return exitError
	}
	rest := args[1:]
	if len(rest) < 2 {
		printUsage(stdout, stderr, lang)
		return exitError
	}
	numArg := rest[0]
	n, ok := parseTicketNumber(numArg)
	if !ok {
		return notFound(stderr, lang, numArg)
	}
	secArg := rest[1]
	sec, ok := domain.ParseSectionName(secArg)
	if !ok {
		fmt.Fprintln(stderr, domain.ErrWriteSectionNotEditable(lang, secArg, domain.EditableSections()))
		return exitError
	}
	src, ok := parseWriteSources(rest[2:], "section set", lang, stderr)
	if !ok {
		return exitError
	}
	payload, ok := readWritePayload(src, lang, stderr)
	if !ok {
		return exitError
	}
	if rc, done := deletedGuard(st, stderr, lang, n); done {
		return rc
	}
	warnings, err := st.SetSection(n, sec, payload, who)
	if err != nil {
		return writeError(st, stderr, lang, numArg, secArg, err)
	}
	logParseWarnings(st, warnings)
	for _, w := range warnings {
		fmt.Fprintf(stderr, "ticket: %s\n", w.Error())
	}
	return exitOK
}
