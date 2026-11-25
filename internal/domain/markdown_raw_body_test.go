package domain

// T-0245 C2 v2: the raw-body sidecar preserves editable bodies' CR bytes
// across Parse->Render while the public fields stay parser-normalized.
// Structure (head/meta/headers/journal) renders LF, so these fixtures mix
// LF structure with CRLF bodies — the realistic Windows-authored case.

import (
	"bytes"
	"strings"
	"testing"
)

const rawBodyHead = "# T-0001 · BUG: raw sidecar\n\n" +
	"- Status (Статус): open\n" +
	"- Priority (Приоритет): normal\n" +
	"- Created (Создан): 2026-01-01 10:00 · by (кем): tester\n" +
	"- Project (Проект): tickets\n\n"

// crlfSection builds one section whose body lines are joined by CRLF
// (interior CR preserved) with LF structure around them.
func crlfSection(header string, lines ...string) string {
	return header + "\n\n" + strings.Join(lines, "\r\n") + "\n\n"
}

func rawBodySrc() string {
	return rawBodyHead +
		crlfSection("## Summary (Кратко)", "итог один", "итог два") +
		crlfSection("## Details (Подробности)", "деталь один", "деталь два") +
		crlfSection("## User comments (Комментарии от пользователя)", "замечание один") +
		crlfSection("## Comments (Комментарии)", "коммент один", "коммент два") +
		"## Journal (Журнал)\n\n- 2026-01-01 10:00 — тикет создан (tester).\n" +
		"\nmanual tail\r\n"
}

// TestParseRenderRawBodyRoundTrip pins byte-identity for CRLF editable
// bodies plus free-suffix headers, a CRLF unknown tail and populated
// neighbours, and that repeated cycles accumulate no structural blanks.
func TestParseRenderRawBodyRoundTrip(t *testing.T) {
	src := []byte(rawBodySrc())
	tk, unknown, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Equal(out, src) {
		t.Fatalf("round trip not byte-identical:\n got %q\nwant %q", out, src)
	}
	for i := 0; i < 3; i++ {
		tk2, u2, err := Parse(out)
		if err != nil {
			t.Fatalf("cycle %d Parse: %v", i, err)
		}
		out2, err := Render(tk2, u2)
		if err != nil {
			t.Fatalf("cycle %d Render: %v", i, err)
		}
		if !bytes.Equal(out2, out) {
			t.Fatalf("cycle %d drifted:\n got %q\nwant %q", i, out2, out)
		}
		out = out2
	}
}

// TestRawBodyPublicFieldsNormalized pins the LOCK: SectionBody and the
// public fields stay parser-normalized (LF) even though the file carries
// CRLF bodies; only the internal sidecar keeps the CR bytes.
func TestRawBodyPublicFieldsNormalized(t *testing.T) {
	tk, _, err := Parse([]byte(rawBodySrc()))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cases := []struct {
		sec  Section
		want string
	}{
		{SectionSummary, "итог один\nитог два"},
		{SectionDetails, "деталь один\nдеталь два"},
		{SectionUserComments, "замечание один"},
		{SectionComments, "коммент один\nкоммент два"},
	}
	for _, tc := range cases {
		if got := tk.SectionBody(tc.sec); got != tc.want {
			t.Errorf("SectionBody(%s) = %q, want %q", tc.sec, got, tc.want)
		}
	}
	if strings.Contains(tk.Details, "\r") || strings.Contains(tk.Comments, "\r") {
		t.Errorf("public fields leaked CR: details=%q comments=%q", tk.Details, tk.Comments)
	}
}

// TestRawBodyWhitespaceOnlyIsContent pins that a whitespace-only line is
// content, never a structural blank, in the raw sidecar too.
func TestRawBodyWhitespaceOnlyIsContent(t *testing.T) {
	src := rawBodyHead +
		crlfSection("## Details (Подробности)", "   ", "деталь") +
		"## Journal (Журнал)\n\n- 2026-01-01 10:00 — тикет создан (tester).\n"
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := tk.SectionBody(SectionDetails); got != "   \nдеталь" {
		t.Fatalf("Details = %q, want whitespace-only line kept", got)
	}
	out, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// The whitespace-only CRLF line stays content byte-for-byte, and a
	// re-parse keeps it (never collapsed into a structural blank).
	if !bytes.Contains(out, []byte("   \r\nдеталь")) {
		t.Fatalf("whitespace-only CRLF line not preserved:\n%q", out)
	}
	tk2, _, err := Parse(out)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	if got := tk2.SectionBody(SectionDetails); got != "   \nдеталь" {
		t.Fatalf("re-parse lost whitespace content: %q", got)
	}
}

// TestRawBodyCRLFStubRecognized pins that a CRLF-terminated stub still
// parses to an empty body and Render emits the dict stub, not the raw CR.
func TestRawBodyCRLFStubRecognized(t *testing.T) {
	src := rawBodyHead +
		"## Details (Подробности)\n\n" + dictRU.detailsStub + "\r\n\n" +
		"## Journal (Журнал)\n\n- 2026-01-01 10:00 — тикет создан (tester).\n"
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tk.Details != "" {
		t.Fatalf("CRLF stub not recognized: Details = %q", tk.Details)
	}
	out, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Contains(out, []byte(dictRU.detailsStub)) || bytes.Contains(out, []byte(dictRU.detailsStub+"\r")) {
		t.Fatalf("stub render resurrected CR or lost stub:\n%q", out)
	}
}

// TestRawBodyLoneCRLineIsContent pins the split between the two blank
// predicates: the public trim calls a line blank only when it is exactly
// "", while the raw-sidecar trim also treats a lone "\r" as the CRLF blank
// left by the LF-only split. A raw "\r\r\n" line therefore arrives as the
// public line "\r" — REAL content — and must survive the public fields, the
// raw sidecar and Render byte-for-byte. The last case is the contrast: a
// true CRLF blank is still trimmed on both sides.
func TestRawBodyLoneCRLineIsContent(t *testing.T) {
	const journal = "## Journal (Журнал)\n\n- 2026-01-01 10:00 — тикет создан (tester).\n"
	check := func(t *testing.T, src, wantDetails, wantRaw string) {
		t.Helper()
		tk, unknown, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if tk.Details != wantDetails {
			t.Fatalf("Details = %q, want %q", tk.Details, wantDetails)
		}
		rb, ok := tk.RawTemplates.Bodies[secNameDetails]
		if !ok || rb.raw != wantRaw {
			t.Fatalf("raw sidecar = %q (present=%v), want %q", rb.raw, ok, wantRaw)
		}
		out, err := Render(tk, unknown)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if !bytes.Contains(out, []byte(wantRaw)) {
			t.Fatalf("render lost raw bytes %q:\n%q", wantRaw, out)
		}
	}
	t.Run("interior CR content kept", func(t *testing.T) {
		check(t, rawBodyHead+
			"## Details (Подробности)\n\nдеталь один\r\n\r\r\nдеталь два\n\n"+journal,
			"деталь один\n\r\nдеталь два",
			"деталь один\r\n\r\r\nдеталь два")
	})
	t.Run("trailing CR content kept", func(t *testing.T) {
		check(t, rawBodyHead+
			"## Details (Подробности)\n\nдеталь один\r\n\r\r\n\n"+journal,
			"деталь один\n\r",
			"деталь один\r\n\r\r")
	})
	t.Run("real CRLF blank still trimmed", func(t *testing.T) {
		check(t, rawBodyHead+
			"## Details (Подробности)\n\nдеталь один\r\n\r\n\r\n"+journal,
			"деталь один",
			"деталь один\r")
	})
}
