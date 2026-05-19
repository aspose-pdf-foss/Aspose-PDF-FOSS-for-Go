package asposepdf

import "fmt"

// AddTable renders the table inside the given rectangle. Per the package
// design, cell content is drawn using each cell's TextStyle override (or the
// table-level DefaultCellStyle), with per-cell padding (margin) applied.
// The table is clipped to the bounding rectangle; rows that don't fit are
// not drawn.
//
// Mirrors Aspose.PDF for .NET's flow-layout Table rendering, but uses
// explicit Rectangle positioning (consistent with AddText / AddImage).
func (p *Page) AddTable(t *Table, rect Rectangle) error {
	if t == nil {
		return fmt.Errorf("add table: nil table")
	}
	if err := rect.validate(); err != nil {
		return fmt.Errorf("add table: %w", err)
	}
	if len(t.columnWidths) == 0 {
		// Empty table — nothing to draw.
		return nil
	}
	for i, w := range t.columnWidths {
		if w <= 0 {
			return fmt.Errorf("add table: column %d has non-positive width %g", i, w)
		}
	}
	for i, row := range t.rows {
		if len(row.cells) != len(t.columnWidths) {
			return fmt.Errorf("add table: row %d has %d cells, want %d", i, len(row.cells), len(t.columnWidths))
		}
	}
	if len(t.rows) == 0 {
		return nil
	}
	// Row-height pass + cell rendering arrive in Tasks 5–8.
	return nil
}
