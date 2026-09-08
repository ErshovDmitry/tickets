//go:build unix

package paths

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestIsRealDirSymlinks covers the Lstat (never follow the trailing link)
// part of the IsRealDir contract (T-0082): a symlink to a directory and a
// broken symlink both report ErrNotRealDir.
func TestIsRealDirSymlinks(t *testing.T) {
	root := t.TempDir()
	real := mkdirTemp(t, filepath.Join(root, "real"))

	toDir := filepath.Join(root, "to-dir")
	if err := os.Symlink(real, toDir); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	broken := filepath.Join(root, "broken")
	if err := os.Symlink(filepath.Join(root, "no-such-target"), broken); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	for _, path := range []string{toDir, broken} {
		got, err := IsRealDir(path)
		if got {
			t.Errorf("IsRealDir(%q) = true, want false", path)
		}
		if !errors.Is(err, ErrNotRealDir) {
			t.Errorf("IsRealDir(%q) err = %v, want errors.Is ErrNotRealDir", path, err)
		}
	}
}
