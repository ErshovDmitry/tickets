package cli_test

// T-0241 code review R3.8: the --json case of an AMBIGUOUS number. The
// --section case is pinned in cmd_show_modes_test.go; --json must behave
// identically — exit 2, every conflicting path on stderr, nothing on stdout —
// because the ambiguity is detected BEFORE any body is read and before the
// document is built.

import (
	"strings"
	"testing"
)

// TestShowJSONAmbiguous pins the exit-2 contract of the JSON mode for both
// ambiguity shapes: two files of the same number in s.Dir, and a live file
// colliding with one in tickets/deleted/.
func TestShowJSONAmbiguous(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
		want  []string
	}{
		{
			name: "two live files",
			setup: func(t *testing.T, dir string) {
				writeShowFile(t, dir, "T-0001-wip.md", showTicketSrc(1, "wip", "## Summary", "## Details", "", ""))
			},
			want: []string{"T-0001-open.md", "T-0001-wip.md"},
		},
		{
			name: "live and deleted",
			setup: func(t *testing.T, dir string) {
				writeShowFile(t, dir, "deleted/T-0001-deleted.md", showTicketSrc(1, "deleted", "## Summary", "## Details", "", ""))
			},
			want: []string{"T-0001-open.md", "T-0001-deleted.md"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := newShowSandbox(t)
			tc.setup(t, dir)
			code, out, errOut := runShow([]string{"show", "1", "--json"}, dir)
			if code != 2 {
				t.Fatalf("code = %d, want 2 (stderr: %s)", code, errOut)
			}
			if out != "" {
				t.Errorf("stdout = %q, want empty: no partial document", out)
			}
			for _, name := range tc.want {
				if !strings.Contains(errOut, name) {
					t.Errorf("stderr = %q, want it to name %s", errOut, name)
				}
			}
		})
	}
}
