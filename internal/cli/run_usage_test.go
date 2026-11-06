package cli

import (
	"bytes"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"ticket/internal/domain"
)

// runFailWriter fails every write.
type runFailWriter struct{ err error }

func (w runFailWriter) Write([]byte) (int, error) { return 0, w.err }

func TestRunUsageBytesAndCodes(t *testing.T) {
	withRunSeams(t, func() error { return nil }, langFrom)

	cases := []struct {
		name     string
		args     []string
		env      map[string]string
		wantCode int
		want     string
	}{
		{"no args ru", nil, nil, 0, versionLine() + usageTextRU},
		{"help ru", []string{"help"}, nil, 0, versionLine() + usageTextRU},
		{"-h ru", []string{"-h"}, nil, 0, versionLine() + usageTextRU},
		{"--help ru", []string{"--help"}, nil, 0, versionLine() + usageTextRU},
		{"unknown ru", []string{"nope"}, nil, 1, versionLine() + usageTextRU},
		{"subcommand help en", []string{"new", "-h"}, map[string]string{"TICKET_LANG": "en"}, 0, versionLine() + usageTextEN},
		{"show subcommand help ru", []string{"show", "-h"}, nil, 0, versionLine() + usageTextRU},
		{"set subcommand help ru", []string{"set", "-h"}, nil, 0, versionLine() + usageTextRU},
		{"init help en", []string{"init", "-h"}, map[string]string{"TICKET_LANG": "en"}, 0, versionLine() + usageTextEN},
		{"init extra args ru", []string{"init", "x"}, nil, 1, versionLine() + usageTextRU},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := Run(tc.args, tc.env, &stdout, &stderr); code != tc.wantCode {
				t.Fatalf("Run(%q) = %d, want %d; stderr: %q", tc.args, code, tc.wantCode, stderr.String())
			}
			if got := stdout.String(); got != tc.want {
				t.Fatalf("stdout is not the expected byte fixture (got %d bytes, want %d)", len(got), len(tc.want))
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

// TestUsageProductionWrapper drives usage() itself with swapped usageFS and
// usageInitErr seams: the production wrapper must pass the ACTIVE filesystem
// and the stored init error to usageFrom. A readable selected template wins
// over an unrelated init error; the same error becomes visible only when the
// selected read falls back. No t.Parallel: the seams are package-level state.
func TestUsageProductionWrapper(t *testing.T) {
	prevFS, prevInitErr := usageFS, usageInitErr
	t.Cleanup(func() { usageFS, usageInitErr = prevFS, prevInitErr })

	usageFS = fstest.MapFS{
		"templates/usage.ru.txt": &fstest.MapFile{Data: []byte("RU-SEAM")},
		"templates/usage.en.txt": &fstest.MapFile{Data: []byte("EN-SEAM")},
	}

	t.Run("valid selected render ignores a stored init error", func(t *testing.T) {
		usageInitErr = errors.New("preload failed")
		var b bytes.Buffer
		if err := usage(&b, domain.LangRU); err != nil {
			t.Fatalf("usage = %v, want nil for a readable selected template", err)
		}
		if got, want := b.String(), versionLine()+"RU-SEAM"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})

	t.Run("fallback joins the stored init error", func(t *testing.T) {
		usageInitErr = errors.New("preload failed")
		var b bytes.Buffer
		err := usage(&b, domain.Lang("zz"))
		if !errors.Is(err, usageInitErr) {
			t.Fatalf("usage error = %v, want the stored init error", err)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("usage error = %v, want fs.ErrNotExist for the selected template", err)
		}
		if got, want := b.String(), versionLine()+"EN-SEAM"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})
}

// TestRunUsageWriteFailureIsOneDiagnostic drives a failing stdout writer
// through EVERY usage call route. There are eight printUsage call sites:
//
//  1. cli.go  init -h
//  2. cli.go  init with extra args
//  3. cli.go  subcommand -h (new/show/set/archive branch)
//  4. cli.go  help/-h/--help
//  5. cli.go  unknown command (default branch)
//  6. cmd_new.go  bare `new`
//  7. cmd_show.go bare `show`
//  8. cmd_set.go  bare `set`
//
// Each route must report the writer failure as exactly ONE stderr line and
// exit code 1, including the paths that normally exit 0 (help, -h). The
// normal-path stdout matrix is covered by TestRunUsageBytesAndCodes and
// TestRunBareCallerUsageErrors, which stay unchanged.
func TestRunUsageWriteFailureIsOneDiagnostic(t *testing.T) {
	withRunSeams(t, func() error { return nil }, langFrom)
	writeErr := errors.New("stdout broken")

	cases := []struct {
		name string
		args []string
	}{
		{"init -h (site 1)", []string{"init", "-h"}},
		{"init extra args (site 2)", []string{"init", "x"}},
		{"new -h (site 3)", []string{"new", "-h"}},
		{"show -h (site 3)", []string{"show", "-h"}},
		{"set -h (site 3)", []string{"set", "-h"}},
		{"help (site 4)", []string{"help"}},
		{"no args (site 4)", nil},
		{"unknown command (site 5)", []string{"nope"}},
		{"new without a title (site 6)", []string{"new"}},
		{"show without a number (site 7)", []string{"show"}},
		{"set without a number and status (site 8)", []string{"set"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			code := Run(tc.args, map[string]string{"TICKETS_DIR": t.TempDir()}, runFailWriter{err: writeErr}, &stderr)
			if code != 1 {
				t.Fatalf("Run(%q) = %d, want 1 on a failing usage writer", tc.args, code)
			}
			if !strings.Contains(stderr.String(), writeErr.Error()) {
				t.Fatalf("stderr = %q, want it to name the write error", stderr.String())
			}
			if strings.Count(stderr.String(), "\n") != 1 {
				t.Fatalf("stderr must be exactly one diagnostic line, got %q", stderr.String())
			}
		})
	}
}

// TestRunBareCallerUsageErrors drives the bare-argument usage branches of the
// three callers that print usage themselves (cmdNew, cmdShow, cmdSet): the
// usage text goes to stdout, stderr stays empty and the exit code is 1.
func TestRunBareCallerUsageErrors(t *testing.T) {
	withRunSeams(t, func() error { return nil }, langFrom)

	cases := []struct {
		name string
		args []string
	}{
		{"new without a title", []string{"new"}},
		{"show without a number", []string{"show"}},
		{"set without a number and status", []string{"set"}},
		{"set with only a number", []string{"set", "1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			var stdout, stderr bytes.Buffer
			code := Run(tc.args, map[string]string{"TICKETS_DIR": dir}, &stdout, &stderr)
			if code != 1 {
				t.Fatalf("Run(%q) = %d, want 1; stderr: %q", tc.args, code, stderr.String())
			}
			if got, want := stdout.String(), versionLine()+usageTextRU; got != want {
				t.Fatalf("stdout is not the version line + RU usage (got %d bytes, want %d)", len(got), len(want))
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}
