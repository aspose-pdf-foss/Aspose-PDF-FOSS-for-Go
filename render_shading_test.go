// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestRenderLinearShadingPattern fills a rectangle with a left-to-right red→blue
// linear gradient (a PatternType 2 shading pattern) and checks the rendered
// endpoints. This exercises resolveShadingPattern + paintShading + the axial
// shading and exponential (type 2) function.
func TestRenderLinearShadingPattern(t *testing.T) {
	doc := asposepdf.NewDocument(100, 100)
	p, _ := doc.Page(1)
	g := asposepdf.NewLinearGradient(0, 0, 100, 0,
		asposepdf.GradientStop{Offset: 0, Color: asposepdf.Color{R: 1, A: 1}},
		asposepdf.GradientStop{Offset: 1, Color: asposepdf.Color{B: 1, A: 1}},
	)
	if err := p.DrawRectangle(
		asposepdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100},
		asposepdf.ShapeStyle{FillGradient: g},
	); err != nil {
		t.Fatalf("DrawRectangle: %v", err)
	}
	img, err := p.RenderImage(asposepdf.RenderOptions{DPI: 72})
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}

	lr, _, lb, _ := img.At(8, 50).RGBA()   // near left → reddish
	rr, _, rb, _ := img.At(92, 50).RGBA()  // near right → bluish
	if !(lr>>8 > 180 && lb>>8 < 80) {
		t.Errorf("left pixel = R%d B%d, want red-dominant", lr>>8, lb>>8)
	}
	if !(rb>>8 > 180 && rr>>8 < 80) {
		t.Errorf("right pixel = R%d B%d, want blue-dominant", rr>>8, rb>>8)
	}
	// Gradient must actually vary: red falls off and blue rises left→right.
	if !(lr>>8 > rr>>8 && rb>>8 > lb>>8) {
		t.Errorf("gradient not monotonic: leftR=%d rightR=%d leftB=%d rightB=%d",
			lr>>8, rr>>8, lb>>8, rb>>8)
	}
}

// TestRenderThreeStopShadingPattern uses a 3-stop gradient (red→green→blue),
// which the writer emits as a stitching (type 3) function, so it exercises the
// fnStitching evaluator: the middle of the rectangle must read green.
func TestRenderThreeStopShadingPattern(t *testing.T) {
	doc := asposepdf.NewDocument(120, 40)
	p, _ := doc.Page(1)
	g := asposepdf.NewLinearGradient(0, 0, 120, 0,
		asposepdf.GradientStop{Offset: 0, Color: asposepdf.Color{R: 1, A: 1}},
		asposepdf.GradientStop{Offset: 0.5, Color: asposepdf.Color{G: 1, A: 1}},
		asposepdf.GradientStop{Offset: 1, Color: asposepdf.Color{B: 1, A: 1}},
	)
	if err := p.DrawRectangle(
		asposepdf.Rectangle{LLX: 0, LLY: 0, URX: 120, URY: 40},
		asposepdf.ShapeStyle{FillGradient: g},
	); err != nil {
		t.Fatalf("DrawRectangle: %v", err)
	}
	img, err := p.RenderImage(asposepdf.RenderOptions{DPI: 72})
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}
	r, gg, b, _ := img.At(60, 20).RGBA() // centre → green stop
	if !(gg>>8 > 150 && r>>8 < 120 && b>>8 < 120) {
		t.Errorf("centre = (%d,%d,%d), want green-dominant", r>>8, gg>>8, b>>8)
	}
}

// TestRenderRadialShadingPattern fills a rectangle with a radial gradient
// (white centre → black edge) and checks the centre is light and a corner dark.
func TestRenderRadialShadingPattern(t *testing.T) {
	doc := asposepdf.NewDocument(100, 100)
	p, _ := doc.Page(1)
	g := asposepdf.NewRadialGradient(50, 50, 50,
		asposepdf.GradientStop{Offset: 0, Color: asposepdf.Color{R: 1, G: 1, B: 1, A: 1}},
		asposepdf.GradientStop{Offset: 1, Color: asposepdf.Color{A: 1}}, // black
	)
	if err := p.DrawRectangle(
		asposepdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100},
		asposepdf.ShapeStyle{FillGradient: g},
	); err != nil {
		t.Fatalf("DrawRectangle: %v", err)
	}
	img, err := p.RenderImage(asposepdf.RenderOptions{DPI: 72})
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}
	cr, _, _, _ := img.At(50, 50).RGBA() // centre → light
	er, _, _, _ := img.At(50, 4).RGBA()  // near edge → dark
	if cr>>8 < 200 {
		t.Errorf("centre = %d, want light (white centre)", cr>>8)
	}
	if er>>8 > cr>>8 {
		t.Errorf("edge (%d) not darker than centre (%d)", er>>8, cr>>8)
	}
}
