// SPDX-License-Identifier: MIT

package asposepdf

import (
	"image"
	"math"
	"testing"
)

var (
	testRed  = &Color{R: 1, G: 0, B: 0, A: 1}
	testBlue = &Color{R: 0, G: 0, B: 1, A: 1}
)

func markDoc(t *testing.T, w, h float64, mark Rectangle, c *Color) *Document {
	t.Helper()
	d := NewDocument(w, h)
	p, _ := d.Page(1)
	if err := p.DrawRectangle(mark, ShapeStyle{FillColor: c}); err != nil {
		t.Fatal(err)
	}
	return d
}

func isColor(img image.Image, x, y int, c *Color) bool {
	r, g, b, _ := img.At(x, y).RGBA()
	return math.Abs(float64(r)/65535-c.R) < 0.15 && math.Abs(float64(g)/65535-c.G) < 0.15 && math.Abs(float64(b)/65535-c.B) < 0.15
}

func stampOnto(t *testing.T, src *Document, w, h float64) image.Image {
	t.Helper()
	dst := NewDocument(w, h)
	dp, _ := dst.Page(1)
	st, err := NewPdfPageStamp(src, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := dp.AddStamp(st); err != nil {
		t.Fatal(err)
	}
	img, err := dst.RenderImage(1, RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// Mark at x 20..80, y 780..830 of a 595x842 page (top-left corner).
var topLeftMark = Rectangle{LLX: 20, LLY: 780, URX: 80, URY: 830}

func TestPageStampHonoursRotate(t *testing.T) {
	cases := []struct {
		rot    RotationAngle
		w, h   float64
		px, py int // pixel (row from top) expected red
		qx, qy int // pixel expected white
	}{
		{Rotate90, 842, 595, 805, 50, 50, 50},
		{Rotate180, 595, 842, 545, 805, 50, 50},
		{Rotate270, 842, 595, 37, 545, 805, 50},
	}
	for _, c := range cases {
		src := markDoc(t, 595, 842, topLeftMark, testRed)
		if err := src.SetRotation(c.rot, 1); err != nil {
			t.Fatal(err)
		}
		img := stampOnto(t, src, c.w, c.h)
		if !isColor(img, c.px, c.py, testRed) {
			t.Errorf("rotate %d: expected red at (%d,%d)", c.rot, c.px, c.py)
		}
		if isColor(img, c.qx, c.qy, testRed) {
			t.Errorf("rotate %d: unexpected red at (%d,%d)", c.rot, c.qx, c.qy)
		}
	}
}

func TestImportPageDisplayedBox(t *testing.T) {
	src := markDoc(t, 595, 842, topLeftMark, testRed)
	_ = src.SetRotation(Rotate90, 1)
	pg, _ := src.Page(1)
	dst := NewDocument(100, 100)
	_, box, err := dst.importPageAsXObject(src, pg, map[int]int{})
	if err != nil {
		t.Fatal(err)
	}
	if box != (Rectangle{URX: 842, URY: 595}) {
		t.Errorf("box = %+v", box)
	}
}

func TestPageStampHonoursCropBox(t *testing.T) {
	src := NewDocument(595, 842)
	sp, _ := src.Page(1)
	_ = sp.DrawRectangle(Rectangle{LLX: 20, LLY: 760, URX: 80, URY: 820}, ShapeStyle{FillColor: testBlue})
	_ = sp.DrawRectangle(Rectangle{LLX: 20, LLY: 100, URX: 80, URY: 160}, ShapeStyle{FillColor: testRed})
	crop := Rectangle{LLX: 0, LLY: 421, URX: 595, URY: 842}
	if err := sp.SetCropBox(crop); err != nil {
		t.Fatal(err)
	}
	dst := NewDocument(100, 100)
	_, box, err := dst.importPageAsXObject(src, sp, map[int]int{})
	if err != nil || box != crop {
		t.Fatalf("box = %+v err=%v", box, err)
	}
	img := stampOnto(t, src, 595, 421)
	// blue at y 760..820 -> rows 22..82 from the top at scale 1
	if !isColor(img, 50, 50, testBlue) {
		t.Error("visible mark missing")
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if isColor(img, x, y, testRed) {
				t.Fatalf("hidden content visible at %d,%d", x, y)
			}
		}
	}
}

func TestPageStampCropBoxOutsideMediaBoxFallsBack(t *testing.T) {
	src := markDoc(t, 595, 842, topLeftMark, testRed)
	sp, _ := src.Page(1)
	_ = sp.SetCropBox(Rectangle{LLX: -100, LLY: 400, URX: 700, URY: 900})
	dst := NewDocument(100, 100)
	_, box, err := dst.importPageAsXObject(src, sp, map[int]int{})
	if err != nil {
		t.Fatal(err)
	}
	if box != (Rectangle{LLX: 0, LLY: 400, URX: 595, URY: 842}) {
		t.Errorf("box = %+v", box)
	}
}

func TestNUpHonoursRotate(t *testing.T) {
	src := markDoc(t, 595, 842, topLeftMark, testRed)
	_ = src.SetRotation(Rotate90, 1)
	out, err := src.NUp(NUpOptions{Rows: 1, Cols: 1, PageSize: PageFormat{Width: 842, Height: 595}})
	if err != nil {
		t.Fatal(err)
	}
	img, err := out.RenderImage(1, RenderOptions{DPI: 72})
	if err != nil {
		t.Fatal(err)
	}
	if !isColor(img, 805, 50, testRed) || isColor(img, 50, 50, testRed) {
		t.Error("rotated page not placed in displayed orientation")
	}
}

func TestPageStampDisjointCropBoxFallsBackToMediaBox(t *testing.T) {
	src := markDoc(t, 595, 842, topLeftMark, testRed)
	sp, _ := src.Page(1)
	if err := sp.SetCropBox(Rectangle{LLX: 1000, LLY: 1000, URX: 1200, URY: 1200}); err != nil {
		t.Fatal(err)
	}
	dst := NewDocument(100, 100)
	_, box, err := dst.importPageAsXObject(src, sp, map[int]int{})
	if err != nil {
		t.Fatal(err)
	}
	if box != (Rectangle{URX: 595, URY: 842}) {
		t.Errorf("box = %+v", box)
	}
}
