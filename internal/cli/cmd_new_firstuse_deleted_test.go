package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/cli"
)

// ruFirstUseMarker / enFirstUseMarker are the language-specific fragments of
// domain.WarnNewProject, used to detect whether the first-use warning fired.
const (
	ruFirstUseMarker = "впервые"
	enFirstUseMarker = "is seen for the first time"
)

// newRun is one `ticket new` invocation: the created ticket path (stdout),
// the diagnostic stream (stderr), and the process exit code.
type newRun struct {
	stdout string
	stderr string
	code   int
}

// runNewProject runs `ticket new` in env. The mustNew* wrappers below are
// the exit-0-asserting variants.
func runNewProject(t *testing.T, env map[string]string, args ...string) newRun {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run(args, env, &stdout, &stderr)
	return newRun{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

// mustNewOK runs `ticket new` and fails the test unless it exits 0.
func mustNewOK(t *testing.T, env map[string]string, args ...string) {
	t.Helper()
	res := runNewProject(t, env, args...)
	if res.code != 0 {
		t.Fatalf("ticket %v = %d, want 0; stderr: %q", args, res.code, res.stderr)
	}
}

// mustNewStdout runs `ticket new` (exit 0 required) and returns the created
// ticket path printed on stdout.
func mustNewStdout(t *testing.T, env map[string]string, args ...string) string {
	t.Helper()
	res := runNewProject(t, env, args...)
	if res.code != 0 {
		t.Fatalf("ticket %v = %d, want 0; stderr: %q", args, res.code, res.stderr)
	}
	return res.stdout
}

// mustNewStderr runs `ticket new` (exit 0 required) and returns stderr, where
// the first-use warning and the destination line are printed.
func mustNewStderr(t *testing.T, env map[string]string, args ...string) string {
	t.Helper()
	res := runNewProject(t, env, args...)
	if res.code != 0 {
		t.Fatalf("ticket %v = %d, want 0; stderr: %q", args, res.code, res.stderr)
	}
	return res.stderr
}

// moveToDeleted moves the ticket file created at createdPath into the store's
// deleted directory under its "deleted" name. The filename status is the
// authoritative one (V19), so the moved ticket parses as StatusDeleted.
func moveToDeleted(t *testing.T, createdPath string) {
	t.Helper()
	dir := filepath.Dir(createdPath)
	deletedDir := filepath.Join(dir, "deleted")
	if err := os.MkdirAll(deletedDir, 0o755); err != nil {
		t.Fatalf("mkdir deleted: %v", err)
	}
	name := strings.TrimSuffix(filepath.Base(createdPath), "-open.md") + "-deleted.md"
	if err := os.Rename(createdPath, filepath.Join(deletedDir, name)); err != nil {
		t.Fatalf("move %s to deleted: %v", createdPath, err)
	}
}

// seedProjectOnlyInDeleted creates one ticket for project and moves it into
// tickets/deleted/, leaving no ticket for that project in the main directory.
func seedProjectOnlyInDeleted(t *testing.T, env map[string]string, dir, project string) {
	t.Helper()
	moveToDeleted(t, strings.TrimSpace(mustNewStdout(t, env, "new", "Seeded", "-P", project)))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "-open.md") {
			t.Fatalf("main dir still holds %s after seeding deleted/", e.Name())
		}
	}
}

// TestFirstUseProjectOnlyInDeleted_Silent pins AC1/AC3 (T-0189, GitHub #22):
// a project whose tickets live only under tickets/deleted/ is a known project,
// so `new` stays silent and the ticket is still created (exit 0).
func TestFirstUseProjectOnlyInDeleted_Silent(t *testing.T) {
	dir := mustEval(t, t.TempDir())
	env := map[string]string{"TICKETS_DIR": dir}
	seedProjectOnlyInDeleted(t, env, dir, "gone-project")

	res := runNewProject(t, env, "new", "Follow up", "-P", "gone-project")
	if res.code != 0 {
		t.Fatalf("new -P gone-project = %d, want 0; stderr: %q", res.code, res.stderr)
	}
	if strings.Contains(res.stderr, ruFirstUseMarker) {
		t.Errorf("first-use warning printed for deleted-only project: %q", res.stderr)
	}
	if res.stderr != "" {
		t.Errorf("unexpected stderr output: %q", res.stderr)
	}
	info, err := os.Stat(strings.TrimSpace(res.stdout))
	if err != nil || info.Size() == 0 {
		t.Errorf("ticket not created: %v", err)
	}
}

// TestFirstUseProjectInMainAndDeleted_Silent pins that the deleted scan does
// not disturb the existing main-directory behaviour: a repeat project stays
// silent even when a copy also lives in deleted/.
func TestFirstUseProjectInMainAndDeleted_Silent(t *testing.T) {
	dir := mustEval(t, t.TempDir())
	env := map[string]string{"TICKETS_DIR": dir}

	firstStderr := mustNewStderr(t, env, "new", "First", "-P", "shared")
	if !strings.Contains(firstStderr, ruFirstUseMarker) {
		t.Fatalf("first ticket stderr missing first-use warning: %q", firstStderr)
	}
	moveToDeleted(t, strings.TrimSpace(mustNewStdout(t, env, "new", "Second", "-P", "shared")))

	thirdStderr := mustNewStderr(t, env, "new", "Third", "-P", "shared")
	if strings.Contains(thirdStderr, ruFirstUseMarker) {
		t.Errorf("warning printed for project in main+deleted: %q", thirdStderr)
	}
}

// TestFirstUseDeletedOtherProject_Warns pins the negative control: an existing
// deleted-only project must not suppress the warning for a genuinely unseen
// project name, and the warning must not block creation (exit 0, AC2).
func TestFirstUseDeletedOtherProject_Warns(t *testing.T) {
	dir := mustEval(t, t.TempDir())
	env := map[string]string{"TICKETS_DIR": dir}
	seedProjectOnlyInDeleted(t, env, dir, "alpha")

	res := runNewProject(t, env, "new", "Brand new", "-P", "beta")
	if res.code != 0 {
		t.Fatalf("new -P beta = %d, want 0 (warning must not block); stderr: %q", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, ruFirstUseMarker) || !strings.Contains(res.stderr, "beta") {
		t.Errorf("missing first-use warning for 'beta': %q", res.stderr)
	}
	info, err := os.Stat(strings.TrimSpace(res.stdout))
	if err != nil || info.Size() == 0 {
		t.Errorf("ticket not created despite warning: %v", err)
	}
}

// TestFirstUseEmptyDeletedDir_WarnsUnchanged pins AC3: an existing but empty
// deleted/ adds no new errors/warnings, and a new project still warns.
func TestFirstUseEmptyDeletedDir_WarnsUnchanged(t *testing.T) {
	dir := mustEval(t, t.TempDir())
	env := map[string]string{"TICKETS_DIR": dir}
	// A populated store with an EMPTY deleted/ (the store must look like a
	// tickets dir: a bare deleted/ does not qualify as one).
	mustNewOK(t, env, "new", "Seed", "-P", "alpha")
	if err := os.Mkdir(filepath.Join(dir, "deleted"), 0o755); err != nil {
		t.Fatalf("mkdir deleted: %v", err)
	}

	stderr := mustNewStderr(t, env, "new", "Solo", "-P", "brandnew")
	if !strings.Contains(stderr, ruFirstUseMarker) {
		t.Errorf("missing first-use warning with empty deleted/: %q", stderr)
	}
	for _, bad := range []string{"store:", "read dir"} {
		if strings.Contains(stderr, bad) {
			t.Errorf("empty deleted/ produced %q: %q", bad, stderr)
		}
	}
}

// TestFirstUseDeletedENParity pins that the change keeps the EN dictionary
// behaviour identical: deleted-only project silent, unseen project warns.
func TestFirstUseDeletedENParity(t *testing.T) {
	dir := mustEval(t, t.TempDir())
	env := map[string]string{"TICKETS_DIR": dir, "TICKET_LANG": "en"}
	seedProjectOnlyInDeleted(t, env, dir, "known-en")

	silentStderr := mustNewStderr(t, env, "new", "Again", "-P", "known-en")
	if strings.Contains(silentStderr, enFirstUseMarker) {
		t.Errorf("EN first-use warning printed for deleted-only project: %q", silentStderr)
	}
	warnStderr := mustNewStderr(t, env, "new", "Fresh", "-P", "unseen-en")
	if !strings.Contains(warnStderr, enFirstUseMarker) || !strings.Contains(warnStderr, "unseen-en") {
		t.Errorf("EN first-use warning missing for 'unseen-en': %q", warnStderr)
	}
}
