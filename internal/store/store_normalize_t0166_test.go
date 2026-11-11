package store

// T-0166 pin for the normalize guard divergence: after T-0166 the renderer
// reproduces a bash-era legacy label line via Ticket.HeadStray, while the
// guard still skips such files. Both sides are pinned so the intentional
// gap cannot drift unnoticed.

import (
	"bytes"
	"testing"

	"ticket/internal/domain"
)

func TestNormalize_LegacyLabelSkipPinnedAfterT0166(t *testing.T) {
	s, dir := newStore(t)
	tk := normalizeTicket("tickets")
	tk.Number = 1
	legacy := delint(renderTicket(t, tk))
	mixed := bytes.Replace(legacy, []byte("- Project (Проект): tickets\n"),
		[]byte("- Project (Проект): tickets\n- Статус: open\n"), 1)
	if bytes.Equal(mixed, legacy) {
		t.Fatal("fixture not built: the project line was not found")
	}
	writeTicketFileBytes(t, dir, "T-0001-open.md", mixed)

	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if len(res.Changed) != 0 {
		t.Errorf("Changed = %v, want none (guard must skip)", res.Changed)
	}
	if len(res.Warnings) != 1 || res.Warnings[0].Reason != reasonUnrecognizedMeta {
		t.Fatalf("Warnings = %v, want exactly one %q", res.Warnings, reasonUnrecognizedMeta)
	}
	if got := mustRead(t, dir, 1); !bytes.Equal(got, mixed) {
		t.Errorf("legacy-label file was rewritten")
	}
	// Divergence pin: the post-T-0166 renderer WOULD reproduce the legacy
	// line (it lands in HeadStray) — the guard stays deliberately stricter.
	parsed, unknown, err := domain.Parse(mixed)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rendered, err := domain.Render(parsed, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Contains(rendered, []byte("- Статус: open")) {
		t.Errorf("Render does not reproduce the legacy line via HeadStray; the pinned guard divergence drifted")
	}
}
