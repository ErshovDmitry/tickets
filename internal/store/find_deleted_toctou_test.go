package store

// T-0189 W3-C1a: TOCTOU containment parity for the deleted-aware seams
// (split out of find_deleted_test.go to keep every file inside the
// 300-line budget).

import (
	"bytes"
	"testing"
)

// TestFindRawAcross_TOCTOUSymlinkSwapCannotLeakOutside is the deleted-side
// containment parity with TestFindRaw_TOCTOUSymlinkSwapCannotLeakOutside:
// a deleted ticket validated by the scan swapped for a symlink to an
// outside sentinel between validation and open must fail BEFORE any byte
// is read — the sentinel content can never come back through either seam.
func TestFindRawAcross_TOCTOUSymlinkSwapCannotLeakOutside(t *testing.T) {
	requireSymlinks(t)
	s, dir := newStore(t)
	deletedDir := seedDeletedFile(t, dir, "T-0001-deleted.md", acrossDeletedBody)
	outside := t.TempDir()
	sentinel, sentinelBody := makeOutsideTicket(t, outside)
	installSwapHook(t, deletedDir, "T-0001-deleted.md", sentinel)

	tk, name, raw, err := s.FindRawAcross(1)
	if err == nil {
		t.Fatalf("FindRawAcross succeeded through a mid-open symlink swap: %+v", tk)
	}
	if raw != nil || bytes.Contains(raw, sentinelBody) || name != "" {
		t.Errorf("leak: raw=%q name=%q", raw, name)
	}
	if _, name, raw, serr := s.FindExactAcross("T-0001-deleted.md"); serr == nil || raw != nil || name != "" {
		t.Errorf("FindExactAcross leak: name=%q raw=%q err=%v", name, raw, serr)
	}
}
