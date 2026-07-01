// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

func TestShapeArabic(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []rune
	}{
		{
			// beh + yeh + teh → initial, medial, final ("bayt", house)
			name: "three dual-joining letters",
			in:   "بيت",
			want: []rune{0xFE91, 0xFEF4, 0xFE96},
		},
		{
			// lam + alef → isolated lam-alef ligature
			name: "lam-alef ligature isolated",
			in:   "لا",
			want: []rune{0xFEFB},
		},
		{
			// seen + lam + alef + meem ("salaam"): seen initial, lam-alef final
			// ligature, meem isolated (alef is right-joining so meem doesn't join).
			name: "salaam with medial lam-alef",
			in:   "سلام",
			want: []rune{0xFEB3, 0xFEFC, 0xFEE1},
		},
		{
			// a lone letter is isolated
			name: "single letter isolated",
			in:   "ب",
			want: []rune{0xFE8F},
		},
		{
			// dal is right-joining — it never connects to the following letter,
			// so the alef after it gets no preceding join and stays isolated too.
			name: "right-joining letter does not connect forward",
			in:   "دا", // dal + alef → both isolated
			want: []rune{0xFEA9, 0xFE8D},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shapeArabic(tc.in)
			if got != string(tc.want) {
				t.Errorf("shapeArabic(% X) = % X, want % X", []rune(tc.in), []rune(got), tc.want)
			}
		})
	}
}

func TestShapeArabicPassthrough(t *testing.T) {
	if got := shapeArabic("hello 123"); got != "hello 123" {
		t.Errorf("non-Arabic changed: %q", got)
	}
	// Latin between Arabic breaks joining: each Arabic letter is isolated.
	in := "ب x ب" // beh, space, x, space, beh
	got := []rune(shapeArabic(in))
	if got[0] != 0xFE8F || got[len(got)-1] != 0xFE8F {
		t.Errorf("Arabic separated by Latin should be isolated: % X", got)
	}
}

func TestShapeArabicMarksTransparent(t *testing.T) {
	// beh + fatha (mark) + yeh: the mark is transparent, so beh still joins yeh
	// (beh initial, yeh final), with the mark preserved between them.
	in := "بَي" // beh, fatha, yeh
	got := []rune(shapeArabic(in))
	if len(got) != 3 {
		t.Fatalf("expected 3 runes, got %d (% X)", len(got), got)
	}
	if got[0] != 0xFE91 { // beh initial (joins across the mark)
		t.Errorf("beh should be initial across a mark: %X", got[0])
	}
	if got[1] != 0x064E { // mark preserved
		t.Errorf("mark not preserved: %X", got[1])
	}
	if got[2] != 0xFEF2 { // yeh final
		t.Errorf("yeh should be final: %X", got[2])
	}
}
