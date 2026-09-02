package cli

import (
	_ "embed"
	"io"
)

// usageTextRU and usageTextEN are the embedded usage messages.
// templates/usage.ru.txt is byte-identical to the bash reference heredoc
// (usage() in tickets/bin/ticket); templates/usage.en.txt is the English
// translation with an identical line layout. Do not edit either file:
// the version line is prepended at runtime (T-0028), not in the templates.
//
//go:embed templates/usage.ru.txt
var usageTextRU string

//go:embed templates/usage.en.txt
var usageTextEN string

// usage writes the version line followed by the embedded usage message to w.
// lang=="en" selects the English text; any other value (including unset)
// falls back to Russian.
func usage(w io.Writer, lang string) {
	_, _ = io.WriteString(w, versionLine())
	text := usageTextRU
	if lang == "en" {
		text = usageTextEN
	}
	_, _ = io.WriteString(w, text)
}
