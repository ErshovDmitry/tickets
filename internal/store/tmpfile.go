package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"ticket/internal/domain"
)

// Swappable os primitives for failure-path tests. The zero-value production
// implementations call into the os package directly. removeTmp removes the
// post-commit tmp name (initial removal, link-fail cleanup, best-effort
// retry); removeFile stays reserved for old/rollback removals so swapRemove
// call-counts remain unchanged; renameFile is the same-status journal-only
// rename-over commit (replaces the existing target: atomic on POSIX, on
// Windows MoveFileEx(MOVEFILE_REPLACE_EXISTING) non-atomically).
var (
	linkFile   = os.Link
	renameFile = os.Rename
	removeFile = os.Remove
	removeTmp  = os.Remove
	nanoNow    = func() int64 { return time.Now().UnixNano() }
)

// MaxTmpAttempts caps writeTmp's O_EXCL name-collision retries (§5).
const MaxTmpAttempts = 5

// scrubFile best-effort closes f (nil-safe) and removes path, joining the
// cause with every cleanup error so a failed Create/writeTmp leaves no
// partial file behind. Removing an already-absent file is not an error.
func scrubFile(cause error, f *os.File, path string) error {
	errs := []error{cause}
	if f != nil {
		if cerr := f.Close(); cerr != nil {
			errs = append(errs, fmt.Errorf("close %s: %w", path, cerr))
		}
	}
	if rerr := os.Remove(path); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
		errs = append(errs, fmt.Errorf("remove %s: %w", path, rerr))
	}
	return errors.Join(errs...)
}

// writeTmp creates a .tmp file in dir with O_CREATE|O_EXCL, retrying
// name collisions up to MaxTmpAttempts, writes body, syncs, closes, and
// returns the path. dir MUST be the commit target's directory so the
// link commit stays inside one filesystem: s.Dir for main-directory
// mutations, s.Dir/archive for archive commits. On any failure the
// partial tmp file is removed and the joined error returned. Caller owns
// the no-replace link commit and tmp cleanup on success.
func (s *Store) writeTmp(n int, next domain.Status, body []byte, dir string) (string, error) {
	var lastErr error
	for attempt := 0; attempt < MaxTmpAttempts; attempt++ {
		tmpName := fmt.Sprintf(".T-%04d-%s.%d-%d.md.tmp", n, next, os.Getpid(), nanoNow())
		tmpPath := filepath.Join(dir, tmpName)
		tf, terr := os.OpenFile(tmpPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if terr != nil {
			if errors.Is(terr, fs.ErrExist) {
				lastErr = terr
				continue
			}
			return "", fmt.Errorf("store: tmp open %s: %w", tmpName, terr)
		}
		if _, werr := tf.Write(body); werr != nil {
			return "", scrubFile(fmt.Errorf("store: tmp write %s: %w", tmpName, werr), tf, tmpPath)
		}
		if serr := tf.Sync(); serr != nil {
			return "", scrubFile(fmt.Errorf("store: tmp sync %s: %w", tmpName, serr), tf, tmpPath)
		}
		if cerr := tf.Close(); cerr != nil {
			return "", scrubFile(fmt.Errorf("store: tmp close %s: %w", tmpName, cerr), nil, tmpPath)
		}
		return tmpPath, nil
	}
	return "", fmt.Errorf("%w: tmp name after %d attempts, last err: %v", ErrCollision, MaxTmpAttempts, lastErr)
}
