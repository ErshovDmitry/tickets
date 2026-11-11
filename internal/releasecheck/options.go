// Package releasecheck implements the T-0173 explicit release check: it
// queries the GitHub API for the latest tickets release, compares it
// against the running version and caches the outcome per user. The
// package is UI-agnostic (no CLI/domain imports) and stdlib-only.
package releasecheck

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// defaultBaseURL is the single source of the repository identity for the
// T-0173 release check: the GitHub API endpoint of the tickets repo. No
// other file in the repository carries this literal.
const defaultBaseURL = "https://api.github.com/repos/ErshovDmitry/tickets"

// defaultHTTPTimeout bounds a startup-adjacent release check (T-0173).
const defaultHTTPTimeout = 10 * time.Second

// defaultCachePath is the single owner of the per-user cache file shape
// (<dir>/ticket/release-check.json); DefaultOptions and withDefaults both
// build the path through it (T-0173).
func defaultCachePath(dir string) string {
	return filepath.Join(dir, "ticket", "release-check.json")
}

// Options carries every seam of the T-0173 release check so tests stay
// hermetic; Check fills nil/empty fields with production defaults, so a
// zero Options cannot panic.
type Options struct {
	// BaseURL is the GitHub API base of the repository; empty means
	// defaultBaseURL (T-0173).
	BaseURL string

	// HTTPClient performs the API request; nil means a client with a
	// 10-second timeout (T-0173).
	HTTPClient *http.Client

	// CachePath is the per-user cache file (<dir>/ticket/release-check.json);
	// an empty CachePath is backfilled from UserCacheDirFn by withDefaults
	// and, when the cache dir is unresolvable, stays empty so Check performs
	// no cache I/O (T-0173).
	CachePath string

	// UserCacheDirFn resolves the per-user cache directory (default
	// os.UserCacheDir); used by DefaultOptions and by the withDefaults
	// CachePath backfill (T-0173).
	UserCacheDirFn func() (string, error)

	// Now is the single time source for TTL/backoff decisions and cache
	// timestamps; nil means time.Now().UTC (T-0173).
	Now func() time.Time

	// CurrentVersion is the running ticket version injected by the CLI;
	// a plain X.Y.Z (one optional leading "v") or a git-describe suffix
	// "[v]X.Y.Z-<distance>-g<hash>" (compared as its base X.Y.Z) is
	// comparable (T-0173).
	CurrentVersion string
}

// DefaultOptions builds the production Options for T-0173: the pinned
// endpoint, a 10-second HTTP client, the per-user cache file and the wall
// clock. The only possible error comes from resolving the user cache dir.
func DefaultOptions(currentVersion string) (Options, error) {
	opts := Options{
		BaseURL:        defaultBaseURL,
		HTTPClient:     &http.Client{Timeout: defaultHTTPTimeout},
		UserCacheDirFn: os.UserCacheDir,
		Now:            func() time.Time { return time.Now().UTC() },
		CurrentVersion: currentVersion,
	}
	dir, err := opts.UserCacheDirFn()
	if err != nil {
		return Options{}, fmt.Errorf("releasecheck: user cache dir: %w", err)
	}
	opts.CachePath = defaultCachePath(dir)
	return opts, nil
}

// withDefaults returns a copy of o with every nil/empty seam filled with
// the production default (T-0173). A CachePath that stays empty (cache
// dir unresolvable) disables cache access; Check never panics on it.
func (o Options) withDefaults() Options {
	if o.BaseURL == "" {
		o.BaseURL = defaultBaseURL
	}
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	if o.UserCacheDirFn == nil {
		o.UserCacheDirFn = os.UserCacheDir
	}
	if o.Now == nil {
		o.Now = func() time.Time { return time.Now().UTC() }
	}
	if o.CachePath == "" {
		if dir, err := o.UserCacheDirFn(); err == nil {
			o.CachePath = defaultCachePath(dir)
		}
	}
	return o
}
