package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"ticket/internal/domain"
)

// runShortWriter accepts everything but reports n-1 bytes written with no
// error, the classic short-write case io.WriteString cannot detect itself.
type runShortWriter struct{}

func (runShortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

// runNthFailWriter fails the n-th (1-based) Write call and accepts every
// other call in full, so a test can target the version-line write (n=1)
// separately from the template write (n=2).
type runNthFailWriter struct {
	n    int
	err  error
	call int
	buf  bytes.Buffer
}

func (w *runNthFailWriter) Write(p []byte) (int, error) {
	w.call++
	if w.call == w.n {
		return 0, w.err
	}
	return w.buf.Write(p)
}

// runNthShortWriter reports len(p)-1 bytes written with no error on the n-th
// Write call only, for stage-specific short-write coverage.
type runNthShortWriter struct {
	n    int
	call int
	buf  bytes.Buffer
}

func (w *runNthShortWriter) Write(p []byte) (int, error) {
	w.call++
	if w.call == w.n {
		if len(p) == 0 {
			return 0, nil
		}
		n := len(p) - 1
		if written, err := w.buf.Write(p[:n]); err != nil {
			return written, err
		}
		return n, nil
	}
	return w.buf.Write(p)
}

// runCountingWriter counts Write calls and keeps the bytes, proving that the
// version line and the template body are two independent writes.
type runCountingWriter struct {
	calls int
	buf   bytes.Buffer
}

func (w *runCountingWriter) Write(p []byte) (int, error) {
	w.calls++
	return w.buf.Write(p)
}

// runErrReadFS forces a non-ErrNotExist open failure for the named paths, so
// the fallback matrix can be exercised against a read error that is not a
// plain "missing" result.
type runErrReadFS struct {
	fsys fs.FS
	fail map[string]error
}

func (f runErrReadFS) Open(name string) (fs.File, error) {
	if err, ok := f.fail[name]; ok {
		return nil, err
	}
	return f.fsys.Open(name)
}

func TestUsageFromFallbackMatrix(t *testing.T) {
	files := fstest.MapFS{
		"templates/usage.ru.txt": &fstest.MapFile{Data: []byte("RU-BODY")},
		"templates/usage.en.txt": &fstest.MapFile{Data: []byte("EN-BODY")},
	}

	t.Run("selected language", func(t *testing.T) {
		var b bytes.Buffer
		if err := usageFrom(&b, domain.LangRU, files, nil); err != nil {
			t.Fatalf("usageFrom = %v, want nil", err)
		}
		if got, want := b.String(), versionLine()+"RU-BODY"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})

	t.Run("selected missing falls back to EN with error naming language and path", func(t *testing.T) {
		var b bytes.Buffer
		err := usageFrom(&b, domain.Lang("zz"), files, nil)
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("usageFrom error = %v, want fs.ErrNotExist", err)
		}
		if !strings.Contains(err.Error(), "\"zz\"") {
			t.Fatalf("usageFrom error = %q, want it to name the requested language %q", err, "zz")
		}
		if !strings.Contains(err.Error(), "templates/usage.zz.txt") {
			t.Fatalf("usageFrom error = %q, want it to name the selected path", err)
		}
		if got, want := b.String(), versionLine()+"EN-BODY"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})

	t.Run("neither resource writes nothing", func(t *testing.T) {
		var b bytes.Buffer
		err := usageFrom(&b, domain.LangRU, fstest.MapFS{}, nil)
		if err == nil {
			t.Fatal("usageFrom = nil, want an error")
		}
		if b.Len() != 0 {
			t.Fatalf("stdout = %q, want empty", b.String())
		}
	})

	t.Run("valid selected render never reads EN", func(t *testing.T) {
		readErr := errors.New("EN unreadable")
		fsys := runErrReadFS{
			fsys: files,
			fail: map[string]error{"templates/usage.en.txt": readErr},
		}
		var b bytes.Buffer
		if err := usageFrom(&b, domain.LangRU, fsys, nil); err != nil {
			t.Fatalf("usageFrom = %v, want nil: EN must not be read when RU is readable", err)
		}
		if got, want := b.String(), versionLine()+"RU-BODY"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})

	t.Run("valid selected render ignores an unrelated init error", func(t *testing.T) {
		preload := errors.New("preload failed")
		var b bytes.Buffer
		if err := usageFrom(&b, domain.LangRU, files, preload); err != nil {
			t.Fatalf("usageFrom = %v, want nil: the selected template is readable", err)
		}
		if got, want := b.String(), versionLine()+"RU-BODY"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})

	t.Run("init error is joined on the EN fallback path", func(t *testing.T) {
		preload := errors.New("preload failed")
		var b bytes.Buffer
		err := usageFrom(&b, domain.Lang("zz"), files, preload)
		if !errors.Is(err, preload) {
			t.Fatalf("usageFrom error = %v, want the preload error", err)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("usageFrom error = %v, want fs.ErrNotExist for the selected template", err)
		}
		if got, want := b.String(), versionLine()+"EN-BODY"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})

	t.Run("non-ErrNotExist selected read error falls back to EN", func(t *testing.T) {
		readErr := fmt.Errorf("disk on fire: %w", fs.ErrPermission)
		fsys := runErrReadFS{
			fsys: files,
			fail: map[string]error{"templates/usage.zz.txt": readErr},
		}
		var b bytes.Buffer
		err := usageFrom(&b, domain.Lang("zz"), fsys, nil)
		if !errors.Is(err, fs.ErrPermission) {
			t.Fatalf("usageFrom error = %v, want fs.ErrPermission", err)
		}
		if errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("usageFrom error = %v, must not be reported as fs.ErrNotExist", err)
		}
		if !strings.Contains(err.Error(), "\"zz\"") {
			t.Fatalf("usageFrom error = %q, want it to name the requested language", err)
		}
		if got, want := b.String(), versionLine()+"EN-BODY"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})

	t.Run("write error is returned", func(t *testing.T) {
		writeErr := errors.New("write failed")
		err := usageFrom(runFailWriter{err: writeErr}, domain.LangRU, files, nil)
		if !errors.Is(err, writeErr) {
			t.Fatalf("usageFrom error = %v, want the write error", err)
		}
	})

	t.Run("version line and template are two writes", func(t *testing.T) {
		var w runCountingWriter
		if err := usageFrom(&w, domain.LangRU, files, nil); err != nil {
			t.Fatalf("usageFrom = %v, want nil", err)
		}
		if w.calls != 2 {
			t.Fatalf("Write called %d times, want 2 (version line + template)", w.calls)
		}
		if got, want := w.buf.String(), versionLine()+"RU-BODY"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})

	t.Run("first write (version line) error is returned", func(t *testing.T) {
		writeErr := errors.New("version write failed")
		w := &runNthFailWriter{n: 1, err: writeErr}
		err := usageFrom(w, domain.LangRU, files, nil)
		if !errors.Is(err, writeErr) {
			t.Fatalf("usageFrom error = %v, want the version-line write error", err)
		}
		if !strings.Contains(err.Error(), "version line") {
			t.Fatalf("usageFrom error = %q, want it to attribute the version line", err)
		}
		// Both stages are attempted: the body write is a separate call and
		// its own failure would be reported too.
		if w.call != 2 {
			t.Fatalf("Write called %d times, want 2", w.call)
		}
		if got, want := w.buf.String(), "RU-BODY"; got != want {
			t.Fatalf("output = %q, want only the body %q when the version line failed", got, want)
		}
	})

	t.Run("second write (template body) error is returned", func(t *testing.T) {
		writeErr := errors.New("body write failed")
		w := &runNthFailWriter{n: 2, err: writeErr}
		err := usageFrom(w, domain.LangRU, files, nil)
		if !errors.Is(err, writeErr) {
			t.Fatalf("usageFrom error = %v, want the body write error", err)
		}
		if !strings.Contains(err.Error(), "body") {
			t.Fatalf("usageFrom error = %q, want it to attribute the template body", err)
		}
		if got, want := w.buf.String(), versionLine(); got != want {
			t.Fatalf("output = %q, want only the version line %q", got, want)
		}
		if w.call != 2 {
			t.Fatalf("Write called %d times, want 2", w.call)
		}
	})

	t.Run("short write is an error", func(t *testing.T) {
		err := usageFrom(runShortWriter{}, domain.LangRU, files, nil)
		if !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("usageFrom error = %v, want io.ErrShortWrite", err)
		}
	})

	t.Run("second-write short write is an error", func(t *testing.T) {
		w := &runNthShortWriter{n: 2}
		err := usageFrom(w, domain.LangRU, files, nil)
		if !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("usageFrom error = %v, want io.ErrShortWrite for the body write", err)
		}
		if got, want := w.buf.String(), versionLine()+"RU-BOD"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})
}
