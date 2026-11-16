package cli

// T-0189 W3: show deleted-aware integration coverage. FindRawAcross/
// FindExactAcross resolve a ticket in tickets/deleted/ by number and by
// exact base name; a number carried by main and deleted/ together is the
// T-0157 ambiguity (exit 2, every absolute path); the not-found hints
// count deleted/ in the store bound (a deleted-only store is neither
// empty nor blind to its maximum number).

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestShowDeleted_ByNumber prints the deleted file's exact bytes.
func TestShowDeleted_ByNumber(t *testing.T) {
	dir := resolvedTempDir(t)
	deletedPath := deleteTicketByPath(t, dir, "ru", newTicketPath(t, dir, "ru", "gone ticket"))
	raw, err := os.ReadFile(deletedPath)
	if err != nil {
		t.Fatalf("read deleted: %v", err)
	}
	so, se, code := runDeleteCLI(t, dir, "ru", "show", "1")
	if code != 0 {
		t.Fatalf("show 1: code=%d stderr=%q", code, se)
	}
	if so != string(raw) {
		t.Errorf("stdout = %q, want the deleted file bytes", so)
	}
}

// TestShowDeleted_ByExactName pins the T-0157 escape hatch for the
// deleted source: an exact base name resolves the very file even when
// the number alone would be ambiguous.
func TestShowDeleted_ByExactName(t *testing.T) {
	dir := resolvedTempDir(t)
	deletedPath := deleteTicketByPath(t, dir, "ru", newTicketPath(t, dir, "ru", "gone ticket"))
	raw, err := os.ReadFile(deletedPath)
	if err != nil {
		t.Fatalf("read deleted: %v", err)
	}
	name := filepath.Base(deletedPath)
	so, se, code := runDeleteCLI(t, dir, "ru", "show", name)
	if code != 0 {
		t.Fatalf("show %s: code=%d stderr=%q", name, code, se)
	}
	if so != string(raw) {
		t.Errorf("stdout = %q, want the deleted file bytes", so)
	}
}

// TestShowDeleted_AmbiguityAcrossDirs pins the cross-directory ambiguity:
// the same number in main and deleted/ is exit 2 with BOTH absolute paths,
// never a silent pick. Paths derive from resolvedTempDir (already
// EvalSymlinks-canonical, which also expands Windows 8.3 short names), so
// the CLI prints them byte-for-byte.
func TestShowDeleted_AmbiguityAcrossDirs(t *testing.T) {
	dir := resolvedTempDir(t)
	mainPath := newTicketPath(t, dir, "ru", "dup ticket")
	deletedPath := duplicateIntoDeleted(t, dir, mainPath)
	so, se, code := runDeleteCLI(t, dir, "ru", "show", "1")
	if code != exitAmbiguous {
		t.Fatalf("show 1: code=%d, want %d (stderr=%q)", code, exitAmbiguous, se)
	}
	if so != "" {
		t.Errorf("stdout = %q, want empty", so)
	}
	if !strings.Contains(se, "неоднозначен") {
		t.Errorf("stderr = %q, want the ambiguity header", se)
	}
	for _, p := range []string{mainPath, deletedPath} {
		if !strings.Contains(se, p) {
			t.Errorf("stderr = %q, missing conflicting path %s", se, p)
		}
	}
}

// TestShowDeleted_NotFoundHintCountsDeleted pins the deleted-aware
// scanMax: with only a deleted ticket present the store is not "empty"
// and its maximum (7 here) bounds the hint for an above-max request.
func TestShowDeleted_NotFoundHintCountsDeleted(t *testing.T) {
	dir := resolvedTempDir(t)
	var gapPath string
	for i := range 7 {
		p := newTicketPath(t, dir, "ru", fmt.Sprintf("gone %d", i))
		if i == 3 {
			gapPath = p
			continue // number 4 is removed below: deleted holds 1,2,3,5,6,7
		}
		deleteTicketByPath(t, dir, "ru", p)
	}
	if err := os.Remove(gapPath); err != nil {
		t.Fatalf("remove gap ticket: %v", err)
	}

	// Above the deleted maximum: the bound names 7, not 0.
	_, se, code := runDeleteCLI(t, dir, "ru", "show", "42")
	if code != exitError {
		t.Fatalf("show 42: code=%d, want %d", code, exitError)
	}
	if !strings.Contains(se, "номера до 7") {
		t.Errorf("stderr = %q, want the bound hint naming 7", se)
	}
	if strings.Contains(se, "хранилище пусто") {
		t.Errorf("stderr = %q must not claim an empty store", se)
	}

	// In-range gap (4 was never deleted and has no main file): the
	// generic list hint.
	_, se, code = runDeleteCLI(t, dir, "ru", "show", "4")
	if code != exitError {
		t.Fatalf("show 5: code=%d, want %d", code, exitError)
	}
	if !strings.Contains(se, "не найден") || !strings.Contains(se, "используйте ticket list") {
		t.Errorf("stderr = %q, want not-found plus the generic list hint", se)
	}
}

// TestShowDeleted_HintsEN pins the English rendering of the same
// deleted-aware hint and proves it carries no Cyrillic leak.
func TestShowDeleted_HintsEN(t *testing.T) {
	dir := resolvedTempDir(t)
	deleteTicketByPath(t, dir, "en", newTicketPath(t, dir, "en", "gone ticket"))
	_, se, code := runDeleteCLI(t, dir, "en", "show", "42")
	if code != exitError {
		t.Fatalf("show 42 EN: code=%d, want %d", code, exitError)
	}
	if !strings.Contains(se, "numbers up to 1") {
		t.Errorf("stderr = %q, want the EN bound hint naming 1", se)
	}
	if hasCyrillic(se) {
		t.Errorf("stderr = %q contains Cyrillic in an EN run", se)
	}
}
