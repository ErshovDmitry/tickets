package releasecheck

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestDefaultOptions proves the production defaults and the cache path
// shape (T-0173). The UserCacheDirFn seam itself is exercised via
// withDefaults in TestWithDefaultsFailingCacheDir — env tricks are
// unforceable on Windows, so the seam is stubbed instead.
func TestDefaultOptions(t *testing.T) {
	opts, err := DefaultOptions("1.2.3")
	if err != nil {
		t.Fatalf("DefaultOptions: %v", err)
	}
	if opts.BaseURL != defaultBaseURL {
		t.Errorf("BaseURL = %q", opts.BaseURL)
	}
	if opts.CachePath != filepath.Join(mustUserCacheDir(t), "ticket", "release-check.json") {
		t.Errorf("CachePath = %q", opts.CachePath)
	}
	if opts.HTTPClient == nil || opts.HTTPClient.Timeout != defaultHTTPTimeout {
		t.Errorf("HTTPClient = %+v, want 10s timeout", opts.HTTPClient)
	}
	if opts.Now == nil || opts.CurrentVersion != "1.2.3" {
		t.Errorf("Now set=%t CurrentVersion=%q", opts.Now != nil, opts.CurrentVersion)
	}
}

func mustUserCacheDir(t *testing.T) string {
	t.Helper()
	dir, err := os.UserCacheDir()
	if err != nil {
		t.Skipf("no user cache dir in this environment: %v", err)
	}
	return dir
}

// TestWithDefaultsBackfillsCachePath pins the empty-CachePath semantics
// chosen for T-0173: withDefaults backfills it from UserCacheDirFn via the
// single defaultCachePath owner.
func TestWithDefaultsBackfillsCachePath(t *testing.T) {
	dir := t.TempDir()
	opts := Options{UserCacheDirFn: func() (string, error) {
		return dir, nil
	}}.withDefaults()
	if want := defaultCachePath(dir); opts.CachePath != want {
		t.Errorf("CachePath = %q, want %q", opts.CachePath, want)
	}
}

// TestWithDefaultsFailingCacheDir proves the UserCacheDirFn seam error is
// non-fatal: withDefaults leaves CachePath empty (caching disabled) while
// the other defaults fill in (T-0173).
func TestWithDefaultsFailingCacheDir(t *testing.T) {
	opts := Options{UserCacheDirFn: func() (string, error) {
		return "", errors.New("no cache dir")
	}}.withDefaults()
	if opts.CachePath != "" {
		t.Errorf("CachePath = %q, want empty when the cache dir is unresolvable", opts.CachePath)
	}
	if opts.BaseURL != defaultBaseURL || opts.HTTPClient == nil || opts.Now == nil {
		t.Errorf("defaults not filled: %+v", opts)
	}
}
