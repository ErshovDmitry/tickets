package domain

import "testing"

func TestFilename(t *testing.T) {
	if got, want := Filename(1, StatusOpen), "T-0001-open.md"; got != want {
		t.Errorf("Filename(1, open) = %q, want %q", got, want)
	}
	if got, want := Filename(42, StatusWip), "T-0042-wip.md"; got != want {
		t.Errorf("Filename(42, wip) = %q, want %q", got, want)
	}
	if got, want := Filename(9999, StatusClosed), "T-9999-closed.md"; got != want {
		t.Errorf("Filename(9999, closed) = %q, want %q", got, want)
	}
}

func TestParseFilenameOK(t *testing.T) {
	n, st, err := ParseFilename("T-0001-open.md")
	if err != nil {
		t.Fatalf("ParseFilename(T-0001-open.md) error: %v", err)
	}
	if n != 1 || st != StatusOpen {
		t.Errorf("ParseFilename(T-0001-open.md) = %d, %q; want 1, open", n, st)
	}
	n, st, err = ParseFilename("T-0123-done.md")
	if err != nil || n != 123 || st != StatusDone {
		t.Errorf("ParseFilename(T-0123-done.md) = %d, %q, %v; want 123, done, nil", n, st, err)
	}
}

func TestParseFilenameRejects(t *testing.T) {
	for _, name := range []string{
		"T-00001-open.md", // five digits
		"T-001-open.md",   // three digits
		"T-0001-open",     // no extension
		"T-0001-open.txt", // wrong extension
		"t-0001-open.md",  // wrong case
		"T-0001-Open.md",  // status case
		"X-0001-open.md",  // wrong prefix
		"T-0001--open.md", // empty status
		"",                // empty
	} {
		if _, _, err := ParseFilename(name); err == nil {
			t.Errorf("ParseFilename(%q) = nil error, want error", name)
		}
	}
}
