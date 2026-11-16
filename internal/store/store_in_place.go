package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"ticket/internal/domain"
)

// rewriteInPlaceLocked is the shared commit seam for mutations that keep
// the file name and directory unchanged: a same-status journal append
// (appendSameStatusLocked) and a metadata edit (Store.Edit, T-0187). It
// reads the current file, sets the status from the file name (the filename
// is authoritative, V16; the body status is untrusted), lets the caller's
// mutate update the in-place ticket and return the journal entries to
// append, renders, writes a same-directory .tmp file and rename-over's
// the original through the renameFile hook. The rename-over replaces the existing file atomically on
// POSIX (rename(2)); on Windows MoveFileEx(MOVEFILE_REPLACE_EXISTING)
// replaces it non-atomically, and a sharing violation (target held open
// without FILE_SHARE_DELETE) fails the commit with the old file intact and
// the tmp removed. Any rename error is joined with the tmp cleanup; no
// rollback is needed because old == target stays alive. Caveats inherited
// from the cross-status commit (T-0187 decision bd56bdd8, no new
// atomicity/recovery guarantees): the rename-over is unconditional
// last-writer-wins — not a compare-and-swap against the bytes read — so a
// journal append written out of lock between readTicketFile and the rename
// is silently lost (the same lost-update window the cross-status path has);
// and the tmp file is fsynced by writeTmp but the directory is NOT fsynced
// after the rename, so a crash may lose the renamed entry —
// crash-durability is no stronger than the pre-existing same-status path.
//
// mutate returning an error aborts before any write: the file on disk is
// untouched (e.g. an explicitly unchanged edit field rejects the whole
// edit with *AlreadyFieldError). event is the caller's log-event prefix
// (e.g. "store.edit"); failure stages log event+".failure" with a stage
// attr, success logs event with number/status/path.
func (s *Store) rewriteInPlaceLocked(n int, cur fileEntry, curDir string, mutate func(tk *domain.Ticket, at time.Time) ([]domain.JournalEntry, error), event string) (string, []ParseWarning, error) {
	old := filepath.Join(curDir, cur.Name)
	tk, _, unknown, err := readTicketFile(old, cur.Number)
	if err != nil {
		return "", nil, fmt.Errorf("store: read %s: %w", cur.Name, err)
	}
	warnings := journalDupWarnings(cur.Name, tk)
	tk.Status = cur.Status
	entries, err := mutate(tk, time.Now())
	if err != nil {
		return "", nil, err
	}
	tk.Journal = append(tk.Journal, entries...)
	rendered, err := domain.Render(tk, unknown)
	if err != nil {
		return "", nil, fmt.Errorf("store: render: %w", err)
	}
	tmpPath, err := s.writeTmp(n, cur.Status, rendered, curDir)
	if err != nil {
		s.Logger().Error(event+".failure", "number", n, "stage", "tmp")
		return "", nil, err
	}
	if err := renameFile(tmpPath, old); err != nil {
		s.Logger().Error(event+".failure", "number", n, "stage", "rename")
		return "", nil, errors.Join(fmt.Errorf("store: rename commit: %w", err), removeTmp(tmpPath))
	}
	s.Logger().Info(event, "number", n, "status", cur.Status, "path", old)
	return old, warnings, nil
}

// appendSameStatusLocked journals a same-status set (comment != "") in
// place: the file name and directory never change (target == old), one
// From==To==cur.Status journal entry is appended, and the commit goes
// through rewriteInPlaceLocked (see there for the POSIX/Windows rename-over
// semantics and the lost-update-window caveat).
func (s *Store) appendSameStatusLocked(n int, cur fileEntry, curDir, who, comment string) (string, []ParseWarning, error) {
	// T-0065: same-status journal-only path is a write site of the
	// line-oriented journal format, so the same guard applies.
	if err := validateJournalInput(comment, who); err != nil {
		return "", nil, err
	}
	return s.rewriteInPlaceLocked(n, cur, curDir, func(_ *domain.Ticket, at time.Time) ([]domain.JournalEntry, error) {
		return []domain.JournalEntry{{
			From:    cur.Status,
			To:      cur.Status,
			Comment: comment,
			Who:     who,
			At:      at,
		}}, nil
	}, "store.set.append")
}
