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

// TestFloatingBoxSideBorder: a box with BorderSideLeft renders only a left rule,
// not a full outline.
func TestFloatingBoxSideBorder(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	box := pdf.NewFloatingBox().
		SetSpacing(0).
		SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideLeft, Width: 4, Color: &pdf.Color{R: 0.9, A: 1}}).
		SetPadding(pdf.MarginInfo{Top: 12, Right: 12, Bottom: 12, Left: 12}).
		AddParagraph("Quote text", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12})
	// rect spans x 100..300 pt, y 600..700 pt.
	if err := p.AddFloatingBox(box, pdf.Rectangle{LLX: 100, LLY: 600, URX: 300, URY: 700}); err != nil {
		t.Fatal(err)
	}
	img, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 96})
	if err != nil {
		t.Fatal(err)
	}
	isRed := func(x, y int) bool {
		r, g, b, _ := img.At(x, y).RGBA()
		return r > 40000 && g < 25000 && b < 25000
	}
	px := 96.0 / 72.0
	ym := int((842 - 650) * px) // mid-height of the box
	// The left edge carries the red rule; the right edge does not.
	leftEdge := false
	for x := int(98 * px); x < int(106*px); x++ {
		if isRed(x, ym) {
			leftEdge = true
		}
	}
	rightEdge := false
	for x := int(296 * px); x < int(304*px); x++ {
		if isRed(x, ym) {
			rightEdge = true
		}
	}
	if !leftEdge {
		t.Error("expected a left border rule")
	}
	if rightEdge {
		t.Error("BorderSideLeft drew a right edge (full outline) — Sides not honored")
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
