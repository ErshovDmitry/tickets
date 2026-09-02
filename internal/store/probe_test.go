package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNew_PreservesExistingWriteProbe pins the T-0007 fix: the
// writeability probe must create a UNIQUE O_EXCL file. The old probe
// opened the fixed name .write-probe (O_CREATE without O_EXCL) and then
// removed it — silently deleting a pre-existing user file of that name.
func TestNew_PreservesExistingWriteProbe(t *testing.T) {
	dir := t.TempDir()
	probe := filepath.Join(dir, ".write-probe")
	sentinel := []byte("user-data-do-not-delete")
	if err := os.WriteFile(probe, sentinel, 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s == nil || s.Dir != dir {
		t.Fatalf("New returned %+v, want Store in %s", s, dir)
	}

	got, err := os.ReadFile(probe)
	if err != nil {
		t.Fatalf("pre-existing .write-probe deleted by New: %v", err)
	}
	if string(got) != string(sentinel) {
		t.Errorf(".write-probe contents = %q, want %q", got, sentinel)
	}

	// No probe artefacts may be left behind (unique names included).
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != ".write-probe" && strings.HasPrefix(e.Name(), ".write-probe") {
			t.Errorf("leftover probe file %q after New", e.Name())
		}
	}
}
