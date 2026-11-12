// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"sort"
	"strings"
)

// TableAbsorber detects tables on a page and exposes their logical structure
// (epic pdf-go-w4ht). Mirrors Aspose.PDF for .NET's TableAbsorber:
//
//	absorber := pdf.NewTableAbsorber()
//	if err := absorber.Visit(page); err != nil { ... }
//	for _, table := range absorber.TableList() {
//	    for _, row := range table.RowList() {
//	        for _, cell := range row.CellList() { _ = cell.Text() }
//	    }
//	}
//
// The current detector is vector-native lattice recognition (ruled tables,
// including borders drawn as thin filled rectangles, dashed/segmented rules,
// and rowspan/colspan from missing inner rulings). Borderless (stream-mode)
// recognition is the next phase; note Aspose's own absorber handles ruled
// tables only.
type TableAbsorber struct {
	tables []*AbsorbedTable
}

// NewTableAbsorber creates a table absorber.
func NewTableAbsorber() *TableAbsorber {
	return &TableAbsorber{}
}

// TableList returns the tables recognized by the last Visit, top-to-bottom.
func (ta *TableAbsorber) TableList() []*AbsorbedTable {
	return ta.tables
}

// AbsorbedTable is one recognized table.
type AbsorbedTable struct {
	// Rect is the table's bounding rectangle in PDF user space.
	Rect Rectangle
	// PageNumber is the 1-based source page.
	PageNumber int
	rows       []*AbsorbedRow
	page       *Page     // the page Visit found it on
	colXs      []float64 // ascending column boundaries (len = cols+1)
	rowYs      []float64 // ascending row boundaries (len = rows+1)
}

// RowList returns the table's rows, top-to-bottom.
func (t *AbsorbedTable) RowList() []*AbsorbedRow { return t.rows }

// AbsorbedRow is one logical table row.
type AbsorbedRow struct {
	Rect  Rectangle
	cells []*AbsorbedCell
}

// CellList returns the row's cells, left-to-right (one per logical column;
// positions covered by a span carry Covered=true).
func (r *AbsorbedRow) CellList() []*AbsorbedCell { return r.cells }

// AbsorbedCell is one logical grid position.
type AbsorbedCell struct {
	// Rect is the cell's rectangle (the full span for spanning cells;
	// covered positions carry the covering cell's rectangle).
	Rect Rectangle
	// RowSpan/ColSpan are >= 1 on a cell's anchor position.
	RowSpan, ColSpan int
	// Covered marks a position occupied by another cell's span.
	Covered bool
	// Shading is the cell's background fill colour, when the page paints one.
	Shading   *Color
	fragments []TextFragment
}

// TextFragments returns the text fragments inside the cell.
func (c *AbsorbedCell) TextFragments() []TextFragment { return c.fragments }

// Text returns the cell's text: fragments grouped into visual lines, lines
// joined with newlines, gap-separated fragments with spaces.
func (c *AbsorbedCell) Text() string {
	if len(c.fragments) == 0 {
		return ""
	}
	frs := framedFragments(c.fragments)
	var b strings.Builder
	prevY := frs[0].y
	prevEnd := 0.0
	for i, f := range frs {
		switch {
		case i == 0:
		case f.y < prevY-0.5:
			b.WriteString("\n")
		default:
			if gapIsSpace(prevEnd, f.at(f.x)) {
				b.WriteString(" ")
			}
		}
		b.WriteString(f.Text)
		prevY = f.y
		prevEnd = f.x + f.Width
	}
	return strings.TrimSpace(b.String())
}

// framedFragment is a fragment with its position in its own text direction:
// x along the baseline, y across it, so rotated text orders into lines as
// horizontal text does.
type framedFragment struct {
	TextFragment
	x, y float64
}

// at returns the fragment with X replaced by x, for gapIsSpace.
func (f framedFragment) at(x float64) TextFragment {
	fr := f.TextFragment
	fr.X = x
	return fr
}

// framedFragments sorts fragments into reading order in their text frame: top
// line first, left to right. For unrotated text the frame is the page.
func framedFragments(in []TextFragment) []framedFragment {
	out := make([]framedFragment, len(in))
	for i, fr := range in {
		sin, cos := math.Sincos(fr.Rotation * math.Pi / 180)
		out[i] = framedFragment{fr, fr.X*cos + fr.Y*sin, -fr.X*sin + fr.Y*cos}
	}
	sort.Slice(out, func(i, j int) bool {
		if diff := out[i].y - out[j].y; diff > 0.5 || diff < -0.5 {
			return out[i].y > out[j].y
		}
		return out[i].x < out[j].x
	})
	return out
}

// absorbedCellRuns converts the cell's fragments to styled runs: visual
// lines separated by break runs, gap-synthesized spaces, full fragment look.
// Shared by the DOCX/EPUB/HTML/Markdown table serializers.
func absorbedCellRuns(cell *AbsorbedCell) []docRun {
	if len(cell.TextFragments()) == 0 {
		return nil
	}
	frs := framedFragments(cell.TextFragments())
	var runs []docRun
	prevY, prevEnd := frs[0].y, 0.0
	for i, f := range frs {
		fr := f.TextFragment
		if i > 0 {
			if f.y < prevY-0.5 {
				runs = append(runs, docRun{br: true})
			} else if gapIsSpace(prevEnd, f.at(f.x)) && len(runs) > 0 {
				runs[len(runs)-1].text += " "
			}
		}
		runs = append(runs, docRun{
			text:     fr.Text,
			bold:     fr.Bold,
			italic:   fr.Italic,
			code:     fontFamilyClass(fr.FontName) == "mono",
			fontName: fr.FontName,
			fontSize: fr.FontSize,
			color:    fr.Color,
			sub:      fr.IsSubscript,
			super:    fr.IsSuperscript,
		})
		prevY, prevEnd = f.y, f.x+fr.Width
	}
	return runs
}

// Visit runs table detection on the page, replacing the absorber's TableList.
func (ta *TableAbsorber) Visit(p *Page) error {
	ta.tables = nil
	hRules, vRules, fills := pageRules(p)

	lines, err := p.ExtractTextWithLayout()
	if err != nil {
		return err
	}

	if len(hRules) >= 2 && len(vRules) >= 2 {
		gh, gv := closeOpenEdges(hRules, vRules)
		if cells := latticeCells(buildLatticeGrid(gh, gv)); len(cells) > 0 {
			for _, group := range groupCells(cells) {
				for _, comp := range splitAtGaps(group) {
					if len(comp) < minTableCells {
						continue
					}
					if t := buildAbsorbedTable(comp, lines, fills, p.Number()); t != nil {
						ta.tables = append(ta.tables, t)
					}
				}
			}
		}
	}

	// Stream (borderless) pass on the regions not claimed by ruled tables.
	var latticeRects []Rectangle
	for _, t := range ta.tables {
		latticeRects = append(latticeRects, t.Rect)
	}
	pageMarkup := func() (*PageMarkup, error) {
		m, err := p.Paragraphs()
		return &m, err
	}
	ta.tables = append(ta.tables, detectStreamTables(lines, latticeRects, hRules, p.Number(), pageMarkup)...)
	// Then the same in the frame of text set at a quarter turn.
	for _, rot := range []float64{90, 270} {
		var claimed []Rectangle
		for _, t := range ta.tables {
			claimed = append(claimed, t.Rect)
		}
		ta.tables = append(ta.tables, detectRotatedStreamTables(p, lines, claimed, vRules, rot)...)
	}
	for _, t := range ta.tables {
		t.page = p
	}
	sort.SliceStable(ta.tables, func(i, j int) bool {
		if diff := ta.tables[i].Rect.URY - ta.tables[j].Rect.URY; diff > 0.5 || diff < -0.5 {
			return diff > 0
		}
		return ta.tables[i].Rect.LLX < ta.tables[j].Rect.LLX
	})
	return nil
}

// buildAbsorbedTable maps a component's physical cells onto the logical grid
// and assigns text fragments; nil when the component is not table-shaped.
func buildAbsorbedTable(comp []latticeCell, lines []TextLine, fills []shadedRect, pageNo int) *AbsorbedTable {
	var xs, ys []float64
	for _, c := range comp {
		xs = append(xs, c.LLX, c.URX)
		ys = append(ys, c.LLY, c.URY)
	}
	colXs := snapPositions(xs) // ascending
	rowYs := snapPositions(ys) // ascending; grid rows run top-to-bottom
	rows, cols := len(rowYs)-1, len(colXs)-1
	if rows < 2 || cols < 2 {
		return nil
	}

	gridCells := make([][]*AbsorbedCell, rows)
	for i := range gridCells {
		gridCells[i] = make([]*AbsorbedCell, cols)
	}
	// rowIdx 0 = top row: rowYs is ascending, so top boundary is the last.
	rowOfTop := func(y float64) int { return len(rowYs) - 1 - nearestIdx(rowYs, y, xTolPt*2) }
	colOfLeft := func(x float64) int { return nearestIdx(colXs, x, xTolPt*2) }

	for _, c := range comp {
		top := rowOfTop(c.URY)
		bottom := rowOfTop(c.LLY)
		left := colOfLeft(c.LLX)
		right := colOfLeft(c.URX)
		if top < 0 || left < 0 || bottom <= top-1 || right <= left-1 ||
			top >= rows+1 || left >= cols+1 {
			continue
		}
		rowSpan, colSpan := bottom-top, right-left
		if rowSpan < 1 || colSpan < 1 || top+rowSpan > rows || left+colSpan > cols {
			continue
		}
		cell := &AbsorbedCell{Rect: c.Rectangle, RowSpan: rowSpan, ColSpan: colSpan}
		for r := top; r < top+rowSpan; r++ {
			for cc := left; cc < left+colSpan; cc++ {
				if gridCells[r][cc] != nil {
					continue // overlapping evidence; first cell wins
				}
				if r == top && cc == left {
					gridCells[r][cc] = cell
				} else {
					gridCells[r][cc] = &AbsorbedCell{Rect: c.Rectangle, Covered: true}
				}
			}
		}
	}

	table := &AbsorbedTable{PageNumber: pageNo, colXs: colXs, rowYs: rowYs}
	table.Rect = Rectangle{LLX: colXs[0], LLY: rowYs[0], URX: colXs[cols], URY: rowYs[rows]}
	for r := 0; r < rows; r++ {
		rowTop := rowYs[len(rowYs)-1-r]
		rowBot := rowYs[len(rowYs)-2-r]
		row := &AbsorbedRow{Rect: Rectangle{LLX: colXs[0], LLY: rowBot, URX: colXs[cols], URY: rowTop}}
		for cc := 0; cc < cols; cc++ {
			cell := gridCells[r][cc]
			if cell == nil {
				cell = &AbsorbedCell{Rect: Rectangle{
					LLX: colXs[cc], LLY: rowBot, URX: colXs[cc+1], URY: rowTop,
				}, RowSpan: 1, ColSpan: 1}
				gridCells[r][cc] = cell
			}
			row.cells = append(row.cells, cell)
		}
		table.rows = append(table.rows, row)
	}

	// Cell shading: a fill box covering most of an anchor cell becomes its
	// background colour (white fills are the page, not shading).
	for r := 0; r < rows; r++ {
		for cc := 0; cc < cols; cc++ {
			cell := gridCells[r][cc]
			if cell == nil || cell.Covered {
				continue
			}
			for _, fb := range fills {
				if fb.col.R > 0.97 && fb.col.G > 0.97 && fb.col.B > 0.97 {
					continue
				}
				w := minf(cell.Rect.URX, fb.URX) - maxf(cell.Rect.LLX, fb.LLX)
				h := minf(cell.Rect.URY, fb.URY) - maxf(cell.Rect.LLY, fb.LLY)
				area := (cell.Rect.URX - cell.Rect.LLX) * (cell.Rect.URY - cell.Rect.LLY)
				if w > 0 && h > 0 && area > 0 && w*h >= 0.6*area {
					col := fb.col
					cell.Shading = &col
					break
				}
			}
		}
	}

	// Assign text fragments to anchor cells by baseline midpoint.
	for _, line := range lines {
		for _, fr := range line.Fragments {
			midX := fr.X + fr.Width/2
			midY := fr.Y + fr.FontSize*0.35
			if midX < table.Rect.LLX || midX > table.Rect.URX ||
				midY < table.Rect.LLY || midY > table.Rect.URY {
				continue
			}
			cell := cellAt(gridCells, rowYs, colXs, midX, midY)
			if cell != nil {
				cell.fragments = append(cell.fragments, fr)
			}
		}
	}
	orientTable(table)
	return table
}

// tableTextRotation is the direction most of a table's text runs in, by
// character count: 0, 90, 180 or 270 degrees.
func tableTextRotation(t *AbsorbedTable) int {
	count := map[int]int{}
	for _, row := range t.rows {
		for _, c := range row.cells {
			if c.Covered {
				continue
			}
			for _, fr := range c.fragments {
				q := (int(math.Round(fr.Rotation/90))%4 + 4) % 4
				count[q*90] += len([]rune(fr.Text))
			}
		}
	}
	best, bestN := 0, 0
	for _, rot := range []int{0, 90, 180, 270} {
		if count[rot] > bestN {
			best, bestN = rot, count[rot]
		}
	}
	return best
}

// orientTable turns the grid of a table whose text runs at 90 or 270 degrees
// into the orientation the text reads in (pdf-go-c19b): the grid was built on
// the page's axes, so its rows were the table's columns. At 90 degrees the
// glyphs' tops point left, so the table's first row is the leftmost page
// column and its columns run up the page; at 270 everything mirrors.
func orientTable(t *AbsorbedTable) {
	rot := tableTextRotation(t)
	if rot != 90 && rot != 270 {
		return
	}
	nR, nC := len(t.rows), len(t.rows[0].cells)
	grid := make([][]*AbsorbedCell, nC)
	for i := range grid {
		grid[i] = make([]*AbsorbedCell, nR)
	}
	for r, row := range t.rows {
		for c, cell := range row.cells {
			if cell.Covered {
				continue
			}
			top, left := c, nR-r-cell.RowSpan // 90
			if rot == 270 {
				top, left = nC-c-cell.ColSpan, r
			}
			moved := *cell
			moved.RowSpan, moved.ColSpan = cell.ColSpan, cell.RowSpan
			for i := top; i < top+moved.RowSpan; i++ {
				for j := left; j < left+moved.ColSpan; j++ {
					if i == top && j == left {
						grid[i][j] = &moved
					} else {
						grid[i][j] = &AbsorbedCell{Rect: cell.Rect, Covered: true}
					}
				}
			}
		}
	}
	colXs, rowYs := t.colXs, t.rowYs
	newCols := make([]float64, nR+1) // widths are the old rows' heights
	newRows := make([]float64, nC+1)
	for k := range newCols {
		if rot == 90 {
			newCols[k] = rowYs[k]
		} else {
			newCols[k] = -rowYs[nR-k]
		}
	}
	for k := range newRows {
		if rot == 90 {
			newRows[k] = -colXs[nC-k]
		} else {
			newRows[k] = colXs[k]
		}
	}
	t.rows = nil
	for i := range nC {
		c := i // the page column this row is
		if rot == 270 {
			c = nC - 1 - i
		}
		row := &AbsorbedRow{Rect: Rectangle{LLX: colXs[c], LLY: t.Rect.LLY, URX: colXs[c+1], URY: t.Rect.URY}}
		row.cells = grid[i]
		t.rows = append(t.rows, row)
	}
	t.colXs, t.rowYs = newCols, newRows
}

// cellAt locates the anchor cell containing the point.
func cellAt(grid [][]*AbsorbedCell, rowYs, colXs []float64, x, y float64) *AbsorbedCell {
	rows, cols := len(grid), len(grid[0])
	// Row: rowYs ascending; find interval containing y, then flip to top-index.
	ri := sort.SearchFloat64s(rowYs, y) - 1
	if ri < 0 || ri >= rows {
		return nil
	}
	r := rows - 1 - ri
	ci := sort.SearchFloat64s(colXs, x) - 1
	if ci < 0 || ci >= cols {
		return nil
	}
	cell := grid[r][ci]
	if cell == nil {
		return nil
	}
	if cell.Covered {
		// Walk to the anchor: scan up/left for the cell owning this span.
		for rr := r; rr >= 0; rr-- {
			for cc := ci; cc >= 0; cc-- {
				a := grid[rr][cc]
				if a != nil && !a.Covered &&
					a.Rect.LLX <= x && x <= a.Rect.URX && a.Rect.LLY <= y && y <= a.Rect.URY {
					return a
				}
			}
		}
		return nil
	}
	return cell
}
