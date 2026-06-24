// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestFloatingBoxAbsolute: a positioned box paints its border/background and
// flows its content, keeping the text.
func TestFloatingBoxAbsolute(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	box := pdf.NewFloatingBox().
		SetBackground(&pdf.Color{R: 0.95, G: 0.95, B: 0.8, A: 1}).
		SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1.5, Color: &pdf.Color{R: 0.6, G: 0.5, A: 1}}).
		SetPadding(pdf.MarginInfo{Top: 10, Right: 10, Bottom: 10, Left: 10}).
		AddHeading(3, "Note", pdf.TextStyle{}).
		AddParagraph("A callout with a border, background and padding.", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 11})
	if err := p.AddFloatingBox(box, pdf.Rectangle{LLX: 300, LLY: 600, URX: 545, URY: 760}); err != nil {
		t.Fatal(err)
	}

	img, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 80})
	if err != nil {
		t.Fatal(err)
	}
	if nonWhitePixels(img) == 0 {
		t.Error("floating box rendered blank")
	}
	if txt, _ := p.ExtractText(); !bytes.Contains([]byte(txt), []byte("callout")) {
		t.Errorf("box text lost: %q", txt)
	}
}

// TestFloatingBoxInFlowTagged: an in-flow box inside a tagged flow produces a
// PDF/UA-conformant document (a /Div) and round-trips.
func TestFloatingBoxInFlowTagged(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	tc := doc.TaggedContent()
	tc.SetTitle("Doc")
	tc.SetLanguage("en-US")
	flow := doc.NewFlow(pdf.FlowOptions{Tagged: tc})
	flow.AddHeading(1, "Report", pdf.TextStyle{})
	flow.AddFloatingBox(pdf.NewFloatingBox().
		SetBackground(&pdf.Color{R: 0.9, G: 0.95, B: 1, A: 1}).
		SetPadding(pdf.MarginInfo{Top: 8, Right: 8, Bottom: 8, Left: 8}).
		AddParagraph("Important sidebar note inside the flow.", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 11}))
	flow.AddParagraph("Body continues after the box.", pdf.TextStyle{})
	if _, err := flow.Render(); err != nil {
		t.Fatal(err)
	}
	if rep := doc.ValidatePDFUA(); !rep.Conformant {
		t.Fatalf("flow with box not PDF/UA-conformant: %+v", rep.Issues)
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	if !bytes.Contains(buf.Bytes(), []byte("/Div")) {
		t.Error("box did not produce a /Div structure element")
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if rep := out.ValidatePDFUA(); !rep.Conformant {
		t.Errorf("not conformant after round-trip: %+v", rep.Issues)
	}
	page, _ := out.Page(1)
	if txt, _ := page.ExtractText(); !bytes.Contains([]byte(txt), []byte("sidebar")) {
		t.Errorf("box text lost after round-trip: %q", txt)
	}
}

// TestFloatingBoxErrors covers the rejected inputs.
func TestFloatingBoxErrors(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	if err := p.AddFloatingBox(nil, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}); err == nil {
		t.Error("expected error for a nil box")
	}
	if err := p.AddFloatingBox(pdf.NewFloatingBox(), pdf.Rectangle{}); err == nil {
		t.Error("expected error for an empty rect")
	}
}
