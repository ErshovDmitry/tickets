package store

// Stage-failure log contracts, extracted from logging_test.go to keep both
// files under the 300-line test budget. Each fixture plants one failure and
// asserts the single terminal store.set.failure / store.archive.failure
// event. T-0157: a number carried by two files fails at stage=ambiguous,
// before the target status is known.

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"ticket/internal/domain"
)

func writeAt(name string) func(*testing.T, *Store, string) {
	return func(t *testing.T, _ *Store, dir string) {
		path := filepath.Join(dir, name)
		must(t, os.MkdirAll(filepath.Dir(path), 0o755))
		must(t, os.WriteFile(path, nil, 0o644))
	}
}
func removeAt(name string) func(*testing.T, *Store, string) {
	return func(t *testing.T, _ *Store, dir string) {
		swapRemove(t, func(p string) error {
			if p != filepath.Join(dir, name) {
				return os.Remove(p)
			}
			return errors.New("synthetic-remove-boom")
		})
	}
}
func linkErr(e error) func(*testing.T, *Store, string) {
	return func(t *testing.T, _ *Store, _ string) { swapLink(t, func(string, string) error { return e }) }
}

// prepTmpWrite pins nanoNow and plants writeTmp's exact pid-based tmp name in sub ("" = store root): every MaxTmpAttempts retry collides.
func prepTmpWrite(sub string, st domain.Status) func(*testing.T, *Store, string) {
	return func(t *testing.T, _ *Store, dir string) {
		orig := nanoNow
		nanoNow = func() int64 { return 7 }
		t.Cleanup(func() { nanoNow = orig })
		must(t, os.MkdirAll(filepath.Join(dir, sub), 0o755))
		must(t, os.WriteFile(filepath.Join(dir, sub, fmt.Sprintf(".T-0001-%s.%d-7.md.tmp", st, os.Getpid())), nil, 0o644))
	}
}

// fixtures plants one stage failure before the operation: foreign target, broken removal/link or tmp.
var fixtures = map[string]func(*testing.T, *Store, string){
	// set-target stays scan-visible: the T-0157 ambiguity gate then fails the operation
	// before any read, journal append, tmp write or rename.
	"set-target": func(t *testing.T, s *Store, dir string) {
		writeAt("T-0001-wip.md")(t, s, dir)
		if files, err := s.findAllNumber(1); err != nil || len(files) != 2 {
			t.Fatalf("fixture must stay scan-visible: %v, %v", files, err)
		}
	},
	"archive-target": writeAt(filepath.Join("archive", "T-0001-done.md")),
	"link":           linkErr(errors.New("synthetic-link-boom")), "link-exist": linkErr(os.ErrExist),
	"tmp-remove": func(t *testing.T, _ *Store, _ string) { n := 0; swapRemoveTmp(t, failTmpOnce(&n)) }, "tmp-write": prepTmpWrite("", domain.StatusOpen),
	"set-tmp-write": prepTmpWrite("", domain.StatusWip), "archive-tmp-write": prepTmpWrite("archive", domain.StatusDone),
	"rename": func(t *testing.T, _ *Store, _ string) {
		swapRename(t, func(string, string) error { return errors.New("boom") })
	},
	"old": removeAt("T-0001-open.md"), "archive-old": removeAt("T-0001-done.md"),
	"first-archive": func(t *testing.T, s *Store, _ string) { _, err := s.Archive(1, "tester"); must(t, err) },
}

func TestStoreLogging_FailureStages(t *testing.T) {
	// seed is the pre-operation status; setTo is the SetStatus target (zero for archive, which takes none).
	for _, tc := range []struct {
		name, kind, stage, fixture string
		seed, setTo                domain.Status
	}{
		{"set target exists", "set", "ambiguous", "set-target", domain.StatusOpen, domain.StatusWip}, {"set link exists", "set", "link", "link-exist", domain.StatusOpen, domain.StatusWip}, {"set link error", "set", "link", "link", domain.StatusOpen, domain.StatusWip},
		{"set tmp write", "set", "tmp", "set-tmp-write", domain.StatusOpen, domain.StatusWip}, {"set tmp remove", "set", "tmp", "tmp-remove", domain.StatusOpen, domain.StatusWip}, {"set old remove", "set", "old", "old", domain.StatusOpen, domain.StatusWip},
		{"set already", "set", "already", "", domain.StatusOpen, domain.StatusOpen}, {"append rename", "append", "rename", "rename", domain.StatusOpen, domain.StatusOpen}, {"append tmp", "append", "tmp", "tmp-write", domain.StatusOpen, domain.StatusOpen},
		{"archive target exists", "archive", "ambiguous", "archive-target", domain.StatusDone, ""}, {"archive link exists", "archive", "link", "link-exist", domain.StatusDone, ""}, {"archive tmp write", "archive", "tmp", "archive-tmp-write", domain.StatusDone, ""},
		{"archive tmp remove", "archive", "tmp", "tmp-remove", domain.StatusDone, ""}, {"archive old remove", "archive", "old", "archive-old", domain.StatusDone, ""}, {"archive already", "archive", "already", "first-archive", domain.StatusDone, ""},
		{"archive not closed", "archive", "not_closed", "", domain.StatusOpen, ""},
	} {
		t.Run(tc.name, func(t *testing.T) { failureStage(t, tc.kind, tc.seed, tc.setTo, tc.stage, tc.fixture) })
	}
}
func failureStage(t *testing.T, kind string, seedStatus, setTo domain.Status, stage, fixture string) {
	t.Helper()
	s, dir, rec := newLoggingStore(t)
	seed(t, s, rec, seedStatus)
	if f := fixtures[fixture]; f != nil {
		f(t, s, dir)
		rec.reset()
	}
	event, attrs := "store.set.failure", kv("number", "1", "to", string(setTo), "stage", stage)
	var err error
	switch kind {
	case "append":
		event, attrs = "store.set.append.failure", kv("number", "1", "stage", stage)
		_, _, err = s.SetStatus(1, setTo, "tester", "note")
	case "archive":
		event, attrs = "store.archive.failure", kv("number", "1", "stage", stage)
		_, err = s.Archive(1, "tester")
	default:
		_, _, err = s.SetStatus(1, setTo, "tester", "")
	}
	if stage == "ambiguous" {
		// The ambiguity gate logs before the requested status is known (T-0157).
		attrs = kv("number", "1", "stage", stage)
	}
	mustFail(t, err)
	checkLog(t, rec, acq(s), entry{slog.LevelError, event, attrs}, rel(s))
}
