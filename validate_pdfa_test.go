// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func hasRule(rep *pdf.PDFAValidationReport, rule string) bool {
	for _, is := range rep.Issues {
		if is.Rule == rule {
			return true
		}
	}
	return false
}

const pdfaidXMP = `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
 <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description rdf:about="" xmlns:pdfaid="http://www.aiim.org/pdfa/ns/id/">
   <pdfaid:part>1</pdfaid:part>
   <pdfaid:conformance>B</pdfaid:conformance>
  </rdf:Description>
 </rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>`

// TestValidatePDFAPlainDoc: a Standard-14 document is not conformant and the
// font/XMP/colour violations are reported.
func TestValidatePDFAPlainDoc(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	if err := p.AddText("Hello PDF/A", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 24},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 500, URY: 760}); err != nil {
		t.Fatal(err)
	}
	rep := doc.ValidatePDFA(pdf.PDFA1B)
	if rep.Conformant {
		t.Fatal("plain Standard-14 document reported PDF/A-conformant")
	}
	if !hasRule(rep, "FONT_NOT_EMBEDDED") {
		t.Error("expected FONT_NOT_EMBEDDED for Standard-14 Helvetica")
	}
	if !hasRule(rep, "XMP_MISSING") {
		t.Error("expected XMP_MISSING")
	}
	if rep.Format != pdf.PDFA1B {
		t.Errorf("report format = %v, want PDFA1B", rep.Format)
	}
}

// TestValidatePDFAEmbeddedFont: an embedded TrueType font does not trigger
// FONT_NOT_EMBEDDED.
func TestValidatePDFAEmbeddedFont(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Skipf("embed font unavailable: %v", err)
	}
	p, _ := doc.Page(1)
	if err := p.AddText("Embedded", pdf.TextStyle{Font: font, Size: 24},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 500, URY: 760}); err != nil {
		t.Fatal(err)
	}
	rep := doc.ValidatePDFA(pdf.PDFA1B)
	if hasRule(rep, "FONT_NOT_EMBEDDED") {
		t.Error("embedded TrueType font wrongly flagged FONT_NOT_EMBEDDED")
	}
}

// TestValidatePDFANotdefGlyphUsed reproduces GitHub issue #19 point 2
// (shadowhunter1470-svg): a character the embedded font has no glyph
// for draws .notdef (glyph 0) silently under the default
// MissingGlyphAllow policy, and that is a real PDF/A violation
// (veraPDF 6.2.11.8-1) ValidatePDFA/ConvertToPDFA used to miss
// entirely. AddTextAt (not AddText) is used here because AddText
// normalizes a bare TAB away via strings.Fields before it ever reaches
// the font.
func TestValidatePDFANotdefGlyphUsed(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Skipf("embed font unavailable: %v", err)
	}
	p, _ := doc.Page(1)
	if err := p.AddTextAt("a\tb", pdf.TextStyle{Font: font, Size: 12}, 50, 700); err != nil {
		t.Fatal(err)
	}
	rep := doc.ValidatePDFA(pdf.PDFA2B)
	if !hasRule(rep, "NOTDEF_GLYPH_USED") {
		t.Errorf("expected NOTDEF_GLYPH_USED for a drawn .notdef glyph, got: %+v", rep.Issues)
	}
	if rep.Conformant {
		t.Error("document with a used .notdef glyph reported PDF/A-conformant")
	}

	// ConvertToPDFA's own returned report must reflect the same
	// violation -- it used to report Conformant:true (the exact bug
	// the issue reported) because this check didn't exist yet.
	convRep, err := doc.ConvertToPDFA(pdf.PDFA2B)
	if err != nil {
		t.Fatal(err)
	}
	if !hasRule(convRep, "NOTDEF_GLYPH_USED") {
		t.Errorf("ConvertToPDFA's report missing NOTDEF_GLYPH_USED: %+v", convRep.Issues)
	}
	if convRep.Conformant {
		t.Error("ConvertToPDFA reported Conformant:true for a document with a used .notdef glyph")
	}
}

// TestValidatePDFANoFalsePositiveWithoutMissingGlyphs confirms ordinary
// text that the font fully covers never trips NOTDEF_GLYPH_USED.
func TestValidatePDFANoFalsePositiveWithoutMissingGlyphs(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Skipf("embed font unavailable: %v", err)
	}
	p, _ := doc.Page(1)
	if err := p.AddText("Hello, world!", pdf.TextStyle{Font: font, Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 500, URY: 760}); err != nil {
		t.Fatal(err)
	}
	rep := doc.ValidatePDFA(pdf.PDFA2B)
	if hasRule(rep, "NOTDEF_GLYPH_USED") {
		t.Errorf("ordinary fully-covered text wrongly flagged NOTDEF_GLYPH_USED: %+v", rep.Issues)
	}
}

// TestValidatePDFAXMP: a pdfaid packet clears the XMP violations for the matching
// part and triggers a part mismatch for a different level.
func TestValidatePDFAXMP(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	if err := doc.SetXMPRaw([]byte(pdfaidXMP)); err != nil {
		t.Fatal(err)
	}
	rep1 := doc.ValidatePDFA(pdf.PDFA1B)
	if hasRule(rep1, "XMP_MISSING") || hasRule(rep1, "XMP_PDFAID_MISSING") || hasRule(rep1, "XMP_PART_MISMATCH") {
		t.Errorf("pdfaid part=1/B wrongly flagged for PDFA1B: %+v", rep1.Issues)
	}
	rep2 := doc.ValidatePDFA(pdf.PDFA2B)
	if !hasRule(rep2, "XMP_PART_MISMATCH") {
		t.Error("expected XMP_PART_MISMATCH when validating a part-1 packet as PDF/A-2B")
	}
}

// TestValidatePDFAEncrypted: encryption is a violation.
func TestValidatePDFAEncrypted(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	doc.SetEncryption(pdf.EncryptionOptions{UserPassword: "u", OwnerPassword: "o"})
	rep := doc.ValidatePDFA(pdf.PDFA1B)
	if !hasRule(rep, "ENCRYPTED") {
		t.Error("expected ENCRYPTED violation")
	}
}

// TestValidatePDFAFormatString covers the conformance-name strings.
func TestValidatePDFAFormatString(t *testing.T) {
	for f, want := range map[pdf.PDFAFormat]string{
		pdf.PDFA1B: "PDF/A-1B", pdf.PDFA2B: "PDF/A-2B", pdf.PDFA3B: "PDF/A-3B",
		pdf.PDFA1A: "PDF/A-1A", pdf.PDFA2A: "PDF/A-2A", pdf.PDFA3A: "PDF/A-3A",
	} {
		if got := f.String(); !strings.EqualFold(got, want) {
			t.Errorf("%d.String() = %q, want %q", f, got, want)
		}
	}
}

// TestValidatePDFAAccessibleRequiresTagging: an untagged document fails the "a"
// levels with the tagging rules but is fine at the "b" levels.
func TestValidatePDFAAccessibleRequiresTagging(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	repA := doc.ValidatePDFA(pdf.PDFA1A)
	for _, want := range []string{"NOT_TAGGED", "NO_STRUCT_TREE", "NO_LANG"} {
		if !hasRule(repA, want) {
			t.Errorf("expected %s when validating an untagged document as PDF/A-1A", want)
		}
	}
	repB := doc.ValidatePDFA(pdf.PDFA1B)
	for _, unexpected := range []string{"NOT_TAGGED", "NO_STRUCT_TREE", "NO_LANG"} {
		if hasRule(repB, unexpected) {
			t.Errorf("PDF/A-1B should not require tagging, but got %s", unexpected)
		}
	}
}
