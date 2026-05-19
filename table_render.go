package asposepdf

import (
	"fmt"
	"strings"
)

// AddTable renders the table inside the given rectangle.
//
// Returns the number of pages automatically appended to the document (0 when
// the table fits in rect). When the table doesn't fit and overflow is needed,
// new pages are appended with dimensions matching the receiver page; the
// continuation rectangle is computed from t.OverflowMargins().
//
// (Overflow logic arrives in Phase 2 Task 9 — this task only changes the
// signature; the function still always returns 0 for pagesAdded.)
//
// Errors before any drawing on validation failures: nil table, bad rect,
// non-positive column widths, mismatched cell counts (span-aware), merge
// overlaps, rowspan crossing the header/body boundary, or a spanning group
// too tall to fit any page.
func (p *Page) AddTable(t *Table, rect Rectangle) (int, error) {
	if t == nil {
		return 0, fmt.Errorf("add table: nil table")
	}
	if err := rect.validate(); err != nil {
		return 0, fmt.Errorf("add table: %w", err)
	}
	if len(t.columnWidths) == 0 {
		// Empty table — nothing to draw.
		return 0, nil
	}
	for i, w := range t.columnWidths {
		if w <= 0 {
			return 0, fmt.Errorf("add table: column %d has non-positive width %g", i, w)
		}
	}
	if len(t.rows) == 0 {
		return 0, nil
	}
	covered, err := validateAndCover(t)
	if err != nil {
		return 0, err
	}
	_ = covered // used by subsequent rendering tasks
	heights, err := computeRowHeights(t)
	if err != nil {
		return 0, fmt.Errorf("add table: %w", err)
	}

	// Render cells. For each cell, compute its rect and interior, then call
	// AddText (which handles font resolution, encoding, wrap, alignment, clipping).
	y := rect.URY
	drawnHeight := 0.0
	for i, row := range t.rows {
		if y-heights[i] < rect.LLY {
			// Row doesn't fit — stop drawing further rows (clipping; Task 9
			// adds a regression test for this).
			break
		}
		x := rect.LLX
		for col, cell := range row.cells {
			colWidth := t.columnWidths[col]
			cellLLX := x
			cellURX := x + colWidth
			cellURY := y
			cellLLY := y - heights[i]

			margin := effectiveCellMargin(t, cell)
			style := effectiveCellStyle(t, cell)

			interior := Rectangle{
				LLX: cellLLX + margin.Left,
				LLY: cellLLY + margin.Bottom,
				URX: cellURX - margin.Right,
				URY: cellURY - margin.Top,
			}

			// 1. Background first (so text and borders go on top).
			if cell.background != nil {
				if err := p.appendToContentStream([]byte(
					drawCellBackground(cellLLX, cellLLY, cellURX, cellURY, cell.background),
				)); err != nil {
					return 0, fmt.Errorf("add table: row %d col %d background: %w", i, col, err)
				}
			}

			// 2. Text (existing AddText call).
			if interior.URX > interior.LLX && interior.URY > interior.LLY && cell.text != "" {
				if err := p.AddText(cell.text, style, interior); err != nil {
					return 0, fmt.Errorf("add table: row %d col %d text: %w", i, col, err)
				}
			}

			// 3. Cell border on top of text edges.
			border := effectiveCellBorder(t, cell)
			if ops := drawBorderSides(cellLLX, cellLLY, cellURX, cellURY, border); ops != "" {
				if err := p.appendToContentStream([]byte(ops)); err != nil {
					return 0, fmt.Errorf("add table: row %d col %d border: %w", i, col, err)
				}
			}
			x += colWidth
		}
		y -= heights[i]
		drawnHeight += heights[i]
	}

	// Outer table border. Drawn last so it appears on top of cell-edge strokes.
	// Height equals the sum of rendered rows (not the rect), so the border
	// tightly wraps the visible content even when later rows were clipped.
	if drawnHeight > 0 {
		totalW := 0.0
		for _, w := range t.columnWidths {
			totalW += w
		}
		if ops := drawBorderSides(
			rect.LLX, rect.URY-drawnHeight,
			rect.LLX+totalW, rect.URY,
			t.border,
		); ops != "" {
			if err := p.appendToContentStream([]byte(ops)); err != nil {
				return 0, fmt.Errorf("add table: outer border: %w", err)
			}
		}
	}

	return 0, nil
}

// validateAndCover walks the rows, validates span boundaries + non-overlap,
// and returns a [rows][cols] grid where covered[i][j] == true means position
// (i, j) is filled by a cell that started at an earlier row (rowspan) — i.e.
// the caller does not add a *Cell for this position in row i.
//
// Per the spec: every row's cells, placed left-to-right and skipping covered
// positions, must exactly cover the remaining column slots in that row.
func validateAndCover(t *Table) ([][]bool, error) {
	numRows := len(t.rows)
	numCols := len(t.columnWidths)
	covered := make([][]bool, numRows)
	for i := range covered {
		covered[i] = make([]bool, numCols)
	}

	for i, row := range t.rows {
		col := 0
		for cellIdx, cell := range row.cells {
			// Skip positions covered by inherited rowspans.
			for col < numCols && covered[i][col] {
				col++
			}
			if col >= numCols {
				return nil, fmt.Errorf(
					"add table: row %d has extra cell %d but all columns already covered",
					i, cellIdx)
			}
			cs := cell.ColSpan()
			rs := cell.RowSpan()
			if col+cs > numCols {
				return nil, fmt.Errorf(
					"add table: colspan at row %d cell %d (col %d, span %d) exceeds column count %d",
					i, cellIdx, col, cs, numCols)
			}
			if i+rs > numRows {
				return nil, fmt.Errorf(
					"add table: rowspan at row %d cell %d (span %d) exceeds row count %d",
					i, cellIdx, rs, numRows)
			}
			// Mark future-row coverage.
			for r := 1; r < rs; r++ {
				for c := 0; c < cs; c++ {
					if covered[i+r][col+c] {
						return nil, fmt.Errorf(
							"add table: merge overlap at row %d col %d", i+r, col+c)
					}
					covered[i+r][col+c] = true
				}
			}
			col += cs
		}
		// After placing all of row i's cells, every column must be covered:
		//   columns 0..col-1 are covered by this row's cells (placed left-to-right)
		//   columns col..numCols-1 must be covered by inherited rowspans
		for c := col; c < numCols; c++ {
			if !covered[i][c] {
				return nil, fmt.Errorf(
					"add table: row %d undercoverage at col %d (cells stop at %d, no inherited rowspan)",
					i, c, col)
			}
		}
	}

	return covered, nil
}

// computeRowHeights returns the drawn height of each row in t.
//
// For rows with an explicit SetHeight > 0, the explicit value is returned.
// For rows with auto-fit (height == 0), the height is the max of cell content
// heights in the row, where each cell's content height is:
//
//	lines * (fontSize * lineSpacing) + margin.Top + margin.Bottom
//
// Lines come from measureText against the column's interior width
// (column width - margin.Left - margin.Right).
func computeRowHeights(t *Table) ([]float64, error) {
	heights := make([]float64, len(t.rows))
	for i, row := range t.rows {
		if row.height > 0 {
			heights[i] = row.height
			continue
		}
		maxH := 0.0
		for col, cell := range row.cells {
			colWidth := t.columnWidths[col]
			margin := effectiveCellMargin(t, cell)
			style := effectiveCellStyle(t, cell)
			interiorWidth := colWidth - margin.Left - margin.Right
			if interiorWidth < 0 {
				interiorWidth = 0
			}
			lines, lineHeight, err := measureText(cell.text, style, interiorWidth)
			if err != nil {
				return nil, fmt.Errorf("row %d col %d: %w", i, col, err)
			}
			cellH := float64(lines)*lineHeight + margin.Top + margin.Bottom
			if cellH > maxH {
				maxH = cellH
			}
		}
		heights[i] = maxH
	}
	return heights, nil
}

// effectiveCellMargin returns the per-cell margin, falling back to the table
// default if the cell has no override.
func effectiveCellMargin(t *Table, c *Cell) MarginInfo {
	if c.margin != nil {
		return *c.margin
	}
	return t.defaultCellMargin
}

// drawCellBackground returns a content-stream fragment that fills the cell
// rect with the given color. Returns empty string if col is nil.
func drawCellBackground(cellLLX, cellLLY, cellURX, cellURY float64, col *Color) string {
	if col == nil {
		return ""
	}
	w := cellURX - cellLLX
	h := cellURY - cellLLY
	return fmt.Sprintf("q\n%s %s %s rg\n%s %s %s %s re f\nQ\n",
		formatFloat(col.R), formatFloat(col.G), formatFloat(col.B),
		formatFloat(cellLLX), formatFloat(cellLLY),
		formatFloat(w), formatFloat(h))
}

// drawBorderSides returns stroking operators for each side of a rectangle
// selected by the bitmask. Returns empty string if no sides or zero width.
func drawBorderSides(llx, lly, urx, ury float64, b BorderInfo) string {
	if b.Sides == BorderSideNone || b.Width <= 0 {
		return ""
	}
	col := Color{R: 0, G: 0, B: 0, A: 1}
	if b.Color != nil {
		col = *b.Color
	}
	var buf strings.Builder
	buf.WriteString("q\n")
	buf.WriteString(fmt.Sprintf("%s w\n", formatFloat(b.Width)))
	buf.WriteString(fmt.Sprintf("%s %s %s RG\n",
		formatFloat(col.R), formatFloat(col.G), formatFloat(col.B)))
	if b.Sides&BorderSideTop != 0 {
		buf.WriteString(fmt.Sprintf("%s %s m %s %s l S\n",
			formatFloat(llx), formatFloat(ury), formatFloat(urx), formatFloat(ury)))
	}
	if b.Sides&BorderSideRight != 0 {
		buf.WriteString(fmt.Sprintf("%s %s m %s %s l S\n",
			formatFloat(urx), formatFloat(ury), formatFloat(urx), formatFloat(lly)))
	}
	if b.Sides&BorderSideBottom != 0 {
		buf.WriteString(fmt.Sprintf("%s %s m %s %s l S\n",
			formatFloat(urx), formatFloat(lly), formatFloat(llx), formatFloat(lly)))
	}
	if b.Sides&BorderSideLeft != 0 {
		buf.WriteString(fmt.Sprintf("%s %s m %s %s l S\n",
			formatFloat(llx), formatFloat(lly), formatFloat(llx), formatFloat(ury)))
	}
	buf.WriteString("Q\n")
	return buf.String()
}

// effectiveCellBorder returns the per-cell border, falling back to the table default.
func effectiveCellBorder(t *Table, c *Cell) BorderInfo {
	if c.border != nil {
		return *c.border
	}
	return t.defaultCellBorder
}

// effectiveCellStyle returns the resolved TextStyle for a cell, layering:
// table.defaultCellStyle ← cell.style overlay ← cell H/V align overrides.
func effectiveCellStyle(t *Table, c *Cell) TextStyle {
	style := t.defaultCellStyle
	if c.style != nil {
		style = overlayTextStyle(style, *c.style)
	}
	if c.hAlignSet {
		style.HAlign = c.hAlign
	}
	if c.vAlignSet {
		style.VAlign = c.vAlign
	}
	return style
}

// overlayTextStyle returns base with every non-zero field of overlay applied
// on top. Zero-value fields in overlay leave base unchanged.
//
// Field list mirrors the TextStyle declared in color.go (Font, Size, Color,
// Background, HAlign, VAlign, LineSpacing, Underline, Strikethrough, Rotation, Behind).
func overlayTextStyle(base, overlay TextStyle) TextStyle {
	out := base
	if overlay.Font != nil {
		out.Font = overlay.Font
	}
	if overlay.Size != 0 {
		out.Size = overlay.Size
	}
	if overlay.Color != nil {
		out.Color = overlay.Color
	}
	if overlay.Background != nil {
		out.Background = overlay.Background
	}
	if overlay.HAlign != 0 {
		out.HAlign = overlay.HAlign
	}
	if overlay.VAlign != 0 {
		out.VAlign = overlay.VAlign
	}
	if overlay.LineSpacing != 0 {
		out.LineSpacing = overlay.LineSpacing
	}
	if overlay.Underline {
		out.Underline = true
	}
	if overlay.Strikethrough {
		out.Strikethrough = true
	}
	if overlay.Rotation != 0 {
		out.Rotation = overlay.Rotation
	}
	if overlay.Behind {
		out.Behind = true
	}
	return out
}
