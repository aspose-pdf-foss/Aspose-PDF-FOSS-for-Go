package asposepdf

import (
	"image"
	"testing"
)

func TestRawPixelsToImageRGB(t *testing.T) {
	// 2x2 red/green/blue/white image.
	pixels := []byte{
		255, 0, 0, 0, 255, 0,
		0, 0, 255, 255, 255, 255,
	}
	img := rawPixelsToImage(pixels, 2, 2, "/DeviceRGB")
	if img == nil {
		t.Fatal("expected non-nil image")
	}
	bounds := img.Bounds()
	if bounds.Dx() != 2 || bounds.Dy() != 2 {
		t.Fatalf("bounds = %v, want 2x2", bounds)
	}
	r, g, b, a := img.At(0, 0).RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 0 || a>>8 != 255 {
		t.Errorf("pixel (0,0) = (%d,%d,%d,%d), want red", r>>8, g>>8, b>>8, a>>8)
	}
}

func TestRawPixelsToImageGray(t *testing.T) {
	pixels := []byte{0, 128, 255, 64}
	img := rawPixelsToImage(pixels, 2, 2, "/DeviceGray")
	if img == nil {
		t.Fatal("expected non-nil image")
	}
	g := img.(*image.Gray)
	if g.GrayAt(0, 0).Y != 0 {
		t.Errorf("pixel (0,0) = %d, want 0", g.GrayAt(0, 0).Y)
	}
	if g.GrayAt(1, 0).Y != 128 {
		t.Errorf("pixel (1,0) = %d, want 128", g.GrayAt(1, 0).Y)
	}
}

func TestRawPixelsToImageUnsupported(t *testing.T) {
	img := rawPixelsToImage([]byte{0, 0, 0, 0}, 1, 1, "/DeviceCMYK")
	if img != nil {
		t.Error("expected nil for unsupported color space")
	}
}
