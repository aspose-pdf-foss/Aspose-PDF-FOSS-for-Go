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

// TestFormsFromPage lists a page's existing Form XObjects and re-places one on
// another page — a single shared form object serves both.
func TestFormsFromPage(t *testing.T) {
	doc := pdf.NewDocument(300, 200)
	doc.AddBlankPage(300, 200)

	src := doc.CreateForm(100, 30)
	if err := src.Canvas().AddText("X", pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 14, Color: &pdf.Color{A: 1}},
		pdf.Rectangle{LLX: 2, LLY: 2, URX: 98, URY: 28}); err != nil {
		t.Fatalf("AddText: %v", err)
	}
	p1, _ := doc.Page(1)
	if err := p1.AddForm(src, pdf.Rectangle{LLX: 20, LLY: 150, URX: 120, URY: 180}); err != nil {
		t.Fatalf("AddForm p1: %v", err)
	}

	forms := p1.Forms()
	if len(forms) != 1 {
		t.Fatalf("Page.Forms() = %d, want 1", len(forms))
	}
	if w, h := forms[0].Size(); w != 100 || h != 30 {
		t.Errorf("parsed form size = %vx%v, want 100x30", w, h)
	}

	// Re-place the parsed form on page 2.
	p2, _ := doc.Page(2)
	if err := p2.AddForm(forms[0], pdf.Rectangle{LLX: 20, LLY: 150, URX: 220, URY: 190}); err != nil {
		t.Fatalf("re-place on p2: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	for _, n := range []int{1, 2} {
		img, err := out.RenderImage(n, pdf.RenderOptions{DPI: 120})
		if err != nil {
			t.Fatalf("render page %d: %v", n, err)
		}
		if px := nonWhitePixels(img); px < 50 {
			t.Errorf("page %d: re-placed form rendered almost nothing (%d px)", n, px)
		}
	}
}

// TestImportForm copies a form (with its resource graph) into another document.
func TestImportForm(t *testing.T) {
	src := pdf.NewDocument(200, 200)
	f := src.CreateForm(90, 30)
	if err := f.Canvas().AddText("IMP", pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 14, Color: &pdf.Color{A: 1}},
		pdf.Rectangle{LLX: 2, LLY: 2, URX: 88, URY: 28}); err != nil {
		t.Fatalf("AddText: %v", err)
	}
	sp, _ := src.Page(1)
	if err := sp.AddForm(f, pdf.Rectangle{LLX: 10, LLY: 150, URX: 100, URY: 180}); err != nil {
		t.Fatalf("AddForm src: %v", err)
	}

	dst := pdf.NewDocument(200, 200)
	imported, err := dst.ImportForm(f)
	if err != nil {
		t.Fatalf("ImportForm: %v", err)
	}
	dp, _ := dst.Page(1)
	if err := dp.AddForm(imported, pdf.Rectangle{LLX: 20, LLY: 140, URX: 180, URY: 180}); err != nil {
		t.Fatalf("AddForm dst: %v", err)
	}

	var buf bytes.Buffer
	if _, err := dst.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	img, err := out.RenderImage(1, pdf.RenderOptions{DPI: 120})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if px := nonWhitePixels(img); px < 50 {
		t.Errorf("imported form rendered almost nothing (%d px) — resource graph not copied?", px)
	}

	// Importing a form already in the document returns it unchanged.
	if same, _ := dst.ImportForm(imported); same != imported {
		t.Error("ImportForm of a same-document form should return it unchanged")
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
