package cli_test

// T-0215 integration: the real binary resolves a SINGLE `set` comment token
// from a file or from stdin (the same resolveDetails rules as `new -d`),
// keeps the legacy space-join for 2+ tokens, and mutates nothing on a read
// failure. Only TempDir sandboxes are used.

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// setSandbox creates a sandbox tickets dir holding one open ticket and
// returns the tickets dir, a foreign cwd and the ticket path.
func setSandbox(t *testing.T) (string, string, string) {
	t.Helper()
	tickets := t.TempDir()
	cwd := t.TempDir()
	out, stderr, code := runBinEnv(t, ticketBin, cwd, tickets, []string{"TICKET_LANG=en"}, "new", "Set comment sandbox")
	if code != 0 {
		t.Fatalf("new: code=%d stderr=%q", code, stderr)
	}
	assertEmptyDir(t, cwd, "foreign cwd")
	path := strings.TrimSpace(out)
	return tickets, cwd, path
}

// readCause returns the cause a real os.ReadFile reports for path, so the
// expected diagnostic names the path once and the OS cause exactly.
func readCause(t *testing.T, path string) error {
	t.Helper()
	_, err := os.ReadFile(path)
	var pe *fs.PathError
	if !errors.As(err, &pe) {
		t.Fatalf("read error %v is not a *fs.PathError", err)
	}
	return pe.Err
}

// writeCommentFile writes payload to a fresh file and returns its path.
func writeCommentFile(t *testing.T, payload string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "comment.txt")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write comment file: %v", err)
	}
	return path
}

// setSandboxState snapshots the tickets sandbox recursively: relative entry
// paths plus the bytes of every regular file, so writes into subdirectories
// (archive/) are detected too. A failed set must leave it byte-identical.
func setSandboxState(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		b.WriteString(rel)
		b.WriteString("\n")
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		b.Write(data)
		b.WriteString("\n")
		return nil
	})
	if err != nil {
		t.Fatalf("walk sandbox %q: %v", dir, err)
	}
	return b.String()
}

// TestSetCommentFromFileIntegration pins the "@<file>" source end to end:
// the file bytes land verbatim in the journal transition line and the
// ticket is renamed to the new status.
func TestSetCommentFromFileIntegration(t *testing.T) {
	requireBin(t)
	tickets, cwd, oldPath := setSandbox(t)
	commentPath := writeCommentFile(t, "  padded  ")
	out, stderr, code := runBinEnv(t, ticketBin, cwd, tickets, []string{"TICKET_LANG=en"},
		"set", "1", "wip", "@"+commentPath)
	if code != 0 {
		t.Fatalf("set: code=%d stderr=%q", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	wantPath := filepath.Join(filepath.Dir(oldPath), "T-0001-wip.md")
	if got := strings.TrimSpace(out); got != wantPath {
		t.Fatalf("stdout = %q, want %q", got, wantPath)
	}
	if body := readCreatedTicket(t, out); !strings.Contains(string(body), "·   padded   (agent)") {
		t.Fatalf("journal must keep the file bytes verbatim, no trimming:\n%s", body)
	}
}

// TestSetCommentFromStdinIntegration pins the single "-" token end to end:
// stdin is read and its bytes reach the journal.
func TestSetCommentFromStdinIntegration(t *testing.T) {
	requireBin(t)
	tickets, cwd, _ := setSandbox(t)
	out, stderr, code := runBinWithStdin(t, ticketBin, cwd, tickets, "comment from stdin", "set", "1", "wip", "-")
	if code != 0 {
		t.Fatalf("set: code=%d stderr=%q", code, stderr)
	}
	if body := readCreatedTicket(t, out); !strings.Contains(string(body), "· comment from stdin (agent)") {
		t.Fatalf("journal missing the stdin comment:\n%s", body)
	}
}

// TestSetCommentDoubleAtIntegration pins "@@name" end to end: one "@" is
// stripped, so the file NAMED "@name" is read.
func TestSetCommentDoubleAtIntegration(t *testing.T) {
	requireBin(t)
	tickets, _, _ := setSandbox(t)
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "@name"), []byte("at-named file"), 0o644); err != nil {
		t.Fatalf("write @name file: %v", err)
	}
	out, stderr, code := runBinWithStdin(t, ticketBin, cwd, tickets, "", "set", "1", "wip", "@@name")
	if code != 0 {
		t.Fatalf("set: code=%d stderr=%q", code, stderr)
	}
	if body := readCreatedTicket(t, out); !strings.Contains(string(body), "· at-named file (agent)") {
		t.Fatalf("journal missing the @@name comment:\n%s", body)
	}
}

// TestSetCommentMultiTokenLiteralIntegration pins the gate: with two comment
// tokens the arguments are joined literally, so an "@"-prefixed token is NOT
// a file (even when the file exists) and "-x" stays literal.
func TestSetCommentMultiTokenLiteralIntegration(t *testing.T) {
	requireBin(t)
	tickets, cwd, _ := setSandbox(t)
	commentPath := writeCommentFile(t, "must not be read")
	out, stderr, code := runBinEnv(t, ticketBin, cwd, tickets, []string{"TICKET_LANG=en"},
		"set", "1", "wip", "@"+commentPath, "tail")
	if code != 0 {
		t.Fatalf("set: code=%d stderr=%q", code, stderr)
	}
	body := string(readCreatedTicket(t, out))
	if !strings.Contains(body, "· @"+commentPath+" tail (agent)") {
		t.Fatalf("journal missing the joined literal comment:\n%s", body)
	}
	if strings.Contains(body, "· must not be read") {
		t.Fatalf("the multi-token form read the file:\n%s", body)
	}
}

// TestSetCommentFailureNoMutationIntegration pins the failure contract end
// to end (English diagnostics): exit 1, the exact localized message and an
// untouched sandbox — no rename, no journal line.
func TestSetCommentFailureNoMutationIntegration(t *testing.T) {
	requireBin(t)
	cases := []struct {
		name string
		args func(t *testing.T) []string
		want func(t *testing.T, args []string) string
	}{
		{
			"bare at",
			func(*testing.T) []string { return []string{"set", "1", "wip", "@"} },
			func(*testing.T, []string) string { return domain.ErrDetailsAtBare(domain.LangEN) + "\n" },
		},
		{
			"missing file",
			func(t *testing.T) []string {
				return []string{"set", "1", "wip", "@" + filepath.Join(t.TempDir(), "nope.txt")}
			},
			func(t *testing.T, args []string) string {
				path := strings.TrimPrefix(args[3], "@")
				return domain.ErrDetailsFileRead(domain.LangEN, path, readCause(t, path)) + "\n"
			},
		},
		{
			"crlf file",
			func(t *testing.T) []string {
				return []string{"set", "1", "wip", "@" + writeCommentFile(t, "forged\r\n- entry")}
			},
			func(*testing.T, []string) string { return domain.ErrJournalNewline(domain.LangEN) + "\n" },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tickets, cwd, oldPath := setSandbox(t)
			before := setSandboxState(t, tickets)
			args := tc.args(t)
			out, stderr, code := runBinEnv(t, ticketBin, cwd, tickets, []string{"TICKET_LANG=en"}, args...)
			if code != 1 {
				t.Fatalf("set: code=%d, want 1; stderr=%q", code, stderr)
			}
			if out != "" {
				t.Fatalf("stdout = %q, want empty", out)
			}
			if want := tc.want(t, args); stderr != want {
				t.Fatalf("stderr = %q, want %q", stderr, want)
			}
			if after := setSandboxState(t, tickets); after != before {
				t.Fatalf("sandbox changed on a failed set:\nbefore=%s\nafter=%s", before, after)
			}
			if _, err := os.Stat(oldPath); err != nil {
				t.Fatalf("original ticket path must survive: %v", err)
			}
		})
	}
}

// TestSetCommentUnreadableFileIntegration pins an unreadable "@<file>":
// exit 1, the localized read message, no mutation. chmod-based, so it skips
// where permissions cannot hold (root, Windows).
func TestSetCommentUnreadableFileIntegration(t *testing.T) {
	requireBin(t)
	skipRootOrWindows(t)
	tickets, cwd, oldPath := setSandbox(t)
	commentPath := writeCommentFile(t, "secret")
	if err := os.Chmod(commentPath, 0o000); err != nil {
		t.Fatalf("chmod comment file: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(commentPath, 0o644) })
	before := setSandboxState(t, tickets)
	out, stderr, code := runBinEnv(t, ticketBin, cwd, tickets, []string{"TICKET_LANG=en"},
		"set", "1", "wip", "@"+commentPath)
	if code != 1 {
		t.Fatalf("set: code=%d, want 1; stderr=%q", code, stderr)
	}
	if out != "" {
		t.Fatalf("stdout = %q, want empty", out)
	}
	if want := domain.ErrDetailsFileRead(domain.LangEN, commentPath, readCause(t, commentPath)) + "\n"; stderr != want {
		t.Fatalf("stderr = %q, want %q", stderr, want)
	}
	if after := setSandboxState(t, tickets); after != before {
		t.Fatalf("sandbox changed on a failed set:\nbefore=%s\nafter=%s", before, after)
	}
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("original ticket path must survive: %v", err)
	}
}
