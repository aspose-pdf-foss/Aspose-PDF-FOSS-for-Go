// SPDX-License-Identifier: MIT

package asposepdf

import (
	"sort"
	"strings"
)

// Structural (paragraph) text extraction — the counterpart to the flat
// ExtractText / ExtractTextWithLayout that groups a page's text into columns
// (sections) and paragraphs. Mirrors the intent of Aspose.PDF for .NET's
// ParagraphAbsorber (PageMarkup → MarkupSection → MarkupParagraph).
//
// Built on the same layout pipeline as ExtractTextWithLayout: fragments are
// clustered into columns by a horizontal occupancy histogram (a wide vertical
// gap with text on both sides is a column gutter), then each column's lines are
// grouped into paragraphs by the vertical gap between baselines and font-size
// changes. Heuristic — figures/tables and irregular layouts may split or merge;
// for prose it recovers the paragraph/column structure well.

// MarkupParagraph is a run of consecutive lines forming one paragraph.
type MarkupParagraph struct {
	Text      string     // the paragraph text (its lines joined with spaces)
	Rectangle Rectangle  // bounding box in PDF user space
	Lines     []TextLine // the paragraph's lines (with per-fragment positions)
}

// MarkupSection is a column of paragraphs (left-to-right across the page).
type MarkupSection struct {
	Rectangle  Rectangle
	Paragraphs []MarkupParagraph
}

// PageMarkup is the structured text of one page.
type PageMarkup struct {
	PageNumber int
	Sections   []MarkupSection
}

// Paragraphs returns the page's text grouped into columns (sections) and
// paragraphs. Mirrors Aspose.PDF for .NET's ParagraphAbsorber.
func (p *Page) Paragraphs() (PageMarkup, error) {
	out := PageMarkup{PageNumber: p.Number()}
	frags, err := p.pageFragments()
	if err != nil {
		return out, err
	}
	if len(frags) == 0 {
		return out, nil
	}
	pageW := 612.0
	if sz, err := p.Size(); err == nil && sz.Width > 0 {
		pageW = sz.Width
	}

	cols := detectColumns(frags, pageW)
	if len(cols) == 0 {
		cols = [][2]float64{{0, pageW}}
	}
	buckets := make([][]textFragment, len(cols))
	for _, f := range frags {
		buckets[assignColumn(f, cols)] = append(buckets[assignColumn(f, cols)], f)
	}
	for _, cf := range buckets {
		if len(cf) == 0 {
			continue
		}
		lines := groupFragmentsIntoLines(cf)
		paras := groupLinesToParagraphs(lines)
		if len(paras) == 0 {
			continue
		}
		out.Sections = append(out.Sections, MarkupSection{
			Rectangle:  paragraphsBBox(paras),
			Paragraphs: paras,
		})
	}
	return out, nil
}

// Paragraphs returns the structured text of every page.
func (d *Document) Paragraphs() ([]PageMarkup, error) {
	pages := d.Pages()
	out := make([]PageMarkup, len(pages))
	for i, p := range pages {
		pm, err := p.Paragraphs()
		if err != nil {
			return nil, err
		}
		out[i] = pm
	}
	return out, nil
}

// pageFragments runs the text extractor and returns the raw fragments (the same
// ones ExtractTextWithLayout groups into lines).
func (p *Page) pageFragments() ([]textFragment, error) {
	data, err := p.contentStreams()
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	ops, err := parseContentStream(data)
	if err != nil {
		return nil, err
	}
	resources := p.pageResources()
	fonts := resolveFontResources(p.doc.objects, resources)
	ext := newTextExtractor(p.doc.objects, fonts)
	ext.process(ops, resources)
	ext.flushFragment()
	return ext.fragments, nil
}

// detectColumns clusters fragments into columns via a horizontal occupancy
// histogram: an internal run of empty bins at least ~4.5% of the page wide, with
// text on both sides, is a column gutter. Returns each column's [minX, maxX],
// left-to-right. A single column is returned when there is no such gutter.
func detectColumns(frags []textFragment, pageW float64) [][2]float64 {
	const bins = 240
	binW := pageW / bins
	if binW <= 0 {
		return nil
	}
	occ := make([]bool, bins)
	for _, f := range frags {
		x0, x1 := int(f.x/binW), int(f.endX/binW)
		if x0 < 0 {
			x0 = 0
		}
		if x1 > bins-1 {
			x1 = bins - 1
		}
		for b := x0; b <= x1; b++ {
			occ[b] = true
		}
	}
	first, last := -1, -1
	for b := 0; b < bins; b++ {
		if occ[b] {
			if first < 0 {
				first = b
			}
			last = b
		}
	}
	if first < 0 {
		return nil
	}
	gutterMin := int(pageW * 0.045 / binW)
	if gutterMin < 2 {
		gutterMin = 2
	}
	var cols [][2]float64
	segStart := first
	b := first
	for b <= last {
		if occ[b] {
			b++
			continue
		}
		e := b
		for e <= last && !occ[e] {
			e++
		}
		if e-b >= gutterMin {
			cols = append(cols, [2]float64{float64(segStart) * binW, float64(b) * binW})
			segStart = e
		}
		b = e
	}
	cols = append(cols, [2]float64{float64(segStart) * binW, float64(last+1) * binW})
	return cols
}

// assignColumn returns the index of the column containing (or nearest to) the
// fragment's horizontal centre.
func assignColumn(f textFragment, cols [][2]float64) int {
	cx := (f.x + f.endX) / 2
	best, bestD := 0, 1e18
	for i, c := range cols {
		if cx >= c[0] && cx < c[1] {
			return i
		}
		d := absf(cx - c[0])
		if e := absf(cx - c[1]); e < d {
			d = e
		}
		if d < bestD {
			bestD, best = d, i
		}
	}
	return best
}

// groupLinesToParagraphs groups top-to-bottom lines into paragraphs, starting a
// new paragraph on a vertical gap larger than 1.6× the typical line spacing or a
// significant font-size change (a heading vs body text).
func groupLinesToParagraphs(lines []TextLine) []MarkupParagraph {
	if len(lines) == 0 {
		return nil
	}
	gaps := make([]float64, 0, len(lines))
	for i := 1; i < len(lines); i++ {
		gaps = append(gaps, lines[i-1].Y-lines[i].Y)
	}
	typical := medianFloat(gaps)

	var paras []MarkupParagraph
	cur := []TextLine{lines[0]}
	for i := 1; i < len(lines); i++ {
		gap := lines[i-1].Y - lines[i].Y
		prevFS, curFS := lineFontSize(lines[i-1]), lineFontSize(lines[i])
		newPara := (typical > 0 && gap > typical*1.6) ||
			curFS > prevFS*1.3 || curFS < prevFS*0.7
		if newPara {
			paras = append(paras, makeParagraph(cur))
			cur = nil
		}
		cur = append(cur, lines[i])
	}
	if len(cur) > 0 {
		paras = append(paras, makeParagraph(cur))
	}
	return paras
}

// makeParagraph assembles a paragraph from its lines.
func makeParagraph(lines []TextLine) MarkupParagraph {
	parts := make([]string, 0, len(lines))
	for _, l := range lines {
		if t := strings.TrimSpace(l.Text); t != "" {
			parts = append(parts, t)
		}
	}
	return MarkupParagraph{
		Text:      strings.Join(parts, " "),
		Rectangle: linesBBox(lines),
		Lines:     lines,
	}
}

// lineFontSize returns a line's representative font size (its first fragment's).
func lineFontSize(l TextLine) float64 {
	if len(l.Fragments) > 0 && l.Fragments[0].FontSize > 0 {
		return l.Fragments[0].FontSize
	}
	return 12
}

// linesBBox returns the bounding box of every fragment in the lines.
func linesBBox(lines []TextLine) Rectangle {
	first := true
	var r Rectangle
	for _, l := range lines {
		for _, f := range l.Fragments {
			fr := Rectangle{LLX: f.X, LLY: f.Y, URX: f.X + f.Width, URY: f.Y + f.Height}
			if first {
				r, first = fr, false
				continue
			}
			r = unionRect(r, fr)
		}
	}
	return r
}

// paragraphsBBox returns the bounding box over a set of paragraphs.
func paragraphsBBox(paras []MarkupParagraph) Rectangle {
	first := true
	var r Rectangle
	for _, p := range paras {
		if first {
			r, first = p.Rectangle, false
			continue
		}
		r = unionRect(r, p.Rectangle)
	}
	return r
}

func unionRect(a, b Rectangle) Rectangle {
	return Rectangle{
		LLX: minf(a.LLX, b.LLX), LLY: minf(a.LLY, b.LLY),
		URX: maxf(a.URX, b.URX), URY: maxf(a.URY, b.URY),
	}
}

func medianFloat(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return s[len(s)/2]
}

func absf(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
