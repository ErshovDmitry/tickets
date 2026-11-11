package releasecheck

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// errNoReleases signals HTTP 404 from the releases endpoint: the
// repository has no releases yet (T-0173).
var errNoReleases = errors.New("releasecheck: no releases")

// fetchLatest asks the GitHub API for the latest release of the
// repository identified by base (T-0173). version is USED: the request
// carries it in User-Agent so GitHub sees "ticket/<version>"; no repo
// URL is added — base is the single source of repository identity. On
// 200 it returns the tag name and the HTML page; 404 answers
// errNoReleases; anything else yields an error naming the cause.
func fetchLatest(c *http.Client, base, version string) (tag, html string, err error) {
	u := strings.TrimRight(base, "/") + "/releases/latest"
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return "", "", fmt.Errorf("releasecheck: build request: %w", err)
	}
	req.Header.Set("User-Agent", "ticket/"+version)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28") // pinned (T-0173)

	resp, err := c.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("releasecheck: request: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK:
		var payload struct {
			TagName string `json:"tag_name"`
			HTMLURL string `json:"html_url"`
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
			return "", "", fmt.Errorf("releasecheck: decode latest release: %w", err)
		}
		return payload.TagName, payload.HTMLURL, nil
	case resp.StatusCode == http.StatusNotFound:
		return "", "", errNoReleases
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		msg := fmt.Sprintf("releasecheck: GitHub rate limit (HTTP %d)", resp.StatusCode)
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			msg += ", retry after " + ra + "s"
		}
		if rem := resp.Header.Get("X-RateLimit-Remaining"); rem != "" {
			msg += ", remaining " + rem
		}
		return "", "", errors.New(msg)
	case resp.StatusCode >= 500:
		return "", "", fmt.Errorf("releasecheck: GitHub server error: HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	default:
		return "", "", fmt.Errorf("releasecheck: GitHub API error: HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
}
