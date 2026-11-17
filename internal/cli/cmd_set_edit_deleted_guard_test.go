package cli

// T-0189 W3: the terminal/read-only guard for set and edit (shared
// deletedGuard in cmd_deleted_guard.go). A ticket whose file lives in
// tickets/deleted/ is refused with the localized errTicketDeletedReadOnly
// message (exit 1, file byte-unchanged) — for a plain status change, for
// a reopen (set N open) and for edit alike. A number carried by main and
// deleted/ together is the T-0157 ambiguity (exit 2, every path). A broken
// deleted/ directory surfaces as a real "ticket: <error>" line (exit 1),
// never masked as not-found. `set N deleted` is rejected earlier by the
// pinned invalid-status message: statusByName does not know deleted.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// readFileBytes reads a file for byte-identity assertions.
func readFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// assertDeletedRefusal pins the guard refusal: exit 1, the localized
// message on stderr, empty stdout, byte-unchanged file; an EN run must
// carry no Cyrillic.
func assertDeletedRefusal(t *testing.T, lang domain.Lang, name, se string, code int, want string, path string, before []byte) {
	t.Helper()
	if code != exitError {
		t.Errorf("%s: code=%d, want %d (stderr=%q)", name, code, exitError, se)
	}
	if se != want+"\n" {
		t.Errorf("%s: stderr=%q, want byte-exact %q", name, se, want+"\n")
	}
	if lang == "en" && hasCyrillic(se) {
		t.Errorf("%s: EN stderr contains Cyrillic: %q", name, se)
	}
	if after := readFileBytes(t, path); string(after) != string(before) {
		t.Errorf("%s: file %s changed\nbefore: %q\nafter: %q", name, path, before, after)
	}
}

// TestSetDeletedRefusalRUEN drives `set N <status>` against a deleted-only
// number in both languages and pins the refusal byte-for-byte.
func TestSetDeletedRefusalRUEN(t *testing.T) {
	for _, lang := range []domain.Lang{domain.LangRU, domain.LangEN} {
		t.Run(string(lang), func(t *testing.T) {
			dir := resolvedTempDir(t)
			deletedPath := deleteTicketByPath(t, dir, string(lang), newTicketPath(t, dir, string(lang), "gone ticket"))
			before := readFileBytes(t, deletedPath)
			_, se, code := runDeleteCLI(t, dir, string(lang), "set", "1", "wip", "note")
			want := domain.ErrTicketDeletedReadOnly(lang, 1)
			assertDeletedRefusal(t, lang, "set wip", se, code, want, deletedPath, before)
		})
	}
}

// TestSetDeletedReopenRefused pins that a reopen (set N open) hits the
// same terminal guard: deleted is not a backdoor to resurrection.
func TestSetDeletedReopenRefused(t *testing.T) {
	for _, lang := range []domain.Lang{domain.LangRU, domain.LangEN} {
		t.Run(string(lang), func(t *testing.T) {
			dir := resolvedTempDir(t)
			deletedPath := deleteTicketByPath(t, dir, string(lang), newTicketPath(t, dir, string(lang), "gone ticket"))
			before := readFileBytes(t, deletedPath)
			_, se, code := runDeleteCLI(t, dir, string(lang), "set", "1", "open")
			want := domain.ErrTicketDeletedReadOnly(lang, 1)
			assertDeletedRefusal(t, lang, "set open", se, code, want, deletedPath, before)
		})
	}
}

// TestSetDeletedStatusArgInvalid pins that `set N deleted` is rejected by
// the pinned invalid-status message: statusByName never learns deleted,
// so the deleted status cannot be assigned through set at all.
func TestSetDeletedStatusArgInvalid(t *testing.T) {
	for _, lang := range []domain.Lang{domain.LangRU, domain.LangEN} {
		t.Run(string(lang), func(t *testing.T) {
			dir := resolvedTempDir(t)
			mainPath := newTicketPath(t, dir, string(lang), "live ticket")
			before := readFileBytes(t, mainPath)
			_, se, code := runDeleteCLI(t, dir, string(lang), "set", "1", "deleted")
			if code != exitError {
				t.Fatalf("set 1 deleted: code=%d, want %d (stderr=%q)", code, exitError, se)
			}
			if want := domain.ErrStatusInvalid(lang); se != want+"\n" {
				t.Errorf("stderr=%q, want byte-exact %q", se, want+"\n")
			}
			if lang == domain.LangEN && hasCyrillic(se) {
				t.Errorf("EN stderr contains Cyrillic: %q", se)
			}
			if after := readFileBytes(t, mainPath); string(after) != string(before) {
				t.Errorf("main file changed on a rejected status arg")
			}
		})
	}
}

// TestSetDeletedDuplicateExit2 pins the cross-directory ambiguity for a
// mutating command: the number lives in main and deleted/, the guard
// refuses with exit 2 and lists BOTH absolute paths (deduped, sorted),
// and neither file is touched.
func TestSetDeletedDuplicateExit2(t *testing.T) {
	dir := resolvedTempDir(t)
	mainPath := newTicketPath(t, dir, "ru", "dup ticket")
	deletedPath := duplicateIntoDeleted(t, dir, mainPath)
	mainBefore := readFileBytes(t, mainPath)
	deletedBefore := readFileBytes(t, deletedPath)
	so, se, code := runDeleteCLI(t, dir, "ru", "set", "1", "wip")
	if code != exitAmbiguous {
		t.Fatalf("set 1 wip: code=%d, want %d (stderr=%q)", code, exitAmbiguous, se)
	}
	if so != "" {
		t.Errorf("stdout = %q, want empty", so)
	}
	if !strings.Contains(se, "неоднозначен") {
		t.Errorf("stderr = %q, want the ambiguity header", se)
	}
	// Paths derive from resolvedTempDir (already EvalSymlinks-canonical, the
	// cmd_delete_test.go precedent), so the CLI prints them byte-for-byte.
	for _, p := range []string{mainPath, deletedPath} {
		if !strings.Contains(se, p) {
			t.Errorf("stderr = %q, missing path %s", se, p)
		}
	}
	if after := readFileBytes(t, mainPath); string(after) != string(mainBefore) {
		t.Errorf("main file changed under the guard")
	}
	if after := readFileBytes(t, deletedPath); string(after) != string(deletedBefore) {
		t.Errorf("deleted file changed under the guard")
	}
}

// TestSetDeletedGuardSeamError pins the guard-seam failure mode: an
// invalid (regular file, not a directory) tickets/deleted/ makes the
// guard refuse with the raw store error ("ticket: <error>", exit 1) —
// never masked as a not-found report — and the main file is unchanged.
func TestSetDeletedGuardSeamError(t *testing.T) {
	dir := resolvedTempDir(t)
	mainPath := newTicketPath(t, dir, "ru", "live ticket")
	before := readFileBytes(t, mainPath)
	if err := os.WriteFile(filepath.Join(dir, "deleted"), []byte("x"), 0o644); err != nil {
		t.Fatalf("plant invalid deleted/: %v", err)
	}
	_, se, code := runDeleteCLI(t, dir, "ru", "set", "1", "wip")
	if code != exitError {
		t.Fatalf("set 1 wip: code=%d, want %d (stderr=%q)", code, exitError, se)
	}
	if !strings.HasPrefix(se, "ticket: ") {
		t.Errorf("stderr = %q, want the raw store error line", se)
	}
	if strings.Contains(se, "не найден") {
		t.Errorf("stderr = %q masks a guard-seam error as not-found", se)
	}
	if after := readFileBytes(t, mainPath); string(after) != string(before) {
		t.Errorf("main file changed under the failed guard")
	}
}

// TestEditDeletedRefusalRUEN drives `edit` against a deleted-only number
// in both languages: the same refusal as set, the file byte-unchanged.
func TestEditDeletedRefusalRUEN(t *testing.T) {
	for _, lang := range []domain.Lang{domain.LangRU, domain.LangEN} {
		t.Run(string(lang), func(t *testing.T) {
			dir := resolvedTempDir(t)
			deletedPath := deleteTicketByPath(t, dir, string(lang), newTicketPath(t, dir, string(lang), "gone ticket"))
			before := readFileBytes(t, deletedPath)
			so, se, code := runDeleteCLI(t, dir, string(lang), "edit", "1", "--title", "new title")
			want := domain.ErrTicketDeletedReadOnly(lang, 1)
			assertDeletedRefusal(t, lang, "edit title", se, code, want, deletedPath, before)
			if so != "" {
				t.Errorf("stdout = %q, want empty", so)
			}
		})
	}
}

// TestEditDeletedDuplicateExit2 pins the guard ambiguity for edit too:
// main+deleted for one number is exit 2 with both paths, no writes.
func TestEditDeletedDuplicateExit2(t *testing.T) {
	dir := resolvedTempDir(t)
	mainPath := newTicketPath(t, dir, "ru", "dup ticket")
	deletedPath := duplicateIntoDeleted(t, dir, mainPath)
	mainBefore := readFileBytes(t, mainPath)
	deletedBefore := readFileBytes(t, deletedPath)
	_, se, code := runDeleteCLI(t, dir, "ru", "edit", "1", "--title", "x")
	if code != exitAmbiguous {
		t.Fatalf("edit 1: code=%d, want %d (stderr=%q)", code, exitAmbiguous, se)
	}
	for _, p := range []string{mainPath, deletedPath} {
		if !strings.Contains(se, p) {
			t.Errorf("stderr = %q, missing path %s", se, p)
		}
	}
	if after := readFileBytes(t, mainPath); string(after) != string(mainBefore) {
		t.Errorf("main file changed under the guard")
	}
	if after := readFileBytes(t, deletedPath); string(after) != string(deletedBefore) {
		t.Errorf("deleted file changed under the guard")
	}
}
