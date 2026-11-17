package cli

// T-0189 W3: normalize CLI coverage for the deleted walk. The store-level
// contract (in-place rewrite, name/status/journal preserved, dry-run
// no-write, warning parity) is pinned in internal/store (C1a); here we
// check the command surface: stdout lists the rewritten paths in the
// main → archive → deleted walk order, --dry-run prints the same paths
// and writes nothing, and a broken file inside deleted/ warns on stderr
// without changing the exit code.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// delint rewrites the ticket at path into the pre-fix shape (blank lines
// around "## " headers dropped) so normalize has something to restore.
func delint(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	legacy := strings.ReplaceAll(string(body), "\n\n## ", "\n## ")
	legacy = strings.ReplaceAll(legacy, "\n\n\n", "\n\n")
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatalf("delint %s: %v", path, err)
	}
	return []byte(legacy)
}

// seedThreeDirs builds one legacy-shaped ticket in each of main,
// archive (done) and deleted/, and returns their paths in walk order.
// Tickets are numbered by creation order: 1 stays main, 2 is archived,
// 3 is deleted (set/archive/delete rename the file, so every path is
// recomputed from the final name, never from the new-command stdout).
func seedThreeDirs(t *testing.T, dir string) (mainPath, archivePath, deletedPath string) {
	t.Helper()
	newTicketPath(t, dir, "ru", "main ticket")
	mainPath = filepath.Join(dir, "T-0001-open.md")
	newTicketPath(t, dir, "ru", "archive ticket")
	if _, se, code := runDeleteCLI(t, dir, "ru", "set", "2", "done"); code != 0 {
		t.Fatalf("set 2 done: code=%d stderr=%q", code, se)
	}
	if so, se, code := runDeleteCLI(t, dir, "ru", "archive", "2"); code != 0 {
		t.Fatalf("archive 2: code=%d stdout=%q stderr=%q", code, so, se)
	}
	archivePath = filepath.Join(dir, "archive", "T-0002-done.md")
	deletedPath = deleteTicketByPath(t, dir, "ru", newTicketPath(t, dir, "ru", "gone ticket"))
	delint(t, mainPath)
	delint(t, archivePath)
	delint(t, deletedPath)
	return mainPath, archivePath, deletedPath
}

// TestNormalizeDeleted_WalkOrderStdout pins the CLI surface of the C1a
// walk: changed paths print on stdout in main → archive → deleted order.
func TestNormalizeDeleted_WalkOrderStdout(t *testing.T) {
	dir := resolvedTempDir(t)
	mainPath, archivePath, deletedPath := seedThreeDirs(t, dir)

	so, se, code := runDeleteCLI(t, dir, "ru", "normalize", "--dry-run")
	if code != 0 {
		t.Fatalf("normalize --dry-run: code=%d stderr=%q", code, se)
	}
	assertWalkOrder(t, so, mainPath, archivePath, deletedPath)

	// Dry-run writes nothing: every file is still in the legacy shape.
	for _, p := range []string{mainPath, archivePath, deletedPath} {
		if got := readFileBytes(t, p); !strings.Contains(string(got), "\n## ") {
			t.Errorf("%s was rewritten by --dry-run", p)
		}
	}

	// The real run rewrites all three in place (same paths, same order).
	so, se, code = runDeleteCLI(t, dir, "ru", "normalize")
	if code != 0 {
		t.Fatalf("normalize: code=%d stderr=%q", code, se)
	}
	assertWalkOrder(t, so, mainPath, archivePath, deletedPath)
	for _, p := range []string{mainPath, archivePath, deletedPath} {
		if got := readFileBytes(t, p); strings.Contains(string(got), "\n## ") && !strings.Contains(string(got), "\n\n## ") {
			t.Errorf("%s still in the legacy shape after normalize", p)
		}
	}
}

// assertWalkOrder fails unless stdout lists the three paths in the
// main → archive → deleted order, one per line.
func assertWalkOrder(t *testing.T, stdout string, paths ...string) {
	t.Helper()
	pos := -1
	for _, p := range paths {
		i := strings.Index(stdout, p)
		if i < 0 {
			t.Fatalf("stdout = %q, missing path %s", stdout, p)
		}
		if i <= pos {
			t.Fatalf("stdout = %q, path %s out of walk order", stdout, p)
		}
		pos = i
	}
}

// TestNormalizeDeleted_WarningParity pins archive parity for a broken
// file inside deleted/: the warning lands on stderr ("ticket: ..."), the
// exit stays 0 and the fixable tickets still rewrite.
func TestNormalizeDeleted_WarningParity(t *testing.T) {
	dir := resolvedTempDir(t)
	mainPath, _, deletedPath := seedThreeDirs(t, dir)
	deletedDir := filepath.Dir(deletedPath)
	garbage := filepath.Join(deletedDir, "garbage.md")
	if err := os.WriteFile(garbage, []byte("x"), 0o644); err != nil {
		t.Fatalf("write garbage: %v", err)
	}

	so, se, code := runDeleteCLI(t, dir, "ru", "normalize", "--dry-run")
	if code != 0 {
		t.Fatalf("normalize --dry-run: code=%d stderr=%q", code, se)
	}
	if !strings.Contains(se, "garbage.md") {
		t.Errorf("stderr = %q, want a warning naming garbage.md", se)
	}
	if !strings.Contains(so, mainPath) {
		t.Errorf("stdout = %q, want the fixable main path still listed", so)
	}
}
