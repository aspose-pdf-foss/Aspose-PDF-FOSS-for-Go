// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

// Rewriting a content stream (redaction, ReplaceText, grayscale,
// auto-tagging) re-serializes every operand. Numbers used to be rounded to
// six decimals, so a small-scale matrix — 0.001968504 drawing a page in
// 1/508-point units — moved everything after it by up to a point, and the
// rewritten page no longer rendered like the original. Numbers must
// survive a parse/serialize round trip unchanged.
func TestSerializeContentOpsKeepsNumberPrecision(t *testing.T) {
	src := "0.001968504 0 0 0.001968504 0 0 cm 0 0.3750173 842 473.625 re f 0.48780487804878 0 0 0.651465798045603 100 100 cm"
	ops, err := parseContentStream([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	got := string(serializeContentOps(ops))
	for _, n := range []string{"0.001968504", "0.3750173", "0.48780487804878", "0.651465798045603"} {
		if !strings.Contains(got, n) {
			t.Errorf("%s lost precision: %q", n, got)
		}
	}
	re, err := parseContentStream([]byte(got))
	if err != nil {
		t.Fatal(err)
	}
	for i := range ops {
		for j := range ops[i].Operands {
			if ops[i].Operands[j] != re[i].Operands[j] {
				t.Errorf("op %d operand %d: %v became %v", i, j, ops[i].Operands[j], re[i].Operands[j])
			}
		}
	}
}
