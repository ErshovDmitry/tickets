package releasecheck

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestCheckDevGate proves non-comparable current versions answer Dev with
// ZERO requests and ZERO cache access (T-0173 comparability gate).
func TestCheckDevGate(t *testing.T) {
	versions := []string{
		"dev", "", "garbage", "29111c6",
		"1.2.3-rc1", "1.2", "v1", "1.2.3+build",
		"1.2.4-37", "1.2.4-37-", "1.2.4-37-g", "1.2.4-37-gZZ", "1.2.4-abc-g123",
	}
	for _, v := range versions {
		t.Run("version="+v, func(t *testing.T) {
			srv, h := newStubServer(t, okResponder(t, "1.2.3", "u"))
			clock := newStubClock()
			opts := hermeticOptions(t, srv, clock, v)
			res := Check(opts)
			if res.State != Dev || res.Err != nil {
				t.Errorf("State=%v err=%v, want Dev without error", res.State, res.Err)
			}
			if h.count() != 0 {
				t.Errorf("requests = %d, want 0", h.count())
			}
			if _, err := os.Stat(opts.CachePath); !os.IsNotExist(err) {
				t.Errorf("cache file must not be touched, stat err = %v", err)
			}
		})
	}
}

// TestCheckZeroValueOptions proves a zero-value current version is
// treated safely (Dev, no panic). The options are hermetic by
// construction — stub server URL, its client, a temp cache path — so no
// real network or user-cache access is possible even if the Dev gate
// regresses and a request is attempted.
func TestCheckZeroValueOptions(t *testing.T) {
	srv, h := newStubServer(t, okResponder(t, "1.2.3", "u"))
	clock := newStubClock()
	opts := hermeticOptions(t, srv, clock, "")
	res := Check(opts)
	if res.State != Dev {
		t.Errorf("State=%v, want Dev", res.State)
	}
	if h.count() != 0 {
		t.Errorf("requests = %d, want 0", h.count())
	}
	if fi, err := os.Stat(opts.CachePath); !os.IsNotExist(err) {
		t.Errorf("cache file must not be touched, stat = %v, err = %v", fi, err)
	}
}

// TestCheckFreshCacheNoRequest proves an OK entry younger than cacheTTL
// answers without network (T-0173 boundary: 24h-ε is fresh).
func TestCheckFreshCacheNoRequest(t *testing.T) {
	srv, h := newStubServer(t, okResponder(t, "1.2.3", "u"))
	clock := newStubClock()
	opts := hermeticOptions(t, srv, clock, "1.0.0")
	writeCacheEntry(t, opts.CachePath, cacheEntry{
		CheckedAt: clock.now().Add(-cacheTTL + time.Second),
		OK:        true,
		Latest:    "1.2.3",
		URL:       "u",
	})
	res := Check(opts)
	if res.State != UpdateAvailable || res.Latest != "1.2.3" || res.URL != "u" {
		t.Errorf("got %+v, want UpdateAvailable{1.2.3,u}", res)
	}
	if h.count() != 0 {
		t.Errorf("requests = %d, want 0", h.count())
	}
}

// TestCheckTwoCallsOneRequest proves the cache is written on success and
// the second Check contacts nothing (T-0173).
func TestCheckTwoCallsOneRequest(t *testing.T) {
	srv, h := newStubServer(t, okResponder(t, "1.2.3", "u"))
	clock := newStubClock()
	opts := hermeticOptions(t, srv, clock, "1.0.0")
	if res := Check(opts); res.State != UpdateAvailable {
		t.Fatalf("first Check: State=%v", res.State)
	}
	if res := Check(opts); res.State != UpdateAvailable {
		t.Fatalf("second Check: State=%v", res.State)
	}
	if h.count() != 1 {
		t.Errorf("requests = %d, want 1", h.count())
	}
}

// TestCheck404NoCache proves 404 answers NoReleases and writes NO cache
// entry at all (T-0173).
func TestCheck404NoCache(t *testing.T) {
	srv, h := newStubServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	clock := newStubClock()
	opts := hermeticOptions(t, srv, clock, "1.0.0")
	res := Check(opts)
	if res.State != NoReleases || res.Err != nil {
		t.Errorf("State=%v err=%v, want NoReleases", res.State, res.Err)
	}
	if h.count() != 1 {
		t.Errorf("requests = %d, want 1", h.count())
	}
	if _, err := os.Stat(opts.CachePath); !os.IsNotExist(err) {
		t.Errorf("cache file must not exist after 404, stat err = %v", err)
	}
}

// TestCheckUnparseableTagNotCached proves a 200 with an unparseable tag
// answers Failed naming the tag and writes NO cache entry — it must not
// be cached as OK (T-0173 flow d).
func TestCheckUnparseableTagNotCached(t *testing.T) {
	srv, _ := newStubServer(t, okResponder(t, "release-1.2.3", "u"))
	clock := newStubClock()
	opts := hermeticOptions(t, srv, clock, "1.0.0")
	res := Check(opts)
	if res.State != Failed {
		t.Fatalf("State=%v, want Failed", res.State)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "release-1.2.3") {
		t.Errorf("Err=%v, want non-nil naming the tag", res.Err)
	}
	if _, err := os.Stat(opts.CachePath); !os.IsNotExist(err) {
		t.Errorf("cache file must not exist after unparseable tag, stat err = %v", err)
	}
}

// TestCheckCompareResults pins the compare outcome in both directions.
func TestCheckCompareResults(t *testing.T) {
	cases := []struct {
		name    string
		current string
		latest  string
		want    State
	}{
		{"update available", "1.0.0", "v1.2.10", UpdateAvailable},
		{"up to date", "2.0.0", "1.2.10", UpToDate},
		{"equal", "1.2.3", "v1.2.3", UpToDate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newStubServer(t, okResponder(t, tc.latest, "https://example.invalid/"+tc.latest))
			clock := newStubClock()
			res := Check(hermeticOptions(t, srv, clock, tc.current))
			if res.State != tc.want {
				t.Errorf("State=%v, want %v", res.State, tc.want)
			}
			if tc.want == UpdateAvailable && (res.Latest != tc.latest || res.URL == "") {
				t.Errorf("Latest=%q URL=%q, want tag %q and non-empty URL", res.Latest, res.URL, tc.latest)
			}
		})
	}
}

// TestCheckRequestHeaders proves the end-to-end request identity: the
// injected version rides in User-Agent with the pinned Accept and API
// version headers (T-0173).
func TestCheckRequestHeaders(t *testing.T) {
	srv, h := newStubServer(t, okResponder(t, "9.9.9", "u"))
	clock := newStubClock()
	res := Check(hermeticOptions(t, srv, clock, "3.4.5"))
	if res.State != UpdateAvailable || res.Latest != "9.9.9" {
		t.Fatalf("State=%v Latest=%q, want UpdateAvailable{9.9.9}", res.State, res.Latest)
	}
	hdr := h.lastHeader()
	if got := hdr.Get("User-Agent"); got != "ticket/3.4.5" {
		t.Errorf("User-Agent = %q, want %q", got, "ticket/3.4.5")
	}
	if got := hdr.Get("Accept"); got != "application/vnd.github+json" {
		t.Errorf("Accept = %q", got)
	}
	if got := hdr.Get("X-GitHub-Api-Version"); got != "2022-11-28" {
		t.Errorf("X-GitHub-Api-Version = %q", got)
	}
}

// TestCheckOfflineNoPanic proves end-to-end offline failures answer
// Failed without a panic (T-0173).
func TestCheckOfflineNoPanic(t *testing.T) {
	failing := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	})}
	srv, _ := newStubServer(t, okResponder(t, "1.2.3", "u")) // only for its URL shape
	clock := newStubClock()
	opts := hermeticOptions(t, srv, clock, "1.0.0")
	opts.HTTPClient = failing
	res := Check(opts)
	if res.State != Failed || res.Err == nil {
		t.Errorf("State=%v err=%v, want Failed with non-nil Err", res.State, res.Err)
	}
}

// TestCheckGitDescribeMakesRequest is the T-0173 root-verification
// regression: a current version shaped like real git-describe output
// (1.2.4-37-g29111c6, the AGENTS.md build) is comparable, so Check DOES
// contact the API exactly once and compares latest against the tagged
// base 1.2.4 — newer means UpdateAvailable, equal or older UpToDate.
func TestCheckGitDescribeMakesRequest(t *testing.T) {
	cases := []struct {
		name    string
		current string
		latest  string
		want    State
	}{
		{"newer release", "1.2.4-37-g29111c6", "1.2.5", UpdateAvailable},
		{"v-prefix newer release", "v1.2.4-37-g29111c6", "1.2.5", UpdateAvailable},
		{"equal to base", "1.2.4-37-g29111c6", "1.2.4", UpToDate},
		{"older than base", "1.2.4-37-g29111c6", "1.2.3", UpToDate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, h := newStubServer(t, okResponder(t, tc.latest, "https://example.invalid/"+tc.latest))
			clock := newStubClock()
			res := Check(hermeticOptions(t, srv, clock, tc.current))
			if res.State != tc.want {
				t.Errorf("State=%v, want %v", res.State, tc.want)
			}
			if h.count() != 1 {
				t.Errorf("requests = %d, want 1 (comparable git-describe must check the network)", h.count())
			}
			if tc.want == UpdateAvailable && (res.Latest != tc.latest || res.URL == "") {
				t.Errorf("Latest=%q URL=%q, want tag %q and non-empty URL", res.Latest, res.URL, tc.latest)
			}
		})
	}
}
