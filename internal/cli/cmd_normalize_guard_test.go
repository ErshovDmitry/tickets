package cli_test

// CLI-level coverage of the normalize guard variants (T-0125): a skipped file
// leaves a warning on stderr, keeps its bytes and does NOT change the exit
// code — a guard is a skip, not a failure. The three variants are the ones a
// hand-edited or pre-tool file can carry and Render cannot reproduce.

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// insertLabelAfterProject puts label on its own line right after the project
// metadata line of a ticket body, so the fixture is a valid ticket plus one
// line the renderer cannot reproduce.
func insertLabelAfterProject(t *testing.T, body []byte, label string) []byte {
	t.Helper()
	lines := strings.Split(string(body), "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "- Project") {
			continue
		}
		out := make([]string, 0, len(lines)+1)
		out = append(out, lines[:i+1]...)
		out = append(out, label)
		return []byte(strings.Join(append(out, lines[i+1:]...), "\n"))
	}
	t.Fatal("fixture has no project line")
	return nil
}

// TestCmdNormalize_LegacyLabelVariantsExitZero pins the three guard shapes:
// an indented marker, a "*" bullet and a space before the colon are all
// reported as "bash-era metadata labels", every file stays byte-identical,
// stdout stays empty and the command exits 0.
func TestCmdNormalize_LegacyLabelVariantsExitZero(t *testing.T) {
	for _, tc := range []struct{ name, label string }{
		{"leading-space", " - Статус: open"},
		{"star-bullet", "* Статус: open"},
		{"space-before-colon", "- Статус : open"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := normalizeEnv(t)
			path := seedNormalizeTicket(t, env, "guard variant")
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			fixture := insertLabelAfterProject(t, body, tc.label)
			if err := os.WriteFile(path, fixture, 0o644); err != nil {
				t.Fatalf("write %s: %v", path, err)
			}

			stdout, stderr, code := runNormalizeCLI(t, []string{"normalize"}, env)
			if code != 0 {
				t.Fatalf("normalize = %d, want 0; stderr: %q", code, stderr)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want empty (nothing rewritten)", stdout)
			}
			if !strings.Contains(stderr, "bash-era metadata labels") {
				t.Errorf("stderr = %q, want the guard warning", stderr)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			if !bytes.Equal(after, fixture) {
				t.Errorf("guarded file was rewritten")
			}
		})
	}
}
