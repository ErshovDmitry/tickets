package store

// Delete log contracts (W1 A6), mirroring the archive logging tests:
// every failed delete logs exactly one terminal store.delete.failure
// with number+stage only; a successful delete logs store.delete.commit
// with number/from/path and never the who string.

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// deleteFixtures plant one delete stage failure, reusing the archive
// fixture helpers (removeAt/linkErr/prepTmpWrite).
var deleteFixtures = map[string]func(*testing.T, *Store, string){
	// A regular-file plant would be scan-visible and fail at resolve
	// (ambiguous, a silent exit); a directory named like the target is
	// skipped by scanDir yet still trips the Lstat collision gate.
	"collision": func(t *testing.T, _ *Store, dir string) {
		must(t, os.MkdirAll(filepath.Join(dir, "deleted", "T-0001-deleted.md"), 0o755))
	},
	"link":       linkErr(errors.New("synthetic-link-boom")),
	"link-exist": linkErr(os.ErrExist),
	"tmp-remove": func(t *testing.T, _ *Store, _ string) { n := 0; swapRemoveTmp(t, failTmpOnce(&n)) },
	"tmp-write":  prepTmpWrite("deleted", domain.StatusDeleted),
	"old":        removeAt("T-0001-open.md"),
	"first-delete": func(t *testing.T, s *Store, _ string) {
		_, err := s.Delete("1", false, "tester")
		must(t, err)
	},
}

func TestStoreLogging_DeleteFailureStages(t *testing.T) {
	for _, tc := range []struct {
		name, stage, fixture string
		seed                 domain.Status
	}{
		{"target exists", "collision", "collision", domain.StatusOpen},
		{"link exists", "link", "link-exist", domain.StatusOpen},
		{"link error", "link", "link", domain.StatusOpen},
		{"tmp write", "tmp", "tmp-write", domain.StatusOpen},
		{"tmp remove", "tmp", "tmp-remove", domain.StatusOpen},
		{"old remove", "old", "old", domain.StatusOpen},
		{"already", "already", "first-delete", domain.StatusOpen},
		{"not open", "not_open", "", domain.StatusWip},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, dir, rec := newLoggingStore(t)
			seed(t, s, rec, tc.seed)
			if f := deleteFixtures[tc.fixture]; f != nil {
				f(t, s, dir)
				rec.reset()
			}
			_, err := s.Delete("1", false, "tester")
			mustFail(t, err)
			checkLog(t, rec, acq(s), entry{slog.LevelError, "store.delete.failure", kv("number", "1", "stage", tc.stage)}, rel(s))
		})
	}
}

func TestStoreLogging_DeleteCommit(t *testing.T) {
	for _, tc := range []struct {
		name  string
		seed  domain.Status
		force bool
	}{
		{"open", domain.StatusOpen, false},
		{"wip force", domain.StatusWip, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, dir, rec := newLoggingStore(t)
			seed(t, s, rec, tc.seed)
			got, err := s.Delete("1", tc.force, "who-SECRET")
			must(t, err)
			target := filepath.Join(dir, "deleted", "T-0001-deleted.md")
			checkLog(t, rec, acq(s), entry{slog.LevelInfo, "store.delete.commit", kv("number", "1", "from", string(tc.seed), "path", target)}, rel(s))
			if got != target || strings.Contains(fmt.Sprint(rec.all()), "SECRET") {
				t.Fatalf("Delete = %q, want %q without a who leak", got, target)
			}
		})
	}
}
