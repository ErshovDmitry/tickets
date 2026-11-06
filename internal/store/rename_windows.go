//go:build windows

package store

import (
	"errors"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// Windows rename-over commit policy (T-0122). A same-status journal commit
// replaces the target ticket file in place (MoveFileEx with
// MOVEFILE_REPLACE_EXISTING), and that fails with ERROR_ACCESS_DENIED while
// another process holds the target open without FILE_SHARE_DELETE — likely an
// AV scanner or search indexer, though no holder was ever identified.
//
// Observed on dser97: one failure in thirty full runs at baseline, and one in
// thirty with an earlier, shorter retry policy. With the two-second horizon
// below the target test passed 10/10 in isolation and the full store package
// passed 90/90 on the same host. Those runs establish only the absence of a
// rename failure: a retried rename was never observed to succeed after an
// ERROR_ACCESS_DENIED, so whether the retry or a quieter holder explains the
// result is unknown.
//
// Retrying is bounded both in attempts and in wall-clock time, because the
// store lock is held for the whole commit. Only the wall-clock budget and the
// retried errno pair follow the Go standard library precedent
// (testing.removeAll retries ERROR_ACCESS_DENIED/ERROR_SHARING_VIOLATION for
// up to two seconds); the attempt limit and the backoff shape are
// project-specific, since the stdlib helper retries without a limit. Same
// class as go.dev/issue/50051 and go.dev/issue/51442.
const (
	// renameRetryAttempts is the total number of rename calls, the first
	// attempt included. It is a project-specific bound: the store lock is held
	// for the whole commit, so the loop must end even if the holder never
	// releases the file.
	renameRetryAttempts = 10
	// renameRetryBaseDelay is the pause before the second attempt; it doubles
	// on each further attempt. It is project-specific: the standard library
	// helper starts at one millisecond and jitters from there, whereas the
	// earlier 2ms base here did not outlast a real transient holder.
	renameRetryBaseDelay = 10 * time.Millisecond
	// renameRetryMaxDelay caps the exponential backoff. Project-specific, like
	// the base delay.
	renameRetryMaxDelay = 500 * time.Millisecond
	// renameRetryBudget is a best-effort bound on the wall-clock time spent
	// in the retry loop, checked before each pause: the sleep and the rename
	// call that follow are not bounded by it, so the total time the store lock
	// is held may overshoot this value somewhat. The value mirrors the Go
	// standard library's testing.removeAll, which gives a transient rename up
	// to two seconds before surfacing the error. Because the nine backoff
	// pauses sum to 2.13s, the budget — not the attempt limit — is what
	// normally ends the loop, and it is checked before every pause.
	renameRetryBudget = 2 * time.Second
)

// Swappable primitives for the retry tests; the production values call the os
// package and the system clock directly. renameOp is a hook separate from
// renameFile on purpose: init replaces renameFile with renameWithRetry, so
// calling renameFile here would recurse into the wrapper.
var (
	renameOp    = os.Rename
	renameSleep = time.Sleep
	renameNow   = time.Now
)

// init installs the retrying rename-over as the package commit hook.
func init() {
	renameFile = renameWithRetry
}

// renameWithRetry renames oldpath to newpath, retrying only the transient
// Windows sharing errors classified by isTransientRenameError. Any other
// error, an exhausted attempt limit, or an exhausted wall-clock budget is
// returned unchanged for the caller to join with the tmp cleanup. A nil result
// means the target was replaced.
func renameWithRetry(oldpath, newpath string) error {
	deadline := renameNow().Add(renameRetryBudget)
	delay := renameRetryBaseDelay
	err := renameOp(oldpath, newpath)
	for attempt := 1; attempt < renameRetryAttempts; attempt++ {
		if !isTransientRenameError(err) {
			return err
		}
		if renameNow().Add(delay).After(deadline) {
			return err
		}
		renameSleep(delay)
		delay = nextRenameDelay(delay)
		err = renameOp(oldpath, newpath)
	}
	return err
}

// nextRenameDelay doubles delay without exceeding renameRetryMaxDelay.
func nextRenameDelay(delay time.Duration) time.Duration {
	if delay >= renameRetryMaxDelay/2 {
		return renameRetryMaxDelay
	}
	return delay * 2
}

// isTransientRenameError reports whether err unwraps to a Windows errno that
// the OS may clear by itself. It mirrors isWindowsRetryable in the Go
// standard library's testing/testing_windows.go (not a path in this repo):
// unwrap to the innermost error, then match only ERROR_ACCESS_DENIED and
// ERROR_SHARING_VIOLATION. Other states — including
// ERROR_DELETE_PENDING and ERROR_LOCK_VIOLATION — are not retried: they were
// not observed and describe different holders.
func isTransientRenameError(err error) bool {
	inner := err
	for {
		unwrapped := errors.Unwrap(inner)
		if unwrapped == nil {
			break
		}
		inner = unwrapped
	}
	return errors.Is(inner, syscall.ERROR_ACCESS_DENIED) ||
		errors.Is(inner, windows.ERROR_SHARING_VIOLATION)
}
