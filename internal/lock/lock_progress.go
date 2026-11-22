package lock

// Holder progress / liveness tracking (T-0226). A plain wall-clock
// timeout expires while a *healthy* holder is still working: on
// oversubscribed hosts the waiter's budget is burned by scheduling noise
// rather than by a stuck peer, and `ticket new` fails spuriously with
// "still held by another process after 5s" (CI run 44942515928,
// reproduced with the exact stderr in probe run 44983487666). The holder
// therefore refreshes the lock file's mtime while it holds the lock, and
// a waiter extends its deadline on every observed progress:
// TICKET_LOCK_TIMEOUT bounds the time the lock may remain WITHOUT HOLDER
// PROGRESS, not the waiter's wall clock. The extension is capped so a
// livelocked (heartbeating but never finishing) holder is still
// reported after a bounded wait.

import (
	"errors"
	"os"
	"sync/atomic"
	"time"
)

// progressInterval is how often a holder refreshes the lock file mtime.
// It is a var (not a const) so tests can shorten the heartbeat; only
// sequential tests may override it (never under t.Parallel).
var progressInterval = 250 * time.Millisecond

// chtimesFailures counts heartbeat stamp failures since process start.
// Stamping is best-effort and fail-safe: a failure (permissions, a
// filesystem that will not update mtimes, a Windows sharing violation)
// silently degrades that one beat, so the waiter sees no progress and
// reverts to the plain configured timeout. The counter keeps the
// degradation observable for tests and future debug hooks without
// polluting the CLI (the lock package has no logger).
var chtimesFailures atomic.Int64

// progressCapFactor bounds a waiter's total patience at
// progressCapFactor × timeout: beyond that, a holder that keeps
// heartbeating without finishing is treated as livelocked and the waiter
// fails exactly as it would have without progress tracking. It is a var
// (not a const) so tests can shrink the cap; only sequential tests may
// override it (never under t.Parallel).
var progressCapFactor int64 = 12

// progressMark is one observation of the lock file's identity and mtime.
type progressMark struct {
	fi    os.FileInfo
	mtime time.Time
}

// statProgress reads the lock file's mark. Any stat error (a missing
// file included) is returned: the caller skips progress tracking for
// that attempt, and tryAcquire reports a replacement file as a swap
// (T-0073) or recreates the file.
func statProgress(path string) (*progressMark, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return &progressMark{fi: fi, mtime: fi.ModTime()}, nil
}

// progressed reports whether cur proves a living holder: the same file
// as before (T-0073: a replacement is never progress) with a mtime
// strictly newer than the previous observation. Equal mtimes do NOT
// count: on filesystems with coarse mtime granularity (FAT 2s, HFS+
// 1s) several heartbeats can land in one granularity window, so
// progress is observed at most once per granularity unit. Configure
// TICKET_LOCK_TIMEOUT above the filesystem granularity (and above the
// ~250ms holder-refresh interval).
func (cur *progressMark) progressed(prev *progressMark) bool {
	return cur.mtime.After(prev.mtime) && os.SameFile(cur.fi, prev.fi)
}

// newLocked wraps a freshly acquired lock in a Locker and starts its
// holder-progress heartbeat.
func newLocked(f *os.File, path string) *Locker {
	lk := &Locker{f: f, path: path}
	lk.startProgress()
	return lk
}

// startProgress stamps the lock file once synchronously (a waiter that
// stats between the grant and the first tick must already see a fresh
// holder) and then launches the heartbeat that refreshes the mtime until
// Release. Idempotent.
func (l *Locker) startProgress() {
	l.progressStartOnce.Do(func() {
		l.progressStop = make(chan struct{})
		l.progressDone = make(chan struct{})
		now := time.Now()
		if err := os.Chtimes(l.path, now, now); err != nil {
			chtimesFailures.Add(1)
		}
		// Read the interval once, before the goroutine: a test may swap
		// the package var while a previous holder's heartbeat is still
		// running (sequential tests, joined stops make this the only
		// reader).
		interval := progressInterval
		go func() {
			defer close(l.progressDone)
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-l.progressStop:
					return
				case <-ticker.C:
					now := time.Now()
					if err := os.Chtimes(l.path, now, now); err != nil {
						chtimesFailures.Add(1)
					}
				}
			}
		}()
	})
}

// AcquireNoHeartbeat is Acquire for a holder that will never report
// progress: it starts no mtime heartbeat, so a competing AcquireLocked
// waiter's deadline is never extended past the configured timeout. It
// models a stalled holder or a pre-T-0226 binary; it exists for tests
// and diagnostics, not for normal ticket commands.
func AcquireNoHeartbeat(path string) (release func() error, err error) {
	lk, err := acquireVia(path, openLockFile, false)
	if err != nil {
		return nil, err
	}
	return lk.OnceRelease, nil
}

// stopProgress halts the heartbeat and joins it: the goroutine has
// exited (no Chtimes in flight) before this returns, so Release can
// drop the lock without a stray stamp landing on a competitor's file.
// Idempotent; safe to call when the heartbeat never started.
func (l *Locker) stopProgress() {
	l.progressStopOnce.Do(func() {
		if l.progressStop != nil {
			close(l.progressStop)
			<-l.progressDone
		}
	})
}

// AcquireLocked is the Locker-returning form of AcquireTimeout: it spins
// on non-blocking attempts until the lock is granted or the deadline
// passes, sleeping at most pollInterval between attempts. The first
// attempt never sleeps; timeout <= 0 means exactly one attempt.
//
// Deadline expiry wraps ErrLocked (match with errors.Is); other errors
// are wrapped with path context. The deadline measures holder STALL, not
// wall time: every observed holder progress (lock-file mtime advancing
// on the same file) pushes it forward, up to progressCapFactor × timeout
// in total (T-0226). A lock held by a non-heartbeating peer (an older
// binary, or one stuck before the heartbeat starts) therefore still
// expires on the plain timeout.
func AcquireLocked(path string, timeout time.Duration) (*Locker, error) {
	if timeout <= 0 {
		// Single-attempt mode: never sleep, never retry.
		lk, err := tryAcquire(path)
		if err == nil {
			return lk, nil
		}
		if errors.Is(err, ErrLocked) {
			return nil, stillHeld(path, timeout)
		}
		return nil, err
	}
	now := time.Now()
	deadline := now.Add(timeout)
	patienceCap := now.Add(time.Duration(progressCapFactor) * timeout)
	var last *progressMark
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			d := time.Until(deadline)
			if d <= 0 {
				return nil, stillHeld(path, timeout)
			}
			if d > pollInterval {
				d = pollInterval
			}
			time.Sleep(d)
		}
		if cur, err := statProgress(path); err == nil {
			if last != nil && cur.progressed(last) {
				if ext := time.Now().Add(timeout); ext.After(deadline) {
					deadline = ext
				}
				if deadline.After(patienceCap) {
					deadline = patienceCap
				}
			}
			last = cur
		}
		lk, err := tryAcquire(path)
		if err == nil {
			return lk, nil
		}
		if !errors.Is(err, ErrLocked) {
			return nil, err
		}
	}
}
