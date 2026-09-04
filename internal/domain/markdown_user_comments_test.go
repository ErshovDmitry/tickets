package domain

// New-domain tests for the T-0035 two-section layout (plan #1, #2, #3, #7).
// Split from markdown_comments_test.go: the combined suite exceeded the
// 300-line file budget. Helpers (ucTicket, legacyTicket, canonEmptySections)
// live in markdown_comments_test.go, same package.

import (
	"bytes"
	"strings"
	"testing"
)

// TestBothSectionsRoundTrip pins (#1): text in BOTH sections is separated
// by Parse without bleed and re-renders byte-for-byte.
func TestBothSectionsRoundTrip(t *testing.T) {
	uc, fc := "замечание пользователя", "заметка агента\nвторая строка"
	tk, unknown, err := Parse([]byte(ucTicket(uc, fc)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.UserComments != uc || tk.Comments != fc {
		t.Errorf("fields = %q/%q, want %q/%q", tk.UserComments, tk.Comments, uc, fc)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Equal(got, []byte(ucTicket(uc, fc))) {
		t.Errorf("round trip mismatch:\n got %q\nwant %q", got, ucTicket(uc, fc))
	}
}

// TestFreeCommentsEmptyRendersBareHeader pins (#2): an empty free section
// renders exactly "## Comments (Комментарии)\n\n## Journal (Журнал)\n" — no stub, no blank-run.
func TestFreeCommentsEmptyRendersBareHeader(t *testing.T) {
	tk := newTestTicket()
	tk.UserComments = "пометка"
	got, err := Render(tk, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if want := "## Comments (Комментарии)\n\n## Journal (Журнал)\n"; !bytes.Contains(got, []byte(want)) {
		t.Errorf("empty free section must render the bare header:\n%s", got)
	}
	if strings.Contains(string(got), "\n\n\n") {
		t.Errorf("render output must never contain a blank-run \\n\\n\\n:\n%s", got)
	}
	if bytes.Contains(got, []byte("## Comments (Комментарии)\n"+dictRU.userCommentsStub)) {
		t.Errorf("free section must not carry the user stub:\n%s", got)
	}
}

// TestUserSectionInjectedBeforeExistingFreeSection pins (#3): a legacy
// ticket WITH text in the free section (including the old T-0034 stub
// text) gains the user section header-only ABOVE it (the placeholder is
// no longer emitted); free text stays verbatim and is NOT blanked.
func TestUserSectionInjectedBeforeExistingFreeSection(t *testing.T) {
	oldStub := "_Замечания пользователя: пишите сюда — агент прочитает эту секцию перед работой над тикетом._"
	for _, tc := range []struct{ name, fc string }{
		{"plain text", "пометка агента"},
		{"old T-0034 stub", oldStub},
	} {
		src := legacyTicket() + "## Comments (Комментарии)\n" + tc.fc + "\n\n## Journal (Журнал)\n- 2026-01-01 10:00 — тикет создан (я).\n"
		tk, unknown, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("%s: Parse: %v", tc.name, err)
		}
		if tk.Comments != tc.fc {
			t.Errorf("%s: Comments = %q, want %q (must NOT be blanked)", tc.name, tk.Comments, tc.fc)
		}
		if tk.UserComments != "" {
			t.Errorf("%s: UserComments = %q, want empty for a legacy ticket", tc.name, tk.UserComments)
		}
		got, err := Render(tk, unknown)
		if err != nil {
			t.Fatalf("%s: Render: %v", tc.name, err)
		}
		want := legacyTicket() + "## User comments (Комментарии от пользователя)\n\n## Comments (Комментарии)\n" + tc.fc + "\n\n## Journal (Журнал)\n- 2026-01-01 10:00 — тикет создан (я).\n"
		if !bytes.Equal(got, []byte(want)) {
			t.Errorf("%s: round trip mismatch:\n got %q\nwant %q", tc.name, got, want)
		}
	}
}

// legacyRoundTripWant returns src with the legacy UC placeholder line
// removed (any known dictionary, mirroring isStubLine): Parse blanks the
// stub to "" and Render drops it (Option A), so a byte-stable round trip
// compares against src minus the stub line.
func legacyRoundTripWant(src string) string {
	for _, entry := range allDicts {
		src = strings.Replace(src, entry.d.userCommentsStub+"\n", "", 1)
	}
	return src
}

// TestUnknownTailBlanksUserCommentsStub pins (#7): the early-return
// finish() path (manual tail after the journal) blanks a stub-only user
// section exactly like the normal path; the render drops the placeholder
// (bare header) and the tail survives verbatim.
func TestUnknownTailBlanksUserCommentsStub(t *testing.T) {
	src := ucTicket(dictRU.userCommentsStub, "") + "заметка вручную\nещё строка\n"
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.UserComments != "" {
		t.Fatalf("UserComments = %q, want empty (early-return finish path)", tk.UserComments)
	}
	if want := []byte("заметка вручную\nещё строка\n"); !bytes.Equal(unknown, want) {
		t.Fatalf("unknown = %q, want %q", unknown, want)
	}
	got, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// Legacy stub input: parses to "" and renders the bare header.
	if want := legacyRoundTripWant(src); !bytes.Equal(got, []byte(want)) {
		t.Fatalf("round trip mismatch:\n got %q\nwant %q", got, want)
	}
}
