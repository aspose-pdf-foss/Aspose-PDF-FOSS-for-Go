// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

func pdfaHasRule(r *PDFAValidationReport, rule string) bool {
	for _, is := range r.Issues {
		if is.Rule == rule {
			return true
		}
	}
	return false
}

// PDF/A asks for the fonts used to render text to be embedded. A face that
// merely sits in a resource dictionary — every Acrobat form leaves
// ZapfDingbats in /AcroForm/DR — renders nothing, and reporting it would
// raise a violation no conforming validator raises.
func TestPDFAUnusedFontIsNotAViolation(t *testing.T) {
	doc := NewDocument(300, 200)
	page, _ := doc.Page(1)
	fontID := doc.addObject(pdfDict{
		"/Type":     pdfName("/Font"),
		"/Subtype":  pdfName("/Type1"),
		"/BaseFont": pdfName("/ZapfDingbats"),
	})
	pd, _ := page.pageObj().Value.(pdfDict)
	pd["/Resources"] = pdfDict{"/Font": pdfDict{"/ZaDb": pdfRef{Num: fontID}}}

	if rep := doc.ValidatePDFA(PDFA2B); pdfaHasRule(rep, "FONT_NOT_EMBEDDED") {
		t.Error("a font nothing draws with was reported as a violation")
	}

	// Once something is drawn with it, it has to be embedded.
	if err := page.appendToContentStream([]byte("BT /ZaDb 12 Tf 20 20 Td (n) Tj ET\n")); err != nil {
		t.Fatal(err)
	}
	if rep := doc.ValidatePDFA(PDFA2B); !pdfaHasRule(rep, "FONT_NOT_EMBEDDED") {
		t.Error("a non-embedded font used for rendering was not reported")
	}
}

// A visible annotation must carry its own appearance: a conforming reader is
// not allowed to build one, so a checkbox or a square with no /AP would simply
// not show up in an archival viewer.
func TestConvertToPDFAGeneratesMissingAppearance(t *testing.T) {
	doc := NewDocument(300, 200)
	page, _ := doc.Page(1)
	sq := NewSquareAnnotation(page, Rectangle{LLX: 20, LLY: 20, URX: 120, URY: 90})
	sq.SetColor(&Color{R: 1, A: 1})
	if err := page.Annotations().Add(sq); err != nil {
		t.Fatal(err)
	}
	// Drop the appearance, as a producer that leaves it to the viewer does.
	delete(sq.dict, "/AP")
	if rep := doc.ValidatePDFA(PDFA2B); !pdfaHasRule(rep, "ANNOTATION_NO_AP") {
		t.Fatal("setup: expected the missing appearance to be reported")
	}

	rep, err := doc.ConvertToPDFA(PDFA2B)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sq.dict["/AP"]; !ok {
		t.Error("conversion left the annotation without an appearance")
	}
	if pdfaHasRule(rep, "ANNOTATION_NO_AP") {
		t.Error("the missing appearance is still reported after conversion")
	}
}

// A composite (CJK) font with no program of its own is embedded from the face
// the renderer would use for the same document, carrying only the glyphs the
// text needs — a whole CJK font runs to tens of megabytes.
func TestConvertToPDFAEmbedsCompositeFont(t *testing.T) {
	if fontRepo.findCJK(fontInfo{name: "/SimSun"}, "GB1") == nil {
		t.Skip("no CJK face installed")
	}
	doc := NewDocument(300, 200)
	page, _ := doc.Page(1)

	cidID := doc.addObject(pdfDict{
		"/Type":     pdfName("/Font"),
		"/Subtype":  pdfName("/CIDFontType2"),
		"/BaseFont": pdfName("/SimSun"),
		"/CIDSystemInfo": pdfDict{
			"/Registry": "Adobe", "/Ordering": "GB1", "/Supplement": 2,
		},
		"/DW": 1000,
	})
	fontID := doc.addObject(pdfDict{
		"/Type":            pdfName("/Font"),
		"/Subtype":         pdfName("/Type0"),
		"/BaseFont":        pdfName("/SimSun"),
		"/Encoding":        pdfName("/UniGB-UCS2-H"),
		"/DescendantFonts": pdfArray{pdfRef{Num: cidID}},
	})
	pd, _ := page.pageObj().Value.(pdfDict)
	pd["/Resources"] = pdfDict{"/Font": pdfDict{"/C0": pdfRef{Num: fontID}}}
	// U+4F60 U+597D ("hello") as UCS-2 codes, which the encoding CMap turns
	// into the CIDs the font is asked for.
	if err := page.appendToContentStream([]byte("BT /C0 24 Tf 20 100 Td <4F60597D> Tj ET\n")); err != nil {
		t.Fatal(err)
	}
	if rep := doc.ValidatePDFA(PDFA2B); !pdfaHasRule(rep, "FONT_NOT_EMBEDDED") {
		t.Fatal("setup: the composite font should start out unembedded")
	}

	rep, err := doc.ConvertToPDFA(PDFA2B)
	if err != nil {
		t.Fatal(err)
	}
	if pdfaHasRule(rep, "FONT_NOT_EMBEDDED") {
		t.Fatal("the composite font was not embedded")
	}
	cid, ok := doc.objects[cidID].Value.(pdfDict)
	if !ok {
		t.Fatal("the descendant font is gone")
	}
	if _, ok := resolveRef(doc.objects, cid["/CIDToGIDMap"]).(*pdfStream); !ok {
		t.Error("no /CIDToGIDMap stream maps the document CIDs onto the subset")
	}
	desc, ok := resolveRefToDict(doc.objects, cid["/FontDescriptor"])
	if !ok {
		t.Fatal("no font descriptor")
	}
	prog, ok := resolveRef(doc.objects, desc["/FontFile2"]).(*pdfStream)
	if !ok {
		t.Fatal("no embedded font program")
	}
	if n := len(decodedStreamData(prog)); n == 0 || n > 400*1024 {
		t.Errorf("embedded program is %d bytes; expected a subset, not the whole face", n)
	}
	if w, ok := cid["/W"].(pdfArray); !ok || len(w) == 0 {
		t.Error("the descendant carries no widths for its CIDs")
	}
}
