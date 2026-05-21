// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

func TestToRoman(t *testing.T) {
	cases := []struct {
		n     int
		lower string
		upper string
	}{
		{1, "i", "I"},
		{4, "iv", "IV"},
		{9, "ix", "IX"},
		{14, "xiv", "XIV"},
		{40, "xl", "XL"},
		{90, "xc", "XC"},
		{399, "cccxcix", "CCCXCIX"},
		{2024, "mmxxiv", "MMXXIV"},
	}
	for _, c := range cases {
		if got := toRoman(c.n, false); got != c.lower {
			t.Errorf("toRoman(%d, false) = %q, want %q", c.n, got, c.lower)
		}
		if got := toRoman(c.n, true); got != c.upper {
			t.Errorf("toRoman(%d, true) = %q, want %q", c.n, got, c.upper)
		}
	}
}

func TestToAlpha(t *testing.T) {
	cases := []struct {
		n     int
		lower string
		upper string
	}{
		{1, "a", "A"},
		{26, "z", "Z"},
		{27, "aa", "AA"},
		{28, "ab", "AB"},
		{52, "az", "AZ"},
		{53, "ba", "BA"},
	}
	for _, c := range cases {
		if got := toAlpha(c.n, false); got != c.lower {
			t.Errorf("toAlpha(%d, false) = %q, want %q", c.n, got, c.lower)
		}
		if got := toAlpha(c.n, true); got != c.upper {
			t.Errorf("toAlpha(%d, true) = %q, want %q", c.n, got, c.upper)
		}
	}
}
