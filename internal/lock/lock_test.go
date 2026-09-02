package lock

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestAcquireRelease verifies the basic happy path: Acquire returns a
// non-nil release closure, the lock is held until release is called,
// and the closure is idempotent (every call returns the same error as
// the first one).
func TestAcquireRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".lock")

	release, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if release == nil {
		t.Fatal("Acquire returned a nil release closure")
	}

	// While held, a competing TryAcquire must report ErrLocked.
	if r, err := TryAcquire(path); !errors.Is(err, ErrLocked) {
		if r != nil {
			_ = r()
		}
		t.Errorf("TryAcquire while held: err = %v, want ErrLocked", err)
	}

	firstErr := release()
	for i := 0; i < 4; i++ {
		if gotErr := release(); gotErr != firstErr {
			t.Errorf("release call #%d = %v, want %v (idempotent)", i+2, gotErr, firstErr)
		}
	}

	// After release the file should be lockable again by someone else.
	r2, err := TryAcquire(path)
	if err != nil {
		t.Fatalf("TryAcquire after release: %v", err)
	}
	if err := r2(); err != nil {
		t.Errorf("post-release release: %v", err)
	}
}

// TestContention: process A holds the lock; process B's TryAcquire must
// return (nil, ErrLocked) within one second without blocking.
func TestContention(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".lock")

	releaseA, err := Acquire(path)
	if err != nil {
		t.Fatalf("holder Acquire: %v", err)
	}
	defer releaseA()

	done := make(chan struct {
		r   func() error
		err error
	}, 1)
	go func() {
		r, err := TryAcquire(path)
		done <- struct {
			r   func() error
			err error
		}{r, err}
	}()

	select {
	case res := <-done:
		if res.r != nil {
			t.Errorf("TryAcquire under contention returned non-nil release: %p", res.r)
		}
		if !errors.Is(res.err, ErrLocked) {
			t.Errorf("TryAcquire under contention err = %v, want ErrLocked", res.err)
		}
	case <-time.After(time.Second):
		t.Fatal("TryAcquire blocked for > 1s; expected non-blocking failure")
	}
}

// TestReleaseConcurrent fires many goroutines at a single release
// closure. With sync.Once guarding the unlock+close, exactly one of
// them performs the work and all observe the same error.
func TestReleaseConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".lock")

	release, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if release == nil {
		t.Fatal("Acquire returned a nil release closure")
	}

	const n = 32
	var wg sync.WaitGroup
	errs := make([]error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = release()
		}(i)
	}
	close(start)
	wg.Wait()

	var first error
	for i, e := range errs {
		if i == 0 {
			first = e
			continue
		}
		if e != first {
			t.Errorf("release #%d = %v, want %v", i, e, first)
		}
	}
}

// TestReleaseNilSafe asserts that calling the release-style methods on a
// nil *Locker is a no-op and never panics, even after the sync.Once
// machinery is involved.
func TestReleaseNilSafe(t *testing.T) {
	var nilLocker *Locker

	if err := nilLocker.Release(); err != nil {
		t.Errorf("nil.Release() = %v, want nil", err)
	}
	if err := nilLocker.OnceRelease(); err != nil {
		t.Errorf("nil.OnceRelease() = %v, want nil", err)
	}
}

// TestCompileMatrix inspects the platform-specific source files to make
// sure the build tags spelled out in AGENTS_ARCHITECTURE.md and the
// plan are byte-present, including the unix-tagged EINTR test file.
// Actual cross-GOOS vet/build runs in the go.mod integration wave; the
// structural guarantee keeps the v1 acceptance criteria auditable.
func TestCompileMatrix(t *testing.T) {
	cases := []struct {
		file string
		tag  string
	}{
		{"lock_unix.go", "//go:build unix"},
		{"lock_unix_test.go", "//go:build unix"},
		{"lock_windows.go", "//go:build windows"},
	}
	for _, c := range cases {
		data, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatalf("missing %s: %v", c.file, err)
		}
		if !bytes.Contains(data, []byte(c.tag)) {
			t.Errorf("%s missing build tag %q", c.file, c.tag)
		}
	}
}
