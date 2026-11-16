package domain

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// W1/A (T-0189, GitHub #22): the terminal status "deleted" is accepted by
// the domain enums, filename codec and Strict validation. Round-trip
// preservation lives in status_deleted_roundtrip_test.go.

func TestParseStatusDeleted(t *testing.T) {
	if StatusDeleted != "deleted" {
		t.Fatalf("StatusDeleted = %q, want deleted", StatusDeleted)
	}
	got, err := ParseStatus("deleted")
	if err != nil || got != StatusDeleted {
		t.Errorf("ParseStatus(deleted) = %q, %v; want deleted, nil", got, err)
	}
}

func TestParseStatusDeletedRejectsNearMisses(t *testing.T) {
	for _, s := range []string{"delete", "Deleted", "DELETED", "deleted ", ""} {
		if _, err := ParseStatus(s); err == nil {
			t.Errorf("ParseStatus(%q) = nil error, want error", s)
		}
	}
}

func TestParseStatusErrorListsDeleted(t *testing.T) {
	_, err := ParseStatus("bogus")
	if err == nil || !strings.Contains(err.Error(), "deleted") {
		t.Errorf("ParseStatus(bogus) error = %v, want the valid list to mention deleted", err)
	}
}

func TestFilenameDeletedRoundTrip(t *testing.T) {
	if got, want := Filename(7, StatusDeleted), "T-0007-deleted.md"; got != want {
		t.Errorf("Filename(7, deleted) = %q, want %q", got, want)
	}
	n, st, err := ParseFilename("T-0007-deleted.md")
	if err != nil || n != 7 || st != StatusDeleted {
		t.Errorf("ParseFilename(T-0007-deleted.md) = %d, %q, %v; want 7, deleted, nil", n, st, err)
	}
}

func TestValidateDeleted(t *testing.T) {
	tk := Ticket{Number: 7, Status: StatusDeleted, Type: TypeBUG, Priority: PriorityNormal, Title: "x"}
	if err := tk.Validate(Strict); err != nil {
		t.Errorf("Validate(Strict) with deleted: %v, want nil", err)
	}
	if err := tk.Validate(Tolerant); err != nil {
		t.Errorf("Validate(Tolerant) with deleted: %v, want nil", err)
	}
	tk.Status = "bogus"
	if err := tk.Validate(Strict); err == nil {
		t.Error("Validate(Strict) with bogus status = nil error, want error")
	}
}

// TestRenderDeletedJournalDictForms covers the dict emission seam used by
// the future store.Delete journal entry (empty Raw): old→deleted renders in
// both languages, with and without a comment.
func TestRenderDeletedJournalDictForms(t *testing.T) {
	at := time.Date(2026, 11, 16, 11, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		lang Lang
		e    JournalEntry
		want string
	}{
		{"RU no comment", LangRU, JournalEntry{At: at, From: StatusOpen, To: StatusDeleted, Who: "агент"},
			"- 2026-11-16 11:00 — статус: open → deleted (агент)\n"},
		{"RU comment", LangRU, JournalEntry{At: at, From: StatusWip, To: StatusDeleted, Comment: "удалён", Who: "агент"},
			"- 2026-11-16 11:00 — статус: wip → deleted · удалён (агент)\n"},
		{"EN no comment", LangEN, JournalEntry{At: at, From: StatusOpen, To: StatusDeleted, Who: "agent"},
			"- 2026-11-16 11:00 — status: open → deleted (agent)\n"},
		{"EN comment", LangEN, JournalEntry{At: at, From: StatusDone, To: StatusDeleted, Comment: "removed", Who: "agent"},
			"- 2026-11-16 11:00 — status: done → deleted · removed (agent)\n"},
	}
	for _, tc := range cases {
		tk := &Ticket{Journal: []JournalEntry{tc.e}, Lang: tc.lang}
		got, err := Render(tk, nil)
		if err != nil {
			t.Fatalf("%s: Render: %v", tc.name, err)
		}
		if !bytes.Contains(got, []byte(tc.want)) {
			t.Errorf("%s: render output %q misses %q", tc.name, got, tc.want)
		}
	}
}
