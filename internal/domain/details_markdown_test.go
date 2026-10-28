package domain

import (
	"bytes"
	"strings"
	"testing"
)

// detailsMultilineBody is the CURRENT Details format (T-0109): a Russian
// numbered list, one item per line, preceded by a short sentence. It is
// the Details body of a real ticket file and must survive Parse→Render
// byte-exactly — the multiline body is content, never a stub.
const detailsMultilineBody = `Решение пользователя 2026-10-27: вариант A — нумерованный список, один пункт на строку.
1. mapsloop ×1 — internal/cli/usage_i18n_test.go:27; заменить на maps.Copy.
2. go-ignored-call-result ×6 — cli_test.go:120,292; проверять ошибки Glob/Stat.
3. go-path-traversal ×4 — ложные срабатывания (путь из t.TempDir); wontfix.`

// ruTicketWithDetails builds a canonical RU ticket file whose Details
// section carries body. The result is byte-identical to what Render emits
// for the same ticket, so Parse→Render can be compared byte-exactly.
func ruTicketWithDetails(body string) string {
	return "# T-0109 · ENH: формат раздела Details\n" +
		"\n" +
		"- Status (Статус): open\n" +
		"- Priority (Приоритет): normal\n" +
		"- Created (Создан): 2026-10-27 14:17 · by (кем): erdmitry\n" +
		"- Project (Проект): tickets\n" +
		"\n" +
		"## Summary (Кратко)\n" +
		"\n" +
		"формат раздела Details\n" +
		"\n" +
		"## Details (Подробности)\n" +
		"\n" +
		body + "\n" +
		"\n" +
		"## User comments (Комментарии от пользователя)\n" +
		"\n" +
		"## Comments (Комментарии)\n" +
		"\n" +
		"## Journal (Журнал)\n" +
		"\n" +
		"- 2026-10-27 14:17 — тикет создан (erdmitry).\n"
}

// TestDetailsMultilineNumberedRoundTrip pins the CURRENT behaviour of the
// markdown layer for a multiline Russian numbered Details body: Parse
// keeps it verbatim and Render re-emits the whole file byte-exactly.
// The template layer (empty Details → detailsFormat) is covered by
// template_test.go and is NOT duplicated here.
func TestDetailsMultilineNumberedRoundTrip(t *testing.T) {
	src := ruTicketWithDetails(detailsMultilineBody)
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(unknown) != 0 {
		t.Errorf("unknown = %q, want empty", unknown)
	}
	if tk.Details != detailsMultilineBody {
		t.Errorf("Details = %q, want %q", tk.Details, detailsMultilineBody)
	}
	out, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Equal(out, []byte(src)) {
		t.Errorf("Parse→Render is not byte-stable:\n got %q\nwant %q", out, src)
	}
}

// TestDetailsLegacyStubParsesEmpty pins the legacy placeholder: a ticket
// file whose Details section holds the detailsStub line parses to an empty
// Details and Render re-emits the stub (the marker of an empty section).
func TestDetailsLegacyStubParsesEmpty(t *testing.T) {
	src := ruTicketWithDetails(dictRU.detailsStub)
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(unknown) != 0 {
		t.Errorf("unknown = %q, want empty", unknown)
	}
	if tk.Details != "" {
		t.Errorf("Details = %q, want empty (legacy stub)", tk.Details)
	}
	out, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(out), dictRU.detailsStub) {
		t.Errorf("Render did not re-emit the legacy stub:\n%s", out)
	}
	if !bytes.Equal(out, []byte(src)) {
		t.Errorf("Parse→Render is not byte-stable:\n got %q\nwant %q", out, src)
	}
}

// TestDetailsFormatBlockParsesAsContent pins the NEW detailsFormat block
// (T-0109) when it appears in a ticket file as CONTENT: it is not a stub
// line, so Parse keeps it verbatim and Render re-emits it as content
// instead of the legacy stub.
func TestDetailsFormatBlockParsesAsContent(t *testing.T) {
	if dictRU.detailsFormat == dictRU.detailsStub {
		t.Fatal("detailsFormat equals detailsStub: the format block would be lost")
	}
	src := ruTicketWithDetails(dictRU.detailsFormat)
	tk, unknown, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(unknown) != 0 {
		t.Errorf("unknown = %q, want empty", unknown)
	}
	if tk.Details != dictRU.detailsFormat {
		t.Errorf("Details = %q, want %q", tk.Details, dictRU.detailsFormat)
	}
	out, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(out), dictRU.detailsFormat) {
		t.Errorf("Render did not re-emit the format block as content:\n%s", out)
	}
	if strings.Contains(string(out), dictRU.detailsStub) {
		t.Errorf("Render emitted the legacy stub instead of the format block:\n%s", out)
	}
	if !bytes.Equal(out, []byte(src)) {
		t.Errorf("Parse→Render is not byte-stable:\n got %q\nwant %q", out, src)
	}
}
