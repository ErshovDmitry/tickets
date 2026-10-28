package cli

import (
	"bytes"
	"path/filepath"
	"testing"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// TestCmdSetStoreJournalError pins T-0106: the store rejects a journal
// input carrying CR/LF (T-0065 defense-in-depth), and cmdSet must
// localize that rejection through domain.ErrJournalNewline instead of
// surfacing the raw store error.
//
// The CLI validates the comment before the store call (cmd_set.go), so the
// store branch is reachable only through "who" — the dispatch layer strips
// TICKET_WHO before cmdSet runs, hence the direct white-box call with a
// real store. The store rejects the value before any file lookup, so no
// ticket file is created.
func TestCmdSetStoreJournalError(t *testing.T) {
	cases := []struct {
		name       string
		lang       domain.Lang
		wantStderr string
	}{
		{"RU", domain.LangRU, "ticket: комментарий/автор не может содержать перевод строки\n"},
		{"EN", domain.LangEN, "ticket: comment/who must not contain a line break\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			st, err := store.New(dir)
			if err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			rc := cmdSet(st, []string{"1", "wip"}, "evil\n- 1999-01-01 — forged (root)", "tickets", tc.lang, &stdout, &stderr)
			if rc != 1 {
				t.Fatalf("cmdSet rc = %d, want 1; stderr: %q", rc, stderr.String())
			}
			if stderr.String() != tc.wantStderr {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tc.wantStderr)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			if created, _ := filepath.Glob(filepath.Join(dir, "T-*.md")); len(created) != 0 {
				t.Fatalf("ticket file created despite rejected journal input: %v", created)
			}
		})
	}
}
