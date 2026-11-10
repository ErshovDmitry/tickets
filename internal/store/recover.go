package store

// T-0159: recovery from a status commit interrupted between the link of the
// new file and the removal of the old one. Such a crash leaves exactly two
// files for one number (e.g. T-0001-open.md and T-0001-done.md). Recover
// proves the crash signature — the selected file is the old file plus
// exactly one trailing journal record From=old→To=new, and the old file
// does not already end in that same transition — and then removes
// ONLY the stale old file. It never writes the new file, never creates
// tmp/journal files, and never renames anything (no new rename-over risk).

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"ticket/internal/domain"
)

// Recovery refusal reasons, carried by RecoveryStateError.Reason. The CLI
// (cmd_recover) maps them to exit codes: same-status and divergent mean
// "the number is not cleanly recoverable" (exit 2), the rest are plain
// refusal/usage states (exit 1).
const (
	// RecoverReasonSelectionNoMatch: the selected basename is not one of
	// the two files carrying the number.
	RecoverReasonSelectionNoMatch = "selection-not-in-pair"
	// RecoverReasonSameStatus: both files carry the same filename status,
	// so there is no status transition to recover.
	RecoverReasonSameStatus = "same-status"
	// RecoverReasonDivergent: the contents do not match the crash
	// signature in either direction.
	RecoverReasonDivergent = "divergent"
	// RecoverReasonKeepOld: the selected file is the stale old one; the
	// caller must select the new file instead (rollback happens via a
	// normal set after recovery).
	RecoverReasonKeepOld = "keep-old"
	// RecoverReasonSingleMismatch: exactly one file remains and it is not
	// the selected new file.
	RecoverReasonSingleMismatch = "single-mismatch"
)

// RecoveryStateError is a typed refusal: the on-disk state does not match
// the recoverable crash signature, so nothing was changed. Keep is the
// selected basename; Other is the second file of the pair (when any);
// Reason is one of the RecoverReason* constants. The CLI translates this
// to a localized message and an exit code; the store carries no UI text.
type RecoveryStateError struct {
	Number int
	Keep   string
	Other  string
	Reason string
}

// Error renders the internal (non-UI) refusal description.
func (e *RecoveryStateError) Error() string {
	switch e.Reason {
	case RecoverReasonSameStatus:
		return fmt.Sprintf("store: recover: ticket %d files share filename status; refusing to guess", e.Number)
	case RecoverReasonDivergent:
		return fmt.Sprintf("store: recover: ticket %d: %s is not the crash-pair successor of %s (contents diverge)", e.Number, e.Keep, e.Other)
	case RecoverReasonKeepOld:
		return fmt.Sprintf("store: recover: ticket %d: selected %s is the stale file; recover %s first, then roll back with set", e.Number, e.Keep, e.Other)
	case RecoverReasonSingleMismatch:
		return fmt.Sprintf("store: recover: ticket %d: single remaining file %s is not the selected %s", e.Number, e.Other, e.Keep)
	default:
		return fmt.Sprintf("store: recover: ticket %d: selected %s is not part of the conflicting pair", e.Number, e.Keep)
	}
}

// Recover resolves an interrupted status commit for ticket n. keepBase is
// the basename of the file the caller identified as the canonical new file
// (from the ambiguity diagnostics), e.g. "T-0001-done.md". On success it
// returns the absolute path of the kept file and removes only the stale
// old file; a repeated call with exactly the kept file left is a no-op
// returning the same path. Any refusal or I/O error leaves the tree
// untouched and returns "" with a typed error (ErrNotFound,
// *AmbiguousNumberError for more than two files, *RecoveryStateError for
// state mismatches).
func (s *Store) Recover(n int, keepBase string) (string, error) {
	var kept string
	err := s.withLock(func() error {
		var rerr error
		kept, rerr = s.recoverLocked(n, keepBase)
		return rerr
	})
	if err != nil {
		return "", err
	}
	return kept, nil
}

// recoverLocked implements Recover under the store lock.
func (s *Store) recoverLocked(n int, keepBase string) (string, error) {
	log := s.Logger()
	fail := func(stage, discard string, err error) (string, error) {
		log.Error("store.recover.failure", "number", n, "keep", keepBase,
			"discard", discard, "stage", stage, "err", err)
		return "", err
	}
	if perr := validSelection(n, keepBase); perr != nil {
		return fail("selection", "", perr)
	}
	files, err := s.findAllNumber(n)
	if err != nil {
		return fail("scan", "", err)
	}
	switch len(files) {
	case 0:
		return fail("scan", "", ErrNotFound)
	case 1:
		return s.recoverSingle(log, n, files[0], keepBase)
	}
	if len(files) > 2 {
		return fail("ambiguous", "", ambiguousNumberError(n, files))
	}
	if files[0].Status == files[1].Status {
		return fail("state", "", &RecoveryStateError{
			Number: n, Keep: keepBase, Other: files[0].Name + ", " + files[1].Name,
			Reason: RecoverReasonSameStatus,
		})
	}
	sel, oth := splitSelection(files, keepBase)
	if sel == nil || oth == nil {
		return fail("state", "", &RecoveryStateError{Number: n, Keep: keepBase, Reason: RecoverReasonSelectionNoMatch})
	}
	if stateErr := s.verifyPair(n, sel, oth); stateErr != nil {
		return fail("state", filepath.Join(oth.dir, oth.Name), stateErr)
	}
	keepPath := filepath.Join(sel.dir, sel.Name)
	oldPath := filepath.Join(oth.dir, oth.Name)
	if rerr := removeFile(oldPath); rerr != nil {
		// The pair stays on disk for a retry; nothing else was touched.
		return fail("remove", oldPath, fmt.Errorf("store: recover: remove old %s (keep %s): %w", oldPath, keepPath, rerr))
	}
	log.Info("store.recover.commit", "number", n, "keep", keepPath, "discard", oldPath)
	return keepPath, nil
}

// validSelection rejects a keep argument that is not a canonical ticket
// basename for number n with a known status.
func validSelection(n int, keepBase string) error {
	if strings.ContainsAny(keepBase, `/\\`) {
		return fmt.Errorf("store: recover: keep must be a basename, got %q", keepBase)
	}
	kn, kst, err := domain.ParseFilename(keepBase)
	if err != nil {
		return fmt.Errorf("store: recover: keep: %w", err)
	}
	if kn != n {
		return fmt.Errorf("store: recover: keep %q names ticket %d, not %d", keepBase, kn, n)
	}
	if _, err := domain.ParseStatus(string(kst)); err != nil {
		return fmt.Errorf("store: recover: keep %q: %w", keepBase, err)
	}
	return nil
}

// recoverSingle handles the idempotent repeat: exactly one file remains
// and it is the previously kept new file — a no-op success. Any other
// single file is a typed refusal.
func (s *Store) recoverSingle(log *slog.Logger, n int, f locatedFile, keepBase string) (string, error) {
	fail := func(err error) (string, error) {
		log.Error("store.recover.failure", "number", n, "keep", keepBase,
			"discard", "", "stage", "state", "err", err)
		return "", err
	}
	if f.Name != keepBase {
		return fail(&RecoveryStateError{Number: n, Keep: keepBase, Other: f.Name, Reason: RecoverReasonSingleMismatch})
	}
	fi, err := os.Lstat(filepath.Join(f.dir, f.Name))
	if err != nil {
		return fail(fmt.Errorf("store: recover: lstat %s: %w", f.Name, err))
	}
	if !fi.Mode().IsRegular() {
		return fail(&RecoveryStateError{Number: n, Keep: keepBase, Other: f.Name, Reason: RecoverReasonSingleMismatch})
	}
	path := filepath.Join(f.dir, f.Name)
	log.Info("store.recover.commit", "number", n, "keep", path, "discard", "")
	return path, nil
}

// splitSelection picks the selected file (matching basename) and the other
// half of the pair. Same-status pairs are rejected before this, so two
// files can never share the selected name.
func splitSelection(files []locatedFile, keepBase string) (sel, oth *locatedFile) {
	for i := range files {
		if files[i].Name == keepBase {
			sel = &files[i]
		} else {
			oth = &files[i]
		}
	}
	return sel, oth
}

// verifyPair proves the crash signature with sel as the candidate new
// file. If that fails it tries the reverse direction to distinguish a
// keep-old selection from genuinely divergent contents.
func (s *Store) verifyPair(n int, sel, oth *locatedFile) error {
	ok, err := s.crashSignature(oth, sel)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	ok, err = s.crashSignature(sel, oth)
	if err != nil {
		return err
	}
	if ok {
		return &RecoveryStateError{Number: n, Keep: sel.Name, Other: oth.Name, Reason: RecoverReasonKeepOld}
	}
	return &RecoveryStateError{Number: n, Keep: sel.Name, Other: oth.Name, Reason: RecoverReasonDivergent}
}

// crashSignature reports whether newLoc's file is exactly oldLoc's file
// plus one trailing journal record From=old status → To=new status, where
// oldLoc's file does not itself already end in that same transition (a
// duplicate terminal record marks a replayed commit, not an interrupted
// one). The
// filename statuses are authoritative (V19); comparison is content-based,
// never path-order-based. Equality is proven by re-rendering both tickets:
// the render covers every parsed field, the unknown trailer bytes and the
// raw templates, so any other difference fails the comparison.
func (s *Store) crashSignature(oldLoc, newLoc *locatedFile) (bool, error) {
	oldTk, _, oldUnknown, err := readTicketFile(filepath.Join(oldLoc.dir, oldLoc.Name), oldLoc.Number)
	if err != nil {
		return false, fmt.Errorf("store: recover: read %s: %w", oldLoc.Name, err)
	}
	newTk, _, newUnknown, err := readTicketFile(filepath.Join(newLoc.dir, newLoc.Name), newLoc.Number)
	if err != nil {
		return false, fmt.Errorf("store: recover: read %s: %w", newLoc.Name, err)
	}
	// Old must lack the terminal transition: an old journal already
	// ending with the same From=old→To=new record means the commit was
	// replayed, so the pair is divergent even though every other check
	// (length, prefix, render equality) would pass.
	if n := len(oldTk.Journal); n > 0 {
		last := oldTk.Journal[n-1]
		if last.From == oldLoc.Status && last.To == newLoc.Status {
			return false, nil
		}
	}
	if len(newTk.Journal) != len(oldTk.Journal)+1 {
		return false, nil
	}
	last := newTk.Journal[len(newTk.Journal)-1]
	if last.From != oldLoc.Status || last.To != newLoc.Status {
		return false, nil
	}
	for i := range oldTk.Journal {
		if newTk.Journal[i] != oldTk.Journal[i] {
			return false, nil
		}
	}
	oldTk.Status = oldLoc.Status
	stripped := *newTk
	stripped.Journal = stripped.Journal[:len(stripped.Journal)-1]
	stripped.Status = oldLoc.Status
	oldBody, err := domain.Render(oldTk, oldUnknown)
	if err != nil {
		return false, fmt.Errorf("store: recover: render %s: %w", oldLoc.Name, err)
	}
	newBody, err := domain.Render(&stripped, newUnknown)
	if err != nil {
		return false, fmt.Errorf("store: recover: render %s: %w", newLoc.Name, err)
	}
	return bytes.Equal(oldBody, newBody), nil
}
