// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
)

// Graphical (pixel) document comparison — phase 2 of the comparison epic
// (pdf-go-175w / pdf-go-dyu9). Mirrors Aspose.PDF for .NET's
// Aspose.Pdf.Comparison.GraphicalComparison namespace (GraphicalPdfComparer,
// ImagesDifference): renders both pages with the built-in rasterizer and
// compares pixels, for changes the word-level text comparer in comparison.go
// can't see — moved or edited graphics, font substitution, a scanned-image
// replacement. For text content, ComparePages / CompareDocumentsPageByPage /
// CompareFlatDocuments are far cheaper and locate differences precisely by
// word; use this comparer only for graphical changes those can't see.

// GraphicalPdfComparer compares PDF pages/documents pixel by pixel. Mirrors
// Aspose.PDF for .NET's GraphicalPdfComparer. Construct with
// NewGraphicalPdfComparer, or use the zero value (equivalent defaults).
type GraphicalPdfComparer struct {
	// Threshold is the per-pixel colour-distance percentage (0-100) below
	// which a difference is ignored — a way to ignore small changes (JPEG
	// recompression noise, anti-aliasing). The zero value compares exactly.
	// Mirrors GraphicalPdfComparer.Threshold.
	Threshold float64
	// Color marks differing pixels in the default DifferenceToImage call and
	// in CompareDocumentsToPdf/ComparePagesToImage output. The zero value is
	// red. Mirrors GraphicalPdfComparer.Color.
	Color Color
	// Resolution is the DPI both pages are rasterized at before comparing.
	// The zero value is 150 DPI. Mirrors GraphicalPdfComparer.Resolution.
	Resolution Resolution
}

// NewGraphicalPdfComparer returns a comparer with Aspose's defaults: exact
// comparison, red highlight, 150 DPI.
func NewGraphicalPdfComparer() *GraphicalPdfComparer {
	return &GraphicalPdfComparer{Color: Color{R: 1, A: 1}, Resolution: Resolution{DPI: 150}}
}

func (c *GraphicalPdfComparer) dpi() float64 {
	if c.Resolution.DPI <= 0 {
		return 150
	}
	return c.Resolution.DPI
}

func (c *GraphicalPdfComparer) color() Color {
	if c.Color == (Color{}) {
		return Color{R: 1, A: 1}
	}
	return c.Color
}

// GetDifference rasterizes both pages at Resolution and compares them pixel
// by pixel. Mirrors GraphicalPdfComparer.GetDifference.
func (c *GraphicalPdfComparer) GetDifference(page1, page2 *Page) (*ImagesDifference, error) {
	if page1 == nil || page2 == nil {
		return nil, errors.New("GraphicalPdfComparer.GetDifference: nil page")
	}
	opts := RenderOptions{DPI: c.dpi()}
	img1, err := page1.RenderImage(opts)
	if err != nil {
		return nil, fmt.Errorf("GetDifference: render page1: %w", err)
	}
	img2, err := page2.RenderImage(opts)
	if err != nil {
		return nil, fmt.Errorf("GetDifference: render page2: %w", err)
	}
	return newImagesDifference(img1, img2, c.Threshold), nil
}

// ComparePagesToImage renders the two pages' difference (DifferenceToImage
// with this comparer's Color as foreground, white as background) and saves
// it as a PNG. Mirrors GraphicalPdfComparer.ComparePagesToImage.
func (c *GraphicalPdfComparer) ComparePagesToImage(page1, page2 *Page, path string) error {
	var buf bytes.Buffer
	if err := c.WritePagesToImage(page1, page2, &buf); err != nil {
		return err
	}
	return writeFile(path, buf.Bytes())
}

// WritePagesToImage is the io.Writer variant of ComparePagesToImage (PNG).
func (c *GraphicalPdfComparer) WritePagesToImage(page1, page2 *Page, w io.Writer) error {
	diff, err := c.GetDifference(page1, page2)
	if err != nil {
		return err
	}
	img := diff.DifferenceToImage(c.color(), Color{R: 1, G: 1, B: 1, A: 1})
	return png.Encode(w, img)
}

// CompareDocumentsToPdf compares every page pair by index (mirroring
// CompareDocumentsPageByPage's pairing; the longer document's tail pages are
// compared against a blank page of the same size, so a page added or removed
// still reports 100% different rather than erroring) and writes a new PDF
// with one page per pair: the difference image full-page, captioned with the
// changed-pixel ratio. Mirrors GraphicalPdfComparer.CompareDocumentsToPdf.
func (c *GraphicalPdfComparer) CompareDocumentsToPdf(doc1, doc2 *Document, outputPath string) error {
	var buf bytes.Buffer
	if err := c.WriteDocumentsToPdf(doc1, doc2, &buf); err != nil {
		return err
	}
	return writeFile(outputPath, buf.Bytes())
}

// WriteDocumentsToPdf is the io.Writer variant of CompareDocumentsToPdf.
func (c *GraphicalPdfComparer) WriteDocumentsToPdf(doc1, doc2 *Document, w io.Writer) error {
	if doc1 == nil || doc2 == nil {
		return errors.New("WriteDocumentsToPdf: nil document")
	}
	count := doc1.PageCount()
	if doc2.PageCount() > count {
		count = doc2.PageCount()
	}
	if count == 0 {
		return errors.New("WriteDocumentsToPdf: both documents have no pages")
	}

	out := NewDocumentFromFormat(PageFormatA4)
	for i := 1; i <= count; i++ {
		diff, err := c.pairDifference(doc1, doc2, i)
		if err != nil {
			return fmt.Errorf("WriteDocumentsToPdf: page %d: %w", i, err)
		}
		var page *Page
		if i == 1 {
			page, err = out.Page(1)
		} else {
			if err = out.AddBlankPageFromFormat(PageFormatA4); err == nil {
				page, err = out.Page(out.PageCount())
			}
		}
		if err != nil {
			return err
		}
		if err := drawDifferenceReportPage(page, diff, i, c.color()); err != nil {
			return err
		}
	}
	_, err := out.WriteTo(w)
	return err
}

// pairDifference renders page pageNum of each document (RenderImage) and
// compares them; a document missing that page contributes a blank white
// canvas the size of the other side's render, so a wholly added/removed page
// reports fully different instead of erroring.
func (c *GraphicalPdfComparer) pairDifference(doc1, doc2 *Document, pageNum int) (*ImagesDifference, error) {
	opts := RenderOptions{DPI: c.dpi()}
	var img1, img2 image.Image
	if pageNum <= doc1.PageCount() {
		p, err := doc1.Page(pageNum)
		if err != nil {
			return nil, err
		}
		if img1, err = p.RenderImage(opts); err != nil {
			return nil, err
		}
	}
	if pageNum <= doc2.PageCount() {
		p, err := doc2.Page(pageNum)
		if err != nil {
			return nil, err
		}
		if img2, err = p.RenderImage(opts); err != nil {
			return nil, err
		}
	}
	switch {
	case img1 == nil && img2 == nil:
		return nil, fmt.Errorf("page %d exists in neither document", pageNum)
	case img1 == nil:
		img1 = blankWhiteImage(img2.Bounds())
	case img2 == nil:
		img2 = blankWhiteImage(img1.Bounds())
	}
	return newImagesDifference(img1, img2, c.Threshold), nil
}

func blankWhiteImage(b image.Rectangle) image.Image {
	out := image.NewRGBA(b)
	draw.Draw(out, b, image.White, image.Point{}, draw.Src)
	return out
}

// drawDifferenceReportPage lays diff's highlighted image full-page (fit,
// centred, preserving aspect) with a caption below it.
func drawDifferenceReportPage(page *Page, diff *ImagesDifference, pageNum int, highlight Color) error {
	size, err := page.Size()
	if err != nil {
		return err
	}
	img := diff.DifferenceToImage(highlight, Color{R: 1, G: 1, B: 1, A: 1})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	const margin, captionH = 36.0, 24.0
	avail := Rectangle{LLX: margin, LLY: margin + captionH, URX: size.Width - margin, URY: size.Height - margin}
	rect := fitRectCentered(avail, float64(diff.w), float64(diff.h))
	if err := page.AddImageFromStream(bytes.NewReader(buf.Bytes()), rect); err != nil {
		return err
	}
	caption := fmt.Sprintf("Page %d  —  %.2f%% pixels different (%d / %d)",
		pageNum, diff.Ratio()*100, diff.DifferentPixels(), diff.TotalPixels())
	return page.AddText(caption, TextStyle{Font: FontHelvetica, Size: 10, HAlign: HAlignCenter},
		Rectangle{LLX: margin, LLY: margin, URX: size.Width - margin, URY: margin + captionH})
}

// fitRectCentered returns the largest w×h-aspect rectangle that fits inside
// outer, centred.
func fitRectCentered(outer Rectangle, w, h float64) Rectangle {
	ow, oh := outer.URX-outer.LLX, outer.URY-outer.LLY
	if w <= 0 || h <= 0 || ow <= 0 || oh <= 0 {
		return outer
	}
	scale := ow / w
	if oh/h < scale {
		scale = oh / h
	}
	dw, dh := w*scale, h*scale
	x0 := outer.LLX + (ow-dw)/2
	y0 := outer.LLY + (oh-dh)/2
	return Rectangle{LLX: x0, LLY: y0, URX: x0 + dw, URY: y0 + dh}
}

// ImagesDifference is the result of comparing two rendered pages pixel by
// pixel. Mirrors Aspose.PDF for .NET's ImagesDifference; DifferentPixels /
// TotalPixels / Ratio are this library's addition (the design goal the
// backlog names it by: "difference image plus the changed-pixel ratio") —
// Aspose instead exposes the raw LockBits-style difference array plus its
// Height/Stride, a .NET Bitmap implementation detail with no Go equivalent
// worth mirroring literally when image.Image already carries that shape.
type ImagesDifference struct {
	source, dest *image.RGBA
	diff         []bool // row-major, len == w*h of the compared (union) canvas
	w, h         int
	different    int
}

// newImagesDifference compares a and b over their union bounding box; either
// side smaller than that box is padded with white (so a page shorter than
// the other's canvas compares its missing area against white — the common
// page background — rather than transparent/black).
func newImagesDifference(a, b image.Image, thresholdPct float64) *ImagesDifference {
	ab, bb := a.Bounds(), b.Bounds()
	w := ab.Dx()
	if bb.Dx() > w {
		w = bb.Dx()
	}
	h := ab.Dy()
	if bb.Dy() > h {
		h = bb.Dy()
	}
	src := toRGBAWhite(a, w, h)
	dst := toRGBAWhite(b, w, h)

	const maxDist = 255.0 * 3
	threshold := thresholdPct / 100 * maxDist
	diff := make([]bool, w*h)
	n := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := src.PixOffset(x, y)
			j := dst.PixOffset(x, y)
			dr := absInt(int(src.Pix[i]) - int(dst.Pix[j]))
			dg := absInt(int(src.Pix[i+1]) - int(dst.Pix[j+1]))
			db := absInt(int(src.Pix[i+2]) - int(dst.Pix[j+2]))
			d := float64(dr+dg+db) > threshold
			diff[y*w+x] = d
			if d {
				n++
			}
		}
	}
	return &ImagesDifference{source: src, dest: dst, diff: diff, w: w, h: h, different: n}
}

// toRGBAWhite draws src into a w×h *image.RGBA canvas with a white
// background, offset so src's own origin (which may be non-zero) lands at
// (0,0) of the canvas.
func toRGBAWhite(src image.Image, w, h int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(out, out.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(out, image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()), src, src.Bounds().Min, draw.Src)
	return out
}

// SourceImage returns the first page's rendered image (padded to the
// compared canvas size, see newImagesDifference). Mirrors
// ImagesDifference.SourceImage.
func (d *ImagesDifference) SourceImage() image.Image { return d.source }

// DestinationImage returns the second page's rendered image (padded to the
// compared canvas size, see newImagesDifference). Mirrors
// ImagesDifference.GetDestinationImage.
func (d *ImagesDifference) DestinationImage() image.Image { return d.dest }

// DifferenceToImage renders a bitmap the size of the compared canvas:
// foreground where pixels differ, background where they agree. Mirrors
// ImagesDifference.DifferenceToImage.
func (d *ImagesDifference) DifferenceToImage(foreground, background Color) image.Image {
	out := image.NewRGBA(image.Rect(0, 0, d.w, d.h))
	fg, bg := colorToRGBA(foreground), colorToRGBA(background)
	for y := 0; y < d.h; y++ {
		for x := 0; x < d.w; x++ {
			c := bg
			if d.diff[y*d.w+x] {
				c = fg
			}
			out.SetRGBA(x, y, c)
		}
	}
	return out
}

// DifferentPixels returns the number of pixels whose colour distance
// exceeded Threshold.
func (d *ImagesDifference) DifferentPixels() int { return d.different }

// TotalPixels returns the pixel count of the compared (union) canvas.
func (d *ImagesDifference) TotalPixels() int { return d.w * d.h }

// Ratio returns DifferentPixels/TotalPixels in [0,1]; 0 for an empty canvas.
func (d *ImagesDifference) Ratio() float64 {
	if d.w*d.h == 0 {
		return 0
	}
	return float64(d.different) / float64(d.w*d.h)
}

// HasDifferences reports whether any pixel differs.
func (d *ImagesDifference) HasDifferences() bool { return d.different > 0 }

func colorToRGBA(c Color) color.RGBA {
	return color.RGBA{
		R: uint8(clamp01(c.R) * 255),
		G: uint8(clamp01(c.G) * 255),
		B: uint8(clamp01(c.B) * 255),
		A: uint8(clamp01(c.A) * 255),
	}
}

