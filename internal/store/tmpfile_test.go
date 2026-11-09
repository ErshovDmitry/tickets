package store

// T-0154: test-only coverage for scrubFile (tmpfile.go). Production code is
// untouched; the hooks above scrubFile do not intercept its direct os.Remove
// and (*os.File).Close calls, so these are in-package tests on a throwaway
// t.TempDir() per case (never the live tickets/ tree).

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scrubCase is one scrubFile scenario: scratch builds its inputs inside a
// per-case temp dir and verify asserts on the value scrubFile returns.
type scrubCase struct {
	name    string
	scratch func(t *testing.T) (f *os.File, path string, cause error)
	verify  func(t *testing.T, cause error, path string, got error)
}

func TestScrubFile(t *testing.T) {
	cause := errors.New("synthetic-scrub-cause")

	cases := []scrubCase{
		{
			name: "nil file removes existing file and keeps cause",
			scratch: func(t *testing.T) (*os.File, string, error) {
				return nil, scratchFile(t), cause
			},
			verify: func(t *testing.T, wantCause error, path string, got error) {
				if !errors.Is(got, wantCause) {
					t.Fatalf("errors.Is(%v, cause) = false, want true", got)
				}
				if strings.Contains(got.Error(), "close "+path+": ") {
					t.Fatalf("unexpected close error for %s: %v", path, got)
				}
				assertAbsent(t, path)
			},
		},
		{
			name: "open file closes and file is removed",
			scratch: func(t *testing.T) (*os.File, string, error) {
				path := scratchFile(t)
				f, err := os.OpenFile(path, os.O_WRONLY, 0o600)
				if err != nil {
					t.Fatalf("OpenFile(%s): %v", path, err)
				}
				return f, path, cause
			},
			verify: func(t *testing.T, wantCause error, path string, got error) {
				if !errors.Is(got, wantCause) {
					t.Fatalf("errors.Is(%v, cause) = false, want true", got)
				}
				if strings.Contains(got.Error(), "close "+path+": ") {
					t.Fatalf("unexpected close error for %s: %v", path, got)
				}
				assertAbsent(t, path)
			},
		},
		{
			name: "already closed file reports close error",
			scratch: func(t *testing.T) (*os.File, string, error) {
				path := scratchFile(t)
				f, err := os.OpenFile(path, os.O_WRONLY, 0o600)
				if err != nil {
					t.Fatalf("OpenFile(%s): %v", path, err)
				}
				if cerr := f.Close(); cerr != nil {
					t.Fatalf("first Close(%s): %v", path, cerr)
				}
				return f, path, cause
			},
			verify: func(t *testing.T, wantCause error, path string, got error) {
				assertContains(t, got, "close "+path+": ")
				if !errors.Is(got, wantCause) {
					t.Fatalf("errors.Is(%v, cause) = false, want true", got)
				}
				assertAbsent(t, path)
			},
		},
		{
			name: "missing file adds no wrapper to cause",
			scratch: func(t *testing.T) (*os.File, string, error) {
				return nil, filepath.Join(t.TempDir(), "absent.md"), cause
			},
			verify: func(t *testing.T, wantCause error, path string, got error) {
				if !errors.Is(got, wantCause) {
					t.Fatalf("errors.Is(%v, cause) = false, want true", got)
				}
				if got.Error() != wantCause.Error() {
					t.Fatalf("got %q, want cause text %q", got.Error(), wantCause.Error())
				}
			},
		},
		{
			name: "non-empty directory reports remove error",
			scratch: func(t *testing.T) (*os.File, string, error) {
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "keep.md"), []byte("keep"), 0o600); err != nil {
					t.Fatalf("WriteFile in %s: %v", dir, err)
				}
				return nil, dir, cause
			},
			verify: func(t *testing.T, wantCause error, path string, got error) {
				assertContains(t, got, "remove "+path+": ")
				if !errors.Is(got, wantCause) {
					t.Fatalf("errors.Is(%v, cause) = false, want true", got)
				}
				if strings.Contains(got.Error(), "close "+path+": ") {
					t.Fatalf("unexpected close error for %s: %v", path, got)
				}
			},
		},
		{
			name: "closed file and non-empty directory report both errors",
			scratch: func(t *testing.T) (*os.File, string, error) {
				dir := t.TempDir()
				path := filepath.Join(dir, "keep.md")
				if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
					t.Fatalf("WriteFile(%s): %v", path, err)
				}
				f, err := os.OpenFile(path, os.O_WRONLY, 0o600)
				if err != nil {
					t.Fatalf("OpenFile(%s): %v", path, err)
				}
				if cerr := f.Close(); cerr != nil {
					t.Fatalf("first Close(%s): %v", path, cerr)
				}
				return f, dir, cause
			},
			verify: func(t *testing.T, wantCause error, path string, got error) {
				assertContains(t, got, "close "+path+": ")
				assertContains(t, got, "remove "+path+": ")
				if !errors.Is(got, wantCause) {
					t.Fatalf("errors.Is(%v, cause) = false, want true", got)
				}
			},
		},
		{
			name: "nil cause and missing file yield nil",
			scratch: func(t *testing.T) (*os.File, string, error) {
				return nil, filepath.Join(t.TempDir(), "absent.md"), nil
			},
			verify: func(t *testing.T, _ error, _ string, got error) {
				if got != nil {
					t.Fatalf("scrubFile(nil, nil, absent) = %v, want nil", got)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, path, cause := tc.scratch(t)
			tc.verify(t, cause, path, scrubFile(cause, f, path))
		})
	}
}

// scratchFile creates an empty file inside a fresh temp dir and returns it.
func scratchFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scratch.md")
	if err := os.WriteFile(path, []byte("body"), 0o600); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

// assertAbsent fails when path still exists.
func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Stat(%s) error = %v, want fs.ErrNotExist", path, err)
	}
}

// assertContains fails when got is nil or lacks want.
func assertContains(t *testing.T, got error, want string) {
	t.Helper()
	if got == nil {
		t.Fatalf("error is nil, want one containing %q", want)
	}
	if !strings.Contains(got.Error(), want) {
		t.Fatalf("error %q does not contain %q", got.Error(), want)
	}
}
