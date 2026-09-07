package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
