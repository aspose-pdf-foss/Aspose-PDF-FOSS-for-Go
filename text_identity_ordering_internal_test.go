// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

// TestIdentityHAdobeOrderingExtracts: an Identity-H composite font whose
// descendant is an Adobe-Japan1 CIDFont, with no /ToUnicode, extracts through
// the ordering's CID→Unicode table (pdf-go-q2nq). CID 741 is U+2208 in the
// table's UTF-16 source; CID 15464, an alternate glyph for the same character,
// is mapped only by Adobe's UTF-32 columns.
func TestIdentityHAdobeOrderingExtracts(t *testing.T) {
	d := NewDocument(300, 100)
	cidFont := d.addObject(pdfDict{
		"/Type": pdfName("/Font"), "/Subtype": pdfName("/CIDFontType0"), "/BaseFont": pdfName("/KozMinPr6N-Regular"),
		"/CIDSystemInfo": pdfDict{"/Registry": "Adobe", "/Ordering": "Japan1", "/Supplement": 6},
		"/DW":            1000,
	})
	font := d.addObject(pdfDict{
		"/Type": pdfName("/Font"), "/Subtype": pdfName("/Type0"), "/BaseFont": pdfName("/KozMinPr6N-Regular"),
		"/Encoding": pdfName("/Identity-H"), "/DescendantFonts": pdfArray{pdfRef{Num: cidFont}},
	})
	p, _ := d.Page(1)
	p.pageDict()["/Resources"] = pdfDict{"/Font": pdfDict{"/F1": pdfRef{Num: font}}}
	// <02E5> = CID 741, <3C68> = CID 15464.
	if err := p.appendToContentStream([]byte("BT /F1 12 Tf 10 50 Td <02E53C68> Tj ET")); err != nil {
		t.Fatal(err)
	}
	got, err := p.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got) != "∈∈" {
		t.Errorf("ExtractText = %q, want %q", got, "∈∈")
	}
}
