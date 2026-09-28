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
// A Symbol font has no metric-compatible Latin clone to stand in for it, so
// conversion depends on a real symbol face being available: when one is
// registered (or installed), it is embedded; otherwise the document keeps the
// violation. Both outcomes are correct — what must not happen is a clean
// report with no font program in the file.
func TestConvertToPDFASymbolFont(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	if err := p.AddText("abcd", pdf.TextStyle{Font: pdf.FontSymbol, Size: 18},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 400, URY: 740}); err != nil {
		t.Fatal(err)
	}
	rep, err := doc.ConvertToPDFA(pdf.PDFA1B)
	if err != nil {
		t.Fatal(err)
	}
	if hasRule(rep, "XMP_MISSING") || hasRule(rep, "COLOR_NO_OUTPUT_INTENT") {
		t.Error("metadata/colour should still be fixed even with a Symbol font")
	}
	if hasRule(rep, "FONT_NOT_EMBEDDED") {
		return // no symbol face available here, and the report says so
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("/FontFile2")) {
		t.Error("conversion reported no font violation but embedded no font program")
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

// TestConvertToPDFA1FlattensTransparency: PDF/A-1 forbids transparency
// outright, so ConvertToPDFA(PDFA1B) must clear TRANSPARENCY on its own —
// without a separate FlattenTransparency call — and the fully-conformant
// report must survive a Save/Open round-trip.
func TestConvertToPDFA1FlattensTransparency(t *testing.T) {
	doc := buildTransparentPageDoc(t)

	before := doc.ValidatePDFA(pdf.PDFA1B)
	if !hasRule(before, "TRANSPARENCY") {
		t.Fatal("setup: expected TRANSPARENCY before conversion")
	}

	rep, err := doc.ConvertToPDFA(pdf.PDFA1B)
	if err != nil {
		t.Fatal(err)
	}
	if hasRule(rep, "TRANSPARENCY") {
		t.Errorf("TRANSPARENCY still reported after ConvertToPDFA(PDFA1B): %+v", rep.Issues)
	}
	if !rep.Conformant {
		t.Errorf("not conformant after conversion: %+v", rep.Issues)
	}

	// Direct, black-box confirmation that page 1 was actually rasterized
	// (not just that the TRANSPARENCY rule happens to be silent) — the
	// complement of TestConvertToPDFA2KeepsTransparency's ==0 check.
	p1, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if imgs, err := p1.ImageInfos(); err != nil {
		t.Fatal(err)
	} else if len(imgs) != 1 {
		t.Errorf("page 1 has %d image(s) after ConvertToPDFA(PDFA1B), want 1 (rasterized)", len(imgs))
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if rt := out.ValidatePDFA(pdf.PDFA1B); !rt.Conformant {
		t.Errorf("not conformant after round-trip: %+v", rt.Issues)
	}
}

// TestConvertToPDFA2KeepsTransparency: PDF/A-2 and -3 permit transparency, so
// ConvertToPDFA must not rasterize pages for them. The interesting page here
// is page 1 (the one buildTransparentPageDoc actually gives transparency to,
// via an alpha-filled rectangle) — checking only page 2 (always opaque,
// never a flatten candidate either way) would pass even if page 1 got
// rasterized, since pdfaCheckTransparency itself already gates on
// format.part()==1 and would report no TRANSPARENCY regardless of whether
// flattening ran. ImageInfos is the direct, black-box signal that
// rasterization did or didn't happen: a vector-drawn rectangle contributes no
// image; a flattened page would gain exactly one full-page raster.
func TestConvertToPDFA2KeepsTransparency(t *testing.T) {
	doc := buildTransparentPageDoc(t)
	p1, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	before, err := p1.ImageInfos()
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 0 {
		t.Fatalf("setup: page 1 already has %d image(s), want 0 (vector rectangle only)", len(before))
	}

	rep, err := doc.ConvertToPDFA(pdf.PDFA2B)
	if err != nil {
		t.Fatal(err)
	}
	if hasRule(rep, "TRANSPARENCY") {
		t.Error("PDF/A-2 permits transparency; TRANSPARENCY should not be reported")
	}

	after, err := p1.ImageInfos()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Errorf("page 1 gained %d image(s) — it was rasterized despite PDF/A-2 permitting transparency", len(after))
	}

	p2, err := doc.Page(2)
	if err != nil {
		t.Fatal(err)
	}
	txt, err := p2.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if txt != "plain opaque text" {
		t.Errorf("page 2 text lost/changed — page appears to have been rasterized despite PDF/A-2 permitting transparency: %q", txt)
	}
}

// TestConvertToPDFA1AFlattensTransparency: the flatten step is shared by both
// PDF/A-1 conformance letters (format.part()==1), so PDFA1A must also clear
// TRANSPARENCY end-to-end through ConvertToPDFA, not just via ValidatePDFA.
// The document isn't tagged, so Conformant stays false (NOT_TAGGED etc.) —
// only TRANSPARENCY specifically is asserted gone.
func TestConvertToPDFA1AFlattensTransparency(t *testing.T) {
	doc := buildTransparentPageDoc(t)
	rep, err := doc.ConvertToPDFA(pdf.PDFA1A)
	if err != nil {
		t.Fatal(err)
	}
	if hasRule(rep, "TRANSPARENCY") {
		t.Errorf("TRANSPARENCY still reported after ConvertToPDFA(PDFA1A): %+v", rep.Issues)
	}
}

// TestValidatePDFA1AFlagsTransparency: PDF/A-1a is still PDF/A-1 (ISO
// 19005-1) and forbids transparency exactly like PDF/A-1b — the checker
// used to gate this rule on == PDFA1B specifically, so it silently skipped
// PDFA1A.
func TestValidatePDFA1AFlagsTransparency(t *testing.T) {
	doc := buildTransparentPageDoc(t)
	rep := doc.ValidatePDFA(pdf.PDFA1A)
	if !hasRule(rep, "TRANSPARENCY") {
		t.Error("expected TRANSPARENCY when validating a transparent page as PDF/A-1a")
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
