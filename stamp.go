// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
)

// Stamp is content overlaid on (or behind) a page: a TextStamp, an ImageStamp,
// or a PageNumberStamp. Apply it with (*Page).AddStamp or (*Document).AddStamp.
// Mirrors Aspose.PDF for .NET's Aspose.Pdf.Stamp class hierarchy.
//
// Every stamp shares the placement and appearance fields of stampBase
// (embedded): a placement Rect (zero value = the whole page), HAlign/VAlign
// within that Rect, Opacity, RotateAngle, and Background (draw behind page
// content). Concrete types add their own content.
type Stamp interface {
	// applyToPage draws the stamp onto p. Unexported so only this package's
	// stamp types satisfy the interface.
	applyToPage(p *Page) error
}

// stampBase holds the placement and appearance common to every stamp. Its
// exported fields mirror the corresponding Aspose.Pdf.Stamp properties
// (HorizontalAlignment/VerticalAlignment, Opacity, RotateAngle, Background).
type stampBase struct {
	// Rect is the placement box in PDF user space. The zero value means the
	// stamp covers the whole page (the page's MediaBox) — convenient for
	// full-page watermarks, headers, and footers.
	Rect Rectangle
	// HAlign/VAlign position the content within Rect. Mirror Aspose's
	// HorizontalAlignment / VerticalAlignment.
	HAlign HAlign
	VAlign VAlign
	// Opacity in [0,1]; 1 = fully opaque. The zero value is treated as 1, so a
	// stamp built with a struct literal is still visible. Mirrors Aspose Opacity.
	Opacity float64
	// RotateAngle rotates the content counter-clockwise about the lower-left
	// corner of Rect, in degrees. Mirrors Aspose RotateAngle.
	RotateAngle float64
	// Background, when true, draws the stamp behind existing page content
	// (a watermark). When false (default) it draws on top. Mirrors Aspose
	// Stamp.Background.
	Background bool
}

func (b *stampBase) opacity() float64 {
	if b.Opacity <= 0 || b.Opacity > 1 {
		return 1
	}
	return b.Opacity
}

// resolveRect returns the explicit Rect, or the page's full MediaBox when Rect
// is the zero value.
func (b *stampBase) resolveRect(p *Page) (Rectangle, error) {
	if b.Rect != (Rectangle{}) {
		return b.Rect, nil
	}
	sz, err := p.Size()
	if err != nil {
		return Rectangle{}, err
	}
	return Rectangle{LLX: 0, LLY: 0, URX: sz.Width, URY: sz.Height}, nil
}

// TextStamp overlays a line (or wrapped block) of text. Mirrors
// Aspose.Pdf.TextStamp. Construct with NewTextStamp.
type TextStamp struct {
	stampBase
	// Value is the text to stamp. Mirrors Aspose TextStamp.Value.
	Value string
	// TextStyle controls font, size, and colour (the stamp's HAlign/VAlign/
	// RotateAngle/Background override the style's alignment/rotation/behind).
	// Mirrors Aspose TextStamp.TextState.
	TextStyle TextStyle
}

// NewTextStamp creates a text stamp. Mirrors Aspose's TextStamp(string, ...).
func NewTextStamp(text string, style TextStyle) *TextStamp {
	return &TextStamp{stampBase: stampBase{Opacity: 1}, Value: text, TextStyle: style}
}

func (s *TextStamp) applyToPage(p *Page) error {
	rect, err := s.resolveRect(p)
	if err != nil {
		return err
	}
	// AddText rotates about the rect's lower-left corner; a stamp should rotate
	// about the rect's centre (so a centred watermark stays on the page).
	// Pre-shift the rect so the corner-pivot rotation lands as a centre-pivot one.
	if s.RotateAngle != 0 {
		rect = centerPivotRect(rect, s.RotateAngle)
	}
	st := s.TextStyle
	st.HAlign, st.VAlign = s.HAlign, s.VAlign
	st.Rotation, st.Behind = s.RotateAngle, s.Background

	// Fold the stamp opacity into the text and background colour alpha so the
	// existing AddText machinery renders it translucent.
	op := s.opacity()
	col := Color{A: 1}
	if st.Color != nil {
		col = *st.Color
	}
	col.A *= op
	st.Color = &col
	if st.Background != nil {
		bg := *st.Background
		bg.A *= op
		st.Background = &bg
	}
	return p.AddText(s.Value, st, rect)
}

// PageNumberStamp stamps the page number (and optionally the total), formatted
// by Format. Mirrors Aspose.Pdf.PageNumberStamp. Construct with
// NewPageNumberStamp; typically placed as a footer (set VAlign = VAlignBottom).
type PageNumberStamp struct {
	stampBase
	// Format is the template. "{0}" is replaced by the current page number and
	// "{1}" by the total page count — e.g. "Page {0} of {1}". Empty = "{0}".
	// Mirrors Aspose PageNumberStamp.Format (which uses "#").
	Format string
	// StartingNumber is the number shown on page 1 (default 1); page n shows
	// StartingNumber-1+n. Mirrors Aspose PageNumberStamp.StartingNumber.
	StartingNumber int
	// TextStyle controls font, size, and colour.
	TextStyle TextStyle
}

// NewPageNumberStamp creates a page-number stamp. Mirrors Aspose's
// PageNumberStamp(format, ...).
func NewPageNumberStamp(format string, style TextStyle) *PageNumberStamp {
	return &PageNumberStamp{
		stampBase:      stampBase{Opacity: 1},
		Format:         format,
		StartingNumber: 1,
		TextStyle:      style,
	}
}

func (s *PageNumberStamp) applyToPage(p *Page) error {
	num := s.StartingNumber - 1 + p.Number()
	total := p.doc.PageCount()
	f := s.Format
	if f == "" {
		f = "{0}"
	}
	text := strings.ReplaceAll(f, "{0}", strconv.Itoa(num))
	text = strings.ReplaceAll(text, "{1}", strconv.Itoa(total))

	ts := &TextStamp{stampBase: s.stampBase, Value: text, TextStyle: s.TextStyle}
	return ts.applyToPage(p)
}

// ImageStamp overlays a raster image (PNG or JPEG), stretched to fill Rect.
// Mirrors Aspose.Pdf.ImageStamp. Construct with NewImageStamp /
// NewImageStampFromStream.
type ImageStamp struct {
	stampBase
	data []byte
}

// NewImageStamp creates an image stamp from a PNG/JPEG file. Mirrors Aspose's
// ImageStamp(string).
func NewImageStamp(path string) (*ImageStamp, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("new image stamp: %w", err)
	}
	return &ImageStamp{stampBase: stampBase{Opacity: 1}, data: data}, nil
}

// NewImageStampFromStream creates an image stamp from a PNG/JPEG reader.
// Mirrors Aspose's ImageStamp(Stream).
func NewImageStampFromStream(r io.Reader) (*ImageStamp, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("new image stamp: %w", err)
	}
	return &ImageStamp{stampBase: stampBase{Opacity: 1}, data: data}, nil
}

func (s *ImageStamp) applyToPage(p *Page) error {
	rect, err := s.resolveRect(p)
	if err != nil {
		return err
	}
	if err := rect.validate(); err != nil {
		return err
	}
	return p.drawImageStamp(s.data, rect, s.RotateAngle, s.opacity(), s.Background)
}

// drawImageStamp places image data into rect, rotated CCW by rotateDeg about
// rect's lower-left corner, at the given opacity, in front of (or behind) the
// existing page content.
func (p *Page) drawImageStamp(data []byte, rect Rectangle, rotateDeg, opacity float64, behind bool) error {
	if len(data) == 0 {
		return fmt.Errorf("add image stamp: empty image data")
	}
	format, err := detectImageFormat(data)
	if err != nil {
		return err
	}
	resName, _, _, err := p.addSVGImageXObject(data, format)
	if err != nil {
		return err
	}

	w := rect.URX - rect.LLX
	h := rect.URY - rect.LLY
	th := rotateDeg * math.Pi / 180
	cos, sin := math.Cos(th), math.Sin(th)
	// Rotate about the rect centre (not its corner): cm =
	// Translate(C) · Rotate(th) · Translate(-w/2,-h/2) · Scale(w,h).
	a, b, c, d := w*cos, w*sin, -h*sin, h*cos
	cx, cy := rect.LLX+w/2, rect.LLY+h/2
	rx := cos*(w/2) - sin*(h/2)
	ry := sin*(w/2) + cos*(h/2)
	e, f := cx-rx, cy-ry

	gs := ""
	if opacity < 1 {
		name, err := p.ensureExtGState(opacity)
		if err != nil {
			return err
		}
		gs = name + " gs\n"
	}

	ops := fmt.Sprintf("\nq\n%s%s %s %s %s %s %s cm\n%s Do\nQ\n",
		gs,
		formatFloat(a), formatFloat(b), formatFloat(c), formatFloat(d),
		formatFloat(e), formatFloat(f), resName)

	if behind {
		return p.prependToContentStream([]byte(ops))
	}
	return p.appendToContentStream([]byte(ops))
}

// centerPivotRect returns a rect of the same size whose lower-left corner is
// placed so that AddText's lower-left-corner rotation by deg produces the same
// result as rotating the original rect's content about its centre.
func centerPivotRect(rect Rectangle, deg float64) Rectangle {
	w := rect.URX - rect.LLX
	h := rect.URY - rect.LLY
	cx, cy := rect.LLX+w/2, rect.LLY+h/2
	rad := deg * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)
	rx := cos*(w/2) - sin*(h/2)
	ry := sin*(w/2) + cos*(h/2)
	lx, ly := cx-rx, cy-ry
	return Rectangle{LLX: lx, LLY: ly, URX: lx + w, URY: ly + h}
}

// AddStamp draws a stamp onto the page. Mirrors Aspose.PDF for .NET's
// Page.AddStamp(Stamp).
func (p *Page) AddStamp(s Stamp) error {
	if s == nil {
		return fmt.Errorf("add stamp: nil stamp")
	}
	return s.applyToPage(p)
}

// AddStamp draws a stamp onto the given 1-based pages (all pages when none are
// given) — convenient for watermarks, headers, footers, and page numbers. A
// PageNumberStamp renders the correct number on each page.
func (d *Document) AddStamp(s Stamp, pageNums ...int) error {
	if s == nil {
		return fmt.Errorf("add stamp: nil stamp")
	}
	indices, err := resolvePageIndices(len(d.pages), pageNums)
	if err != nil {
		return fmt.Errorf("add stamp: %w", err)
	}
	for _, i := range indices {
		page := &Page{doc: d, index: i}
		if err := s.applyToPage(page); err != nil {
			return fmt.Errorf("add stamp: page %d: %w", i+1, err)
		}
	}
	return nil
}
