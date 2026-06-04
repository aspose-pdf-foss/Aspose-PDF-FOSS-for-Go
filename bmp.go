// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bufio"
	"encoding/binary"
	"image"
	"io"
)

// encodeBMP writes m as an uncompressed 24-bit BMP (BGR, bottom-up rows padded
// to 4 bytes). The stdlib has no BMP encoder, so this is written in-house to
// keep the zero-dependency promise. Pixels with alpha < 255 are composited over
// white (BMP 24-bit has no alpha channel).
func encodeBMP(w io.Writer, m image.Image) error {
	b := m.Bounds()
	width, height := b.Dx(), b.Dy()
	rowSize := (width*3 + 3) &^ 3
	imageSize := rowSize * height
	const headerSize = 14 + 40
	fileSize := headerSize + imageSize

	bw := bufio.NewWriter(w)

	// BITMAPFILEHEADER (14 bytes).
	bw.WriteString("BM")
	putU32(bw, uint32(fileSize))
	putU32(bw, 0) // reserved
	putU32(bw, headerSize)

	// BITMAPINFOHEADER (40 bytes).
	putU32(bw, 40)
	putU32(bw, uint32(width))
	putU32(bw, uint32(height)) // positive → bottom-up
	putU16(bw, 1)              // planes
	putU16(bw, 24)             // bits per pixel
	putU32(bw, 0)              // BI_RGB, no compression
	putU32(bw, uint32(imageSize))
	putU32(bw, 2835) // ~72 DPI in pixels/metre
	putU32(bw, 2835)
	putU32(bw, 0)
	putU32(bw, 0)

	src := toNRGBA(m)
	row := make([]byte, rowSize)
	for y := height - 1; y >= 0; y-- { // bottom-up
		for i := range row {
			row[i] = 0
		}
		for x := 0; x < width; x++ {
			off := src.PixOffset(b.Min.X+x, b.Min.Y+y)
			r, g, bl, a := src.Pix[off], src.Pix[off+1], src.Pix[off+2], src.Pix[off+3]
			if a < 255 {
				af := float64(a) / 255
				r = uint8(float64(r)*af + 255*(1-af) + 0.5)
				g = uint8(float64(g)*af + 255*(1-af) + 0.5)
				bl = uint8(float64(bl)*af + 255*(1-af) + 0.5)
			}
			row[x*3+0] = bl // B
			row[x*3+1] = g  // G
			row[x*3+2] = r  // R
		}
		if _, err := bw.Write(row); err != nil {
			return err
		}
	}
	return bw.Flush()
}

func putU32(w io.Writer, v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	w.Write(b[:])
}

func putU16(w io.Writer, v uint16) {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], v)
	w.Write(b[:])
}

// RenderBMP renders the page and writes it as an uncompressed BMP.
func (p *Page) RenderBMP(w io.Writer, opts RenderOptions) error {
	img, err := p.RenderImage(opts)
	if err != nil {
		return err
	}
	return encodeBMP(w, img)
}

// BmpDevice renders a page to BMP. Mirrors Aspose.PDF for .NET's BmpDevice.
type BmpDevice struct{ res Resolution }

// NewBmpDevice returns a BmpDevice for the given resolution.
func NewBmpDevice(res Resolution) *BmpDevice { return &BmpDevice{res: res} }

// Process renders page and writes it to w as BMP.
func (d *BmpDevice) Process(page *Page, w io.Writer) error {
	return page.RenderBMP(w, d.res.options())
}
