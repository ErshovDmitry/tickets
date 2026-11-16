package store

// Store.Edit input validation coverage (T-0187): every rejected input —
// a CR/LF in "who", an empty or line-broken title, a whitespace-only or
// line-broken project, an unparsable type or priority — fails BEFORE any
// read or write, so the ticket file stays byte-identical and no tmp
// files are left behind. Success cases live in store_edit_test.go.

import (
	"strings"
	"testing"

	"ticket/internal/domain"
)

// TestEdit_ValidationRefused pins each validation branch of editLocked:
// error non-nil, message on the pinned fragment, file untouched.
func TestEdit_ValidationRefused(t *testing.T) {
	cases := []struct {
		name    string
		opts    EditOptions
		who     string
		wantErr string
	}{
		{"who CR", EditOptions{Title: new("New name")}, "al\rice", "journal who must not contain a line break"},
		{"who LF", EditOptions{Title: new("New name")}, "al\nice", "journal who must not contain a line break"},
		{"title empty", EditOptions{Title: new("")}, "alice", "edit title must not be empty"},
		{"title blank", EditOptions{Title: new("   ")}, "alice", "edit title must not be empty"},
		{"title CR", EditOptions{Title: new("a\rb")}, "alice", "edit title must not contain a line break"},
		{"title LF", EditOptions{Title: new("a\nb")}, "alice", "edit title must not contain a line break"},
		{"project empty", EditOptions{Project: new("")}, "alice", "edit project must not be empty"},
		{"project whitespace", EditOptions{Project: new("  ")}, "alice", "edit project must not be empty"},
		{"project CR", EditOptions{Project: new("a\rb")}, "alice", "edit project must not contain a line break"},
		{"project LF", EditOptions{Project: new("a\nb")}, "alice", "edit project must not contain a line break"},
		{"type invalid", EditOptions{Type: new(domain.Type("nonsense"))}, "alice", "store: edit type:"},
		{"priority invalid", EditOptions{Priority: new(domain.Priority("yesterday"))}, "alice", "store: edit priority:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, dir := newStore(t)
			if _, err := s.Create(fakeTicket(0)); err != nil {
				t.Fatalf("Create: %v", err)
			}
			before := mustRead(t, dir, 1)
			_, err := s.Edit(1, tc.opts, tc.who)
			if err == nil {
				t.Fatalf("Edit(%+v, who=%q) succeeded, want %q", tc.opts, tc.who, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %q, want fragment %q", err.Error(), tc.wantErr)
			}
			if after := mustRead(t, dir, 1); string(after) != string(before) {
				t.Fatalf("file changed on refused edit:\nbefore:\n%s\nafter:\n%s", before, after)
			}
			assertNoTmpFiles(t, dir)
		})
	}
}

// TestEdit_ValidationSkipsLookup: input validation runs before the
// number lookup, so a rejected edit on a MISSING ticket still fails with
// the validation error and never touches the directory.
func TestEdit_ValidationSkipsLookup(t *testing.T) {
	s, dir := newStore(t)
	_, err := s.Edit(99, EditOptions{Title: new("")}, "alice")
	if err == nil || !strings.Contains(err.Error(), "edit title must not be empty") {
		t.Fatalf("err = %v, want title validation failure", err)
	}
	assertNoTmpFiles(t, dir)
}
