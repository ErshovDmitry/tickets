package cli

import (
	_ "embed"
	"io"

	"ticket/internal/domain"
)

// usageTextRU and usageTextEN are the embedded usage messages.
// templates/usage.ru.txt was originally byte-identical to the bash
// reference heredoc (usage() in tickets/bin/ticket) and usage.en.txt its
// English translation with an identical line layout; since T-0036 both
// files carry an appended ticket-migration section and are therefore no
// longer byte-identical to the bash reference (deliberate divergence, as
// with the new-ticket template in T-0032/T-0035). The version line is
// prepended at runtime (T-0028), not in the templates.
//
//go:embed templates/usage.ru.txt
var usageTextRU string

//go:embed templates/usage.en.txt
var usageTextEN string

// usage writes the version line followed by the embedded usage message to w.
// LangEN selects the English text; any other value (including LangRU and all
// env vars unset) falls back to Russian.
func usage(w io.Writer, lang domain.Lang) {
	_, _ = io.WriteString(w, versionLine())
	text := usageTextRU
	if lang == domain.LangEN {
		text = usageTextEN
	}
	_, _ = io.WriteString(w, text)
}
