// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"compress/zlib"
	"image"
	"testing"
)

// TestInlineImagePNGPredictor checks that a FlateDecode inline image with a
// /DecodeParms PNG predictor is fully decoded — the predictor (and its
// per-row filter-type byte) must be reversed, not just the raw inflate.
// 33697-1.pdf tiles such images (dashed TOC leaders); skipping the predictor
// decoded them to garbage that rendered as solid lines.
func TestInlineImagePNGPredictor(t *testing.T) {
	// 2×2 DeviceRGB image, PNG filter type 0 (None) per row — so the only thing
	// the predictor step does is strip the leading filter-type byte. If it is
	// skipped, every sample shifts by one byte and the pixels are wrong.
	want := [][3]uint8{
		{255, 0, 0}, {0, 255, 0}, // row 0: red, green
		{0, 0, 255}, {255, 255, 255}, // row 1: blue, white
	}
	raw := []byte{
		0, 255, 0, 0, 0, 255, 0, // filter byte + row 0 samples
		0, 0, 0, 255, 255, 255, 255, // filter byte + row 1 samples
	}
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	zw.Write(raw)
	zw.Close()

	dict := pdfDict{
		"/Width":            2,
		"/Height":           2,
		"/BitsPerComponent": 8,
		"/ColorSpace":       pdfName("/DeviceRGB"),
		"/Filter":           pdfName("/FlateDecode"),
		"/DecodeParms": pdfDict{
			"/Predictor": 15,
			"/Columns":   2,
			"/Colors":    3,
		},
	}

	info, ok := inlineImageInfo(dict, buf.String(), identityMatrix())
	if !ok {
		t.Fatal("inlineImageInfo failed")
	}
	img, err := info.Extract()
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	m, _, err := image.Decode(bytes.NewReader(img.Data))
	if err != nil {
		t.Fatalf("decode PNG: %v", err)
	}

	b := m.Bounds()
	i := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := m.At(x, y).RGBA()
			got := [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8)}
			if got != want[i] {
				t.Errorf("pixel %d = %v, want %v (predictor not applied?)", i, got, want[i])
			}
			i++
		}
	}
}
