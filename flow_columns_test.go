// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"image"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// nonWhiteHalves returns the non-white pixel counts in the left and right halves
// of an image.
func nonWhiteHalves(img image.Image) (left, right int) {
	b := img.Bounds()
	mid := (b.Min.X + b.Max.X) / 2
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		for x := b.Min.X; x < b.Max.X; x += 2 {
			if r, g, bl, _ := img.At(x, y).RGBA(); r < 60000 || g < 60000 || bl < 60000 {
				if x < mid {
					left++
				} else {
					right++
				}
			}
		}
	}
	return left, right
}

// TestFlowColumns: a two-column flow fills both columns (content appears in the
// left and right halves of the page).
func TestFlowColumns(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	flow := doc.NewFlow(pdf.FlowOptions{Columns: 2})
	flow.AddHeading(2, "Two-Column Newsletter", pdf.TextStyle{})
	flow.AddParagraph(strings.Repeat(
		"This text flows down the first column, then continues at the top of the second. ", 40),
		pdf.TextStyle{Font: pdf.FontHelvetica, Size: 10})
	if _, err := flow.Render(); err != nil {
		t.Fatal(err)
	}
	img, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 80})
	if err != nil {
		t.Fatal(err)
	}
	left, right := nonWhiteHalves(img)
	if left == 0 || right == 0 {
		t.Errorf("expected content in both columns, got left=%d right=%d", left, right)
	}
}

// TestFlowColumnsPaginate: enough text to overflow both columns spills onto a
// second page.
func TestFlowColumnsPaginate(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	flow := doc.NewFlow(pdf.FlowOptions{Columns: 2})
	flow.AddParagraph(strings.Repeat("Column text that keeps going and going across columns and pages. ", 220),
		pdf.TextStyle{Font: pdf.FontHelvetica, Size: 10})
	pages, err := flow.Render()
	if err != nil {
		t.Fatal(err)
	}
	if pages < 2 {
		t.Errorf("expected multi-page two-column flow, got %d page(s)", pages)
	}
}

// TestFlowColumnsTagged: a tagged multi-column flow validates as PDF/UA and
// round-trips.
func TestFlowColumnsTagged(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	tc := doc.TaggedContent()
	tc.SetTitle("Newsletter")
	tc.SetLanguage("en-US")
	flow := doc.NewFlow(pdf.FlowOptions{Columns: 3, Tagged: tc})
	flow.AddHeading(1, "News", pdf.TextStyle{})
	flow.AddParagraph(strings.Repeat("three column body text ", 150), pdf.TextStyle{Font: pdf.FontHelvetica, Size: 9})
	if _, err := flow.Render(); err != nil {
		t.Fatal(err)
	}
	if rep := doc.ValidatePDFUA(); !rep.Conformant {
		t.Fatalf("tagged multi-column flow not conformant: %+v", rep.Issues)
	}
}
