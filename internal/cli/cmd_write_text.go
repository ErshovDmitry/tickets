package cli

// T-0240 A2: shared input machinery for the text write commands (comment /
// section set). The payload source is exactly one of - (stdin) or --file;
// --json is a FORMAT applied to whichever source was chosen, never a source
// itself. Reading and JSON decoding happen here, BEFORE any ticket lookup,
// so a refused input leaves the file and journal byte-identical (DS9). The
// payload bytes are passed through unchanged — LF/CRLF normalization, the
// ATX-heading guard and section protection all belong to the store (A1).

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// writeSources holds the parsed payload-source/format flags. schema is not
// here: comment --schema is a standalone pre-mode intercepted in Run before
// dispatch, so it never reaches this parser.
type writeSources struct {
	stdin bool
	file  *string
	json  bool
}

// parseWriteSources scans the tokens after the positional number/section.
// Only separate "--flag value" pairs are accepted (parseEditFlags style):
// "--flag=value", short flags and the "--" separator are refused as unknown
// flags, a bare positional is ErrUnknownArgument, and a repeat keeps the last
// value. A --file value is consumed blindly — even one starting with '-'.
func parseWriteSources(args []string, cmd string, lang domain.Lang, stderr io.Writer) (writeSources, bool) {
	var w writeSources
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-":
			w.stdin = true
		case "--file":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, domain.ErrFlagRequiresValue(lang, arg))
				return w, false
			}
			i++
			v := args[i]
			w.file = &v
		case "--json":
			w.json = true
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintln(stderr, domain.ErrWriteUnknownFlag(lang, cmd, arg))
				return w, false
			}
			fmt.Fprintln(stderr, domain.ErrUnknownArgument(lang, arg))
			return w, false
		}
	}
	return w, true
}

// readWritePayload reads the single chosen source verbatim and, for --json,
// decodes the strict {"text":"<payload>"} contract. Zero or both sources is
// a usage error and the terminal is never read. An empty file/stdin is a
// valid empty payload (the store decides whether empty is acceptable).
func readWritePayload(w writeSources, lang domain.Lang, stderr io.Writer) (string, bool) {
	hasFile := w.file != nil
	if (w.stdin && hasFile) || (!w.stdin && !hasFile) {
		fmt.Fprintln(stderr, domain.ErrWriteOneSource(lang))
		return "", false
	}
	var data []byte
	if w.stdin {
		d, err := io.ReadAll(stdinReader)
		if err != nil {
			fmt.Fprintln(stderr, domain.ErrWriteStdinRead(lang, err))
			return "", false
		}
		data = d
	} else {
		d, err := os.ReadFile(*w.file) // #nosec G304 -- the user explicitly chooses the payload file; the CLI has no elevated privileges
		if err != nil {
			fmt.Fprintln(stderr, domain.ErrWriteFileRead(lang, *w.file, pathErrorCause(err)))
			return "", false
		}
		data = d
	}
	if !w.json {
		return string(data), true
	}
	return decodeWriteJSON(data, lang, stderr)
}

// Contract causes carried as the %v detail of the localized invalid-JSON
// message; the template itself supplies the expected format and example.
var (
	errJSONObjectExpected = errors.New("expected a JSON object")
	errJSONMissingText    = errors.New("missing required field \"text\"")
)

// decodeWriteJSON enforces the strict single-field contract: a JSON object
// with exactly the string field "text". json.Unmarshal rejects trailing
// garbage; unknown fields are listed; there is no tolerant parsing and no
// fallback to plain text. The decoded text is the payload, returned unchanged.
func decodeWriteJSON(data []byte, lang domain.Lang, stderr io.Writer) (string, bool) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		fmt.Fprintln(stderr, domain.ErrWriteJSONInvalid(lang, jsonErrorOffset(err), err))
		return "", false
	}
	if m == nil { // JSON null is not an object
		fmt.Fprintln(stderr, domain.ErrWriteJSONInvalid(lang, 0, errJSONObjectExpected))
		return "", false
	}
	var unknown []string
	for k := range m {
		if k != "text" {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		slices.Sort(unknown)
		fmt.Fprintln(stderr, domain.ErrWriteJSONUnknownFields(lang, strings.Join(unknown, ", ")))
		return "", false
	}
	raw, ok := m["text"]
	if !ok {
		fmt.Fprintln(stderr, domain.ErrWriteJSONInvalid(lang, 0, errJSONMissingText))
		return "", false
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		fmt.Fprintln(stderr, domain.ErrWriteJSONInvalid(lang, jsonErrorOffset(err), err))
		return "", false
	}
	return text, true
}

// jsonErrorOffset extracts the byte position of a JSON parse failure; both
// json error types carry an Offset. Anything else reports 0.
func jsonErrorOffset(err error) int64 {
	var se *json.SyntaxError
	if errors.As(err, &se) {
		return se.Offset
	}
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		return te.Offset
	}
	return 0
}

// writeError maps a failed AddComment/SetSection to the pinned messages: the
// T-0157 ambiguity report (exit 2), the ATX-heading and not-editable refusals
// (line number / editable allowlist from the store), the empty-comment
// refusal, the hinted not-found report, read-only, or the generic store line.
// secArg names the offending section for the not-editable case ("" for comment).
func writeError(st *store.Store, stderr io.Writer, lang domain.Lang, numArg, secArg string, err error) int {
	var amb *store.AmbiguousNumberError
	var heading *store.PayloadHeadingError
	var notEditable *store.SectionNotEditableError
	switch {
	case errors.As(err, &amb):
		return reportAmbiguous(stderr, lang, amb)
	case errors.As(err, &heading):
		fmt.Fprintln(stderr, domain.ErrWritePayloadHeading(lang, heading.Line))
	case errors.As(err, &notEditable):
		fmt.Fprintln(stderr, domain.ErrWriteSectionNotEditable(lang, secArg, notEditable.Accepted))
	case errors.Is(err, store.ErrEmptyCommentPayload):
		fmt.Fprintln(stderr, domain.ErrWriteEmptyComment(lang))
	case errors.Is(err, store.ErrNotFound):
		return notFoundHinted(st, stderr, lang, numArg)
	case errors.Is(err, store.ErrReadOnly):
		fmt.Fprintln(stderr, domain.ErrReadOnly(lang))
	default:
		fmt.Fprintf(stderr, "ticket: %v\n", err)
	}
	return exitError
}
