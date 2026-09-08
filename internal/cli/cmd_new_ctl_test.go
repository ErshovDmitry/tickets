package cli

import (
	"bytes"
	"io"
	"path/filepath"
	"testing"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// TestNewRejectsCTLInTitle pins T-0075: a title carrying a C0 control
// character (0x00–0x1F except TAB) or DEL (0x7F) is rejected with rc=1 and
// the localized errTitleCTL message before any store access — no ticket
// file is created. TAB and multi-byte UTF-8 (emoji) titles are allowed
// (rc=0). The LF case guards the removed T-0044 ContainsAny check.
func TestNewRejectsCTLInTitle(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := store.New(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		title string
		want  int
	}{
		{"BEL", "test\x07title", 1},
		{"NUL", "test\x00title", 1},
		{"ESC", "test\x1Btitle", 1},
		{"DEL", "test\x7Ftitle", 1},
		{"LF", "test\ntitle", 1},
		{"TAB_allowed", "test\ttitle", 0},
		{"emoji_allowed", "test 🚀 title", 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stderrBuf bytes.Buffer
			rc := cmdNew(st, []string{tc.title, "-t", "BUG", "-p", "low"}, "test", "prj", domain.LangEN, io.Discard, &stderrBuf)
			if rc != tc.want {
				t.Fatalf("cmdNew(%q) = %d, want %d; stderr: %q", tc.title, rc, tc.want, stderrBuf.String())
			}
			if tc.want == 0 {
				return
			}
			wantStderr := domain.ErrTitleCTL(domain.LangEN) + "\n"
			if stderrBuf.String() != wantStderr {
				t.Fatalf("stderr = %q, want %q", stderrBuf.String(), wantStderr)
			}
			if created, _ := filepath.Glob(filepath.Join(tmpDir, "T-*.md")); len(created) != 0 {
				t.Fatalf("ticket file created despite CTL title: %v", created)
			}
		})
	}
}
