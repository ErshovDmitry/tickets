package cli

// T-0189 W2-A1: argument-form, invalid-ref and ambiguity-resolution tests for
// ticket delete. Usage errors print the usage text and exit 1; a flag in the
// ref slot is the T-0068 ErrUnknownFlagList report; an invalid ref is localized;
// a duplicated number or an identical base name in several dirs is exit 2
// with every absolute path, before any prompt and without touching the tree.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
	"ticket/internal/store"
)

const ruUsageMarker = "ticket — тикеты проекта"

func TestDeleteUsageErrors(t *testing.T) {
	cases := [][]string{
		{"delete"},
		{"delete", "1", "--bogus"},
		{"delete", "1", "--yes=x"},
		{"delete", "1", "--force=y"},
		{"delete", "1", "--yes", "--yes"},
		{"delete", "1", "--force", "--force"},
		{"delete", "1", "2"},
		{"delete", "1", "--yes", "extra"},
		{"delete", "1", "-y"},
		{"delete", "1", "--force", "--yes", "--force"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir := resolvedTempDir(t)
			seedOpen(t, dir)
			before := treeState(t, dir)
			so, se, code := runDeleteCLI(t, dir, "ru", args...)
			if code != exitError {
				t.Fatalf("code=%d want %d (stderr=%q)", code, exitError, se)
			}
			if !strings.Contains(so, ruUsageMarker) {
				t.Errorf("usage error must print usage, stdout=%q", so)
			}
			assertSameTree(t, dir, before)
		})
	}
}

func TestDeleteRefSlotFlagIsUnknownFlag(t *testing.T) {
	for _, flag := range []string{"--yes", "--force", "-x", "--bogus"} {
		t.Run(flag, func(t *testing.T) {
			dir := resolvedTempDir(t)
			seedOpen(t, dir)
			before := treeState(t, dir)
			so, se, code := runDeleteCLI(t, dir, "ru", "delete", flag)
			if code != exitError {
				t.Fatalf("code=%d want %d", code, exitError)
			}
			if !strings.Contains(se, domain.ErrUnknownFlagList(domain.LangRU, flag)) {
				t.Errorf("stderr=%q, want ErrUnknownFlagList for %s", se, flag)
			}
			if strings.Contains(so, ruUsageMarker) {
				t.Errorf("ref-slot flag is not a usage error, stdout=%q", so)
			}
			assertSameTree(t, dir, before)
		})
	}
}

func TestDeleteInvalidRef(t *testing.T) {
	for _, ref := range []string{"abc", "../x", "a/b", "0", "10000", "12abc", "", "1.5", "+1", " 1", "99999"} {
		t.Run("ref="+ref, func(t *testing.T) {
			dir := resolvedTempDir(t)
			seedOpen(t, dir)
			before := treeState(t, dir)
			_, se, code := runDeleteCLI(t, dir, "ru", "delete", ref, "--yes")
			if code != exitError {
				t.Fatalf("ref %q: code=%d want %d (stderr=%q)", ref, code, exitError, se)
			}
			if !strings.Contains(se, domain.ErrDeleteInvalidRef(domain.LangRU, ref)) {
				t.Errorf("stderr=%q, want invalid-ref for %q", se, ref)
			}
			assertSameTree(t, dir, before)
		})
	}
}

func TestDeleteHelpInterception(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		t.Run(flag, func(t *testing.T) {
			dir := resolvedTempDir(t)
			so, se, code := runDeleteCLI(t, dir, "ru", "delete", flag)
			if code != exitOK || !strings.Contains(so, ruUsageMarker) || se != "" {
				t.Fatalf("delete %s: code=%d want 0 with usage on stdout (stderr=%q)", flag, code, se)
			}
		})
	}
}

func TestDeleteFlagOrderAndLeadingZeros(t *testing.T) {
	for _, args := range [][]string{
		{"delete", "1", "--yes", "--force"},
		{"delete", "1", "--force", "--yes"},
		{"delete", "0001", "--yes"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir := resolvedTempDir(t)
			seedOpen(t, dir)
			so, se, code := runDeleteCLI(t, dir, "ru", args...)
			if code != exitOK || so != deletedPath(dir, 1)+"\n" {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, so, se)
			}
		})
	}
}

// cloneIntoArchive copies the main ticket body under a second name in
// archive/, manufacturing a number/base-name clash without a second create.
func cloneIntoArchive(t *testing.T, dir, srcName, dstName string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, srcName))
	if err != nil {
		t.Fatalf("read %s: %v", srcName, err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "archive"), 0o755); err != nil {
		t.Fatalf("mkdir archive: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "archive", dstName), body, 0o644); err != nil {
		t.Fatalf("write archive/%s: %v", dstName, err)
	}
}

func TestDeleteDuplicateNumberBeforePrompt(t *testing.T) {
	dir := resolvedTempDir(t)
	seedOpen(t, dir)
	cloneIntoArchive(t, dir, "T-0001-open.md", "T-0001-done.md")
	before := treeState(t, dir)
	withRecoverSeams(t, "yes\n", true)
	so, se, code := runDeleteCLI(t, dir, "ru", "delete", "1")
	if code != exitAmbiguous {
		t.Fatalf("duplicate number: code=%d want %d (stderr=%q)", code, exitAmbiguous, se)
	}
	for _, p := range []string{filepath.Join(dir, "T-0001-open.md"), filepath.Join(dir, "archive", "T-0001-done.md")} {
		if !strings.Contains(se, p) {
			t.Errorf("ambiguity report must list %s, got %q", p, se)
		}
	}
	if strings.Contains(so, "yes/no") {
		t.Errorf("ambiguity must refuse before the prompt, stdout=%q", so)
	}
	assertSameTree(t, dir, before)
}

func TestDeleteExactBasenameSelectsOneDuplicate(t *testing.T) {
	t.Run("main basename", func(t *testing.T) {
		dir := resolvedTempDir(t)
		seedOpen(t, dir)
		cloneIntoArchive(t, dir, "T-0001-open.md", "T-0001-done.md")
		so, se, code := runDeleteCLI(t, dir, "ru", "delete", "T-0001-open.md", "--yes")
		if code != exitOK || so != deletedPath(dir, 1)+"\n" {
			t.Fatalf("exact main basename: code=%d stdout=%q stderr=%q", code, so, se)
		}
		if _, err := os.Stat(filepath.Join(dir, "archive", "T-0001-done.md")); err != nil {
			t.Errorf("archive twin must survive: %v", err)
		}
	})
	t.Run("archive basename with force", func(t *testing.T) {
		dir := resolvedTempDir(t)
		seedOpen(t, dir)
		cloneIntoArchive(t, dir, "T-0001-open.md", "T-0001-done.md")
		so, se, code := runDeleteCLI(t, dir, "ru", "delete", "T-0001-done.md", "--yes", "--force")
		if code != exitOK || so != deletedPath(dir, 1)+"\n" {
			t.Fatalf("exact archive basename: code=%d stdout=%q stderr=%q", code, so, se)
		}
		if _, err := os.Stat(filepath.Join(dir, "T-0001-open.md")); err != nil {
			t.Errorf("main twin must survive: %v", err)
		}
	})
}

func TestDeleteIdenticalBasenameAcrossDirs(t *testing.T) {
	dir := resolvedTempDir(t)
	seedOpen(t, dir)
	cloneIntoArchive(t, dir, "T-0001-open.md", "T-0001-open.md")
	before := treeState(t, dir)
	// A yes-answering TTY seam keeps the case hermetic: should the
	// pre-prompt ambiguity guard ever regress, the test must not fall
	// through to the real terminal and must still fail on the tree.
	withRecoverSeams(t, "yes\n", true)
	for _, ref := range []string{"1", "T-0001-open.md"} {
		so, se, code := runDeleteCLI(t, dir, "ru", "delete", ref, "--yes")
		if code != exitAmbiguous {
			t.Fatalf("ref %q: code=%d want %d (stderr=%q)", ref, code, exitAmbiguous, se)
		}
		if !strings.Contains(se, filepath.Join(dir, "archive", "T-0001-open.md")) {
			t.Errorf("ref %q: must list the archive twin, got %q", ref, se)
		}
		if strings.Contains(so, "yes/no") {
			t.Errorf("ref %q: must refuse before the prompt, stdout=%q", ref, so)
		}
	}
	assertSameTree(t, dir, before)
}

func TestDeleteBasenameNotFound(t *testing.T) {
	dir := resolvedTempDir(t)
	seedOpen(t, dir)
	_, se, code := runDeleteCLI(t, dir, "ru", "delete", "T-9999-open.md", "--yes")
	if code != exitError || !strings.Contains(se, domain.ErrTicketNotFound(domain.LangRU, "T-9999-open.md")) {
		t.Fatalf("absent basename: code=%d stderr=%q", code, se)
	}
}

// TestDeleteResolveDeleteAmbiguousTyped proves the pre-check renders the
// store's typed ambiguity (exit 2) rather than a generic error.
func TestDeleteResolveDeleteAmbiguousTyped(t *testing.T) {
	dir := resolvedTempDir(t)
	seedOpen(t, dir)
	cloneIntoArchive(t, dir, "T-0001-open.md", "T-0001-done.md")
	st, err := store.NewReadOnly(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if _, err := st.ResolveDelete("1"); err == nil {
		t.Fatal("ResolveDelete(1) must be ambiguous")
	} else if _, ok := err.(*store.AmbiguousNumberError); !ok {
		t.Fatalf("ResolveDelete error type = %T, want *store.AmbiguousNumberError", err)
	}
}
