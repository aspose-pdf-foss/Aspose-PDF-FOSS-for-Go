// SPDX-License-Identifier: MIT

package asposepdf

import (
	"sort"
	"strings"
)

// Stream (borderless) table detection — epic pdf-go-w4ht.3, the beyond-Aspose
// phase of the TableAbsorber (Aspose.PDF for .NET recognizes ruled tables
// only). Ruled tables are handled by the lattice detector; this pass finds
// tables whose structure exists only as aligned whitespace: rows are text
// lines split into cells by wide intra-line gaps, columns are seeded from the
// modal cell count (the Camelot approach), and a battery of conservative
// guards rejects anything prose-shaped — a false negative merely leaves text
// flowing as paragraphs, while a false positive would wrongly reshape prose
// into a table, so every threshold errs toward rejection.
//
// The detector runs on the raw layout lines (ExtractTextWithLayout), NOT on
// Paragraphs() output: the paragraph extractor's column splitter dissects
// exactly the whitespace channels this pass needs to observe.
//
// v1 scope (per the ticket): no merged cells in unruled tables, no
// cross-page stitching, no rotated tables.

const (
	streamMinRows     = 3    // minimum table rows
	streamMinCols     = 2    // minimum table columns
	streamMaxCols     = 15   // sanity cap
	streamGapEm       = 0.56 // intra-line cell gap: >= 2 space widths (2 x 0.28 em)
	streamGapMinPt    = 4.0  // absolute floor for the cell gap
	streamRowGapK     = 2.6  // vertical run break: baseline gap > K x font size
	streamChannelFrac = 0.85 // whitespace channel must be clear in this share of rows
	streamModalMin    = 3    // modal cell count must appear in >= this many rows
	streamProseCover  = 0.80 // candidate covered this much by a prose paragraph -> reject
	streamFontSizes   = 3    // max distinct font sizes covering 90% of the text
	streamDotRows     = 0.30 // rows with dot leaders above this share -> TOC, reject
)

// streamCellSpan is one gap-delimited fragment group within a line.
type streamCellSpan struct {
	lo, hi float64
	frs    []TextFragment
}

// streamRow is one candidate table row.
type streamRow struct {
	top, bot  float64 // vertical extent (URY/LLY)
	baseline  float64
	size      float64 // dominant font size
	cells     []streamCellSpan
	dotLeader bool
}

// detectStreamTables finds borderless tables among the page's layout lines.
// skipRects lists regions already claimed by lattice tables; hRules feeds the
// structural-agreement guard.
func detectStreamTables(p *Page, lines []TextLine, skipRects []Rectangle, hRules []rule, pageNo int) []*AbsorbedTable {
	rows := buildStreamRows(lines, skipRects)
	if len(rows) < streamMinRows {
		return nil
	}

	var tables []*AbsorbedTable
	var pm *PageMarkup // lazy: Paragraphs() only when a candidate survives to guard 5
	for _, run := range streamRuns(rows) {
		t := buildStreamTable(run, hRules, &pm, p, pageNo)
		if t != nil {
			tables = append(tables, t)
		}
	}
	return tables
}

// buildStreamRows converts layout lines to candidate rows, skipping lines
// inside already-detected (lattice) tables and rotated lines.
func buildStreamRows(lines []TextLine, skipRects []Rectangle) []streamRow {
	var rows []streamRow
	for _, line := range lines {
		if len(line.Fragments) == 0 {
			continue
		}
		rotated := false
		for _, fr := range line.Fragments {
			if fr.Rotation != 0 {
				rotated = true
				break
			}
		}
		if rotated {
			continue
		}
		if lineInsideAny(line, skipRects) {
			continue
		}
		row := splitStreamRow(line)
		if len(row.cells) > 0 {
			rows = append(rows, row)
		}
	}
	return rows
}

// lineInsideAny reports whether most of the line's fragments sit inside one
// of the rectangles (midpoint rule, as everywhere in the detector).
func lineInsideAny(line TextLine, rects []Rectangle) bool {
	if len(rects) == 0 {
		return false
	}
	in := 0
	for _, fr := range line.Fragments {
		mx, my := fr.X+fr.Width/2, fr.Y+fr.Height/2
		for _, r := range rects {
			if mx >= r.LLX-2 && mx <= r.URX+2 && my >= r.LLY-2 && my <= r.URY+2 {
				in++
				break
			}
		}
	}
	return in*2 > len(line.Fragments)
}

// splitStreamRow splits a line's fragments into cells at gaps of two or more
// space widths (guard 1: the intra-line gap requirement).
func splitStreamRow(line TextLine) streamRow {
	frs := append([]TextFragment(nil), line.Fragments...)
	sort.Slice(frs, func(i, j int) bool { return frs[i].X < frs[j].X })

	row := streamRow{baseline: line.Y}
	sizeW := map[float64]int{}
	var cur streamCellSpan
	flush := func() {
		if len(cur.frs) > 0 {
			row.cells = append(row.cells, cur)
		}
		cur = streamCellSpan{}
	}
	prevEnd := 0.0
	prevSize := 0.0
	for i, fr := range frs {
		if fr.Height > 0 {
			if row.top == 0 && row.bot == 0 {
				row.top, row.bot = fr.Y+fr.Height, fr.Y
			} else {
				row.top = maxf(row.top, fr.Y+fr.Height)
				row.bot = minf(row.bot, fr.Y)
			}
		}
		sizeW[roundHalf(fr.FontSize)] += len([]rune(fr.Text))
		if strings.Contains(fr.Text, ".....") {
			row.dotLeader = true
		}
		if i > 0 {
			em := (fr.FontSize + prevSize) / 2
			if fr.X-prevEnd >= maxf(streamGapMinPt, streamGapEm*em) {
				flush()
			}
		}
		if len(cur.frs) == 0 {
			cur.lo = fr.X
		}
		cur.hi = maxf(cur.hi, fr.X+fr.Width)
		cur.frs = append(cur.frs, fr)
		prevEnd = maxf(prevEnd, fr.X+fr.Width)
		prevSize = fr.FontSize
	}
	flush()
	bestW := 0
	for s, w := range sizeW {
		if w > bestW || (w == bestW && s > row.size) {
			row.size, bestW = s, w
		}
	}
	return row
}

func roundHalf(v float64) float64 { return float64(int(v*2+0.5)) / 2 }

// streamRuns groups rows into vertical runs of >= streamMinRows consecutive
// multi-cell lines with compact spacing.
func streamRuns(rows []streamRow) [][]streamRow {
	var runs [][]streamRow
	var cur []streamRow
	flush := func() {
		if len(cur) >= streamMinRows {
			runs = append(runs, cur)
		}
		cur = nil
	}
	for _, r := range rows {
		if len(r.cells) < streamMinCols {
			flush()
			continue
		}
		if len(cur) > 0 {
			prev := cur[len(cur)-1]
			gap := prev.baseline - r.baseline
			if gap <= 0 || gap > streamRowGapK*maxf(prev.size, r.size) {
				flush()
			}
		}
		cur = append(cur, r)
	}
	flush()
	return runs
}

// buildStreamTable derives the column model for one run and applies the
// guards; nil when any guard rejects.
func buildStreamTable(run []streamRow, hRules []rule, pm **PageMarkup, p *Page, pageNo int) *AbsorbedTable {
	// Guard 3: modal cell count, >= 2, appearing in >= streamModalMin rows.
	counts := map[int]int{}
	for _, r := range run {
		counts[len(r.cells)]++
	}
	modal, modalFreq := 0, 0
	for c, f := range counts {
		if f > modalFreq || (f == modalFreq && c > modal) {
			modal, modalFreq = c, f
		}
	}
	if modal < streamMinCols || modal > streamMaxCols || modalFreq < streamModalMin {
		return nil
	}

	// Trim leading/trailing rows that don't match the modal count — a
	// centered heading or a trailing note with an incidental gap must not
	// stretch the table; interior off-modal rows stay (a sparse row is
	// legitimate) but are covered by the channel guard below.
	lo, hi := 0, len(run)
	for lo < hi && len(run[lo].cells) != modal {
		lo++
	}
	for hi > lo && len(run[hi-1].cells) != modal {
		hi--
	}
	run = run[lo:hi]
	if len(run) < streamMinRows {
		return nil
	}

	// Column ranges from the modal rows (union per position); overlapping
	// ranges mean the "columns" don't actually align — reject.
	ranges := make([]struct{ lo, hi float64 }, modal)
	seeded := false
	for _, r := range run {
		if len(r.cells) != modal {
			continue
		}
		for k, c := range r.cells {
			if !seeded {
				ranges[k].lo, ranges[k].hi = c.lo, c.hi
			} else {
				ranges[k].lo = minf(ranges[k].lo, c.lo)
				ranges[k].hi = maxf(ranges[k].hi, c.hi)
			}
		}
		seeded = true
	}
	for k := 1; k < modal; k++ {
		if ranges[k].lo <= ranges[k-1].hi+1 {
			return nil // column ranges overlap: misaligned gaps, not columns
		}
	}

	// Column boundaries at gap midpoints.
	colXs := make([]float64, modal+1)
	colXs[0] = ranges[0].lo - 2
	for k := 1; k < modal; k++ {
		colXs[k] = (ranges[k-1].hi + ranges[k].lo) / 2
	}
	colXs[modal] = ranges[modal-1].hi + 2

	// Guard 2: each internal boundary must be a vertically continuous
	// whitespace channel — clear in >= streamChannelFrac of the rows.
	// Justified-prose word gaps do not stack at one X.
	for k := 1; k < modal; k++ {
		clear := 0
		for _, r := range run {
			blocked := false
			for _, c := range r.cells {
				for _, fr := range c.frs {
					if fr.X-0.5 <= colXs[k] && fr.X+fr.Width+0.5 >= colXs[k] {
						blocked = true
						break
					}
				}
				if blocked {
					break
				}
			}
			if !blocked {
				clear++
			}
		}
		if float64(clear) < streamChannelFrac*float64(len(run)) {
			return nil
		}
	}

	// Guard 7: dot leaders mark a table of contents, not a table.
	dots := 0
	for _, r := range run {
		if r.dotLeader {
			dots++
		}
	}
	if float64(dots) >= streamDotRows*float64(len(run)) {
		return nil
	}

	// Guard 6: a table sticks to few type sizes; prose with inline emphasis
	// and mixed headings does not.
	sizeW := map[float64]int{}
	total := 0
	for _, r := range run {
		for _, c := range r.cells {
			for _, fr := range c.frs {
				n := len([]rune(fr.Text))
				sizeW[roundHalf(fr.FontSize)] += n
				total += n
			}
		}
	}
	type sw struct {
		s float64
		w int
	}
	var sws []sw
	for s, w := range sizeW {
		sws = append(sws, sw{s, w})
	}
	sort.Slice(sws, func(i, j int) bool {
		if sws[i].w != sws[j].w {
			return sws[i].w > sws[j].w
		}
		return sws[i].s < sws[j].s
	})
	covered, distinct := 0, 0
	for _, e := range sws {
		if float64(covered) >= 0.9*float64(total) {
			break
		}
		covered += e.w
		distinct++
	}
	if distinct > streamFontSizes {
		return nil
	}

	rect := Rectangle{LLX: colXs[0], LLY: run[len(run)-1].bot, URX: colXs[modal], URY: run[0].top}

	// Guard 4: where partial horizontal rules exist, the ruled band count
	// and the stream row count must roughly agree (tabula's isTabular).
	width := rect.URX - rect.LLX
	rulesIn := 0
	for _, hr := range hRules {
		if hr.pos < rect.LLY-2 || hr.pos > rect.URY+2 {
			continue
		}
		if minf(hr.hi, rect.URX)-maxf(hr.lo, rect.LLX) >= 0.5*width {
			rulesIn++
		}
	}
	if rulesIn >= 3 {
		ratio := float64(len(run)) / float64(rulesIn-1)
		if ratio <= 0.65 || ratio >= 1.5 {
			return nil
		}
	}

	// Guard 8: a two-column candidate whose first column is a strictly
	// ascending run of bare integers is a line-numbered listing (code
	// dumps, transcripts), not a table. Real tables with a numeric # column
	// carry more columns.
	if modal == 2 {
		prev, seq := 0, true
		for _, r := range run {
			if len(r.cells) != 2 {
				continue
			}
			n, ok := bareInt(r.cells[0])
			if !ok || (prev != 0 && n != prev+1) {
				seq = false
				break
			}
			prev = n
		}
		if seq && prev != 0 {
			return nil
		}
	}

	// Guard 9: a candidate whose first column is list markers (bullets,
	// "1.", "2)") is a bulleted/numbered list with a hanging indent — or,
	// wider than two columns, side-by-side list columns — not a table.
	{
		markers, withCells := 0, 0
		for _, r := range run {
			if len(r.cells) == 0 {
				continue
			}
			withCells++
			if isListMarkerCell(r.cells[0]) {
				markers++
			}
		}
		if withCells > 0 && float64(markers) >= 0.7*float64(withCells) {
			return nil
		}
	}

	// Guard 12: cells that read as prose disqualify the candidate — a table
	// column holds short fields, a text column holds sentences. Reject when
	// every column's typical cell is multi-word prose (interleaved
	// multi-column body text whose gutter the section splitter missed), or
	// when a two-column candidate's value column is (specimen sheets,
	// definition lists — Word would re-wrap the long text anyway).
	{
		prose := make([]bool, modal)
		proseCols := 0
		for k := 0; k < modal; k++ {
			var words []int
			for _, r := range run {
				if len(r.cells) != modal {
					continue
				}
				if s := strings.Fields(streamCellText(r.cells[k])); len(s) > 0 {
					words = append(words, len(s))
				}
			}
			if len(words) == 0 {
				continue
			}
			sort.Ints(words)
			if words[len(words)/2] >= 4 {
				prose[k] = true
				proseCols++
			}
		}
		if proseCols == modal {
			return nil // every column is sentences: interleaved page columns
		}
		if modal == 2 && prose[1] {
			// The VALUE column is sentences: a specimen sheet or definition
			// list ("Helvetica | The quick brown fox…"), not a table. A
			// prose-y FIRST column with short values stays — that is the
			// ordinary description-amount shape of invoices and payslips.
			return nil
		}
	}

	// Guard 10: a two-column candidate whose text column is long entries
	// mostly ENDING in a bare number is a TOC / list-of-figures without dot
	// leaders ("Plate 2   Uncalibrated color (...)  244"). Genuine
	// property-value tables keep their values short.
	if modal == 2 {
		long, endNum, twoCol := 0, 0, 0
		for _, r := range run {
			if len(r.cells) != 2 {
				continue
			}
			twoCol++
			s := streamCellText(r.cells[1])
			if len(s) >= 25 {
				long++
			}
			if i := strings.LastIndexByte(s, ' '); i >= 0 {
				if _, ok := bareIntText(strings.TrimRight(s[i+1:], ").")); ok {
					endNum++
				}
			}
		}
		if twoCol > 0 && float64(long) >= 0.6*float64(twoCol) && float64(endNum) >= 0.6*float64(twoCol) {
			return nil
		}
	}

	// Guard 11: a two-column candidate set almost entirely in a monospace
	// face is a code listing with an aligned comment/value column, not a
	// table (the flow exporters render it as a code block). Monospace
	// tables with three or more columns — bank statements — survive.
	if modal == 2 {
		monoChars := 0
		for _, r := range run {
			for _, c := range r.cells {
				for _, fr := range c.frs {
					if fontFamilyClass(fr.FontName) == "mono" {
						monoChars += len([]rune(fr.Text))
					}
				}
			}
		}
		if total > 0 && float64(monoChars) >= 0.8*float64(total) {
			return nil
		}
	}

	// Guard 5: reject a candidate covered by flowing prose. The coverage is
	// the UNION over prose-shaped paragraphs, not any single one — the
	// classic false positive is two-column page text whose gutter forms a
	// perfect whitespace channel (a book index, justified columns): each
	// column's paragraphs cover only half the candidate, together nearly
	// all of it.
	if *pm == nil {
		markup, err := p.Paragraphs()
		if err != nil {
			return nil
		}
		*pm = &markup
	}
	area := (rect.URX - rect.LLX) * (rect.URY - rect.LLY)
	if area <= 0 {
		return nil
	}
	proseArea := 0.0
	for si := range (*pm).Sections {
		for pi := range (*pm).Sections[si].Paragraphs {
			para := &(*pm).Sections[si].Paragraphs[pi]
			if !proseShaped(para) {
				continue
			}
			pr := para.Rectangle
			w := minf(rect.URX, pr.URX) - maxf(rect.LLX, pr.LLX)
			h := minf(rect.URY, pr.URY) - maxf(rect.LLY, pr.LLY)
			if w > 0 && h > 0 {
				proseArea += w * h // paragraphs don't overlap; sum ~ union
			}
		}
	}
	if proseArea >= streamProseCover*area {
		return nil
	}

	return assembleStreamTable(run, colXs, rect, pageNo)
}

// streamCellText joins a cell's fragments in X order, inserting a space
// only across gaps that read as one (gapIsSpace) — fragment boundaries are
// not word boundaries.
func streamCellText(c streamCellSpan) string {
	frs := append([]TextFragment(nil), c.frs...)
	sort.Slice(frs, func(i, j int) bool { return frs[i].X < frs[j].X })
	var b strings.Builder
	prevEnd := 0.0
	for i, fr := range frs {
		if i > 0 && gapIsSpace(prevEnd, fr) {
			b.WriteString(" ")
		}
		b.WriteString(fr.Text)
		prevEnd = maxf(prevEnd, fr.X+fr.Width)
	}
	return strings.TrimSpace(b.String())
}

// isListMarkerCell reports whether the cell is nothing but a list marker: a
// single bullet glyph or "N." / "N)" numbering.
func isListMarkerCell(c streamCellSpan) bool {
	var b strings.Builder
	for _, fr := range c.frs {
		b.WriteString(fr.Text)
	}
	s := strings.TrimSpace(b.String())
	if s == "" || len(s) > 5 {
		return false
	}
	if n := []rune(s); len(n) == 1 && strings.ContainsRune("•◦▪‣∙·–—*-", n[0]) {
		return true
	}
	body, tail := s[:len(s)-1], s[len(s)-1]
	if tail != '.' && tail != ')' {
		return false
	}
	if _, ok := bareIntText(body); ok {
		return true
	}
	return false
}

// bareIntText parses s as a bare integer of table-plausible length.
func bareIntText(s string) (int, bool) {
	if s == "" || len(s) > 6 {
		return 0, false
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, true
}

// bareInt returns the cell's text as an integer when it is nothing else.
func bareInt(c streamCellSpan) (int, bool) {
	var b strings.Builder
	for _, fr := range c.frs {
		b.WriteString(fr.Text)
	}
	return bareIntText(strings.TrimSpace(b.String()))
}

// proseShaped reports whether a paragraph reads as flowing text: most of its
// lines have no wide internal gaps.
func proseShaped(para *MarkupParagraph) bool {
	if len(para.Lines) == 0 {
		return false
	}
	single := 0
	for _, l := range para.Lines {
		if len(splitStreamRow(l).cells) <= 1 {
			single++
		}
	}
	return float64(single) >= 0.7*float64(len(para.Lines))
}

// assembleStreamTable materializes the logical grid: every cell 1x1,
// fragments assigned by midpoint.
func assembleStreamTable(run []streamRow, colXs []float64, rect Rectangle, pageNo int) *AbsorbedTable {
	nRows, nCols := len(run), len(colXs)-1

	// Row boundaries, ascending (grid rows run top-to-bottom).
	rowYs := make([]float64, nRows+1)
	rowYs[nRows] = rect.URY
	rowYs[0] = rect.LLY
	for i := 1; i < nRows; i++ {
		// Boundary between visual rows i-1 (above) and i.
		rowYs[nRows-i] = (run[i-1].bot + run[i].top) / 2
	}

	t := &AbsorbedTable{Rect: rect, PageNumber: pageNo, colXs: colXs, rowYs: rowYs}
	for i := 0; i < nRows; i++ {
		top, bot := rowYs[nRows-i], rowYs[nRows-i-1]
		row := &AbsorbedRow{Rect: Rectangle{LLX: rect.LLX, LLY: bot, URX: rect.URX, URY: top}}
		for k := 0; k < nCols; k++ {
			row.cells = append(row.cells, &AbsorbedCell{
				Rect:    Rectangle{LLX: colXs[k], LLY: bot, URX: colXs[k+1], URY: top},
				RowSpan: 1, ColSpan: 1,
			})
		}
		for _, c := range run[i].cells {
			for _, fr := range c.frs {
				mx := fr.X + fr.Width/2
				k := sort.SearchFloat64s(colXs, mx) - 1
				if k < 0 {
					k = 0
				}
				if k >= nCols {
					k = nCols - 1
				}
				row.cells[k].fragments = append(row.cells[k].fragments, fr)
			}
		}
		t.rows = append(t.rows, row)
	}
	return t
}
