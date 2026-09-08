package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func mustEvalSymlinks(t *testing.T, p string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", p, err)
	}
	return resolved
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatalf("Abs(%q): %v", p, err)
	}
	return abs
}

// mkdirTemp creates dir and its parents under t.TempDir().
func mkdirTemp(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", dir, err)
	}
	return dir
}

// assertInvalidDir checks ErrInvalidDir discrimination: the sentinel, the
// source label ("(TICKETS_DIR)" or "(--tickets-dir)") and the human hint.
func assertInvalidDir(t *testing.T, err error, label, hint string) {
	t.Helper()
	if !errors.Is(err, ErrInvalidDir) {
		t.Fatalf("err = %v, want errors.Is ErrInvalidDir", err)
	}
	if !strings.Contains(err.Error(), "("+label+")") {
		t.Errorf("err = %q, want source label (%s)", err, label)
	}
	if !strings.Contains(err.Error(), hint) {
		t.Errorf("err = %q, want hint %q", err, hint)
	}
}

// TestResolveEnv covers the $TICKETS_DIR tier: a valid dir resolves, while
// a missing path or a regular file yields ErrInvalidDir carrying the
// (TICKETS_DIR) source label. Assumption: no ancestor of t.TempDir()
// contains a directory named "tickets" (empty-env rows below).
func TestResolveEnv(t *testing.T) {
	root := t.TempDir()
	tickets := mkdirTemp(t, filepath.Join(root, "tickets"))
	file := filepath.Join(root, "plain.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tests := []struct {
		name    string
		env     string
		want    string
		wantErr error
		hint    string
	}{
		{name: "existing dir", env: tickets, want: mustEvalSymlinks(t, tickets)},
		{name: "missing path", env: filepath.Join(root, "nope"), wantErr: ErrInvalidDir,
			hint: "path does not exist: " + mustAbs(t, filepath.Join(root, "nope"))},
		{name: "regular file", env: file, wantErr: ErrInvalidDir,
			hint: "not a directory: " + mustEvalSymlinks(t, file)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(map[string]string{envTicketsDir: tt.env}, t.TempDir(), "")
			if tt.wantErr != nil {
				assertInvalidDir(t, err, srcEnv, tt.hint)
				return
			}
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got != tt.want {
				t.Errorf("Resolve = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestResolveEmptyEnvFallsThrough covers the empty/unset $TICKETS_DIR
// fall-through to the upward scan.
func TestResolveEmptyEnvFallsThrough(t *testing.T) {
	root := t.TempDir()
	tickets := mkdirTemp(t, filepath.Join(root, "tickets"))
	cwd := mkdirTemp(t, filepath.Join(root, "deep", "nested"))
	want := mustEvalSymlinks(t, tickets)

	tests := []struct {
		name string
		env  map[string]string
	}{
		{name: "unset key", env: map[string]string{}},
		{name: "empty value", env: map[string]string{envTicketsDir: ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.env, cwd, "")
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got != want {
				t.Errorf("Resolve = %q, want %q", got, want)
			}
		})
	}
}

// TestScanUpward covers Tier 2: upward scan from cwd, file rejection and
// root stopping. Assumption: no ancestor of t.TempDir() contains a directory
// named "tickets" for the wantOK=false rows.
func TestScanUpward(t *testing.T) {
	root := t.TempDir()
	tickets := mkdirTemp(t, filepath.Join(root, "tickets"))
	deep := mkdirTemp(t, filepath.Join(root, "a", "b"))

	fileRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(fileRoot, dirName), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	filesystemRoot := filepath.Dir(filepath.Clean(string(filepath.Separator)))

	tests := []struct {
		name   string
		cwd    string
		want   string
		wantOK bool
	}{
		{name: "tickets in cwd", cwd: root, want: mustEvalSymlinks(t, tickets), wantOK: true},
		{name: "tickets in ancestor", cwd: deep, want: mustEvalSymlinks(t, tickets), wantOK: true},
		{name: "regular file tickets rejected", cwd: fileRoot, want: "", wantOK: false},
		{name: "filesystem root stops walk", cwd: filesystemRoot, want: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := scanUpward(tt.cwd)
			if ok != tt.wantOK {
				t.Fatalf("scanUpward(%q) ok = %v, want %v", tt.cwd, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("scanUpward(%q) = %q, want %q", tt.cwd, got, tt.want)
			}
		})
	}
}

// TestScanUpwardParentSymlink covers the Tier-2 parent-symlink note: the
// accepted candidate is returned through EvalSymlinks. Skipped on hosts
// where directory symlinks cannot be created (e.g. unprivileged Windows).
func TestScanUpwardParentSymlink(t *testing.T) {
	root := t.TempDir()
	real := mkdirTemp(t, filepath.Join(root, "real", "tickets"))
	link := filepath.Join(root, "link")
	if err := os.Symlink(filepath.Join(root, "real"), link); err != nil {
		t.Skipf("symlink unavailable on this host: %v", err)
	}
	got, ok := scanUpward(link)
	if !ok {
		t.Fatal("scanUpward through symlink parent: ok = false, want true")
	}
	if want := mustEvalSymlinks(t, real); got != want {
		t.Errorf("scanUpward through symlink parent = %q, want %q", got, want)
	}
}

// TestResolveNotResolvedHints checks the final ErrNotResolved diagnostics
// (plan T-0057 §C1-3): unquoted hints cover the flag, the env var and the
// cwd — with no "exe=" remnant of the removed exe-relative tier.
func TestResolveNotResolvedHints(t *testing.T) {
	cwd := t.TempDir()
	_, err := Resolve(map[string]string{}, cwd, "")
	if !errors.Is(err, ErrNotResolved) {
		t.Fatalf("err = %v, want errors.Is ErrNotResolved", err)
	}
	for _, hint := range []string{"--tickets-dir", "-C", "TICKETS_DIR", cwd} {
		if !strings.Contains(err.Error(), hint) {
			t.Errorf("err = %q, want hint %q", err, hint)
		}
	}
	// Plan format (unquoted): --tickets-dir=, $TICKETS_DIR=, cwd=<abs>.
	if want := fmt.Sprintf("--tickets-dir=, $TICKETS_DIR=, cwd=%s", cwd); !strings.Contains(err.Error(), want) {
		t.Errorf("err = %q, want fragment %q", err, want)
	}
	for _, bad := range []string{"exe=", fmt.Sprintf("cwd=%q", cwd)} {
		if strings.Contains(err.Error(), bad) {
			t.Errorf("err = %q must not contain %q", err, bad)
		}
	}
}

// TestIsRealDir covers the cross-platform IsRealDir contract (T-0082):
// a real directory, an absent path and a regular file. Symlink cases live
// in paths_unix_test.go (Lstat never follows the trailing link).
func TestIsRealDir(t *testing.T) {
	root := t.TempDir()
	dir := mkdirTemp(t, filepath.Join(root, "dir"))
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		want    bool
		wantErr error
	}{
		{name: "real dir", path: dir, want: true},
		{name: "absent", path: filepath.Join(root, "nope")},
		{name: "regular file", path: file, wantErr: ErrNotRealDir},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsRealDir(tt.path)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("IsRealDir(%q) err = %v, want errors.Is %v", tt.path, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("IsRealDir(%q): %v", tt.path, err)
			}
			if got != tt.want {
				t.Errorf("IsRealDir(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestVolumeRootAbstraction is host-independent: Dir(Clean(Separator)) is the
// filesystem root and carries no volume name. On Windows it additionally
// verifies the C:\ volume-root precondition (plan F7').
func TestVolumeRootAbstraction(t *testing.T) {
	root := filepath.Dir(filepath.Clean(string(filepath.Separator)))
	if vol := filepath.VolumeName(root); vol != "" {
		t.Errorf("VolumeName(%q) = %q, want empty", root, vol)
	}
	if runtime.GOOS == "windows" {
		if vol := filepath.VolumeName(`C:\`); vol != "C:" {
			t.Fatalf("VolumeName(`C:\\`) = %q, want %q", vol, "C:")
		}
	}
}
