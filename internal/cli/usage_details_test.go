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
//
// T-0215 extends the same table with the `set` comment fragments: the
// single-argument source forms, the multi-argument literal join and the
// lone-marker trade-off. T-0237 replaced the old blanket "a CR or LF is
// refused" claim with the real rule: one trailing LF or CRLF is stripped,
// anything left over is refused. The fragments are matched against the
// whole usage text, so they pin the wording of the `set` block in both
// languages; notWant keeps the retired wording from creeping back.
var usageDetailsDoc = []struct {
	name    string
	fixture []byte
	lang    string
	want    []string
	notWant []string
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
			"комментарий можно дать ОДНИМ аргументом",
			"@ без пути как ОДИН аргумент комментария — ошибка (код 1)",
			"срезается ровно один завершающий LF или CRLF",
			"второй завершающий LF, одиночный завершающий CR, перевод строки внутри текста — отклоняется (код 1)",
			"два или больше аргументов комментария соединяются пробелом и берутся буквально",
			"одиночный \"-\" или \"@...\" никогда не будет буквальным комментарием",
		},
		notWant: []string{
			"CR или LF в нём отклоняются",
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
			"the comment may be given as ONE argument",
			"a bare @ as the ONE comment argument (no path) is an error (exit 1)",
			"exactly one trailing LF or CRLF is stripped",
			"a second trailing LF, a lone trailing CR, an embedded line break — is refused (exit 1)",
			"two or more comment arguments are joined with a space and used literally",
			"a lone \"-\" or \"@...\" argument is never a literal comment",
		},
		notWant: []string{
			"a CR or LF in it is refused",
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
			for _, banned := range doc.notWant {
				if bytes.Contains(doc.fixture, []byte(banned)) {
					t.Errorf("usage text (%s) still carries the retired wording %q", doc.name, banned)
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
			for _, banned := range doc.notWant {
				if strings.Contains(out, banned) {
					t.Errorf("printed usage (%s) still carries the retired wording %q", doc.name, banned)
				}
			}
		})
	}
}
