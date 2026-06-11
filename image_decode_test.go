// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestEncodePNGRGB(t *testing.T) {
	// 2x2 RGB image: red, green, blue, white
	pixels := []byte{
		255, 0, 0, 0, 255, 0,
		0, 0, 255, 255, 255, 255,
	}
	data, err := encodePNG(pixels, 2, 2, 8, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Verify it's a valid PNG.
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal("invalid PNG:", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != 2 || bounds.Dy() != 2 {
		t.Errorf("size=%dx%d, want 2x2", bounds.Dx(), bounds.Dy())
	}
	// Check top-left pixel is red.
	r, g, b, _ := img.At(0, 0).RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Errorf("pixel(0,0)=(%d,%d,%d), want (255,0,0)", r>>8, g>>8, b>>8)
	}
}

func TestEncodePNGGray(t *testing.T) {
	// 2x2 grayscale: black, dark gray, light gray, white
	pixels := []byte{0, 85, 170, 255}
	data, err := encodePNG(pixels, 2, 2, 8, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal("invalid PNG:", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != 2 || bounds.Dy() != 2 {
		t.Errorf("size=%dx%d, want 2x2", bounds.Dx(), bounds.Dy())
	}
}

func TestEncodePNGWithAlpha(t *testing.T) {
	// 2x1 RGB with soft mask (alpha)
	pixels := []byte{255, 0, 0, 0, 255, 0}
	alpha := []byte{255, 128}
	data, err := encodePNG(pixels, 2, 1, 8, 3, alpha)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal("invalid PNG:", err)
	}
	// Second pixel should have alpha=128.
	_, _, _, a := img.At(1, 0).RGBA()
	if a>>8 != 128 {
		t.Errorf("alpha(1,0)=%d, want 128", a>>8)
	}
}

func TestCMYKToRGB(t *testing.T) {
	// Conversion goes through the baked Adobe-profile LUT (adobeCMYKToRGB), so the
	// process colours match Acrobat, not the naive (1-C)(1-K) formula.
	eq := func(name string, cmyk []byte, wantR, wantG, wantB byte) {
		rgb := cmykToRGB(cmyk, 1)
		if rgb[0] != wantR || rgb[1] != wantG || rgb[2] != wantB {
			t.Errorf("%s → (%d,%d,%d), want (%d,%d,%d)", name, rgb[0], rgb[1], rgb[2], wantR, wantG, wantB)
		}
	}
	eq("cyan", []byte{255, 0, 0, 0}, 0, 173, 239)
	eq("black", []byte{0, 0, 0, 255}, 34, 31, 31)
	eq("white", []byte{0, 0, 0, 0}, 255, 255, 255)
}

func TestEncodePNGCMYK(t *testing.T) {
	// 1x1 CMYK pixel (pure magenta) → valid RGB PNG; the Adobe-profile LUT maps
	// process magenta to (236,0,139), not the naive (255,0,255).
	pixels := []byte{0, 255, 0, 0}
	data, err := encodePNG(pixels, 1, 1, 8, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal("invalid PNG:", err)
	}
	r, g, b, _ := img.At(0, 0).RGBA()
	if r>>8 != 236 || g>>8 != 0 || b>>8 != 139 {
		t.Errorf("magenta → (%d,%d,%d), want (236,0,139)", r>>8, g>>8, b>>8)
	}
}

func TestParseInlineImageDict(t *testing.T) {
	dict := pdfDict{
		"/W":   10,
		"/H":   5,
		"/BPC": 8,
		"/CS":  pdfName("/RGB"),
	}
	norm := normalizeInlineDict(dict)
	if dictGetInt(norm, "/Width") != 10 {
		t.Errorf("Width=%d, want 10", dictGetInt(norm, "/Width"))
	}
	if dictGetInt(norm, "/Height") != 5 {
		t.Errorf("Height=%d, want 5", dictGetInt(norm, "/Height"))
	}
	if dictGetInt(norm, "/BitsPerComponent") != 8 {
		t.Errorf("BPC=%d, want 8", dictGetInt(norm, "/BitsPerComponent"))
	}
	if dictGetName(norm, "/ColorSpace") != "/DeviceRGB" {
		t.Errorf("CS=%s, want /DeviceRGB", dictGetName(norm, "/ColorSpace"))
	}
}

func TestDecodeJPEGToPixels(t *testing.T) {
	// Create a tiny JPEG in memory.
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	img.SetNRGBA(1, 0, color.NRGBA{G: 255, A: 255})
	img.SetNRGBA(0, 1, color.NRGBA{B: 255, A: 255})
	img.SetNRGBA(1, 1, color.NRGBA{R: 255, G: 255, B: 255, A: 255})

	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100})

	pixels, w, h, err := decodeJPEGToPixels(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if w != 2 || h != 2 {
		t.Errorf("size=%dx%d, want 2x2", w, h)
	}
	if len(pixels) != 2*2*3 {
		t.Errorf("pixel count=%d, want %d", len(pixels), 2*2*3)
	}
}

func TestExpandIndexed(t *testing.T) {
	palette := []byte{255, 0, 0, 0, 255, 0, 0, 0, 255}
	indices := []byte{0, 1, 2, 0}
	rgb := expandIndexed(indices, palette, 3)
	expected := []byte{255, 0, 0, 0, 255, 0, 0, 0, 255, 255, 0, 0}
	if len(rgb) != len(expected) {
		t.Fatalf("len=%d, want %d", len(rgb), len(expected))
	}
	for i := range expected {
		if rgb[i] != expected[i] {
			t.Errorf("byte[%d]=%d, want %d", i, rgb[i], expected[i])
		}
	}
}

// TestColourKeyAlpha covers /Mask array colour-key masking (ISO 32000-1
// §8.9.6.4): samples inside the ranges become transparent, others opaque.
func TestColourKeyAlpha(t *testing.T) {
	// 8-bpc single-component (e.g. Indexed): 3 pixels, mask value 8.
	alpha := colourKeyAlpha([]byte{7, 8, 9}, 3, 1, 8, 1, pdfArray{8, 8})
	want := []byte{255, 0, 255}
	for i := range want {
		if alpha[i] != want[i] {
			t.Errorf("8bpc alpha[%d] = %d, want %d", i, alpha[i], want[i])
		}
	}
	// 8-bpc RGB: pixel masked only when all three components are in range.
	alpha = colourKeyAlpha([]byte{255, 0, 0, 255, 255, 255}, 2, 1, 8, 3,
		pdfArray{250, 255, 250, 255, 250, 255})
	if alpha[0] != 255 || alpha[1] != 0 {
		t.Errorf("rgb alpha = %v, want [255 0]", alpha[:2])
	}
	// 4-bpc: rows are byte-aligned; 3 pixels/row → 2 bytes per row.
	// Row 0 nibbles: 1,8,1 → masked,opaque,masked; row 1: 8,8,1 → opaque,opaque,masked.
	alpha = colourKeyAlpha([]byte{0x18, 0x10, 0x88, 0x10}, 3, 2, 4, 1, pdfArray{1, 1})
	want = []byte{0, 255, 0, 255, 255, 0}
	for i := range want {
		if alpha[i] != want[i] {
			t.Errorf("4bpc alpha[%d] = %d, want %d", i, alpha[i], want[i])
		}
	}
}
