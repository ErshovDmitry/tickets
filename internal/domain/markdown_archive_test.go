package domain

import (
	"bytes"
	"testing"
	"time"
)

// archiveLine is the bash archive journal entry appended by cmd_archive
// (printf without a trailing dot, tickets/bin/ticket:177).
const archiveLine = "- 2026-09-02 12:00 — перенесён в архив (система)\n"

// TestParseArchiveLine pins the archive codec: the archive journal line
// parses into a dedicated entry with Archived=true and empty From/To/
// Comment — archival is a directory move, not a status transition.
func TestParseArchiveLine(t *testing.T) {
	tk, unknown, err := Parse([]byte(ticketT0001 + archiveLine))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(unknown) != 0 {
		t.Errorf("unknown = %q, want empty", unknown)
	}
	if len(tk.Journal) != 2 {
		t.Fatalf("len(Journal) = %d, want 2", len(tk.Journal))
	}
	e := tk.Journal[1]
	if !e.Archived {
		t.Errorf("entry[1].Archived = false, want true")
	}
	if e.From != "" || e.To != "" || e.Comment != "" {
		t.Errorf("entry[1] From/To/Comment = %q/%q/%q, want all empty", e.From, e.To, e.Comment)
	}
	if e.Who != "система" {
		t.Errorf("entry[1].Who = %q, want система", e.Who)
	}
	if !e.At.Equal(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("entry[1].At = %v, want 2026-09-02 12:00", e.At)
	}
}

// TestRenderArchiveLineByteExact pins the bash printf shape: the archive
// entry renders as "- TS — перенесён в архив (who)\n" and must NOT gain a
// trailing dot (the dot would diverge from tickets/bin/ticket:177). The
// always-emitted comments section (T-0032) must not disturb the line.
func TestRenderArchiveLineByteExact(t *testing.T) {
	at := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	tk := &Ticket{
		Number: 1, Type: TypeBUG, Priority: PriorityNormal,
		Title: "x", Who: "a", Project: "p",
		Created: at, Status: StatusOpen,
		Lang:    LangRU,
		Journal: []JournalEntry{{At: at, Who: "система", Archived: true}},
	}
	got, err := Render(tk, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := "- 2026-09-02 12:00 — перенесён в архив (система)\n"
	if !bytes.Contains(got, []byte(want)) {
		t.Errorf("render output misses the byte-exact archive line:\n%s", got)
	}
	if bytes.Contains(got, []byte("(система).\n")) {
		t.Errorf("archive line must NOT end with a dot:\n%s", got)
	}
	// T-0035: empty sections render the bare UC header and the bare free
	// header (the placeholder is no longer emitted).
	if !bytes.Contains(got, []byte(canonEmptySections())) {
		t.Errorf("render output misses the canonical two-section layout:\n%s", got)
	}
}

// TestArchiveReopenJournalRoundTrip pins the full reopen cycle
// (created → done → archived → reopened): a four-line journal parses in
// chronological order and renders back byte-for-byte, so no entry is
// reordered or dropped by the codec.
func TestArchiveReopenJournalRoundTrip(t *testing.T) {
	src := ticketT0001 +
		"- 2026-09-02 10:00 — статус: open → done (agent)\n" +
		archiveLine +
		"- 2026-09-02 13:00 — статус: done → open (agent)\n"
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(unknown) != 0 {
		t.Errorf("unknown = %q, want empty", unknown)
	}
	if len(tk.Journal) != 4 {
		t.Fatalf("len(Journal) = %d, want 4", len(tk.Journal))
	}
	created, done, archived, reopened := tk.Journal[0], tk.Journal[1], tk.Journal[2], tk.Journal[3]
	if created.From != StatusOpen || created.To != StatusOpen {
		t.Errorf("entry[0] = %v→%v, want open→open (creation)", created.From, created.To)
	}
	if done.From != StatusOpen || done.To != StatusDone {
		t.Errorf("entry[1] = %v→%v, want open→done", done.From, done.To)
	}
	if !archived.Archived {
		t.Errorf("entry[2].Archived = false, want true")
	}
	if reopened.From != StatusDone || reopened.To != StatusOpen {
		t.Errorf("entry[3] = %v→%v, want done→open (reopen)", reopened.From, reopened.To)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Equal(got, []byte(legacyRoundTripWant(src))) {
		t.Errorf("round trip mismatch (chronology lost):\n got %q\nwant %q", got, src)
	}
}

// TestParseArchiveKeepsStatus pins §8a: the archive journal line must not
// touch the "- Status (Статус): " metadata, so Ticket.Status stays as written.
func TestParseArchiveKeepsStatus(t *testing.T) {
	tk, _, err := Parse([]byte(ticketT0001 + archiveLine))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.Status != StatusOpen {
		t.Errorf("Status = %q, want open (archive line must not change status)", tk.Status)
	}
}
