package domain

import "testing"

// TestContainsCTL pins the contract of ContainsCTL: it reports whether s
// holds a rune r with (r <= 0x1F && r != 0x09) || r == 0x7F, i.e. a C0
// control character except TAB, or DEL. Every case below states which
// clause or boundary it exercises.
func TestContainsCTL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		// Contract: no runes at all, hence no control rune, hence false.
		{name: "empty string", in: "", want: false},

		// TAB (0x09) is the only C0 rune the contract explicitly excludes.
		{name: "tab between letters", in: "a\tb", want: false},
		{name: "only TAB", in: "\t", want: false},

		// 0x20 is the first printable rune, just above the C0 range.
		{name: "space 0x20", in: " ", want: false},

		// 0x7E '~' is the last printable ASCII rune, just below DEL.
		{name: "tilde 0x7E", in: "~", want: false},

		// 0x7F DEL is named by the contract even though it is not C0.
		{name: "DEL 0x7F only", in: "\x7f", want: true},

		// Position independence: the control rune is found first and last,
		// so the scan does not stop at the wrong edge.
		{name: "leading NUL", in: "\x00abc", want: true},
		{name: "trailing 0x1F", in: "abc\x1f", want: true},
		{name: "trailing DEL", in: "abc\x7f", want: true},

		// CR/LF lie inside the C0 range and are covered (T-0075).
		{name: "embedded LF", in: "a\nb", want: true},

		// Multi-byte UTF-8 and invalid bytes decode to runes above 0x7F,
		// so they are not control runes.
		{name: "cyrillic", in: "Привет", want: false},
		{name: "cyrillic with emoji", in: "тест 🚀 тикет", want: false},
		// The scan must keep going after multi-byte runes (guards an
		// early-return-on-non-ASCII mutation).
		{name: "CTL after cyrillic", in: "тест\x1f", want: true},

		// "\xff" is an invalid byte: the range loop decodes it to U+FFFD,
		// which is neither C0 nor DEL, so the rune-based check is false.
		{name: "invalid UTF-8 byte", in: "\xff", want: false},

		// Realistic ticket title: colon and spaces are printable.
		{name: "realistic title", in: "TD: разобраться с логом", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ContainsCTL(tc.in); got != tc.want {
				t.Errorf("ContainsCTL(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestContainsCTL_C0Exhaustive characterizes the current behaviour over the
// whole C0 range and its boundaries. If the contract is ever widened (for
// example to C1 0x80-0x9F), update these assertions instead of treating the
// failure as a regression.
func TestContainsCTL_C0Exhaustive(t *testing.T) {
	// Every C0 rune is rejected except TAB (0x09).
	for r := rune(0x00); r <= 0x1F; r++ {
		want := r != 0x09
		if got := ContainsCTL(string(r)); got != want {
			t.Errorf("ContainsCTL(%q) = %v, want %v", string(r), got, want)
		}
	}

	// Boundaries around DEL: DEL is rejected, C1 controls (0x80-0x9F) and
	// the first non-control rune after them (0xA0) are not. These are real
	// runes produced by string(rune), not UTF-8 continuation bytes.
	boundaries := []struct {
		name string
		r    rune
		want bool
	}{
		// Upper boundary of the contract: DEL 0x7F is rejected.
		{name: "DEL 0x7F", r: 0x7F, want: true},
		// First C1 control: outside the declared contract.
		{name: "C1 0x80", r: 0x80, want: false},
		// Last C1 control: still outside the declared contract.
		{name: "C1 0x9F", r: 0x9F, want: false},
		// First rune past the C1 block: NBSP (category Zs); it is false
		// only because 0xA0 is neither C0 nor DEL.
		{name: "NBSP 0xA0", r: 0xA0, want: false},
	}

	for _, tc := range boundaries {
		t.Run(tc.name, func(t *testing.T) {
			in := string(tc.r)
			if got := ContainsCTL(in); got != tc.want {
				t.Errorf("ContainsCTL(%q) = %v, want %v", in, got, tc.want)
			}
		})
	}
}
