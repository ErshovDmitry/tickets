package cli_test

// T-0109: the usage text must document the details input of `ticket new`
// and the Details format rule in BOTH languages (RU is the default, EN is
// selected with TICKET_LANG). The fragments are pinned against the
// embedded fixtures (usageFixture / usageFixtureEN) — the very bytes the
// binary embeds and prints.

import (
	"bytes"
	"strings"
	"testing"

	"ticket/internal/cli"
)

// usageDetailsDoc lists, per language, the fragments the usage text MUST
// carry for the `-d` details argument (T-0109):
//
//   - a plain literal argument,
//   - "@<file>" — the content of a file,
//   - "@@name" — the file named "@name" (exactly one "@" is stripped, so
//     "@@" is NOT an escape),
//   - a bare "@" — rejected, no ticket is created,
//   - "-" — stdin,
//   - byte-for-byte copy (no trimming, no size limit),
//   - the numbered-list format of the Details section.
var usageDetailsDoc = []struct {
	name    string
	fixture []byte
	lang    string
	want    []string
}{
	{
		name:    "ru",
		fixture: usageFixture,
		lang:    "ru",
		want: []string{
			"@<файл>",
			`@@имя — файл с именем "@имя"`,
			"@ без пути — ошибка",
			"- — stdin",
			"байт в байт",
			"нумерованный список",
			"N. <что> — <файл:строка>; <решение>",
		},
	},
	{
		name:    "en",
		fixture: usageFixtureEN,
		lang:    "en",
		want: []string{
			"@<file>",
			`@@name — file named "@name"`,
			"@ without a path is an error",
			"- — stdin",
			"byte for byte",
			"numbered list",
			"N. <what> — <file:line>; <fix>",
		},
	},
}

// TestUsageDetailsDocumentsInputForms checks the embedded usage texts: each
// language must document every details input form and the Details format
// rule. A fragment that disappears from one language breaks the contract
// for that language only.
func TestUsageDetailsDocumentsInputForms(t *testing.T) {
	for _, doc := range usageDetailsDoc {
		t.Run(doc.name, func(t *testing.T) {
			for _, want := range doc.want {
				if !bytes.Contains(doc.fixture, []byte(want)) {
					t.Errorf("usage text (%s) is missing %q", doc.name, want)
				}
			}
		})
	}
}

// TestUsageDetailsPrintedByCLI checks the same fragments in the text the
// binary actually prints (the help route), so the documented forms and the
// printed usage cannot drift apart. The output is the version line plus
// the usage text; only the fragments are asserted here — the byte-exact
// stdout is pinned by the usage tests.
func TestUsageDetailsPrintedByCLI(t *testing.T) {
	for _, doc := range usageDetailsDoc {
		t.Run(doc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			env := langEnv(t, map[string]string{"TICKET_LANG": doc.lang})
			if code := cli.Run([]string{"help"}, env, &stdout, &stderr); code != 0 {
				t.Fatalf("Run(help) = %d, want 0; stderr: %q", code, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
			out := stdout.String()
			for _, want := range doc.want {
				if !strings.Contains(out, want) {
					t.Errorf("printed usage (%s) is missing %q", doc.name, want)
				}
			}
		})
	}
}
