package asposepdf

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestParseJPEGDPI(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want float64
	}{
		{
			"JFIF 300 DPI",
			buildJFIF(1, 300, 300), // units=1 (DPI), 300x300
			300,
		},
		{
			"JFIF dots/cm 118",
			buildJFIF(2, 118, 118), // units=2 (dots/cm), 118 ≈ 300 DPI
			118 * 2.54,
		},
		{
			"JFIF no units",
			buildJFIF(0, 72, 72), // units=0 (aspect ratio)
			72,
		},
		{
			"no JFIF marker",
			[]byte{0xFF, 0xD8, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x0A, 0x00, 0x0A, 0x03, 0x01, 0x22, 0x00, 0x02, 0x11, 0x01, 0x03, 0x11, 0x01, 0xFF, 0xD9},
			72,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseJPEGDPI(tt.data)
			if got < tt.want-0.1 || got > tt.want+0.1 {
				t.Errorf("got %.2f, want %.2f", got, tt.want)
			}
		})
	}
}

func TestParsePNGDPI(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want float64
	}{
		{
			"pHYs 300 DPI",
			buildPNGWithPHYs(11811, 11811, 1), // 11811 ppm ≈ 300 DPI
			float64(11811) / 39.3701,
		},
		{
			"pHYs unknown units",
			buildPNGWithPHYs(72, 72, 0),
			72,
		},
		{
			"no pHYs",
			buildPNGNoPHYs(),
			72,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePNGDPI(tt.data)
			if got < tt.want-0.1 || got > tt.want+0.1 {
				t.Errorf("got %.2f, want %.2f", got, tt.want)
			}
		})
	}
}

// buildJFIF creates minimal JPEG data with a JFIF APP0 marker.
func buildJFIF(units byte, xDensity, yDensity uint16) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0xFF, 0xD8}) // SOI
	buf.Write([]byte{0xFF, 0xE0}) // APP0
	// Length = 16 (standard JFIF)
	buf.Write([]byte{0x00, 0x10})
	buf.WriteString("JFIF\x00")   // identifier
	buf.Write([]byte{0x01, 0x01}) // version 1.1
	buf.WriteByte(units)
	binary.Write(&buf, binary.BigEndian, xDensity)
	binary.Write(&buf, binary.BigEndian, yDensity)
	buf.Write([]byte{0x00, 0x00}) // thumbnail 0x0
	// Add SOF0 so it's a valid-ish JPEG.
	buf.Write([]byte{0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x0A, 0x00, 0x0A, 0x03, 0x01, 0x22, 0x00, 0x02, 0x11, 0x01, 0x03, 0x11, 0x01})
	buf.Write([]byte{0xFF, 0xD9}) // EOI
	return buf.Bytes()
}

// buildPNGWithPHYs creates minimal PNG bytes containing a pHYs chunk.
func buildPNGWithPHYs(ppuX, ppuY uint32, unit byte) []byte {
	// Create real PNG, then inject pHYs chunk before IDAT.
	img := imageRGB(1, 1)
	var pngBuf bytes.Buffer
	pngEncode(&pngBuf, img)
	pngData := pngBuf.Bytes()

	// pHYs chunk: 4 bytes X, 4 bytes Y, 1 byte unit = 9 data bytes.
	var phys bytes.Buffer
	binary.Write(&phys, binary.BigEndian, ppuX)
	binary.Write(&phys, binary.BigEndian, ppuY)
	phys.WriteByte(unit)

	chunk := buildPNGChunk("pHYs", phys.Bytes())

	// Insert pHYs after the IHDR chunk (8 byte signature + IHDR chunk).
	// IHDR is always first: 8 (sig) + 4 (len) + 4 (type) + 13 (data) + 4 (crc) = 33.
	ihdrEnd := 33
	var result bytes.Buffer
	result.Write(pngData[:ihdrEnd])
	result.Write(chunk)
	result.Write(pngData[ihdrEnd:])
	return result.Bytes()
}

// buildPNGNoPHYs creates a minimal PNG without pHYs.
func buildPNGNoPHYs() []byte {
	img := imageRGB(1, 1)
	var buf bytes.Buffer
	pngEncode(&buf, img)
	return buf.Bytes()
}

// buildPNGChunk creates a PNG chunk with the given type and data.
func buildPNGChunk(chunkType string, data []byte) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, uint32(len(data)))
	buf.WriteString(chunkType)
	buf.Write(data)
	// CRC over type+data.
	crc := crc32.ChecksumIEEE(append([]byte(chunkType), data...))
	binary.Write(&buf, binary.BigEndian, crc)
	return buf.Bytes()
}

// imageRGB returns a 1x1 opaque NRGBA image (fully opaque, no alpha channel needed).
func imageRGB(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 128, B: 0, A: 255})
		}
	}
	return img
}

// pngEncode encodes img as PNG into buf.
func pngEncode(buf *bytes.Buffer, img image.Image) {
	png.Encode(buf, img)
}
