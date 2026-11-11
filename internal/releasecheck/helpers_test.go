package releasecheck

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// stubHandler is an httptest handler that counts requests and captures
// the last request headers, then delegates the response to respond.
type stubHandler struct {
	mu       sync.Mutex
	requests int
	header   http.Header
	respond  func(w http.ResponseWriter, r *http.Request)
}

func (h *stubHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	h.requests++
	h.header = r.Header.Clone()
	h.mu.Unlock()
	h.respond(w, r)
}

func (h *stubHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.requests
}

func (h *stubHandler) lastHeader() http.Header {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.header
}

// newStubServer starts an httptest server with the given responder;
// the server is closed on test cleanup.
func newStubServer(t *testing.T, respond func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *stubHandler) {
	t.Helper()
	h := &stubHandler{respond: respond}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, h
}

// stubClock is an injectable time source for TTL/backoff boundaries —
// no sleeping anywhere (T-0173 test matrix).
type stubClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *stubClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// newStubClock returns a clock pinned at a fixed UTC instant.
func newStubClock() *stubClock {
	return &stubClock{t: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
}

// hermeticOptions builds fully injected Options: stub server, temp-dir
// cache, stub clock (T-0173: hermetic only, never the real user cache
// dir or network).
func hermeticOptions(t *testing.T, srv *httptest.Server, clock *stubClock, version string) Options {
	t.Helper()
	return Options{
		BaseURL:        srv.URL,
		HTTPClient:     srv.Client(),
		CachePath:      filepath.Join(t.TempDir(), "release-check.json"),
		Now:            clock.now,
		CurrentVersion: version,
	}
}

// writeCacheEntry seeds the cache file at path with a marshalled entry.
func writeCacheEntry(t *testing.T, path string, e cacheEntry) {
	t.Helper()
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal cache entry: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
}

// readCacheEntry loads the cache file at path, failing the test on error.
func readCacheEntry(t *testing.T, path string) cacheEntry {
	t.Helper()
	e, err := loadCache(path)
	if err != nil {
		t.Fatalf("load cache file: %v", err)
	}
	return e
}

// latestPayload is a GitHub /releases/latest response body.
func latestPayload(t *testing.T, tag, url string) string {
	t.Helper()
	data, err := json.Marshal(map[string]string{"tag_name": tag, "html_url": url})
	if err != nil {
		t.Fatalf("marshal latest payload: %v", err)
	}
	return string(data)
}

// okResponder answers 200 with the given tag/url pair.
func okResponder(t *testing.T, tag, url string) func(w http.ResponseWriter, r *http.Request) {
	t.Helper()
	body := latestPayload(t, tag, url)
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	}
}
