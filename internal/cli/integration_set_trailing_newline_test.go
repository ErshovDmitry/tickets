package cli_test

// T-0225 (github.com/ErshovDmitry/tickets/issues/50) integration: the real
// binary strips exactly ONE trailing LF/CRLF from a single-token \`set\`
// comment source (stdin, @file, literal); every other CR/LF placement exits
// 1 with the localized ErrJournalNewline and leaves the sandbox
// byte-identical. Only TempDir sandboxes are used. Helpers are reused from
// integration_set_comment_source_test.go and integration_new_details_test.go.

import (
	"strings"
	"testing"

	"ticket/internal/domain"
)

// TestSetCommentTrailingNewlineIntegration pins the strip matrix (T-0225 D2
// truth table) end to end for both source kinds: "x\n", "x\r\n" and "x"
// land in the journal as "· x (agent)"; inner/bare/multiple CR or LF exits
// 1 with ErrJournalNewline before the store, the sandbox untouched.
func TestSetCommentTrailingNewlineIntegration(t *testing.T) {
	requireBin(t)
	cases := []struct {
		name    string
		payload string
		file    bool
		want    string // journal line; "" means the CR/LF guard rejects
	}{
		{"LF stdin", "x\n", false, "· x (agent)"},
		{"CRLF stdin", "x\r\n", false, "· x (agent)"},
		{"no newline stdin", "x", false, "· x (agent)"},
		{"LF file", "x\n", true, "· x (agent)"},
		{"CRLF file", "x\r\n", true, "· x (agent)"},
		{"no newline file", "x", true, "· x (agent)"},
		{"inner LF stdin", "a\nb\n", false, ""},
		{"inner CR stdin", "a\rb", false, ""},
		{"inner CRLF stdin", "a\r\nb\n", false, ""},
		{"double LF stdin", "x\n\n", false, ""},
		{"trailing CR stdin", "x\r", false, ""},
		{"CR before CRLF stdin", "x\r\r\n", false, ""},
		{"inner LF file", "a\nb\n", true, ""},
		{"double LF file", "x\n\n", true, ""},
		{"trailing CR file", "x\r", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tickets, cwd, _ := setSandbox(t)
			token, in := "-", tc.payload
			if tc.file {
				token, in = "@"+writeCommentFile(t, tc.payload), ""
			}
			before := setSandboxState(t, tickets)
			out, stderr, code := runBinWithStdin(t, ticketBin, cwd, tickets, in, "set", "1", "wip", token)
			if tc.want == "" {
				if code != 1 || out != "" || stderr != domain.ErrJournalNewline(domain.LangEN)+"\n" {
					t.Fatalf("set: code=%d stdout=%q stderr=%q; want 1 with ErrJournalNewline", code, out, stderr)
				}
				if after := setSandboxState(t, tickets); after != before {
					t.Fatalf("sandbox changed on %q:\nbefore=%s\nafter=%s", tc.payload, before, after)
				}
				return
			}
			if code != 0 || stderr != "" {
				t.Fatalf("set: code=%d stderr=%q; want 0 and empty stderr", code, stderr)
			}
			if body := string(readCreatedTicket(t, out)); !strings.Contains(body, tc.want) {
				t.Fatalf("journal missing %q:\n%s", tc.want, body)
			}
		})
	}
}

// TestSetCommentNewlineOnlySourceIntegration pins the T-0225 boundary end
// to end: a newline-only source becomes an empty comment — same-status
// keeps the pinned AlreadyStatus rejection with an untouched sandbox, a
// real status change succeeds with NO journal line.
func TestSetCommentNewlineOnlySourceIntegration(t *testing.T) {
	requireBin(t)
	for _, payload := range []string{"\n", "\r\n"} {
		t.Run(strings.ReplaceAll(payload, "\r", "CR"), func(t *testing.T) {
			tickets, cwd, _ := setSandbox(t)
			before := setSandboxState(t, tickets)
			out, stderr, code := runBinWithStdin(t, ticketBin, cwd, tickets, payload, "set", "1", "open", "-")
			if code != 1 || out != "" || stderr != domain.ErrTicketAlreadyStatus(domain.LangEN, 1, "open")+"\n" {
				t.Fatalf("same-status: code=%d stdout=%q stderr=%q; want 1 with AlreadyStatus", code, out, stderr)
			}
			if after := setSandboxState(t, tickets); after != before {
				t.Fatalf("sandbox changed on rejected same-status set:\nbefore=%s\nafter=%s", before, after)
			}
			out, stderr, code = runBinWithStdin(t, ticketBin, cwd, tickets, payload, "set", "1", "wip", "-")
			if code != 0 || stderr != "" {
				t.Fatalf("status change: code=%d stderr=%q; want 0 and empty stderr", code, stderr)
			}
			body := string(readCreatedTicket(t, out))
			if journal := body[strings.Index(body, "## Journal"):]; strings.Contains(journal, "· ") {
				t.Fatalf("journal line leaked from newline-only payload:\n%s", body)
			}
		})
	}
}

// TestSetCommentTrailingNewlineScopeIntegration pins the T-0225 scope end
// to end: the multi-token join keeps "y\n" verbatim and still fails the
// guard, while a single LITERAL token "x\n" is normalized to "x".
func TestSetCommentTrailingNewlineScopeIntegration(t *testing.T) {
	requireBin(t)
	tickets, cwd, _ := setSandbox(t)
	before := setSandboxState(t, tickets)
	out, stderr, code := runBinEnv(t, ticketBin, cwd, tickets, []string{"TICKET_LANG=en"},
		"set", "1", "wip", "x", "y\n")
	if code != 1 || out != "" || stderr != domain.ErrJournalNewline(domain.LangEN)+"\n" {
		t.Fatalf("multi-token: code=%d stdout=%q stderr=%q; want 1 with ErrJournalNewline", code, out, stderr)
	}
	if after := setSandboxState(t, tickets); after != before {
		t.Fatalf("sandbox changed on rejected multi-token set:\nbefore=%s\nafter=%s", before, after)
	}
	out, stderr, code = runBinEnv(t, ticketBin, cwd, tickets, []string{"TICKET_LANG=en"},
		"set", "1", "wip", "x\n")
	if code != 0 || stderr != "" {
		t.Fatalf("literal: code=%d stderr=%q; want 0 and empty stderr", code, stderr)
	}
	if body := string(readCreatedTicket(t, out)); !strings.Contains(body, "· x (agent)") {
		t.Fatalf("journal missing %q:\n%s", "· x (agent)", body)
	}
}
