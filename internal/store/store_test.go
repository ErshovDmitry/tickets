package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ticket/internal/domain"
)

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

// newStore creates a per-test temp directory and a Store backed by it.
func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	return s, dir
}

// mustRead reads the ticket file whose number equals n inside dir, dying
// on any error.
func mustRead(t *testing.T, dir string, n int) []byte {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, fmt.Sprintf("T-%04d-*.md", n)))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly one ticket %d, got %d files: %v", n, len(matches), matches)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read %s: %v", matches[0], err)
	}
	return data
}

// fakeTicket builds a minimal in-memory ticket that domain.RenderNewTicket
// can render.
func fakeTicket(n int) *domain.Ticket {
	return &domain.Ticket{
		Number:   n,
		Lang:     domain.LangRU,
		Type:     domain.TypeENH,
		Priority: domain.PriorityNormal,
		Title:    fmt.Sprintf("ticket-%d", n),
		Details:  "details for ticket " + fmt.Sprint(n),
		Who:      "tester",
		Project:  "tickets",
		Status:   domain.StatusOpen,
	}
}

// tsLayout is the bash-compatible journal timestamp layout (§2).
const tsLayout = "2006-01-02 15:04"

// assertJournalTransition finds the transition journal line in body and
// asserts it byte-equals the bash reference shape
// "- <ts> — статус: <from> → <to>[ · <comment>] (<who>)", where <ts> must
// be exactly len(tsLayout) chars parsing as tsLayout. No regex involved.
func assertJournalTransition(t *testing.T, body, from, to, comment, who string) {
	t.Helper()
	const marker = " — статус: "
	for _, line := range strings.Split(body, "\n") {
		if !strings.Contains(line, marker) {
			continue
		}
		if !strings.HasPrefix(line, "- ") {
			t.Fatalf("journal line must start with \"- \": %q", line)
		}
		rest := line[2:]
		if i := strings.Index(rest, marker); i != len(tsLayout) {
			t.Fatalf("timestamp must be %d chars: %q", len(tsLayout), line)
		}
		ts := rest[:len(tsLayout)]
		if _, err := time.Parse(tsLayout, ts); err != nil {
			t.Fatalf("timestamp %q does not parse as %q: %v", ts, tsLayout, err)
		}
		want := fmt.Sprintf("- %s — статус: %s → %s", ts, from, to)
		if comment != "" {
			want += " · " + comment
		}
		want += " (" + who + ")"
		if line != want {
			t.Fatalf("journal line byte mismatch\n got: %q\nwant: %q", line, want)
		}
		return
	}
	t.Fatalf("no journal transition line in body:\n%s", body)
}

// swapLink sets linkFile for the duration of t.
func swapLink(t *testing.T, fn func(string, string) error) {
	t.Helper()
	orig := linkFile
	linkFile = fn
	t.Cleanup(func() { linkFile = orig })
}

// swapRemove sets removeFile for the duration of t.
func swapRemove(t *testing.T, fn func(string) error) {
	t.Helper()
	orig := removeFile
	removeFile = fn
	t.Cleanup(func() { removeFile = orig })
}

// assertNoTmpFiles fails the test if dir contains *.tmp entries.
func assertNoTmpFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leaked tmp file: %s", e.Name())
		}
	}
}

// ----------------------------------------------------------------------------
// New — directory validation
// ----------------------------------------------------------------------------

func TestNew_OK(t *testing.T) {
	s, _ := newStore(t)
	if s == nil {
		t.Fatal("expected non-nil Store")
	}
}

func TestNew_NotADirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(file); err == nil {
		t.Fatal("expected error for non-directory")
	}
}

func TestNew_MissingDirectory(t *testing.T) {
	if _, err := New(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected error for missing dir")
	}
}

// ----------------------------------------------------------------------------
// SetStatus — journal byte-shape (d)
// ----------------------------------------------------------------------------

func TestSetStatus_UpdatesNameAndAppendsJournal(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetStatus(1, domain.StatusWip, "tester", "starting"); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	// Old filename must be gone.
	if _, err := os.Lstat(filepath.Join(dir, "T-0001-open.md")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected old filename gone, Lstat err=%v", err)
	}
	// New filename must exist.
	data, err := os.ReadFile(filepath.Join(dir, "T-0001-wip.md"))
	if err != nil {
		t.Fatalf("read new file: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "- Status (Статус): wip") {
		t.Errorf("body missing '- Status (Статус): wip' line\n%s", body)
	}
	assertJournalTransition(t, body, "open", "wip", "starting", "tester")
}

func TestSetStatus_NoComment(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetStatus(1, domain.StatusWip, "tester", ""); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	assertJournalTransition(t, string(mustRead(t, dir, 1)), "open", "wip", "", "tester")
}

func TestSetStatus_PreservesUnknownBytes(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	extra := "\n\n<!-- manual analyst note: do not touch -->\n"
	if err := os.WriteFile(filepath.Join(dir, "T-0001-open.md"), append(mustRead(t, dir, 1), []byte(extra)...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetStatus(1, domain.StatusWip, "tester", "go"); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	body := string(mustRead(t, dir, 1))
	if !strings.Contains(body, "manual analyst note") {
		t.Fatalf("unknown bytes lost after SetStatus; body:\n%s", body)
	}
}

func TestSetStatus_SameStatusRefused(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatal(err)
	}
	_, err := s.SetStatus(1, domain.StatusOpen, "tester", "")
	if err == nil {
		t.Fatal("expected error for no-op status change")
	}
}

func TestSetStatus_CollisionOnExistingTarget(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil { // T-0001-open.md
		t.Fatal(err)
	}
	// Pre-create the target filename to force the collision pre-check.
	if err := os.WriteFile(filepath.Join(dir, "T-0001-wip.md"), []byte("# pre-existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := s.SetStatus(1, domain.StatusWip, "tester", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrCollision) {
		t.Fatalf("expected ErrCollision, got %v", err)
	}
}

func TestSetStatus_NotFound(t *testing.T) {
	s, _ := newStore(t)
	_, err := s.SetStatus(4242, domain.StatusWip, "tester", "ghost")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
