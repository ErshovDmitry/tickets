package cli

// T-0240 A2 CLI tests for ticket section set: replacing an editable section,
// clearing with an empty payload, the protected/unknown refusal listing the
// editable allowlist, canonical tokens + aliases, usage and exit codes, and
// EN localization.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSectionSetReplaces pins DS1/DS8: the target section body is replaced,
// a neighbouring section is preserved, and one one-line journal event names
// the section. Success is silent (exit 0).
func TestSectionSetReplaces(t *testing.T) {
	dir := writeTestDir(t)
	seedWriteTicket(t, dir)
	withStdin(t, strings.NewReader("new details"))
	if _, serr, code := writeRun(t, dir, "", "section", "set", "1", "details", "-"); code != 0 || serr != "" {
		t.Fatalf("set details: code=%d stderr=%q", code, serr)
	}
	body := readWriteTicket(t, dir)
	if !strings.Contains(body, "new details") {
		t.Errorf("details not replaced: %q", body)
	}
	if !strings.Contains(body, "Тестовый тикет") {
		t.Errorf("neighbouring Summary lost: %q", body)
	}
	if !strings.Contains(body, "section details: заменено") {
		t.Errorf("journal event missing: %q", body)
	}
}

// TestSectionSetClears pins that an empty payload clears the section (an
// empty Summary renders as the title — Render behavior, exit 0).
func TestSectionSetClears(t *testing.T) {
	dir := writeTestDir(t)
	seedWriteTicket(t, dir)
	withStdin(t, strings.NewReader("brief text"))
	if _, serr, code := writeRun(t, dir, "", "section", "set", "1", "summary", "-"); code != 0 {
		t.Fatalf("seed summary: code=%d stderr=%q", code, serr)
	}
	withStdin(t, strings.NewReader(""))
	if _, serr, code := writeRun(t, dir, "", "section", "set", "1", "summary", "-"); code != 0 || serr != "" {
		t.Fatalf("clear summary: code=%d stderr=%q", code, serr)
	}
	if body := readWriteTicket(t, dir); strings.Contains(body, "brief text") {
		t.Errorf("empty payload did not clear the summary: %q", body)
	}
}

// TestSectionProtectedAndUnknown pins DS2: a protected (journal,
// user-comments) or unknown token is refused (exit 1) listing the EDITABLE
// names from the code (summary, details, comments), never a hard-coded list,
// and the ticket stays byte-identical.
func TestSectionProtectedAndUnknown(t *testing.T) {
	dir := writeTestDir(t)
	seedWriteTicket(t, dir)
	before := readWriteTicket(t, dir)
	for _, sec := range []string{"journal", "user-comments", "user_comments", "bogus"} {
		t.Run(sec, func(t *testing.T) {
			withStdin(t, strings.NewReader("x"))
			_, serr, code := writeRun(t, dir, "en", "section", "set", "1", sec, "-")
			if code != 1 {
				t.Fatalf("section %q: code=%d, want 1; stderr=%q", sec, code, serr)
			}
			if !strings.Contains(serr, "editable sections: summary, details, comments") {
				t.Errorf("refusal must list the editable allowlist: %q", serr)
			}
			if after := readWriteTicket(t, dir); after != before {
				t.Error("refusal mutated the ticket")
			}
		})
	}
}

// TestSectionTokensAndAliases pins ParseSectionName acceptance: canonical
// tokens (case-insensitive) and the brief alias resolve to editable sections.
func TestSectionTokensAndAliases(t *testing.T) {
	dir := writeTestDir(t)
	seedWriteTicket(t, dir)
	for _, sec := range []string{"summary", "Summary", "details", "comments", "brief"} {
		t.Run(sec, func(t *testing.T) {
			withStdin(t, strings.NewReader("token payload"))
			_, serr, code := writeRun(t, dir, "en", "section", "set", "1", sec, "-")
			if code != 0 {
				t.Errorf("section %q: code=%d stderr=%q", sec, code, serr)
			}
		})
	}
}

// TestSectionUsage pins the subcommand/positional shape: section alone, a
// non-set subcommand, and missing number/section all print usage (exit 1).
func TestSectionUsage(t *testing.T) {
	dir := writeTestDir(t)
	seedWriteTicket(t, dir)
	for _, args := range [][]string{
		{"section"},
		{"section", "bogus"},
		{"section", "set"},
		{"section", "set", "1"},
	} {
		stdout, _, code := writeRun(t, dir, "en", args...)
		if code != 1 || !strings.Contains(stdout, "ticket section set") {
			t.Errorf("%q: code=%d stdout=%q, want usage mentioning section set", args, code, stdout)
		}
	}
}

// TestSectionExitCodes pins not-found (exit 1) and ambiguous (exit 2).
func TestSectionExitCodes(t *testing.T) {
	dir := writeTestDir(t)
	seedWriteTicket(t, dir)
	withStdin(t, strings.NewReader("x"))
	if _, serr, code := writeRun(t, dir, "en", "section", "set", "42", "details", "-"); code != 1 || !strings.Contains(serr, "not found") {
		t.Errorf("not-found: code=%d stderr=%q", code, serr)
	}
	data, err := os.ReadFile(filepath.Join(dir, "T-0001-open.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "T-0001-wip.md"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	withStdin(t, strings.NewReader("x"))
	_, serr, code := writeRun(t, dir, "en", "section", "set", "1", "details", "-")
	if code != 2 || !strings.Contains(serr, "T-0001-open.md") || !strings.Contains(serr, "T-0001-wip.md") {
		t.Errorf("ambiguous: code=%d stderr=%q", code, serr)
	}
}

// TestSectionErrorsLocalizeToEN pins the EN refusal with no Cyrillic leak.
func TestSectionErrorsLocalizeToEN(t *testing.T) {
	dir := writeTestDir(t)
	seedWriteTicket(t, dir)
	withStdin(t, strings.NewReader("x"))
	_, serr, code := writeRun(t, dir, "en", "section", "set", "1", "journal", "-")
	if code != 1 || !strings.Contains(serr, "cannot write section journal") {
		t.Fatalf("EN protected: code=%d stderr=%q", code, serr)
	}
	for _, r := range serr {
		if r >= 0x0400 && r <= 0x04FF {
			t.Fatalf("Cyrillic leak in EN message: %q", serr)
		}
	}
}
