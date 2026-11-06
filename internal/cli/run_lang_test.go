package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"ticket/internal/domain"
)

// TestLangFromDefaultValidatesRU covers the whole-diff fix: the no-locale
// default may be RU only when the RU dictionary actually registered. In a
// degraded initialisation (ru.json broken, EN usable) the default becomes EN,
// so the resolved language matches the dictionary domain.getDict would use
// instead of silently substituting EN behind a LangRU label.
func TestLangFromDefaultValidatesRU(t *testing.T) {
	prevKnown := isKnownLangFn
	t.Cleanup(func() { isKnownLangFn = prevKnown })

	withRunSeams(t,
		func() error { return fmt.Errorf("%w: ru.json broken", domain.ErrInitDegraded) },
		langFrom,
	)

	// Degraded registry: EN registered, RU not.
	isKnownLangFn = func(l domain.Lang) bool { return l == domain.LangEN }

	// No locale variables at all (nil and empty map) and all-empty strings
	// both take the default branch.
	for _, env := range []map[string]string{
		nil,
		{},
		{"TICKET_LANG": "", "LC_ALL": "", "LANG": ""},
	} {
		if got := langFrom(env); got != domain.LangEN {
			t.Fatalf("langFrom(%v) = %q, want EN with an unregistered RU", env, got)
		}
	}

	// End to end: `ticket help` with no locale env must render the EN
	// template while still surfacing the one degraded-init warning.
	var stdout, stderr bytes.Buffer
	code := Run([]string{"help"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(help) = %d, want 0; stderr: %q", code, stderr.String())
	}
	if got, want := stdout.String(), versionLine()+usageTextEN; got != want {
		t.Fatalf("stdout = %d bytes, want version line + EN usage (%d bytes)", len(got), len(want))
	}
	if !strings.Contains(stderr.String(), domain.ErrInitDegraded.Error()) {
		t.Fatalf("stderr = %q, want the degraded-init warning", stderr.String())
	}

	// A healthy registry still defaults to RU.
	withRunSeams(t, func() error { return nil }, langFrom)
	isKnownLangFn = func(l domain.Lang) bool { return l == domain.LangRU || l == domain.LangEN }

	if got := langFrom(nil); got != domain.LangRU {
		t.Fatalf("langFrom(nil) = %q, want RU when RU is registered", got)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"help"}, map[string]string{}, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(help) = %d, want 0; stderr: %q", code, stderr.String())
	}
	if got, want := stdout.String(), versionLine()+usageTextRU; got != want {
		t.Fatalf("stdout = %d bytes, want version line + RU usage (%d bytes)", len(got), len(want))
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty on a healthy init", stderr.String())
	}
}
