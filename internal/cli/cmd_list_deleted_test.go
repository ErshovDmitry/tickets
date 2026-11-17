package cli

// T-0189 W3: list deleted integration coverage. The mode lists
// tickets/deleted/ with the same AND filters as archive (-P/--type/
// /--priority/--json), never applies the status filter, hides deleted
// tickets from every other mode (all stays main-only), names the mode in
// the empty message and keeps the duplicate warning non-fatal (exit 0).
// Every scenario drives the real dispatch inside a t.TempDir() sandbox.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTicketPath creates one ticket through the CLI and returns its path.
func newTicketPath(t *testing.T, dir, lang, title string, extra ...string) string {
	t.Helper()
	args := append([]string{"new", title}, extra...)
	so, se, code := runDeleteCLI(t, dir, lang, args...)
	if code != 0 {
		t.Fatalf("new %q: code=%d stderr=%q", title, code, se)
	}
	return strings.TrimSpace(so)
}

// deleteTicketByPath deletes the ticket at path via the CLI and returns the
// deleted file path printed on stdout.
func deleteTicketByPath(t *testing.T, dir, lang, path string) string {
	t.Helper()
	base := filepath.Base(path)
	num := strings.TrimPrefix(strings.TrimSuffix(base, filepath.Ext(base))[:6], "T-")
	so, se, code := runDeleteCLI(t, dir, lang, "delete", num, "--yes")
	if code != 0 {
		t.Fatalf("delete %s: code=%d stdout=%q stderr=%q", num, code, so, se)
	}
	return strings.TrimSpace(so)
}

// duplicateIntoDeleted writes a copy of the main ticket file (same number)
// into tickets/deleted/ under its deleted name — the cheapest way to get a
// real main+deleted duplicate without touching the store.
func duplicateIntoDeleted(t *testing.T, dir, mainPath string) string {
	t.Helper()
	data, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read %s: %v", mainPath, err)
	}
	deletedDir := filepath.Join(dir, "deleted")
	if err := os.MkdirAll(deletedDir, 0o755); err != nil {
		t.Fatalf("mkdir deleted: %v", err)
	}
	var n int
	if _, err := fmt.Sscanf(filepath.Base(mainPath), "T-%04d", &n); err != nil {
		t.Fatalf("parse number from %s: %v", mainPath, err)
	}
	p := filepath.Join(deletedDir, fmt.Sprintf("T-%04d-deleted.md", n))
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

// TestListDeleted_ShowsDeletedAndHidesFromOtherModes pins the visibility
// contract: deleted lists only deleted/, every other mode (incl. all)
// hides deleted tickets.
func TestListDeleted_ShowsDeletedAndHidesFromOtherModes(t *testing.T) {
	dir := resolvedTempDir(t)
	deleteTicketByPath(t, dir, "ru", newTicketPath(t, dir, "ru", "gone ticket"))
	newTicketPath(t, dir, "ru", "live ticket")

	so, se, code := runDeleteCLI(t, dir, "ru", "list", "deleted")
	if code != 0 {
		t.Fatalf("list deleted: code=%d stderr=%q", code, se)
	}
	if !strings.Contains(so, "T-0001") || strings.Contains(so, "T-0002") {
		t.Errorf("list deleted stdout = %q, want only T-0001", so)
	}

	// all stays main-only: the deleted ticket must not leak into it.
	for _, mode := range []string{"active", "open", "wip", "done", "closed", "archive", "all"} {
		so, se, code := runDeleteCLI(t, dir, "ru", "list", mode)
		if code != 0 {
			t.Fatalf("list %s: code=%d stderr=%q", mode, code, se)
		}
		if strings.Contains(so, "T-0001") {
			t.Errorf("list %s stdout = %q leaks the deleted T-0001", mode, so)
		}
	}
	if so, se, _ := runDeleteCLI(t, dir, "ru", "list", "all"); !strings.Contains(so, "T-0002") {
		t.Errorf("list all stdout = %q must still show the main ticket (stderr %q)", so, se)
	}
}

// TestListDeleted_EmptyMessageNamesMode pins the localized empty message:
// the filter description names the deleted mode (same shape as archive).
func TestListDeleted_EmptyMessageNamesMode(t *testing.T) {
	dir := resolvedTempDir(t)
	so, se, code := runDeleteCLI(t, dir, "ru", "list", "deleted")
	if code != 0 {
		t.Fatalf("list deleted: code=%d stderr=%q", code, se)
	}
	if want := "Нет тикетов (deleted).\n"; so != want {
		t.Errorf("stdout = %q, want %q", so, want)
	}
	so, se, code = runDeleteCLI(t, dir, "en", "list", "deleted")
	if code != 0 {
		t.Fatalf("list deleted EN: code=%d stderr=%q", code, se)
	}
	if want := "No tickets (deleted).\n"; so != want {
		t.Errorf("stdout = %q, want %q", so, want)
	}
}

// TestListDeleted_FiltersAND pins the archive parity: -P/--type/--priority
// combine by AND and the status filter never applies to the deleted source.
func TestListDeleted_FiltersAND(t *testing.T) {
	dir := resolvedTempDir(t)
	deleteTicketByPath(t, dir, "ru", newTicketPath(t, dir, "ru", "bug high", "-t", "BUG", "-p", "high", "-P", "alpha"))
	deleteTicketByPath(t, dir, "ru", newTicketPath(t, dir, "ru", "ops low", "-t", "OPS", "-p", "low", "-P", "beta"))

	cases := []struct {
		name string
		args []string
		want string // number that must appear; "" = empty result
	}{
		{"no filters", []string{"list", "deleted"}, "T-0001"},
		{"type match", []string{"list", "deleted", "--type", "OPS"}, "T-0002"},
		{"type miss", []string{"list", "deleted", "--type", "TD"}, ""},
		{"priority match", []string{"list", "deleted", "--priority", "high"}, "T-0001"},
		{"project match", []string{"list", "deleted", "-P", "beta"}, "T-0002"},
		{"AND both", []string{"list", "deleted", "--type", "BUG", "--priority", "high", "-P", "alpha"}, "T-0001"},
		{"AND miss", []string{"list", "deleted", "--type", "BUG", "-P", "beta"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			so, se, code := runDeleteCLI(t, dir, "ru", tc.args...)
			if code != 0 {
				t.Fatalf("%v: code=%d stderr=%q", tc.args, code, se)
			}
			if tc.want == "" {
				if !strings.Contains(so, "Нет тикетов (deleted") {
					t.Errorf("stdout = %q, want empty-result message", so)
				}
				return
			}
			if !strings.Contains(so, tc.want) {
				t.Errorf("stdout = %q, missing %s", so, tc.want)
			}
		})
	}

	// JSON mode honors the same filters and shape (6 fields).
	so, se, code := runDeleteCLI(t, dir, "ru", "list", "deleted", "--type", "OPS", "--json")
	if code != 0 {
		t.Fatalf("list deleted --json: code=%d stderr=%q", code, se)
	}
	if !strings.Contains(so, "\"number\":2") || strings.Contains(so, "\"number\":1") {
		t.Errorf("json stdout = %q, want only ticket 2", so)
	}
	so, se, code = runDeleteCLI(t, dir, "ru", "list", "deleted", "--type", "TD", "--json")
	if code != 0 || so != "[]\n" {
		t.Errorf("empty json: code=%d stdout=%q stderr=%q, want 0/[]", code, so, se)
	}
}

// TestListDeleted_ParseWarningNonFatal pins archive parity for a broken
// file inside deleted/: the warning lands on stderr, the exit stays 0 and
// the parseable ticket still lists.
func TestListDeleted_ParseWarningNonFatal(t *testing.T) {
	dir := resolvedTempDir(t)
	deleteTicketByPath(t, dir, "ru", newTicketPath(t, dir, "ru", "good ticket"))
	deletedDir := filepath.Join(dir, "deleted")
	if err := os.WriteFile(filepath.Join(deletedDir, "garbage.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write garbage: %v", err)
	}
	so, se, code := runDeleteCLI(t, dir, "ru", "list", "deleted")
	if code != 0 {
		t.Fatalf("list deleted: code=%d stderr=%q", code, se)
	}
	if !strings.Contains(so, "T-0001") {
		t.Errorf("stdout = %q, want the parseable ticket", so)
	}
	if !strings.Contains(se, "garbage.md") {
		t.Errorf("stderr = %q, want a parse warning naming garbage.md", se)
	}
}

// TestListDeleted_DuplicateMainDeletedWarnsExit0 pins the W1 duplicate
// contract for the deleted source: one warning line naming both paths on
// stderr, the rows still print, the exit stays 0.
func TestListDeleted_DuplicateMainDeletedWarnsExit0(t *testing.T) {
	dir := resolvedTempDir(t)
	mainPath := newTicketPath(t, dir, "ru", "dup ticket")
	deletedPath := duplicateIntoDeleted(t, dir, mainPath)
	so, se, code := runDeleteCLI(t, dir, "ru", "list", "deleted")
	if code != 0 {
		t.Fatalf("list deleted: code=%d stderr=%q", code, se)
	}
	if !strings.Contains(so, "T-0001") {
		t.Errorf("stdout = %q, want the deleted row", so)
	}
	if !strings.Contains(se, "неоднозначен") || !strings.Contains(se, deletedPath) || !strings.Contains(se, mainPath) {
		t.Errorf("stderr = %q, want one duplicate warning naming %s and %s", se, mainPath, deletedPath)
	}
}
