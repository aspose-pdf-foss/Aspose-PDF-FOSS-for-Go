// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// pdf-go-amjo: sysFamilyPrefix returned the first match while ranging over
// a map, so when several installed families shared the requested prefix
// ("microsoft jhenghei" → "… ui", "… light") the face picked — and the
// page's rendering — changed from run to run. The closest family (the
// shortest name, then alphabetical) must win, every time.
func TestSysFamilyPrefixIsDeterministic(t *testing.T) {
	r := &fontRepository{sysByFamily: map[string]fontRef{}}
	for i, fam := range []string{
		"microsoft jhenghei ui light", "microsoft jhenghei light", "microsoft jhenghei ui",
		"microsoft jhenghei ext-b", "microsoft jhengheiold", "other family",
	} {
		r.sysByFamily[fam+"|regular"] = fontRef{path: fam, index: i}
	}
	for i := 0; i < 200; i++ {
		ref, ok := r.sysFamilyPrefix("microsoft jhenghei", "regular")
		if !ok || ref.path != "microsoft jhenghei ui" {
			t.Fatalf("run %d: picked %q (ok=%v), want the closest family %q", i, ref.path, ok, "microsoft jhenghei ui")
		}
	}
}
