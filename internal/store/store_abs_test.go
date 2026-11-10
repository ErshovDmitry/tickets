package store

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNewAbsDirFromRelativePath pins the single test that protects the
// T-0157 decision that New and NewReadOnly make Store.Dir absolute: every
// path the store derives from it — including the conflict paths an
// AmbiguousNumberError reports — must be absolute regardless of the caller's
// working directory. Removing the filepath.Abs step fails here.
func TestNewAbsDirFromRelativePath(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.Mkdir("rel", 0o755); err != nil {
		t.Fatalf("mkdir rel: %v", err)
	}

	s, err := New("rel")
	if err != nil {
		t.Fatalf("New(rel): %v", err)
	}
	if !filepath.IsAbs(s.Dir) {
		t.Errorf("New: Dir = %q, want an absolute path", s.Dir)
	}

	ro, err := NewReadOnly("rel")
	if err != nil {
		t.Fatalf("NewReadOnly(rel): %v", err)
	}
	if !filepath.IsAbs(ro.Dir) {
		t.Errorf("NewReadOnly: Dir = %q, want an absolute path", ro.Dir)
	}
}
