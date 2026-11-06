package cli_test

// T-0109 integration: the real binary resolves the `-d` details argument
// from a file, from stdin or literally; the resolver preserves the input
// bytes unchanged, while rendering normalizes only trailing LF (T-0131).
// The Details region is compared as RAW bytes (no domain.Parse).

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// detailsHeaderEN and nextHeaderEN are the known English section headers
// of a new ticket (internal/domain/langs/en.json): the Details section is
// followed by the User comments section, so the raw bytes between them
// are exactly the details payload.
const (
	detailsHeaderEN = "## Details\n\n"
	nextHeaderEN    = "## User comments"
)

// runBinWithStdin runs the real binary with stdin piped from stdinBytes.
// runBin (integration_test.go) has no stdin support, so this local helper
// adds it. The child env is hermetic: TICKETS_DIR pins the sandbox and
// TICKET_LANG=en pins the English headers used by detailsRegion.
func runBinWithStdin(t *testing.T, bin, dir, ticketsDir, stdinBytes string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = []string{"TICKETS_DIR=" + ticketsDir, "TICKET_LANG=en"}
	cmd.Stdin = strings.NewReader(stdinBytes)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("run %v: %v", args, err)
		}
		code = ee.ExitCode()
	}
	return stdout.String(), stderr.String(), code
}

// detailsRegion returns the raw bytes of the Details section of a created
// ticket: everything between the "## Details" header and the next known
// header. The template writes the details, then a blank line, then the
// next header, so exactly that blank-line separator is dropped.
func detailsRegion(t *testing.T, raw []byte) string {
	t.Helper()
	body := string(raw)
	start := strings.Index(body, detailsHeaderEN)
	if start < 0 {
		t.Fatalf("ticket file has no %q header:\n%s", strings.TrimSpace(detailsHeaderEN), body)
	}
	start += len(detailsHeaderEN)
	end := strings.Index(body[start:], nextHeaderEN)
	if end < 0 {
		t.Fatalf("ticket file has no %q header after the Details section:\n%s", nextHeaderEN, body)
	}
	return strings.TrimSuffix(body[start:start+end], "\n\n")
}

// readCreatedTicket reads the file the binary printed (the path of the
// created ticket) and returns its raw bytes.
func readCreatedTicket(t *testing.T, stdout string) []byte {
	t.Helper()
	path := strings.TrimSpace(stdout)
	if path == "" {
		t.Fatal("the binary printed no ticket path")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read created ticket %q: %v", path, err)
	}
	return raw
}

// largeDetailsPayload builds a numbered details payload larger than
// 64 KiB. No line starts with "## " — the section headers must stay
// unambiguous so the raw Details region can be sliced.
func largeDetailsPayload(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	for i := 1; b.Len() <= 64*1024; i++ {
		fmt.Fprintf(&b, "%d. пункт %d — src/file.go:%d; решение\n", i, i, i)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// TestNewDetailsFromFile covers the "@<file>" form: the created ticket
// holds the file bytes verbatim.
func TestNewDetailsFromFile(t *testing.T) {
	requireBin(t)
	tickets := t.TempDir()
	cwd := t.TempDir()
	payload := "1. first — a.go:1; fixed\n2. second — a.go:2; fixed"
	detailsFile := filepath.Join(t.TempDir(), "details.md")
	if err := os.WriteFile(detailsFile, []byte(payload), 0o644); err != nil {
		t.Fatalf("write details file: %v", err)
	}
	out, stderr, code := runBinEnv(t, ticketBin, cwd, tickets, []string{"TICKET_LANG=en"},
		"new", "Details from file", "-d", "@"+detailsFile)
	if code != 0 {
		t.Fatalf("new: code=%d stderr=%q", code, stderr)
	}
	assertEmptyDir(t, cwd, "foreign cwd")
	if got := detailsRegion(t, readCreatedTicket(t, out)); got != payload {
		t.Fatalf("Details region = %q, want %q", got, payload)
	}
}

// TestNewDetailsAtMarkerStrippedOnce covers the "@@name" form: exactly one
// "@" is stripped, so "@@name" reads the file named "@name" (it is NOT an
// escape sequence).
func TestNewDetailsAtMarkerStrippedOnce(t *testing.T) {
	requireBin(t)
	tickets := t.TempDir()
	cwd := t.TempDir()
	payload := "literal content of the @-named file"
	detailsFile := filepath.Join(t.TempDir(), "@name")
	if err := os.WriteFile(detailsFile, []byte(payload), 0o644); err != nil {
		t.Fatalf("write details file: %v", err)
	}
	out, stderr, code := runBinEnv(t, ticketBin, cwd, tickets, []string{"TICKET_LANG=en"},
		"new", "Details from @@name", "-d", "@"+detailsFile)
	if code != 0 {
		t.Fatalf("new: code=%d stderr=%q", code, stderr)
	}
	assertEmptyDir(t, cwd, "foreign cwd")
	if got := detailsRegion(t, readCreatedTicket(t, out)); got != payload {
		t.Fatalf("Details region = %q, want %q", got, payload)
	}
}

// TestNewDetailsFromStdin covers the "-" form: the piped bytes land in the
// Details section unchanged.
func TestNewDetailsFromStdin(t *testing.T) {
	requireBin(t)
	tickets := t.TempDir()
	cwd := t.TempDir()
	payload := "1. piped — b.go:7; fixed\nsecond line"
	out, stderr, code := runBinWithStdin(t, ticketBin, cwd, tickets, payload,
		"new", "Details from stdin", "-d", "-")
	if code != 0 {
		t.Fatalf("new: code=%d stderr=%q", code, stderr)
	}
	assertEmptyDir(t, cwd, "foreign cwd")
	if got := detailsRegion(t, readCreatedTicket(t, out)); got != payload {
		t.Fatalf("Details region = %q, want %q", got, payload)
	}
}

// TestNewDetailsLiteral covers the plain form: a literal argument is taken
// as the details text as is.
func TestNewDetailsLiteral(t *testing.T) {
	requireBin(t)
	tickets := t.TempDir()
	cwd := t.TempDir()
	payload := "literal details text"
	out, stderr, code := runBinEnv(t, ticketBin, cwd, tickets, []string{"TICKET_LANG=en"},
		"new", "Literal details", "-d", payload)
	if code != 0 {
		t.Fatalf("new: code=%d stderr=%q", code, stderr)
	}
	assertEmptyDir(t, cwd, "foreign cwd")
	if got := detailsRegion(t, readCreatedTicket(t, out)); got != payload {
		t.Fatalf("Details region = %q, want %q", got, payload)
	}
}

// TestNewDetailsMissingFile covers the failure path: a missing file exits
// 1, names the path in the localized message and creates NO ticket (and no
// journal entry).
func TestNewDetailsMissingFile(t *testing.T) {
	requireBin(t)
	tickets := t.TempDir()
	cwd := t.TempDir()
	missing := filepath.Join(t.TempDir(), "missing.md")
	out, stderr, code := runBinEnv(t, ticketBin, cwd, tickets, []string{"TICKET_LANG=en"},
		"new", "Missing details file", "-d", "@"+missing)
	if code != 1 {
		t.Fatalf("new: code=%d, want 1; stdout=%q stderr=%q", code, out, stderr)
	}
	if !strings.Contains(stderr, "cannot read details file") {
		t.Errorf("stderr = %q, want the localized file-read error", stderr)
	}
	if !strings.Contains(stderr, missing) {
		t.Errorf("stderr = %q, want the missing path %q", stderr, missing)
	}
	if out != "" {
		t.Errorf("stdout = %q, want empty", out)
	}
	entries, err := os.ReadDir(tickets)
	if err != nil {
		t.Fatalf("ReadDir(tickets): %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			t.Fatalf("a ticket file was created: %s", e.Name())
		}
	}
}

// TestNewDetailsLargePayload covers a details payload larger than 64 KiB
// (from a file and from stdin): the bytes survive unchanged and no line
// of the payload may be mistaken for a section header.
func TestNewDetailsLargePayload(t *testing.T) {
	requireBin(t)
	payload := largeDetailsPayload(t)
	if len(payload) <= 64*1024 {
		t.Fatalf("payload is %d bytes, want more than 64 KiB", len(payload))
	}
	for line := range strings.SplitSeq(payload, "\n") {
		if strings.HasPrefix(line, "## ") {
			t.Fatalf("payload line %q looks like a section header", line)
		}
	}

	t.Run("from file", func(t *testing.T) {
		tickets := t.TempDir()
		cwd := t.TempDir()
		detailsFile := filepath.Join(t.TempDir(), "big.md")
		if err := os.WriteFile(detailsFile, []byte(payload), 0o644); err != nil {
			t.Fatalf("write details file: %v", err)
		}
		out, stderr, code := runBinEnv(t, ticketBin, cwd, tickets, []string{"TICKET_LANG=en"},
			"new", "Big details from file", "-d", "@"+detailsFile)
		if code != 0 {
			t.Fatalf("new: code=%d stderr=%q", code, stderr)
		}
		assertEmptyDir(t, cwd, "foreign cwd")
		if got := detailsRegion(t, readCreatedTicket(t, out)); got != payload {
			t.Fatalf("Details region is %d bytes, want the %d payload bytes", len(got), len(payload))
		}
	})

	t.Run("from stdin", func(t *testing.T) {
		tickets := t.TempDir()
		cwd := t.TempDir()
		out, stderr, code := runBinWithStdin(t, ticketBin, cwd, tickets, payload,
			"new", "Big details from stdin", "-d", "-")
		if code != 0 {
			t.Fatalf("new: code=%d stderr=%q", code, stderr)
		}
		assertEmptyDir(t, cwd, "foreign cwd")
		if got := detailsRegion(t, readCreatedTicket(t, out)); got != payload {
			t.Fatalf("Details region is %d bytes, want the %d payload bytes", len(got), len(payload))
		}
	})
}
