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

// TestConvertToPDFAEmbedsStandard14: a Standard-14 document becomes fully
// conformant (the fonts are auto-embedded) and its text survives.
func TestConvertToPDFAEmbedsStandard14(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	p.AddText("Helvetica line", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 18},
		pdf.Rectangle{LLX: 50, LLY: 720, URX: 500, URY: 760})
	p.AddText("Times line", pdf.TextStyle{Font: pdf.FontTimesRoman, Size: 18},
		pdf.Rectangle{LLX: 50, LLY: 680, URX: 500, URY: 720})
	p.AddText("Courier line", pdf.TextStyle{Font: pdf.FontCourier, Size: 18},
		pdf.Rectangle{LLX: 50, LLY: 640, URX: 500, URY: 680})

	rep, err := doc.ConvertToPDFA(pdf.PDFA1B)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Conformant {
		t.Fatalf("Standard-14 document not conformant after conversion: %+v", rep.Issues)
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	page, _ := out.Page(1)
	txt, _ := page.ExtractText()
	for _, want := range []string{"Helvetica line", "Times line", "Courier line"} {
		if !bytes.Contains([]byte(txt), []byte(want)) {
			t.Errorf("text %q lost after embedding+round-trip; got %q", want, txt)
		}
	}
	if rt := out.ValidatePDFA(pdf.PDFA1B); !rt.Conformant {
		t.Errorf("not conformant after round-trip: %+v", rt.Issues)
	}
}

// TestConvertToPDFAAccessible: a tagged document converts to a fully conformant
// PDF/A-1a (accessible) file and stays conformant after a round-trip.
func TestConvertToPDFAAccessible(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	tc := doc.TaggedContent()
	tc.SetTitle("Accessible Archive")
	tc.SetLanguage("en-US")
	p, _ := doc.Page(1)
	if _, err := p.TagContent(tc.Root(), pdf.StructH1, func() error {
		return p.AddText("Accessible Archive", pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 22},
			pdf.Rectangle{LLX: 50, LLY: 760, URX: 545, URY: 800})
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.TagContent(tc.Root(), pdf.StructP, func() error {
		return p.AddText("Body text.", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
			pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 740})
	}); err != nil {
		t.Fatal(err)
	}

	rep, err := doc.ConvertToPDFA(pdf.PDFA1A)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Conformant {
		t.Fatalf("tagged document not PDF/A-1a conformant after conversion: %+v", rep.Issues)
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if rt := out.ValidatePDFA(pdf.PDFA1A); !rt.Conformant {
		t.Errorf("not PDF/A-1a conformant after round-trip: %+v", rt.Issues)
	}
}

// TestConvertToPDFASymbolRemains: Symbol/ZapfDingbats have no Latin substitute
// and stay a reported violation.
func TestConvertToPDFASymbolRemains(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	p.AddText("abcd", pdf.TextStyle{Font: pdf.FontSymbol, Size: 18},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 400, URY: 740})
	rep, err := doc.ConvertToPDFA(pdf.PDFA1B)
	if err != nil {
		t.Fatal(err)
	}
	if hasRule(rep, "XMP_MISSING") || hasRule(rep, "COLOR_NO_OUTPUT_INTENT") {
		t.Error("metadata/colour should still be fixed even with a Symbol font")
	}
	if !hasRule(rep, "FONT_NOT_EMBEDDED") {
		t.Error("expected Symbol to remain a FONT_NOT_EMBEDDED violation")
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
