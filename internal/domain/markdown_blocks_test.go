package domain

import (
	"strings"
	"testing"
)

// T-0230 / GitHub #56: spaceDetailBlocks inserts MD022/MD032 blank lines
// into NEW-ticket Details only. Every case pins exact bytes: only blank
// lines may appear, original lines and EOLs stay untouched. Each case also
// checks idempotence and non-whitespace byte preservation.

func TestSpaceDetailBlocks(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"text before unordered list", "Факт:\n- первый\n- второй\n", "Факт:\n\n- первый\n- второй\n"},
		{"text before ordered list", "text\n1. a\n2. b\n", "text\n\n1. a\n2. b\n"},
		{"text before paren ordered", "text\n1) a\n", "text\n\n1) a\n"},
		{"ordered start 2 no interrupt", "text\n2. b\n", "text\n2. b\n"},
		{"ordered start 9 no interrupt", "text\n9. b\n", "text\n9. b\n"},
		{"ordered paren 9 no interrupt", "text\n9) b\n", "text\n9) b\n"},
		{"ordered start 0 no interrupt", "text\n0. b\n", "text\n0. b\n"},
		{"ordered start 10 no interrupt", "text\n10. b\n", "text\n10. b\n"},
		{"ordered leading zero interrupts", "text\n01. b\n", "text\n\n01. b\n"},
		{"ordered start 2 after blank stays list", "text\n\n2. b\n- c\n", "text\n\n2. b\n- c\n"},
		{"ordered start 2 after thematic stays list", "---\n2. b\n- c\n", "---\n\n2. b\n- c\n"},
		{"ordered start 2 after heading", "## H\n2. b\n", "## H\n\n2. b\n"},
		{"start 1 after demoted ordered", "text\n2. b\n1. c\n", "text\n2. b\n\n1. c\n"},
		{"unordered after demoted ordered", "text\n2. b\n- c\n", "text\n2. b\n\n- c\n"},
		{"ordered start 2 after indented continuation", "para\n    cont\n2. item\n", "para\n    cont\n2. item\n"},
		{"ordered start 2 after tab continuation", "para\n\tcont\n2. item\n", "para\n\tcont\n2. item\n"},
		{"ordered start 2 after lazy continuation", "para\n   cont\n2. item\n", "para\n   cont\n2. item\n"},
		{"ordered start 2 after indented run", "para\n    a\n    b\n2. item\n", "para\n    a\n    b\n2. item\n"},
		{"ordered start 3 after demoted and continuation", "para\n2. a\n    cont\n3. b\n", "para\n2. a\n    cont\n3. b\n"},
		{"ordered start 1 after indented continuation", "para\n    cont\n1. item\n", "para\n    cont\n\n1. item\n"},
		{"unordered after indented continuation", "para\n    cont\n- item\n", "para\n    cont\n\n- item\n"},
		{"ordered start 2 after standalone indented", "    code\n2. b\n", "    code\n\n2. b\n"},
		{"ordered start 2 after blank and indented", "para\n\n    code\n2. b\n", "para\n\n    code\n\n2. b\n"},
		{"ordered start 2 after heading and indented", "## H\n    code\n2. b\n", "## H\n    code\n\n2. b\n"},
		{"ordered start 2 after fence and indented", "```\ncode\n```\n    code2\n2. b\n", "```\ncode\n```\n    code2\n\n2. b\n"},
		{"ordered start 2 after thematic and indented", "---\n    code\n2. b\n", "---\n    code\n\n2. b\n"},
		{"crlf indented continuation", "para\r\n    cont\r\n2. item\r\n", "para\r\n    cont\r\n2. item\r\n"},
		{"plus and star markers", "text\n+ a\n* b\n", "text\n\n+ a\n* b\n"},
		{"tab after marker", "text\n-\ta\n", "text\n\n-\ta\n"},
		{"empty unordered marker keeps setext", "text\n- \nmore\n", "text\n- \nmore\n"},
		{"empty star marker", "text\n* \nmore\n", "text\n* \nmore\n"},
		{"empty plus tab marker", "text\n+\t\nmore\n", "text\n+\t\nmore\n"},
		{"empty ordered marker", "text\n1. \nmore\n", "text\n1. \nmore\n"},
		{"empty paren ordered marker", "text\n1) \nmore\n", "text\n1) \nmore\n"},
		{"empty marker whitespace run", "text\n-  \t\nmore\n", "text\n-  \t\nmore\n"},
		{"empty ordered start 2", "text\n2. \nmore\n", "text\n2. \nmore\n"},
		{"empty marker at payload end", "text\n- \n", "text\n- \n"},
		{"empty marker between items", "- a\n- \n- b\n", "- a\n- \n- b\n"},
		{"empty marker crlf", "text\r\n- \r\nmore\r\n", "text\r\n- \r\nmore\r\n"},
		{"heading between text", "Тело:\n## Подзаголовок\nтекст\n", "Тело:\n\n## Подзаголовок\n\nтекст\n"},
		{"heading then list one blank", "## H\n- a\n", "## H\n\n- a\n"},
		{"text heading list", "text\n## H\n- a\n", "text\n\n## H\n\n- a\n"},
		{"list then heading", "- a\n## H\ntext\n", "- a\n\n## H\n\ntext\n"},
		{"heading then heading", "## A\n## B\n", "## A\n\n## B\n"},
		{"heading at payload start", "## H\ntext\n", "## H\n\ntext\n"},
		{"heading at payload end", "text\n## H\n", "text\n\n## H\n"},
		{"indented heading three spaces", "text\n   ## H\n", "text\n\n   ## H\n"},
		{"list at payload start", "- a\n- b\n", "- a\n- b\n"},
		{"list at payload end", "text\n- a\n", "text\n\n- a\n"},
		{"already clean", "text\n\n- a\n\n## H\n\ntext\n", "text\n\n- a\n\n## H\n\ntext\n"},
		{"siblings and nested", "- a\n- b\n  - c\n    - d\n- e\n", "- a\n- b\n  - c\n    - d\n- e\n"},
		{"lazy continuation", "- a\nmore text\n", "- a\nmore text\n"},
		{"ordered lazy continuation", "1. a\nmore\n2. b\n", "1. a\nmore\n2. b\n"},
		{"loose list continuation", "- a\n\n  cont\n\n- b\n", "- a\n\n  cont\n\n- b\n"},
		{"list ends at blank then new list", "- a\n\ntext\n- b\n", "- a\n\ntext\n\n- b\n"},
		{"list then thematic break", "- a\n---\n", "- a\n\n---\n"},
		{"thematic then list", "---\n- a\n", "---\n\n- a\n"},
		{"thematic then heading", "---\n## H\n", "---\n\n## H\n"},
		{"text thematic list", "text\n---\n- a\n", "text\n---\n\n- a\n"},
		{"heading then thematic", "## H\n---\n", "## H\n\n---\n"},
		{"single line", "text\n", "text\n"},
		{"empty", "", ""},
		{"lf only", "\n", "\n"},
		{"whitespace only", "  \n\t\n", "  \n\t\n"},
		{"crlf list", "text\r\n- a\r\n", "text\r\n\r\n- a\r\n"},
		{"crlf heading", "text\r\n## H\r\nmore\r\n", "text\r\n\r\n## H\r\n\r\nmore\r\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := spaceDetailBlocks(tc.in)
			if got != tc.want {
				t.Fatalf("spaceDetailBlocks(%q) =\n%q\nwant\n%q", tc.in, got, tc.want)
			}
			if again := spaceDetailBlocks(got); again != got {
				t.Fatalf("not idempotent: second pass =\n%q", again)
			}
			assertNonBlankBytesPreserved(t, tc.in, got)
		})
	}
}

// TestListMarker pins the marker classifier directly: validity plus the
// ordered start number used for the paragraph-interruption rule.
func TestListMarker(t *testing.T) {
	positive := []struct {
		text  string
		start int
	}{
		{"- a", 1}, {"+ a", 1}, {"* a", 1}, {"-\ta", 1},
		{"1. a", 1}, {"9. a", 9}, {"1) a", 1}, {"9) a", 9},
		{"01. a", 1}, {"10. a", 10}, {"123456789. a", 123456789},
		{"-  a", 1}, {"1.  a", 1}, {"1)\ta", 1}, {"-\t \ta", 1},
	}
	for _, tc := range positive {
		if start, ok := listMarker(tc.text); !ok || start != tc.start {
			t.Errorf("listMarker(%q) = %d,%v; want %d,true", tc.text, start, ok, tc.start)
		}
	}
	for _, text := range []string{"", "-", "1.", "1)", "-item", "1.item", "1234567890. a", "1 . a", "  - a", "## x", "text",
		"- ", "* ", "+\t", "1. ", "1) ", "-  ", "-\t", "1.  \t", "- \t ", "9) \t"} {
		if _, ok := listMarker(text); ok {
			t.Errorf("listMarker(%q) accepted, want rejected", text)
		}
	}
}

// TestSpaceDetailBlocksHeadingLevels covers ATX levels 1-6.
func TestSpaceDetailBlocksHeadingLevels(t *testing.T) {
	for level := 1; level <= 6; level++ {
		h := strings.Repeat("#", level) + " H"
		in := "text\n" + h + "\nmore\n"
		want := "text\n\n" + h + "\n\nmore\n"
		if got := spaceDetailBlocks(in); got != want {
			t.Fatalf("level %d: got %q, want %q", level, got, want)
		}
	}
}

// assertNonBlankBytesPreserved checks that every non-whitespace line of the
// input survives byte-for-byte (raw bytes including its EOL) in the output:
// the helper may only add blank lines.
func assertNonBlankBytesPreserved(t *testing.T, in, out string) {
	t.Helper()
	nonBlank := func(s string) []string {
		var keep []string
		for _, ln := range strings.SplitAfter(s, "\n") {
			if ln != "" && strings.TrimSpace(ln) != "" {
				keep = append(keep, ln)
			}
		}
		return keep
	}
	a, b := nonBlank(in), nonBlank(out)
	if len(a) != len(b) {
		t.Fatalf("non-blank line count changed: %d -> %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("non-blank line %d changed: %q -> %q", i, a[i], b[i])
		}
	}
}
