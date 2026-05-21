// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
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

func TestImageToDocumentJPEG(t *testing.T) {
	doc, err := ImageToDocument("testdata/Koala.jpg")
	if err != nil {
		t.Fatalf("ImageToDocument: %v", err)
	}
	if doc.PageCount() != 1 {
		t.Fatalf("pages = %d, want 1", doc.PageCount())
	}

	page, _ := doc.Page(1)
	size, err := page.Size()
	if err != nil {
		t.Fatalf("Size: %v", err)
	}
	// Page dimensions should be > 0.
	if size.Width <= 0 || size.Height <= 0 {
		t.Errorf("invalid page size: %.1fx%.1f", size.Width, size.Height)
	}

	// Should be able to save and reopen.
	outPath := t.TempDir() + "/koala.pdf"
	if err := doc.Save(outPath); err != nil {
		t.Fatalf("Save: %v", err)
	}
	reopened, err := Open(outPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.PageCount() != 1 {
		t.Errorf("reopened pages = %d, want 1", reopened.PageCount())
	}
}

func TestImageToDocumentPNG(t *testing.T) {
	doc, err := ImageToDocument("testdata/Penguins.png")
	if err != nil {
		t.Fatalf("ImageToDocument: %v", err)
	}
	if doc.PageCount() != 1 {
		t.Fatalf("pages = %d, want 1", doc.PageCount())
	}

	outPath := t.TempDir() + "/penguins.pdf"
	if err := doc.Save(outPath); err != nil {
		t.Fatalf("Save: %v", err)
	}
	reopened, err := Open(outPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.PageCount() != 1 {
		t.Errorf("reopened pages = %d, want 1", reopened.PageCount())
	}
}

func TestImageToDocumentWithOptions(t *testing.T) {
	doc, err := ImageToDocument("testdata/aspose-logo.png", ImageToDocumentOptions{
		PageWidth:    595, // A4
		PageHeight:   842,
		MarginLeft:   36,
		MarginRight:  36,
		MarginTop:    36,
		MarginBottom: 36,
	})
	if err != nil {
		t.Fatalf("ImageToDocument: %v", err)
	}

	page, _ := doc.Page(1)
	size, _ := page.Size()
	if size.Width < 594 || size.Width > 596 {
		t.Errorf("page width = %.1f, want ~595", size.Width)
	}
	if size.Height < 841 || size.Height > 843 {
		t.Errorf("page height = %.1f, want ~842", size.Height)
	}

	outPath := t.TempDir() + "/logo.pdf"
	if err := doc.Save(outPath); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

func TestImageToDocumentMarginsExceedPage(t *testing.T) {
	_, err := ImageToDocument("testdata/aspose-logo.png", ImageToDocumentOptions{
		PageWidth:   100,
		PageHeight:  100,
		MarginLeft:  60,
		MarginRight: 60,
	})
	if err == nil {
		t.Fatal("expected error when margins exceed page dimensions")
	}
}

func TestImageToDocumentFromStream(t *testing.T) {
	f, err := os.Open("testdata/Koala.jpg")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	doc, err := ImageToDocumentFromStream(f)
	if err != nil {
		t.Fatalf("ImageToDocumentFromStream: %v", err)
	}
	if doc.PageCount() != 1 {
		t.Fatalf("pages = %d, want 1", doc.PageCount())
	}
}
