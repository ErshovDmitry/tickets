package domain

import "testing"

func TestParseStatus(t *testing.T) {
	for _, s := range []string{"open", "wip", "done", "closed"} {
		if got, err := ParseStatus(s); err != nil || string(got) != s {
			t.Errorf("ParseStatus(%q) = %q, %v; want %q, nil", s, got, err, s)
		}
	}
}

func TestParseStatusErrors(t *testing.T) {
	for _, s := range []string{"in-progress", "OPEN", "", "opens"} {
		if _, err := ParseStatus(s); err == nil {
			t.Errorf("ParseStatus(%q) = nil error, want error", s)
		}
	}
}

func TestParseType(t *testing.T) {
	for _, s := range []string{"BUG", "OPS", "TD", "ENH"} {
		if got, err := ParseType(s); err != nil || string(got) != s {
			t.Errorf("ParseType(%q) = %q, %v; want %q, nil", s, got, err, s)
		}
	}
	for _, s := range []string{"bug", "", "FEAT"} {
		if _, err := ParseType(s); err == nil {
			t.Errorf("ParseType(%q) = nil error, want error", s)
		}
	}
}

func TestParsePriority(t *testing.T) {
	for _, s := range []string{"low", "normal", "high"} {
		if got, err := ParsePriority(s); err != nil || string(got) != s {
			t.Errorf("ParsePriority(%q) = %q, %v; want %q, nil", s, got, err, s)
		}
	}
	for _, s := range []string{"urgent", "", "High"} {
		if _, err := ParsePriority(s); err == nil {
			t.Errorf("ParsePriority(%q) = nil error, want error", s)
		}
	}
}
