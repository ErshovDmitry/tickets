package domain

// T-0245 C2 v2: the pure mutation methods update raw target + normalized
// public field + snapshot atomically, and Render falls back to the current
// semantic value whenever the snapshot no longer matches (unknown edit).

import (
	"bytes"
	"strings"
	"testing"
)

// TestAppendCommentPayloadPreservesOldRaw pins old raw + one LF blank
// separator + new raw payload, with the public field normalized.
func TestAppendCommentPayloadPreservesOldRaw(t *testing.T) {
	tk, unknown, err := Parse([]byte(rawBodySrc()))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tk.AppendCommentPayload("новый\r\nблок")
	out, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := "коммент один\r\nкоммент два\n\nновый\r\nблок"
	if !bytes.Contains(out, []byte(want)) {
		t.Fatalf("old CRLF block or LF separator lost:\n got %q\nwant substring %q", out, want)
	}
	if tk.Comments != "коммент один\nкоммент два\n\nновый\nблок" {
		t.Fatalf("public Comments not normalized: %q", tk.Comments)
	}
	if strings.Contains(tk.Comments, "\r") {
		t.Fatalf("public Comments leaked CR: %q", tk.Comments)
	}
}

// TestSetEditablePayloadReplacesOnlyTarget pins that replacing one section
// leaves the other sections' raw CRLF bytes untouched.
func TestSetEditablePayloadReplacesOnlyTarget(t *testing.T) {
	tk, unknown, err := Parse([]byte(rawBodySrc()))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := tk.SetEditablePayload(SectionDetails, "замена\r\nдеталей"); err != nil {
		t.Fatalf("SetEditablePayload: %v", err)
	}
	out, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, keep := range []string{"итог один\r\nитог два", "замечание один", "коммент один\r\nкоммент два", "замена\r\nдеталей"} {
		if !bytes.Contains(out, []byte(keep)) {
			t.Fatalf("lost %q:\n%q", keep, out)
		}
	}
	if bytes.Contains(out, []byte("деталь один\r")) {
		t.Fatalf("old Details raw survived replace:\n%q", out)
	}
	if tk.Details != "замена\nдеталей" {
		t.Fatalf("public Details not normalized: %q", tk.Details)
	}
}

// TestSetEditablePayloadEmptyClears pins that an empty payload clears the
// public field and the sidecar so Render emits the dict default.
func TestSetEditablePayloadEmptyClears(t *testing.T) {
	tk, unknown, err := Parse([]byte(rawBodySrc()))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := tk.SetEditablePayload(SectionDetails, ""); err != nil {
		t.Fatalf("SetEditablePayload: %v", err)
	}
	if tk.Details != "" {
		t.Fatalf("Details not cleared: %q", tk.Details)
	}
	if _, ok := tk.RawTemplates.Bodies[secNameDetails]; ok {
		t.Fatalf("sidecar entry survived empty payload")
	}
	out, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Contains(out, []byte(dictRU.detailsStub)) {
		t.Fatalf("empty Details did not render stub:\n%q", out)
	}
}

// TestSetEditablePayloadNonEditableErrors pins the default guard against a
// silent no-op for a section outside the editable mapping.
func TestSetEditablePayloadNonEditableErrors(t *testing.T) {
	tk := newTestTicket()
	if err := tk.SetEditablePayload(SectionJournal, "x"); err == nil {
		t.Fatalf("want error for non-editable section")
	}
}

// TestRawBodySnapshotMismatchFallback pins that a direct public-field edit
// (snapshot no longer equal) makes Render emit the current semantic value,
// never the stale raw bytes.
func TestRawBodySnapshotMismatchFallback(t *testing.T) {
	tk, unknown, err := Parse([]byte(rawBodySrc()))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tk.Details = "ручная правка"
	out, err := Render(tk, unknown)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Contains(out, []byte("ручная правка")) {
		t.Fatalf("current semantic value not rendered:\n%q", out)
	}
	if bytes.Contains(out, []byte("деталь один\r")) {
		t.Fatalf("stale raw CRLF resurrected after mismatch:\n%q", out)
	}
}
