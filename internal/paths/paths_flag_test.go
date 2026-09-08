package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveFlagDir covers the --tickets-dir/-C tier: the same validation
// chain as $TICKETS_DIR but with the (--tickets-dir) source label. The
// relative case proves filepath.Abs resolves the flag value against the
// PROCESS cwd: the unique name exists neither there nor under the injected
// cwd argument, so ErrInvalidDir + "path does not exist" is stable
// (os.Chdir is NOT used — it would break parallel tests).
func TestResolveFlagDir(t *testing.T) {
	root := t.TempDir()
	tickets := mkdirTemp(t, filepath.Join(root, "tickets"))
	file := filepath.Join(root, "plain.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	relative := fmt.Sprintf("zzz-no-such-%d", os.Getpid())

	tests := []struct {
		name    string
		flagDir string
		want    string
		wantErr error
		hint    string
	}{
		{name: "existing dir", flagDir: tickets, want: mustEvalSymlinks(t, tickets)},
		{name: "missing path", flagDir: filepath.Join(root, "nope"), wantErr: ErrInvalidDir,
			hint: "path does not exist: " + mustAbs(t, filepath.Join(root, "nope"))},
		{name: "regular file", flagDir: file, wantErr: ErrInvalidDir,
			hint: "not a directory: " + mustEvalSymlinks(t, file)},
		{name: "relative path", flagDir: relative, wantErr: ErrInvalidDir, hint: "path does not exist"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(map[string]string{}, t.TempDir(), tt.flagDir)
			if tt.wantErr != nil {
				assertInvalidDir(t, err, srcFlag, tt.hint)
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

// TestResolvePrecedence pins the tier order flag > $TICKETS_DIR > upward
// scan (T-0057 hub decision #2). scanT sits at <root>/tickets, cwd deep
// inside the tree so the scan would find it if asked.
func TestResolvePrecedence(t *testing.T) {
	root := t.TempDir()
	flagT := mkdirTemp(t, filepath.Join(root, "flag"))
	envT := mkdirTemp(t, filepath.Join(root, "env"))
	scanT := mkdirTemp(t, filepath.Join(root, "tickets"))
	cwd := mkdirTemp(t, filepath.Join(root, "deep", "nested"))

	tests := []struct {
		name    string
		flagDir string
		env     map[string]string
		cwd     string
		want    string
		wantErr error
	}{
		{"flag beats env and scan", flagT, map[string]string{envTicketsDir: envT}, cwd, mustEvalSymlinks(t, flagT), nil},
		{"env beats scan", "", map[string]string{envTicketsDir: envT}, cwd, mustEvalSymlinks(t, envT), nil},
		{"scan when nothing set", "", map[string]string{}, cwd, mustEvalSymlinks(t, scanT), nil},
		{"nothing found", "", map[string]string{}, t.TempDir(), "", ErrNotResolved},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.env, tt.cwd, tt.flagDir)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want errors.Is %v", err, tt.wantErr)
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

// TestResolveRejectsNonTicketsDir covers the fail-fast marker check (T-0069
// D1): a dir holding only stray files must be rejected with ErrInvalidDir +
// the "not a tickets directory" hint, while marker-bearing dirs still
// resolve: empty (bootstrap), .lock, archive/ and a single ticket file.
// Hidden (write-probe) and *.tmp entries are transient artifacts of parallel
// processes and must not make an otherwise-empty dir look foreign.
func TestResolveRejectsNonTicketsDir(t *testing.T) {
	root := t.TempDir()
	stray := mkdirTemp(t, filepath.Join(root, "stray"))
	for _, name := range []string{"readme.txt", "notes.md"} {
		if err := os.WriteFile(filepath.Join(stray, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}

	_, err := Resolve(map[string]string{}, t.TempDir(), stray)
	assertInvalidDir(t, err, srcFlag, "not a tickets directory")

	locked := mkdirTemp(t, filepath.Join(root, "locked"))
	if err := os.WriteFile(filepath.Join(locked, ".lock"), []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile(.lock): %v", err)
	}
	archived := mkdirTemp(t, filepath.Join(root, "archived"))
	mkdirTemp(t, filepath.Join(archived, "archive"))
	withTicket := mkdirTemp(t, filepath.Join(root, "with-ticket"))
	if err := os.WriteFile(filepath.Join(withTicket, "T-0001-open.md"), []byte("# T-0001\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(T-0001-open.md): %v", err)
	}
	probe := mkdirTemp(t, filepath.Join(root, "probe"))
	if err := os.WriteFile(filepath.Join(probe, ".write-probe.123-456"), []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile(.write-probe.123-456): %v", err)
	}
	tmpfile := mkdirTemp(t, filepath.Join(root, "tmpfile"))
	if err := os.WriteFile(filepath.Join(tmpfile, "foo.tmp"), []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile(foo.tmp): %v", err)
	}

	oks := []struct {
		name string
		dir  string
	}{
		{"empty dir bootstraps", t.TempDir()},
		{"dir with .lock", locked},
		{"dir with archive/", archived},
		{"dir with ticket file", withTicket},
		{"dir with only write-probe file", probe},
		{"dir with only .tmp file", tmpfile},
	}
	for _, tt := range oks {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(map[string]string{}, t.TempDir(), tt.dir)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if want := mustEvalSymlinks(t, tt.dir); got != want {
				t.Errorf("Resolve = %q, want %q", got, want)
			}
		})
	}
}

// TestResolveStatNoDoublePrefix covers T-0069 D2: a stat failure is wrapped
// WITHOUT the extra "stat %s:" prefix — os.Stat's *os.PathError already
// renders "stat <path>: <cause>", so the message must contain exactly one
// "stat". A self-referential symlink forces os.Stat to fail with ELOOP.
// Skipped on hosts where symlinks cannot be created (unprivileged Windows).
func TestResolveStatNoDoublePrefix(t *testing.T) {
	loop := filepath.Join(t.TempDir(), "loop")
	if err := os.Symlink(loop, loop); err != nil {
		t.Skipf("symlink unavailable on this host: %v", err)
	}
	_, err := Resolve(map[string]string{}, t.TempDir(), loop)
	if !errors.Is(err, ErrInvalidDir) {
		t.Fatalf("err = %v, want errors.Is ErrInvalidDir", err)
	}
	if n := strings.Count(err.Error(), "stat"); n != 1 {
		t.Errorf("err = %q, want exactly 1 occurrence of \"stat\", got %d", err, n)
	}
}
