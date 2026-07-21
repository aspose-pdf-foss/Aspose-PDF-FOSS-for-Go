// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// PDF → SVG export (pdf-go-3wi0): each page becomes one standalone SVG file
// with true vector content, riding the same vector backend as the HTML
// exporter's native mode — path fills/strokes with real Bézier curves, images
// carrying the PDF's own JPEG/PNG bytes, chained clip paths, opacity and CSS
// blend modes; shadings/patterns/soft masks degrade locally to embedded
// raster patches. Unlike the HTML native mode (where a text layer carries the
// glyphs), page text is rendered INTO the SVG as outline paths — exact glyph
// shapes with no font dependencies (not selectable; that is the trade-off).
// Mirrors the intent of Aspose.PDF for .NET's Document.Save(SaveFormat.Svg).

// SVGSaveOptions configures SaveSVG / WriteSVG. The zero value exports all
// pages at 150 DPI with images embedded as data: URLs (self-contained files).
type SVGSaveOptions struct {
	// DPI sets the coordinate scale (the viewBox is the page at this
	// resolution) and the resolution of raster patches. 0 → 150. The vector
	// content itself is resolution-independent.
	DPI float64
	// Pages is a 1-based subset for Document.SaveSVG; nil = all pages.
	Pages []int
	// ResourceWriter externalizes binary parts (images, raster patches)
	// instead of embedding them as data: URLs: it receives a unique name +
	// bytes and returns the URL to reference from the SVG.
	ResourceWriter func(name string, data []byte) (url string, err error)
}

// WriteSVG writes the page as one standalone SVG document to w.
func (p *Page) WriteSVG(w io.Writer, opts ...SVGSaveOptions) error {
	var opt SVGSaveOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	dpi := opt.DPI
	if dpi <= 0 {
		dpi = DefaultDPI
	}
	svg, err := renderPageSVGCore(p, dpi, false, false, htmlResourceSink(opt.ResourceWriter), p.Number())
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"); err != nil {
		return err
	}
	_, err = io.WriteString(w, svg)
	return err
}

// SaveSVG writes the page as a standalone SVG file.
func (p *Page) SaveSVG(path string, opts ...SVGSaveOptions) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("save svg: %w", err)
	}
	werr := p.WriteSVG(f, opts...)
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	if cerr != nil {
		return fmt.Errorf("save svg: %w", cerr)
	}
	return nil
}

// SaveSVG exports the document's pages as SVG files. A single selected page
// is written to path itself; several pages become sibling files
// "<stem>_pN.svg" (N = the source page number), like the HTML exporter's
// SplitPages. Mirrors Aspose.PDF for .NET's Document.Save(SaveFormat.Svg).
func (d *Document) SaveSVG(path string, opts ...SVGSaveOptions) error {
	var opt SVGSaveOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	sel := opt.Pages
	if len(sel) == 0 {
		sel = make([]int, d.PageCount())
		for i := range sel {
			sel[i] = i + 1
		}
	} else {
		for _, n := range sel {
			if n < 1 || n > d.PageCount() {
				return fmt.Errorf("SaveSVG: page %d out of range 1..%d", n, d.PageCount())
			}
		}
	}
	if len(sel) == 1 {
		p, err := d.Page(sel[0])
		if err != nil {
			return err
		}
		return p.SaveSVG(path, opt)
	}
	ext := filepath.Ext(path)
	if ext == "" {
		ext = ".svg"
	}
	stem := strings.TrimSuffix(path, filepath.Ext(path))
	for _, n := range sel {
		p, err := d.Page(n)
		if err != nil {
			return err
		}
		if err := p.SaveSVG(fmt.Sprintf("%s_p%d%s", stem, n, ext), opt); err != nil {
			return err
		}
	}
	return nil
}
