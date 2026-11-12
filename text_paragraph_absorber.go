// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Re-aligning text already on a page (pdf-go-v0v4), mirroring Aspose.PDF for
// .NET's TextParagraphAbsorber and TextParagraph.HorizontalAlignment. The
// glyphs are not redrawn: each word is moved where the alignment puts it with
// TJ numbers (shiftLineText, the machinery of ReplaceAdjustmentShiftRestOfLine),
// so the text keeps its fonts, its embedded subsets and its extraction.

// HorizontalAlignment is how a TextParagraph's lines sit in its rectangle.
// Mirrors Aspose.PDF for .NET's HorizontalAlignment.
type HorizontalAlignment int

const (
	// HorizontalAlignmentNone leaves the lines where they are.
	HorizontalAlignmentNone HorizontalAlignment = iota
	// HorizontalAlignmentLeft starts every line at the rectangle's left edge.
	HorizontalAlignmentLeft
	// HorizontalAlignmentCenter centres every line in the rectangle.
	HorizontalAlignmentCenter
	// HorizontalAlignmentRight ends every line at the rectangle's right edge.
	HorizontalAlignmentRight
	// HorizontalAlignmentJustify spreads every line but the last across the
	// rectangle by the space between its words; the last starts at the left
	// edge, as does a line the source ended a paragraph on (the next line's
	// first word would have fitted on it).
	HorizontalAlignmentJustify
	// HorizontalAlignmentFullJustify spreads every line, the last included.
	HorizontalAlignmentFullJustify
)

// TextParagraphAbsorber finds the text inside rectangles of a page as
// paragraphs whose alignment can be changed. Mirrors Aspose.PDF for .NET's
// TextParagraphAbsorber.
type TextParagraphAbsorber struct {
	rects      []Rectangle
	paragraphs []*TextParagraph
}

// NewTextParagraphAbsorber returns an absorber taking one paragraph from
// each rectangle. With none, Visit takes the paragraphs Page.Paragraphs
// finds, each in its own rectangle.
func NewTextParagraphAbsorber(rects ...Rectangle) *TextParagraphAbsorber {
	return &TextParagraphAbsorber{rects: append([]Rectangle(nil), rects...)}
}

// TextParagraphs returns the paragraphs the last Visit found.
func (a *TextParagraphAbsorber) TextParagraphs() []*TextParagraph {
	return append([]*TextParagraph(nil), a.paragraphs...)
}

// TextParagraph is the text inside one rectangle of a page: its lines, each
// made of words, in reading order.
type TextParagraph struct {
	page  *Page
	rect  Rectangle
	lines []paraTextLine
	align HorizontalAlignment
}

// paraTextLine is a line of a TextParagraph: its baseline, size and words,
// left to right.
type paraTextLine struct {
	y, size float64
	words   []paraWord
	// ends says the line ends a paragraph of the source: the next line's
	// first word would have fitted on it, so it was not broken for want of
	// room. Justify leaves such a line, like the last, unstretched.
	ends bool
}

type paraWord struct {
	text   string
	x0, x1 float64 // left and right edge of the word's glyphs
}

// Visit finds the paragraphs on p, replacing those of an earlier visit. Text
// is taken glyph by glyph as ExtractText does with
// TextExtractOptions.Rectangle; a rectangle holding no text gives no
// paragraph.
func (a *TextParagraphAbsorber) Visit(p *Page) error {
	a.paragraphs = nil
	rects := a.rects
	if len(rects) == 0 {
		m, err := p.Paragraphs()
		if err != nil {
			return err
		}
		for _, s := range m.Sections {
			for _, par := range s.Paragraphs {
				if strings.TrimSpace(par.Text) != "" {
					rects = append(rects, par.Rectangle)
				}
			}
		}
	}
	for _, r := range rects {
		r = orderedRect(r)
		// Glyphs are taken with half a point to spare: a line set flush with
		// a clip rectangle's edge may start a hair outside it, and a glyph
		// left out would not move with its word.
		const slack = 0.5
		lines, err := p.layoutLinesIn(Rectangle{LLX: r.LLX - slack, LLY: r.LLY - slack, URX: r.URX + slack, URY: r.URY + slack})
		if err != nil {
			return err
		}
		tp := &TextParagraph{page: p, rect: r}
		for i := range lines {
			if l, ok := paragraphTextLine(&lines[i]); ok {
				tp.lines = append(tp.lines, l)
			}
		}
		if len(tp.lines) > 0 {
			markParagraphEnds(tp.lines)
			a.paragraphs = append(a.paragraphs, tp)
		}
	}
	return nil
}

// markParagraphEnds sets ends on each line after which the source started a
// new paragraph: one the next line's first word would have fitted on, with an
// em to spare, within the widest line of the block. The em keeps a line that
// broke for want of a point or two of room (the source's spaces may be wider
// than a quarter em) from counting as an end.
func markParagraphEnds(lines []paraTextLine) {
	right := math.Inf(-1)
	for _, l := range lines {
		right = maxf(right, l.words[len(l.words)-1].x1)
	}
	for i := range lines {
		if i == len(lines)-1 {
			lines[i].ends = true
			break
		}
		l, next := lines[i], lines[i+1].words[0]
		end := l.words[len(l.words)-1].x1 + l.size + (next.x1 - next.x0)
		lines[i].ends = end <= right
	}
}

// layoutLinesIn is ExtractTextWithLayout limited to the glyphs inside r.
func (p *Page) layoutLinesIn(r Rectangle) ([]TextLine, error) {
	data, err := p.contentStreams()
	if err != nil || len(data) == 0 {
		return nil, err
	}
	ops, err := parseContentStream(data)
	if err != nil {
		return nil, err
	}
	resources := p.pageResources()
	ext := newTextExtractor(p.doc.objects, resolveFontResources(p.doc.objects, resources))
	ext.process(ops, resources)
	ext.flushFragment()
	return groupFragmentsIntoLines(clipFragments(ext.fragments, r)), nil
}

// paragraphTextLine splits a horizontal line into words with their extents.
func paragraphTextLine(line *TextLine) (paraTextLine, bool) {
	for _, f := range line.Fragments {
		if math.Abs(f.Rotation) > 0.5 {
			return paraTextLine{}, false
		}
	}
	m := buildLineRuneMap(line)
	runes := []rune(string(m.text))
	out := paraTextLine{y: line.Y}
	for i := 0; i < len(runes); {
		if isSpaceRune(runes[i]) {
			i++
			continue
		}
		j := i
		for j < len(runes) && !isSpaceRune(runes[j]) {
			j++
		}
		if r, ok := matchRect(line.Fragments, m.owner, m.local, m.runeCounts, i, j); ok {
			out.words = append(out.words, paraWord{text: string(runes[i:j]), x0: r.LLX, x1: r.URX})
		}
		i = j
	}
	for _, f := range line.Fragments {
		out.size = maxf(out.size, f.FontSize)
	}
	sort.Slice(out.words, func(a, b int) bool { return out.words[a].x0 < out.words[b].x0 })
	return out, len(out.words) > 0
}

func isSpaceRune(r rune) bool { return r == ' ' || r == '\t' || r == ' ' }

// Page returns the page the paragraph is on.
func (tp *TextParagraph) Page() *Page { return tp.page }

// Rectangle returns the rectangle the paragraph was taken from, which its
// alignment refers to.
func (tp *TextParagraph) Rectangle() Rectangle { return tp.rect }

// Lines returns the text of each line, its words joined by single spaces.
func (tp *TextParagraph) Lines() []string {
	out := make([]string, len(tp.lines))
	for i, l := range tp.lines {
		w := make([]string, len(l.words))
		for k, word := range l.words {
			w[k] = word.text
		}
		out[i] = strings.Join(w, " ")
	}
	return out
}

// Text returns the paragraph's lines joined by spaces.
func (tp *TextParagraph) Text() string { return strings.Join(tp.Lines(), " ") }

// HorizontalAlignment returns the alignment last set, None until then.
func (tp *TextParagraph) HorizontalAlignment() HorizontalAlignment { return tp.align }

// SetHorizontalAlignment moves the paragraph's words so its lines sit in its
// rectangle as a says; each line keeps its words and its baseline. The
// glyphs stay what they were, moved with TJ numbers in the page's content
// stream. Text drawn inside a Form XObject, or on a rotated baseline, is not
// moved. Mirrors setting Aspose.PDF for .NET's TextParagraph.HorizontalAlignment.
func (tp *TextParagraph) SetHorizontalAlignment(a HorizontalAlignment) error {
	if a < HorizontalAlignmentNone || a > HorizontalAlignmentFullJustify {
		return fmt.Errorf("set horizontal alignment: unknown alignment %d", a)
	}
	if tp.page == nil {
		return fmt.Errorf("set horizontal alignment: the paragraph was not found by a TextParagraphAbsorber")
	}
	tp.align = a
	if a == HorizontalAlignmentNone {
		return nil
	}
	var shifts []lineShift
	targets := make([][]float64, len(tp.lines))
	for li, l := range tp.lines {
		targets[li] = alignedWordStarts(l, tp.rect, a, l.ends)
		for wi, w := range l.words {
			dx := targets[li][wi] - w.x0
			if math.Abs(dx) < 1e-6 {
				continue
			}
			// A word takes the glyphs from its start to the next word's
			// (the space between moves with it); the last stops at the
			// rectangle, so text further along the baseline stays.
			to := maxf(tp.rect.URX, w.x1) + 0.5
			if wi+1 < len(l.words) {
				to = l.words[wi+1].x0 - 0.02
			}
			shifts = append(shifts, lineShift{y: l.y, tol: 0.3 * l.size, fromX: w.x0, toX: to, dx: dx})
		}
	}
	if len(shifts) == 0 {
		return nil
	}
	p := tp.page
	data, err := p.contentStreams()
	if err != nil {
		return err
	}
	old := contentStreamIDs(p.pageDict()["/Contents"])
	data, err = shiftLineText(data, p.doc.objects, p.pageResources(), shifts)
	if err != nil {
		return fmt.Errorf("set horizontal alignment: %w", err)
	}
	if err := replacePageContents(p, data); err != nil {
		return fmt.Errorf("set horizontal alignment: %w", err)
	}
	p.doc.dropUnreachable(old)
	for li := range tp.lines {
		for wi := range tp.lines[li].words {
			w := &tp.lines[li].words[wi]
			dx := targets[li][wi] - w.x0
			w.x0 += dx
			w.x1 += dx
		}
	}
	return nil
}

// alignedWordStarts returns where each word of l starts under alignment a in r.
func alignedWordStarts(l paraTextLine, r Rectangle, a HorizontalAlignment, last bool) []float64 {
	n := len(l.words)
	out := make([]float64, n)
	lineW := l.words[n-1].x1 - l.words[0].x0
	justify := (a == HorizontalAlignmentJustify && !last) || a == HorizontalAlignmentFullJustify
	var start, extra float64
	switch {
	case justify && n > 1:
		// Every gap between words grows (or shrinks) by the same amount.
		start = r.LLX
		extra = (r.URX - r.LLX - lineW) / float64(n-1)
	case a == HorizontalAlignmentRight:
		start = r.URX - lineW
	case a == HorizontalAlignmentCenter:
		start = (r.LLX+r.URX)/2 - lineW/2
	default: // Left, a justified paragraph's last line, a one-word line
		start = r.LLX
	}
	x := start
	for i, w := range l.words {
		if i > 0 {
			x += w.x0 - l.words[i-1].x1 + extra
		}
		out[i] = x
		x += w.x1 - w.x0
	}
	return out
}
