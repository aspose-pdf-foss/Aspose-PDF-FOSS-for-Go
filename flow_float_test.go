// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"image"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// darkTextHalves counts near-black (text, not the light box fill) pixels in the
// horizontal band [y0,y1) of img, split at x == xsplit.
func darkTextHalves(img image.Image, y0, y1, xsplit int) (left, right int) {
	b := img.Bounds()
	for y := y0; y < y1 && y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if r, g, bl, _ := img.At(x, y).RGBA(); r < 20000 && g < 20000 && bl < 20000 {
				if x < xsplit {
					left++
				} else {
					right++
				}
			}
		}
	}
	return left, right
}

// TestFlowFloatWrap: a left-floated box makes the body text wrap to its right in
// the float's vertical band, then return to full width below it.
func TestFlowFloatWrap(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	flow := doc.NewFlow(pdf.FlowOptions{})
	fb := pdf.NewFloatingBox().
		SetBackground(&pdf.Color{R: 0.85, G: 0.9, B: 1, A: 1}).
		SetPadding(pdf.MarginInfo{Top: 6, Right: 6, Bottom: 6, Left: 6}).
		AddParagraph("Side note box pinned to the left edge.", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 10})
	flow.AddFloatBox(fb, pdf.FloatLeft, 150)
	flow.AddParagraph(strings.Repeat(
		"Body text wraps to the right of the floated box while it is present, then returns to full width below it. ", 12),
		pdf.TextStyle{Font: pdf.FontHelvetica, Size: 11})
	if _, err := flow.Render(); err != nil {
		t.Fatal(err)
	}

	img, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 96})
	if err != nil {
		t.Fatal(err)
	}
	// Left margin (54pt≈72px) + 150pt box (≈200px) ≈ x=270 separates the float
	// band from the text column.
	const xsplit = 270
	// In the float's band the body text must sit to the right of the split, with
	// essentially no body text under the box.
	nearL, nearR := darkTextHalves(img, 78, 150, xsplit)
	if nearR == 0 {
		t.Error("expected wrapped text to the right of the float")
	}
	if nearL > nearR/4 {
		t.Errorf("text overlaps the floated box: left=%d right=%d", nearL, nearR)
	}
	// Below the float (which is short, ≈px140), text returns to full width
	// (dark content on the left half).
	belowL, _ := darkTextHalves(img, 200, 300, xsplit)
	if belowL == 0 {
		t.Error("expected text to return to full width below the float")
	}
}

// TestFlowFloatRight: a right-floated box pushes the text to the left in its band.
func TestFlowFloatRight(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	flow := doc.NewFlow(pdf.FlowOptions{})
	fb := pdf.NewFloatingBox().
		SetBackground(&pdf.Color{R: 1, G: 0.9, B: 0.85, A: 1}).
		SetPadding(pdf.MarginInfo{Top: 6, Right: 6, Bottom: 6, Left: 6}).
		AddParagraph("Pull quote on the right.", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 10})
	flow.AddFloatBox(fb, pdf.FloatRight, 150)
	flow.AddParagraph(strings.Repeat("Left-flowing body text beside a right float. ", 14),
		pdf.TextStyle{Font: pdf.FontHelvetica, Size: 11})
	if _, err := flow.Render(); err != nil {
		t.Fatal(err)
	}
	img, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 96})
	if err != nil {
		t.Fatal(err)
	}
	// Box hugs the right (~x>520); body text must be on the left in the band.
	const xsplit = 520
	left, right := darkTextHalves(img, 78, 150, xsplit)
	if left == 0 {
		t.Error("expected wrapped text to the left of the right float")
	}
	if right > left/4 {
		t.Errorf("text overlaps the right float: left=%d right=%d", left, right)
	}
}

// TestFlowFloatTagged: a float in a tagged flow keeps the document PDF/UA-conformant.
func TestFlowFloatTagged(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	tc := doc.TaggedContent()
	tc.SetTitle("Floats")
	tc.SetLanguage("en-US")
	flow := doc.NewFlow(pdf.FlowOptions{Tagged: tc})
	flow.AddHeading(1, "Article", pdf.TextStyle{})
	flow.AddFloatBox(pdf.NewFloatingBox().
		SetPadding(pdf.MarginInfo{Top: 6, Right: 6, Bottom: 6, Left: 6}).
		AddParagraph("Aside.", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 10}), pdf.FloatRight, 140)
	flow.AddParagraph(strings.Repeat("flowing article body ", 60), pdf.TextStyle{Font: pdf.FontHelvetica, Size: 11})
	if _, err := flow.Render(); err != nil {
		t.Fatal(err)
	}
	if rep := doc.ValidatePDFUA(); !rep.Conformant {
		t.Fatalf("tagged flow with float not conformant: %+v", rep.Issues)
	}
}

// TestFlowFloatErrors covers rejected float inputs.
func TestFlowFloatErrors(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	if _, err := doc.NewFlow(pdf.FlowOptions{}).
		AddFloatBox(nil, pdf.FloatLeft, 100).
		AddParagraph("x", pdf.TextStyle{}).Render(); err == nil {
		t.Error("expected an error for a nil float box")
	}
	doc2 := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	if _, err := doc2.NewFlow(pdf.FlowOptions{}).
		AddFloatBox(pdf.NewFloatingBox(), pdf.FloatLeft, 9999).
		AddParagraph("x", pdf.TextStyle{}).Render(); err == nil {
		t.Error("expected an error for an over-wide float box")
	}
}
