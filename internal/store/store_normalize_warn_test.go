package store

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// TestNormalize_BodyStatusMismatchWarns pins the V19 notice: a body status
// that disagrees with the file name is replaced by the name (the file is
// still rewritten, not skipped) and reported exactly once, so the divergence
// is not silently discarded.
func TestNormalize_BodyStatusMismatchWarns(t *testing.T) {
	s, dir := newStore(t)
	_, canonical := canonicalNormalize(t, s, dir)
	tamperBodyStatus(t, dir, 1, domain.StatusDone)

	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	wantChanged(t, res.Changed, filepath.Join(dir, "T-0001-open.md"))
	if len(res.Warnings) != 1 || res.Warnings[0].Reason != reasonStatusFromName {
		t.Fatalf("Warnings = %v, want exactly one %q", res.Warnings, reasonStatusFromName)
	}
	want := `T-0001-open.md: body status "done" differs from file name; the file name wins`
	if got := res.Warnings[0].Error(); got != want {
		t.Errorf("warning = %q, want %q", got, want)
	}
	got := mustRead(t, dir, 1)
	if !bytes.Equal(got, canonical) {
		t.Errorf("body not canonicalized to the name's status:\n--- got ---\n%s", got)
	}
	if n := strings.Count(string(got), " — тикет создан "); n != 1 {
		t.Errorf("journal creation entries = %d, want 1 (no line may be appended)", n)
	}
}

// TestNormalize_NoStatusWarningWhenStatusesMatch is the counterpart of the
// notice above: identical statuses produce no warning even on a rewrite, so
// the warning really signals the divergence and not every normalize run.
func TestNormalize_NoStatusWarningWhenStatusesMatch(t *testing.T) {
	s, dir := newStore(t)
	_, canonical := canonicalNormalize(t, s, dir)
	path := writeTicketFileBytes(t, dir, "T-0001-open.md", delint(canonical))

	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	wantChanged(t, res.Changed, path)
	if len(res.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none", res.Warnings)
	}
}

// TestNormalize_RestoreFailureHasOwnReason pins the diagnostics split: when
// the rename succeeded and only the attribute restore failed, the rewrite
// stands (the file is changed, the run returns no error) and the warning
// carries reasonRestoreAttrs instead of the read-only skip code, so "file
// left untouched" and "file rewritten, attribute not restored" stay apart.
func TestNormalize_RestoreFailureHasOwnReason(t *testing.T) {
	s, dir := newStore(t)
	_, canonical := canonicalNormalize(t, s, dir)
	path := writeTicketFileBytes(t, dir, "T-0001-open.md", delint(canonical))
	prev := prepareReplace
	prepareReplace = func(string) (func() error, error) {
		return func() error { return errors.New("restore boom") }, nil
	}
	t.Cleanup(func() { prepareReplace = prev })

	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("Normalize = %v, want nil (the bytes are committed)", err)
	}
	wantChanged(t, res.Changed, path)
	if len(res.Warnings) != 1 || res.Warnings[0].Reason != reasonRestoreAttrs {
		t.Fatalf("Warnings = %v, want exactly one %q", res.Warnings, reasonRestoreAttrs)
	}
	if w := res.Warnings[0].Error(); !strings.Contains(w, "rewritten") || !strings.Contains(w, "restore boom") {
		t.Errorf("warning = %q, want the rewrite notice and the restore failure", w)
	}
	if got := mustRead(t, dir, 1); !bytes.Equal(got, canonical) {
		t.Errorf("rewritten bytes lost after a failed restore")
	}
}

// withoutStatusLine drops the "- Status (Статус): open" meta line from a
// canonical body, leaving a body that states no status at all.
func withoutStatusLine(t *testing.T, body []byte) []byte {
	t.Helper()
	const line = "- Status (Статус): open\n"
	out := strings.Replace(string(body), line, "", 1)
	if out == string(body) {
		t.Fatalf("status line not found in body:\n%s", body)
	}
	return []byte(out)
}

// TestNormalize_NoStatusLineNoWarning covers the other half of the V19
// notice: a body that states NO status says nothing to contradict the file
// name, so the reasonStatusFromName notice must stay silent (it is reserved
// for a body that claims a different status). The file is still rewritten —
// Render emits the missing line from the name — the journal is not touched,
// and the run finishes without an error.
func TestNormalize_NoStatusLineNoWarning(t *testing.T) {
	s, dir := newStore(t)
	_, canonical := canonicalNormalize(t, s, dir)
	path := writeTicketFileBytes(t, dir, "T-0001-open.md", withoutStatusLine(t, canonical))

	// The fixture really is status-less for the parser; if Parse ever invents
	// a status, the "no warning" assertion below would no longer mean this.
	if tk, _, perr := domain.Parse(mustRead(t, dir, 1)); perr != nil || tk.Status != "" {
		t.Fatalf("Parse = (%+v, %v), want empty Status and no error", tk, perr)
	}

	res, err := s.Normalize(NormalizeOptions{})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	wantChanged(t, res.Changed, path)
	if len(res.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none (no body status to contradict)", res.Warnings)
	}
	got := mustRead(t, dir, 1)
	if !bytes.Equal(got, canonical) {
		t.Errorf("body not restored to the canonical form:\n--- got ---\n%s", got)
	}
	if n := strings.Count(string(got), " — тикет создан "); n != 1 {
		t.Errorf("journal creation entries = %d, want 1 (no line may be appended)", n)
	}
}
