// SPDX-License-Identifier: MIT

package asposepdf

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// errReflowLost reports replaced text not found in the paragraph reflowed.
var errReflowLost = errors.New("reflow: the replaced text is not in its paragraph")

// Paragraph reflow (pdf-go-1vv8): Aspose.PDF for .NET's WholeWordsHyphenation
// re-wraps the whole paragraph a replacement lands in, so the rest of the
// paragraph flows on after the new text. reflowParagraph finds that
// paragraph, and the reflow draws it again with the replacement in place,
// each run in its own font, size and colour, wrapped to the paragraph's
// width, aligned as it was.

// reflowPara is a paragraph of a page found for a reflow: its lines in
// reading order (only the fragments of the cell it lies in) and its box.
type reflowPara struct {
	lines []TextLine
	box   Rectangle
}

// reflowCell is the cell of a ruled table a reflowed paragraph lies in;
// a side with no rule is 0.
type reflowCell struct{ lo, hi, bottom, top float64 }

// pageReflowCell returns the cell of p's ruled table holding (x, y): the
// vertical rules either side (as columnRules.cell) and the horizontal rules
// above and below that cross x.
func pageReflowCell(p *Page, x, y float64) reflowCell {
	h, v, _ := pageRules(p)
	var c reflowCell
	c.lo, c.hi = columnRules(normalizeRules(v)).cell(x, y)
	top, bottom := math.Inf(1), math.Inf(-1)
	for _, r := range normalizeRules(h) {
		if x < r.lo-cellTol || x > r.hi+cellTol {
			continue
		}
		switch {
		case r.pos > y && r.pos < top:
			top = r.pos
		case r.pos <= y && r.pos > bottom:
			bottom = r.pos
		}
	}
	if !math.IsInf(top, 1) {
		c.top = top
	}
	if !math.IsInf(bottom, -1) {
		c.bottom = bottom
	}
	return c
}

// reflowItemRe matches a line opening a list item: "(a)", "1.", "b)", a
// bullet (brackets either way round: a right-to-left line's are mirrored).
// Such a line starts a paragraph of its own, as an item is.
var reflowItemRe = regexp.MustCompile(`^\s*([()]\s*[\p{L}\p{N}]{1,3}\s*[()]|[\p{L}\p{N}]{1,3}[.)]|[•◦▪‣∙·–—*-])\s`)

// reflowParagraphAt returns the paragraph of frags holding the point (x, y),
// a baseline point of the replaced text, reading only the fragments that
// start in cell and that the clip shows. A line opening a list item starts
// a paragraph. ok is false when no paragraph holds it.
func reflowParagraphAt(frags []textFragment, x, y float64, cell reflowCell, rtl bool) (reflowPara, bool) {
	var in []textFragment
	for _, f := range frags {
		if (cell.lo != 0 && f.x < cell.lo-cellTol) || (cell.hi != 0 && f.x >= cell.hi-cellTol) {
			continue
		}
		if (cell.top != 0 && f.y > cell.top) || (cell.bottom != 0 && f.y < cell.bottom) {
			continue
		}
		if len(f.runeHidden) > 0 && f.runeHidden[0] {
			continue
		}
		if f.rotation != 0 {
			continue
		}
		in = append(in, f)
	}
	if len(in) == 0 {
		return reflowPara{}, false
	}
	lines := groupFragmentsIntoLines(in)
	for _, p := range groupLinesToParagraphs(lines, 0) {
		for _, part := range splitWideGaps(splitListItems(p.Lines, rtl)) {
			for _, l := range part {
				if math.Abs(l.Y-y) > 0.5*lineSize(l) {
					continue
				}
				for _, f := range l.Fragments {
					if x >= f.X-cellTol && x <= f.X+f.Width+cellTol {
						return reflowPara{lines: part, box: linesBox(part)}, true
					}
				}
			}
		}
	}
	return reflowPara{}, false
}

// splitListItems cuts lines before every line after the first that opens a
// list item, read in the paragraph's direction.
func splitListItems(lines []TextLine, rtl bool) [][]TextLine {
	var out [][]TextLine
	start := 0
	for i := 1; i < len(lines); i++ {
		text := lines[i].Text
		if rtl {
			text = string(rtlFragmentRuneMap(&lines[i], nil).text)
		}
		if reflowItemRe.MatchString(strings.TrimSpace(text) + " ") {
			out = append(out, lines[start:i])
			start = i
		}
	}
	return append(out, lines[start:])
}

// linesBox is the union of the text boxes of lines' fragments.
func linesBox(lines []TextLine) Rectangle {
	var r Rectangle
	first := true
	for _, l := range lines {
		for _, f := range l.Fragments {
			b := Rectangle{LLX: f.X, LLY: f.Y + f.descent, URX: f.X + f.Width, URY: f.Y + f.descent + f.Height}
			if first {
				r, first = b, false
			} else {
				r = unionRect(r, b)
			}
		}
	}
	return r
}

// lineSize is the largest font size on l.
func lineSize(l TextLine) float64 {
	s := 0.0
	for _, f := range l.Fragments {
		s = math.Max(s, f.FontSize)
	}
	if s == 0 {
		s = 10
	}
	return s
}

// reflowEdit is one replacement in a reflowed paragraph: the boxes of the
// text it replaces and the runs that take its place.
type reflowEdit struct {
	rects []Rectangle
	runs  []reflowRun
	done  bool
}

// reflowRun is a run of the reflowed paragraph: its text and text state.
// A state found on the page names its font as a documentFont, resolved to a
// drawable font by reflowPlan.
type reflowRun struct {
	text string
	st   TextState
	edit bool // the replacement's, not the paragraph's own
}

// hits reports whether the glyph starting at (x, y) is replaced by e.
func (e *reflowEdit) hits(x, y float64) bool {
	for _, r := range e.rects {
		if x >= r.LLX-0.01 && x < r.URX-0.01 && y >= r.LLY-0.01 && y <= r.URY+0.01 {
			return true
		}
	}
	return false
}

// paragraphRuns reads para's text in logical order as styled runs (as the
// absorber finds them), with the text each edit hits replaced by its runs,
// the lines joined by a space and white space collapsed. It fails when an
// edit's text is not in the paragraph.
func paragraphRuns(p *Page, para reflowPara, edits []*reflowEdit, rtl bool) ([]reflowRun, error) {
	a := &TextFragmentAbsorber{}
	num := p.Number()
	byNum := map[int]*Page{num: p}
	fills := map[int][]shadedRect{}
	cols := map[int]columnRules{num: nil}
	var out []reflowRun
	for li := range para.lines {
		line := &para.lines[li]
		m := buildLineRuneMapBase(line, 0)
		if rtl {
			m = rtlFragmentRuneMap(line, p.doc.ownFont)
		}
		owner := make([]*reflowEdit, len(m.owner))
		for k, fi := range m.owner {
			if fi < 0 {
				continue
			}
			f := &line.Fragments[fi]
			x := f.X
			if l := m.local[k]; l >= 0 && l < len(f.runeX) {
				x = f.runeX[l]
			}
			if r, _ := utf8.DecodeRune(m.text[m.runeByte[k]:]); unicode.IsSpace(r) {
				continue // white space around the replaced text stays
			}
			for _, e := range edits {
				if e.hits(x+0.1, f.Y) {
					owner[k] = e
					break
				}
			}
		}
		// An inserted space between two runes of one edit is the edit's.
		for k := range owner {
			if m.owner[k] < 0 && k > 0 && k+1 < len(owner) && owner[k-1] != nil && owner[k-1] == owner[k+1] {
				owner[k] = owner[k-1]
			}
		}
		if li > 0 {
			out = append(out, reflowRun{text: " ", st: lastState(out)})
		}
		for k := 0; k < len(owner); {
			j := k + 1
			for j < len(owner) && owner[j] == owner[k] {
				j++
			}
			if e := owner[k]; e != nil {
				if !e.done {
					out = append(out, e.runs...)
					e.done = true
				}
			} else {
				sm := spanMatch{text: string(m.text[m.runeByte[k]:byteEnd(m, j)]), spans: []lineSpan{{page: num, line: line, m: m, r0: k, r1: j}}}
				if f := a.build(sm, byNum, fills, cols); f != nil {
					for _, s := range f.Segments {
						out = append(out, reflowRun{text: s.Text, st: s.TextState})
					}
				} else {
					out = append(out, reflowRun{text: sm.text, st: lastState(out)})
				}
			}
			k = j
		}
	}
	for _, e := range edits {
		if !e.done {
			return nil, errReflowLost
		}
	}
	return collapseRunSpaces(out), nil
}

// byteEnd is the byte offset where rune j of m starts (the text's end for
// j past the last rune).
func byteEnd(m lineRuneMap, j int) int {
	if j < len(m.runeByte) {
		return m.runeByte[j]
	}
	return len(m.text)
}

// lastState is the state of the last run of runs (the zero state for none).
func lastState(runs []reflowRun) TextState {
	if len(runs) == 0 {
		return TextState{}
	}
	return runs[len(runs)-1].st
}

// collapseRunSpaces collapses every run of white space across runs into one
// space and drops it at both ends and empty runs.
func collapseRunSpaces(runs []reflowRun) []reflowRun {
	out := runs[:0]
	space := true // at the start: drop leading space
	for _, r := range runs {
		var b strings.Builder
		for _, c := range r.text {
			if unicode.IsSpace(c) && c != 0xA0 {
				if !space {
					b.WriteByte(' ')
					space = true
				}
				continue
			}
			b.WriteRune(c)
			space = false
		}
		if b.Len() > 0 {
			r.text = b.String()
			out = append(out, r)
		}
	}
	for len(out) > 0 {
		last := &out[len(out)-1]
		last.text = strings.TrimRight(last.text, " ")
		if last.text != "" {
			break
		}
		out = out[:len(out)-1]
	}
	return out
}

// reflowLayout is a paragraph laid out again, ready to draw.
type reflowLayout struct {
	runs    []reflowRun // fonts drawable
	styles  []TextStyle
	ms      []runMetrics
	asc     []float64
	desc    []float64
	lines   []runLine
	rtl     bool
	justify bool
	// The lines' edges: the first line's and the others' start (left
	// for left to right, right for right to left) and the far edge.
	firstStart, restStart, far float64
	baseline, pitch            float64
	// own reports that the paragraph keeps text of its own beside the
	// replacements; one the replacements cover whole is laid out as a
	// replacement alone is (drawReplacementParagraph), nothing being left
	// to flow on.
	own bool
}

// reflowLineExtent is l's text from its first to its last non-space glyph.
func reflowLineExtent(l TextLine) (left, right float64) {
	left, right = math.Inf(1), math.Inf(-1)
	for _, f := range l.Fragments {
		k := 0
		runes := []rune(f.Text)
		for k = range runes {
			if !unicode.IsSpace(runes[k]) {
				break
			}
		}
		if strings.TrimSpace(f.Text) == "" {
			continue
		}
		start := f.X
		if k < len(f.runeX) {
			start = f.runeX[k]
		}
		end := f.X + f.Width
		j := len(runes)
		for j > 0 && unicode.IsSpace(runes[j-1]) {
			j--
		}
		if j < len(runes) && j < len(f.runeX) {
			end = f.runeX[j]
		}
		left, right = math.Min(left, start), math.Max(right, end)
	}
	return left, right
}

// planReflow lays para out again with edits in place: the paragraph's runs
// in their own fonts (the page's where they cover the text, as an edit of a
// found fragment draws), wrapped to the paragraph's width — widened to its
// cell, keeping the margin it has on the near side — with its first-line
// and hanging indents, its line pitch, justified when the source was. rtl is
// the paragraph's direction. Nothing is drawn.
func (d *Document) planReflow(p *Page, para reflowPara, cell reflowCell, edits []*reflowEdit, rtl bool, fonts *replaceFonts) (*reflowLayout, error) {
	runs, err := paragraphRuns(p, para, edits, rtl)
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return &reflowLayout{}, nil
	}
	own := false
	for _, r := range runs {
		own = own || (!r.edit && strings.TrimSpace(r.text) != "")
	}
	// The fonts: the page's where they cover the text.
	var snap fragmentSnapshot
	for _, r := range runs {
		snap.segments = append(snap.segments, segmentSnapshot{text: r.text, state: r.st})
	}
	lay := &reflowLayout{runs: runs, rtl: rtl, own: own}
	textRuns := make([]textRun, len(runs))
	for i := range lay.runs {
		r := &lay.runs[i]
		sn := &snap
		if hasRTLScript(r.text) {
			// Right-to-left text is drawn again from its letters, shaped
			// in a font of its family: the page's font holds the forms the
			// producer shaped, and drawing letters through it loses them.
			// The spacing was set for the producer's glyphs, not these.
			r.text = unshapeArabic(r.text)
			r.st.CharacterSpacing, r.st.WordSpacing = 0, 0
			sn = nil
		}
		f, err := d.drawingFont(r.st, r.text, fonts, sn, NoCharacterActionFontReplace)
		if err != nil {
			return nil, err
		}
		r.st.Font = f
		st := r.st.resolved()
		style := st.style()
		style.Background = nil
		asc, desc, err := textBox(style.Font, st.FontSize)
		if err != nil {
			return nil, err
		}
		m, err := metricsFor(style, r.text)
		if err != nil {
			return nil, err
		}
		if tc, tw, tz := st.CharacterSpacing, st.WordSpacing, st.HorizontalScaling; tc != 0 || tw != 0 || (tz != 0 && tz != 100) {
			font, rtl := style.Font, style.RTL
			m.adjust = func(text string, w float64) float64 {
				return spacedWidth(w, glyphSpacingText(font, text, rtl), tc, tw, tz)
			}
		}
		lay.styles = append(lay.styles, style)
		lay.ms = append(lay.ms, m)
		lay.asc = append(lay.asc, asc)
		lay.desc = append(lay.desc, desc)
		textRuns[i] = textRun{text: r.text, style: style}
	}

	// Geometry, from the source lines.
	n := len(para.lines)
	type ext struct{ l, r float64 }
	exts := make([]ext, n)
	minL, maxR := math.Inf(1), math.Inf(-1)
	for i, l := range para.lines {
		exts[i].l, exts[i].r = reflowLineExtent(l)
		minL, maxR = math.Min(minL, exts[i].l), math.Max(maxR, exts[i].r)
	}
	if math.IsInf(minL, 0) || math.IsInf(maxR, 0) {
		return &reflowLayout{}, nil
	}
	reach := 0
	tol := math.Max(3, 0.02*(maxR-minL))
	for i := 0; i+1 < n; i++ {
		if (!rtl && exts[i].r >= maxR-tol) || (rtl && exts[i].l <= minL+tol) {
			reach++
		}
	}
	lay.justify = reach >= 2 || (n == 2 && reach == 1)
	lay.baseline = para.lines[0].Y
	lay.pitch = 1.2 * lineSize(para.lines[0])
	if n > 1 {
		lay.pitch = (para.lines[0].Y - para.lines[n-1].Y) / float64(n-1)
	}
	second := 0
	if n > 1 {
		second = 1
	}
	box, err := p.CropBox()
	haveBox := err == nil
	var firstW, restW float64
	if !rtl {
		far := maxR
		switch {
		case cell.lo != 0 && cell.hi != 0:
			far = math.Max(far, cell.hi-(minL-cell.lo))
		case n == 1 && haveBox:
			// One line says nothing of the column's width: the free space
			// to the page's edge less the margin before it, as Aspose lays
			// out a replacement.
			far = math.Max(far, box.URX-(minL-box.LLX))
		}
		lay.firstStart, lay.restStart, lay.far = exts[0].l, exts[second].l, far
		firstW, restW = far-exts[0].l, far-exts[second].l
	} else {
		far := minL
		switch {
		case cell.lo != 0 && cell.hi != 0:
			far = math.Min(far, cell.lo+(cell.hi-maxR))
		case n == 1 && haveBox:
			far = math.Min(far, box.LLX+(box.URX-maxR))
		}
		lay.firstStart, lay.restStart, lay.far = exts[0].r, exts[second].r, far
		firstW, restW = exts[0].r-far, exts[second].r-far
	}
	if !(firstW > 0) || !(restW > 0) {
		return nil, errors.New("reflow: the paragraph has no width")
	}
	lay.lines = layoutRunsOpt(textRuns, lay.ms, restW, firstW, false)
	return lay, nil
}

// draw draws the laid-out paragraph on p and returns the boxes drawn.
func (lay *reflowLayout) draw(p *Page) ([]QuadPoint, error) {
	var drawn []QuadPoint
	base := 0
	if lay.rtl {
		base = 1
	}
	for li, ln := range lay.lines {
		baseline := lay.baseline - float64(li)*lay.pitch
		start := lay.restStart
		if li == 0 {
			start = lay.firstStart
		}
		avail := math.Abs(lay.far - start)
		pieces, _ := linePieces(ln, lay.ms, true)
		if len(pieces) == 0 {
			continue
		}
		levels := make([]int, len(pieces))
		for i, pc := range pieces {
			levels[i] = pieceLevel(pc.text, base)
		}
		order := bidiReorderIndices(levels)
		gapAfter := make([]float64, len(pieces))
		typical := 0.0
		for i := 0; i+1 < len(pieces); i++ {
			gapAfter[i] = pieces[i+1].x - (pieces[i].x + pieces[i].w)
			typical += gapAfter[i]
		}
		if len(pieces) > 1 {
			typical /= float64(len(pieces) - 1)
		}
		gaps := make([]float64, len(order)) // before each visual piece
		stretch := 0                        // the gaps justification widens: word spaces
		for k := 1; k < len(order); k++ {
			a, b := order[k-1], order[k]
			if a > b {
				a, b = b, a
			}
			gaps[k] = typical
			if b == a+1 {
				gaps[k] = gapAfter[a]
			}
			if gaps[k] > 0.01 {
				stretch++
			}
		}
		lineW := 0.0
		for k, i := range order {
			lineW += gaps[k] + pieces[i].w
		}
		extra := 0.0
		last := li == len(lay.lines)-1 || ln.hard
		if lay.justify && !last && stretch > 0 && lineW < avail {
			extra = (avail - lineW) / float64(stretch)
			lineW = avail
		}
		x := start
		if lay.rtl {
			x = start - lineW
		}
		for k, i := range order {
			if k > 0 {
				x += gaps[k]
				if gaps[k] > 0.01 {
					x += extra
				}
			}
			pc := pieces[i]
			r := &lay.runs[pc.run]
			text := pc.text
			if levels[i]%2 == 1 && !hasRTLScript(text) {
				// Punctuation alone on a right-to-left level: drawn on its
				// own it has no context to be mirrored by.
				text = mirrorBrackets(text)
			}
			_, quad, _, err := drawTextPiece(p, r.st.resolved(), lay.styles[pc.run], nil, text, x, baseline, pc.w, lay.asc[pc.run], lay.desc[pc.run])
			if err != nil {
				return nil, err
			}
			drawn = append(drawn, quad)
			x += pc.w
		}
	}
	return drawn, nil
}

// pieceLevel is the bidi embedding level of a word drawn on a line of base
// level base: right to left when it holds a right-to-left letter, left to
// right when it holds a letter or digit, else the line's.
func pieceLevel(text string, base int) int {
	ltr := false
	for _, r := range text {
		switch bidiClass(r) {
		case clsR, clsAL:
			return 1
		case clsL, clsEN, clsAN:
			ltr = true
		}
	}
	switch {
	case ltr && base == 1:
		return 2
	case ltr:
		return 0
	}
	return base
}

// reflowGroup is a paragraph to reflow and the edits landing in it.
type reflowGroup struct {
	para  reflowPara
	cell  reflowCell
	rtl   bool
	edits []*reflowEdit
	lay   *reflowLayout
}

// findReflowGroups places each edit, at the baseline point (x, y) of its
// first glyph with direction rtl, in its paragraph, joining edits in one
// paragraph. An edit no paragraph holds gets a nil group entry.
func findReflowGroups(p *Page, frags []textFragment, at func(i int) (x, y float64, rtl bool), edits []*reflowEdit) ([]*reflowGroup, []*reflowGroup) {
	var groups []*reflowGroup
	byEdit := make([]*reflowGroup, len(edits))
	for i, e := range edits {
		x, y, rtl := at(i)
		cell := pageReflowCell(p, x, y)
		para, ok := reflowParagraphAt(frags, x+0.5, y, cell, rtl)
		if !ok {
			continue
		}
		var g *reflowGroup
		for _, o := range groups {
			if o.para.box == para.box && len(o.para.lines) == len(para.lines) {
				g = o
				break
			}
		}
		if g == nil {
			g = &reflowGroup{para: para, cell: cell, rtl: rtl}
			groups = append(groups, g)
		}
		g.edits = append(g.edits, e)
		byEdit[i] = g
	}
	return groups, byEdit
}

// regions is the boxes of the group's paragraph lines, removed before it is
// drawn again.
func (g *reflowGroup) regions() []QuadPoint {
	var out []QuadPoint
	for _, l := range g.para.lines {
		b := linesBox([]TextLine{l})
		b.LLX, b.LLY, b.URX, b.URY = b.LLX-0.5, b.LLY-0.5, b.URX+0.5, b.URY+0.5
		out = append(out, rectAsQuadPoint(b))
	}
	return out
}

// applyReflowToPage replaces the matches on p re-wrapping the paragraph each
// is in (ReplaceText with WholeWordsHyphenation). A match no paragraph
// holds is laid out alone, as before.
func applyReflowToPage(p *Page, matches []replaceMatch, replacement string, lay *replaceLayout, fonts *replaceFonts) (int, error) {
	frags, err := p.pageFragments()
	if err != nil {
		return 0, err
	}
	edits := make([]*reflowEdit, len(matches))
	for i, m := range matches {
		col := m.color
		col.A = 1
		e := &reflowEdit{rects: append([]Rectangle{m.rect}, m.more...)}
		if replacement != "" {
			e.runs = []reflowRun{{text: replacement, st: TextState{Font: m.font, FontSize: m.size, ForegroundColor: &col}, edit: true}}
		}
		edits[i] = e
	}
	groups, byEdit := findReflowGroups(p, frags, func(i int) (float64, float64, bool) {
		return matches[i].x, matches[i].y, matches[i].rtl
	}, edits)
	for _, g := range groups {
		if g.lay, err = p.doc.planReflow(p, g.para, g.cell, g.edits, g.rtl, fonts); err != nil {
			return 0, err
		}
	}
	groups = dropWholeGroups(groups, byEdit)
	var regions []QuadPoint
	for _, g := range groups {
		regions = append(regions, g.regions()...)
	}
	for i, m := range matches {
		if byEdit[i] == nil {
			regions = append(regions, rectAsQuadPoint(m.rect))
			for _, r := range m.more {
				regions = append(regions, rectAsQuadPoint(r))
			}
		}
	}
	data, err := stripTextInRegions(p, regions, nil)
	if err != nil {
		return 0, err
	}
	if err := replacePageContents(p, data); err != nil {
		return 0, fmt.Errorf("replace content: %w", err)
	}
	for _, g := range groups {
		if _, err := g.lay.draw(p); err != nil {
			return 0, fmt.Errorf("draw replacement: %w", err)
		}
	}
	if replacement != "" {
		for i, m := range matches {
			if byEdit[i] == nil {
				if err := drawReplacementParagraph(p, m, replacement, lay); err != nil {
					return 0, fmt.Errorf("draw replacement: %w", err)
				}
			}
		}
	}
	return len(matches), nil
}

// rtlFragmentRuneMap is the rune map of a line of a right-to-left paragraph
// read fragment by fragment, right to left, each fragment's text in its own
// logical order (a fragment of Arabic reversed, one of Latin kept), with a
// space where fragments stand apart. Reading the line as one bidi paragraph
// would join Latin placeholders standing side by side into one left-to-right
// run and swap them; each fragment is a run of its own, as Aspose reads it.
func rtlFragmentRuneMap(line *TextLine, shapeNamed func(ref int) bool) lineRuneMap {
	m := lineRuneMap{runeCounts: make([]int, len(line.Fragments))}
	idx := make([]int, 0, len(line.Fragments))
	for i, f := range line.Fragments {
		if f.Text != "" {
			idx = append(idx, i)
		}
	}
	sort.SliceStable(idx, func(a, b int) bool { return line.Fragments[idx[a]].X > line.Fragments[idx[b]].X })
	prevLeft, prevSize, prevText := 0.0, 0.0, ""
	for n, fi := range idx {
		f := &line.Fragments[fi]
		runes := []rune(f.Text)
		m.runeCounts[fi] = len(runes)
		if n > 0 {
			gap := prevLeft - (f.X + f.Width)
			if gap > math.Max(1, wordGapEm*math.Min(f.FontSize, prevSize)) && !edgeSpace(f.Text, prevText) {
				m.runeByte = append(m.runeByte, len(m.text))
				m.owner = append(m.owner, -1)
				m.local = append(m.local, -1)
				m.text = append(m.text, ' ')
			}
		}
		// Producers (Word) map a bracket drawn mirrored to the character it
		// stands for, so it is not mirrored back; this library's writer
		// maps it to the bracket drawn, and text in a font it embedded is
		// mirrored back as the algorithm does.
		mirrored, order := bidiVisualToLogical(runes)
		logical := mirrored
		if shapeNamed == nil || !shapeNamed(f.fontRef) {
			logical = make([]rune, len(order))
			for k, v := range order {
				logical[k] = runes[v]
			}
		}
		if bidiVisualBaseLevel(runes) == 0 {
			// A left-to-right fragment in a right-to-left line: the white
			// space at its right edge comes before it, at its left after.
			logical, order = flipEdgeSpaces(logical, order)
		}
		for k, r := range logical {
			m.runeByte = append(m.runeByte, len(m.text))
			m.owner = append(m.owner, fi)
			m.local = append(m.local, order[k])
			m.text = utf8.AppendRune(m.text, r)
		}
		prevLeft, prevSize, prevText = f.X, f.FontSize, f.Text
	}
	return m
}

// flipEdgeSpaces moves the white space at the start of runes to the end and
// that at the end to the start, with order.
func flipEdgeSpaces(runes []rune, order []int) ([]rune, []int) {
	a := 0
	for a < len(runes) && unicode.IsSpace(runes[a]) {
		a++
	}
	if a == len(runes) {
		return runes, order
	}
	b := len(runes)
	for b > a && unicode.IsSpace(runes[b-1]) {
		b--
	}
	outR := append(append(append([]rune(nil), runes[b:]...), runes[a:b]...), runes[:a]...)
	outO := append(append(append([]int(nil), order[b:]...), order[a:b]...), order[:a]...)
	return outR, outO
}

// arabicBase maps an Arabic presentation form (Forms-B) back to the letter
// or letters it shows.
var arabicBase = func() map[rune]string {
	m := map[rune]string{}
	for base, f := range arabicShapeTable {
		for _, r := range []rune{f.iso, f.fin, f.ini, f.med} {
			if r != 0 {
				m[r] = string(base)
			}
		}
	}
	for alef, forms := range lamAlefLigature {
		for _, r := range forms {
			m[r] = string([]rune{0x0644, alef})
		}
	}
	return m
}()

// unshapeArabic turns Arabic presentation forms in s back into letters, so
// the text is shaped again when drawn.
func unshapeArabic(s string) string {
	var b strings.Builder
	for _, r := range s {
		if base, ok := arabicBase[r]; ok {
			b.WriteString(base)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// hasRTLScript reports whether s holds a right-to-left letter.
func hasRTLScript(s string) bool {
	for _, r := range s {
		if c := bidiClass(r); c == clsR || c == clsAL {
			return true
		}
	}
	return false
}

// mirrorBrackets replaces every paired bracket in s by its mirror image.
func mirrorBrackets(s string) string {
	runes := []rune(s)
	for i, r := range runes {
		runes[i] = bidiMirror(r)
	}
	return string(runes)
}

// ownFont reports whether ref is the font object of a font this document
// embedded (LoadFont, LoadFontByName).
func (d *Document) ownFont(ref int) bool {
	if ref == 0 {
		return false
	}
	for _, ef := range d.embeddedFonts {
		if ef != nil && ef.fontObjectID == ref {
			return true
		}
	}
	return false
}

// planReflows places the plans of page idx that re-wrap their paragraph in
// it and lays each paragraph out: its first plan removes and draws it, the
// others are folded into it. A plan whose text no paragraph holds is laid
// out alone (drawParagraph).
func (d *Document) planReflows(idx int, pls []*textEditPlan, fonts *replaceFonts) error {
	var rp []*textEditPlan
	var edits []*reflowEdit
	for _, pl := range pls {
		if pl.reflow != nil {
			rp = append(rp, pl)
			edits = append(edits, pl.reflow)
		}
	}
	if len(rp) == 0 {
		return nil
	}
	p, err := d.Page(idx + 1)
	if err != nil {
		return err
	}
	frags, err := p.pageFragments()
	if err != nil {
		return err
	}
	groups, byEdit := findReflowGroups(p, frags, func(i int) (float64, float64, bool) {
		pos := rp[i].snap.position
		x := pos.XIndent
		if len(edits[i].rects) > 0 {
			x = edits[i].rects[0].LLX
		}
		return x, pos.YIndent, rp[i].rtl()
	}, edits)
	for _, g := range groups {
		if g.lay, err = d.planReflow(p, g.para, g.cell, g.edits, g.rtl, fonts); err != nil {
			for i := range edits {
				if byEdit[i] == g {
					return rp[i].errorf("%w", err)
				}
			}
			return err
		}
	}
	dropWholeGroups(groups, byEdit) // unplaces their edits
	led := map[*reflowGroup]bool{}
	for i, pl := range rp {
		g := byEdit[i]
		if g == nil {
			continue
		}
		pl.group = g
		pl.hiddenOnly = nil
		if !led[g] {
			led[g] = true
			pl.leader = true
			pl.regions = g.regions()
		} else {
			pl.regions = nil
		}
	}
	return nil
}

// dropWholeGroups removes the groups whose paragraph the replacements cover
// whole, unplacing their edits (byEdit nil): those are laid out alone.
func dropWholeGroups(groups []*reflowGroup, byEdit []*reflowGroup) []*reflowGroup {
	kept := groups[:0]
	for _, g := range groups {
		if g.lay != nil && g.lay.own {
			kept = append(kept, g)
			continue
		}
		for i := range byEdit {
			if byEdit[i] == g {
				byEdit[i] = nil
			}
		}
	}
	return kept
}

// splitWideGaps cuts each part where its baselines stand more than twice
// the font size apart: lines set that far apart are not one paragraph
// (placeholders on a form, one to a line), whatever spacing the page's
// other paragraphs have.
func splitWideGaps(parts [][]TextLine) [][]TextLine {
	var out [][]TextLine
	for _, part := range parts {
		start := 0
		for i := 1; i < len(part); i++ {
			if part[i-1].Y-part[i].Y > 2*math.Max(lineSize(part[i-1]), lineSize(part[i])) {
				out = append(out, part[start:i])
				start = i
			}
		}
		out = append(out, part[start:])
	}
	return out
}
