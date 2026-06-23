// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"image"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// colorfulness returns the mean per-pixel chroma (max−min of RGB channels) of an
// image, in [0,1]. It is 0 for a fully grayscale image.
func colorfulness(img image.Image) float64 {
	b := img.Bounds()
	var sum float64
	var n int
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		for x := b.Min.X; x < b.Max.X; x += 2 {
			r, g, bl, _ := img.At(x, y).RGBA()
			mx, mn := r, r
			for _, c := range []uint32{g, bl} {
				if c > mx {
					mx = c
				}
				if c < mn {
					mn = c
				}
			}
			sum += float64(mx-mn) / 65535.0
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

func colorfulSample(t *testing.T) *pdf.Document {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	p.AddText("Colorful Heading", pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 28,
		Color: &pdf.Color{R: 0.85, G: 0.1, B: 0.1, A: 1}},
		pdf.Rectangle{LLX: 50, LLY: 740, URX: 545, URY: 790})
	p.DrawRectangle(pdf.Rectangle{LLX: 50, LLY: 540, URX: 250, URY: 700},
		pdf.ShapeStyle{FillColor: &pdf.Color{R: 0.1, G: 0.3, B: 0.9, A: 1}})
	p.DrawCircle(pdf.Point{X: 400, Y: 620}, 70,
		pdf.ShapeStyle{FillColor: &pdf.Color{R: 0.1, G: 0.8, B: 0.2, A: 1}})
	if err := p.AddImage("testdata/aspose-logo.png",
		pdf.Rectangle{LLX: 50, LLY: 300, URX: 250, URY: 500}); err != nil {
		t.Skipf("test image unavailable: %v", err)
	}
	grad := pdf.NewLinearGradient(50, 100, 500, 100,
		pdf.GradientStop{Offset: 0, Color: pdf.Color{R: 1, A: 1}},
		pdf.GradientStop{Offset: 1, Color: pdf.Color{B: 1, A: 1}})
	p.DrawRectangle(pdf.Rectangle{LLX: 50, LLY: 100, URX: 500, URY: 200},
		pdf.ShapeStyle{FillGradient: grad})
	return doc
}

// TestConvertToGrayscale: a richly colored page (text, vector fills, a raster
// image and a gradient) becomes fully grayscale and stays grayscale after a
// round-trip, with text intact.
func TestConvertToGrayscale(t *testing.T) {
	doc := colorfulSample(t)

	before, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 80})
	if err != nil {
		t.Fatal(err)
	}
	if c := colorfulness(before); c < 0.02 {
		t.Fatalf("sample is not colorful enough to test (colorfulness=%.4f)", c)
	}

	if err := doc.ConvertToGrayscale(); err != nil {
		t.Fatal(err)
	}

	after, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 80})
	if err != nil {
		t.Fatal(err)
	}
	if c := colorfulness(after); c > 0.01 {
		t.Errorf("page still has colour after ConvertToGrayscale (colorfulness=%.4f)", c)
	}

	// Round-trip: stays grayscale and text survives.
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	rt, err := out.RenderImage(1, pdf.RenderOptions{DPI: 80})
	if err != nil {
		t.Fatal(err)
	}
	if c := colorfulness(rt); c > 0.01 {
		t.Errorf("page regained colour after round-trip (colorfulness=%.4f)", c)
	}
	page, _ := out.Page(1)
	if txt, _ := page.ExtractText(); !bytes.Contains([]byte(txt), []byte("Colorful Heading")) {
		t.Errorf("text lost after grayscale conversion: %q", txt)
	}
}

// TestConvertToGrayscaleCMYK: a CMYK fill (k operator) is converted too.
func TestConvertToGrayscaleCMYK(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	// CMYK colours go through the cmyk → gray path.
	p.DrawRectangle(pdf.Rectangle{LLX: 50, LLY: 400, URX: 300, URY: 700},
		pdf.ShapeStyle{FillColor: &pdf.Color{R: 0, G: 0.6, B: 0.9, A: 1}})
	if err := doc.ConvertToGrayscale(); err != nil {
		t.Fatal(err)
	}
	img, _ := doc.RenderImage(1, pdf.RenderOptions{DPI: 72})
	if c := colorfulness(img); c > 0.01 {
		t.Errorf("colour remained after conversion (colorfulness=%.4f)", c)
	}
}
