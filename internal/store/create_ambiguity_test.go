package store

// T-0157 D5.5: Create verifies its own number after the commit (all under
// the same lock). A foreign file carrying the same number — any status, in
// s.Dir or in archive/ — means a competing writer exists, so the just-created
// file is removed and the number is reported as ambiguous. The three
// branches: clean removal, failed removal (the leftover ticket stays valid
// and is named by absolute path), and a failed verification scan (no
// ambiguity is invented; the file is scrubbed and the scan error returned).

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// ticketFileNames returns the sorted names of the regular files in dir,
// skipping the store lock file and tmp files. A missing directory yields an
// empty slice (no archive yet).
func ticketFileNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []string{}
		}
		t.Fatalf("read dir %s: %v", dir, err)
	}
	names := []string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || name == ".lock" || strings.HasSuffix(name, ".tmp") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// assertNoTmpFilesIn fails the test if dir exists and contains *.tmp
// entries; a missing directory is fine (no archive yet).
func assertNoTmpFilesIn(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return
	}
	assertNoTmpFiles(t, dir)
}

// swapAfterCreate sets the afterCreateHook seam for the duration of t.
func swapAfterCreate(t *testing.T, fn func(n int, path string)) {
	t.Helper()
	orig := afterCreateHook
	afterCreateHook = fn
	t.Cleanup(func() { afterCreateHook = orig })
}

// validTicketBody renders a real new-ticket body for number, so a plant can
// write a foreign file that parses (the ambiguity is about the name, not the
// body).
func validTicketBody(t *testing.T, number int) []byte {
	t.Helper()
	body, err := domain.RenderNewTicket(fakeTicket(number))
	if err != nil {
		t.Fatalf("render ticket %d: %v", number, err)
	}
	return body
}

// TestCreate_AmbiguousNumberRemovesOwnFile pins the three conflict
// placements: the same number in s.Dir, the same number in archive/, and the
// same base name in archive/ (the own-pair exclusion is by directory AND
// name, so a same-named archive file is a conflict too). Every case must
// return 0 and *AmbiguousNumberError listing only the foreign path, remove
// the own file, leave the foreign file byte-identical and leak no tmp file.
func TestCreate_AmbiguousNumberRemovesOwnFile(t *testing.T) {
	for _, tc := range []struct {
		name, rel string
		status    domain.Status
	}{
		{"same number in dir", "T-0001-done.md", domain.StatusDone},
		{"same number in archive", filepath.Join("archive", "T-0001-done.md"), domain.StatusDone},
		{"same base name in archive", filepath.Join("archive", "T-0001-open.md"), domain.StatusOpen},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, dir := newStore(t)
			foreign := filepath.Join(dir, tc.rel)
			foreignBody := validTicketBody(t, 1)
			swapAfterCreate(t, func(int, string) { cloneTicket(t, foreign, foreignBody) })

			n, err := s.Create(fakeTicket(0))
			if n != 0 {
				t.Errorf("Create returned number %d on ambiguity, want 0", n)
			}
			assertAmbiguous(t, err, 1, []string{foreign})
			if _, serr := os.Stat(filepath.Join(dir, "T-0001-open.md")); !errors.Is(serr, fs.ErrNotExist) {
				t.Errorf("own file must be removed on ambiguity: %v", serr)
			}
			assertFileContent(t, foreign, string(foreignBody))
			// No tmp file anywhere — the conflict may sit in archive/.
			assertNoTmpFiles(t, dir)
			assertNoTmpFilesIn(t, filepath.Join(dir, "archive"))
			mainTickets, _ := s.List()
			archivedTickets, _ := s.ListArchive()
			tickets := append(mainTickets, archivedTickets...)
			if len(tickets) != 1 || tickets[0].Number != 1 {
				t.Errorf("tickets = %v, want the single foreign ticket 1", tickets)
			}
			// The filename status is authoritative (V19): pin it explicitly,
			// so a done-vs-open mix-up cannot pass unnoticed.
			if tickets[0].Status != tc.status {
				t.Errorf("ticket status = %q, want %q", tickets[0].Status, tc.status)
			}
			// No stray file: the tree holds exactly the foreign ticket (and
			// the archive/ directory holding it) and nothing else.
			wantTop := []string{}
			wantArchive := []string{}
			if filepath.Dir(tc.rel) == "." {
				wantTop = []string{filepath.Base(tc.rel)}
			} else {
				wantArchive = []string{filepath.Base(tc.rel)}
			}
			if got := ticketFileNames(t, dir); !reflect.DeepEqual(got, wantTop) {
				t.Errorf("files in %s = %v, want %v", dir, got, wantTop)
			}
			if got := ticketFileNames(t, filepath.Join(dir, "archive")); !reflect.DeepEqual(got, wantArchive) {
				t.Errorf("files in %s = %v, want %v", filepath.Join(dir, "archive"), got, wantArchive)
			}
		})
	}
}

// TestCreate_RemovalFailureLeavesCompleteTicket pins the Windows-shaped
// branch: when removing the own file fails, the leftover file IS a valid
// ticket (it stays visible in List) and the joined error names its absolute
// path along with the removal cause.
func TestCreate_RemovalFailureLeavesCompleteTicket(t *testing.T) {
	s, dir := newStore(t)
	own := filepath.Join(dir, "T-0001-open.md")
	foreign := filepath.Join(dir, "T-0001-done.md")
	foreignBody := validTicketBody(t, 1)
	swapAfterCreate(t, func(int, string) { cloneTicket(t, foreign, foreignBody) })
	swapRemove(t, func(p string) error {
		if p == own {
			return errors.New("synthetic-remove-boom")
		}
		return os.Remove(p)
	})

	n, err := s.Create(fakeTicket(0))
	if n != 0 {
		t.Errorf("Create returned number %d, want 0", n)
	}
	assertAmbiguous(t, err, 1, []string{foreign})
	for _, want := range []string{own, "left on disk", "synthetic-remove-boom"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q must name %q", err, want)
		}
	}
	tickets, _ := s.List()
	if len(tickets) != 2 || tickets[0].Number != 1 || tickets[1].Number != 1 {
		t.Fatalf("List = %v, want both files of number 1 (the leftover ticket stays valid)", tickets)
	}
	assertNoTmpFiles(t, dir)
}

// TestCreate_VerifyScanErrorScrubsCreatedFile pins the scan-failure branch:
// an unreadable directory during verification is NOT reported as ambiguity —
// the created file is scrubbed, the wrapped "store: verify number" error
// carries the scan cause, and nothing is left behind.
func TestCreate_VerifyScanErrorScrubsCreatedFile(t *testing.T) {
	s, dir := newStore(t)
	armed := false
	swapAfterCreate(t, func(int, string) { armed = true })
	swapReadDir(t, func(p string) ([]os.DirEntry, error) {
		if armed {
			return nil, errInjected
		}
		return os.ReadDir(p)
	})

	n, err := s.Create(fakeTicket(0))
	if n != 0 {
		t.Errorf("Create returned number %d, want 0", n)
	}
	if err == nil {
		t.Fatal("expected a verification error")
	}
	var amb *AmbiguousNumberError
	if errors.As(err, &amb) {
		t.Errorf("scan failure must not be reported as ambiguity: %v", err)
	}
	if !errors.Is(err, errInjected) || !strings.Contains(err.Error(), "store: verify number 1") {
		t.Errorf("error = %v, want the wrapped scan cause", err)
	}
	entries, derr := os.ReadDir(dir)
	if derr != nil {
		t.Fatalf("read %s: %v", dir, derr)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") || strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("verification failure must leave no ticket behind: %s", e.Name())
		}
	}
}
