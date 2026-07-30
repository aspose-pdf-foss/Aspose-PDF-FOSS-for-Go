// SPDX-License-Identifier: MIT

package asposepdf

import (
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
	Covered   bool
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
	frs := append([]TextFragment(nil), c.fragments...)
	sort.Slice(frs, func(i, j int) bool {
		if diff := frs[i].Y - frs[j].Y; diff > 0.5 || diff < -0.5 {
			return frs[i].Y > frs[j].Y
		}
		return frs[i].X < frs[j].X
	})
	var b strings.Builder
	prevY := frs[0].Y
	prevEnd := 0.0
	for i, fr := range frs {
		switch {
		case i == 0:
		case fr.Y < prevY-0.5:
			b.WriteString("\n")
			prevEnd = 0
		default:
			if gapIsSpace(prevEnd, fr) {
				b.WriteString(" ")
			}
		}
		b.WriteString(fr.Text)
		prevY = fr.Y
		prevEnd = fr.X + fr.Width
	}
	return strings.TrimSpace(b.String())
}

// Visit runs table detection on the page, replacing the absorber's TableList.
func (ta *TableAbsorber) Visit(p *Page) error {
	ta.tables = nil
	hRules, vRules, _ := pageRules(p)
	if len(hRules) < 2 || len(vRules) < 2 {
		return nil
	}
	grid := buildLatticeGrid(hRules, vRules)
	cells := latticeCells(grid)
	if len(cells) == 0 {
		return nil
	}

	lines, err := p.ExtractTextWithLayout()
	if err != nil {
		return err
	}

	for _, comp := range groupCells(cells) {
		if len(comp) < minTableCells {
			continue
		}
		if t := buildAbsorbedTable(comp, lines, p.Number()); t != nil {
			ta.tables = append(ta.tables, t)
		}
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
func buildAbsorbedTable(comp []latticeCell, lines []TextLine, pageNo int) *AbsorbedTable {
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

	type anchor struct {
		cell *AbsorbedCell
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

	table := &AbsorbedTable{PageNumber: pageNo}
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
	return table
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
