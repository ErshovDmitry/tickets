package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"ticket/internal/cli"
)

// verboseFlag is the global flag documented in the usage text. The pin below
// looks for it as a standalone line: the templates also spell the rejected
// glued form "--verbose=x" inside the description, so a plain
// strings.Contains(out, "--verbose") would survive deletion of the flag line.
const verboseFlag = "--verbose"

// helpOutput renders the real usage text through the public CLI entry point
// (cli.Run → printUsage → usage → usageFrom) for the given TICKET_LANG, so the
// expectations come from the shipped renderer instead of the template files.
func helpOutput(t *testing.T, lang string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run([]string{"help"}, langEnv(t, map[string]string{"TICKET_LANG": lang}), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(help) with TICKET_LANG=%s = %d, want 0; stderr: %q", lang, code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(help) with TICKET_LANG=%s wrote to stderr: %q", lang, stderr.String())
	}
	return stdout.String()
}

// verboseBlockLines returns the rendered --verbose documentation block: the
// flag line itself plus every directly following indented non-empty line.
func verboseBlockLines(t *testing.T, lang, usage string) []string {
	t.Helper()
	lines := strings.Split(usage, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != verboseFlag {
			continue
		}
		block := []string{line}
		for _, cont := range lines[i+1:] {
			if cont == "" || !strings.HasPrefix(cont, " ") {
				break
			}
			block = append(block, cont)
		}
		return block
	}
	t.Fatalf("printed usage (TICKET_LANG=%s) does not document %s as a standalone line", lang, verboseFlag)
	return nil
}

// TestUsageVerboseAndLogEnvDocumented pins that ticket --help advertises the
// global --verbose flag and the TICKET_LOG environment variable (T-0129) in
// both languages.
func TestUsageVerboseAndLogEnvDocumented(t *testing.T) {
	for _, lang := range []string{"en", "ru"} {
		t.Run(lang, func(t *testing.T) {
			usage := helpOutput(t, lang)
			if block := verboseBlockLines(t, lang, usage); len(block) < 2 {
				t.Errorf("printed usage (TICKET_LANG=%s) documents %s without a description", lang, verboseFlag)
			}
			if !strings.Contains(usage, "TICKET_LOG=") {
				t.Errorf("printed usage (TICKET_LANG=%s) does not document TICKET_LOG=", lang)
			}
		})
	}
}

// TestUsageVerboseBlockParity pins that the --verbose block keeps the same
// number of lines in both languages and that each rendered block still spells
// the rejected glued form. The count is read from the output, never hardcoded.
func TestUsageVerboseBlockParity(t *testing.T) {
	en := verboseBlockLines(t, "en", helpOutput(t, "en"))
	ru := verboseBlockLines(t, "ru", helpOutput(t, "ru"))

	if len(en) != len(ru) {
		t.Errorf("--verbose block lines differ: en=%d, ru=%d", len(en), len(ru))
	}
	for lang, block := range map[string][]string{"en": en, "ru": ru} {
		if !strings.Contains(strings.Join(block, "\n"), verboseFlag+"=") {
			t.Errorf("--verbose block (%s) does not document the glued form %s=", lang, verboseFlag)
		}
	}
}
