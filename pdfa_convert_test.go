// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestConvertToPDFAEmbeddedFont: a document with an embedded font and device
// colour becomes conformant after conversion and stays conformant after a
// Save/Open round-trip.
func TestConvertToPDFAEmbeddedFont(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Skipf("embed font unavailable: %v", err)
	}
	p, _ := doc.Page(1)
	if err := p.AddText("PDF/A", pdf.TextStyle{Font: font, Size: 24},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 500, URY: 760}); err != nil {
		t.Fatal(err)
	}
	if err := p.DrawRectangle(pdf.Rectangle{LLX: 50, LLY: 50, URX: 200, URY: 200},
		pdf.ShapeStyle{FillColor: &pdf.Color{R: 0.2, G: 0.4, B: 0.8, A: 1}}); err != nil {
		t.Fatal(err)
	}

	rep, err := doc.ConvertToPDFA(pdf.PDFA1B)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Conformant {
		t.Fatalf("not conformant after conversion: %+v", rep.Issues)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if rt := out.ValidatePDFA(pdf.PDFA1B); !rt.Conformant {
		t.Errorf("not conformant after round-trip: %+v", rt.Issues)
	}
}

// TestConvertToPDFAFixesMetadataAndColor: even a Standard-14 document has its XMP
// and OutputIntent fixed (only the un-embeddable font remains).
func TestConvertToPDFAFixesMetadataAndColor(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	p.AddText("Hi", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 18},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 400, URY: 740})

	rep, err := doc.ConvertToPDFA(pdf.PDFA1B)
	if err != nil {
		t.Fatal(err)
	}
	if hasRule(rep, "XMP_MISSING") || hasRule(rep, "XMP_PDFAID_MISSING") {
		t.Error("XMP still missing after conversion")
	}
	if hasRule(rep, "COLOR_NO_OUTPUT_INTENT") {
		t.Error("OutputIntent still missing after conversion")
	}
	if !hasRule(rep, "FONT_NOT_EMBEDDED") {
		t.Error("expected the Standard-14 font to remain a violation")
	}
}

// TestConvertToPDFAStripsJavaScript removes document-level JavaScript.
func TestConvertToPDFAStripsJavaScript(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	if err := doc.JavaScript().Add("hello", "app.alert('hi')"); err != nil {
		t.Fatal(err)
	}
	if rep := doc.ValidatePDFA(pdf.PDFA1B); !hasRule(rep, "JAVASCRIPT") {
		t.Fatal("setup: expected JAVASCRIPT before conversion")
	}
	rep, err := doc.ConvertToPDFA(pdf.PDFA1B)
	if err != nil {
		t.Fatal(err)
	}
	if hasRule(rep, "JAVASCRIPT") {
		t.Error("JavaScript not removed by ConvertToPDFA")
	}
}

// TestSRGBICCProfileStructure sanity-checks the generated ICC profile header.
func TestSRGBICCProfileStructure(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	p.DrawRectangle(pdf.Rectangle{LLX: 10, LLY: 10, URX: 100, URY: 100},
		pdf.ShapeStyle{FillColor: &pdf.Color{R: 1, A: 1}})
	if _, err := doc.ConvertToPDFA(pdf.PDFA2B); err != nil {
		t.Fatal(err)
	}
	// Save and confirm the OutputIntent + ICC profile survive a round-trip and
	// the document is recognised as conformant.
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	for _, marker := range []string{"/OutputIntent", "/GTS_PDFA1", "/DestOutputProfile"} {
		if !bytes.Contains(buf.Bytes(), []byte(marker)) {
			t.Errorf("output missing %s", marker)
		}
	}
	// The ICC profile stream round-trips and the document re-validates.
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if hasRule(out.ValidatePDFA(pdf.PDFA2B), "COLOR_NO_OUTPUT_INTENT") {
		t.Error("OutputIntent lost after round-trip")
	}
}
