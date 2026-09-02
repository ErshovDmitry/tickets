package cli_test

import (
	"bytes"
	_ "embed"
	"testing"

	"ticket/internal/cli"
)

// usageFixtureEN is the byte-exact English usage text. Production usage.go
// embeds the same file; tests treat templates/usage.en.txt as a byte fixture
// and assert verbatim stdout output for TICKET_LANG=en (T-0026 §A, plan
// 91e2f66d). Since T-0028 usage() prepends a runtime version line before
// the fixture, expected stdout is wantUsage(fixture), not the raw bytes.
//
//go:embed templates/usage.en.txt
var usageFixtureEN []byte

// langEnv layers lang-selection env vars (TICKET_LANG/LC_ALL/LANG) over a
// fresh testEnv map. Every case gets its own map: env is per-Run input, so
// t.Setenv on shared state is never needed (and would leak between tests).
func langEnv(t *testing.T, vars map[string]string) map[string]string {
	t.Helper()
	env := testEnv(t)
	for k, v := range vars {
		env[k] = v
	}
	return env
}

// TestHelpUsageLang pins the lang resolution contract of cli.Run for the
// help path (bash:11 LANG="${TICKET_LANG:-${LC_ALL:-${LANG:-ru}}}"): the
// first non-empty of TICKET_LANG → LC_ALL → LANG decides, a locale prefix
// up to '_'/'.' is matched ("en_US.UTF-8" → en), anything non-"en" (and all
// unset) falls back to the Russian text byte-identical to the bash
// reference.
func TestHelpUsageLang(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want []byte
	}{
		{"no lang env defaults to ru", nil, usageFixture},
		{"TICKET_LANG=en", map[string]string{"TICKET_LANG": "en"}, usageFixtureEN},
		{"TICKET_LANG beats LC_ALL and LANG", map[string]string{"TICKET_LANG": "ru", "LC_ALL": "en", "LANG": "en"}, usageFixture},
		{"LC_ALL beats LANG", map[string]string{"LC_ALL": "en", "LANG": "ru"}, usageFixtureEN},
		{"empty var counts as unset", map[string]string{"TICKET_LANG": "", "LANG": "en_US.UTF-8"}, usageFixtureEN},
		{"LANG en_US.UTF-8", map[string]string{"LANG": "en_US.UTF-8"}, usageFixtureEN},
		{"LANG ru_RU.KOI8-R", map[string]string{"LANG": "ru_RU.KOI8-R"}, usageFixture},
		{"LANG garbage falls back to ru", map[string]string{"LANG": "frobnicate_garbage"}, usageFixture},
		// Both rows prove non-en → ru fallback for other env vars too; the
		// prepended version line is lang-neutral (identical for every lang).
		{"TICKET_LANG garbage falls back to ru", map[string]string{"TICKET_LANG": "garbage"}, usageFixture},
		{"LANG zh_CN.UTF-8 falls back to ru", map[string]string{"LANG": "zh_CN.UTF-8"}, usageFixture},
		{"LANG bare en", map[string]string{"LANG": "en"}, usageFixtureEN},
		{"LANG en_GB", map[string]string{"LANG": "en_GB"}, usageFixtureEN},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := cli.Run([]string{"help"}, langEnv(t, tc.env), &stdout, &stderr)
			if code != 0 {
				t.Fatalf("Run(help) = %d, want 0; stderr: %q", code, stderr.String())
			}
			want := wantUsage(tc.want)
			if got := stdout.String(); got != want {
				t.Fatalf("stdout is not the expected usage text with version line (got %d bytes, want %d)", len(got), len(want))
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

// TestRunHelpPathLangRouting checks that every help route in cli.Run (no
// command, explicit help, -h, --help) prints the English usage text when
// TICKET_LANG=en (bash:147,153).
func TestRunHelpPathLangRouting(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no args", nil},
		{"help", []string{"help"}},
		{"short flag", []string{"-h"}},
		{"long flag", []string{"--help"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			env := langEnv(t, map[string]string{"TICKET_LANG": "en"})
			code := cli.Run(tc.args, env, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("Run(%q) = %d, want 0; stderr: %q", tc.args, code, stderr.String())
			}
			want := wantUsage(usageFixtureEN)
			if got := stdout.String(); got != want {
				t.Fatalf("stdout is not the verbatim EN usage text with version line (got %d bytes, want %d)", len(got), len(want))
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

// TestNewArgErrorLang checks the cmd_new argument-error path: `new` without
// a title prints the usage text for the resolved lang to stdout and exits 1
// (usage goes to stdout, stderr stays empty — bash:155 usage; exit 1).
func TestNewArgErrorLang(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := langEnv(t, map[string]string{"TICKET_LANG": "en"})
	code := cli.Run([]string{"new"}, env, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run(new) = %d, want 1", code)
	}
	want := wantUsage(usageFixtureEN)
	if got := stdout.String(); got != want {
		t.Fatalf("stdout is not the verbatim EN usage text with version line (got %d bytes, want %d)", len(got), len(want))
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

// TestUsageFixtureENCompleteness guards against a partial English
// translation: the EN text must carry all five command synopses and the
// Statuses/Types/Secrets blocks of the reference layout (plan 91e2f66d §A).
func TestUsageFixtureENCompleteness(t *testing.T) {
	for _, want := range []string{
		"ticket new",
		"ticket list",
		"ticket show",
		"ticket set",
		"ticket archive",
		"Statuses:",
		"Types:",
		"Secrets (passwords",
	} {
		if !bytes.Contains(usageFixtureEN, []byte(want)) {
			t.Errorf("usageFixtureEN is missing %q", want)
		}
	}
}
