// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
)

// DefaultDPI is the rendering resolution used when RenderOptions.DPI is unset.
// 150 matches Aspose.PDF for .NET's default Resolution.
const DefaultDPI = 150

// RenderOptions controls page rasterization.
type RenderOptions struct {
	DPI        float64 // dots per inch; 0 → DefaultDPI
	Background *Color  // page backdrop; nil → opaque white
}

func (o RenderOptions) dpi() float64 {
	if o.DPI <= 0 {
		return DefaultDPI
	}
	return o.DPI
}

// RenderImage rasterizes the page to an *image.RGBA at the requested DPI.
// The rendered region is the CropBox (falling back to MediaBox). Content the
// renderer does not yet support is skipped rather than erroring, so a page
// always produces an image.
func (p *Page) RenderImage(opts RenderOptions) (image.Image, error) {
	box, err := p.CropBox()
	if err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}
	scale := opts.dpi() / 72.0
	base, w, h := deviceMatrix(box, scale, p.Rotation())
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("render: degenerate page size %dx%d", w, h)
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fillBackground(img, opts.Background)

	rd := newRenderer(p, img, w, h, base)
	rd.run()
	return img, nil
}

// RenderPNG renders the page and writes it as PNG.
func (p *Page) RenderPNG(w io.Writer, opts RenderOptions) error {
	img, err := p.RenderImage(opts)
	if err != nil {
		return err
	}
	return png.Encode(w, img)
}

// RenderJPEG renders the page and writes it as JPEG. quality is 1..100;
// values outside that range use 90.
func (p *Page) RenderJPEG(w io.Writer, opts RenderOptions, quality int) error {
	img, err := p.RenderImage(opts)
	if err != nil {
		return err
	}
	if quality < 1 || quality > 100 {
		quality = 90
	}
	return jpeg.Encode(w, img, &jpeg.Options{Quality: quality})
}

// RenderGIF renders the page and writes it as GIF (256 colours, quantized).
func (p *Page) RenderGIF(w io.Writer, opts RenderOptions) error {
	img, err := p.RenderImage(opts)
	if err != nil {
		return err
	}
	return gif.Encode(w, img, &gif.Options{NumColors: 256})
}

// RenderImage renders the 1-based page number to an image.
func (d *Document) RenderImage(pageNum int, opts RenderOptions) (image.Image, error) {
	p, err := d.Page(pageNum)
	if err != nil {
		return nil, err
	}
	return p.RenderImage(opts)
}

// Resolution is a rendering resolution in DPI. Mirrors Aspose.PDF for .NET's
// Aspose.Pdf.Devices.Resolution.
type Resolution struct{ DPI float64 }

// NewResolution returns a Resolution of the given DPI.
func NewResolution(dpi float64) Resolution { return Resolution{DPI: dpi} }

func (r Resolution) options() RenderOptions { return RenderOptions{DPI: r.DPI} }

// PngDevice renders a page to PNG at a fixed resolution. Mirrors Aspose.PDF for
// .NET's PngDevice.
type PngDevice struct{ res Resolution }

// NewPngDevice returns a PngDevice for the given resolution.
func NewPngDevice(res Resolution) *PngDevice { return &PngDevice{res: res} }

// Process renders page and writes it to w as PNG.
func (d *PngDevice) Process(page *Page, w io.Writer) error {
	return page.RenderPNG(w, d.res.options())
}

// JpegDevice renders a page to JPEG. Mirrors Aspose.PDF for .NET's JpegDevice.
type JpegDevice struct {
	res     Resolution
	quality int
}

// NewJpegDevice returns a JpegDevice; quality outside 1..100 defaults to 90.
func NewJpegDevice(res Resolution, quality int) *JpegDevice {
	return &JpegDevice{res: res, quality: quality}
}

// Process renders page and writes it to w as JPEG.
func (d *JpegDevice) Process(page *Page, w io.Writer) error {
	return page.RenderJPEG(w, d.res.options(), d.quality)
}

// GifDevice renders a page to GIF. Mirrors Aspose.PDF for .NET's GifDevice.
type GifDevice struct{ res Resolution }

// NewGifDevice returns a GifDevice for the given resolution.
func NewGifDevice(res Resolution) *GifDevice { return &GifDevice{res: res} }

// Process renders page and writes it to w as GIF.
func (d *GifDevice) Process(page *Page, w io.Writer) error {
	return page.RenderGIF(w, d.res.options())
}

// deviceMatrix builds the matrix mapping PDF user space to device pixels
// (origin top-left, Y down) for the given crop box, scale, and page rotation,
// and returns the pixel dimensions.
func deviceMatrix(box Rectangle, s float64, rot RotationAngle) (m [6]float64, w, h int) {
	bw := (box.URX - box.LLX) * s
	bh := (box.URY - box.LLY) * s

	switch normalizeRotation(rot) {
	case Rotate90:
		m = [6]float64{0, s, s, 0, -box.LLY * s, -box.LLX * s}
		w, h = roundPx(bh), roundPx(bw)
	case Rotate180:
		m = [6]float64{-s, 0, 0, s, box.URX * s, -box.LLY * s}
		w, h = roundPx(bw), roundPx(bh)
	case Rotate270:
		m = [6]float64{0, -s, -s, 0, box.URY * s, box.URX * s}
		w, h = roundPx(bh), roundPx(bw)
	default: // Rotate0
		m = [6]float64{s, 0, 0, -s, -box.LLX * s, box.URY * s}
		w, h = roundPx(bw), roundPx(bh)
	}
	return m, w, h
}

func roundPx(v float64) int { return int(math.Round(v)) }

func normalizeRotation(r RotationAngle) RotationAngle {
	n := int(r) % 360
	if n < 0 {
		n += 360
	}
	return RotationAngle(n)
}

// fillBackground paints the whole image with bg (opaque white when nil).
func fillBackground(img *image.RGBA, bg *Color) {
	r, g, b := uint8(255), uint8(255), uint8(255)
	if bg != nil {
		r, g, b = colorToRGB8(*bg)
	}
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i+0] = r
		img.Pix[i+1] = g
		img.Pix[i+2] = b
		img.Pix[i+3] = 255
	}
}

// applyPt transforms (x,y) by a PDF [a b c d e f] matrix.
func applyPt(m [6]float64, x, y float64) (float64, float64) {
	return m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]
}

func colorToRGB8(c Color) (uint8, uint8, uint8) {
	return clamp8(c.R), clamp8(c.G), clamp8(c.B)
}

func clamp8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return uint8(v*255 + 0.5)
}

func cmykToRGB8(c, m, y, k float64) (uint8, uint8, uint8) {
	return clamp8((1 - c) * (1 - k)), clamp8((1 - m) * (1 - k)), clamp8((1 - y) * (1 - k))
}
