package cli

// T-0189 W2-A1: happy-path, confirmation and guard tests for ticket delete.
// Every test drives the real dispatch inside a t.TempDir() sandbox and reuses
// the recover sandbox helpers (resolvedTempDir, withRecoverSeams, treeState,
// assertSameTree). Argument-form and ambiguity cases live in
// cmd_delete_args_test.go; RU/EN snapshots, who CR/LF and read-only in
// cmd_delete_i18n_test.go.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// runDeleteCLI drives one command through the real dispatch in the given
// language and returns both streams and the exit code.
func runDeleteCLI(t *testing.T, dir, lang string, args ...string) (string, string, int) {
	t.Helper()
	var so, se bytes.Buffer
	env := map[string]string{"TICKETS_DIR": dir, "TICKET_LANG": lang, "USER": "tester"}
	code := Run(args, env, &so, &se)
	return so.String(), se.String(), code
}

// seedOpen creates one open main ticket and returns its absolute path.
func seedOpen(t *testing.T, dir string) string {
	t.Helper()
	so, se, code := runDeleteCLI(t, dir, "ru", "new", "seed ticket")
	if code != 0 {
		t.Fatalf("seed new: code=%d stderr=%q", code, se)
	}
	return strings.TrimSpace(so)
}

func deletedPath(dir string, n int) string {
	return filepath.Join(dir, "deleted", domain.Filename(n, domain.StatusDeleted))
}

func TestDeleteSuccessPrintsReturnedPath(t *testing.T) {
	dir := resolvedTempDir(t)
	openPath := seedOpen(t, dir)
	so, se, code := runDeleteCLI(t, dir, "ru", "delete", "1", "--yes")
	if code != exitOK {
		t.Fatalf("delete open: code=%d want %d (stderr=%q)", code, exitOK, se)
	}
	want := deletedPath(dir, 1)
	if so != want+"\n" {
		t.Errorf("stdout = %q, want returned deleted path %q", so, want)
	}
	if _, err := os.Stat(openPath); !os.IsNotExist(err) {
		t.Errorf("main file still present after delete: %v", err)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("deleted file missing: %v", err)
	}
}

func TestDeletePromptAnswers(t *testing.T) {
	yes := []string{"yes\n", "y\n", "YES\n", "Yes\n", "да\n", "д\n", "ДА\n", "  yes  \n"}
	no := []string{"no\n", "n\n", "\n", "maybe\n", "да нет\n", ""}
	for _, answer := range yes {
		t.Run("yes/"+strings.TrimSpace(answer), func(t *testing.T) {
			dir := resolvedTempDir(t)
			openPath := seedOpen(t, dir)
			withRecoverSeams(t, answer, true)
			so, se, code := runDeleteCLI(t, dir, "ru", "delete", "1")
			if code != exitOK {
				t.Fatalf("answer %q: code=%d want %d (stderr=%q)", answer, code, exitOK, se)
			}
			if !strings.Contains(so, openPath) || !strings.Contains(so, "yes/no") {
				t.Errorf("prompt must print the candidate path and ask, got %q", so)
			}
			if so != domain.DeletePrompt(domain.LangRU, openPath)+"\n"+domain.DeleteAsk(domain.LangRU)+deletedPath(dir, 1)+"\n" {
				t.Errorf("prompt+result layout unexpected: %q", so)
			}
		})
	}
	for _, answer := range no {
		t.Run("no/"+strings.TrimSpace(answer), func(t *testing.T) {
			dir := resolvedTempDir(t)
			seedOpen(t, dir)
			before := treeState(t, dir)
			withRecoverSeams(t, answer, true)
			so, se, code := runDeleteCLI(t, dir, "ru", "delete", "1")
			if code != exitError {
				t.Fatalf("answer %q: code=%d want %d (stderr=%q)", answer, code, exitError, se)
			}
			if !strings.Contains(se, domain.DeleteCancelled(domain.LangRU)) {
				t.Errorf("stderr = %q, want cancelled notice", se)
			}
			// A cancel prints the prompt but never the result path: nothing
			// was deleted, so no path may be announced as deleted.
			if strings.Contains(so, deletedPath(dir, 1)) {
				t.Errorf("answer %q: cancelled run must not print the deleted path, stdout=%q", answer, so)
			}
			assertSameTree(t, dir, before)
		})
	}
}

func TestDeleteNonTTYRefuses(t *testing.T) {
	dir := resolvedTempDir(t)
	seedOpen(t, dir)
	before := treeState(t, dir)
	// stdin would answer yes, but a non-terminal stdin must refuse first.
	withRecoverSeams(t, "yes\n", false)
	so, se, code := runDeleteCLI(t, dir, "ru", "delete", "1")
	if code != exitError {
		t.Fatalf("non-TTY: code=%d want %d (stderr=%q)", code, exitError, se)
	}
	if !strings.Contains(se, domain.DeleteRefused(domain.LangRU)) {
		t.Errorf("stderr = %q, want non-TTY refusal", se)
	}
	if strings.Contains(so, "yes/no") {
		t.Errorf("non-TTY must not print the prompt, got %q", so)
	}
	assertSameTree(t, dir, before)
}

func TestDeleteYesSkipsPromptButNotGuard(t *testing.T) {
	t.Run("open succeeds without TTY", func(t *testing.T) {
		dir := resolvedTempDir(t)
		seedOpen(t, dir)
		withRecoverSeams(t, "", false)
		so, _, code := runDeleteCLI(t, dir, "ru", "delete", "1", "--yes")
		if code != exitOK || strings.Contains(so, "yes/no") {
			t.Fatalf("--yes must skip the prompt and succeed: code=%d stdout=%q", code, so)
		}
	})
	t.Run("guard still refuses with --yes", func(t *testing.T) {
		dir := resolvedTempDir(t)
		seedOpen(t, dir)
		if _, se, code := runDeleteCLI(t, dir, "ru", "set", "1", "wip"); code != 0 {
			t.Fatalf("set wip: code=%d stderr=%q", code, se)
		}
		before := treeState(t, dir)
		withRecoverSeams(t, "", false)
		_, se, code := runDeleteCLI(t, dir, "ru", "delete", "1", "--yes")
		if code != exitError || !strings.Contains(se, domain.ErrDeleteOnlyOpen(domain.LangRU, "wip")) {
			t.Fatalf("--yes must not bypass the guard: code=%d stderr=%q", code, se)
		}
		assertSameTree(t, dir, before)
	})
}

func TestDeleteGuardsWithoutForce(t *testing.T) {
	for _, status := range []string{"wip", "done", "closed"} {
		t.Run(status, func(t *testing.T) {
			dir := resolvedTempDir(t)
			seedOpen(t, dir)
			if _, se, code := runDeleteCLI(t, dir, "ru", "set", "1", status); code != 0 {
				t.Fatalf("set %s: code=%d stderr=%q", status, code, se)
			}
			before := treeState(t, dir)
			withRecoverSeams(t, "yes\n", true)
			so, se, code := runDeleteCLI(t, dir, "ru", "delete", "1")
			if code != exitError {
				t.Fatalf("%s without force: code=%d want %d (stderr=%q)", status, code, exitError, se)
			}
			if !strings.Contains(se, domain.ErrDeleteOnlyOpen(domain.LangRU, status)) {
				t.Errorf("stderr = %q, want not-open guard naming %s", se, status)
			}
			if strings.Contains(so, "yes/no") {
				t.Errorf("guard must refuse before the prompt, got %q", so)
			}
			assertSameTree(t, dir, before)
		})
	}
}

func TestDeleteForceNonOpenAndArchive(t *testing.T) {
	t.Run("force main wip", func(t *testing.T) {
		dir := resolvedTempDir(t)
		seedOpen(t, dir)
		if _, se, code := runDeleteCLI(t, dir, "ru", "set", "1", "wip"); code != 0 {
			t.Fatalf("set wip: code=%d stderr=%q", code, se)
		}
		so, se, code := runDeleteCLI(t, dir, "ru", "delete", "1", "--yes", "--force")
		if code != exitOK || so != deletedPath(dir, 1)+"\n" {
			t.Fatalf("force wip: code=%d stdout=%q stderr=%q", code, so, se)
		}
	})
	t.Run("force archive", func(t *testing.T) {
		dir := resolvedTempDir(t)
		seedOpen(t, dir)
		for _, args := range [][]string{{"set", "1", "closed"}, {"archive", "1"}} {
			if _, se, code := runDeleteCLI(t, dir, "ru", args...); code != 0 {
				t.Fatalf("%v: code=%d stderr=%q", args, code, se)
			}
		}
		so, se, code := runDeleteCLI(t, dir, "ru", "delete", "1", "--yes", "--force")
		if code != exitOK || so != deletedPath(dir, 1)+"\n" {
			t.Fatalf("force archive: code=%d stdout=%q stderr=%q", code, so, se)
		}
		if _, err := os.Stat(filepath.Join(dir, "archive", "T-0001-closed.md")); !os.IsNotExist(err) {
			t.Errorf("archive file still present: %v", err)
		}
	})
	t.Run("archive without force refuses", func(t *testing.T) {
		dir := resolvedTempDir(t)
		seedOpen(t, dir)
		for _, args := range [][]string{{"set", "1", "closed"}, {"archive", "1"}} {
			if _, se, code := runDeleteCLI(t, dir, "ru", args...); code != 0 {
				t.Fatalf("%v: code=%d stderr=%q", args, code, se)
			}
		}
		before := treeState(t, dir)
		_, se, code := runDeleteCLI(t, dir, "ru", "delete", "1", "--yes")
		if code != exitError || !strings.Contains(se, domain.ErrDeleteOnlyOpen(domain.LangRU, "closed")) {
			t.Fatalf("archive without force: code=%d stderr=%q", code, se)
		}
		assertSameTree(t, dir, before)
	})
}

func TestDeleteRepeatNotIdempotent(t *testing.T) {
	dir := resolvedTempDir(t)
	seedOpen(t, dir)
	if _, se, code := runDeleteCLI(t, dir, "ru", "delete", "1", "--yes"); code != 0 {
		t.Fatalf("first delete: code=%d stderr=%q", code, se)
	}
	before := treeState(t, dir)
	so, se, code := runDeleteCLI(t, dir, "ru", "delete", "1", "--yes")
	if code != exitError {
		t.Fatalf("repeat: code=%d want %d (not success-idempotent)", code, exitError)
	}
	if !strings.Contains(se, domain.ErrDeleteAlreadyDeleted(domain.LangRU)) {
		t.Errorf("repeat stderr = %q, want already-deleted", se)
	}
	if so != "" {
		t.Errorf("repeat must print no path, got %q", so)
	}
	assertSameTree(t, dir, before)
}
