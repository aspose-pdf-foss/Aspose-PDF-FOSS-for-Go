// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"errors"
	"io"
)

// MarkupSide selects which document the markup is drawn on.
type MarkupSide int

const (
	// MarkupDestination marks the second document: insertions highlighted,
	// deletions shown as carets carrying the removed text. The zero value.
	MarkupDestination MarkupSide = iota
	// MarkupSource marks the first document: deletions struck out,
	// insertions shown as carets carrying the added text.
	MarkupSource
)

// MarkupOptions styles the marked-up copy. The zero value marks the second
// document in green and red, titles every annotation "Comparison" and keeps
// the annotations live.
type MarkupOptions struct {
	Side        MarkupSide
	InsertColor *Color
	DeleteColor *Color
	Title       string
	Flatten     bool
}

// resolved fills in the defaults.
func (o MarkupOptions) resolved() MarkupOptions {
	if o.InsertColor == nil {
		o.InsertColor = &Color{R: 0.20, G: 0.72, B: 0.35, A: 1}
	}
	if o.DeleteColor == nil {
		o.DeleteColor = &Color{R: 0.88, G: 0.22, B: 0.22, A: 1}
	}
	if o.Title == "" {
		o.Title = "Comparison"
	}
	return o
}

func lastMarkupOption(opts []MarkupOptions) MarkupOptions {
	if len(opts) == 0 {
		return MarkupOptions{}.resolved()
	}
	return opts[len(opts)-1].resolved()
}

// SaveMarkup writes a copy of one of the compared documents with every
// difference marked as an annotation. The documents handed to the comparer
// are left untouched.
func (r *ComparisonResult) SaveMarkup(path string, opts ...MarkupOptions) error {
	var buf bytes.Buffer
	if err := r.WriteMarkup(&buf, opts...); err != nil {
		return err
	}
	return writeFile(path, buf.Bytes())
}

// WriteMarkup writes the marked-up copy to w.
func (r *ComparisonResult) WriteMarkup(w io.Writer, opts ...MarkupOptions) error {
	o := lastMarkupOption(opts)
	source := r.src
	if o.Side == MarkupDestination {
		source = r.dst
	}
	if source == nil {
		return errors.New("WriteMarkup: the comparison has no document on that side")
	}
	doc, err := copyDocument(source)
	if err != nil {
		return err
	}
	if err := r.annotate(doc, o); err != nil {
		return err
	}
	if o.Flatten {
		for i := 1; i <= doc.PageCount(); i++ {
			page, err := doc.Page(i)
			if err != nil {
				return err
			}
			if err := page.Annotations().Flatten(); err != nil {
				return err
			}
		}
	}
	_, err = doc.WriteTo(w)
	return err
}

// copyDocument serializes a document and opens the bytes again, yielding an
// independent document the caller's is unaffected by.
func copyDocument(d *Document) (*Document, error) {
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		return nil, err
	}
	return OpenStream(bytes.NewReader(buf.Bytes()))
}

// annotate draws every difference onto doc.
func (r *ComparisonResult) annotate(doc *Document, o MarkupOptions) error {
	for i, op := range r.ops {
		if op.Operation == OperationEqual {
			continue
		}
		pageNum, rects := markupTarget(op, o.Side)
		if len(rects) > 0 {
			if err := addSpanAnnotation(doc, pageNum, rects, op, o); err != nil {
				return err
			}
			continue
		}
		// The run has no text on this side — a deletion seen on the new
		// document, or an insertion seen on the old one. Anchor a caret
		// between the neighbouring unchanged words.
		anchorPage, anchor, ok := anchorFor(r.ops, i, o.Side)
		if !ok {
			continue // the whole page is gone: nowhere to put a mark
		}
		if err := addCaretAnnotation(doc, anchorPage, anchor, op, o); err != nil {
			return err
		}
	}
	return nil
}

// markupTarget returns the page and rectangles an operation occupies on the
// marked side, if any.
func markupTarget(op DiffOperation, side MarkupSide) (int, []Rectangle) {
	if side == MarkupSource {
		return op.SourcePage, op.SourceRects
	}
	return op.DestPage, op.DestRects
}

// anchorFor finds where to put a caret for the operation at index i: the
// right edge of the last unchanged word before it, else the left edge of the
// first unchanged word after it.
func anchorFor(ops []DiffOperation, i int, side MarkupSide) (int, Rectangle, bool) {
	for j := i - 1; j >= 0; j-- {
		if ops[j].Operation != OperationEqual {
			continue
		}
		page, rects := markupTarget(ops[j], side)
		if len(rects) == 0 {
			continue
		}
		last := rects[len(rects)-1]
		h := last.URY - last.LLY
		return page, Rectangle{LLX: last.URX, LLY: last.LLY, URX: last.URX + h/2, URY: last.URY}, true
	}
	for j := i + 1; j < len(ops); j++ {
		if ops[j].Operation != OperationEqual {
			continue
		}
		page, rects := markupTarget(ops[j], side)
		if len(rects) == 0 {
			continue
		}
		first := rects[0]
		h := first.URY - first.LLY
		return page, Rectangle{LLX: first.LLX - h/2, LLY: first.LLY, URX: first.LLX, URY: first.URY}, true
	}
	return 0, Rectangle{}, false
}

// addSpanAnnotation highlights an insertion or strikes out a deletion.
func addSpanAnnotation(doc *Document, pageNum int, rects []Rectangle, op DiffOperation, o MarkupOptions) error {
	page, err := doc.Page(pageNum)
	if err != nil {
		return err
	}
	bbox := rects[0]
	quads := make([]QuadPoint, 0, len(rects))
	for _, r := range rects {
		bbox = unionRect(bbox, r)
		quads = append(quads, rectAsQuadPoint(r))
	}

	colour := o.InsertColor
	if op.Operation == OperationDelete {
		colour = o.DeleteColor
	}

	if op.Operation == OperationInsert {
		a := NewHighlightAnnotation(page, bbox)
		a.SetQuadPoints(quads)
		a.SetColor(colour)
		a.SetTitle(o.Title)
		a.SetContents(op.Text)
		setAppearanceN(&a.annotationBase, highlightAppearance(rects, bbox, *colour))
		return page.Annotations().Add(a)
	}

	a := NewStrikeOutAnnotation(page, bbox)
	a.SetQuadPoints(quads)
	a.SetColor(colour)
	a.SetTitle(o.Title)
	a.SetContents(op.Text)
	setAppearanceN(&a.annotationBase, strikeOutAppearance(rects, bbox, *colour))
	return page.Annotations().Add(a)
}

// addCaretAnnotation marks the point where text was removed or added.
func addCaretAnnotation(doc *Document, pageNum int, rect Rectangle, op DiffOperation, o MarkupOptions) error {
	page, err := doc.Page(pageNum)
	if err != nil {
		return err
	}
	colour := o.DeleteColor
	if op.Operation == OperationInsert {
		colour = o.InsertColor
	}
	a := NewCaretAnnotation(page, rect)
	a.SetColor(colour)
	a.SetTitle(o.Title)
	a.SetContents(op.Text)
	return page.Annotations().Add(a)
}

// highlightAppearance paints the marked words in a translucent wash. Multiply
// blending keeps the glyphs readable through the colour, which is what a
// highlighter pen does and what viewers synthesize for /Highlight.
func highlightAppearance(rects []Rectangle, bbox Rectangle, colour Color) *pdfStream {
	b := newAppearanceBuilder()
	b.SetFillColorRGB(colour)
	for _, r := range rects {
		b.Rect(r.LLX-bbox.LLX, r.LLY-bbox.LLY, r.URX-r.LLX, r.URY-r.LLY)
	}
	b.Fill()
	content := append([]byte("/GSMul gs\n"), b.Bytes()...)
	resources := pdfDict{
		"/ExtGState": pdfDict{
			"/GSMul": pdfDict{
				"/Type": pdfName("/ExtGState"),
				"/BM":   pdfName("/Multiply"),
				"/ca":   1.0,
			},
		},
	}
	return makeFormXObjectWithResources(content, Rectangle{URX: bbox.URX - bbox.LLX, URY: bbox.URY - bbox.LLY}, resources)
}

// strikeOutAppearance draws a line through the middle of each marked run.
func strikeOutAppearance(rects []Rectangle, bbox Rectangle, colour Color) *pdfStream {
	b := newAppearanceBuilder()
	b.SetStrokeColorRGB(colour)
	b.SetLineWidth(1)
	for _, r := range rects {
		y := (r.LLY+r.URY)/2 - bbox.LLY
		b.MoveTo(r.LLX-bbox.LLX, y)
		b.LineTo(r.URX-bbox.LLX, y)
	}
	b.Stroke()
	return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: bbox.URX - bbox.LLX, URY: bbox.URY - bbox.LLY}, pdfDict{})
}
