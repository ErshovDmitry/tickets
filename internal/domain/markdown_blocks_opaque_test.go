package domain

import (
	"bytes"
	"testing"
)

// T-0230 conservative edges: opaque blocks (fences, indented code, HTML),
// setext/thematic ambiguities and non-heading/non-list shapes must receive
// NO insertion. Missing a true positive here is the documented conservative
// trade-off; a false insertion would corrupt user bytes.

func TestSpaceDetailBlocksConservativeNoInsert(t *testing.T) {
	unchanged := []struct{ name, in string }{
		{"hashtag not heading", "text\n##hashtag\nmore\n"},
		{"single hashtag", "text\n#tag\n"},
		{"seven hashes", "text\n####### x\n"},
		{"setext underline", "Title\n---\n"},
		{"setext equals", "Title\n===\n"},
		{"thematic between text", "text\n- - -\ntext\n"},
		{"thematic stars", "text\n***\ntext\n"},
		{"thematic underscores", "text\n___\ntext\n"},
		{"marker without space", "text\n-item\n1.item\n"},
		{"marker only line", "text\n-\n1.\n"},
		{"indented marker", "text\n  - a\n"},
		{"ten digit ordered", "text\n1234567890. a\n"},
		{"fence opaque", "text\n```\n- x\n## y\n```\ntext\n"},
		{"tilde fence", "text\n~~~\n## x\n~~~\n"},
		{"unclosed fence", "text\n```\n## x\n- y\n"},
		{"fence info string", "text\n```go\n## x\n```\n"},
		{"indented code", "text\n    ## not heading\n    - not list\n"},
		{"indented code then text", "text\n    code\nmore\n"},
		{"html comment opaque", "text\n<!--\n## x\n- y\n-->\ntext\n"},
		{"html comment single line", "text\n<!-- c -->\n## H\n"},
		{"short html comment then heading", "text\n<!-->\n## H\n"},
		{"short html comment double dash then heading", "text\n<!--->\n## H\n"},
		{"short closer trailing text stays open", "text\n<!-->x\nmore\n- a\n"},
		{"html tag block", "<div>\n## x\n- y\n</div>\n"},
		{"heading then indented", "## H\n    code\n"},
		{"list then indented continuation", "- a\n    code\n"},
		{"heading inside list indent", "- a\n  ## not a block heading\n"},
		{"indented fence inside list", "- a\n  ```\n  code\n  ```\n- b\n"},
		{"indented thematic inside list", "- a\n  ---\n- b\n"},
		{"indented heading after blank in list", "- a\n\n  ## h\n\n- b\n"},
		{"nbsp closer never closes fence", "text\n```\n## x\n```\u00a0\n- y\n"},
	}
	for _, tc := range unchanged {
		t.Run(tc.name, func(t *testing.T) {
			if got := spaceDetailBlocks(tc.in); got != tc.in {
				t.Fatalf("conservative no-insert violated:\n%q\nwant unchanged\n%q", got, tc.in)
			}
		})
	}
}

// TestSpaceDetailBlocksFenceBoundaries: fence delimiter lines are active
// neighbours for heading/list demands (MD022/MD032), while fence content
// stays opaque and text->fence gets no blank (MD031 is out of scope).
// CommonMark 0.31.2: a closer may be followed only by spaces/tabs, so an
// NBSP trail keeps the fence open (T-0230 V3 regression).
func TestSpaceDetailBlocksFenceBoundaries(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"heading then fence", "## H\n```\ncode\n```\n", "## H\n\n```\ncode\n```\n"},
		{"list then fence", "- a\n```\ncode\n```\n", "- a\n\n```\ncode\n```\n"},
		{"fence then list", "```\ncode\n```\n- a\n", "```\ncode\n```\n\n- a\n"},
		{"fence then heading", "```\ncode\n```\n## H\n", "```\ncode\n```\n\n## H\n"},
		{"text then fence untouched", "text\n```\ncode\n```\n", "text\n```\ncode\n```\n"},
		{"longer opener shorter closer stays open", "````\n```\n## x\n````\n- a\n", "````\n```\n## x\n````\n\n- a\n"},
		{"closer trailing text stays open", "```\ncode\n``` tail\n## x\n```\n- a\n", "```\ncode\n``` tail\n## x\n```\n\n- a\n"},
		{"longer closer closes", "```\ncode\n`````\n- a\n", "```\ncode\n`````\n\n- a\n"},
		{"indented closer closes", "```\ncode\n   ```\n- a\n", "```\ncode\n   ```\n\n- a\n"},
		{"tilde closer trailing text stays open", "~~~\ncode\n~~~ tail\n## x\n~~~\n- a\n", "~~~\ncode\n~~~ tail\n## x\n~~~\n\n- a\n"},
		{"nbsp after closer stays open", "```\ncode\n```\u00a0\n## x\n```\n- a\n", "```\ncode\n```\u00a0\n## x\n```\n\n- a\n"},
		{"space tab after closer closes", "```\ncode\n``` \t\n- a\n", "```\ncode\n``` \t\n\n- a\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := spaceDetailBlocks(tc.in)
			if got != tc.want {
				t.Fatalf("got\n%q\nwant\n%q", got, tc.want)
			}
			if again := spaceDetailBlocks(got); again != got {
				t.Fatalf("not idempotent: %q", again)
			}
			assertNonBlankBytesPreserved(t, tc.in, got)
		})
	}
}

// TestSpaceDetailBlocksIndentedCodeBoundary: standalone indented code gets a
// blank AFTER the block before a heading/list (MD022/MD032), never inside the
// block and never between an open list's indented continuation and its item.
func TestSpaceDetailBlocksIndentedCodeBoundary(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"indented code then list", "text\n    code\n- a\n", "text\n    code\n\n- a\n"},
		{"indented code then heading", "text\n    code\n## H\n", "text\n    code\n\n## H\n"},
		{"indented run then list", "text\n    a\n    b\n- c\n", "text\n    a\n    b\n\n- c\n"},
		{"tab indented then heading", "text\n\tcode\n## H\n", "text\n\tcode\n\n## H\n"},
		{"ordered start 2 does not interrupt paragraph", "text\n    code\n2. b\n", "text\n    code\n2. b\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := spaceDetailBlocks(tc.in)
			if got != tc.want {
				t.Fatalf("got\n%q\nwant\n%q", got, tc.want)
			}
			if again := spaceDetailBlocks(got); again != got {
				t.Fatalf("not idempotent: %q", again)
			}
			assertNonBlankBytesPreserved(t, tc.in, got)
		})
	}
}

// TestSpaceDetailBlocksShortHTMLComments: <!--> and <!---> are complete
// comments (CommonMark abrupt closers); later blocks must not stay swallowed.
func TestSpaceDetailBlocksShortHTMLComments(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"abrupt closer releases list", "text\n<!-->\nmore\n- a\n", "text\n<!-->\nmore\n\n- a\n"},
		{"double dash abrupt closer", "text\n<!--->\nmore\n- a\n", "text\n<!--->\nmore\n\n- a\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := spaceDetailBlocks(tc.in)
			if got != tc.want {
				t.Fatalf("got\n%q\nwant\n%q", got, tc.want)
			}
			if again := spaceDetailBlocks(got); again != got {
				t.Fatalf("not idempotent: %q", again)
			}
			assertNonBlankBytesPreserved(t, tc.in, got)
		})
	}
}

// TestNewTicketSpacingParseRenderRoundTrip pins that a spaced new ticket
// survives Parse -> Render byte-identically: normalizeDetails keeps the
// inserted single blank lines, so normalize is a no-op on the new output.
func TestNewTicketSpacingParseRenderRoundTrip(t *testing.T) {
	for _, details := range []string{
		"Факт:\n- первый\n- второй",
		"Тело:\n## Подзаголовок\nтекст",
		"## H\n- a\ntext",
		"text\n9. b\nmore",
	} {
		tk := newTestTicket()
		tk.Details = details
		body, err := RenderNewTicket(tk)
		if err != nil {
			t.Fatalf("RenderNewTicket: %v", err)
		}
		parsed, unknown, err := Parse(body)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		rendered, err := Render(parsed, unknown)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if !bytes.Equal(body, rendered) {
			t.Fatalf("round-trip changed bytes for %q:\n%s\n---\n%s", details, body, rendered)
		}
	}
}
