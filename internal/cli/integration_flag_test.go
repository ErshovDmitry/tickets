package cli_test

// T-0057 §C2b-4: the global --tickets-dir/-C flag against the real binary
// from a foreign cwd — all four recognized forms, precedence over
// $TICKETS_DIR, and the not-resolved error text. One shared tickets
// fixture for the whole function: the first subtest creates T-0001, the
// rest reuse the directory (mutating subtests assert sequential numbers).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFlagTicketsDirForeignCWD covers the global path flag end-to-end.
// The flag is recognized only BEFORE the command name (git-style); the
// child env carries no TICKETS_DIR unless a subtest passes ticketsDir.
func TestFlagTicketsDirForeignCWD(t *testing.T) {
	requireBin(t)
	tickets := t.TempDir()
	cwd := t.TempDir()
	ticketsEval := mustEval(t, tickets)

	// long separate form creates T-0001 in the shared fixture.
	out, stderr, code := runBin(t, ticketBin, cwd, "", "--tickets-dir", tickets, "new", "Флаг тикет")
	if code != 0 {
		t.Fatalf("new --tickets-dir: code=%d stderr=%q", code, stderr)
	}
	if want := filepath.Join(ticketsEval, "T-0001-open.md") + "\n"; out != want {
		t.Fatalf("new stdout = %q, want %q", out, want)
	}
	assertEmptyDir(t, cwd, "foreign cwd")

	// The three remaining recognized forms list the same fixture.
	listShowsFirst := func(name string, args ...string) {
		t.Run(name, func(t *testing.T) {
			out, stderr, code := runBin(t, ticketBin, cwd, "", args...)
			if code != 0 {
				t.Fatalf("list: code=%d stderr=%q", code, stderr)
			}
			if !strings.Contains(out, "T-0001") {
				t.Errorf("list output=%q missing T-0001", out)
			}
		})
	}
	listShowsFirst("long equals", "--tickets-dir="+tickets, "list")
	listShowsFirst("short separate", "-C", tickets, "list")
	listShowsFirst("short attached", "-C"+tickets, "list")

	// Global flags are parsed only before the command: `list -C <dir>`
	// hands -C to cmdList as a plain argument; from a foreign cwd the
	// resolver (which runs before arg parsing) fails first.
	t.Run("flag not extracted after cmd", func(t *testing.T) {
		_, stderr, code := runBin(t, ticketBin, cwd, "", "list", "-C", tickets)
		if code != 1 {
			t.Fatalf("list -C after cmd: code=%d, want 1", code)
		}
		if !strings.Contains(stderr, "cannot locate tickets dir") {
			t.Errorf("stderr=%q missing resolution error", stderr)
		}
		assertEmptyDir(t, cwd, "foreign cwd")
	})

	// A -d value starting with "-C" must be preserved verbatim: the
	// global flag scan stops at the command name.
	t.Run("-d value preserved", func(t *testing.T) {
		out, stderr, code := runBin(t, ticketBin, cwd, "", "-C", tickets, "new", "x", "-d", "-C foo")
		if code != 0 {
			t.Fatalf("new: code=%d stderr=%q", code, stderr)
		}
		if want := filepath.Join(ticketsEval, "T-0002-open.md") + "\n"; out != want {
			t.Fatalf("new stdout = %q, want %q", out, want)
		}
		data, err := os.ReadFile(filepath.Join(ticketsEval, "T-0002-open.md"))
		if err != nil {
			t.Fatalf("ReadFile T-0002: %v", err)
		}
		if !strings.Contains(string(data), "-C foo") {
			t.Errorf("details not preserved, file=%q", data)
		}
		assertEmptyDir(t, cwd, "foreign cwd")
	})

	// set/show work through the flag: rename lands in the flagged dir.
	t.Run("set and show via flag", func(t *testing.T) {
		out, stderr, code := runBin(t, ticketBin, cwd, "", "-C", tickets, "show", "1")
		if code != 0 {
			t.Fatalf("show: code=%d stderr=%q", code, stderr)
		}
		if !strings.Contains(out, "Флаг тикет") {
			t.Errorf("show output=%q missing title", out)
		}
		out, stderr, code = runBin(t, ticketBin, cwd, "", "-C", tickets, "set", "1", "wip", "в работу")
		if code != 0 {
			t.Fatalf("set: code=%d stderr=%q", code, stderr)
		}
		if _, err := os.Stat(filepath.Join(ticketsEval, "T-0001-wip.md")); err != nil {
			t.Fatalf("T-0001-wip.md missing after set: %v", err)
		}
		assertEmptyDir(t, cwd, "foreign cwd")
	})

	// The flag beats $TICKETS_DIR (hub decision #2): <other> stays empty.
	t.Run("flag beats env", func(t *testing.T) {
		other := t.TempDir()
		out, stderr, code := runBin(t, ticketBin, cwd, other, "-C", tickets, "new", "Env перекрыт")
		if code != 0 {
			t.Fatalf("new: code=%d stderr=%q", code, stderr)
		}
		if want := filepath.Join(ticketsEval, "T-0003-open.md") + "\n"; out != want {
			t.Fatalf("new stdout = %q, want %q", out, want)
		}
		assertEmptyDir(t, other, "env-only dir")
		assertEmptyDir(t, cwd, "foreign cwd")
	})

	// Without any hint the resolver fails with flag+env guidance.
	t.Run("no flag, foreign cwd", func(t *testing.T) {
		_, stderr, code := runBin(t, ticketBin, cwd, "", "list")
		if code != 1 {
			t.Fatalf("list: code=%d, want 1", code)
		}
		for _, want := range []string{"cannot locate tickets dir", "--tickets-dir", "TICKETS_DIR"} {
			if !strings.Contains(stderr, want) {
				t.Errorf("stderr=%q missing %q", stderr, want)
			}
		}
	})
}
