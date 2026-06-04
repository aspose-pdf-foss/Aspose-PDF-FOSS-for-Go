// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"image"
	"math"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// pixNear asserts the pixel at (x,y) is approximately (r,g,b) on 0..255 scale.
func pixNear(t *testing.T, img image.Image, x, y int, r, g, b uint8, tol int) {
	t.Helper()
	pr, pg, pb, _ := img.At(x, y).RGBA()
	gr, gg, gb := int(pr>>8), int(pg>>8), int(pb>>8)
	if absInt(gr-int(r)) > tol || absInt(gg-int(g)) > tol || absInt(gb-int(b)) > tol {
		t.Errorf("pixel (%d,%d) = (%d,%d,%d), want ~(%d,%d,%d)", x, y, gr, gg, gb, r, g, b)
	}
}

func absInt(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

func TestRenderBlankPageDimensions(t *testing.T) {
	doc := asposepdf.NewDocument(200, 100)
	p, _ := doc.Page(1)
	img, err := p.RenderImage(asposepdf.RenderOptions{DPI: 150})
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}
	wantW := int(math.Round(200.0 / 72 * 150))
	wantH := int(math.Round(100.0 / 72 * 150))
	if b := img.Bounds(); b.Dx() != wantW || b.Dy() != wantH {
		t.Errorf("bounds = %dx%d, want %dx%d", b.Dx(), b.Dy(), wantW, wantH)
	}
	pixNear(t, img, wantW/2, wantH/2, 255, 255, 255, 1) // blank → white
}

func TestRenderFilledRectangle(t *testing.T) {
	doc := asposepdf.NewDocument(100, 100)
	p, _ := doc.Page(1)
	if err := p.DrawRectangle(
		asposepdf.Rectangle{LLX: 20, LLY: 20, URX: 80, URY: 80},
		asposepdf.ShapeStyle{FillColor: &asposepdf.Color{R: 1, A: 1}},
	); err != nil {
		t.Fatalf("DrawRectangle: %v", err)
	}
	img, err := p.RenderImage(asposepdf.RenderOptions{DPI: 72}) // 1 px per point
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}
	pixNear(t, img, 50, 50, 255, 0, 0, 3)   // interior → red
	pixNear(t, img, 5, 5, 255, 255, 255, 1) // outside → white
}

func TestRenderBackgroundOption(t *testing.T) {
	doc := asposepdf.NewDocument(50, 50)
	p, _ := doc.Page(1)
	img, err := p.RenderImage(asposepdf.RenderOptions{DPI: 72, Background: &asposepdf.Color{R: 0, G: 0, B: 1, A: 1}})
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}
	pixNear(t, img, 25, 25, 0, 0, 255, 1) // blue backdrop
}

func TestRenderStrokedLine(t *testing.T) {
	doc := asposepdf.NewDocument(100, 100)
	p, _ := doc.Page(1)
	if err := p.DrawLine(
		asposepdf.Point{X: 10, Y: 50}, asposepdf.Point{X: 90, Y: 50},
		asposepdf.LineStyle{Color: &asposepdf.Color{A: 1}, Width: 6},
	); err != nil {
		t.Fatalf("DrawLine: %v", err)
	}
	img, err := p.RenderImage(asposepdf.RenderOptions{DPI: 72})
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}
	// Line at device y=50, ~6px tall band → (50,50) black, (50,30) white.
	pixNear(t, img, 50, 50, 0, 0, 0, 4)
	pixNear(t, img, 50, 30, 255, 255, 255, 1)
}
