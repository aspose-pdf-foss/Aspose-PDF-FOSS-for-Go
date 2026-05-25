// SPDX-License-Identifier: MIT

package asposepdf

import (
	"io"
	"os"
)

// AddSVG reads an SVG file and renders it into the given rectangle on the page.
// Unsupported elements (text, image, gradients, masks) are skipped silently per
// Phase 2 scope.
//
// Returns error only on XML parse failure, invalid numeric attributes, or I/O errors.
func (p *Page) AddSVG(path string, rect Rectangle) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return p.AddSVGFromStream(f, rect)
}

// AddSVGFromStream renders an SVG from any io.Reader into the given rectangle on
// the page. Best-effort: unsupported elements are silently skipped.
func (p *Page) AddSVGFromStream(r io.Reader, rect Rectangle) error {
	svg, err := parseSVGReader(r)
	if err != nil {
		return err
	}
	return p.AddSVGObject(svg, rect)
}

// AddSVGObject renders a pre-parsed SVG into the given rectangle on the page.
// Useful when the same SVG is rendered on multiple pages without re-parsing.
func (p *Page) AddSVGObject(svg *SVG, rect Rectangle) error {
	return renderSVG(p, svg, rect)
}

// ViewBox returns the viewBox attribute as (x, y, width, height).
// If no viewBox is set, returns (0, 0, intrinsicWidth, intrinsicHeight).
func (s *SVG) ViewBox() (x, y, w, h float64) {
	if s.viewBox != nil {
		return s.viewBox.x, s.viewBox.y, s.viewBox.w, s.viewBox.h
	}
	return 0, 0, s.width, s.height
}

// Size returns the intrinsic width and height as parsed from the <svg> root
// element's width and height attributes. Returns (0, 0) if neither is present.
func (s *SVG) Size() (width, height float64) {
	return s.width, s.height
}

// LoadSVG reads and parses an SVG file once, returning a *SVG that can be passed
// to Page.AddSVGObject or Document.AddSVGObjectWatermark multiple times without
// re-parsing.
func (d *Document) LoadSVG(path string) (*SVG, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return d.LoadSVGFromStream(f)
}

// LoadSVGFromStream is the io.Reader variant of LoadSVG.
func (d *Document) LoadSVGFromStream(r io.Reader) (*SVG, error) {
	return parseSVGReader(r)
}

// AddSVGWatermark applies an SVG watermark to all pages (when pageNums is empty)
// or to the specified 1-based page numbers. The SVG is positioned to fill each
// page's MediaBox honoring its own preserveAspectRatio attribute.
func (d *Document) AddSVGWatermark(path string, pageNums ...int) error {
	svg, err := d.LoadSVG(path)
	if err != nil {
		return err
	}
	return d.AddSVGObjectWatermark(svg, pageNums...)
}

// AddSVGWatermarkFromStream is the io.Reader variant of AddSVGWatermark.
func (d *Document) AddSVGWatermarkFromStream(r io.Reader, pageNums ...int) error {
	svg, err := d.LoadSVGFromStream(r)
	if err != nil {
		return err
	}
	return d.AddSVGObjectWatermark(svg, pageNums...)
}

// AddSVGObjectWatermark uses a pre-parsed *SVG for the watermark content.
// Renders into each target page's full MediaBox.
func (d *Document) AddSVGObjectWatermark(svg *SVG, pageNums ...int) error {
	targets := pageNums
	if len(targets) == 0 {
		targets = make([]int, d.PageCount())
		for i := range targets {
			targets[i] = i + 1
		}
	}
	for _, n := range targets {
		page, err := d.Page(n)
		if err != nil {
			continue
		}
		ps, _ := page.Size()
		rect := Rectangle{LLX: 0, LLY: 0, URX: ps.Width, URY: ps.Height}
		if err := page.AddSVGObject(svg, rect); err != nil {
			return err
		}
	}
	return nil
}
