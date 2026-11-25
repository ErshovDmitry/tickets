package store

// T-0245 C2 v2: replace/append through the write path preserve the OLD
// block's raw CRLF bytes and leave seeded CRLF neighbours, the protected
// user-comments section and the unknown tail byte-identical.

import (
	"bytes"
	"strings"
	"testing"

	"ticket/internal/domain"
)

const rawSeedHead = "# T-0001 · ENH: ticket-0\n\n" +
	"- Status (Статус): open\n- Priority (Приоритет): normal\n" +
	"- Created (Создан): 2026-01-01 10:00 · by (кем): tester\n- Project (Проект): tickets\n\n"

func rawSeedSection(header string, lines ...string) string {
	return header + "\n\n" + strings.Join(lines, "\r\n") + "\n\n"
}

func rawSeedSrc() string {
	return rawSeedHead +
		rawSeedSection("## Summary (Кратко)", "brief один", "brief два") +
		rawSeedSection("## Details (Подробности)", "detail один", "detail два") +
		rawSeedSection("## User comments (Комментарии от пользователя)", "user один", "user два") +
		rawSeedSection("## Comments (Комментарии)", "comment один", "comment два") +
		"## Journal (Журнал)\n\n- 2026-01-01 10:00 — тикет создан (tester).\n" +
		"\nmanual tail\r\n"
}

func seedRaw(t *testing.T) (*Store, string) {
	t.Helper()
	s, dir := newStore(t)
	writeTicketFileBytes(t, dir, "T-0001-open.md", []byte(rawSeedSrc()))
	return s, dir
}

// TestSetSection_PreservesCRLFNeighbours: replacing one editable section
// leaves the other sections' CRLF bodies and the unknown tail byte-identical.
func TestSetSection_PreservesCRLFNeighbours(t *testing.T) {
	blocks := map[domain.Section]string{
		domain.SectionSummary:      "brief один\r\nbrief два",
		domain.SectionDetails:      "detail один\r\ndetail два",
		domain.SectionUserComments: "user один\r\nuser два",
		domain.SectionComments:     "comment один\r\ncomment два",
	}
	for _, target := range []domain.Section{domain.SectionSummary, domain.SectionDetails, domain.SectionComments} {
		t.Run(string(target), func(t *testing.T) {
			s, dir := seedRaw(t)
			if _, err := s.SetSection(1, target, "замена\r\nстрока", "alice"); err != nil {
				t.Fatalf("SetSection: %v", err)
			}
			body := mustRead(t, dir, 1)
			for sec, block := range blocks {
				if sec == target {
					continue
				}
				if !bytes.Contains(body, []byte(block)) {
					t.Fatalf("neighbour %s CRLF lost:\n%q", sec, body)
				}
			}
			if !bytes.Contains(body, []byte("замена\r\nстрока")) {
				t.Fatalf("replacement CRLF lost:\n%q", body)
			}
			if !bytes.HasSuffix(body, []byte("\nmanual tail\r\n")) {
				t.Fatalf("unknown tail changed:\n%q", body)
			}
		})
	}
}

// TestSetSection_SummaryThenAppendPreservesBrief: a CRLF Brief written first
// survives a later comment append byte-for-byte.
func TestSetSection_SummaryThenAppendPreservesBrief(t *testing.T) {
	s, dir := seedRaw(t)
	if _, err := s.SetSection(1, domain.SectionSummary, "итог\r\nновый", "alice"); err != nil {
		t.Fatalf("SetSection: %v", err)
	}
	if _, err := s.AddComment(1, "коммент\r\nблок", "bob"); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	body := mustRead(t, dir, 1)
	if !bytes.Contains(body, []byte("итог\r\nновый")) {
		t.Fatalf("old CRLF Brief normalized by append:\n%q", body)
	}
	if !bytes.Contains(body, []byte("коммент\r\nблок")) {
		t.Fatalf("appended CRLF block lost:\n%q", body)
	}
}

// TestSetSection_RepeatedCommentsKeepsNeighbours: a second Comments replace
// leaves Details and the CRLF Brief untouched.
func TestSetSection_RepeatedCommentsKeepsNeighbours(t *testing.T) {
	s, dir := seedRaw(t)
	if _, err := s.SetSection(1, domain.SectionComments, "первый\r\nзамена", "alice"); err != nil {
		t.Fatalf("SetSection 1: %v", err)
	}
	if _, err := s.SetSection(1, domain.SectionComments, "второй\r\nзамена", "bob"); err != nil {
		t.Fatalf("SetSection 2: %v", err)
	}
	body := mustRead(t, dir, 1)
	for _, keep := range []string{"detail один\r\ndetail два", "brief один\r\nbrief два", "второй\r\nзамена"} {
		if !bytes.Contains(body, []byte(keep)) {
			t.Fatalf("lost %q:\n%q", keep, body)
		}
	}
	if bytes.Contains(body, []byte("первый\r\nзамена")) {
		t.Fatalf("first replacement survived second:\n%q", body)
	}
}

// TestAddComment_LFOldBlockNoCR: an LF-only old block acquires no CR when a
// CRLF payload is appended; only the new block carries CRLF.
func TestAddComment_LFOldBlockNoCR(t *testing.T) {
	s, dir := seedRaw(t)
	if _, err := s.SetSection(1, domain.SectionComments, "старый\nблок", "alice"); err != nil {
		t.Fatalf("SetSection: %v", err)
	}
	if _, err := s.AddComment(1, "новый\r\nблок", "bob"); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	body := mustRead(t, dir, 1)
	if !bytes.Contains(body, []byte("старый\nблок\n\nновый\r\nблок")) {
		t.Fatalf("LF old block or CRLF new block wrong:\n%q", body)
	}
	if bytes.Contains(body, []byte("старый\r")) {
		t.Fatalf("LF old block acquired CR:\n%q", body)
	}
}

// TestAddComment_MixedEOLPreserved: mixed internal CRLF/LF and an internal
// CRLF blank line in the old block survive an append byte-for-byte.
func TestAddComment_MixedEOLPreserved(t *testing.T) {
	s, dir := seedRaw(t)
	const mixed = "a\r\nb\nc\r\n\r\nd"
	if _, err := s.SetSection(1, domain.SectionComments, mixed, "alice"); err != nil {
		t.Fatalf("SetSection: %v", err)
	}
	if _, err := s.AddComment(1, "новый", "bob"); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	body := mustRead(t, dir, 1)
	if !bytes.Contains(body, []byte(mixed+"\n\nновый")) {
		t.Fatalf("mixed EOL / internal CRLF blank lost:\n%q", body)
	}
}
