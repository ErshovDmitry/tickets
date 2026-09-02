package domain

import "testing"

func TestValidateStrictOK(t *testing.T) {
	tk := Ticket{
		Number:   1,
		Status:   StatusOpen,
		Type:     TypeBUG,
		Priority: PriorityNormal,
		Title:    "Ломается",
	}
	if err := tk.Validate(Strict); err != nil {
		t.Fatalf("Validate(Strict) = %v, want nil", err)
	}
}

func TestValidateStrictDetailsAndWhoMayBeEmpty(t *testing.T) {
	tk := Ticket{Number: 7, Status: StatusWip, Type: TypeTD, Priority: PriorityLow, Title: "x"}
	if err := tk.Validate(Strict); err != nil {
		t.Fatalf("empty Details/Who rejected: %v", err)
	}
}

func TestValidateStrictErrors(t *testing.T) {
	base := Ticket{Number: 1, Status: StatusOpen, Type: TypeBUG, Priority: PriorityNormal, Title: "t"}
	cases := []struct {
		name string
		mut  func(*Ticket)
	}{
		{"zero number", func(t *Ticket) { t.Number = 0 }},
		{"negative number", func(t *Ticket) { t.Number = -1 }},
		{"bad status", func(t *Ticket) { t.Status = "in-progress" }},
		{"empty status", func(t *Ticket) { t.Status = "" }},
		{"bad type", func(t *Ticket) { t.Type = "bug" }},
		{"bad priority", func(t *Ticket) { t.Priority = "urgent" }},
		{"empty title", func(t *Ticket) { t.Title = "" }},
		{"blank title", func(t *Ticket) { t.Title = "   " }},
	}
	for _, tc := range cases {
		tk := base
		tc.mut(&tk)
		if err := tk.Validate(Strict); err == nil {
			t.Errorf("%s: Validate(Strict) = nil, want error", tc.name)
		}
	}
}

func TestValidateTolerantNeverErrors(t *testing.T) {
	var zero Ticket
	if err := zero.Validate(Tolerant); err != nil {
		t.Fatalf("Validate(Tolerant) on zero Ticket = %v, want nil", err)
	}
	empty := Ticket{Number: 0, Status: "", Type: "", Priority: "", Title: ""}
	if err := empty.Validate(Tolerant); err != nil {
		t.Fatalf("Validate(Tolerant) on empty fields = %v, want nil", err)
	}
}
