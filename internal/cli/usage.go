package cli

import (
	"embed"
	"fmt"
	"io"

	"ticket/internal/domain"
)

//go:embed templates/usage.*.txt
var usageFS embed.FS

// usageTextRU and usageTextEN are the embedded usage messages, populated
// from usageFS in init() for compatibility with existing tests.
// templates/usage.ru.txt was originally byte-identical to the bash
// reference heredoc (usage() in tickets/bin/ticket) and usage.en.txt its
// English translation with an identical line layout; since T-0036 both
// files carry an appended ticket-migration section and are therefore no
// longer byte-identical to the bash reference (deliberate divergence, as
// with the new-ticket template in T-0032/T-0035). The version line is
// prepended at runtime (T-0028), not in the templates.
var usageTextRU string
var usageTextEN string

func init() {
	var err error
	data, err := usageFS.ReadFile("templates/usage.ru.txt")
	if err != nil {
		panic(fmt.Sprintf("usage: read usage.ru.txt: %v", err))
	}
	usageTextRU = string(data)

	data, err = usageFS.ReadFile("templates/usage.en.txt")
	if err != nil {
		panic(fmt.Sprintf("usage: read usage.en.txt: %v", err))
	}
	usageTextEN = string(data)
}

// usage writes the version line followed by the embedded usage message to w.
// The language is selected by the lang parameter (LangRU or LangEN).
func usage(w io.Writer, lang domain.Lang) {
	_, _ = io.WriteString(w, versionLine())
	fname := "templates/usage." + string(lang) + ".txt"
	data, err := usageFS.ReadFile(fname)
	if err != nil {
		// The error is possible when a language is registered (its data
		// file exists) but templates/usage.<lang>.txt is missing. The panic
		// is a deliberate fast stop: a new language must ship its usage
		// file (see the checklist in docs/add-language.md).
		panic(fmt.Sprintf("usage: read %s: %v", fname, err))
	}
	_, _ = io.WriteString(w, string(data))
}
