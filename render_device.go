// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"image"
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
