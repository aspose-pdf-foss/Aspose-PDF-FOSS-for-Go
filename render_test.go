// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
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

func TestRenderEncodersDecode(t *testing.T) {
	doc := asposepdf.NewDocument(60, 40)
	p, _ := doc.Page(1)
	p.DrawRectangle(asposepdf.Rectangle{LLX: 0, LLY: 0, URX: 60, URY: 40},
		asposepdf.ShapeStyle{FillColor: &asposepdf.Color{R: 1, A: 1}})

	t.Run("png", func(t *testing.T) {
		var buf bytes.Buffer
		if err := p.RenderPNG(&buf, asposepdf.RenderOptions{DPI: 72}); err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(&buf)
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds().Dx() != 60 || img.Bounds().Dy() != 40 {
			t.Errorf("png bounds = %v", img.Bounds())
		}
		pixNear(t, img, 30, 20, 255, 0, 0, 2)
	})
	t.Run("jpeg", func(t *testing.T) {
		var buf bytes.Buffer
		if err := p.RenderJPEG(&buf, asposepdf.RenderOptions{DPI: 72}, 90); err != nil {
			t.Fatal(err)
		}
		img, err := jpeg.Decode(&buf)
		if err != nil {
			t.Fatal(err)
		}
		pixNear(t, img, 30, 20, 255, 0, 0, 20) // lossy → loose tolerance
	})
	t.Run("gif", func(t *testing.T) {
		var buf bytes.Buffer
		if err := p.RenderGIF(&buf, asposepdf.RenderOptions{DPI: 72}); err != nil {
			t.Fatal(err)
		}
		if _, err := gif.Decode(&buf); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("PngDevice", func(t *testing.T) {
		var buf bytes.Buffer
		dev := asposepdf.NewPngDevice(asposepdf.NewResolution(72))
		if err := dev.Process(p, &buf); err != nil {
			t.Fatal(err)
		}
		if _, err := png.Decode(&buf); err != nil {
			t.Fatal(err)
		}
	})
}

func TestRenderImageXObject(t *testing.T) {
	// An 8x8 solid green PNG.
	src := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for i := 0; i < len(src.Pix); i += 4 {
		src.Pix[i+0], src.Pix[i+1], src.Pix[i+2], src.Pix[i+3] = 0, 180, 0, 255
	}
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, src); err != nil {
		t.Fatal(err)
	}

	doc := asposepdf.NewDocument(100, 100)
	p, _ := doc.Page(1)
	if err := p.AddImageFromStream(&pngBuf, asposepdf.Rectangle{LLX: 20, LLY: 20, URX: 80, URY: 80}); err != nil {
		t.Fatalf("AddImageFromStream: %v", err)
	}
	img, err := p.RenderImage(asposepdf.RenderOptions{DPI: 72})
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}
	pixNear(t, img, 50, 50, 0, 180, 0, 6)   // image interior → green
	pixNear(t, img, 5, 5, 255, 255, 255, 1) // outside → white
}

func TestRenderBMP(t *testing.T) {
	doc := asposepdf.NewDocument(10, 10)
	p, _ := doc.Page(1)
	p.DrawRectangle(asposepdf.Rectangle{LLX: 0, LLY: 0, URX: 10, URY: 10},
		asposepdf.ShapeStyle{FillColor: &asposepdf.Color{R: 1, A: 1}}) // red page

	var buf bytes.Buffer
	if err := p.RenderBMP(&buf, asposepdf.RenderOptions{DPI: 72}); err != nil {
		t.Fatalf("RenderBMP: %v", err)
	}
	d := buf.Bytes()
	if len(d) < 54 || d[0] != 'B' || d[1] != 'M' {
		t.Fatalf("not a BMP (magic %q)", d[:2])
	}
	w := int(binary.LittleEndian.Uint32(d[18:22]))
	h := int(binary.LittleEndian.Uint32(d[22:26]))
	if w != 10 || h != 10 {
		t.Errorf("BMP size = %dx%d, want 10x10", w, h)
	}
	bpp := int(binary.LittleEndian.Uint16(d[28:30]))
	if bpp != 24 {
		t.Errorf("BMP bpp = %d, want 24", bpp)
	}
	// First pixel of pixel data (bottom-left) is red → BGR (0,0,255).
	px := d[54:]
	if px[0] != 0 || px[1] != 0 || px[2] != 255 {
		t.Errorf("first pixel BGR = (%d,%d,%d), want (0,0,255)", px[0], px[1], px[2])
	}
}

func TestRenderEmbeddedText(t *testing.T) {
	doc := asposepdf.NewDocument(220, 90)
	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatalf("LoadFont: %v", err)
	}
	p, _ := doc.Page(1)
	if err := p.AddText("Hello",
		asposepdf.TextStyle{Font: font, Size: 48, Color: &asposepdf.Color{A: 1}},
		asposepdf.Rectangle{LLX: 10, LLY: 20, URX: 210, URY: 80}); err != nil {
		t.Fatalf("AddText: %v", err)
	}
	img, err := p.RenderImage(asposepdf.RenderOptions{DPI: 72})
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}
	dark := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r>>8 < 100 && g>>8 < 100 && bl>>8 < 100 {
				dark++
			}
		}
	}
	if dark < 100 {
		t.Errorf("expected glyph pixels from embedded-font text, got only %d dark pixels", dark)
	}
}

func TestRenderStandard14Text(t *testing.T) {
	doc := asposepdf.NewDocument(240, 90)
	p, _ := doc.Page(1)
	if err := p.AddText("Hello",
		asposepdf.TextStyle{Font: asposepdf.FontHelvetica, Size: 48, Color: &asposepdf.Color{A: 1}},
		asposepdf.Rectangle{LLX: 10, LLY: 20, URX: 230, URY: 80}); err != nil {
		t.Fatalf("AddText: %v", err)
	}
	img, err := p.RenderImage(asposepdf.RenderOptions{DPI: 72})
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}
	dark := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r>>8 < 100 && g>>8 < 100 && bl>>8 < 100 {
				dark++
			}
		}
	}
	if dark < 100 {
		t.Errorf("Standard-14 Helvetica text produced only %d dark pixels (fallback not rendering?)", dark)
	}
}
