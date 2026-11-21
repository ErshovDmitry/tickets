package store

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"ticket/internal/domain"
)

// errNoEditFields refuses an Edit carrying no options at all: there is
// nothing to change, and committing a byte-identical rewrite would only
// churn mtime (T-0187).
var errNoEditFields = errors.New("store: edit: no fields provided")

// EditOptions carries the optional metadata changes for Store.Edit (T-0187).
// A nil pointer means "leave the field alone"; a non-nil pointer — including
// one pointing at the current value — means "set exactly this". An
// explicitly unchanged field rejects the whole edit with
// *AlreadyFieldError, including a mix of changed and unchanged fields.
type EditOptions struct {
	Title    *string
	Type     *domain.Type
	Priority *domain.Priority
	Project  *string
}

// Edit mutates the metadata of ticket n in place — title, type, priority
// and project only; the status, file name and directory never change, so
// an archived ticket is edited inside archive/ without a reopen and
// "project" is a metadata field, not a move (per-project directories do
// not exist). The whole validation/lookup/mutation/commit runs under the
// per-store advisory lock; a read-only Store is rejected by withLock with
// ErrReadOnly, and an ambiguous number is refused before any read (T-0157).
// Each changed field gets its own journal line in the fixed order title,
// type, priority, project, all sharing one timestamp, with
// From==To==current status and Comment "field: old → new". Success is
// silent: the returned warnings are parse notices for the rewritten file
// (e.g. a merged duplicate journal section, T-0070); errors return nil
// warnings and leave the file byte-identical.
func (s *Store) Edit(n int, opts EditOptions, who string) ([]ParseWarning, error) {
	var warnings []ParseWarning
	err := s.withLock(func() error {
		var serr error
		warnings, serr = s.editLocked(n, opts, who)
		return serr
	})
	if err != nil {
		return nil, err
	}
	return warnings, nil
}

// fieldChange is one validated, pending metadata edit: the journal field
// name, the old/new values for the "field: old → new" comment, and the
// ticket mutation applying it. Changes are built and applied in the fixed
// journal order title, type, priority, project.
type fieldChange struct {
	field string
	old   string
	new   string
	apply func(tk *domain.Ticket)
}

// editLocked assumes the per-store advisory lock is held.
func (s *Store) editLocked(n int, opts EditOptions, who string) ([]ParseWarning, error) {
	// T-0065: "who" is written verbatim into the line-oriented journal, so
	// reject CR/LF up-front, before any read or write.
	if err := validateJournalInput("", who); err != nil {
		return nil, err
	}
	if opts.Title == nil && opts.Type == nil && opts.Priority == nil && opts.Project == nil {
		return nil, errNoEditFields
	}
	// Validate every provided value BEFORE the lookup, so a rejected edit
	// never touches the directory (same precedence as setStatusLocked).
	title, err := validateEditTitle(opts.Title)
	if err != nil {
		return nil, err
	}
	project, err := validateEditProject(opts.Project)
	if err != nil {
		return nil, err
	}
	if opts.Type != nil {
		if _, err := domain.ParseType(string(*opts.Type)); err != nil {
			return nil, fmt.Errorf("store: edit type: %w", err)
		}
	}
	if opts.Priority != nil {
		if _, err := domain.ParsePriority(string(*opts.Priority)); err != nil {
			return nil, fmt.Errorf("store: edit priority: %w", err)
		}
	}

	// T-0157: a missing number yields ErrNotFound, a duplicated one
	// *AmbiguousNumberError; either way nothing is read or written.
	cur, err := s.findOneLocked(n, "store.edit.failure")
	if err != nil {
		return nil, err
	}

	_, warnings, err := s.rewriteInPlaceLocked(n, cur.fileEntry, cur.dir, func(tk *domain.Ticket, at time.Time) ([]domain.JournalEntry, error) {
		var changes []fieldChange
		if opts.Title != nil {
			if title == tk.Title {
				return nil, &AlreadyFieldError{Number: n, Field: "title", Value: title}
			}
			old := tk.Title
			changes = append(changes, fieldChange{
				field: "title", old: old, new: title,
				apply: func(tk *domain.Ticket) {
					// T-0166: a Summary body that is empty or still equals
					// the old title is the renderer's default — carry it to
					// the new title; a custom Brief survives the edit.
					if len(tk.Brief) == 0 || strings.Join(tk.Brief, "\n") == old {
						tk.Brief = []string{title}
					}
					tk.Title = title
				},
			})
		}
		if opts.Type != nil {
			if *opts.Type == tk.Type {
				return nil, &AlreadyFieldError{Number: n, Field: "type", Value: string(*opts.Type)}
			}
			changes = append(changes, fieldChange{
				field: "type", old: string(tk.Type), new: string(*opts.Type),
				apply: func(tk *domain.Ticket) { tk.Type = *opts.Type },
			})
		}
		if opts.Priority != nil {
			if *opts.Priority == tk.Priority {
				return nil, &AlreadyFieldError{Number: n, Field: "priority", Value: string(*opts.Priority)}
			}
			changes = append(changes, fieldChange{
				field: "priority", old: string(tk.Priority), new: string(*opts.Priority),
				apply: func(tk *domain.Ticket) { tk.Priority = *opts.Priority },
			})
		}
		if opts.Project != nil {
			if project == tk.Project {
				return nil, &AlreadyFieldError{Number: n, Field: "project", Value: project}
			}
			changes = append(changes, fieldChange{
				field: "project", old: tk.Project, new: project,
				apply: func(tk *domain.Ticket) { tk.Project = project },
			})
		}
		entries := make([]domain.JournalEntry, len(changes))
		for i, ch := range changes {
			ch.apply(tk)
			entries[i] = domain.JournalEntry{
				From:    tk.Status,
				To:      tk.Status,
				Comment: fmt.Sprintf("%s: %s → %s", ch.field, ch.old, ch.new),
				Who:     who,
				At:      at,
			}
		}
		return entries, nil
	}, "store.edit")
	return warnings, err
}

// validateEditTitle checks an explicitly passed title: it must not be
// empty/whitespace-only and must not carry a line break (a CR/LF would
// escape the single-line H1 into the head zone). A nil option passes
// through as absent.
func validateEditTitle(v *string) (string, error) {
	if v == nil {
		return "", nil
	}
	if strings.TrimSpace(*v) == "" {
		return "", errors.New("store: edit title must not be empty")
	}
	if strings.ContainsAny(*v, "\r\n") {
		return "", fmt.Errorf("store: edit title must not contain a line break: %q", preview(*v))
	}
	return *v, nil
}

// validateEditProject checks an explicitly passed project value. A nil
// value leaves the project unchanged; Edit does not support clearing the
// project field. A non-nil value must be non-empty and must not carry a
// line break, which would escape the single-line meta row.
func validateEditProject(v *string) (string, error) {
	if v == nil {
		return "", nil
	}
	if strings.ContainsAny(*v, "\r\n") {
		return "", fmt.Errorf("store: edit project must not contain a line break: %q", preview(*v))
	}
	if strings.TrimSpace(*v) == "" {
		return "", errors.New("store: edit project must not be empty")
	}
	return *v, nil
}
