package asposepdf

import (
	"bytes"
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
	// Pure cyan: C=255, M=0, Y=0, K=0 → R=0, G=255, B=255
	cmyk := []byte{255, 0, 0, 0}
	rgb := cmykToRGB(cmyk, 1)
	if rgb[0] != 0 || rgb[1] != 255 || rgb[2] != 255 {
		t.Errorf("cyan → (%d,%d,%d), want (0,255,255)", rgb[0], rgb[1], rgb[2])
	}

	// Pure black: C=0, M=0, Y=0, K=255 → R=0, G=0, B=0
	cmyk = []byte{0, 0, 0, 255}
	rgb = cmykToRGB(cmyk, 1)
	if rgb[0] != 0 || rgb[1] != 0 || rgb[2] != 0 {
		t.Errorf("black → (%d,%d,%d), want (0,0,0)", rgb[0], rgb[1], rgb[2])
	}

	// White: C=0, M=0, Y=0, K=0 → R=255, G=255, B=255
	cmyk = []byte{0, 0, 0, 0}
	rgb = cmykToRGB(cmyk, 1)
	if rgb[0] != 255 || rgb[1] != 255 || rgb[2] != 255 {
		t.Errorf("white → (%d,%d,%d), want (255,255,255)", rgb[0], rgb[1], rgb[2])
	}
}

func TestEncodePNGCMYK(t *testing.T) {
	// 1x1 CMYK pixel (pure magenta) → should produce valid RGB PNG
	pixels := []byte{0, 255, 0, 0} // C=0, M=255, Y=0, K=0 → R=255, G=0, B=255
	data, err := encodePNG(pixels, 1, 1, 8, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal("invalid PNG:", err)
	}
	r, g, b, _ := img.At(0, 0).RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 255 {
		t.Errorf("magenta → (%d,%d,%d), want (255,0,255)", r>>8, g>>8, b>>8)
	}
}

