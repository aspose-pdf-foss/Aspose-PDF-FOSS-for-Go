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

	for _, r := range columnRegions(frags, pageW) {
		cols := r.cols
		if len(cols) == 0 {
			cols = [][2]float64{{0, pageW}}
		}
		buckets := make([][]textFragment, len(cols))
		for _, f := range r.frags {
			buckets[assignColumn(f, cols)] = append(buckets[assignColumn(f, cols)], f)
		}
		for _, cf := range buckets {
			if len(cf) == 0 {
				continue
			}
			lines := groupFragmentsIntoLines(cf)
			paras := groupLinesToParagraphs(lines, r.typical)
			if len(paras) == 0 {
				continue
			}
			out.Sections = append(out.Sections, MarkupSection{
				Rectangle:  paragraphsBBox(paras),
				Paragraphs: paras,
			})
		}
	}
	return out, nil
}

// columnRegion is a part of a page, top to bottom, with its own columns.
type columnRegion struct {
	frags []textFragment
	cols  [][2]float64 // nil or one: a single column
	// typical is the line spacing paragraphs are judged by; zero means the
	// region's own.
	typical float64
}

// columnRegions divides a page into regions with their own columns. Columns
// running the height of the page win; failing those, the page is cut into
// bands at vertical gaps wider than 1.8 line spacings and each band looks for
// columns of its own, so a letterhead whose address and sender blocks sit side
// by side above full-width text is read block by block (pdf-go-dfax). Bands
// without columns are joined back into one region judged by the page's line
// spacing, so a page with no columns anywhere reads as it did.
func columnRegions(frags []textFragment, pageW float64) []columnRegion {
	if cols := detectColumns(frags, pageW); len(cols) > 1 {
		return []columnRegion{{frags: frags, cols: cols}}
	}
	whole := []columnRegion{{frags: frags}}
	lines := groupFragmentsIntoLines(append([]textFragment(nil), frags...))
	if len(lines) < 2 {
		return whole
	}
	gaps := make([]float64, 0, len(lines)-1)
	for i := 1; i < len(lines); i++ {
		gaps = append(gaps, lines[i-1].Y-lines[i].Y)
	}
	typical := medianFloat(gaps)
	if typical <= 0 {
		return whole
	}
	var cuts []float64 // descending
	for i := 1; i < len(lines); i++ {
		if lines[i-1].Y-lines[i].Y > 1.8*typical {
			cuts = append(cuts, (lines[i-1].Y+lines[i].Y)/2)
		}
	}
	if len(cuts) == 0 {
		return whole
	}
	bands := make([][]textFragment, len(cuts)+1)
	for _, f := range frags {
		k := sort.Search(len(cuts), func(i int) bool { return f.y > cuts[i] })
		bands[k] = append(bands[k], f)
	}
	var out []columnRegion
	found := false
	for _, b := range bands {
		if len(b) == 0 {
			continue
		}
		if cols := detectColumns(b, pageW); len(cols) > 1 && blocksStandApart(b, cols) {
			out = append(out, columnRegion{frags: b, cols: cols})
			found = true
			continue
		}
		if n := len(out); n > 0 && len(out[n-1].cols) == 0 {
			out[n-1].frags = append(out[n-1].frags, b...)
			continue
		}
		out = append(out, columnRegion{frags: b, typical: typical})
	}
	if !found {
		return whole
	}
	return out
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
//
// The histogram counts DISTINCT LINES per bin, not raw fragments, and (on
// pages with enough lines) a bin crossed by a single line still reads as
// empty: a centered running head ("1226  Index") straddling the gutter of a
// two-column page must not conceal it — one straddling line against dozens
// of column lines is decoration, and without the split the two columns
// interleave in reading order.
func detectColumns(frags []textFragment, pageW float64) [][2]float64 {
	const bins = 240
	binW := pageW / bins
	if binW <= 0 {
		return nil
	}
	lineSets := make([]map[int]struct{}, bins)
	counts := make([]int, bins)
	pageLines := map[int]struct{}{}
	for _, f := range frags {
		x0, x1 := int(f.x/binW), int(f.endX/binW)
		if x0 < 0 {
			x0 = 0
		}
		if x1 > bins-1 {
			x1 = bins - 1
		}
		key := int(f.y + 0.5)
		pageLines[key] = struct{}{}
		for b := x0; b <= x1; b++ {
			if lineSets[b] == nil {
				lineSets[b] = map[int]struct{}{}
			}
			if _, seen := lineSets[b][key]; !seen {
				lineSets[b][key] = struct{}{}
				counts[b]++
			}
		}
	}
	minLines := 1
	if len(pageLines) >= 6 {
		minLines = 2
	}
	occ := make([]bool, bins)
	for b := range occ {
		occ[b] = counts[b] >= minLines
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
		if e-b >= gutterMin && gutterSeparates(frags, float64(b)*binW, float64(e)*binW) {
			cols = append(cols, [2]float64{float64(segStart) * binW, float64(b) * binW})
			segStart = e
		}
		b = e
	}
	cols = append(cols, [2]float64{float64(segStart) * binW, float64(last+1) * binW})
	return cols
}

// blocksStandApart reports whether a band's columns are blocks set beside one
// another rather than the cells of a table or the labels of a list: in a table
// nearly every line has text in several columns, beside blocks most lines keep
// to one, and every block has at least two lines (a list item's label has
// one). A band with more than a third of its lines spanning columns, or with a
// column of a single line, reads row by row.
func blocksStandApart(frags []textFragment, cols [][2]float64) bool {
	lines := map[int]int{}                    // line → column of its first fragment, or -1 when it spans several
	perCol := make([]map[int]bool, len(cols)) // column → the lines with text in it
	for i := range frags {
		f := &frags[i]
		if strings.TrimSpace(f.text.String()) == "" {
			continue
		}
		key, c := int(f.y+0.5), assignColumn(*f, cols)
		if perCol[c] == nil {
			perCol[c] = map[int]bool{}
		}
		perCol[c][key] = true
		if prev, ok := lines[key]; !ok {
			lines[key] = c
		} else if prev != c {
			lines[key] = -1
		}
	}
	for _, l := range perCol {
		if len(l) < 2 {
			return false
		}
	}
	spanning := 0
	for _, c := range lines {
		if c < 0 {
			spanning++
		}
	}
	return 3*spanning <= len(lines)
}

// gutterSeparates reports whether the gap [x0, x1] parts columns. It does not
// when a line runs across it and otherwise the text either side never stands
// side by side — no line has text on both sides, and the lines entirely on the
// left and those entirely on the right share no height: a paragraph whose
// first line opens at the left margin and whose rest sits indented below is
// one block (pdf-go-dfax). With no line across, the gap stands as before.
func gutterSeparates(frags []textFragment, x0, x1 float64) bool {
	type sides struct{ left, right, cross bool }
	lines := map[int]*sides{}
	mid := (x0 + x1) / 2
	for i := range frags {
		f := &frags[i]
		if f.text.Len() == 0 {
			continue
		}
		key := int(f.y + 0.5)
		s := lines[key]
		if s == nil {
			s = &sides{}
			lines[key] = s
		}
		switch {
		case f.x < x0 && f.endX > x1:
			s.cross = true
		case (f.x+f.endX)/2 < mid:
			s.left = true
		default:
			s.right = true
		}
	}
	lo := [2]float64{1e18, -1e18} // y range of the lines only on the left
	ro := [2]float64{1e18, -1e18}
	crossed := false
	for y, s := range lines {
		fy := float64(y)
		switch {
		case s.cross:
			crossed = true
		case s.left && s.right:
			return true
		case s.left:
			lo = [2]float64{minf(lo[0], fy), maxf(lo[1], fy)}
		case s.right:
			ro = [2]float64{minf(ro[0], fy), maxf(ro[1], fy)}
		}
	}
	return !crossed || (lo[0] <= ro[1] && ro[0] <= lo[1])
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
// significant font-size change (a heading vs body text). A typical spacing of
// zero means the median of these lines' own.
func groupLinesToParagraphs(lines []TextLine, typical float64) []MarkupParagraph {
	if len(lines) == 0 {
		return nil
	}
	if typical <= 0 {
		gaps := make([]float64, 0, len(lines))
		for i := 1; i < len(lines); i++ {
			gaps = append(gaps, lines[i-1].Y-lines[i].Y)
		}
		typical = medianFloat(gaps)
	}

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
