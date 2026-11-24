package cli

// T-0241 A5: package-internal unit test for the JSON warning rendering. Every
// warning PRODUCER sets an error (scan.go, archivedir.go, deleteddir.go,
// store.go), so the error-less shape is unreachable end-to-end and can only be
// pinned here.

import (
	"testing"

	"ticket/internal/store"
)

// TestShowJSONWarningWithoutError pins the ParseWarning.Error() semantics the
// DTO relies on: a warning carrying no error renders as the bare name, and the
// service reason code never enters the text.
func TestShowJSONWarningWithoutError(t *testing.T) {
	got := showJSONWarnings([]store.ParseWarning{{Name: "T-0001-open.md", Reason: "probe"}})
	if len(got) != 1 || got[0] != "T-0001-open.md" {
		t.Fatalf("showJSONWarnings = %v, want [\"T-0001-open.md\"]", got)
	}
	if got := showJSONWarnings(nil); len(got) != 0 {
		t.Errorf("showJSONWarnings(nil) = %v, want an empty slice", got)
	}
}
