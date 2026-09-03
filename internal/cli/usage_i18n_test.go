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
// help path (bash:11 LANG="${TICKET_LANG:-${LC_ALL:-${LANG:-ru}}}"), as
// adjusted by T-0036: the first non-empty of TICKET_LANG → LC_ALL → LANG
// decides, a locale prefix up to '_'/'.' is matched case-insensitively
// ("en_US.UTF-8" → en, "ru_RU.KOI8-R" → ru); a known ru/en prefix selects
// that text, an UNKNOWN non-empty locale falls back to the ENGLISH text
// (T-0036 deliberate change: it used to be Russian), and all vars unset
// still defaults to the Russian text.
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
		// T-0036: garbage/unknown locales now fall back to the ENGLISH text
		// (deliberate behavior change); the prepended version line is
		// lang-neutral (identical for every lang).
		{"LANG garbage falls back to en", map[string]string{"LANG": "frobnicate_garbage"}, usageFixtureEN},
		{"TICKET_LANG garbage falls back to en", map[string]string{"TICKET_LANG": "garbage"}, usageFixtureEN},
		{"LANG zh_CN.UTF-8 falls back to en", map[string]string{"LANG": "zh_CN.UTF-8"}, usageFixtureEN},
		{"LANG bare ru", map[string]string{"LANG": "ru"}, usageFixture},
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
// translation: the EN text must carry all five command synopses, the
// Statuses/Types/Secrets blocks of the reference layout (plan 91e2f66d §A)
// and the T-0036 migration section.
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
		"Migrating old tickets",
	} {
		if !bytes.Contains(usageFixtureEN, []byte(want)) {
			t.Errorf("usageFixtureEN is missing %q", want)
		}
	}
}

// TestUsageFixturesMigrationBlock pins the T-0036 appended migration
// sections: both usage files carry their language's section heading and
// the same sed recipe (the recipe bytes are identical, only the
// explanation is translated).
func TestUsageFixturesMigrationBlock(t *testing.T) {
	for _, tc := range []struct {
		name    string
		fixture []byte
		heading string
	}{
		{"ru", usageFixture, "Миграция старых тикетов"},
		{"en", usageFixtureEN, "Migrating old tickets"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, want := range []string{tc.heading, "sed -i -E", "tickets/*.md"} {
				if !bytes.Contains(tc.fixture, []byte(want)) {
					t.Errorf("usage fixture (%s) is missing %q", tc.name, want)
				}
			}
		})
	}
}
