// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"math"
)

// PdfPageStamp overlays (or underlays) a page from another PDF document as a
// stamp — the source page is imported once as a Form XObject and drawn into the
// stamp's Rect, scaled to fit while preserving aspect ratio, positioned by
// HAlign/VAlign, and honoring Opacity, RotateAngle and Background. Mirrors
// Aspose.PDF for .NET's PdfPageStamp. Construct with NewPdfPageStamp.
type PdfPageStamp struct {
	stampBase
	srcDoc  *Document
	srcPage int
}

// NewPdfPageStamp creates a stamp that draws page srcPageNum (1-based) of srcDoc.
func NewPdfPageStamp(srcDoc *Document, srcPageNum int) (*PdfPageStamp, error) {
	if srcDoc == nil {
		return nil, fmt.Errorf("NewPdfPageStamp: nil source document")
	}
	if srcPageNum < 1 || srcPageNum > srcDoc.PageCount() {
		return nil, fmt.Errorf("NewPdfPageStamp: source page %d out of range [1,%d]", srcPageNum, srcDoc.PageCount())
	}
	return &PdfPageStamp{srcDoc: srcDoc, srcPage: srcPageNum}, nil
}

func (s *PdfPageStamp) applyToPage(p *Page) error {
	srcPage, err := s.srcDoc.Page(s.srcPage)
	if err != nil {
		return err
	}
	rect, err := s.resolveRect(p)
	if err != nil {
		return err
	}
	if err := rect.validate(); err != nil {
		return err
	}
	// Import the source page as a Form XObject into the target document, then
	// register it on this page and draw it.
	xobjID, box, err := p.doc.importPageAsXObject(s.srcDoc, srcPage, map[int]int{})
	if err != nil {
		return err
	}
	name := p.registerFormXObject(pdfRef{Num: xobjID})
	return p.drawFormStamp(name, box, rect, s.HAlign, s.VAlign, s.RotateAngle, s.opacity(), s.Background)
}

// drawFormStamp draws the Form XObject named name (whose bounding box is box)
// into rect: scaled to fit preserving aspect, aligned by ha/va, rotated CCW by
// rotateDeg about rect's centre, at the given opacity, in front of or behind the
// existing page content.
func (p *Page) drawFormStamp(name string, box, rect Rectangle, ha HAlign, va VAlign, rotateDeg, opacity float64, behind bool) error {
	bw, bh := box.URX-box.LLX, box.URY-box.LLY
	if bw <= 0 || bh <= 0 {
		return fmt.Errorf("draw page stamp: source page has an empty box")
	}
	rw, rh := rect.URX-rect.LLX, rect.URY-rect.LLY
	scale := math.Min(rw/bw, rh/bh)
	sw, sh := bw*scale, bh*scale

	var x0, y0 float64
	switch ha {
	case HAlignCenter:
		x0 = rect.LLX + (rw-sw)/2
	case HAlignRight:
		x0 = rect.URX - sw
	default: // HAlignLeft
		x0 = rect.LLX
	}
	switch va {
	case VAlignMiddle:
		y0 = rect.LLY + (rh-sh)/2
	case VAlignBottom:
		y0 = rect.LLY
	default: // VAlignTop
		y0 = rect.URY - sh
	}

	// Placement: form space → device (fit-scale then translate; box may not
	// start at the origin).
	place := [6]float64{scale, 0, 0, scale, x0 - scale*box.LLX, y0 - scale*box.LLY}

	// Rotation about the rect centre.
	final := place
	if rotateDeg != 0 {
		cx, cy := rect.LLX+rw/2, rect.LLY+rh/2
		th := rotateDeg * math.Pi / 180
		cos, sin := math.Cos(th), math.Sin(th)
		rot := matMul(matMul(
			translateMatrix(-cx, -cy),
			[6]float64{cos, sin, -sin, cos, 0, 0}),
			translateMatrix(cx, cy))
		final = matMul(place, rot) // place applied first, then rotate
	}

	gs := ""
	if opacity < 1 {
		gsName, err := p.ensureExtGState(opacity)
		if err != nil {
			return err
		}
		gs = gsName + " gs\n"
	}

	ops := fmt.Sprintf("\nq\n%s%s %s %s %s %s %s cm\n%s Do\nQ\n",
		gs,
		formatFloat(final[0]), formatFloat(final[1]), formatFloat(final[2]),
		formatFloat(final[3]), formatFloat(final[4]), formatFloat(final[5]), name)

	if behind {
		return p.prependToContentStream([]byte(ops))
	}
	return p.appendToContentStream([]byte(ops))
}
