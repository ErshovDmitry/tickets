package cli

// T-0240 A2: shared input machinery for the text write commands (comment /
// section set). The payload source is exactly one of - (stdin) or --file;
// --json is a FORMAT applied to whichever source was chosen, never a source
// itself. Reading and JSON decoding happen here, BEFORE any ticket lookup,
// so a refused input leaves the file and journal byte-identical (DS9). The
// payload bytes are passed through unchanged — LF/CRLF normalization, the
// ATX-heading guard and section protection all belong to the store (A1).

import (
	"bytes"
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

// walkWriteFlags scans write-command flag tokens left to right, consuming the
// value after --file blindly, and reports each flag to visit. It owns the
// payload-source grammar (isSource marks "-" and "--file") shared by
// parseWriteSources and detectCommentSchema, so a source flag added here is
// seen by both and cannot bypass the --schema conflict check. For --file,
// fileValue is the following token and hasFileValue reports its presence.
func walkWriteFlags(args []string, visit func(flag string, isSource bool, fileValue string, hasFileValue bool)) {
	for i := 0; i < len(args); i++ {
		flag := args[i]
		switch flag {
		case "-":
			visit(flag, true, "", false)
		case "--file":
			if i+1 < len(args) {
				i++
				visit(flag, true, args[i], true)
			} else {
				visit(flag, true, "", false)
			}
		default:
			visit(flag, false, "", false)
		}
	}
}

// parseWriteSources scans the tokens after the positional number/section.
// Only separate "--flag value" pairs are accepted (parseEditFlags style):
// "--flag=value", short flags and the "--" separator are refused as unknown
// flags, a bare positional is ErrUnknownArgument, and a repeat keeps the last
// value. A --file value is consumed blindly — even one starting with '-'.
func parseWriteSources(args []string, cmd string, lang domain.Lang, stderr io.Writer) (writeSources, bool) {
	var w writeSources
	ok := true
	walkWriteFlags(args, func(flag string, _ bool, fileValue string, hasFileValue bool) {
		if !ok {
			return
		}
		switch flag {
		case "-":
			w.stdin = true
		case "--file":
			if !hasFileValue {
				fmt.Fprintln(stderr, domain.ErrFlagRequiresValue(lang, flag))
				ok = false
				return
			}
			w.file = &fileValue
		case "--json":
			w.json = true
		default:
			if strings.HasPrefix(flag, "-") {
				fmt.Fprintln(stderr, domain.ErrWriteUnknownFlag(lang, cmd, flag))
			} else {
				fmt.Fprintln(stderr, domain.ErrUnknownArgument(lang, flag))
			}
			ok = false
		}
	})
	return w, ok
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
	errJSONTextNotString  = errors.New("field \"text\" must be a string")
)

// decodeWriteJSON enforces the strict single-field contract: a JSON object
// with exactly the string field "text". json.Unmarshal rejects trailing
// garbage; unknown fields are listed; there is no tolerant parsing and no
// fallback to plain text. The decoded text is the payload, returned unchanged.
// Refusal offsets are document-relative (T-0244) and share one anchor
// (T-0246): both a null text and a non-string value report the FIRST byte of
// the selected value (its zero-based start in the original document + 1), so
// the byte position matches what the user sees in the payload.
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
	// encoding/json unmarshals null into a string as a no-op, leaving it
	// empty; refuse null explicitly or it would silently clear the payload.
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		fmt.Fprintln(stderr, domain.ErrWriteJSONInvalid(lang, jsonValueStart(data)+1, errJSONTextNotString))
		return "", false
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		// The same anchor as null: the first byte of the selected value,
		// document-relative.
		fmt.Fprintln(stderr, domain.ErrWriteJSONInvalid(lang, jsonValueStart(data)+1, err))
		return "", false
	}
	return text, true
}

// jsonValueStart locates the selected "text" value in data. A locator error
// is practically unreachable once the primary Unmarshal succeeded; it
// degrades to the deterministic 0 fallback so the original type/null cause
// is still reported.
func jsonValueStart(data []byte) int64 {
	start, err := locateJSONField(data, "text")
	if err != nil {
		return 0
	}
	return start
}

// locateJSONField returns the zero-based byte offset of the first byte of the
// value of the LAST top-level occurrence of key in the JSON object data,
// matching the map last-wins rule of the primary Unmarshal. It walks the
// document with a streaming Decoder: after each value Decode, InputOffset is
// the end of that value, so start = end - len(raw value). Keys are compared
// decoded (Token unescapes them), so an escaped key spelling still matches,
// and other keys never influence the selection. An error is returned when
// data is not a well-formed object or the key is absent.
func locateJSONField(data []byte, key string) (int64, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return 0, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return 0, errJSONObjectExpected
	}
	start := int64(-1)
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return 0, err
		}
		k, ok := tok.(string)
		if !ok {
			return 0, fmt.Errorf("expected object key, got %v", tok)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return 0, err
		}
		if k == key {
			start = dec.InputOffset() - int64(len(raw))
		}
	}
	if start < 0 {
		return 0, fmt.Errorf("missing field %q", key)
	}
	return start, nil
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
