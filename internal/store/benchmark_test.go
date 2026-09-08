package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ticket/internal/domain"
)

// benchCreated is a fixed timestamp so benchmark files are deterministic
// and their Created meta line always parses.
var benchCreated = time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)

// BenchmarkList measures the cost of one full List scan over a store of
// header+body ticket files, exercising the header-only read path. The
// files carry a real body so the header-only advantage is measurable:
// List stops reading at the first "## " line instead of the whole file.
func BenchmarkList(b *testing.B) {
	const n = 100
	dir := b.TempDir()
	s, err := New(dir)
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	body := strings.Repeat("body line\n", 100)
	for i := 1; i <= n; i++ {
		t := &domain.Ticket{
			Number:   i,
			Status:   domain.StatusOpen,
			Type:     domain.TypeBUG,
			Priority: domain.PriorityNormal,
			Title:    fmt.Sprintf("benchmark ticket %d", i),
			Who:      "bench",
			Project:  "tickets",
			Lang:     domain.LangRU,
			Created:  benchCreated,
		}
		data, rerr := domain.RenderNewTicket(t)
		if rerr != nil {
			b.Fatalf("RenderNewTicket(%d): %v", i, rerr)
		}
		// Append a body so the header-only read advantage is measurable.
		data = append(data, []byte("\n## Details\n"+body)...)
		if werr := os.WriteFile(filepath.Join(dir, domain.Filename(t.Number, t.Status)), data, 0o644); werr != nil {
			b.Fatalf("write %d: %v", i, werr)
		}
	}
	// The scan must see all n tickets; otherwise the benchmark measures a
	// broken store.
	tickets, warns := s.List()
	if len(tickets) != n {
		b.Fatalf("List returned %d tickets, want %d (warnings=%v)", len(tickets), n, warns)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s.List()
	}
}
