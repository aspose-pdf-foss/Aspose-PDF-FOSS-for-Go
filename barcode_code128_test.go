// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// decodeCode128Modules is an independent-of-encoding-order self-decoder used
// only by tests: it reconstructs the bar/space width sequence from the
// module bits (run-length, after stripping the quiet zone), chunks it into
// symbols (six widths each, except the final seven-wide STOP), looks each
// chunk up in code128Patterns, and reassembles the original text plus a
// freshly-recomputed checksum. This exercises the encoder's bit-packing
// geometry (quiet zone, bar/space alternation, symbol concatenation)
// end-to-end; it does not validate code128Patterns itself against an
// external reference (see the doc comment on that table).
func decodeCode128Modules(t *testing.T, mat barcodeModules) (string, error) {
	t.Helper()
	if mat.Rows != 1 {
		t.Fatalf("Code128 matrix has %d rows, want 1", mat.Rows)
	}

	// Strip leading/trailing quiet zone (runs of light modules) and
	// run-length-encode the remainder into widths.
	bits := mat.Bits
	start := 0
	for start < len(bits) && !bits[start] {
		start++
	}
	end := len(bits)
	for end > start && !bits[end-1] {
		end--
	}
	bits = bits[start:end]

	var widths []int
	for i := 0; i < len(bits); {
		j := i
		for j < len(bits) && bits[j] == bits[i] {
			j++
		}
		widths = append(widths, j-i)
		i = j
	}

	if len(widths) < 6+7 || (len(widths)-7)%6 != 0 {
		t.Fatalf("Code128 width count %d is not 6*n+7", len(widths))
	}
	numSymbols := (len(widths)-7)/6 + 1

	lookup := func(digits []int) int {
		want := make([]byte, len(digits))
		for i, d := range digits {
			want[i] = byte('0' + d)
		}
		for v, pattern := range code128Patterns {
			if pattern == string(want) {
				return v
			}
		}
		t.Fatalf("no Code128 symbol matches widths %v", digits)
		return -1
	}

	pos := 0
	values := make([]int, numSymbols)
	for s := 0; s < numSymbols; s++ {
		n := 6
		if s == numSymbols-1 {
			n = 7
		}
		values[s] = lookup(widths[pos : pos+n])
		pos += n
	}

	if values[0] != code128StartB {
		t.Fatalf("first symbol value = %d, want START B (%d)", values[0], code128StartB)
	}
	if values[numSymbols-1] != code128Stop {
		t.Fatalf("last symbol value = %d, want STOP (%d)", values[numSymbols-1], code128Stop)
	}

	data := values[1 : numSymbols-2]
	gotChecksum := values[numSymbols-2]
	wantChecksum := values[0]
	for i, v := range data {
		wantChecksum += v * (i + 1)
	}
	wantChecksum %= 103
	if gotChecksum != wantChecksum {
		return "", errChecksumMismatch
	}

	out := make([]rune, len(data))
	for i, v := range data {
		out[i] = rune(v + 0x20)
	}
	return string(out), nil
}

var errChecksumMismatch = &checksumError{}

type checksumError struct{}

func (*checksumError) Error() string { return "Code128 checksum mismatch" }

func TestCode128RoundTrip(t *testing.T) {
	cases := []string{
		"HELLO",
		"Hello, World! 123",
		" ",
		"~",
		"0123456789",
		"ABC-def_XYZ 456",
		"!\"#$%&'()*+,-./",
	}
	for _, s := range cases {
		mat, err := encodeCode128Modules(s)
		if err != nil {
			t.Fatalf("encodeCode128Modules(%q): %v", s, err)
		}
		got, err := decodeCode128Modules(t, mat)
		if err != nil {
			t.Fatalf("decode(%q): %v", s, err)
		}
		if got != s {
			t.Errorf("round trip: got %q, want %q", got, s)
		}
	}
}

func TestCode128RejectsNonASCII(t *testing.T) {
	if _, err := encodeCode128Modules("héllo"); err == nil {
		t.Error("expected an error for non-ASCII input")
	}
	if _, err := encodeCode128Modules("hi\tthere"); err == nil {
		t.Error("expected an error for a control character")
	}
}

func TestCode128RejectsEmpty(t *testing.T) {
	if _, err := encodeCode128Modules(""); err == nil {
		t.Error("expected an error for empty input")
	}
}

func TestCode128PatternTableStructure(t *testing.T) {
	for v, pattern := range code128Patterns {
		want := 11
		if v == code128Stop {
			want = 13
		}
		sum := 0
		for _, c := range pattern {
			sum += int(c - '0')
		}
		if sum != want {
			t.Errorf("value %d: pattern %q sums to %d, want %d", v, pattern, sum, want)
		}
		wantLen := 6
		if v == code128Stop {
			wantLen = 7
		}
		if len(pattern) != wantLen {
			t.Errorf("value %d: pattern %q has %d digits, want %d", v, pattern, len(pattern), wantLen)
		}
	}
}
