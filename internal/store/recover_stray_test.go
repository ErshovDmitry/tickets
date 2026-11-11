package store

// T-0166: stray content (head-strays and Summary-strays) must survive an
// interrupted SetStatus so the crash-pair signature still matches.

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
)

func TestRecover_CrashPairWithStrayContent(t *testing.T) {
	s, dir := newStore(t)
	body := seedTicketOne(t, s, dir)
	oldPath := filepath.Join(dir, "T-0001-open.md")
	step1 := bytes.Replace(body,
		[]byte("- Project (Проект): tickets\n\n## Summary (Кратко)"),
		[]byte("- Project (Проект): tickets\n\nhead stray line\n## Orphan section\norphan body line\n\n## Summary (Кратко)"), 1)
	if bytes.Equal(step1, body) {
		t.Fatal("anchor 1 (Summary header) not found in the seeded body")
	}
	injected := bytes.Replace(step1,
		[]byte("\n\n## Details (Подробности)"),
		[]byte("\nsummary stray line\n\n## Details (Подробности)"), 1)
	if bytes.Equal(injected, step1) {
		t.Fatal("anchor 2 (Details header) not found in the seeded body")
	}
	if err := os.WriteFile(oldPath, injected, 0o644); err != nil {
		t.Fatalf("write injected ticket: %v", err)
	}

	// Interrupt the SetStatus commit: the old file survives next to the new.
	newPath := filepath.Join(dir, domain.Filename(1, domain.StatusDone))
	orig := removeFile
	removeFile = func(p string) error { return fmt.Errorf("boom(%s)", filepath.Base(p)) }
	restored := false
	t.Cleanup(func() {
		if !restored {
			removeFile = orig
		}
	})
	_, _, err := s.SetStatus(1, domain.StatusDone, "tester", "transition")
	removeFile = orig
	restored = true
	if err == nil {
		t.Fatal("expected the failed old-file removal to surface an error")
	}
	newBody, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read new: %v", err)
	}

	// The crash-pair probe re-renders both files; it matches only if the
	// strays survive in both, so a successful Recover is the assertion.
	kept, err := s.Recover(1, "T-0001-done.md")
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if kept != newPath {
		t.Errorf("kept %q, want %q", kept, newPath)
	}
	if _, err := os.Stat(oldPath); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("old file must be removed, stat err: %v", err)
	}
	assertTicketFileNames(t, dir, "T-0001-done.md")
	after := mustRead(t, dir, 1)
	if !bytes.Equal(after, newBody) {
		t.Errorf("kept file changed by recovery: %q", after)
	}
	for _, stray := range []string{"head stray line", "## Orphan section", "orphan body line", "summary stray line"} {
		if !strings.Contains(string(after), stray) {
			t.Errorf("stray %q lost during set/recover:\n%s", stray, after)
		}
	}
	assertJournalTransition(t, string(after), "open", "done", "transition", "tester")
	assertNoTmpFiles(t, dir)
}
