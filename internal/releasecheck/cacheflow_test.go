package releasecheck

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestCheckCacheTTLBoundary pins the TTL boundary: 24h-ε is fresh (no
// contact), exactly 24h and 24h+ε are stale (contacted) — no sleeping,
// the stub clock provides the boundary (T-0173).
func TestCheckCacheTTLBoundary(t *testing.T) {
	cases := []struct {
		name string
		age  time.Duration
		want int
	}{
		{"fresh below TTL", cacheTTL - time.Second, 0},
		{"exactly TTL is stale", cacheTTL, 1},
		{"above TTL", cacheTTL + time.Second, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Responder echoes the current version: a stale entry must
			// re-contact and still compare equal.
			srv, h := newStubServer(t, okResponder(t, "1.0.0", "u"))
			clock := newStubClock()
			opts := hermeticOptions(t, srv, clock, "1.0.0")
			writeCacheEntry(t, opts.CachePath, cacheEntry{
				CheckedAt: clock.now().Add(-tc.age),
				OK:        true,
				Latest:    "1.0.0",
			})
			res := Check(opts)
			if res.State != UpToDate {
				t.Errorf("State=%v, want UpToDate", res.State)
			}
			if h.count() != tc.want {
				t.Errorf("requests = %d, want %d", h.count(), tc.want)
			}
		})
	}
}

// TestCheckCorruptCacheMiss proves corrupt/empty cache files are misses:
// the server is contacted and the cache rewritten with CheckedAt equal to
// the stubbed Now (T-0173).
func TestCheckCorruptCacheMiss(t *testing.T) {
	for _, content := range []string{"{not json", ""} {
		srv, h := newStubServer(t, okResponder(t, "1.2.3", "u"))
		clock := newStubClock()
		opts := hermeticOptions(t, srv, clock, "1.0.0")
		if err := os.WriteFile(opts.CachePath, []byte(content), 0o600); err != nil {
			t.Fatalf("seed corrupt cache: %v", err)
		}
		res := Check(opts)
		if res.State != UpdateAvailable {
			t.Errorf("content=%q: State=%v, want UpdateAvailable", content, res.State)
		}
		if h.count() != 1 {
			t.Errorf("content=%q: requests = %d, want 1", content, h.count())
		}
		entry := readCacheEntry(t, opts.CachePath)
		if !entry.CheckedAt.Equal(clock.now()) {
			t.Errorf("CheckedAt=%v, want stubbed now %v", entry.CheckedAt, clock.now())
		}
	}
}

// TestCheckSkewedCacheMiss proves a negative-age entry (clock skew) is a
// miss: contacted and rewritten with the stubbed Now (T-0173).
func TestCheckSkewedCacheMiss(t *testing.T) {
	srv, h := newStubServer(t, okResponder(t, "1.2.3", "u"))
	clock := newStubClock()
	opts := hermeticOptions(t, srv, clock, "1.0.0")
	writeCacheEntry(t, opts.CachePath, cacheEntry{
		CheckedAt: clock.now().Add(time.Hour),
		OK:        true,
		Latest:    "9.9.9",
	})
	res := Check(opts)
	if res.State != UpdateAvailable {
		t.Errorf("State=%v, want UpdateAvailable", res.State)
	}
	if h.count() != 1 {
		t.Errorf("requests = %d, want 1", h.count())
	}
	if got := readCacheEntry(t, opts.CachePath).CheckedAt; !got.Equal(clock.now()) {
		t.Errorf("CheckedAt=%v, want %v", got, clock.now())
	}
}

// TestCheckUnparseableCachedTagSelfHeals proves a fresh cache entry whose
// Latest tag no longer parses is a MISS: the server is re-contacted, the
// parseable answer is compared and re-cached (T-0173).
func TestCheckUnparseableCachedTagSelfHeals(t *testing.T) {
	srv, h := newStubServer(t, okResponder(t, "1.2.3", "u"))
	clock := newStubClock()
	opts := hermeticOptions(t, srv, clock, "1.0.0")
	writeCacheEntry(t, opts.CachePath, cacheEntry{
		CheckedAt: clock.now(),
		OK:        true,
		Latest:    "not-a-version",
	})
	res := Check(opts)
	if res.State != UpdateAvailable {
		t.Errorf("State=%v, want UpdateAvailable", res.State)
	}
	if h.count() != 1 {
		t.Errorf("requests = %d, want 1", h.count())
	}
	entry := readCacheEntry(t, opts.CachePath)
	if !entry.OK || entry.Latest != "1.2.3" {
		t.Errorf("rewritten entry = %+v, want OK with Latest 1.2.3", entry)
	}
}

// TestCheckUnparseableCachedTagStillBad proves that when the refetched tag
// is STILL unparseable the check fails for real and the bad value is not
// cached as OK (T-0173).
func TestCheckUnparseableCachedTagStillBad(t *testing.T) {
	srv, h := newStubServer(t, okResponder(t, "not-a-version", "u"))
	clock := newStubClock()
	opts := hermeticOptions(t, srv, clock, "1.0.0")
	writeCacheEntry(t, opts.CachePath, cacheEntry{
		CheckedAt: clock.now(),
		OK:        true,
		Latest:    "not-a-version",
	})
	before, err := os.ReadFile(opts.CachePath)
	if err != nil {
		t.Fatalf("read seeded cache: %v", err)
	}
	res := Check(opts)
	if res.State != Failed || res.Err == nil {
		t.Fatalf("State=%v err=%v, want Failed with non-nil Err", res.State, res.Err)
	}
	if h.count() != 1 {
		t.Errorf("requests = %d, want 1", h.count())
	}
	// The unparseable tag must not be cached as OK: no write at all, the
	// poisoned entry stays byte-identical.
	after, err := os.ReadFile(opts.CachePath)
	if err != nil {
		t.Fatalf("read cache after check: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("cache file changed: got %q, want the seeded entry untouched", after)
	}
}

// TestCheckFreshFailureRawError proves a fresh failure returns Failed
// with the RAW error (not the backoff sentinel) and caches the negative
// entry (T-0173).
func TestCheckFreshFailureRawError(t *testing.T) {
	srv, h := newStubServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	clock := newStubClock()
	opts := hermeticOptions(t, srv, clock, "1.0.0")
	res := Check(opts)
	if res.State != Failed || res.Err == nil {
		t.Fatalf("State=%v err=%v, want Failed with non-nil Err", res.State, res.Err)
	}
	if errors.Is(res.Err, errCachedFailure) {
		t.Errorf("fresh failure must carry the raw error, got sentinel %v", res.Err)
	}
	if !strings.Contains(res.Err.Error(), "500") {
		t.Errorf("Err=%q, want it to name HTTP 500", res.Err.Error())
	}
	if h.count() != 1 {
		t.Errorf("requests = %d, want 1", h.count())
	}
	entry := readCacheEntry(t, opts.CachePath)
	if entry.OK || !strings.Contains(entry.Reason, "500") {
		t.Errorf("negative entry = %+v, want OK=false with 500 reason", entry)
	}
}

// TestCheckBackoffBoundary pins the backoff boundary with the stub clock:
// 1h-ε suppresses contact (Failed wrapping errCachedFailure with the
// cached reason), exactly 1h and 1h+ε re-contact (T-0173).
func TestCheckBackoffBoundary(t *testing.T) {
	t.Run("inside backoff", func(t *testing.T) {
		srv, h := newStubServer(t, okResponder(t, "1.2.3", "u"))
		clock := newStubClock()
		opts := hermeticOptions(t, srv, clock, "1.0.0")
		writeCacheEntry(t, opts.CachePath, cacheEntry{
			CheckedAt: clock.now().Add(-backoffTTL + time.Second),
			OK:        false,
			Reason:    "cached boom",
		})
		res := Check(opts)
		if res.State != Failed {
			t.Fatalf("State=%v, want Failed", res.State)
		}
		if !errors.Is(res.Err, errCachedFailure) {
			t.Errorf("Err=%v, want errors.Is errCachedFailure", res.Err)
		}
		if !strings.Contains(res.Err.Error(), "cached boom") {
			t.Errorf("Err=%q, want cached reason", res.Err.Error())
		}
		if h.count() != 0 {
			t.Errorf("requests = %d, want 0", h.count())
		}
	})
	for _, age := range []time.Duration{backoffTTL, backoffTTL + time.Second} {
		t.Run(fmt.Sprintf("expired after %s", age), func(t *testing.T) {
			srv, h := newStubServer(t, okResponder(t, "1.2.3", "u"))
			clock := newStubClock()
			opts := hermeticOptions(t, srv, clock, "1.0.0")
			writeCacheEntry(t, opts.CachePath, cacheEntry{
				CheckedAt: clock.now().Add(-age),
				OK:        false,
				Reason:    "cached boom",
			})
			res := Check(opts)
			if res.State != UpdateAvailable {
				t.Errorf("State=%v, want UpdateAvailable", res.State)
			}
			if h.count() != 1 {
				t.Errorf("requests = %d, want 1", h.count())
			}
		})
	}
}

// TestCheckFreshFailureThenBackoff proves two sequential checks hit one
// request total: the first failure returns the raw error, the second is
// served from the negative entry via the sentinel (T-0173).
func TestCheckFreshFailureThenBackoff(t *testing.T) {
	srv, h := newStubServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	clock := newStubClock()
	opts := hermeticOptions(t, srv, clock, "1.0.0")
	if res := Check(opts); res.State != Failed || res.Err == nil {
		t.Fatalf("first: State=%v err=%v", res.State, res.Err)
	}
	res := Check(opts)
	if res.State != Failed || !errors.Is(res.Err, errCachedFailure) {
		t.Errorf("second: State=%v Err=%v, want Failed wrapping errCachedFailure", res.State, res.Err)
	}
	if h.count() != 1 {
		t.Errorf("requests = %d, want 1", h.count())
	}
}
