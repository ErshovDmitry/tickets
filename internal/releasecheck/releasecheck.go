package releasecheck

import (
	"errors"
	"fmt"
	"time"
)

// State is the machine-readable outcome of the T-0173 release check.
type State int

// T-0173 release check states.
const (
	// UpToDate means the current version is at least the latest release.
	UpToDate State = iota

	// UpdateAvailable means a newer release exists; Result carries its
	// tag and page URL.
	UpdateAvailable

	// NoReleases means the repository has no releases at all (HTTP 404).
	NoReleases

	// Dev means the current version is not comparable (unknown dev
	// build, bare hash, malformed version, prerelease, build metadata);
	// no network or cache access happened.
	Dev

	// Failed means the check could not complete; Result.Err names the
	// cause (never nil for this state).
	Failed
)

// Result is the T-0173 outcome of a release check.
type Result struct {
	// State is the machine-readable outcome (T-0173).
	State State

	// Latest is the newest release tag; set only for UpdateAvailable (T-0173).
	Latest string

	// URL is the release page; set only for UpdateAvailable (T-0173).
	URL string

	// Err names the failure cause; set only for Failed and always
	// non-nil there (T-0173).
	Err error
}

// cacheTTL is how long a successful check stays fresh (T-0173).
const cacheTTL = 24 * time.Hour

// backoffTTL is how long a failed check suppresses retries (T-0173).
const backoffTTL = time.Hour

// errCachedFailure is the T-0173 backoff sentinel: a Failed result caused
// by a cached recent failure wraps it together with the cached reason.
var errCachedFailure = errors.New("cached failed check")

// Check runs the T-0173 release check. The current version passes the
// comparability gate first (plain X.Y.Z, one optional leading "v", or a
// git-describe suffix "[v]X.Y.Z-<distance>-g<hash>" compared as its base
// X.Y.Z); anything else returns State Dev with zero network and zero
// cache access. A fresh cached success answers without network; a cached
// failure younger than backoffTTL returns Failed wrapping
// errCachedFailure. Otherwise the GitHub API is contacted: a parsed tag
// is compared and cached as OK (parse-before-save), 404 answers
// NoReleases without a cache write, and every other failure is cached as
// a negative entry and returned raw.
func Check(opts Options) Result {
	opts = opts.withDefaults()

	cur, ok := comparable(opts.CurrentVersion)
	if !ok {
		return Result{State: Dev}
	}

	var entry cacheEntry
	haveCache := false
	if opts.CachePath != "" {
		// Any load/parse problem is a miss, never fatal (T-0173); a
		// zero CheckedAt (empty JSON object) is a miss too.
		if e, err := loadCache(opts.CachePath); err == nil && !e.CheckedAt.IsZero() {
			entry, haveCache = e, true
		}
	}
	if haveCache {
		age := opts.Now().Sub(entry.CheckedAt)
		switch {
		case age < 0:
			// Clock skew: treat as a miss, re-contact and rewrite.
		case entry.OK && age < cacheTTL:
			if _, ok := parseVersion(entry.Latest); ok {
				return compareCached(entry, cur)
			}
			// An unparseable tag in a fresh entry is a cache miss: refetch
			// so a poisoned cache self-heals instead of failing for the
			// whole TTL (T-0173).
		case !entry.OK && age < backoffTTL:
			return Result{State: Failed, Err: fmt.Errorf("%w: %s", errCachedFailure, entry.Reason)}
		}
	}

	tag, html, err := fetchLatest(opts.HTTPClient, opts.BaseURL, opts.CurrentVersion)
	switch {
	case err == nil:
		res, ok := compareResult(tag, html, cur)
		if !ok {
			// T-0173 flow (d): an unparseable tag must NOT be cached
			// as OK, so this path writes nothing.
			return Result{State: Failed, Err: fmt.Errorf("releasecheck: latest release tag %q is not a comparable version", tag)}
		}
		if opts.CachePath != "" {
			// Best-effort only: a cache error never changes Result.
			_ = saveCache(opts.CachePath, cacheEntry{CheckedAt: opts.Now(), OK: true, Latest: tag, URL: html})
		}
		return res
	case errors.Is(err, errNoReleases):
		// T-0173: no cache write by design — one request per
		// invocation until a release exists.
		return Result{State: NoReleases}
	default:
		if opts.CachePath != "" {
			_ = saveCache(opts.CachePath, cacheEntry{CheckedAt: opts.Now(), OK: false, Reason: err.Error()})
		}
		return Result{State: Failed, Err: err}
	}
}

// compareCached answers from a fresh cached OK entry without network
// (T-0173). Entries are only written after a successful parse, so the
// unparsable branch is defensive.
func compareCached(e cacheEntry, cur semver) Result {
	res, ok := compareResult(e.Latest, e.URL, cur)
	if !ok {
		return Result{State: Failed, Err: fmt.Errorf("releasecheck: cached latest %q is not a comparable version", e.Latest)}
	}
	return res
}

// compareResult compares the latest tag against the current version;
// ok is false when the tag is unparsable (T-0173).
func compareResult(latest, htmlURL string, cur semver) (Result, bool) {
	lv, ok := parseVersion(latest)
	if !ok {
		return Result{}, false
	}
	if compareSemver(lv, cur) <= 0 {
		return Result{State: UpToDate}, true
	}
	return Result{State: UpdateAvailable, Latest: latest, URL: htmlURL}, true
}
