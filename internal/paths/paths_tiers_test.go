package paths

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

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

// TestSourceTiers pins the provenance labels returned by Source: the
// --tickets-dir flag beats $TICKETS_DIR, $TICKETS_DIR beats the upward
// scan. The process environment is poisoned with a TICKETS_DIR sentinel to
// prove that Source reads only the passed map and never the process env.
func TestSourceTiers(t *testing.T) {
	t.Setenv(envTicketsDir, "/process-env-sentinel")
	cases := []struct {
		name    string
		env     map[string]string
		flagDir string
		want    string
	}{
		{"flag beats env", map[string]string{envTicketsDir: "/env"}, "/flag", "--tickets-dir"},
		{"env tier", map[string]string{envTicketsDir: "/env"}, "", "TICKETS_DIR"},
		{"scan tier", map[string]string{}, "", "scan"},
	}
	for _, c := range cases {
		if got := Source(c.env, c.flagDir); got != c.want {
			t.Errorf("%s: Source(%v, %q) = %q, want %q", c.name, c.env, c.flagDir, got, c.want)
		}
	}
}

// TestResolveSourceTierAgreement checks that Resolve and Source always pick
// the same tier: the label Source reports is the label Resolve puts into its
// diagnostics, and the scan tier is reported exactly when the upward scan is
// the tier that resolved the directory.
func TestResolveSourceTierAgreement(t *testing.T) {
	root := t.TempDir()
	tickets := mkdirTemp(t, filepath.Join(root, "tickets"))
	cwd := mkdirTemp(t, filepath.Join(root, "deep"))
	missing := filepath.Join(root, "nope")

	cases := []struct {
		name    string
		env     map[string]string
		flagDir string
	}{
		{"flag", map[string]string{}, missing},
		{"env", map[string]string{envTicketsDir: missing}, ""},
	}
	for _, c := range cases {
		src := Source(c.env, c.flagDir)
		_, err := Resolve(c.env, cwd, c.flagDir)
		if err == nil {
			t.Fatalf("%s: Resolve with a missing dir: want error", c.name)
		}
		if !strings.Contains(err.Error(), "("+src+")") {
			t.Errorf("%s: Resolve error %q does not carry the Source label (%s)", c.name, err, src)
		}
	}
	if src := Source(map[string]string{}, ""); src != srcScan {
		t.Fatalf("Source with no flag and no env = %q, want %q", src, srcScan)
	}
	got, err := Resolve(map[string]string{}, cwd, "")
	if err != nil {
		t.Fatalf("Resolve scan tier: %v", err)
	}
	if want := mustEvalSymlinks(t, tickets); got != want {
		t.Errorf("Resolve scan tier = %q, want %q", got, want)
	}
}
