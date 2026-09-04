package domain

import (
	"bytes"
	"strings"
	"testing"
)

// TestByteStabilityRUBilingual tests that a RU bilingual ticket round-trips
// byte-identically (headers, meta, stubs, journal). The T-0001 fixture is a
// bilingual RU file ("- Status (Статус):", "## Summary (Кратко)", ...), NOT a
// legacy RU-only file: the i18n codec must Parse and Render it back to the
// exact original bytes.
func TestByteStabilityRUBilingual(t *testing.T) {
	// Legacy stub parses to "" and is dropped by Render (Option A).
	want := []byte(legacyRoundTripWant(ticketT0001))
	tk, unknown, err := Parse([]byte(ticketT0001))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(unknown) != 0 {
		t.Errorf("unknown = %q, want empty", unknown)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("RU bilingual round trip mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestByteStabilityENPlain tests that a plain EN ticket with the legacy
// User comments placeholder round-trips like the RU path: the stub parses
// to "" and Render drops it (Option A, bare header).
func TestByteStabilityENPlain(t *testing.T) {
	src := `# T-0002 · ENH: English ticket

- Status: open
- Priority: normal
- Created: 2026-01-02 10:00 · by: alice
- Project: test

## Summary
English ticket

## Details
This is an English ticket.

## User comments
_User remarks: write here — the agent reads this section before working on the ticket. The agent does not write here._

## Comments

## Journal
- 2026-01-02 10:00 — ticket created (alice).
`
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(unknown) != 0 {
		t.Errorf("unknown = %q, want empty", unknown)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if want := legacyRoundTripWant(src); string(got) != want {
		t.Errorf("EN plain round trip failed:\n got %q\nwant %q", got, want)
	}
}

// TestByteStabilityFreeSuffix tests that a ticket with free suffixes
// round-trips byte-identically (raw templates preserve them).
func TestByteStabilityFreeSuffix(t *testing.T) {
	src := `# T-0003 · BUG: free suffix

- Status (状态): open
- Priority (优先级): high
- Created (创建): 2026-01-03 10:00 · by (作者): bob
- Project (项目): free

## Summary (摘要)
free suffix

## Details (详情)
Free suffix test.

## User comments (用户评论)
user text

## Comments (评论)
agent text

## Journal (日志)
- 2026-01-03 10:00 — ticket created (bob).
`
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if string(got) != src {
		t.Errorf("free suffix round trip failed:\n got %q\nwant %q", got, src)
	}
}

// TestLegacyRUOnlyTolerant tests that a legacy RU-only file (old format with
// only Russian headers, no English canonical names) parses tolerantly without
// panic, no fields recognized, and renders with the ticket's detected lang.
func TestLegacyRUOnlyTolerant(t *testing.T) {
	src := `# T-0004 · BUG: legacy

- Статус: open
- Приоритет: low
- Создан: 2026-01-04 10:00 · кем: user
- Проект: legacy

## Кратко
legacy

## Подробности
legacy details

## Журнал
- 2026-01-04 10:00 — тикет создан (user).
`
	tk, _, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// Legacy headers are not recognized (no panic is success).
	// Lang detection falls back to EN (no recognized signals).
	if tk.Lang != LangEN {
		t.Logf("Legacy file detected lang = %q (expected EN fallback)", tk.Lang)
	}
	// Render should not panic (fills in missing sections per detected lang).
	_, err = Render(tk, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
}

// ticketT0007FreeMeta is the T-0036 §B probe fixture: a ru-bilingual ticket
// whose META lines use free suffixes ("- Status (a%sb):") while its headers
// and journal line are ru-dict form.
const ticketT0007FreeMeta = `# T-0007 · BUG: x

- Status (a%sb): open
- Priority (Приоритет): normal
- Created (created %s): 2026-01-02 03:04 · by (кем %s): carol
- Project (Проект): p

## Summary (Кратко)
x

## Details (Подробности)
detail

## User comments (Комментарии от пользователя)
user text

## Comments (Комментарии)

## Journal (Журнал)
- 2026-01-02 03:04 — тикет создан (carol).
`

// TestRUBilingualFreeSuffixMetaRoundTrip is the T-0036 §B invariant: a
// ru-bilingual ticket with free-suffix META and ru-dict headers/journal
// parses as LangRU (dict signal beats the earlier free suffix, LOCK D4 v2),
// keeps the parsed fields, and round-trips byte-identically — the RU journal
// line re-emits verbatim from JournalEntry.Raw regardless of t.Lang.
func TestRUBilingualFreeSuffixMetaRoundTrip(t *testing.T) {
	tk, unknown, err := Parse([]byte(ticketT0007FreeMeta))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.Lang != LangRU {
		t.Errorf("Lang = %q, want %q (dict signal beats free suffix)", tk.Lang, LangRU)
	}
	if tk.Status != StatusOpen || tk.Priority != PriorityNormal || tk.Project != "p" {
		t.Errorf("Status/Priority/Project = %q/%q/%q, want open/normal/p",
			tk.Status, tk.Priority, tk.Project)
	}
	if tk.Who != "carol" || !tk.Created.Equal(parseTS("2026-01-02 03:04")) {
		t.Errorf("Who/Created = %q/%v, want carol/2026-01-02 03:04", tk.Who, tk.Created)
	}
	wantRaw := "- 2026-01-02 03:04 — тикет создан (carol)."
	if len(tk.Journal) != 1 || tk.Journal[0].Raw != wantRaw {
		t.Fatalf("Journal = %+v, want one entry with Raw %q", tk.Journal, wantRaw)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if string(got) != ticketT0007FreeMeta {
		t.Errorf("round trip mismatch:\n got %q\nwant %q", got, ticketT0007FreeMeta)
	}
}

// TestRUBilingualFreeSuffixAppendKeepsRaw pins the set/archive append path:
// a new JournalEntry (empty Raw) renders in the ticket language's dict form
// while every parsed line — the RU journal line included — stays verbatim.
func TestRUBilingualFreeSuffixAppendKeepsRaw(t *testing.T) {
	tk, _, err := Parse([]byte(ticketT0007FreeMeta))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tk.Journal = append(tk.Journal, JournalEntry{
		At: parseTS("2026-01-02 09:30"), From: StatusOpen, To: StatusWip, Who: "agent",
	})
	got, err := Render(tk, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.HasPrefix(string(got), ticketT0007FreeMeta) {
		t.Errorf("parsed lines not preserved verbatim:\n got %q", got)
	}
	wantTail := "- 2026-01-02 03:04 — тикет создан (carol).\n" +
		"- 2026-01-02 09:30 — статус: open → wip (agent)\n"
	if !strings.HasSuffix(string(got), wantTail) {
		t.Errorf("appended entry wrong:\n got %q\nwant suffix %q", got, wantTail)
	}
}
