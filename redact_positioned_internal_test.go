// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

// positionedFontPage draws twenty glyphs, 10pt apart, in the font fontDict
// from x 100 on the baseline y 500, and marks the left half for redaction.
func positionedFontPage(t *testing.T, fontDict pdfDict, show string) *Document {
	t.Helper()
	d := NewDocument(400, 600)
	font := d.addObject(fontDict)
	p, _ := d.Page(1)
	p.pageDict()["/Resources"] = pdfDict{"/Font": pdfDict{"/F1": pdfRef{Num: font}}}
	if err := p.appendToContentStream([]byte("BT /F1 10 Tf 100 500 Td " + show + " Tj ET")); err != nil {
		t.Fatal(err)
	}
	p.Annotations().Add(NewRedactAnnotation(p, Rectangle{LLX: 95, LLY: 495, URX: 200, URY: 515}))
	return d
}

// TestRedactUnmappedFontByPosition: text in a font whose characters are
// unknown (an Identity-H font with no /ToUnicode) but whose widths /W states
// is removed by position, half of it here; a simple font with no widths at
// all is still refused (pdf-go-6jxp).
func TestRedactUnmappedFontByPosition(t *testing.T) {
	type0 := pdfDict{
		"/Type": pdfName("/Font"), "/Subtype": pdfName("/Type0"), "/BaseFont": pdfName("/SymbolFace"),
		"/Encoding": pdfName("/Identity-H"),
		"/DescendantFonts": pdfArray{pdfDict{
			"/Type": pdfName("/Font"), "/Subtype": pdfName("/CIDFontType2"), "/BaseFont": pdfName("/SymbolFace"),
			"/CIDSystemInfo": pdfDict{"/Registry": "Adobe", "/Ordering": "Identity", "/Supplement": 0},
			"/DW": 1000,
		}},
	}
	d := positionedFontPage(t, type0, "<"+strings.Repeat("0041", 20)+">")
	p, _ := d.Page(1)
	before, _ := p.ExtractText()
	if n := strings.Count(before, "�"); n != 20 {
		t.Fatalf("before: %d unmapped glyphs, want 20 (%q)", n, before)
	}
	if err := d.ApplyRedactions(); err != nil {
		t.Fatalf("refused: %v", err)
	}
	after, _ := p.ExtractText()
	// Glyphs at 100, 110, ... 190 have their centres inside [95, 200].
	if n := strings.Count(after, "�"); n != 10 {
		t.Errorf("after: %d glyphs left, want 10 (%q)", n, after)
	}

	simple := pdfDict{"/Type": pdfName("/Font"), "/Subtype": pdfName("/TrueType"), "/BaseFont": pdfName("/NoWidthsFace")}
	d = positionedFontPage(t, simple, "(abc)")
	if err := d.ApplyRedactions(); err == nil {
		t.Error("a font with no widths and no known encoding was redacted by a guess")
	}
}
