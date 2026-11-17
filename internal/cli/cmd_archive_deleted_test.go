package cli

// T-0189 W3: archive stays deleted-blind by design (the store seams were
// deliberately NOT widened). These regression tests pin that contract at
// the CLI surface: a deleted-only number is a plain not-found, archive-all
// never moves a deleted ticket (and never restores one), and a main+
// deleted duplicate is skipped with the ambiguity warning rather than
// archived. The archive code itself is unchanged.

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestArchiveDeletedOnlyNotFound pins that `archive N` on a number living
// only in tickets/deleted/ reports the plain not-found (exit 1) — the
// archive lookup never widens into deleted/.
func TestArchiveDeletedOnlyNotFound(t *testing.T) {
	for _, lang := range []string{"ru", "en"} {
		t.Run(lang, func(t *testing.T) {
			dir := resolvedTempDir(t)
			deleteTicketByPath(t, dir, lang, newTicketPath(t, dir, lang, "gone ticket"))
			so, se, code := runDeleteCLI(t, dir, lang, "archive", "1")
			if code != exitError {
				t.Fatalf("archive 1: code=%d, want %d (stdout=%q stderr=%q)", code, exitError, so, se)
			}
			if !strings.Contains(se, "не найден") && !strings.Contains(se, "not found") {
				t.Errorf("stderr = %q, want a not-found report", se)
			}
			if so != "" {
				t.Errorf("stdout = %q, want empty", so)
			}
		})
	}
}

// TestArchiveAllIgnoresDeleted pins that archive-all moves closed main
// tickets but neither moves a deleted ticket into the archive nor
// restores one from it.
func TestArchiveAllIgnoresDeleted(t *testing.T) {
	dir := resolvedTempDir(t)
	// One done ticket in main, one deleted ticket under deleted/.
	newTicketPath(t, dir, "ru", "finished ticket")
	if _, se, code := runDeleteCLI(t, dir, "ru", "set", "1", "done"); code != 0 {
		t.Fatalf("set 1 done: code=%d stderr=%q", code, se)
	}
	deletedPath := deleteTicketByPath(t, dir, "ru", newTicketPath(t, dir, "ru", "gone ticket"))

	so, se, code := runDeleteCLI(t, dir, "ru", "archive")
	if code != 0 {
		t.Fatalf("archive all: code=%d stdout=%q stderr=%q", code, so, se)
	}
	if !strings.Contains(so, "T-0001-done.md") {
		t.Errorf("stdout = %q, want the archived done path", so)
	}

	// The deleted file stays exactly where it was...
	if after := readFileBytes(t, deletedPath); len(after) == 0 {
		t.Errorf("deleted file %s unreadable after archive", deletedPath)
	}
	// ...and no copy of it appeared in the archive directory itself.
	matches, err := filepath.Glob(filepath.Join(dir, "archive", "T-0002*"))
	if err != nil {
		t.Fatalf("glob archive dir: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("archive dir contains the deleted ticket: %v", matches)
	}
}

// TestArchiveAllSkipsMainDeletedDuplicate pins the T-0157 skip pattern for
// a number owned by main and deleted/ together: archive-all warns, skips
// the number and exits ambiguous — the closed main copy is NOT moved.
func TestArchiveAllSkipsMainDeletedDuplicate(t *testing.T) {
	dir := resolvedTempDir(t)
	newTicketPath(t, dir, "ru", "dup ticket")
	if _, se, code := runDeleteCLI(t, dir, "ru", "set", "1", "done"); code != 0 {
		t.Fatalf("set 1 done: code=%d stderr=%q", code, se)
	}
	// set renamed the file: the duplicate source is the done name.
	donePath := filepath.Join(dir, "T-0001-done.md")
	deletedPath := duplicateIntoDeleted(t, dir, donePath)

	so, se, code := runDeleteCLI(t, dir, "ru", "archive")
	if code != exitAmbiguous {
		t.Fatalf("archive all: code=%d, want %d (stdout=%q stderr=%q)", code, exitAmbiguous, so, se)
	}
	if !strings.Contains(se, "неоднозначен") || !strings.Contains(se, donePath) || !strings.Contains(se, deletedPath) {
		t.Errorf("stderr = %q, want the skip warning naming both paths", se)
	}
	if strings.Contains(so, "T-0001-done.md") && strings.Contains(so, "archive") {
		t.Errorf("stdout = %q must not report the skipped duplicate as archived", so)
	}
	if after := readFileBytes(t, donePath); len(after) == 0 {
		t.Errorf("main duplicate moved despite the skip")
	}
}
