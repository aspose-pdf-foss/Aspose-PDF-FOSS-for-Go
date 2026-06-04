// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestRenderConstantAlphaFill checks the gs operator's /ca (constant fill
// alpha): a 50%-opaque red fill over a white page must blend to ~(255,128,128).
// DrawRectangle emits the ExtGState + gs for a translucent colour, so this
// exercises the whole resolve-/ExtGState path, not just the operator.
func TestRenderConstantAlphaFill(t *testing.T) {
	doc := asposepdf.NewDocument(100, 100)
	p, _ := doc.Page(1)
	if err := p.DrawRectangle(
		asposepdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100},
		asposepdf.ShapeStyle{FillColor: &asposepdf.Color{R: 1, A: 0.5}},
	); err != nil {
		t.Fatalf("DrawRectangle: %v", err)
	}
	img, err := p.RenderImage(asposepdf.RenderOptions{DPI: 72})
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}
	pixNear(t, img, 50, 50, 255, 128, 128, 6) // red @ 50% over white
}

// TestRenderOpaqueFillUnaffected guards against the alpha path tinting fully
// opaque fills: a solid red fill must stay pure red.
func TestRenderOpaqueFillUnaffected(t *testing.T) {
	doc := asposepdf.NewDocument(100, 100)
	p, _ := doc.Page(1)
	if err := p.DrawRectangle(
		asposepdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100},
		asposepdf.ShapeStyle{FillColor: &asposepdf.Color{R: 1, A: 1}},
	); err != nil {
		t.Fatalf("DrawRectangle: %v", err)
	}
	img, err := p.RenderImage(asposepdf.RenderOptions{DPI: 72})
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}
	pixNear(t, img, 50, 50, 255, 0, 0, 3)
}
