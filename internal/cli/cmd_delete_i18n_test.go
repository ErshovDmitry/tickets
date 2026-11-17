package cli

// T-0189 W2-A1: RU/EN byte snapshots of every delete refusal, the who CR/LF
// journal guard and the read-only store refusal. Snapshots pin the exact
// localized bytes (an EN run that leaked a RU literal would not match), and
// the last two drive cmdDelete directly to reach the store-level mappings
// that the dispatch who-guard would otherwise short-circuit.

import (
	"bytes"
	"strings"
	"testing"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// assertNoCyrillicOutput fails when an EN-locale stream carries a RU literal,
// which a self-referential snapshot (the wanted text comes from the EN dict
// too) would otherwise never catch.
func assertNoCyrillicOutput(t *testing.T, name, s string) {
	t.Helper()
	if hasCyrillic(s) {
		t.Errorf("%s: EN output contains Cyrillic: %q", name, s)
	}
}

// assertDeleteSnapshot pins one refusal byte-exactly for lang; an EN run is
// additionally proven Cyrillic-free.
func assertDeleteSnapshot(t *testing.T, lang domain.Lang, name, se string, code int, want string) {
	t.Helper()
	if code != exitError {
		t.Errorf("%s: code=%d want %d", name, code, exitError)
	}
	if se != want+"\n" {
		t.Errorf("%s: stderr=%q, want byte-exact %q", name, se, want+"\n")
	}
	if lang == domain.LangEN {
		assertNoCyrillicOutput(t, name, se)
	}
}

func TestDeleteLocalizedSnapshots(t *testing.T) {
	for _, lang := range []domain.Lang{domain.LangRU, domain.LangEN} {
		t.Run(string(lang), func(t *testing.T) {
			l := string(lang)

			dir := resolvedTempDir(t)
			seedOpen(t, dir)
			if _, se, code := runDeleteCLI(t, dir, l, "set", "1", "wip"); code != 0 {
				t.Fatalf("set wip: code=%d stderr=%q", code, se)
			}
			_, se, code := runDeleteCLI(t, dir, l, "delete", "1", "--yes")
			assertDeleteSnapshot(t, lang, "guard", se, code, domain.ErrDeleteOnlyOpen(lang, "wip"))

			// A flag in the ref slot: the command-agnostic unknown-flag text.
			_, se, code = runDeleteCLI(t, dir, l, "delete", "--bogus")
			assertDeleteSnapshot(t, lang, "ref-slot-flag", se, code, domain.ErrUnknownFlagList(lang, "--bogus"))

			dir = resolvedTempDir(t)
			seedOpen(t, dir)
			if _, se, code := runDeleteCLI(t, dir, l, "delete", "1", "--yes"); code != 0 {
				t.Fatalf("first delete: code=%d stderr=%q", code, se)
			}
			_, se, code = runDeleteCLI(t, dir, l, "delete", "1", "--yes")
			assertDeleteSnapshot(t, lang, "already-deleted", se, code, domain.ErrDeleteAlreadyDeleted(lang))

			dir = resolvedTempDir(t)
			seedOpen(t, dir)
			_, se, code = runDeleteCLI(t, dir, l, "delete", "abc", "--yes")
			assertDeleteSnapshot(t, lang, "invalid-ref", se, code, domain.ErrDeleteInvalidRef(lang, "abc"))

			dir = resolvedTempDir(t)
			seedOpen(t, dir)
			withRecoverSeams(t, "", false)
			_, se, code = runDeleteCLI(t, dir, l, "delete", "1")
			assertDeleteSnapshot(t, lang, "non-tty", se, code, domain.DeleteRefused(lang))

			dir = resolvedTempDir(t)
			seedOpen(t, dir)
			withRecoverSeams(t, "no\n", true)
			_, se, code = runDeleteCLI(t, dir, l, "delete", "1")
			assertDeleteSnapshot(t, lang, "cancelled", se, code, domain.DeleteCancelled(lang))
		})
	}
}

// TestDeleteENSuccessHasNoCyrillic proves a successful EN delete prints only
// the returned path — no localized RU literal rides along on stdout/stderr.
func TestDeleteENSuccessHasNoCyrillic(t *testing.T) {
	dir := resolvedTempDir(t)
	seedOpen(t, dir)
	so, se, code := runDeleteCLI(t, dir, "en", "delete", "1", "--yes")
	if code != exitOK || so != deletedPath(dir, 1)+"\n" || se != "" {
		t.Fatalf("EN success: code=%d stdout=%q stderr=%q", code, so, se)
	}
	assertNoCyrillicOutput(t, "success stdout", so)
	assertNoCyrillicOutput(t, "success stderr", se)
}

func TestDeleteWhoNewline(t *testing.T) {
	// Every line-break form the journal guard rejects: a bare LF, a bare CR
	// (a lone CR is a line break for the store's ContainsAny guard) and CRLF.
	for _, tc := range []struct{ name, who string }{
		{"LF", "evil\nforged"},
		{"CR", "evil\rforged"},
		{"CRLF", "evil\r\nforged"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := resolvedTempDir(t)
			seedOpen(t, dir)
			st, err := store.New(dir)
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			before := treeState(t, dir)
			var so, se bytes.Buffer
			code := cmdDelete(st, []string{"1", "--yes"}, tc.who, "proj", domain.LangRU, &so, &se)
			if code != exitError {
				t.Fatalf("who %q: code=%d want %d (stderr=%q)", tc.who, code, exitError, se.String())
			}
			if !strings.Contains(se.String(), domain.ErrJournalNewline(domain.LangRU)) {
				t.Errorf("who %q: stderr=%q, want journal-newline refusal", tc.who, se.String())
			}
			assertSameTree(t, dir, before)
		})
	}
}

func TestDeleteReadOnlyStore(t *testing.T) {
	dir := resolvedTempDir(t)
	seedOpen(t, dir)
	st, err := store.NewReadOnly(dir)
	if err != nil {
		t.Fatalf("open read-only store: %v", err)
	}
	before := treeState(t, dir)
	var so, se bytes.Buffer
	code := cmdDelete(st, []string{"1", "--yes"}, "tester", "proj", domain.LangRU, &so, &se)
	if code != exitError {
		t.Fatalf("read-only: code=%d want %d (stderr=%q)", code, exitError, se.String())
	}
	if !strings.Contains(se.String(), domain.ErrReadOnly(domain.LangRU)) {
		t.Errorf("stderr=%q, want read-only refusal", se.String())
	}
	assertSameTree(t, dir, before)
}
