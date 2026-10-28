package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"ticket/internal/domain"
)

// resolveDetails turns the `-d` argument into the details text of a new
// ticket (T-0109):
//
//   - "-" alone     → the content of stdin,
//   - "@path"       → the content of the file at path (exactly one "@"
//     marker is stripped, so "@@name" is the file "@name"),
//   - anything else → the literal text.
//
// The bytes are preserved exactly: no trimming, no newline conversion, no
// size limit. An empty file or empty stdin yields an empty details text.
// On failure the localized message is printed to stderr and false is
// returned; the caller then exits 1 before any ticket is created.
func resolveDetails(arg string, lang domain.Lang, stderr io.Writer) (string, bool) {
	if arg == "-" {
		data, err := io.ReadAll(stdinReader)
		if err != nil {
			fmt.Fprintln(stderr, domain.ErrDetailsStdinRead(lang, err))
			return "", false
		}
		return string(data), true
	}
	if !strings.HasPrefix(arg, "@") {
		return arg, true
	}
	// Exactly one "@" marker is stripped; the rest is the path.
	path := strings.TrimPrefix(arg, "@")
	if path == "" {
		fmt.Fprintln(stderr, domain.ErrDetailsAtBare(lang))
		return "", false
	}
	data, err := os.ReadFile(path) // #nosec G304 -- the user explicitly chooses the details file; the CLI has no elevated privileges
	if err != nil {
		fmt.Fprintln(stderr, domain.ErrDetailsFileRead(lang, path, pathErrorCause(err)))
		return "", false
	}
	return string(data), true
}

// pathErrorCause keeps only the cause of a *fs.PathError: its Error()
// repeats the path ("open <path>: <cause>") and the path is already
// reported separately. A non-PathError is returned unchanged.
func pathErrorCause(err error) error {
	if pe, ok := errors.AsType[*fs.PathError](err); ok {
		return pe.Err
	}
	return err
}
