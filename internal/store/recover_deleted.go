package store

// T-0189 W2-A2: recover is deleted-aware WITHOUT widening the shared
// findAllNumber scan — that would hand set/edit/archive an implicit
// undelete in W2 (the W3 boundary). Recover runs its own scan instead:
// findAllNumber(n) plus the deleted/ entries for n. The deleted scan
// error propagates exactly like the archive scan error inside
// findAllNumber: recovery refuses rather than acting on a directory it
// could not read.
//
// Invariant: a file under tickets/deleted/ is terminal and read-only for
// recover. It may be the KEPT half of an interrupted Delete (only the
// stale source file is removed); it is never removed, renamed or
// rewritten. The conservative guard below pins that independently of the
// crash signature holding.

import "path/filepath"

// findRecoverFiles returns every file carrying number n that Recover may
// act on: the unchanged shared scan (main+archive — set/edit/archive keep
// their W2 deleted-blindness, regression-pinned in
// recover_deleted_test.go) plus the deleted/ entries for n.
func (s *Store) findRecoverFiles(n int) ([]locatedFile, error) {
	files, err := s.findAllNumber(n)
	if err != nil {
		return nil, err
	}
	deletedDir, entries, derr := s.deletedScan()
	if derr != nil {
		return nil, derr
	}
	for _, e := range entries {
		if e.Number == n {
			files = append(files, locatedFile{fileEntry: e, dir: deletedDir})
		}
	}
	return files, nil
}

// deletedDiscardRefusal enforces the terminal/read-only invariant of
// tickets/deleted/ for a crash pair: when the file recovery would discard
// (oth) lives under deleted/, the selection asks to keep the stale source
// over its deleted successor — refused as keep-old regardless of whether
// the contents would prove the crash signature (the signature check stays
// the authority only for pairs where neither half is deleted).
func (s *Store) deletedDiscardRefusal(n int, sel, oth *locatedFile) *RecoveryStateError {
	if oth == nil || oth.dir != filepath.Join(s.Dir, "deleted") {
		return nil
	}
	return &RecoveryStateError{Number: n, Keep: sel.Name, Other: oth.Name, Reason: RecoverReasonKeepOld}
}
