package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ticket/internal/domain"
)

// goldenFixture reads the immutable golden fixture used by the raw-bytes
// golden tests. An absent testdata/ directory means the test binary runs
// outside the source tree (e.g. a compiled cli.test invoked from an
// arbitrary CWD without a checkout), which is a legitimate environment —
// such runs skip instead of failing. A present but unreadable fixture is
// a repository regression and must fail hard.
func goldenFixture(t *testing.T) []byte {
	t.Helper()
	if _, err := os.Stat("testdata"); err != nil {
		if os.IsNotExist(err) {
			t.Skipf("testdata/ not found: golden tests run only inside the source tree (run go test from the repo root)")
		}
		t.Fatalf("testdata/ not accessible: %v", err)
	}
	b, err := os.ReadFile(filepath.Join("testdata", "golden-T-0001-open.md"))
	if err != nil {
		t.Fatalf("golden fixture unreadable: %v", err)
	}
	return b
}

// TestGoldenRawBytes pins §7.3: RenderNewTicket over the FIXED golden
// ticket must equal the immutable fixture
// internal/cli/testdata/golden-T-0001-open.md byte-for-byte — no
// normalization, no transformation, no trimming. The fixture is the
// canonical output of the GO renderer: originally a byte copy of the
// bash-created tickets/T-0001-open.md, deliberately regenerated when the
// renderer layout changes by design — T-0032 added the "## Комментарии"
// section and T-0035 split it into the stubbed
// "## Комментарии от пользователя" plus the free-form "## Комментарии",
// intentionally breaking bash byte-compat of `new`; T-0036 re-pinned it to
// the ru-bilingual renderer output; the empty-UC plan (Option A) dropped
// the placeholder line — new tickets create the section empty. Any other
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
		Lang:     domain.LangRU,
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
	want := goldenFixture(t)
	if !bytes.Equal(got, want) {
		t.Fatalf("RenderNewTicket != golden (raw bytes):\n--- got ---\n%q\n--- want ---\n%q", got, want)
	}
}
