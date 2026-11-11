package releasecheck

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestCacheRoundTrip proves saveCache writes what loadCache reads (T-0173).
func TestCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "release-check.json")
	entry := cacheEntry{
		CheckedAt: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC),
		OK:        true,
		Latest:    "1.2.3",
		URL:       "https://example.invalid/releases/1.2.3",
	}
	if err := saveCache(path, entry); err != nil {
		t.Fatalf("saveCache: %v", err)
	}
	got, err := loadCache(path)
	if err != nil {
		t.Fatalf("loadCache: %v", err)
	}
	if got != entry {
		t.Errorf("round trip: got %+v, want %+v", got, entry)
	}
	assertNoTempFiles(t, filepath.Dir(path))
}

// TestSaveCacheMode checks the cache file mode (T-0173: 0600); skipped on
// Windows, where POSIX permission bits are not enforced.
func TestSaveCacheMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits not enforced on Windows")
	}
	path := filepath.Join(t.TempDir(), "release-check.json")
	if err := saveCache(path, cacheEntry{CheckedAt: time.Now(), OK: true}); err != nil {
		t.Fatalf("saveCache: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat cache: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("cache mode = %o, want 600", info.Mode().Perm())
	}
}

// TestSaveCacheRenameFailure injects a rename failure (the target path is
// a non-empty directory) and proves the error surfaces and no .tmp file
// leaks (T-0173).
func TestSaveCacheRenameFailure(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "release-check.json")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "occupied"), []byte("x"), 0o600); err != nil {
		t.Fatalf("occupy target dir: %v", err)
	}
	err := saveCache(target, cacheEntry{CheckedAt: time.Now(), OK: true})
	if err == nil {
		t.Fatal("saveCache: want error, got nil")
	}
	assertNoTempFiles(t, root)
}

// TestSaveCacheCreateTempFailure injects a temp-create failure via a
// read-only cache directory and proves the error surfaces with no .tmp
// leak (T-0173). Skipped on Windows (chmod read-only is not enforced).
func TestSaveCacheCreateTempFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits not enforced on Windows")
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatalf("chmod read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
	path := filepath.Join(root, "release-check.json")
	if err := saveCache(path, cacheEntry{CheckedAt: time.Now(), OK: true}); err == nil {
		t.Fatal("saveCache: want error, got nil")
	}
	assertNoTempFiles(t, root)
}

// TestLoadCacheMisses proves absent, corrupt, empty and mistyped cache
// files are all misses — never fatal (T-0173).
func TestLoadCacheMisses(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name    string
		content *string
	}{
		{"absent", nil},
		{"empty", strPtr("")},
		{"corrupt", strPtr("{not json")},
		{"wrong types", strPtr("{\"checked_at\":\"soon\",\"ok\":\"yes\"}")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, tc.name+".json")
			if tc.content != nil {
				if err := os.WriteFile(path, []byte(*tc.content), 0o600); err != nil {
					t.Fatalf("seed cache: %v", err)
				}
			}
			if _, err := loadCache(path); err == nil {
				t.Error("loadCache: want miss error, got nil")
			}
		})
	}
}

// assertNoTempFiles fails the test when dir contains leftover *.tmp files
// (T-0173: temp removed on every saveCache path).
func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("leftover temp files: %v", matches)
	}
}

func strPtr(s string) *string { return &s }
