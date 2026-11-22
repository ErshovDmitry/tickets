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
	"time"
)

// progressInterval is how often a holder refreshes the lock file mtime.
const progressInterval = 250 * time.Millisecond

// progressCapFactor bounds a waiter's total patience at
// progressCapFactor × timeout: beyond that, a holder that keeps
// heartbeating without finishing is treated as livelocked and the waiter
// fails exactly as it would have without progress tracking.
const progressCapFactor = 12

// progressMark is one observation of the lock file's identity and mtime.
type progressMark struct {
	fi    os.FileInfo
	mtime time.Time
}

// statProgress reads the lock file's mark. A missing file is not an
// error: it yields a nil mark and the acquire attempt reports the swap.
func statProgress(path string) (*progressMark, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return &progressMark{fi: fi, mtime: fi.ModTime()}, nil
}

// progressed reports whether cur proves a living holder: the same file
// as before (T-0073: a replacement is never progress) with a newer mtime.
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
		now := time.Now()
		_ = os.Chtimes(l.path, now, now)
		go func() {
			ticker := time.NewTicker(progressInterval)
			defer ticker.Stop()
			for {
				select {
				case <-l.progressStop:
					return
				case <-ticker.C:
					now := time.Now()
					_ = os.Chtimes(l.path, now, now)
				}
			}
		}()
	})
}

// stopProgress halts the heartbeat. Safe to call when it never started.
func (l *Locker) stopProgress() {
	if l.progressStop != nil {
		close(l.progressStop)
	}
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
	deadline := time.Now().Add(timeout)
	patienceCap := time.Now().Add(progressCapFactor * timeout)
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
