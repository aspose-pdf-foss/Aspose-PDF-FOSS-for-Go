// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"image"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// hasNonWhite reports whether any pixel in the rect [x0,x1)×[y0,y1) is not white.
func hasNonWhite(img image.Image, x0, y0, x1, y1 int) bool {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r>>8 < 240 || g>>8 < 240 || b>>8 < 240 {
				return true
			}
		}
	}
	return false
}

// TestRenderFormWidgetAppearance checks that form-field widget appearances —
// which live in /Annots, not the page content stream — are painted by the
// renderer. A push button is drawn into a known rect; that rect must contain
// painted (non-white) pixels while an untouched margin stays white.
func TestRenderFormWidgetAppearance(t *testing.T) {
	doc := asposepdf.NewDocument(200, 200)
	form := doc.Form()
	// Rect in PDF user space (origin bottom-left): a button mid-page.
	if _, err := form.AddPushButton(1, asposepdf.Rectangle{LLX: 50, LLY: 120, URX: 150, URY: 160}, "b1", "Click"); err != nil {
		t.Fatalf("AddPushButton: %v", err)
	}
	p, _ := doc.Page(1)
	img, err := p.RenderImage(asposepdf.RenderOptions{DPI: 72}) // 1px per point, 200×200
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}

	// Device Y is flipped: user y∈[120,160] → device y∈[40,80].
	if !hasNonWhite(img, 50, 40, 150, 80) {
		t.Error("button widget region is blank — annotation appearance not rendered")
	}
	if hasNonWhite(img, 0, 0, 200, 20) { // top margin, no widget
		t.Error("top margin unexpectedly painted")
	}
}
