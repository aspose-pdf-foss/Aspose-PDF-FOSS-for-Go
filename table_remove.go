// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// Removing or replacing a table TableAbsorber found (pdf-go-6jxp), mirroring
// Aspose.PDF for .NET's TableAbsorber.Remove(AbsorbedTable) and
// Replace(Page, AbsorbedTable, Table). What the page draws inside the table's
// rectangle — text, rules, fills, images — is removed with the redaction
// machinery: glyphs by their position, paths and images clipped to the
// rectangle. Annotations over the table are left alone.

// tableRemovePad widens the table's rectangle so its own border rules, drawn
// on the rectangle's edges, go with it.
const tableRemovePad = 1.5

// Remove deletes the table from the page it was found on and from the
// absorber's list.
func (ta *TableAbsorber) Remove(t *AbsorbedTable) error {
	if t == nil || t.page == nil {
		return fmt.Errorf("remove table: not a table this absorber found")
	}
	if err := removeTableContent(t); err != nil {
		return err
	}
	ta.forget(t)
	return nil
}

// Replace removes the table and draws table in its place, its top-left
// corner where the old table's was. The new table runs to the page's
// right and bottom edge at most, and continues on new pages if longer.
func (ta *TableAbsorber) Replace(page *Page, t *AbsorbedTable, table *Table) error {
	if t == nil || t.page == nil {
		return fmt.Errorf("replace table: not a table this absorber found")
	}
	if page == nil || table == nil {
		return fmt.Errorf("replace table: nil page or table")
	}
	if page != t.page && (page.doc != t.page.doc || page.Number() != t.page.Number()) {
		return fmt.Errorf("replace table: the table is on page %d, not %d", t.page.Number(), page.Number())
	}
	box, err := page.CropBox()
	if err != nil {
		return err
	}
	if err := removeTableContent(t); err != nil {
		return err
	}
	ta.forget(t)
	rect := Rectangle{LLX: t.Rect.LLX, URY: t.Rect.URY, URX: box.URX, LLY: box.LLY}
	if w := tableWidth(table); t.Rect.LLX+w > rect.URX {
		rect.URX = t.Rect.LLX + w
	}
	if _, err := page.AddTable(table, rect); err != nil {
		return fmt.Errorf("replace table: %w", err)
	}
	return nil
}

func tableWidth(t *Table) float64 {
	w := 0.0
	for _, c := range t.columnWidths {
		w += c
	}
	return w
}

func removeTableContent(t *AbsorbedTable) error {
	p := t.page
	r := t.Rect
	region := QuadPoint{
		X1: r.LLX - tableRemovePad, Y1: r.URY + tableRemovePad,
		X2: r.URX + tableRemovePad, Y2: r.URY + tableRemovePad,
		X3: r.LLX - tableRemovePad, Y3: r.LLY - tableRemovePad,
		X4: r.URX + tableRemovePad, Y4: r.LLY - tableRemovePad,
	}
	old := contentStreamIDs(p.pageDict()["/Contents"])
	if err := redactPageContent(p, []QuadPoint{region}, false); err != nil {
		return fmt.Errorf("remove table: %w", err)
	}
	p.doc.dropUnreachable(old)
	return nil
}

func (ta *TableAbsorber) forget(t *AbsorbedTable) {
	for i, x := range ta.tables {
		if x == t {
			ta.tables = append(ta.tables[:i], ta.tables[i+1:]...)
			return
		}
	}
}
