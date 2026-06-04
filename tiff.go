// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"fmt"
	"image"
	"io"
)

// The stdlib has no TIFF encoder (it lives in golang.org/x/image/tiff, an
// external module), so — as with the in-house BMP encoder — this writes baseline
// TIFF by hand to keep the zero-dependency promise. Output is little-endian
// ("II"), RGB 8-bit, one strip per page, optionally Deflate-compressed (tag 8,
// a zlib stream from compress/zlib). Multiple pages are chained as successive
// IFDs (image file directories) in one file — the canonical multi-page TIFF
// used for document archival, mirroring Aspose.PDF for .NET's TiffDevice.

// TIFF field types used here (ISO 12639 / TIFF 6.0 §2).
const (
	tiffShort    = 3 // 16-bit unsigned
	tiffLong     = 4 // 32-bit unsigned
	tiffRational = 5 // two LONGs: numerator, denominator
)

// One IFD here always carries this fixed set of 12 tags.
const (
	tiffNumTags = 12
	tiffIFDSize = 2 + tiffNumTags*12 + 4 // entry count + entries + next-IFD offset
)

// encodeTIFF writes count pages as a single (multi-page) baseline TIFF. Pages
// are produced on demand by get(i) so only one page image is held at a time.
// When compress is true each strip is a zlib stream (Compression = 8).
func encodeTIFF(w io.Writer, count int, dpi float64, compress bool, get func(i int) (image.Image, error)) error {
	if count <= 0 {
		return fmt.Errorf("tiff: no pages to encode")
	}
	bw := bufio.NewWriter(w)

	// Image file header: byte order "II", magic 42, offset of the first IFD.
	bw.WriteString("II")
	putU16(bw, 42)
	putU32(bw, 8)

	dpiNum := uint32(dpi + 0.5)
	if dpiNum == 0 {
		dpiNum = 1
	}
	comp := uint32(1) // none
	if compress {
		comp = 8 // Deflate (zlib)
	}

	off := uint32(8) // running offset of the current IFD
	for i := 0; i < count; i++ {
		img, err := get(i)
		if err != nil {
			return fmt.Errorf("tiff: render page %d: %w", i+1, err)
		}
		strip, wd, ht := tiffStrip(img, compress)
		stripLen := uint32(len(strip))
		padded := stripLen
		if padded%2 == 1 {
			padded++ // values/IFDs must start on a word boundary
		}

		// Out-of-line values follow the IFD: BitsPerSample (3×short), then the
		// two RATIONALs, then the strip.
		bitsOffset := off + tiffIFDSize
		xresOffset := bitsOffset + 6
		yresOffset := xresOffset + 8
		stripOffset := yresOffset + 8
		blockLen := uint32(tiffIFDSize) + 6 + 8 + 8 + padded

		var nextIFD uint32
		if i < count-1 {
			nextIFD = off + blockLen
		}

		// IFD entries — MUST be written in ascending tag order.
		putU16(bw, tiffNumTags)
		tiffEntry(bw, 256, tiffLong, 1, uint32(wd))     // ImageWidth
		tiffEntry(bw, 257, tiffLong, 1, uint32(ht))     // ImageLength
		tiffEntry(bw, 258, tiffShort, 3, bitsOffset)    // BitsPerSample → 8,8,8
		tiffEntry(bw, 259, tiffShort, 1, comp)          // Compression
		tiffEntry(bw, 262, tiffShort, 1, 2)             // Photometric = RGB
		tiffEntry(bw, 273, tiffLong, 1, stripOffset)    // StripOffsets
		tiffEntry(bw, 277, tiffShort, 1, 3)             // SamplesPerPixel
		tiffEntry(bw, 278, tiffLong, 1, uint32(ht))     // RowsPerStrip = height
		tiffEntry(bw, 279, tiffLong, 1, stripLen)       // StripByteCounts
		tiffEntry(bw, 282, tiffRational, 1, xresOffset) // XResolution
		tiffEntry(bw, 283, tiffRational, 1, yresOffset) // YResolution
		tiffEntry(bw, 296, tiffShort, 1, 2)             // ResolutionUnit = inch
		putU32(bw, nextIFD)

		// Out-of-line values, in the offset order computed above.
		putU16(bw, 8)
		putU16(bw, 8)
		putU16(bw, 8)
		putU32(bw, dpiNum)
		putU32(bw, 1)
		putU32(bw, dpiNum)
		putU32(bw, 1)

		if _, err := bw.Write(strip); err != nil {
			return err
		}
		if padded != stripLen {
			bw.Write([]byte{0})
		}
		off += blockLen
	}
	return bw.Flush()
}

// tiffEntry writes one 12-byte IFD entry. For inline SHORT/LONG values the
// value occupies the low bytes of the 4-byte field (little-endian places it
// first, exactly as the spec requires); for out-of-line types value is an offset.
func tiffEntry(w io.Writer, tag, typ uint16, count, value uint32) {
	putU16(w, tag)
	putU16(w, typ)
	putU32(w, count)
	putU32(w, value)
}

// tiffStrip builds one image's RGB pixel strip (top-to-bottom rows, alpha
// composited over white since baseline RGB TIFF has no alpha), optionally
// zlib-compressed, and returns it with the pixel dimensions.
func tiffStrip(m image.Image, compress bool) ([]byte, int, int) {
	b := m.Bounds()
	wd, ht := b.Dx(), b.Dy()
	src := toNRGBA(m)
	raw := make([]byte, 0, wd*ht*3)
	for y := 0; y < ht; y++ {
		for x := 0; x < wd; x++ {
			off := src.PixOffset(b.Min.X+x, b.Min.Y+y)
			r, g, bl, a := src.Pix[off], src.Pix[off+1], src.Pix[off+2], src.Pix[off+3]
			if a < 255 {
				af := float64(a) / 255
				r = uint8(float64(r)*af + 255*(1-af) + 0.5)
				g = uint8(float64(g)*af + 255*(1-af) + 0.5)
				bl = uint8(float64(bl)*af + 255*(1-af) + 0.5)
			}
			raw = append(raw, r, g, bl)
		}
	}
	if !compress {
		return raw, wd, ht
	}
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	zw.Write(raw)
	zw.Close()
	return buf.Bytes(), wd, ht
}

// RenderTIFF renders the page and writes it as a single-page Deflate TIFF.
func (p *Page) RenderTIFF(w io.Writer, opts RenderOptions) error {
	img, err := p.RenderImage(opts)
	if err != nil {
		return err
	}
	return encodeTIFF(w, 1, opts.dpi(), true, func(int) (image.Image, error) { return img, nil })
}

// TiffDevice renders pages to TIFF. Mirrors Aspose.PDF for .NET's TiffDevice:
// Process writes one page, ProcessDocument writes a whole document as a single
// multi-page TIFF.
type TiffDevice struct{ res Resolution }

// NewTiffDevice returns a TiffDevice for the given resolution.
func NewTiffDevice(res Resolution) *TiffDevice { return &TiffDevice{res: res} }

// Process renders page and writes it to w as a single-page TIFF.
func (d *TiffDevice) Process(page *Page, w io.Writer) error {
	return page.RenderTIFF(w, d.res.options())
}

// ProcessDocument renders every page of doc into one multi-page TIFF.
func (d *TiffDevice) ProcessDocument(doc *Document, w io.Writer) error {
	return doc.RenderTIFF(w, d.res.options())
}
