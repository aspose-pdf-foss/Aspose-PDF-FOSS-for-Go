// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// makeStampSource builds a one-page document with distinctive content to stamp.
func makeStampSource(t *testing.T) *pdf.Document {
	t.Helper()
	src := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	sp, _ := src.Page(1)
	mustNoErr(t, sp.DrawRectangle(pdf.Rectangle{LLX: 40, LLY: 500, URX: 555, URY: 800},
		pdf.ShapeStyle{FillColor: &pdf.Color{R: 0.2, G: 0.3, B: 0.7, A: 1}}))
	return src
}

// TestPdfPageStamp: stamping a page from another document adds its content to the
// target page, and it survives Save+Open.
func TestPdfPageStamp(t *testing.T) {
	src := makeStampSource(t)
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)

	before, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	baseline := nonWhitePixels(before)

	st, err := pdf.NewPdfPageStamp(src, 1)
	if err != nil {
		t.Fatal(err)
	}
	st.Rect = pdf.Rectangle{LLX: 150, LLY: 300, URX: 450, URY: 520}
	if err := p.AddStamp(st); err != nil {
		t.Fatal(err)
	}

	after, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	if nonWhitePixels(after) <= baseline {
		t.Error("page stamp added no visible content")
	}

	// Survives a Save+Open round-trip (the imported XObject serialises).
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	rp, _ := out.Page(1)
	img, err := rp.RenderImage(pdf.RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	if nonWhitePixels(img) <= baseline {
		t.Error("page stamp lost after save+reopen")
	}
}

// TestPdfPageStampBackground: a background page stamp draws behind existing
// content (still adds pixels; drawn via prepend).
func TestPdfPageStampBackground(t *testing.T) {
	src := makeStampSource(t)
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	mustNoErr(t, p.AddText("foreground", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 20},
		pdf.Rectangle{LLX: 60, LLY: 400, URX: 500, URY: 430}))

	st, _ := pdf.NewPdfPageStamp(src, 1)
	st.Background = true
	st.Opacity = 0.5
	if err := p.AddStamp(st); err != nil { // zero Rect → whole page
		t.Fatal(err)
	}
	img, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	if nonWhitePixels(img) == 0 {
		t.Error("background page stamp rendered blank")
	}
}

// TestPdfPageStampErrors covers the rejected inputs.
func TestPdfPageStampErrors(t *testing.T) {
	src := makeStampSource(t)
	if _, err := pdf.NewPdfPageStamp(nil, 1); err == nil {
		t.Error("expected error for a nil source document")
	}
	if _, err := pdf.NewPdfPageStamp(src, 0); err == nil {
		t.Error("expected error for page 0")
	}
	if _, err := pdf.NewPdfPageStamp(src, 99); err == nil {
		t.Error("expected error for an out-of-range page")
	}
}
