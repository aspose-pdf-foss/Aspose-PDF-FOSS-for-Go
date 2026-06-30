// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// Hebrew letters alef/bet/gimel for readable test strings.
const (
	heAlef  = "א"
	heBet   = "ב"
	heGimel = "ג"
)

func TestBidiVisualString(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		baseLevel int
		want      string
	}{
		{
			name:      "pure Hebrew reverses",
			in:        heAlef + heBet + heGimel,
			baseLevel: 1,
			want:      heGimel + heBet + heAlef,
		},
		{
			name:      "Latin base keeps Latin, reverses embedded Hebrew",
			in:        "abc " + heAlef + heBet + heGimel,
			baseLevel: 0,
			want:      "abc " + heGimel + heBet + heAlef,
		},
		{
			name:      "RTL base keeps numbers LTR to the left of Hebrew",
			in:        heAlef + heBet + heGimel + " 123",
			baseLevel: 1,
			want:      "123 " + heGimel + heBet + heAlef,
		},
		{
			name:      "pure Latin is unchanged",
			in:        "Hello 123",
			baseLevel: 0,
			want:      "Hello 123",
		},
		{
			name:      "parentheses mirror around Hebrew",
			in:        "(" + heAlef + ")",
			baseLevel: 1,
			want:      "(" + heAlef + ")",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := bidiVisualString(tc.in, tc.baseLevel); got != tc.want {
				t.Errorf("bidiVisualString(%q, %d) = %q, want %q", tc.in, tc.baseLevel, got, tc.want)
			}
		})
	}
}

func TestBidiBaseLevel(t *testing.T) {
	cases := []struct {
		in       string
		explicit bool
		want     int
	}{
		{"hello", false, 0},
		{heAlef + "bc", false, 1}, // first strong is RTL
		{"ab" + heAlef, false, 0}, // first strong is LTR
		{"123 " + heAlef, false, 1},
		{"hello", true, 1}, // explicit RTL overrides
		{"", false, 0},
	}
	for _, tc := range cases {
		if got := bidiBaseLevel(tc.in, tc.explicit); got != tc.want {
			t.Errorf("bidiBaseLevel(%q, %v) = %d, want %d", tc.in, tc.explicit, got, tc.want)
		}
	}
}

func TestBidiHasStrongRTL(t *testing.T) {
	if bidiHasStrongRTL("plain latin 123") {
		t.Error("plain Latin should not be flagged RTL")
	}
	if !bidiHasStrongRTL("hello " + heAlef) {
		t.Error("text with Hebrew should be flagged RTL")
	}
	if !bidiHasStrongRTL("السلام") { // Arabic
		t.Error("Arabic should be flagged RTL")
	}
}

func TestBidiClassSpot(t *testing.T) {
	cases := []struct {
		r    rune
		want bidiCls
	}{
		{'A', clsL},
		{'5', clsEN},
		{0x05D0, clsR},  // Hebrew alef
		{0x0627, clsAL}, // Arabic alef
		{0x0661, clsAN}, // Arabic-Indic digit one
		{' ', clsWS},
		{'(', clsON},
		{',', clsCS},
		{'+', clsES},
		{'%', clsET},
		{0x064B, clsNSM}, // Arabic fathatan
	}
	for _, tc := range cases {
		if got := bidiClass(tc.r); got != tc.want {
			t.Errorf("bidiClass(%U) = %d, want %d", tc.r, got, tc.want)
		}
	}
}
