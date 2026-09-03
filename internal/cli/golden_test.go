package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ticket/internal/domain"
)

// TestGoldenRawBytes pins §7.3: RenderNewTicket over the FIXED golden
// ticket must equal the immutable fixture
// internal/cli/testdata/golden-T-0001-open.md byte-for-byte — no
// normalization, no transformation, no trimming. The fixture is the
// canonical output of the GO renderer: originally a byte copy of the
// bash-created tickets/T-0001-open.md, deliberately regenerated when the
// renderer layout changes by design — T-0032 added the "## Комментарии"
// section, intentionally breaking bash byte-compat of `new`. Any other
// drift is a renderer bug.
func TestGoldenRawBytes(t *testing.T) {
	created := time.Date(2026, 9, 2, 3, 24, 0, 0, time.UTC)
	tk := &domain.Ticket{
		Number:   1,
		Status:   domain.StatusOpen,
		Type:     domain.TypeENH,
		Priority: domain.PriorityHigh,
		Title:    goldenTitle,
		Details:  goldenDetails,
		Who:      goldenWho,
		Project:  "tickets",
		Created:  created,
		Journal: []domain.JournalEntry{{
			At:   created,
			From: domain.StatusOpen,
			To:   domain.StatusOpen,
			Who:  goldenWho,
		}},
	}
	got, err := domain.RenderNewTicket(tk)
	if err != nil {
		t.Fatalf("RenderNewTicket: %v", err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "golden-T-0001-open.md"))
	if err != nil {
		t.Fatalf("golden fixture unreadable: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("RenderNewTicket != golden (raw bytes):\n--- got ---\n%q\n--- want ---\n%q", got, want)
	}
}
