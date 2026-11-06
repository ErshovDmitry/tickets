package domain

import (
	"strings"
	"testing"
)

// TestRenderDetailsTrailingLF pins how both renderers treat the details
// payload: only trailing LF bytes are normalized away, every other byte is
// preserved. Each renderer appends its own "\n\n" separator, so a payload
// keeping its trailing LF would emit three consecutive newlines before the
// next section header (markdownlint MD012).
func TestRenderDetailsTrailingLF(t *testing.T) {
	tests := []struct {
		name string
		// details is the parsed Details field handed to the renderers.
		details string
		// body is the expected text between the two headings.
		body string
		// placeholder marks rows whose empty Details makes both renderers
		// fall back to their own placeholder text.
		placeholder bool
	}{
		{name: "no trailing LF", details: "detail line", body: "detail line"},
		{name: "one trailing LF", details: "detail line\n", body: "detail line"},
		{name: "two trailing LF", details: "detail line\n\n", body: "detail line"},
		{name: "empty", details: "", placeholder: true},
		{name: "LF only", details: "\n", placeholder: true},
		{name: "two LF only", details: "\n\n", placeholder: true},
		{name: "CRLF", details: "detail line\r\n", body: "detail line\r"},
		{name: "whitespace only", details: "   ", body: "   "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// RenderNewTicket falls back to the localized DetailsFormat
			// block, Render to the legacy detailsStub line.
			newBody, renderBody := tc.body, tc.body
			if tc.placeholder {
				newBody, renderBody = dictRU.detailsFormat, dictRU.detailsStub
			}
			assertDetailsBlock(t, tc.details, newBody, renderBody)
		})
	}
}

// assertDetailsBlock renders details through both renderers and compares the
// raw Details to User comments block, then re-checks that neither renderer
// mutated the caller's Ticket.
func assertDetailsBlock(t *testing.T, details, newBody, renderBody string) {
	t.Helper()
	newBlock := dictRU.headers[1] + "\n\n" + newBody + "\n\n" + dictRU.headers[2]
	renderBlock := dictRU.headers[1] + "\n\n" + renderBody + "\n\n" + dictRU.headers[2]

	newTicket := newTestTicket()
	newTicket.Details = details
	out, err := RenderNewTicket(newTicket)
	if err != nil {
		t.Fatalf("RenderNewTicket(%q): %v", details, err)
	}
	if !strings.Contains(string(out), newBlock) {
		t.Errorf("RenderNewTicket(%q) block missing\ngot:\n%s\nwant substring:\n%s", details, out, newBlock)
	}
	if newTicket.Details != details {
		t.Errorf("RenderNewTicket mutated Details: %q, want %q", newTicket.Details, details)
	}

	renderTicket := newTestTicket()
	renderTicket.Details = details
	out, err = Render(renderTicket, nil)
	if err != nil {
		t.Fatalf("Render(%q): %v", details, err)
	}
	if !strings.Contains(string(out), renderBlock) {
		t.Errorf("Render(%q) block missing\ngot:\n%s\nwant substring:\n%s", details, out, renderBlock)
	}
	if renderTicket.Details != details {
		t.Errorf("Render mutated Details: %q, want %q", renderTicket.Details, details)
	}
}
