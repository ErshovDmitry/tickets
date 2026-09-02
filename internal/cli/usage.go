package cli

import (
	_ "embed"
	"io"
)

// usageText is the embedded usage message. templates/usage.txt is
// byte-identical to the bash reference heredoc (usage() in
// tickets/bin/ticket); do not edit that file.
//
//go:embed templates/usage.txt
var usageText string

// usage writes the usage message verbatim to w.
func usage(w io.Writer) {
	_, _ = io.WriteString(w, usageText)
}
