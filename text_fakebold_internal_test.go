// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestFakeBoldIsBold: text filled and stroked (Tr 2) over a regular face, the
// way producers fake bold, reads as Bold; with no stroke width it does not,
// and q/Q restores the mode (pdf-go-nhcz).
func TestFakeBoldIsBold(t *testing.T) {
	d := NewDocument(400, 200)
	font := d.addObject(pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/Type1"), "/BaseFont": pdfName("/Helvetica")})
	p, _ := d.Page(1)
	p.pageDict()["/Resources"] = pdfDict{"/Font": pdfDict{"/F1": pdfRef{Num: font}}}
	content := "BT /F1 12 Tf 10 150 Td (Regular) Tj ET " +
		"q 0.36 w 2 Tr BT /F1 12 Tf 10 120 Td (Faked) Tj ET Q " +
		"BT /F1 12 Tf 10 90 Td (After) Tj ET " +
		"q 0 w 2 Tr BT /F1 12 Tf 10 60 Td (Hairline) Tj ET Q " +
		"q 6 Tr BT /F1 12 Tf 10 30 Td (Clipped) Tj ET Q"
	if err := p.appendToContentStream([]byte(content)); err != nil {
		t.Fatal(err)
	}
	lines, err := p.ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"Regular": false, "Faked": true, "After": false, "Hairline": false, "Clipped": true}
	seen := 0
	for _, l := range lines {
		for _, f := range l.Fragments {
			if b, ok := want[f.Text]; ok {
				seen++
				if f.Bold != b {
					t.Errorf("%q: Bold = %v, want %v", f.Text, f.Bold, b)
				}
			}
		}
	}
	if seen != len(want) {
		t.Errorf("saw %d of %d words", seen, len(want))
	}
}
