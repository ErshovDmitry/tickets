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

// TestResolveEnv covers Tier 1: the $TICKETS_DIR override, including
// ErrEnvNotDir discrimination for missing paths and regular files.
// Assumption: no ancestor of t.TempDir() contains a directory named
// "tickets" (relevant for the empty-env fall-through rows).
func TestResolveEnv(t *testing.T) {
	root := t.TempDir()
	tickets := filepath.Join(root, "tickets")
	mkdirTemp(t, tickets)
	want := mustEvalSymlinks(t, tickets)

	file := filepath.Join(root, "plain.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tests := []struct {
		name    string
		env     string
		cwd     string
		want    string
		wantErr error
		hint    string
	}{
		{
			name: "existing dir",
			env:  tickets,
			cwd:  t.TempDir(),
			want: want,
		},
		{
			name:    "missing path",
			env:     filepath.Join(root, "nope"),
			cwd:     t.TempDir(),
			wantErr: ErrEnvNotDir,
			hint:    "path does not exist: " + mustAbs(t, filepath.Join(root, "nope")),
		},
		{
			name:    "regular file",
			env:     file,
			cwd:     t.TempDir(),
			wantErr: ErrEnvNotDir,
			hint:    "not a directory: " + mustEvalSymlinks(t, file),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(map[string]string{envTicketsDir: tt.env}, tt.cwd, "")
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want errors.Is %v", err, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.hint) {
					t.Errorf("err = %q, want hint %q", err, tt.hint)
				}
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
// fall-through to the Tier-2 upward scan.
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

// TestResolveExeRelative covers Tier 3: the tickets/bin/ticket layout,
// ErrNoExePath for an empty exe path, and ErrNotResolved for failures.
func TestResolveExeRelative(t *testing.T) {
	root := t.TempDir()
	tickets := mkdirTemp(t, filepath.Join(root, "tickets"))
	bin := mkdirTemp(t, filepath.Join(tickets, "bin"))
	exe := filepath.Join(bin, "ticket")
	if err := os.WriteFile(exe, []byte{0x7f}, 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	want := filepath.Dir(filepath.Dir(mustEvalSymlinks(t, exe)))

	bareRoot := t.TempDir()
	strayExe := filepath.Join(mkdirTemp(t, filepath.Join(bareRoot, "other")), "ticket")
	if err := os.WriteFile(strayExe, []byte{0x7f}, 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	tests := []struct {
		name    string
		exePath string
		want    string
		wantErr error
		hint    string
	}{
		{
			name:    "bin layout",
			exePath: exe,
			want:    want,
		},
		{
			name:    "empty exe path",
			exePath: "",
			wantErr: ErrNoExePath,
		},
		{
			name:    "missing exe",
			exePath: filepath.Join(bareRoot, "ghost"),
			wantErr: ErrNotResolved,
			hint:    filepath.Join(bareRoot, "ghost"),
		},
		{
			name:    "bin dot-dot is not tickets",
			exePath: strayExe,
			wantErr: ErrNotResolved,
			hint:    strayExe,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// cwd without any tickets ancestor: Tier 2 must miss so the
			// exe tier is reached (see TestScanUpward assumption).
			got, err := Resolve(map[string]string{}, t.TempDir(), tt.exePath)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want errors.Is %v", err, tt.wantErr)
				}
				if tt.hint != "" && !strings.Contains(err.Error(), tt.hint) {
					t.Errorf("err = %q, want hint %q", err, tt.hint)
				}
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

// TestResolveNotResolvedHints checks the diagnostic message of the final
// ErrNotResolved error. Plan §4 step 5: hints are unquoted,
// "$TICKETS_DIR=, cwd=<abs>, exe=<abs>".
func TestResolveNotResolvedHints(t *testing.T) {
	cwd := t.TempDir()
	_, err := Resolve(map[string]string{}, cwd, "some-exe")
	if !errors.Is(err, ErrNotResolved) {
		t.Fatalf("err = %v, want errors.Is ErrNotResolved", err)
	}
	for _, hint := range []string{"set TICKETS_DIR", cwd, "some-exe"} {
		if !strings.Contains(err.Error(), hint) {
			t.Errorf("err = %q, want hint %q", err, hint)
		}
	}
	// Plan format (unquoted): $TICKETS_DIR=, cwd=<abs>, exe=<abs>.
	wantFragment := fmt.Sprintf("$TICKETS_DIR=, cwd=%s, exe=some-exe", cwd)
	if !strings.Contains(err.Error(), wantFragment) {
		t.Errorf("err = %q, want fragment %q", err, wantFragment)
	}
	// No quoted forms (plan §4 step 5 unquoted).
	for _, bad := range []string{
		fmt.Sprintf(`$TICKETS_DIR=%q`, ""),
		fmt.Sprintf(`cwd=%q`, cwd),
		`exe="some-exe"`,
	} {
		if strings.Contains(err.Error(), bad) {
			t.Errorf("err = %q must not contain quoted fragment %q", err, bad)
		}
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
