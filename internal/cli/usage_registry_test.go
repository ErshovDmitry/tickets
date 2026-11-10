package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// Both regexes are package level: compiling them once keeps the helpers usable
// from loops and table cases without repeated recompilation.
var (
	// exitCodeSegmentRe matches the «N — » head of an exit-code segment in the
	// embedded help templates.
	exitCodeSegmentRe = regexp.MustCompile(`(\d+) — `)

	// exitCodeBulletRe matches a README exit-code bullet such as
	// "- `0` — success." and captures the documented number.
	exitCodeBulletRe = regexp.MustCompile("^- `(\\d+)`")
)

func TestUsageRU(t *testing.T) {
	var b bytes.Buffer
	usage(&b, domain.LangRU)
	output := b.String()
	if !strings.Contains(output, "ticket — тикеты проекта") {
		t.Error("missing RU usage marker")
	}
}

func TestUsageEN(t *testing.T) {
	var b bytes.Buffer
	usage(&b, domain.LangEN)
	output := b.String()
	if !strings.Contains(output, "ticket — project tickets") {
		t.Error("missing EN usage marker")
	}
}

// usageParagraph returns the blank-line separated paragraph of text that
// contains marker. It fails the test when no such paragraph exists, so a
// renamed or dropped paragraph can never slip through as a vacuous pass.
func usageParagraph(t *testing.T, text, marker string) string {
	t.Helper()
	for _, para := range strings.Split(text, "\n\n") {
		if strings.Contains(para, marker) {
			return para
		}
	}
	t.Fatalf("usage text has no paragraph containing %q", marker)
	return ""
}

// exitCodeSegments extracts the numbers of the «N — » segments of an
// exit-code paragraph, sorted. Two-sided comparison against an exact set means
// both a missing and a stale extra code fail the test.
func exitCodeSegments(para string) []string {
	matches := exitCodeSegmentRe.FindAllStringSubmatch(para, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	slices.Sort(out)
	return out
}

// exitCodeSet is the exact set of exit codes the CLI documents: success, the
// generic error, and the single ambiguous-ticket-number code.
func exitCodeSet() []string {
	// 0 = success, 1 = error, 2 = ambiguous ticket number — all three are
	// named constants in exitcodes.go (exitOK, exitError, exitAmbiguous, see
	// T-0161/T-0157), so the set is read from there instead of being
	// hardcoded and cannot drift from the production code.
	want := []string{strconv.Itoa(exitOK), strconv.Itoa(exitError), strconv.Itoa(exitAmbiguous)}
	slices.Sort(want)
	return want
}

// TestUsageExitCodesDocumented guards the exit-code documentation: both
// templates must carry the paragraph and must document exactly the code set
// from exitcodes.go. If the constant changes and the help text is not
// updated, this fails instead of silently drifting.
func TestUsageExitCodesDocumented(t *testing.T) {
	want := exitCodeSet()
	cases := []struct {
		name   string
		text   string
		marker string
	}{
		{"RU", usageTextRU, "Коды выхода:"},
		{"EN", usageTextEN, "Exit codes:"},
	}
	for _, tc := range cases {
		para := usageParagraph(t, tc.text, tc.marker)
		got := exitCodeSegments(para)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%s: exit code set mismatch: got %v, want %v: %q", tc.name, got, want, para)
		}
	}
}

// TestUsageExitCodesParity keeps the RU and EN exit-code paragraphs the same
// height: a translation that gains or loses segments breaks the byte-identical
// layout shared by the two templates.
func TestUsageExitCodesParity(t *testing.T) {
	ru := usageParagraph(t, usageTextRU, "Коды выхода:")
	en := usageParagraph(t, usageTextEN, "Exit codes:")
	a, b := len(exitCodeSegments(ru)), len(exitCodeSegments(en))
	if a != b {
		t.Errorf("exit-code segment count differs: RU=%d segments, EN=%d segments", a, b)
	}
}

// usageModuleRoot derives the module root from this file's compile-time path.
// The sibling helper moduleRoot in integration_version_test.go lives in package
// cli_test and is therefore not visible here. runtime.Caller(0) records the
// source path at build time: -trimpath builds yield a module-relative path and
// a binary moved off the build machine points at a nonexistent path — both
// fail the go.mod Stat check.
func usageModuleRoot() (string, bool) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", false
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return "", false
	}
	return root, true
}

// requireUsageModuleRoot skips when the module root cannot be located: the
// documented copies live in the source tree, which a cross-compiled or moved
// test binary does not carry along.
func requireUsageModuleRoot(t *testing.T) string {
	t.Helper()
	root, ok := usageModuleRoot()
	if !ok {
		t.Skipf("cannot locate the ticket module root (binary moved off the build machine or built with -trimpath); the docs copies live in the source tree, so this test cannot verify them here")
	}
	return root
}

// usageFence extracts the fenced help text from a docs file: the first
// ```text fence whose body contains marker, up to the closing fence. Searching
// the marker inside the fences (not in the whole file) keeps prose that quotes
// the marker above the block from hijacking the lookup. The «ticket version
// <ver>» placeholder line (reference.md only) is dropped because the binary
// prepends the real version at runtime; wantVersion states how many such lines
// the file is expected to carry, so a lost placeholder fails loudly instead of
// passing silently.
func usageFence(t *testing.T, path, marker string, wantVersion int) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.Split(string(data), "\n")
	var body []string
	found := false
	for i, line := range lines {
		if line != "```text" {
			continue
		}
		end := -1
		for j := i + 1; j < len(lines); j++ {
			if lines[j] == "```" {
				end = j
				break
			}
		}
		if end < 0 {
			t.Fatalf("%s:%d: fenced help text has no closing ```", path, i+1)
		}
		candidate := lines[i+1 : end]
		for _, l := range candidate {
			if strings.HasPrefix(l, marker) {
				body = candidate
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Fatalf("%s: no ```text fence containing marker %q", path, marker)
	}
	var out []string
	versions := 0
	for _, line := range body {
		if strings.HasPrefix(line, "ticket version ") {
			versions++
			continue
		}
		out = append(out, line)
	}
	if versions != wantVersion {
		t.Fatalf("%s: found %d `ticket version ` placeholder lines inside the fence, want %d", path, versions, wantVersion)
	}
	return strings.Join(out, "\n") + "\n"
}

// firstDiffLine returns the 1-based number and both sides of the first line
// where got and want diverge; it reports 0 when the inputs are identical. When
// one text ends before the other, the missing line is reported together with
// an explanatory note, so a truncated copy is not diagnosed as an empty line.
func firstDiffLine(got, want string) (int, string, string, string) {
	gotLines := strings.Split(got, "\n")
	wantLines := strings.Split(want, "\n")
	n := min(len(gotLines), len(wantLines))
	for i := range n {
		if gotLines[i] != wantLines[i] {
			return i + 1, gotLines[i], wantLines[i], ""
		}
	}
	switch {
	case len(gotLines) < len(wantLines):
		return n + 1, "", wantLines[n], fmt.Sprintf("text ends after line %d, missing want line %d", n, n+1)
	case len(gotLines) > len(wantLines):
		return n + 1, gotLines[n], "", fmt.Sprintf("text ends after line %d, want ends there", n)
	}
	return 0, "", "", ""
}

// TestUsageDocCopiesMatchTemplates keeps the help copies in docs and in the
// skill byte-identical to the embedded templates they advertise as verbatim.
// Paths are resolved against the module root, not the working directory, so
// the test behaves the same from anywhere.
func TestUsageDocCopiesMatchTemplates(t *testing.T) {
	root := requireUsageModuleRoot(t)
	cases := []struct {
		parts    []string
		marker   string
		template string
		wantVer  int
	}{
		{[]string{"docs", "usage.md"}, "ticket — project tickets", usageTextEN, 0},
		{[]string{"docs", "ru", "usage.md"}, "ticket — тикеты проекта", usageTextRU, 0},
		{[]string{"skills", "ticket-cli", "reference.md"}, "ticket — тикеты проекта", usageTextRU, 1},
	}
	for _, tc := range cases {
		path := filepath.Join(append([]string{root}, tc.parts...)...)
		got := usageFence(t, path, tc.marker, tc.wantVer)
		if got == tc.template {
			continue
		}
		line, gotLine, wantLine, note := firstDiffLine(got, tc.template)
		msg := fmt.Sprintf("%s: fenced help text differs from the template (got %d bytes, want %d); first diff at line %d: got %q, want %q", path, len(got), len(tc.template), line, gotLine, wantLine)
		if note != "" {
			msg += " (" + note + ")"
		}
		t.Errorf("%s", msg)
	}
}

// readmeExitCodeSection returns the body lines of a level-2 README section
// such as «## Exit codes» up to the next level-2 heading.
func readmeExitCodeSection(t *testing.T, path, heading string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var out []string
	in := false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "## ") {
			if in {
				break
			}
			in = line == heading
			continue
		}
		if in {
			out = append(out, line)
		}
	}
	if !in {
		t.Fatalf("%s: no %q section", path, heading)
	}
	return out
}

// readmeExitCodes extracts the numbers documented as «- `N`» bullets of a
// README exit-code section, sorted.
func readmeExitCodes(t *testing.T, path, heading string) []string {
	t.Helper()
	var out []string
	for _, line := range readmeExitCodeSection(t, path, heading) {
		if m := exitCodeBulletRe.FindStringSubmatch(line); m != nil {
			out = append(out, m[1])
		}
	}
	slices.Sort(out)
	return out
}

// TestReadmeExitCodesDocumented extends the exit-code guard to the top-level
// READMEs: a new code that reaches the templates but not the README (or a
// stale bullet left behind) fails here.
func TestReadmeExitCodesDocumented(t *testing.T) {
	root := requireUsageModuleRoot(t)
	want := exitCodeSet()
	cases := []struct {
		file    string
		heading string
	}{
		{"README.md", "## Exit codes"},
		{"README.ru.md", "## Коды выхода"},
	}
	for _, tc := range cases {
		path := filepath.Join(root, tc.file)
		got := readmeExitCodes(t, path, tc.heading)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: documented exit codes mismatch: got %v, want %v", path, got, want)
		}
	}
}
