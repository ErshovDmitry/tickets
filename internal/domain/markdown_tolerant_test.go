package domain

import (
	"bytes"
	"strings"
	"testing"
)

func TestUnknownPreservedAfterJournal(t *testing.T) {
	src := ticketT0001 + "- заметка вручную\nещё строка\n"
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if want := []byte("- заметка вручную\nещё строка\n"); !bytes.Equal(unknown, want) {
		t.Errorf("unknown = %q, want %q", unknown, want)
	}
	if len(tk.Unknown) != len(unknown) {
		t.Errorf("Ticket.Unknown length = %d, want %d", len(tk.Unknown), len(unknown))
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Equal(got, []byte(legacyRoundTripWant(src))) {
		t.Errorf("unknown re-emit mismatch:\n got %q\nwant %q", got, src)
	}
}

// TestUnknownBlankLinesPreservedAfterJournal: blanks AFTER the last recognized
// journal line stay Unknown verbatim (T-0070, by design, not a parser bug);
// only the single MD022 blank right after the FIRST header is markup.
func TestUnknownBlankLinesPreservedAfterJournal(t *testing.T) {
	cases := []struct {
		name string
		tail string
	}{
		{"trailing blank line", "\n"},
		{"blank then manual lines", "\n\n- заметка вручную\n\nещё строка\n"},
	}
	for _, tc := range cases {
		src := ticketT0001 + tc.tail
		tk, unknown, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("Parse(%s): %v", tc.name, err)
		}
		if want := []byte(tc.tail); !bytes.Equal(unknown, want) {
			t.Errorf("%s: unknown = %q, want %q", tc.name, unknown, want)
		}
		if len(tk.Journal) != 1 {
			t.Errorf("%s: len(Journal) = %d, want 1", tc.name, len(tk.Journal))
		}
		got, err := Render(tk, unknown)
		if err != nil {
			t.Fatalf("Render(%s): %v", tc.name, err)
		}
		if !bytes.Equal(got, []byte(legacyRoundTripWant(src))) {
			t.Errorf("%s: round trip mismatch:\n got %q\nwant %q", tc.name, got, src)
		}
	}
}

func TestParseTolerantEmptyDetails(t *testing.T) {
	cases := []string{
		"## Details (Подробности)\n\n## Journal (Журнал)\n",      // empty section
		"## Details (Подробности)\n" + dictRU.detailsStub + "\n", // stub
		"## Journal (Журнал)\n",                                  // absent section
	}
	for _, src := range cases {
		tk, _, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("Parse(%q): %v", src, err)
		}
		if tk.Details != "" {
			t.Errorf("Parse(%q).Details = %q, want empty", src, tk.Details)
		}
	}

	// Stub followed by real content is kept verbatim, not normalized.
	src := "## Details (Подробности)\n" + dictRU.detailsStub + "\nпометка\n\n## Journal (Журнал)\n"
	tk, _, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if want := dictRU.detailsStub + "\nпометка"; tk.Details != want {
		t.Errorf("Details = %q, want %q", tk.Details, want)
	}
}

func TestParseTolerantNeverErrors(t *testing.T) {
	for _, src := range []string{
		"",
		"\n\n\n",
		"мусор без структуры",
		"# T-XXXX · ???",
		"# T-1 · BUG: x\n- Status (Статус): bogus\n- Created (Создан): мусор · by (кем): \n",
		"## Journal (Журнал)\n\nне журнал вовсе\n",
		"# T-999999999999999999999 · BUG: overflow\n",
	} {
		tk, unknown, err := Parse([]byte(src))
		if err != nil {
			t.Errorf("Parse(%q) error: %v", src, err)
			continue
		}
		if tk == nil {
			t.Errorf("Parse(%q) ticket = nil", src)
		}
		if unknown == nil && tk.Unknown != nil {
			t.Errorf("Parse(%q): Unknown mismatch between return and field", src)
		}
	}
}

func TestParseUnknownDetailsKept(t *testing.T) {
	src := "# T-0001 · BUG: x\n\n- Status (Статус): open\n- Priority (Приоритет): low\n- Created (Создан): 2026-01-01 10:00 · by (кем): я\n- Project (Проект): p\n\n## Summary (Кратко)\n\nx\n\n## Details (Подробности)\n\nстрока1\n\nстрока2\n\n## User comments (Комментарии от пользователя)\n\n## Comments (Комментарии)\n\n## Journal (Журнал)\n\n- 2026-01-01 10:00 — тикет создан (я).\n"
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if want := "строка1\n\nстрока2"; tk.Details != want {
		t.Errorf("Details = %q, want %q", tk.Details, want)
	}
	if tk.UserComments != "" || tk.Comments != "" {
		t.Errorf("UserComments/Comments = %q/%q, want both empty (stubs blanked)", tk.UserComments, tk.Comments)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Equal(got, []byte(src)) {
		t.Errorf("round trip mismatch:\n got %q\nwant %q", got, src)
	}
}

// TestParseUserCommentsStubCollision documents the known T-0035 limitation
// (by design): user text in "## User comments (Комментарии от пользователя)" byte-identical
// to userCommentsStub is indistinguishable from the placeholder and blanks
// to "" on Parse; Render then re-emits the stub, keeping the round trip
// byte-stable. Control: text differing from the stub (stub + an extra line)
// survives verbatim.
func TestParseUserCommentsStubCollision(t *testing.T) {
	tk, _, err := Parse([]byte(ticketT0001))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.UserComments != "" {
		t.Errorf("UserComments = %q, want empty (stub blanked)", tk.UserComments)
	}

	src := strings.Replace(ticketT0001, dictRU.userCommentsStub+"\n", dictRU.userCommentsStub+"\nдоп. строка\n", 1)
	tk, _, err = Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if want := dictRU.userCommentsStub + "\nдоп. строка"; tk.UserComments != want {
		t.Errorf("UserComments = %q, want %q", tk.UserComments, want)
	}
	got, err := Render(tk, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Equal(got, []byte(src)) {
		t.Errorf("round trip mismatch:\n got %q\nwant %q", got, src)
	}
}
