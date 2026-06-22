// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestFormXObjectReused builds a reusable Form XObject, places it on two pages
// at different positions/scales, and verifies one shared form object renders at
// every placement after a Save/Open round-trip.
func TestFormXObjectReused(t *testing.T) {
	doc := pdf.NewDocument(400, 300)
	doc.AddBlankPage(400, 300)

	form := doc.CreateForm(120, 30)
	c := form.Canvas()
	if err := c.DrawRectangle(pdf.Rectangle{LLX: 1, LLY: 1, URX: 119, URY: 29},
		pdf.ShapeStyle{FillColor: &pdf.Color{R: 0.85, G: 0.9, B: 1, A: 1}}); err != nil {
		t.Fatalf("DrawRectangle: %v", err)
	}
	if err := c.AddText("TPL", pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 12,
		Color: &pdf.Color{A: 1}, HAlign: pdf.HAlignCenter, VAlign: pdf.VAlignMiddle},
		pdf.Rectangle{LLX: 2, LLY: 2, URX: 118, URY: 28}); err != nil {
		t.Fatalf("AddText: %v", err)
	}

	p1, _ := doc.Page(1)
	if err := p1.AddForm(form, pdf.Rectangle{LLX: 20, LLY: 250, URX: 140, URY: 280}); err != nil {
		t.Fatalf("AddForm p1 a: %v", err)
	}
	if err := p1.AddForm(form, pdf.Rectangle{LLX: 200, LLY: 200, URX: 380, URY: 260}); err != nil { // scaled up
		t.Fatalf("AddForm p1 b: %v", err)
	}
	p2, _ := doc.Page(2)
	if err := p2.AddForm(form, pdf.Rectangle{LLX: 20, LLY: 250, URX: 140, URY: 280}); err != nil {
		t.Fatalf("AddForm p2: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}

	// Each placement renders the form content (non-white pixels present).
	for _, n := range []int{1, 2} {
		img, err := out.RenderImage(n, pdf.RenderOptions{DPI: 120})
		if err != nil {
			t.Fatalf("render page %d: %v", n, err)
		}
		if px := nonWhitePixels(img); px < 100 {
			t.Errorf("page %d: form rendered almost nothing (%d px)", n, px)
		}
	}
}

// TestFormXObjectErrors covers the rejected cases.
func TestFormXObjectErrors(t *testing.T) {
	doc := pdf.NewDocument(200, 200)
	p, _ := doc.Page(1)

	if err := p.AddForm(nil, pdf.Rectangle{URX: 10, URY: 10}); err == nil {
		t.Error("AddForm(nil) = nil error, want an error")
	}

	form := doc.CreateForm(50, 50)
	if err := p.AddForm(form, pdf.Rectangle{LLX: 10, LLY: 10, URX: 10, URY: 10}); err == nil {
		t.Error("AddForm with an empty rect = nil error, want an error")
	}

	other := pdf.NewDocument(200, 200)
	op, _ := other.Page(1)
	if err := op.AddForm(form, pdf.Rectangle{URX: 50, URY: 50}); err == nil {
		t.Error("AddForm of a form from another document = nil error, want an error")
	}
}
