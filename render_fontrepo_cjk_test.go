// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestSysFamilyPrefix checks the word-boundary family match used to resolve a
// non-embedded CJK font to an installed face whose family name is a longer
// variant of the candidate. On modern Windows the only common-glyph Traditional
// Chinese face is "Microsoft JhengHei UI" (msjh.ttc); the bundled candidate is
// "microsoft jhenghei". The "-ExtB" faces (mingliub.ttc) must be rejected — they
// hold only rare CJK-Extension-B ideographs, so matching one renders blank.
func TestSysFamilyPrefix(t *testing.T) {
	r := &fontRepository{sysByFamily: map[string]fontRef{
		"microsoft jhenghei ui|regular": {path: "msjh.ttc", index: 1},
		"mingliu-extb|regular":          {path: "mingliub.ttc", index: 0},
		"pmingliu-extb|regular":         {path: "mingliub.ttc", index: 1},
		"simsun|regular":                {path: "simsun.ttc", index: 0},
	}}

	cases := []struct {
		fam, style string
		wantPath   string // "" = expect no match
	}{
		{"microsoft jhenghei", "regular", "msjh.ttc"}, // longer "… UI" variant
		{"mingliu", "regular", ""},                    // only -ExtB present → reject
		{"pmingliu", "regular", ""},                   // only -ExtB present → reject
		{"simsun", "regular", ""},                     // exact name, no " " prefix → not a prefix match
		{"microsoft jhenghei", "bold", ""},            // style mismatch
	}
	for _, c := range cases {
		ref, ok := r.sysFamilyPrefix(c.fam, c.style)
		if c.wantPath == "" {
			if ok {
				t.Errorf("sysFamilyPrefix(%q,%q) matched %q, want no match", c.fam, c.style, ref.path)
			}
			continue
		}
		if !ok || ref.path != c.wantPath {
			t.Errorf("sysFamilyPrefix(%q,%q) = (%q,%v), want %q", c.fam, c.style, ref.path, ok, c.wantPath)
		}
	}
}
