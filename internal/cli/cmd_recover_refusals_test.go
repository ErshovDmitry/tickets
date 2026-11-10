package cli

// T-0159 A2: refusal, exit-code and usage-rendering tests for `ticket recover`
// (the happy paths and the sandbox helpers live in cmd_recover_test.go). Every
// test drives the real dispatch inside a t.TempDir() sandbox.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
	"ticket/internal/store"
)

func TestRecoverRefusalsUseTypedExitCodes(t *testing.T) {
	// The forged file stays a *parseable* ticket: only a foreign trailer is
	// appended, so the run reaches the divergence branch instead of failing
	// early on an unreadable H1 (T-0159 review fix).
	t.Run("divergent is exit 2", func(t *testing.T) {
		dir := t.TempDir()
		_, newPath := crashPair(t, dir)
		body, err := os.ReadFile(newPath)
		if err != nil {
			t.Fatalf("read new: %v", err)
		}
		if err := os.WriteFile(newPath, append(body, []byte("\n<!-- foreign -->\n")...), 0o644); err != nil {
			t.Fatalf("forge divergent new: %v", err)
		}
		before := treeState(t, dir)
		_, stderr, code := runRecoverCLI(t, dir, "recover", "1", "T-0001-done.md", "--yes")
		if code != exitAmbiguous {
			t.Fatalf("divergent: code=%d want %d (stderr=%q)", code, exitAmbiguous, stderr)
		}
		if !strings.Contains(stderr, "T-0001-done.md") {
			t.Errorf("divergence refusal must name the selected file, got %q", stderr)
		}
		assertSameTree(t, dir, before)
	})

	// A same-status pair is only reachable across tickets/ and archive/: the
	// file name carries the status, so two identical names are required.
	t.Run("same status is exit 2", func(t *testing.T) {
		dir := t.TempDir()
		if _, se, code := runRecoverCLI(t, dir, "new", "same status"); code != 0 {
			t.Fatalf("seed new: code=%d stderr=%q", code, se)
		}
		openPath := filepath.Join(dir, "T-0001-open.md")
		body, err := os.ReadFile(openPath)
		if err != nil {
			t.Fatalf("read open: %v", err)
		}
		archiveDir := filepath.Join(dir, "archive")
		if err := os.MkdirAll(archiveDir, 0o755); err != nil {
			t.Fatalf("mkdir archive: %v", err)
		}
		if err := os.WriteFile(filepath.Join(archiveDir, "T-0001-open.md"), body, 0o644); err != nil {
			t.Fatalf("clone into archive: %v", err)
		}
		before := treeState(t, dir)
		_, stderr, code := runRecoverCLI(t, dir, "recover", "1", "T-0001-open.md", "--yes")
		if code != exitAmbiguous {
			t.Fatalf("same status: code=%d want %d (stderr=%q)", code, exitAmbiguous, stderr)
		}
		assertSameTree(t, dir, before)
	})

	// A well-formed basename that is neither half of the pair is a plain
	// refusal, not an ambiguity.
	t.Run("selection outside the pair is exit 1", func(t *testing.T) {
		dir := t.TempDir()
		crashPair(t, dir)
		before := treeState(t, dir)
		_, stderr, code := runRecoverCLI(t, dir, "recover", "1", "T-0001-wip.md", "--yes")
		if code != 1 {
			t.Fatalf("selection not in pair: code=%d want 1 (stderr=%q)", code, stderr)
		}
		if !strings.Contains(stderr, "T-0001-wip.md") {
			t.Errorf("selection refusal must name the selected file, got %q", stderr)
		}
		assertSameTree(t, dir, before)
	})

	// One file remains and it is not the selected new file.
	t.Run("single mismatch is exit 1", func(t *testing.T) {
		dir := t.TempDir()
		if _, se, code := runRecoverCLI(t, dir, "new", "lonely"); code != 0 {
			t.Fatalf("seed new: code=%d stderr=%q", code, se)
		}
		before := treeState(t, dir)
		_, stderr, code := runRecoverCLI(t, dir, "recover", "1", "T-0001-done.md", "--yes")
		if code != 1 {
			t.Fatalf("single mismatch: code=%d want 1 (stderr=%q)", code, stderr)
		}
		assertSameTree(t, dir, before)
	})

	t.Run("keep old is exit 1", func(t *testing.T) {
		dir := t.TempDir()
		crashPair(t, dir)
		before := treeState(t, dir)
		_, stderr, code := runRecoverCLI(t, dir, "recover", "1", "T-0001-open.md", "--yes")
		if code != 1 {
			t.Fatalf("keep old: code=%d want 1 (stderr=%q)", code, stderr)
		}
		if !strings.Contains(stderr, "T-0001-done.md") {
			t.Errorf("keep-old refusal must name the file to recover first, got %q", stderr)
		}
		assertSameTree(t, dir, before)
	})

	t.Run("not found is exit 1", func(t *testing.T) {
		dir := t.TempDir()
		crashPair(t, dir)
		_, stderr, code := runRecoverCLI(t, dir, "recover", "7", "T-0007-done.md", "--yes")
		if code != 1 {
			t.Fatalf("not found: code=%d want 1 (stderr=%q)", code, stderr)
		}
	})
}

// TestRecoverSingleMismatchNamesRemainingFile pins the dedicated single-
// mismatch diagnostic end to end (--yes, both languages; a non-TTY refusal
// must not mask the state error).
func TestRecoverSingleMismatchNamesRemainingFile(t *testing.T) {
	cases := []struct{ name, lang, want string }{
		{"RU", "ru", "ticket: 1: единственный оставшийся файл T-0001-open.md не является выбранным T-0001-done.md; восстановление невозможно, ничего не изменено"},
		{"EN", "en", "ticket: 1: the only remaining file T-0001-open.md is not the selected T-0001-done.md; recovery is impossible, nothing was changed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if _, se, code := runRecoverLangCLI(t, dir, tc.lang, "new", "lonely"); code != 0 {
				t.Fatalf("seed new: code=%d stderr=%q", code, se)
			}
			before := treeState(t, dir)
			_, stderr, code := runRecoverLangCLI(t, dir, tc.lang, "recover", "1", "T-0001-done.md", "--yes")
			if code != 1 {
				t.Fatalf("single mismatch: code=%d want 1 (stderr=%q)", code, stderr)
			}
			if got := strings.TrimSpace(stderr); got != tc.want {
				t.Errorf("single mismatch stderr\nwant %q\n got %q", tc.want, got)
			}
			if tc.lang == "en" && hasCyrillic(stderr) {
				t.Errorf("EN output must not contain Cyrillic: %q", stderr)
			}
			assertSameTree(t, dir, before)
		})
	}
}

// TestReportRecoveryStateSingleMismatchUnit drives reportRecoveryState with
// synthetic errors so the safe fallbacks are pinned (Other == Keep, empty
// Other, SelectionNoMatch).
func TestReportRecoveryStateSingleMismatchUnit(t *testing.T) {
	mismatch := func(o string) *store.RecoveryStateError {
		return &store.RecoveryStateError{Number: 7, Keep: "T-0007-done.md", Other: o, Reason: store.RecoverReasonSingleMismatch}
	}
	cases := []struct {
		name   string
		rse    *store.RecoveryStateError
		single bool // expect singleMismatchFmt, not selectionFmt
	}{
		{"foreign other", mismatch("T-0099-open.md"), true},
		{"other equals keep", mismatch("T-0007-done.md"), false},
		{"empty other", mismatch(""), false},
		{"selection no match", &store.RecoveryStateError{Number: 7, Keep: "T-0007-wip.md", Reason: store.RecoverReasonSelectionNoMatch}, false},
	}
	for _, tc := range cases {
		named := tc.rse.Keep // file the message must name (Other when single)
		if tc.single {
			named = tc.rse.Other
		}
		for lang, ph := range map[domain.Lang][3]string{
			domain.LangRU: {"ticket: 7: файл %s не входит в пару конфликтующих файлов; ничего не изменено",
				"ticket: 7: единственный оставшийся файл %s не является выбранным %s; восстановление невозможно, ничего не изменено", "единственный оставшийся"},
			domain.LangEN: {"ticket: 7: %s is not part of the conflicting pair; nothing was changed",
				"ticket: 7: the only remaining file %s is not the selected %s; recovery is impossible, nothing was changed", "only remaining"},
		} {
			t.Run(tc.name+"/"+string(lang), func(t *testing.T) {
				var stderr bytes.Buffer
				if code := reportRecoveryState(&stderr, lang, tc.rse); code != exitError {
					t.Fatalf("code=%d want %d (stderr=%q)", code, exitError, stderr.String())
				}
				got := strings.TrimSpace(stderr.String())
				if tc.single {
					want := fmt.Sprintf(ph[1], named, tc.rse.Keep)
					if got != want {
						t.Errorf("stderr\nwant %q\n got %q", want, got)
					}
				} else if want := fmt.Sprintf(ph[0], named); got != want {
					t.Errorf("stderr\nwant %q\n got %q", want, got)
				} else {
					for _, a := range []string{ph[2], "%!"} {
						if strings.Contains(got, a) {
							t.Errorf("fallback stderr must not contain %q, got %q", a, got)
						}
					}
				}
			})
		}
	}
}

// runRecoverLangCLI is runRecoverCLI with an explicit ticket language.
func runRecoverLangCLI(t *testing.T, dir, lang string, args ...string) (string, string, int) {
	t.Helper()
	env := recoverEnv(dir)
	env["TICKET_LANG"] = lang
	var so, se bytes.Buffer
	code := Run(args, env, &so, &se)
	return so.String(), se.String(), code
}

// hasCyrillic reports whether s carries any Cyrillic rune (an EN run must
// never leak a Russian literal).
func hasCyrillic(s string) bool {
	for _, r := range s {
		if r >= 0x0400 && r <= 0x04FF {
			return true
		}
	}
	return false
}

func TestRecoverUsageAndHelpBothLanguages(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		lang    string
		synopsy string
	}{
		{"RU", "ru", "ticket recover <номер> <файл-новый> [--yes]"},
		{"EN", "en", "ticket recover <number> <new-file> [--yes]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runRecoverLangCLI(t, dir, tc.lang, "help")
			if code != 0 {
				t.Fatalf("help: code=%d", code)
			}
			if !strings.Contains(stdout, tc.synopsy) {
				t.Errorf("help must document the recover synopsis %q", tc.synopsy)
			}
			if tc.lang == "en" && (hasCyrillic(stdout) || hasCyrillic(stderr)) {
				t.Error("EN help must not contain Cyrillic")
			}
		})
	}
}

func TestRecoverUsageErrors(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		{"recover"},
		{"recover", "1"},
		{"recover", "1", "T-0001-done.md", "--nope"},
		{"recover", "1", "T-0001-done.md", "--yes", "extra"},
	} {
		stdout, stderr, code := runRecoverCLI(t, dir, args...)
		if code != 1 {
			t.Errorf("%v: code=%d want 1 (stderr=%q)", args, code, stderr)
		}
		// A usage error prints the help to stdout, so the caller sees the
		// accepted recover synopsis instead of a bare "bad arguments".
		if !strings.Contains(stdout, "ticket recover") {
			t.Errorf("%v: usage error must print the recover help to stdout, got %q", args, stdout)
		}
	}
	// The subcommand help interception wins over the argument count.
	if _, stderr, code := runRecoverCLI(t, dir, "recover", "-h"); code != 0 {
		t.Errorf("recover -h: code=%d want 0 (stderr=%q)", code, stderr)
	}
}

func TestRecoverMoreThanTwoFilesIsExitAmbiguous(t *testing.T) {
	dir := t.TempDir()
	oldPath, newPath := crashPair(t, dir)
	thirdPath := filepath.Join(dir, "T-0001-wip.md")
	if err := os.WriteFile(thirdPath, []byte("# third\n"), 0o644); err != nil {
		t.Fatalf("seed third file: %v", err)
	}
	before := treeState(t, dir)
	_, stderr, code := runRecoverCLI(t, dir, "recover", "1", "T-0001-done.md", "--yes")
	if code != exitAmbiguous {
		t.Fatalf("three files: code=%d want %d (stderr=%q)", code, exitAmbiguous, stderr)
	}
	// The diagnostics must list every conflicting file, so the caller can
	// pick the right basename without guessing.
	for _, want := range []string{oldPath, newPath, thirdPath} {
		if !strings.Contains(stderr, want) {
			t.Errorf("three-file refusal must name %q, got %q", want, stderr)
		}
	}
	assertSameTree(t, dir, before)
}
