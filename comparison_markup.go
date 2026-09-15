// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

// DiffMarkupSide selects which document the markup is drawn on.
type DiffMarkupSide int

const (
	// DiffMarkupDestination marks the second document: insertions
	// highlighted, deletions shown as carets carrying the removed text. The
	// zero value.
	DiffMarkupDestination DiffMarkupSide = iota
	// DiffMarkupSource marks the first document: deletions struck out,
	// insertions shown as carets carrying the added text.
	DiffMarkupSource
)

// DiffMarkupOptions styles the marked-up copy. The zero value marks the
// second document in green and red, titles every annotation "Comparison" and
// keeps the annotations live.
type DiffMarkupOptions struct {
	Side        DiffMarkupSide
	InsertColor *Color
	DeleteColor *Color
	Title       string
	// Flatten bakes the comparison markup into the page content and removes
	// it, via (*AnnotationCollection).Flatten. That call flattens every
	// non-widget annotation already on the page, not only the ones this
	// writer added — an existing highlight or free-text annotation in the
	// source document is flattened too.
	Flatten bool
}

// resolved fills in the defaults.
func (o DiffMarkupOptions) resolved() DiffMarkupOptions {
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

func lastMarkupOption(opts []DiffMarkupOptions) DiffMarkupOptions {
	if len(opts) == 0 {
		return DiffMarkupOptions{}.resolved()
	}
	return opts[len(opts)-1].resolved()
}

// SaveMarkup writes a copy of one of the compared documents with every
// difference marked as an annotation. The documents handed to the comparer
// are left untouched; the output is a new document, so signatures on the
// input do not survive the copy. Marking up an encrypted document is not
// supported — save it without encryption first.
func (r *ComparisonResult) SaveMarkup(path string, opts ...DiffMarkupOptions) error {
	var buf bytes.Buffer
	if err := r.WriteMarkup(&buf, opts...); err != nil {
		return err
	}
	return writeFile(path, buf.Bytes())
}

// WriteMarkup writes the marked-up copy to w. The output is a new document,
// so signatures on the input do not survive the copy. Marking up an
// encrypted document is not supported — save it without encryption first.
func (r *ComparisonResult) WriteMarkup(w io.Writer, opts ...DiffMarkupOptions) error {
	o := lastMarkupOption(opts)
	source := r.src
	if o.Side == DiffMarkupDestination {
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
// independent document the caller's is unaffected by. A document configured
// for encryption re-encrypts on WriteTo, so OpenStream would reject the
// serialized copy with a bare ErrEncrypted; that case is detected up front
// and reported with an error that names the operation and the cause instead.
func copyDocument(d *Document) (*Document, error) {
	if _, encrypted := d.Permissions(); encrypted {
		return nil, fmt.Errorf("WriteMarkup: marking up an encrypted document is not supported; save it without encryption first: %w", ErrEncrypted)
	}
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		return nil, err
	}
	return OpenStream(bytes.NewReader(buf.Bytes()))
}

// annotate draws every difference onto doc.
func (r *ComparisonResult) annotate(doc *Document, o DiffMarkupOptions) error {
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
func markupTarget(op DiffOperation, side DiffMarkupSide) (int, []Rectangle) {
	if side == DiffMarkupSource {
		return op.SourcePage, op.SourceRects
	}
	return op.DestPage, op.DestRects
}

// anchorFor finds where to put a caret for the operation at index i: the
// right edge of the last unchanged word before it, else the left edge of the
// first unchanged word after it.
func anchorFor(ops []DiffOperation, i int, side DiffMarkupSide) (int, Rectangle, bool) {
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
func addSpanAnnotation(doc *Document, pageNum int, rects []Rectangle, op DiffOperation, o DiffMarkupOptions) error {
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
func addCaretAnnotation(doc *Document, pageNum int, rect Rectangle, op DiffOperation, o DiffMarkupOptions) error {
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
