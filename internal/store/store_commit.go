package store

import (
	"errors"
	"fmt"
	"io/fs"

	"ticket/internal/domain"
)

// commitMove is the shared cross-directory commit sequence of archive
// and delete (T-0189): it writes body to a tmp file in dir, commits it
// to target via the no-replace link, then removes old. A failed
// commit cleans the tmp file up. A failed removal of the tmp name
// after the commit triggers a rollback (removeFile(target), the
// target directory never gains a duplicate) plus a best-effort
// tmp-removal retry; a failed retry is joined with the tmp-removal
// error. A failed removal of old rolls the target back for the same
// reason. A failed rollback in either branch joins the invariant
// breach marker (old+new coexist). An already-absent old file is
// success: only the new file
// remains, which is the invariant. failure logs the caller's terminal
// event (store.archive.failure / store.delete.failure) with the stage.
func (s *Store) commitMove(n int, st domain.Status, body []byte, old, target, dir string, failure func(int, string)) error {
	tmpPath, werr := s.writeTmp(n, st, body, dir)
	if werr != nil {
		failure(n, "tmp")
		return werr
	}
	if lerr := linkFile(tmpPath, target); lerr != nil {
		if errors.Is(lerr, fs.ErrExist) {
			lerr = &CollisionError{Target: target}
		} else {
			lerr = fmt.Errorf("store: link commit: %w", lerr)
		}
		failure(n, "link")
		return errors.Join(lerr, removeTmp(tmpPath))
	}
	// tmp and target share one inode now; drop the tmp name via the
	// removeTmp hook, NOT the removeFile hook (the removeFile call-count
	// stays at remove-old + rollback). A removal failure rolls back the
	// committed target (removeFile) and retries the tmp removal
	// best-effort; a failed retry is joined with the tmp-removal error.
	if rerr := removeTmp(tmpPath); rerr != nil {
		// The commit already happened (target holds the new body);
		// roll it back by removing the target.
		failure(n, "tmp")
		tmpErr := fmt.Errorf("store: remove tmp %s: %w", tmpPath, rerr)
		if rbErr := removeFile(target); rbErr != nil {
			return errors.Join(
				tmpErr,
				fmt.Errorf("rollback remove %s: %w", target, rbErr),
				errors.New("invariant breach: old+new coexist"),
			)
		}
		// Rollback succeeded: retry the tmp removal best-effort; a
		// failed retry is joined with the original tmp-removal error.
		if retryErr := removeTmp(tmpPath); retryErr != nil {
			tmpErr = errors.Join(tmpErr, retryErr)
		}
		return tmpErr
	}
	if rerr := removeFile(old); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
		failure(n, "old")
		rbErr := removeFile(target)
		if rbErr != nil {
			return errors.Join(
				fmt.Errorf("remove old %s: %w", old, rerr),
				fmt.Errorf("rollback remove %s: %w", target, rbErr),
				errors.New("invariant breach: old+new coexist"),
			)
		}
		return fmt.Errorf("remove old %s: %w", old, rerr)
	}
	return nil
}
