package cli

// T-0215: `set <номер> <статус> <комментарий>` resolves a SINGLE comment
// token through the shared resolveDetails ("-" = stdin, "@path" = file,
// one "@" stripped), exactly like `new -d`; zero or 2+ tokens keep the
// legacy space-join. The helpers detailsStore, withStdin, failingReader
// and fileReadMessage are reused from cmd_new_details_test.go.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// setFixture creates one open ticket through cmdNew in a fresh sandbox and
// returns the store, the sandbox dir and the created ticket path.
func setFixture(t *testing.T) (*store.Store, string, string) {
	t.Helper()
	st, dir := detailsStore(t)
	var stdout, stderr bytes.Buffer
	if rc := cmdNew(st, []string{"Тикет с комментарием"}, "tester", "prj", domain.LangRU, &stdout, &stderr); rc != 0 {
		t.Fatalf("cmdNew = %d, want 0; stderr: %q", rc, stderr.String())
	}
	return st, dir, strings.TrimSpace(stdout.String())
}

// sandboxState snapshots the sandbox: every entry name plus the bytes of
// every regular file. A failed set must leave it byte-identical (no rename,
// no journal line, no new file).
func sandboxState(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read sandbox %q: %v", dir, err)
	}
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "%s\n", e.Name())
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %q: %v", e.Name(), err)
		}
		b.Write(data)
		b.WriteString("\n")
	}
	return b.String()
}

// runSetCmd drives cmdSet and returns stdout, stderr and the exit code.
func runSetCmd(st *store.Store, lang domain.Lang, args ...string) (string, string, int) {
	var stdout, stderr bytes.Buffer
	rc := cmdSet(st, args, "tester", "", lang, &stdout, &stderr)
	return stdout.String(), stderr.String(), rc
}

// TestSetCommentFromFile pins the file source: "@<path>" is read byte-exact
// and shows up in the journal transition line, exactly as a literal would.
func TestSetCommentFromFile(t *testing.T) {
	st, _, path := setFixture(t)
	commentFile := filepath.Join(t.TempDir(), "comment.txt")
	if err := os.WriteFile(commentFile, []byte("из файла"), 0o644); err != nil {
		t.Fatalf("write comment file: %v", err)
	}
	stdout, stderr, rc := runSetCmd(st, domain.LangRU, "1", "wip", "@"+commentFile)
	if rc != 0 || stderr != "" {
		t.Fatalf("cmdSet = %d, stderr %q; want 0 and empty stderr", rc, stderr)
	}
	if want := filepath.Join(filepath.Dir(path), "T-0001-wip.md"); strings.TrimSpace(stdout) != want {
		t.Fatalf("stdout = %q, want %q", strings.TrimSpace(stdout), want)
	}
	body := readTicket(t, strings.TrimSpace(stdout))
	if !strings.Contains(body, "· из файла (tester)") {
		t.Fatalf("journal missing the file comment:\n%s", body)
	}
}

// TestSetCommentFromStdin pins the stdin source: a single "-" token reads
// stdin (the seam) and the payload lands in the journal verbatim.
func TestSetCommentFromStdin(t *testing.T) {
	st, _, path := setFixture(t)
	withStdin(t, strings.NewReader("из stdin"))
	stdout, stderr, rc := runSetCmd(st, domain.LangRU, "1", "wip", "-")
	if rc != 0 || stderr != "" {
		t.Fatalf("cmdSet = %d, stderr %q; want 0 and empty stderr", rc, stderr)
	}
	if want := filepath.Join(filepath.Dir(path), "T-0001-wip.md"); strings.TrimSpace(stdout) != want {
		t.Fatalf("stdout = %q, want %q", strings.TrimSpace(stdout), want)
	}
	if body := readTicket(t, strings.TrimSpace(stdout)); !strings.Contains(body, "· из stdin (tester)") {
		t.Fatalf("journal missing the stdin comment:\n%s", body)
	}
}

// TestSetCommentDoubleAtReadsAtName pins the "@@name" form: exactly one "@"
// marker is stripped, so the file NAMED "@name" is read (no escape).
func TestSetCommentDoubleAtReadsAtName(t *testing.T) {
	st, _, path := setFixture(t)
	work := t.TempDir()
	if err := os.WriteFile(filepath.Join(work, "@name"), []byte("файл с собакой"), 0o644); err != nil {
		t.Fatalf("write @name file: %v", err)
	}
	t.Chdir(work)
	stdout, stderr, rc := runSetCmd(st, domain.LangRU, "1", "wip", "@@name")
	if rc != 0 || stderr != "" {
		t.Fatalf("cmdSet = %d, stderr %q; want 0 and empty stderr", rc, stderr)
	}
	if want := filepath.Join(filepath.Dir(path), "T-0001-wip.md"); strings.TrimSpace(stdout) != want {
		t.Fatalf("stdout = %q, want %q", strings.TrimSpace(stdout), want)
	}
	if body := readTicket(t, strings.TrimSpace(stdout)); !strings.Contains(body, "· файл с собакой (tester)") {
		t.Fatalf("journal missing the @@name comment:\n%s", body)
	}
}

// TestSetCommentEmptySourceKeepsSameStatusRules pins the empty payload
// contract: an empty stdin yields an EMPTY comment, so a same-status set
// hits the unchanged pinned "already in status" error (T-0077).
func TestSetCommentEmptySourceKeepsSameStatusRules(t *testing.T) {
	st, dir, _ := setFixture(t)
	before := sandboxState(t, dir)
	withStdin(t, strings.NewReader(""))
	_, stderr, rc := runSetCmd(st, domain.LangRU, "1", "open", "-")
	if rc != 1 {
		t.Fatalf("cmdSet = %d, want 1", rc)
	}
	if want := domain.ErrTicketAlreadyStatus(domain.LangRU, 1, "open") + "\n"; stderr != want {
		t.Fatalf("stderr = %q, want %q", stderr, want)
	}
	if after := sandboxState(t, dir); after != before {
		t.Fatalf("sandbox changed on a failed set:\nbefore=%s\nafter=%s", before, after)
	}
}

// TestSetCommentMultiTokenJoin pins the gate: two or more comment tokens are
// joined with spaces and NEVER resolved, so "@missing" stays literal, a "-"
// token does not read stdin (the reader fails if touched) and "-x" survives.
func TestSetCommentMultiTokenJoin(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"two words", []string{"1", "wip", "два", "слова"}, "· два слова (tester)"},
		{"at-prefixed literal", []string{"1", "wip", "@missing", "tail"}, "· @missing tail (tester)"},
		{"dash token does not read stdin", []string{"1", "wip", "-", "хвост"}, "· - хвост (tester)"},
		{"flag-like literal single token", []string{"1", "wip", "-x"}, "· -x (tester)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st, _, _ := setFixture(t)
			withStdin(t, failingReader{err: errors.New("stdin должен остаться нетронутым")})
			stdout, stderr, rc := runSetCmd(st, domain.LangRU, tc.args...)
			if rc != 0 || stderr != "" {
				t.Fatalf("cmdSet = %d, stderr %q; want 0 and empty stderr", rc, stderr)
			}
			if body := readTicket(t, strings.TrimSpace(stdout)); !strings.Contains(body, tc.want) {
				t.Fatalf("journal missing %q:\n%s", tc.want, body)
			}
		})
	}
}

// TestSetCommentSourceFailureNoMutation pins the failure contract in both
// languages: the exact localized message, exit 1, and an untouched sandbox —
// the failure lands before the newline guard, the deleted guard and
// SetStatus, so no rename and no journal line happen.
func TestSetCommentSourceFailureNoMutation(t *testing.T) {
	readErr := errors.New("stdin сломан")
	for _, lang := range []domain.Lang{domain.LangRU, domain.LangEN} {
		cases := []struct {
			name  string
			token func(t *testing.T) string
			stdin io.Reader
			want  func(t *testing.T, path string) string
		}{
			{
				"bare at",
				func(*testing.T) string { return "@" },
				nil,
				func(*testing.T, string) string { return domain.ErrDetailsAtBare(lang) + "\n" },
			},
			{
				"missing file",
				func(t *testing.T) string { return "@" + filepath.Join(t.TempDir(), "нет-такого.txt") },
				nil,
				func(t *testing.T, token string) string {
					return fileReadMessage(t, lang, strings.TrimPrefix(token, "@"))
				},
			},
			{
				"stdin read failure",
				func(*testing.T) string { return "-" },
				failingReader{err: readErr},
				func(*testing.T, string) string { return domain.ErrDetailsStdinRead(lang, readErr) + "\n" },
			},
			{
				"file with LF",
				func(t *testing.T) string { return commentFile(t, "строка\nвторая") },
				nil,
				func(*testing.T, string) string { return domain.ErrJournalNewline(lang) + "\n" },
			},
			{
				"file with CRLF",
				func(t *testing.T) string { return commentFile(t, "строка\r\nвторая") },
				nil,
				func(*testing.T, string) string { return domain.ErrJournalNewline(lang) + "\n" },
			},
			{
				"stdin with LF",
				func(*testing.T) string { return "-" },
				strings.NewReader("подделка\n- entry"),
				func(*testing.T, string) string { return domain.ErrJournalNewline(lang) + "\n" },
			},
		}
		for _, tc := range cases {
			t.Run(string(lang)+" "+tc.name, func(t *testing.T) {
				st, dir, path := setFixture(t)
				if tc.stdin != nil {
					withStdin(t, tc.stdin)
				}
				before := sandboxState(t, dir)
				token := tc.token(t)
				stdout, stderr, rc := runSetCmd(st, lang, "1", "wip", token)
				if rc != 1 {
					t.Fatalf("cmdSet = %d, want 1; stderr %q", rc, stderr)
				}
				if stdout != "" {
					t.Fatalf("stdout = %q, want empty", stdout)
				}
				if want := tc.want(t, token); stderr != want {
					t.Fatalf("stderr = %q, want %q", stderr, want)
				}
				if after := sandboxState(t, dir); after != before {
					t.Fatalf("sandbox changed on a failed set:\nbefore=%s\nafter=%s", before, after)
				}
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("original ticket path must survive: %v", err)
				}
			})
		}
	}
}

// readTicket reads the ticket file at path and returns its text.
func readTicket(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ticket %q: %v", path, err)
	}
	return string(data)
}

// commentFile writes payload to a fresh file and returns its "@" token.
func commentFile(t *testing.T, payload string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "comment.txt")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write comment file: %v", err)
	}
	return "@" + path
}
