package domain

import (
	"strings"
	"testing"
)

// TestSectionAliasNames pins the exported alias list: sorted, both aliases,
// and disjoint from the canonical tokens (otherwise the unknown-section
// message would name the same spelling twice).
func TestSectionAliasNames(t *testing.T) {
	got := SectionAliasNames()
	want := []string{"brief", "user_comments"}
	if len(got) != len(want) {
		t.Fatalf("SectionAliasNames() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SectionAliasNames() = %v, want %v", got, want)
		}
	}
	canonical := make(map[string]bool, len(sectionNames))
	for _, name := range CanonicalSectionNames() {
		canonical[name] = true
	}
	for _, alias := range got {
		if canonical[alias] {
			t.Errorf("alias %q is also a canonical token", alias)
		}
	}
}

// TestErrUnknownSectionRendersArguments pins that the unknown-section message
// carries the name plus the caller-supplied token and alias lists: both are
// passed in, never baked into the template, so a distinctive probe list must
// reach stdout verbatim in either language.
func TestErrUnknownSectionRendersArguments(t *testing.T) {
	names := []string{"aaa", "bbb"}
	aliases := []string{"zzz"}
	for _, lang := range []Lang{LangRU, LangEN} {
		got := ErrUnknownSection(lang, "nope", names, aliases)
		for _, want := range []string{"nope", strings.Join(names, ", "), strings.Join(aliases, ", ")} {
			if !strings.Contains(got, want) {
				t.Errorf("ErrUnknownSection(%s) = %q, want it to contain %q", lang, got, want)
			}
		}
		if strings.Contains(got, "brief") || strings.Contains(got, "user_comments") {
			t.Errorf("ErrUnknownSection(%s) = %q, template still hard-codes the alias list", lang, got)
		}
	}
}
