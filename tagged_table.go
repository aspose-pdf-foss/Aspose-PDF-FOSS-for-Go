// SPDX-License-Identifier: MIT

package asposepdf

import "fmt"

// tableTagger drives structure-tree tagging while a table is rendered: it owns
// the /Table element and creates a /TR per row and a /TH/TD per cell as cells
// are drawn. Header rows (the table's repeating rows) become /TH.
type tableTagger struct {
	table      *StructElement
	headerRows int
	rows       map[int]*StructElement // row index → /TR (reused across page breaks)
}

// AddTaggedTable renders t inside rect like AddTable, and at the same time builds
// the table's logical structure (/Table → /TR → /TH/TD) under parent (nil = the
// document root), tagging each cell's content as marked content and bracketing
// cell backgrounds and borders as /Artifact. The first RepeatingRowsCount rows
// are tagged as header cells (/TH). Returns the /Table structure element and the
// number of continuation pages appended. Requires Document.TaggedContent to have
// been called. Mirrors the table tagging of Aspose.PDF for .NET's tagged content.
func (p *Page) AddTaggedTable(tc *TaggedContent, parent *StructElement, t *Table, rect Rectangle) (*StructElement, int, error) {
	if tc == nil || p.doc == nil || p.doc.tagged == nil {
		return nil, 0, fmt.Errorf("AddTaggedTable: call Document.TaggedContent() first")
	}
	if t == nil {
		return nil, 0, fmt.Errorf("AddTaggedTable: nil table")
	}
	if parent == nil {
		parent = tc.root
	}
	tableElem := parent.AddChild(StructTable)
	t.tagger = &tableTagger{
		table:      tableElem,
		headerRows: t.repeatingRowsCount,
		rows:       map[int]*StructElement{},
	}
	defer func() { t.tagger = nil }()

	pages, err := p.AddTable(t, rect)
	return tableElem, pages, err
}

// rowElem returns the /TR for the given row index, creating it on first use.
func (tt *tableTagger) rowElem(rowIdx int) *StructElement {
	if tr, ok := tt.rows[rowIdx]; ok {
		return tr
	}
	tr := tt.table.AddChild(StructTR)
	tt.rows[rowIdx] = tr
	return tr
}

// tagCell wraps a cell's content draw in a /TH or /TD structure element.
func (tt *tableTagger) tagCell(page *Page, rowIdx int, draw func() error) error {
	st := StructTD
	if rowIdx < tt.headerRows {
		st = StructTH
	}
	_, err := page.TagContent(tt.rowElem(rowIdx), st, draw)
	return err
}

// artifact brackets decorative drawing (cell backgrounds, borders) in an
// /Artifact marked-content sequence so it is excluded from the logical
// structure, as PDF/UA requires.
func (p *Page) artifact(draw func() error) error {
	if err := p.appendToContentStream([]byte("/Artifact BMC\n")); err != nil {
		return err
	}
	if err := draw(); err != nil {
		return err
	}
	return p.appendToContentStream([]byte("EMC\n"))
}
