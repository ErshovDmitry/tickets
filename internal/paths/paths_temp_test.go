package paths

import (
	"os"
	"path/filepath"
	"testing"
)

// tempRootBoundaryOK reports whether os.TempDir() resolves to root. Only
// Clean is applied: both values originate from the same t.TempDir() string,
// and Windows GetTempPath appends a separator. Kept test-local so the pin
// also compiles — and genuinely FAILS on its assertion — against a pre-T-0127
// paths.go.
func tempRootBoundaryOK(root string) bool {
	host := filepath.Clean(os.TempDir())
	return host != "" && host == filepath.Clean(root)
}

// TestScanUpwardStopsAtTempRoot pins the T-0127 boundary: the upward scan
// must not resolve <temp-root>/tickets. Such a leftover directory is left by
// earlier smoke runs and hijacks resolution from any unrelated cwd, which
// made the CLI integration tests red on a clean HEAD.
func TestScanUpwardStopsAtTempRoot(t *testing.T) {
	root := t.TempDir() // controlled temp root, never the host one
	mkdirTemp(t, filepath.Join(root, "tickets", "archive"))
	foreign := mkdirTemp(t, filepath.Join(root, "foreign", "nested"))
	project := mkdirTemp(t, filepath.Join(root, "project", "sub"))
	mkdirTemp(t, filepath.Join(root, "project", "tickets"))

	// t.Setenv is process-wide, so this test must never be made parallel:
	// a parallel test would observe the controlled temp root mid-run.
	t.Setenv("TMPDIR", root)
	t.Setenv("TMP", root)
	t.Setenv("TEMP", root)
	if !tempRootBoundaryOK(root) {
		t.Fatalf("os.TempDir() = %q, want controlled root %q: TMPDIR/TMP/TEMP did not take effect", os.TempDir(), root)
	}

	if got, ok := scanUpward(foreign); ok || got != "" {
		t.Errorf("scanUpward(%q) = (%q, %v), want (\"\", false): the scan must stop before <temp-root>/tickets", foreign, got, ok)
	}

	// Positive guard: a project inside the temp root still self-locates, so
	// an over-fix that stops the walk at the whole temp subtree is caught.
	want := mustEvalSymlinks(t, filepath.Join(root, "project", "tickets"))
	if got, ok := scanUpward(project); !ok || got != want {
		t.Errorf("scanUpward(%q) = (%q, %v), want (%q, true)", project, got, ok, want)
	}
}
