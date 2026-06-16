// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

// TestExtractRestoresFontOnQ guards the q/Q graphics-state restore in text
// extraction. Binder1.pdf draws its bold "TOTAL …" labels with the page font,
// but interleaves number cells inside "q … /TT5 Tf … Q" blocks. Without
// restoring the font on Q the extractor kept the inner subset font (no
// ToUnicode, mismatched encoding) and decoded the labels as "3?3>;" etc.
func TestExtractRestoresFontOnQ(t *testing.T) {
	doc, err := Open("testdata/Binder1.pdf")
	if err != nil {
		t.Fatal(err)
	}
	txt, err := doc.Pages()[0].ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"TOTAL IMPONIBLE", "TOTAL HABERES", "DESCUENTOS", "TOTAL LIQUIDO A PAGAR"} {
		if !strings.Contains(txt, want) {
			t.Errorf("extracted text missing %q (font not restored on Q?)", want)
		}
	}
	if strings.Contains(txt, "3?3>;") {
		t.Error("extracted text contains the mis-decoded garble \"3?3>;\" — wrong font used")
	}
}
