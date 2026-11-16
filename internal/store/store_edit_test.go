package store

// Store.Edit core coverage (T-0187, C3/3): each field, all fields with the
// exact journal order and one shared timestamp, the explicit-unchanged
// refusal leaving the file byte-identical (including a changed+unchanged
// mix), and the not-found / ambiguous / read-only / no-options gates.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// journalTransitions returns the transition journal lines of a rendered
// body in file order (creation and archive lines do not match).
func journalTransitions(body string) []string {
	var out []string
	for line := range strings.SplitSeq(body, "\n") {
		if strings.Contains(line, " — статус: ") {
			out = append(out, line)
		}
	}
	return out
}

// journalTimestamp extracts the timestamp of a journal line without byte
// slicing: after the "- " bullet it is the first two whitespace-separated
// fields ("2006-01-02 15:04"). Malformed or short lines fail the test.
func journalTimestamp(t *testing.T, line string) string {
	t.Helper()
	const bullet = "- "
	if !strings.HasPrefix(line, bullet) {
		t.Fatalf("journal line missing %q bullet: %q", bullet, line)
	}
	fields := strings.Fields(strings.TrimPrefix(line, bullet))
	if len(fields) < 2 {
		t.Fatalf("journal line too short for a timestamp: %q", line)
	}
	ts := fields[0] + " " + fields[1]
	if len(ts) != len(tsLayout) {
		t.Fatalf("timestamp must be %d chars: %q", len(tsLayout), line)
	}
	return ts
}

// assertSingleTimestamp fails unless every line carries the same timestamp
// prefix (the edit contract: one timestamp for the whole operation).
func assertSingleTimestamp(t *testing.T, lines []string) {
	t.Helper()
	if len(lines) < 2 {
		return
	}
	ts := journalTimestamp(t, lines[0])
	for _, line := range lines[1:] {
		if got := journalTimestamp(t, line); got != ts {
			t.Fatalf("journal timestamps differ: %q vs %q", ts, got)
		}
	}
}

// TestEdit_EachField edits one field at a time and pins the H1/meta change
// plus the exact journal line.
func TestEdit_EachField(t *testing.T) {
	cases := []struct {
		name    string
		opts    EditOptions
		wantSub []string // substrings the body must contain
		comment string   // exact journal comment
	}{
		{
			name:    "title",
			opts:    EditOptions{Title: new("Renamed ticket")},
			wantSub: []string{"# T-0001 · ENH: Renamed ticket"},
			comment: "title: ticket-0 → Renamed ticket",
		},
		{
			name:    "type",
			opts:    EditOptions{Type: new(domain.TypeBUG)},
			wantSub: []string{"# T-0001 · BUG: ticket-0"},
			comment: "type: ENH → BUG",
		},
		{
			name:    "priority",
			opts:    EditOptions{Priority: new(domain.PriorityHigh)},
			wantSub: []string{"- Priority (Приоритет): high"},
			comment: "priority: normal → high",
		},
		{
			name:    "project",
			opts:    EditOptions{Project: new("other-proj")},
			wantSub: []string{"- Project (Проект): other-proj"},
			comment: "project: tickets → other-proj",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, dir := newStore(t)
			if _, err := s.Create(fakeTicket(0)); err != nil {
				t.Fatalf("Create: %v", err)
			}
			warnings, err := s.Edit(1, tc.opts, "alice")
			if err != nil {
				t.Fatalf("Edit: %v", err)
			}
			if len(warnings) != 0 {
				t.Fatalf("warnings = %v, want none", warnings)
			}
			body := string(mustRead(t, dir, 1))
			for _, sub := range tc.wantSub {
				if !strings.Contains(body, sub) {
					t.Errorf("body missing %q:\n%s", sub, body)
				}
			}
			assertJournalTransition(t, body, "open", "open", tc.comment, "alice")
		})
	}
}

// TestEdit_AllFields pins the fixed journal order (title, type, priority,
// project), one shared timestamp, the exact line shape and the field values
// after a domain.Parse round-trip.
func TestEdit_AllFields(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	opts := EditOptions{
		Title:    new("All new"),
		Type:     new(domain.TypeBUG),
		Priority: new(domain.PriorityHigh),
		Project:  new("other2"),
	}
	if _, err := s.Edit(1, opts, "alice"); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	body := string(mustRead(t, dir, 1))

	lines := journalTransitions(body)
	if len(lines) != 4 {
		t.Fatalf("got %d journal lines, want 4:\n%s", len(lines), body)
	}
	assertSingleTimestamp(t, lines)
	wantComments := []string{
		"title: ticket-0 → All new",
		"type: ENH → BUG",
		"priority: normal → high",
		"project: tickets → other2",
	}
	for i, want := range wantComments {
		ts := journalTimestamp(t, lines[i])
		wantLine := fmt.Sprintf("- %s — статус: open → open · %s (alice)", ts, want)
		if lines[i] != wantLine {
			t.Errorf("line %d\n got: %q\nwant: %q", i, lines[i], wantLine)
		}
	}

	tk, _, err := domain.Parse(mustRead(t, dir, 1))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.Title != "All new" || tk.Type != domain.TypeBUG ||
		tk.Priority != domain.PriorityHigh || tk.Project != "other2" {
		t.Fatalf("parsed ticket = %+v", tk)
	}
	if tk.Status != domain.StatusOpen {
		t.Fatalf("status = %q, want open (unchanged)", tk.Status)
	}
}

// TestEdit_UnchangedFieldRefused: every explicitly unchanged field rejects
// the edit with *AlreadyFieldError and the file stays byte-identical.
func TestEdit_UnchangedFieldRefused(t *testing.T) {
	cases := []struct {
		name  string
		opts  EditOptions
		field string
		value string
	}{
		{"title", EditOptions{Title: new("ticket-0")}, "title", "ticket-0"},
		{"type", EditOptions{Type: new(domain.TypeENH)}, "type", "ENH"},
		{"priority", EditOptions{Priority: new(domain.PriorityNormal)}, "priority", "normal"},
		{"project", EditOptions{Project: new("tickets")}, "project", "tickets"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, dir := newStore(t)
			if _, err := s.Create(fakeTicket(0)); err != nil {
				t.Fatalf("Create: %v", err)
			}
			before := mustRead(t, dir, 1)
			_, err := s.Edit(1, tc.opts, "alice")
			var alreadyFieldErr *AlreadyFieldError
			if !errors.As(err, &alreadyFieldErr) {
				t.Fatalf("err = %v, want *AlreadyFieldError", err)
			}
			if alreadyFieldErr.Number != 1 || alreadyFieldErr.Field != tc.field || alreadyFieldErr.Value != tc.value {
				t.Fatalf("AlreadyFieldError = %+v", alreadyFieldErr)
			}
			if after := mustRead(t, dir, 1); string(after) != string(before) {
				t.Fatalf("file changed on refusal:\nbefore:\n%s\nafter:\n%s", before, after)
			}
		})
	}
}

// TestEdit_MixedChangedUnchangedRefused pins that one unchanged field in a
// mix of changed fields still rejects the WHOLE edit before any write.
func TestEdit_MixedChangedUnchangedRefused(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	before := mustRead(t, dir, 1)
	opts := EditOptions{
		Title:    new("New name"),
		Priority: new(domain.PriorityNormal), // unchanged
	}
	_, err := s.Edit(1, opts, "alice")
	var alreadyFieldErr *AlreadyFieldError
	if !errors.As(err, &alreadyFieldErr) || alreadyFieldErr.Field != "priority" {
		t.Fatalf("err = %v, want *AlreadyFieldError{priority}", err)
	}
	if after := mustRead(t, dir, 1); string(after) != string(before) {
		t.Fatal("file changed on mixed refusal")
	}
	assertNoTmpFiles(t, dir)
}

// TestEdit_NoOptions refuses an edit carrying no fields at all.
func TestEdit_NoOptions(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	before := mustRead(t, dir, 1)
	if _, err := s.Edit(1, EditOptions{}, "alice"); !errors.Is(err, errNoEditFields) {
		t.Fatalf("err = %v, want errNoEditFields", err)
	}
	if after := mustRead(t, dir, 1); string(after) != string(before) {
		t.Fatal("file changed on no-options refusal")
	}
}

// TestEdit_NotFound / TestEdit_Ambiguous / TestEdit_ReadOnly pin the lookup
// and store-mode gates; none of them touches the directory.
func TestEdit_NotFound(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.Edit(99, EditOptions{Title: new("x")}, "alice"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestEdit_Ambiguous(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Crash pair: same number at two statuses (T-0157).
	body := mustRead(t, dir, 1)
	if err := os.WriteFile(filepath.Join(dir, "T-0001-done.md"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Edit(1, EditOptions{Title: new("x")}, "alice"); !errors.Is(err, ErrAmbiguousNumber) {
		t.Fatalf("err = %v, want ErrAmbiguousNumber", err)
	}
}

func TestEdit_ReadOnly(t *testing.T) {
	dir := t.TempDir()
	ws, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Create(fakeTicket(0)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	rs, err := NewReadOnly(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rs.Edit(1, EditOptions{Title: new("x")}, "alice"); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("err = %v, want ErrReadOnly", err)
	}
}
