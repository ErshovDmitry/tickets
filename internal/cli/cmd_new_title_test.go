package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/cli"
)

// TestNewTitleWithNewlineRejected pins the T-0075 supersession of the
// T-0044 fix: a title containing CR or LF is rejected with exit 1 and the
// localized errTitleCTL message BEFORE any file is created (a multiline H1
// loses everything past the first line on the next `set` re-render).
// Embedded breaks and boundary placements (leading/trailing newline) are
// covered; the RU message is asserted via the default lang (no lang env →
// ru), plus one EN-pinned case (TICKET_LANG=en) for the localized message.
func TestNewTitleWithNewlineRejected(t *testing.T) {
	cases := []struct {
		name    string
		title   string
		langEnv string // "" → default (ru); otherwise a TICKET_LANG value
		wantSub string
	}{
		{"embedded lf", "abc\ndef", "", "содержит управляющие символы"},
		{"embedded cr", "abc\rdef", "", "содержит управляющие символы"},
		{"embedded crlf", "abc\r\ndef", "", "содержит управляющие символы"},
		{"leading lf", "\nabc", "", "содержит управляющие символы"},
		{"trailing lf", "abc\n", "", "содержит управляющие символы"},
		{"en message", "abc\ndef", "en", "contains control characters"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			env := map[string]string{"TICKETS_DIR": dir}
			if tc.langEnv != "" {
				env["TICKET_LANG"] = tc.langEnv
			}
			var stdout, stderr bytes.Buffer
			code := cli.Run([]string{"new", tc.title}, env, &stdout, &stderr)
			if code != 1 {
				t.Fatalf("Run(new %q) = %d, want 1; stderr: %q", tc.title, code, stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.wantSub) {
				t.Fatalf("stderr %q does not contain %q", stderr.String(), tc.wantSub)
			}
			created, _ := filepath.Glob(filepath.Join(dir, "T-*.md"))
			if len(created) != 0 {
				t.Fatalf("ticket file created despite newline title: %v", created)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatalf("ReadDir(%s): %v", dir, err)
			}
			if len(entries) != 0 {
				names := make([]string, 0, len(entries))
				for _, e := range entries {
					names = append(names, e.Name())
				}
				t.Fatalf("tickets dir not empty after rejection: %v", names)
			}
		})
	}
}
