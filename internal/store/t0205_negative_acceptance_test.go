package store_test

import "testing"

// TestT0205IntentionalFailure is a deliberately failing test used only to
// produce a genuinely red CI run for the ci-retry negative acceptance
// (T-0205, evidence atom B). It must never be merged.
func TestT0205IntentionalFailure(t *testing.T) {
	t.Fatal("T-0205 negative acceptance: intentional failure (do not merge)")
}
