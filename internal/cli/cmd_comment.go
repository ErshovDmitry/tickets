package cli

// T-0240 A2: `ticket comment` — append a text block to the Comments
// section through Store.AddComment (A1). The --schema mode is a standalone
// pre-command intercepted in Run, so it needs no tickets dir, store or lookup.

import (
	"fmt"
	"io"
	"strings"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// writeJSONSchema is the exact JSON Schema printed by `comment --schema`
// (T-0240 DS5): the single-field {"text":"<payload>"} input contract.
const writeJSONSchema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`

// cmdComment implements `comment <number> [-] [--file <path>] [--json]`.
// The number is mandatory and first (a digit-less argument is a not-found
// report, the edit precedent); flags follow in any order as separate pairs.
// The payload source is exactly one of - / --file, and reading/decoding it
// happens BEFORE the lookup so a refused input changes nothing (DS9). Success
// is silent (exit 0) apart from parse warnings on stderr (the edit precedent).
func cmdComment(st *store.Store, args []string, who, _ string, lang domain.Lang, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		printUsage(stdout, stderr, lang)
		return exitError
	}
	n, ok := parseTicketNumber(args[0])
	if !ok {
		return notFound(stderr, lang, args[0])
	}
	src, ok := parseWriteSources(args[1:], "comment", lang, stderr)
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
	warnings, err := st.AddComment(n, payload, who)
	if err != nil {
		return writeError(st, stderr, lang, args[0], "", err)
	}
	logParseWarnings(st, warnings)
	for _, w := range warnings {
		fmt.Fprintf(stderr, "ticket: %s\n", w.Error())
	}
	return exitOK
}

// detectCommentSchema is a silent pre-scan of the post-command tokens used by
// Run to route the standalone --schema mode before tickets-dir resolution and
// store opening. It reports whether a real --schema flag is present and
// whether a payload source accompanies it. Source flags are recognised by
// walkWriteFlags — the same grammar parseWriteSources uses — so a new source
// flag cannot bypass the --schema conflict check. A leading positional number
// is skipped; --file values are consumed blindly, so a "--schema" used as a
// file path is not mistaken for the flag.
func detectCommentSchema(args []string) (schema, hasSource bool) {
	rest := args
	if len(rest) > 0 && rest[0] != "-" && !strings.HasPrefix(rest[0], "-") {
		rest = rest[1:]
	}
	walkWriteFlags(rest, func(flag string, isSource bool, _ string, _ bool) {
		if isSource {
			hasSource = true
		}
		if flag == "--schema" {
			schema = true
		}
	})
	return schema, hasSource
}

// printWriteSchema writes the schema document to stdout (comment --schema).
func printWriteSchema(stdout io.Writer) {
	_, _ = io.WriteString(stdout, writeJSONSchema+"\n")
}
