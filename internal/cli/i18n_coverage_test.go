package cli_test

// T-0080 i18n coverage: proves through the black-box cli.Run entry that
// CLI messages are localized (EN when TICKET_LANG=en, RU by default) and
// that the ru/en JSON dictionaries stay in key parity. All runs use
// t.TempDir() stores — never the real tickets/ directory.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"ticket/internal/cli"
	"ticket/internal/domain"
)

// cyrRe matches any Cyrillic character (U+0400–U+04FF): an EN-locale
// run must produce none of them anywhere in the CLI output.
var cyrRe = regexp.MustCompile(`[\x{0400}-\x{04FF}]`)

// assertNoCyrillic fails the test when s contains a Cyrillic character.
func assertNoCyrillic(t *testing.T, where, s string) {
	t.Helper()
	if loc := cyrRe.FindString(s); loc != "" {
		t.Errorf("%s contains Cyrillic %q in %q", where, loc, s)
	}
}

// runEN invokes cli.Run with TICKET_LANG=en against a temp store dir and
// returns both streams and the exit code.
func runEN(t *testing.T, dir string, args ...string) (string, string, int) {
	t.Helper()
	env := map[string]string{"TICKETS_DIR": dir, "TICKET_LANG": "en"}
	var stdout, stderr bytes.Buffer
	code := cli.Run(args, env, &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

// TestI18nErrorsLocalizeToEN runs every command through cli.Run with
// TICKET_LANG=en, triggers an error path (or the archive/init notice) and
// asserts the expected EN substring on the pinned stream, the exit code,
// and the absence of ANY Cyrillic in both streams (a leaked RU literal
// would fail the no-Cyrillic assertion even if the substring matched).
// The shared store is seeded once with one open ticket; every case below
// fails validation or reads without mutating, so the store stays intact.
func TestI18nErrorsLocalizeToEN(t *testing.T) {
	dir := t.TempDir()
	if sout, serr, code := runEN(t, dir, "new", "seed ticket"); code != 0 {
		t.Fatalf("seed new: code=%d stdout=%q stderr=%q", code, sout, serr)
	}

	cases := []struct {
		name    string
		args    []string
		stream  string // "stderr" or "stdout"
		wantSub string
		wantCod int
	}{
		{"new empty title", []string{"new", ""}, "stderr", "title must not be empty", 1},
		{"new invalid type", []string{"new", "X", "-t", "BAD"}, "stderr", "type must be one of", 1},
		{"new invalid priority", []string{"new", "X", "-p", "BAD"}, "stderr", "priority must be one of", 1},
		{"list bad filter", []string{"list", "BAD"}, "stderr", "filter must be one of", 1},
		{"list unknown flag", []string{"list", "-z"}, "stderr", "unknown flag", 1},
		{"show non-existent", []string{"show", "42"}, "stderr", "not found", 1},
		{"show non-numeric arg", []string{"show", "x"}, "stderr", "is not a ticket number", 1},
		{"set invalid status", []string{"set", "1", "BAD", "note"}, "stderr", "status must be one of", 1},
		{"archive empty", []string{"archive"}, "stdout", "No closed tickets", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sout, serr, code := runEN(t, dir, tc.args...)
			if code != tc.wantCod {
				t.Fatalf("Run(%q) = %d, want %d; stderr: %q", tc.args, code, tc.wantCod, serr)
			}
			got := sout
			if tc.stream == "stderr" {
				got = serr
			}
			if !strings.Contains(got, tc.wantSub) {
				t.Errorf("%s = %q, missing EN substring %q", tc.stream, got, tc.wantSub)
			}
			assertNoCyrillic(t, "stdout", sout)
			assertNoCyrillic(t, "stderr", serr)
		})
	}

	// init: cli.Run routes init to os.Getwd (not TICKETS_DIR), so the
	// hermetic path is t.Chdir into a fresh temp dir; TICKETS_DIR points
	// at a fresh subdir that init ignores.
	t.Run("init", func(t *testing.T) {
		root := t.TempDir()
		t.Chdir(root)
		sout, serr, code := runEN(t, filepath.Join(root, "tickets"), "init")
		if code != 0 {
			t.Fatalf("Run(init) = %d; stderr: %q", code, serr)
		}
		if !strings.Contains(sout, "Initialized") {
			t.Errorf("stdout = %q, missing EN substring \"Initialized\"", sout)
		}
		assertNoCyrillic(t, "stdout", sout)
		assertNoCyrillic(t, "stderr", serr)
	})
}

// TestI18nRUGettersLegacyBytes pins the default (RU) getter output
// byte-for-byte: the CLI defaults to LangRU when no env variable names a
// language (langFrom, cli.go), so these strings are the user-visible
// contract; any wording change here must be deliberate.
func TestI18nRUGettersLegacyBytes(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"ErrTitleEmpty", domain.ErrTitleEmpty(domain.LangRU), "ticket: краткое описание не может быть пустым"},
		{"ErrTicketNotFound", domain.ErrTicketNotFound(domain.LangRU, "x"), "ticket: тикет «x» не найден"},
		{"ErrStatusInvalid", domain.ErrStatusInvalid(domain.LangRU), "ticket: статус — один из: open wip done closed"},
		{"ErrTicketAlreadyStatus", domain.ErrTicketAlreadyStatus(domain.LangRU, 5, "open"), "ticket: тикет 5 уже в статусе open"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %q, want legacy byte-exact %q", tc.got, tc.want)
			}
		})
	}
}

// TestI18nDictJSONKeyParity guards the ru.json/en.json dictionaries: every
// key present in one must exist in the other with a non-empty value, so a
// newly added RU message can never ship without its EN counterpart (or
// vice versa). Values are strings except `headers` (a []string), so the
// dicts unmarshal into map[string]json.RawMessage and the emptiness check
// accepts both shapes. The path is derived from this file's compile-time
// location via the shared moduleRoot() helper (integration_version_test.go).
func TestI18nDictJSONKeyParity(t *testing.T) {
	root, ok := moduleRoot()
	if !ok {
		t.Skip("cannot locate the ticket module root (build with -trimpath or moved binary); dict files unreachable")
	}
	dict := func(name string) map[string]json.RawMessage {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(root, "internal", "domain", "langs", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("unmarshal %s: %v", name, err)
		}
		return m
	}
	ru, en := dict("ru.json"), dict("en.json")
	for _, pair := range []struct {
		missName string
		miss     map[string]json.RawMessage
		other    map[string]json.RawMessage
	}{
		{"en.json", ru, en},
		{"ru.json", en, ru},
	} {
		for key := range pair.miss {
			otherVal, present := pair.other[key]
			if !present {
				t.Errorf("key %q present in one dict, missing in %s", key, pair.missName)
				continue
			}
			if emptyDictValue(otherVal) {
				t.Errorf("key %q has an empty value in %s", key, pair.missName)
			}
		}
	}
}

// emptyDictValue reports whether a raw JSON dict value is empty: an empty
// string ("\"\"") or an array holding only empty strings.
func emptyDictValue(raw json.RawMessage) bool {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s == ""
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		for _, s := range arr {
			if s != "" {
				return false
			}
		}
		return true
	}
	return false
}
