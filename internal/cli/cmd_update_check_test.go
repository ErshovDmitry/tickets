package cli

// Hermetic tests for the update-check command (T-0173). Both seams
// (defaultOptionsFn, checkReleasesFn) are stubbed in every test: no test
// touches the network or the real user cache dir.

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"ticket/internal/domain"
	"ticket/internal/releasecheck"
)

// stubUpdateCheckSeams swaps the update-check seams for one test and returns
// a call counter for checkReleasesFn. optsErr models the DefaultOptions
// failure (e.g. an unresolvable user cache dir).
func stubUpdateCheckSeams(t *testing.T, optsErr error, res releasecheck.Result) *int {
	t.Helper()
	calls := 0
	prevOpts, prevCheck := defaultOptionsFn, checkReleasesFn
	defaultOptionsFn = func(string) (releasecheck.Options, error) {
		return releasecheck.Options{}, optsErr
	}
	checkReleasesFn = func(releasecheck.Options) releasecheck.Result {
		calls++
		return res
	}
	t.Cleanup(func() { defaultOptionsFn, checkReleasesFn = prevOpts, prevCheck })
	return &calls
}

// TestCmdUpdateCheckStates drives the handler directly through all five
// releasecheck states in BOTH languages: exit code, exact stdout line and
// the stream it lands on (failures go to stderr via ErrUpdateCheck).
func TestCmdUpdateCheckStates(t *testing.T) {
	cases := []struct {
		name     string
		lang     domain.Lang
		res      releasecheck.Result
		wantCode int
		wantOut  string // expected stdout line; empty means stdout stays empty
		wantErr  string // expected stderr line; empty means stderr stays empty
	}{
		{"up to date en", domain.LangEN, releasecheck.Result{State: releasecheck.UpToDate}, exitOK, "ticket dev is up to date", ""},
		{"up to date ru", domain.LangRU, releasecheck.Result{State: releasecheck.UpToDate}, exitOK, "ticket dev — актуальная версия", ""},
		{"no releases en", domain.LangEN, releasecheck.Result{State: releasecheck.NoReleases}, exitOK, "no releases published yet", ""},
		{"no releases ru", domain.LangRU, releasecheck.Result{State: releasecheck.NoReleases}, exitOK, "релизы ещё не публиковались", ""},
		{"dev en", domain.LangEN, releasecheck.Result{State: releasecheck.Dev}, exitOK, "development build (dev): release comparison skipped", ""},
		{"dev ru", domain.LangRU, releasecheck.Result{State: releasecheck.Dev}, exitOK, "сборка разработки (dev): сравнение версий пропущено", ""},
		{"available en", domain.LangEN, releasecheck.Result{State: releasecheck.UpdateAvailable, Latest: "9.9.9", URL: "https://example.com/r/9.9.9"}, exitUpdateAvailable, "new release 9.9.9 available: https://example.com/r/9.9.9", ""},
		{"available ru", domain.LangRU, releasecheck.Result{State: releasecheck.UpdateAvailable, Latest: "9.9.9", URL: "https://example.com/r/9.9.9"}, exitUpdateAvailable, "доступен новый релиз 9.9.9: https://example.com/r/9.9.9", ""},
		{"failed en", domain.LangEN, releasecheck.Result{State: releasecheck.Failed, Err: errors.New("boom")}, exitError, "", "ticket: update check failed: boom"},
		{"failed ru", domain.LangRU, releasecheck.Result{State: releasecheck.Failed, Err: errors.New("boom")}, exitError, "", "ticket: не удалось проверить обновления: boom"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := stubUpdateCheckSeams(t, nil, tc.res)
			var stdout, stderr bytes.Buffer
			code := cmdUpdateCheck(tc.lang, version, &stdout, &stderr)
			if code != tc.wantCode {
				t.Fatalf("cmdUpdateCheck = %d, want %d", code, tc.wantCode)
			}
			wantOut, wantErr := tc.wantOut, tc.wantErr
			if wantOut != "" {
				wantOut += "\n"
			}
			if wantErr != "" {
				wantErr += "\n"
			}
			if got := stdout.String(); got != wantOut {
				t.Errorf("stdout = %q, want %q", got, wantOut)
			}
			if got := stderr.String(); got != wantErr {
				t.Errorf("stderr = %q, want %q", got, wantErr)
			}
			if *calls != 1 {
				t.Errorf("checkReleasesFn called %d times, want 1", *calls)
			}
		})
	}
}

// TestCmdUpdateCheckDefaultOptionsError covers the cache-dir failure at the
// handler level: the localized error goes to stderr, the exit code is 1 and
// the check itself never runs.
func TestCmdUpdateCheckDefaultOptionsError(t *testing.T) {
	cases := []struct {
		lang    domain.Lang
		wantErr string
	}{
		{domain.LangEN, "ticket: update check failed: no cache dir"},
		{domain.LangRU, "ticket: не удалось проверить обновления: no cache dir"},
	}
	for _, tc := range cases {
		t.Run(string(tc.lang), func(t *testing.T) {
			calls := stubUpdateCheckSeams(t, errors.New("no cache dir"), releasecheck.Result{})
			var stdout, stderr bytes.Buffer
			code := cmdUpdateCheck(tc.lang, version, &stdout, &stderr)
			if code != exitError {
				t.Fatalf("cmdUpdateCheck = %d, want %d", code, exitError)
			}
			if got, want := stderr.String(), tc.wantErr+"\n"; got != want {
				t.Errorf("stderr = %q, want %q", got, want)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			if *calls != 0 {
				t.Errorf("checkReleasesFn called %d times, want 0", *calls)
			}
		})
	}
}

// TestRunUpdateCheckNoTicketsDir proves the routing contract (T-0173):
// update-check works in a directory with no tickets/ and never resolves a
// tickets dir or opens a store — even with TICKETS_DIR pointing anywhere and
// the global flags in front. Version defaults to "dev" in tests.
func TestRunUpdateCheckNoTicketsDir(t *testing.T) {
	cases := []struct {
		name string
		args []string
		env  map[string]string
		want string
	}{
		{"bare ru", []string{"update-check"}, nil, "ticket dev — актуальная версия"},
		{"tickets dir anywhere", []string{"update-check"}, map[string]string{"TICKETS_DIR": t.TempDir()}, "ticket dev — актуальная версия"},
		{"global flags inert", []string{"-C", t.TempDir(), "--verbose", "update-check"}, nil, "ticket dev — актуальная версия"},
		{"en", []string{"update-check"}, map[string]string{"TICKET_LANG": "en"}, "ticket dev is up to date"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := stubUpdateCheckSeams(t, nil, releasecheck.Result{State: releasecheck.UpToDate})
			var stdout, stderr bytes.Buffer
			code := Run(tc.args, tc.env, &stdout, &stderr)
			if code != exitOK {
				t.Fatalf("Run(%q) = %d, want 0; stderr: %q", tc.args, code, stderr.String())
			}
			if got := stdout.String(); got != tc.want+"\n" {
				t.Errorf("stdout = %q, want %q", got, tc.want)
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty", stderr.String())
			}
			if *calls != 1 {
				t.Errorf("checkReleasesFn called %d times, want 1", *calls)
			}
		})
	}
}

// TestRunUpdateCheckExitThreeRU pins the success-with-code-3 contract in the
// default language: a newer release prints version + URL to stdout.
func TestRunUpdateCheckExitThreeRU(t *testing.T) {
	res := releasecheck.Result{State: releasecheck.UpdateAvailable, Latest: "9.9.9", URL: "https://example.com/r/9.9.9"}
	calls := stubUpdateCheckSeams(t, nil, res)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"update-check"}, nil, &stdout, &stderr)
	if code != exitUpdateAvailable {
		t.Fatalf("Run(update-check) = %d, want %d (exitUpdateAvailable)", code, exitUpdateAvailable)
	}
	if got, want := stdout.String(), "доступен новый релиз 9.9.9: https://example.com/r/9.9.9\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
	if *calls != 1 {
		t.Errorf("checkReleasesFn called %d times, want 1", *calls)
	}
}

// TestRunUpdateCheckFailedEN proves the failure contract in EN: the cause
// lands on stderr prefixed with the localized "ticket: update check failed:".
func TestRunUpdateCheckFailedEN(t *testing.T) {
	res := releasecheck.Result{State: releasecheck.Failed, Err: errors.New("boom")}
	calls := stubUpdateCheckSeams(t, nil, res)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"update-check"}, map[string]string{"TICKET_LANG": "en"}, &stdout, &stderr)
	if code != exitError {
		t.Fatalf("Run(update-check) = %d, want 1", code)
	}
	if got, want := stderr.String(), "ticket: update check failed: boom\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if *calls != 1 {
		t.Errorf("checkReleasesFn called %d times, want 1", *calls)
	}
}

// TestRunUpdateCheckHelpAndExtraArgs mirrors the init shape (T-0041): -h
// prints usage and exits 0, a stray argument prints usage and exits 1, and
// neither path runs the release check.
func TestRunUpdateCheckHelpAndExtraArgs(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantCode int
	}{
		{"-h", []string{"update-check", "-h"}, exitOK},
		{"--help", []string{"update-check", "--help"}, exitOK},
		{"extra arg", []string{"update-check", "garbage"}, exitError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := stubUpdateCheckSeams(t, nil, releasecheck.Result{State: releasecheck.UpToDate})
			var stdout, stderr bytes.Buffer
			code := Run(tc.args, nil, &stdout, &stderr)
			if code != tc.wantCode {
				t.Fatalf("Run(%q) = %d, want %d", tc.args, code, tc.wantCode)
			}
			if !strings.HasPrefix(stdout.String(), "ticket version dev\n") {
				t.Errorf("stdout = %q, want the version line followed by usage", stdout.String())
			}
			if !strings.Contains(stdout.String(), "ticket update-check") {
				t.Errorf("usage text has no update-check block: %q", stdout.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty", stderr.String())
			}
			if *calls != 0 {
				t.Errorf("checkReleasesFn called %d times, want 0", *calls)
			}
		})
	}
}

// TestRunUpdateCheckOrdinaryCommandsDoNotCheck is the no-network proof for
// the rest of the CLI: with the check stub ARMED, ordinary commands
// (new/list/show/set) run their store paths and never call the release check.
func TestRunUpdateCheckOrdinaryCommandsDoNotCheck(t *testing.T) {
	calls := stubUpdateCheckSeams(t, nil, releasecheck.Result{State: releasecheck.UpToDate})
	env := map[string]string{"TICKETS_DIR": t.TempDir()}
	steps := [][]string{
		{"new", "тикет для проверки маршрутизации"},
		{"list"},
		{"show", "1"},
		{"set", "1", "done", "проверка завершена"},
	}
	for _, args := range steps {
		var stdout, stderr bytes.Buffer
		if code := Run(args, env, &stdout, &stderr); code != exitOK {
			t.Fatalf("Run(%q) = %d, want 0; stderr: %q", args, code, stderr.String())
		}
	}
	if *calls != 0 {
		t.Errorf("checkReleasesFn called %d times by ordinary commands, want 0", *calls)
	}
}

// TestRunUpdateCheckDefaultOptionsError proves the CLI-level cache-dir error
// path with both seams stubbed: DefaultOptions fails, so the localized error
// goes to stderr, the exit code is 1 and the check itself is never called.
func TestRunUpdateCheckDefaultOptionsError(t *testing.T) {
	calls := stubUpdateCheckSeams(t, errors.New("no cache dir"), releasecheck.Result{})
	var stdout, stderr bytes.Buffer
	code := Run([]string{"update-check"}, nil, &stdout, &stderr)
	if code != exitError {
		t.Fatalf("Run(update-check) = %d, want 1", code)
	}
	if got, want := stderr.String(), "ticket: не удалось проверить обновления: no cache dir\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if *calls != 0 {
		t.Errorf("checkReleasesFn called %d times, want 0", *calls)
	}
}
