package cli

// T-0225 (github.com/ErshovDmitry/tickets/issues/50): a single-token
// \`set\` comment source (stdin, @file or a literal) may carry exactly ONE
// trailing LF or CRLF, which is stripped after resolveDetails; every other
// CR/LF placement still hits the CR/LF guard with the ticket and the
// sandbox byte-identical. Helpers are reused from
// cmd_set_comment_source_test.go and cmd_new_details_test.go.

import (
	"strings"
	"testing"

	"ticket/internal/domain"
)

// TestSetCommentTrailingNewline pins the strip matrix (T-0225 D2 truth
// table) for both source kinds and both languages: "x\n", "x\r\n" and "x"
// succeed with the newline stripped from the journal line; inner/bare/
// multiple CR or LF still exits 1 with the localized ErrJournalNewline
// before the store, leaving the sandbox untouched.
func TestSetCommentTrailingNewline(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		file    bool
		want    string // journal line; "" means the CR/LF guard rejects
	}{
		{"LF stdin", "x\n", false, "· x (tester)"},
		{"CRLF stdin", "x\r\n", false, "· x (tester)"},
		{"no newline stdin", "x", false, "· x (tester)"},
		{"LF file", "x\n", true, "· x (tester)"},
		{"CRLF file", "x\r\n", true, "· x (tester)"},
		{"no newline file", "x", true, "· x (tester)"},
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
	for _, lang := range []domain.Lang{domain.LangRU, domain.LangEN} {
		for _, tc := range cases {
			t.Run(string(lang)+" "+tc.name, func(t *testing.T) {
				st, dir, _ := setFixture(t)
				token, stdin := "-", tc.payload
				if tc.file {
					token, stdin = commentFile(t, tc.payload), ""
				} else {
					withStdin(t, strings.NewReader(stdin))
				}
				before := sandboxState(t, dir)
				stdout, stderr, rc := runSetCmd(st, lang, "1", "wip", token)
				if tc.want == "" {
					if rc != 1 || stdout != "" || stderr != domain.ErrJournalNewline(lang)+"\n" {
						t.Fatalf("cmdSet = %d, stdout %q, stderr %q; want 1 with ErrJournalNewline", rc, stdout, stderr)
					}
					if after := sandboxState(t, dir); after != before {
						t.Fatalf("sandbox changed on %q:\nbefore=%s\nafter=%s", tc.payload, before, after)
					}
					return
				}
				if rc != 0 || stderr != "" {
					t.Fatalf("cmdSet = %d, stderr %q; want 0 and empty stderr", rc, stderr)
				}
				if body := readTicket(t, strings.TrimSpace(stdout)); !strings.Contains(body, tc.want) {
					t.Fatalf("journal missing %q:\n%s", tc.want, body)
				}
			})
		}
	}
}

// TestSetCommentNewlineOnlySource pins the T-0225 boundary: after the one
// stripped newline the source is EMPTY, which follows the existing
// empty-source rules — same-status keeps the pinned AlreadyStatus error
// with an untouched sandbox, a real status change succeeds with NO
// journal line.
func TestSetCommentNewlineOnlySource(t *testing.T) {
	for _, payload := range []string{"\n", "\r\n"} {
		for _, lang := range []domain.Lang{domain.LangRU, domain.LangEN} {
			t.Run(string(lang)+" "+strings.ReplaceAll(payload, "\r", "CR"), func(t *testing.T) {
				st, dir, _ := setFixture(t)
				withStdin(t, strings.NewReader(payload))
				before := sandboxState(t, dir)
				_, stderr, rc := runSetCmd(st, lang, "1", "open", "-")
				if rc != 1 || stderr != domain.ErrTicketAlreadyStatus(lang, 1, "open")+"\n" {
					t.Fatalf("same-status: cmdSet = %d, stderr %q; want 1 with AlreadyStatus", rc, stderr)
				}
				if after := sandboxState(t, dir); after != before {
					t.Fatalf("sandbox changed on rejected same-status set:\nbefore=%s\nafter=%s", before, after)
				}
				withStdin(t, strings.NewReader(payload))
				stdout, stderr, rc := runSetCmd(st, lang, "1", "wip", "-")
				if rc != 0 || stderr != "" {
					t.Fatalf("status change: cmdSet = %d, stderr %q; want 0 and empty stderr", rc, stderr)
				}
				body := readTicket(t, strings.TrimSpace(stdout))
				if journal := body[strings.Index(body, "## Journal"):]; strings.Contains(journal, "· ") {
					t.Fatalf("journal line leaked from newline-only payload:\n%s", body)
				}
			})
		}
	}
}

// TestSetCommentTrailingNewlineScope pins the T-0225 scope: the strip is
// single-token only — a multi-token join keeps the trailing "\n" verbatim
// and still fails the guard (D1: join is not normalized), while a single
// LITERAL token "x\n" goes through resolveDetails and is normalized.
func TestSetCommentTrailingNewlineScope(t *testing.T) {
	for _, lang := range []domain.Lang{domain.LangRU, domain.LangEN} {
		t.Run(string(lang), func(t *testing.T) {
			st, dir, _ := setFixture(t)
			before := sandboxState(t, dir)
			stdout, stderr, rc := runSetCmd(st, lang, "1", "wip", "x", "y\n")
			if rc != 1 || stdout != "" || stderr != domain.ErrJournalNewline(lang)+"\n" {
				t.Fatalf("multi-token: cmdSet = %d, stdout %q, stderr %q; want 1 with ErrJournalNewline", rc, stdout, stderr)
			}
			if after := sandboxState(t, dir); after != before {
				t.Fatalf("sandbox changed on rejected multi-token set:\nbefore=%s\nafter=%s", before, after)
			}
			stdout, stderr, rc = runSetCmd(st, lang, "1", "wip", "x\n")
			if rc != 0 || stderr != "" {
				t.Fatalf("literal: cmdSet = %d, stderr %q; want 0 and empty stderr", rc, stderr)
			}
			if body := readTicket(t, strings.TrimSpace(stdout)); !strings.Contains(body, "· x (tester)") {
				t.Fatalf("journal missing %q:\n%s", "· x (tester)", body)
			}
		})
	}
}
