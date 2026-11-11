package releasecheck

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// roundTripperFunc adapts a function to http.RoundTripper.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestFetchLatestOK proves 200 decodes tag and page URL, and that the
// version parameter is USED: the request carries it in User-Agent
// (T-0173).
func TestFetchLatestOK(t *testing.T) {
	srv, h := newStubServer(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, latestPayload(t, "v1.2.3", "https://example.invalid/r/1.2.3"))
	})
	tag, html, err := fetchLatest(srv.Client(), srv.URL, "1.0.0")
	if err != nil {
		t.Fatalf("fetchLatest: %v", err)
	}
	if tag != "v1.2.3" || html != "https://example.invalid/r/1.2.3" {
		t.Errorf("got tag=%q html=%q", tag, html)
	}
	hdr := h.lastHeader()
	if got := hdr.Get("User-Agent"); got != "ticket/1.0.0" {
		t.Errorf("User-Agent = %q, want %q", got, "ticket/1.0.0")
	}
	if got := hdr.Get("Accept"); got != "application/vnd.github+json" {
		t.Errorf("Accept = %q", got)
	}
	if got := hdr.Get("X-GitHub-Api-Version"); got != "2022-11-28" {
		t.Errorf("X-GitHub-Api-Version = %q", got)
	}
}

// TestFetchLatestNotFound proves 404 answers errNoReleases (T-0173).
func TestFetchLatestNotFound(t *testing.T) {
	srv, _ := newStubServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	_, _, err := fetchLatest(srv.Client(), srv.URL, "1.0.0")
	if !errors.Is(err, errNoReleases) {
		t.Errorf("err = %v, want errNoReleases", err)
	}
}

// TestFetchLatestRateLimit proves 403/429 name the rate limit and carry
// Retry-After / X-RateLimit-Remaining when present (T-0173).
func TestFetchLatestRateLimit(t *testing.T) {
	for _, code := range []int{http.StatusForbidden, http.StatusTooManyRequests} {
		t.Run(fmt.Sprintf("HTTP %d", code), func(t *testing.T) {
			srv, _ := newStubServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", "30")
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.WriteHeader(code)
			})
			_, _, err := fetchLatest(srv.Client(), srv.URL, "1.0.0")
			if err == nil {
				t.Fatal("want error, got nil")
			}
			for _, want := range []string{"rate limit", "30", "0"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("err %q does not contain %q", err.Error(), want)
				}
			}
		})
	}
}

// TestFetchLatestServerError proves 5xx answers a server error naming
// the status (T-0173).
func TestFetchLatestServerError(t *testing.T) {
	for _, code := range []int{500, 502, 503} {
		srv, _ := newStubServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
		})
		_, _, err := fetchLatest(srv.Client(), srv.URL, "1.0.0")
		if err == nil || !strings.Contains(err.Error(), fmt.Sprint(code)) {
			t.Errorf("HTTP %d: err = %v, want server error naming %d", code, err, code)
		}
	}
}

// TestFetchLatestMalformedJSON proves an unparseable 200 body fails
// without a panic (T-0173).
func TestFetchLatestMalformedJSON(t *testing.T) {
	srv, _ := newStubServer(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "{not json")
	})
	if _, _, err := fetchLatest(srv.Client(), srv.URL, "1.0.0"); err == nil {
		t.Fatal("want error, got nil")
	}
}

// TestFetchLatestOversizedBody proves a body larger than the 1 MiB limit,
// truncated mid-string, fails decoding without a panic (T-0173).
func TestFetchLatestOversizedBody(t *testing.T) {
	srv, _ := newStubServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "{\"tag_name\":\"")
		_, _ = io.Copy(w, strings.NewReader(strings.Repeat("a", 1<<20)))
		// No closing quote: the LimitReader cut makes decoding fail.
	})
	_, _, err := fetchLatest(srv.Client(), srv.URL, "1.0.0")
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

// TestFetchLatestRedirect proves the default client policy follows a 301
// to the final 200 response (T-0173).
func TestFetchLatestRedirect(t *testing.T) {
	var srvURL string
	srv, _ := newStubServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/releases/latest" {
			w.Header().Set("Location", srvURL+"/final")
			w.WriteHeader(http.StatusMovedPermanently)
			return
		}
		fmt.Fprint(w, latestPayload(t, "1.2.3", "https://example.invalid/r"))
	})
	srvURL = srv.URL
	tag, _, err := fetchLatest(srv.Client(), srv.URL, "1.0.0")
	if err != nil || tag != "1.2.3" {
		t.Errorf("tag=%q err=%v, want 1.2.3 without error", tag, err)
	}
}

// TestFetchLatestOffline proves transport failures and a client-side
// timeout both fail without a panic (T-0173). The timeout is
// deterministic: the RoundTripper returns a context deadline error, no
// sleeping.
func TestFetchLatestOffline(t *testing.T) {
	failing := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	})}
	if _, _, err := fetchLatest(failing, "http://stub.invalid", "1.0.0"); err == nil {
		t.Error("failing RoundTripper: want error, got nil")
	}

	slow := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("round trip: %w", context.DeadlineExceeded)
	})}
	if _, _, err := fetchLatest(slow, "http://stub.invalid", "1.0.0"); err == nil {
		t.Error("timeout client: want error, got nil")
	}
}
