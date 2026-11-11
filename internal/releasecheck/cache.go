package releasecheck

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// cacheEntry is the T-0173 per-user cache record. OK entries carry the
// latest tag and page URL; negative entries carry the failure Reason for
// the backoff path.
type cacheEntry struct {
	CheckedAt time.Time `json:"checked_at"`
	OK        bool      `json:"ok"`
	Latest    string    `json:"latest,omitempty"`
	URL       string    `json:"url,omitempty"`
	Reason    string    `json:"reason,omitempty"`
}

// loadCache reads a cache entry from path (T-0173). Any read or parse
// problem — absent, corrupt or unparsable file — is a miss returned as
// an error; it is never fatal for the caller.
func loadCache(path string) (cacheEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cacheEntry{}, err
	}
	var e cacheEntry
	if err := json.Unmarshal(data, &e); err != nil {
		return cacheEntry{}, err
	}
	return e, nil
}

// saveCache atomically writes the entry to path (T-0173): the target
// directory is created 0700, the payload is written to a temp file in
// the SAME directory (created 0600), then f.Sync() and an explicit
// f.Close() — both checked — run BEFORE os.Rename. The temp file is
// removed on every error path; on POSIX the same-dir rename is atomic,
// on Windows it is best-effort and may transiently fail under an AV
// holder (T-0122), which callers treat as non-fatal.
func saveCache(path string, e cacheEntry) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("releasecheck: cache dir: %w", err)
	}
	f, err := os.CreateTemp(dir, "*.tmp")
	if err != nil {
		return fmt.Errorf("releasecheck: cache temp: %w", err)
	}
	tmp := f.Name()

	if err := json.NewEncoder(f).Encode(e); err != nil {
		return scrubCacheTemp(f, tmp, fmt.Errorf("releasecheck: cache encode: %w", err))
	}
	if err := f.Sync(); err != nil {
		return scrubCacheTemp(f, tmp, fmt.Errorf("releasecheck: cache sync: %w", err))
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("releasecheck: cache close: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("releasecheck: cache rename: %w", err)
	}
	return nil
}

// scrubCacheTemp closes f (already synced or not) and removes the temp
// file on an error path of saveCache (T-0173).
func scrubCacheTemp(f *os.File, tmp string, err error) error {
	_ = f.Close()
	_ = os.Remove(tmp)
	return err
}
