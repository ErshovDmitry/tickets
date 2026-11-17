package cli

// T-0192: a mutating command against a read-only store (store.NewReadOnly)
// must report the localized errReadOnly message — not the generic
// "ticket: store: read-only". The commands are driven directly (cmd* entry
// points) so the store-level mapping is exercised without the dispatch
// guards short-circuiting it; store.NewReadOnly works on every platform
// and for root, unlike the chmod-based rodir tests.

import (
	"bytes"
	"strings"
	"testing"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// openReadOnlyStore opens dir as a read-only store.
func openReadOnlyStore(t *testing.T, dir string) *store.Store {
	t.Helper()
	st, err := store.NewReadOnly(dir)
	if err != nil {
		t.Fatalf("store.NewReadOnly(%s): %v", dir, err)
	}
	return st
}

// assertReadOnlyRefusal pins the exact localized refusal for lang and proves
// an EN stream carries no Cyrillic.
func assertReadOnlyRefusal(t *testing.T, lang domain.Lang, se string) {
	t.Helper()
	want := domain.ErrReadOnly(lang) + "\n"
	if se != want {
		t.Errorf("stderr=%q, want byte-exact %q", se, want)
	}
	if lang == domain.LangEN && hasCyrillic(se) {
		t.Errorf("stderr=%q contains Cyrillic", se)
	}
}

// runReadOnlyBothLocales drives call against a fresh read-only store for RU
// and EN and pins the localized refusal in both.
func runReadOnlyBothLocales(t *testing.T, dir string, call func(st *store.Store, lang domain.Lang, so, se *bytes.Buffer) int) {
	t.Helper()
	for _, lang := range []domain.Lang{domain.LangRU, domain.LangEN} {
		st := openReadOnlyStore(t, dir)
		var so, se bytes.Buffer
		if code := call(st, lang, &so, &se); code != exitError {
			t.Fatalf("lang=%s: code=%d want %d (stderr=%q)", lang, code, exitError, se.String())
		}
		assertReadOnlyRefusal(t, lang, se.String())
	}
}

// TestReadOnlyI18nDomain pins the exact dictionary text of errReadOnly in
// both locales, plus the EN fallback for an unknown language.
func TestReadOnlyI18nDomain(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"ru", domain.ErrReadOnly(domain.LangRU), "ticket: хранилище тикетов в режиме только чтения"},
		{"en", domain.ErrReadOnly(domain.LangEN), "ticket: the ticket store is read-only"},
		{"fallback", domain.ErrReadOnly(domain.Lang("zz")), "ticket: the ticket store is read-only"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("ErrReadOnly = %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestReadOnlyI18nNew(t *testing.T) {
	dir := resolvedTempDir(t)
	runReadOnlyBothLocales(t, dir, func(st *store.Store, lang domain.Lang, so, se *bytes.Buffer) int {
		return cmdNew(st, []string{"seed ticket"}, "tester", "proj", lang, so, se)
	})
}

func TestReadOnlyI18nSet(t *testing.T) {
	dir := resolvedTempDir(t)
	seedOpen(t, dir)
	runReadOnlyBothLocales(t, dir, func(st *store.Store, lang domain.Lang, so, se *bytes.Buffer) int {
		return cmdSet(st, []string{"1", "wip"}, "tester", "", lang, so, se)
	})
}

func TestReadOnlyI18nEdit(t *testing.T) {
	dir := resolvedTempDir(t)
	seedOpen(t, dir)
	runReadOnlyBothLocales(t, dir, func(st *store.Store, lang domain.Lang, so, se *bytes.Buffer) int {
		return cmdEdit(st, []string{"1", "--title", "new"}, "tester", "", lang, so, se)
	})
}

func TestReadOnlyI18nDelete(t *testing.T) {
	dir := resolvedTempDir(t)
	seedOpen(t, dir)
	runReadOnlyBothLocales(t, dir, func(st *store.Store, lang domain.Lang, so, se *bytes.Buffer) int {
		return cmdDelete(st, []string{"1", "--yes"}, "tester", "", lang, so, se)
	})
}

func TestReadOnlyI18nArchive(t *testing.T) {
	dir := resolvedTempDir(t)
	seedOpen(t, dir)
	if _, se, code := runDeleteCLI(t, dir, "ru", "set", "1", "done", "seed done"); code != 0 {
		t.Fatalf("seed set done: code=%d stderr=%q", code, se)
	}
	runReadOnlyBothLocales(t, dir, func(st *store.Store, lang domain.Lang, so, se *bytes.Buffer) int {
		return cmdArchive(st, []string{"1"}, "tester", "", lang, so, se)
	})
}

func TestReadOnlyI18nRecover(t *testing.T) {
	dir := resolvedTempDir(t)
	crashPair(t, dir)
	runReadOnlyBothLocales(t, dir, func(st *store.Store, lang domain.Lang, so, se *bytes.Buffer) int {
		return cmdRecover(st, []string{"1", "T-0001-open.md", "--yes"}, "", "", lang, so, se)
	})
}

// TestReadOnlyI18nNormalize: warnings/duplicates may precede the refusal, so
// the stderr contract is Contains — and the raw "store:" prefix must be gone.
func TestReadOnlyI18nNormalize(t *testing.T) {
	dir := resolvedTempDir(t)
	seedOpen(t, dir)
	for _, lang := range []domain.Lang{domain.LangRU, domain.LangEN} {
		st := openReadOnlyStore(t, dir)
		var so, se bytes.Buffer
		if code := cmdNormalize(st, normalizeFlags{}, lang, &so, &se); code != exitError {
			t.Fatalf("lang=%s: code=%d want %d (stderr=%q)", lang, code, exitError, se.String())
		}
		if !strings.Contains(se.String(), domain.ErrReadOnly(lang)) {
			t.Errorf("lang=%s: stderr=%q, want read-only refusal", lang, se.String())
		}
		if strings.Contains(se.String(), "store:") {
			t.Errorf("lang=%s: stderr=%q leaks the raw store prefix", lang, se.String())
		}
		if lang == domain.LangEN && hasCyrillic(se.String()) {
			t.Errorf("lang=EN: stderr=%q contains Cyrillic", se.String())
		}
	}
}
