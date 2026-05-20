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
// continuation rectangle is computed from t.OverflowMargins(). Repeating
// header rows (see Table.SetRepeatingRowsCount) draw at the top of each page,
// including the original.
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
	if t.repeatingRowsCount < 0 {
		return 0, fmt.Errorf("add table: repeating rows count %d is negative", t.repeatingRowsCount)
	}
	if t.repeatingRowsCount > len(t.rows) {
		return 0, fmt.Errorf("add table: repeating rows count %d exceeds row count %d",
			t.repeatingRowsCount, len(t.rows))
	}
	// Rowspan crossing the header/body boundary is rejected (Phase 2 hard rule).
	if t.repeatingRowsCount > 0 {
		for i := 0; i < t.repeatingRowsCount; i++ {
			for _, cell := range t.rows[i].cells {
				rs := cell.RowSpan()
				if i+rs > t.repeatingRowsCount {
					return 0, fmt.Errorf(
						"add table: rowspan at header row %d (span %d) extends into body (rowspan-cross-header not supported)",
						i, rs)
				}
			}
		}
	}
	heights, err := computeRowHeights(t)
	if err != nil {
		return 0, fmt.Errorf("add table: %w", err)
	}

	// Pre-compute xOffsets (running sum of column widths).
	xOffsets := make([]float64, len(t.columnWidths)+1)
	for i, w := range t.columnWidths {
		xOffsets[i+1] = xOffsets[i] + w
	}

	// Compute continuation rect (used on auto-appended pages).
	overflowTop, overflowBottom := t.OverflowMargins()
	sz, err := p.Size()
	if err != nil {
		return 0, fmt.Errorf("add table: page size: %w", err)
	}
	continuationRect := Rectangle{
		LLX: rect.LLX,
		LLY: overflowBottom,
		URX: rect.URX,
		URY: sz.Height - overflowTop,
	}
	continuationHeight := continuationRect.URY - continuationRect.LLY
	if continuationHeight <= 0 {
		return 0, fmt.Errorf(
			"add table: continuation rect has non-positive height (page %g, margins top=%g bottom=%g)",
			sz.Height, overflowTop, overflowBottom)
	}

	// Compute spanning groups for the body (skip header rows).
	groups := computeSpanningGroups(t, t.repeatingRowsCount)

	// Validate header + group heights against available rectangles.
	headerHeight := 0.0
	for i := 0; i < t.repeatingRowsCount; i++ {
		headerHeight += heights[i]
	}
	if headerHeight > rect.URY-rect.LLY {
		return 0, fmt.Errorf("add table: header rows height %g exceeds initial rect height %g",
			headerHeight, rect.URY-rect.LLY)
	}
	if headerHeight > continuationHeight {
		return 0, fmt.Errorf("add table: header rows height %g exceeds continuation rect height %g",
			headerHeight, continuationHeight)
	}
	for _, g := range groups {
		gh := 0.0
		for r := g.start; r <= g.end; r++ {
			gh += heights[r]
		}
		if gh > continuationHeight-headerHeight {
			return 0, fmt.Errorf("add table: group [%d..%d] height %g exceeds continuation rect body height %g",
				g.start, g.end, gh, continuationHeight-headerHeight)
		}
	}

	// Render loop.
	pagesAdded := 0
	currentPage := p
	currentRect := rect
	y := currentRect.URY
	pageDrawn := 0.0

	// Headers on the first page.
	if t.repeatingRowsCount > 0 {
		h, err := drawRowRange(currentPage, t, 0, t.repeatingRowsCount-1, currentRect, y, covered, xOffsets, heights)
		if err != nil {
			return pagesAdded, fmt.Errorf("add table: headers: %w", err)
		}
		y -= h
		pageDrawn += h
	}

	// Walk body groups.
	for _, g := range groups {
		groupH := 0.0
		for r := g.start; r <= g.end; r++ {
			groupH += heights[r]
		}
		if y-groupH < currentRect.LLY {
			// Overflow: finish outer border on current page, append a new page.
			if err := drawOuterBorder(currentPage, t, currentRect, currentRect.URY, pageDrawn, xOffsets); err != nil {
				return pagesAdded, fmt.Errorf("add table: outer border (page break): %w", err)
			}

			if err := p.doc.AddBlankPage(sz.Width, sz.Height); err != nil {
				return pagesAdded, fmt.Errorf("add table: append page: %w", err)
			}
			pagesAdded++
			np, err := p.doc.Page(p.doc.PageCount())
			if err != nil {
				return pagesAdded, fmt.Errorf("add table: continuation page: %w", err)
			}
			currentPage = np
			currentRect = continuationRect
			y = currentRect.URY
			pageDrawn = 0.0

			// Repeat headers on the new page.
			if t.repeatingRowsCount > 0 {
				h, err := drawRowRange(currentPage, t, 0, t.repeatingRowsCount-1, currentRect, y, covered, xOffsets, heights)
				if err != nil {
					return pagesAdded, fmt.Errorf("add table: repeated headers: %w", err)
				}
				y -= h
				pageDrawn += h
			}
		}

		h, err := drawRowRange(currentPage, t, g.start, g.end, currentRect, y, covered, xOffsets, heights)
		if err != nil {
			return pagesAdded, fmt.Errorf("add table: group [%d..%d]: %w", g.start, g.end, err)
		}
		y -= h
		pageDrawn += h
	}

	// Final outer border on the last page.
	if err := drawOuterBorder(currentPage, t, currentRect, currentRect.URY, pageDrawn, xOffsets); err != nil {
		return pagesAdded, fmt.Errorf("add table: outer border (final): %w", err)
	}

	return pagesAdded, nil
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

// spanGroup is a contiguous range of rows that must be drawn together (no
// page break inside). [start, end] are inclusive row indices.
type spanGroup struct {
	start, end int
}

// computeSpanningGroups computes the maximal "atomic" groups of rows starting
// at startIdx. Within a group, no rowspan extends beyond the group's last row.
// Each returned group is the unit that page-break logic moves as a whole.
func computeSpanningGroups(t *Table, startIdx int) []spanGroup {
	var groups []spanGroup
	i := startIdx
	numRows := len(t.rows)
	for i < numRows {
		g := spanGroup{start: i, end: i}
		// Walk j from i upwards, extending g.end whenever a rowspan reaches further.
		j := i
		for j <= g.end {
			for _, cell := range t.rows[j].cells {
				rs := cell.RowSpan()
				if rs < 1 {
					rs = 1
				}
				spanEnd := j + rs - 1
				if spanEnd > g.end {
					g.end = spanEnd
				}
			}
			j++
		}
		groups = append(groups, g)
		i = g.end + 1
	}
	return groups
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

	// Span-aware iteration needs the covered grid. Call validateAndCover here;
	// AddTable also calls it — both calls produce identical output. For MVP
	// this O(rows*cols) duplicate work is acceptable.
	covered, err := validateAndCover(t)
	if err != nil {
		return nil, err
	}

	for i, row := range t.rows {
		if row.height > 0 {
			heights[i] = row.height
			continue
		}
		maxH := 0.0
		col := 0
		for _, cell := range row.cells {
			// Skip positions covered by inherited rowspans.
			for col < len(t.columnWidths) && covered[i][col] {
				col++
			}
			cs := cell.ColSpan()
			rs := cell.RowSpan()
			// Phase 3: image cells — auto-fit to interior width, scale height proportionally.
			// rowspan image cells are handled by the same exclusion as rowspan text cells below.
			if cell.hasImage && rs == 1 {
				sumW := 0.0
				for c := 0; c < cs; c++ {
					sumW += t.columnWidths[col+c]
				}
				margin := effectiveCellMargin(t, cell)
				interiorWidth := sumW - margin.Left - margin.Right
				if interiorWidth < 0 {
					interiorWidth = 0
				}
				var src []byte
				if cell.imageStream != nil {
					src = cell.imageStream
				}
				natW, natH, err := measureImage(cell.imagePath, src)
				if err != nil {
					return nil, fmt.Errorf("row %d col %d image: %w", i, col, err)
				}
				var scaledH float64
				if natW > 0 {
					scaledH = natH * (interiorWidth / natW)
				}
				cellH := scaledH + margin.Top + margin.Bottom
				if cellH > maxH {
					maxH = cellH
				}
				col += cs
				continue
			}
			// Skip rowspan cells: their height is checked separately (currently
			// they're allowed to clip if too tall — matches AddText clip semantics).
			if rs > 1 {
				col += cs
				continue
			}
			// Interior width = sum of cs column widths - margins.
			sumW := 0.0
			for c := 0; c < cs; c++ {
				sumW += t.columnWidths[col+c]
			}
			margin := effectiveCellMargin(t, cell)
			style := effectiveCellStyle(t, cell)
			interiorWidth := sumW - margin.Left - margin.Right
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
			col += cs
		}
		heights[i] = maxH
	}
	return heights, nil
}

// effectiveCellMargin returns the resolved margin for a cell, walking the
// per-cell → per-row → table-default chain.
func effectiveCellMargin(t *Table, c *Cell) MarginInfo {
	if c.margin != nil {
		return *c.margin
	}
	if c.row != nil && c.row.margin != nil {
		return *c.row.margin
	}
	return t.defaultCellMargin
}

// effectiveCellBackground walks the per-cell → per-row chain. Returns nil if
// neither cell nor row sets a background.
func effectiveCellBackground(c *Cell) *Color {
	if c.background != nil {
		return c.background
	}
	if c.row != nil && c.row.background != nil {
		return c.row.background
	}
	return nil
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

// effectiveCellBorder returns the resolved border for a cell, walking the
// per-cell → per-row → table-default chain.
func effectiveCellBorder(t *Table, c *Cell) BorderInfo {
	if c.border != nil {
		return *c.border
	}
	if c.row != nil && c.row.border != nil {
		return *c.row.border
	}
	return t.defaultCellBorder
}

// effectiveCellStyle returns the resolved TextStyle for a cell, layering:
// table.defaultCellStyle ← row.textStyle overlay ← cell.style overlay ← cell H/V align overrides.
func effectiveCellStyle(t *Table, c *Cell) TextStyle {
	style := t.defaultCellStyle
	if c.row != nil && c.row.textStyle != nil {
		style = overlayTextStyle(style, *c.row.textStyle)
	}
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

// drawRowRange renders rows [startRow..endRow] (inclusive) of t on targetPage,
// using rect.LLX as the left origin and topY as the top edge of the first row.
// Returns the total height of rows actually drawn.
//
// covered:  pre-computed coverage grid from validateAndCover.
// xOffsets: pre-computed running-sum of columnWidths.
// heights:  pre-computed row heights.
func drawRowRange(
	targetPage *Page, t *Table,
	startRow, endRow int,
	rect Rectangle, topY float64,
	covered [][]bool, xOffsets, heights []float64,
) (drawnHeight float64, err error) {
	y := topY
	for i := startRow; i <= endRow; i++ {
		rowH := heights[i]
		col := 0
		for _, cell := range t.rows[i].cells {
			for col < len(t.columnWidths) && covered[i][col] {
				col++
			}
			cs := cell.ColSpan()
			rs := cell.RowSpan()
			cellLLX := rect.LLX + xOffsets[col]
			cellURX := rect.LLX + xOffsets[col+cs]
			cellURY := y
			spanH := rowH
			for r := 1; r < rs; r++ {
				spanH += heights[i+r]
			}
			cellLLY := cellURY - spanH

			margin := effectiveCellMargin(t, cell)
			style := effectiveCellStyle(t, cell)

			if bg := effectiveCellBackground(cell); bg != nil {
				if err := targetPage.appendToContentStream([]byte(
					drawCellBackground(cellLLX, cellLLY, cellURX, cellURY, bg),
				)); err != nil {
					return drawnHeight, fmt.Errorf("row %d col %d background: %w", i, col, err)
				}
			}
			interior := Rectangle{
				LLX: cellLLX + margin.Left,
				LLY: cellLLY + margin.Bottom,
				URX: cellURX - margin.Right,
				URY: cellURY - margin.Top,
			}
			if interior.URX > interior.LLX && interior.URY > interior.LLY {
				if cell.hasImage {
					if err := drawImageInCell(targetPage, cell, interior, style); err != nil {
						return drawnHeight, fmt.Errorf("row %d col %d image: %w", i, col, err)
					}
				} else if cell.text != "" {
					if err := targetPage.AddText(cell.text, style, interior); err != nil {
						return drawnHeight, fmt.Errorf("row %d col %d text: %w", i, col, err)
					}
				}
			}
			border := effectiveCellBorder(t, cell)
			if ops := drawBorderSides(cellLLX, cellLLY, cellURX, cellURY, border); ops != "" {
				if err := targetPage.appendToContentStream([]byte(ops)); err != nil {
					return drawnHeight, fmt.Errorf("row %d col %d border: %w", i, col, err)
				}
			}
			col += cs
		}
		y -= rowH
		drawnHeight += rowH
	}
	return drawnHeight, nil
}

// drawOuterBorder draws the table's outer border around the given drawn area
// on targetPage. No-op if t.border.Sides is BorderSideNone or width is 0.
func drawOuterBorder(targetPage *Page, t *Table, rect Rectangle, topY, drawnHeight float64, xOffsets []float64) error {
	if drawnHeight <= 0 {
		return nil
	}
	totalW := xOffsets[len(t.columnWidths)]
	ops := drawBorderSides(
		rect.LLX, topY-drawnHeight,
		rect.LLX+totalW, topY,
		t.border,
	)
	if ops == "" {
		return nil
	}
	return targetPage.appendToContentStream([]byte(ops))
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
