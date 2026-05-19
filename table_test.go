package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

func TestTable_BorderSideBitmask(t *testing.T) {
	if pdf.BorderSideNone != 0 {
		t.Errorf("BorderSideNone = %d, want 0", pdf.BorderSideNone)
	}
	all := pdf.BorderSideTop | pdf.BorderSideRight | pdf.BorderSideBottom | pdf.BorderSideLeft
	if all != pdf.BorderSideAll {
		t.Errorf("composed All = %d, want BorderSideAll %d", all, pdf.BorderSideAll)
	}
}

func TestTable_BorderInfoZeroValue(t *testing.T) {
	var b pdf.BorderInfo
	if b.Sides != pdf.BorderSideNone || b.Width != 0 || b.Color != nil {
		t.Errorf("BorderInfo zero value = %+v, want zero/zero/nil", b)
	}
}

func TestTable_MarginInfoFields(t *testing.T) {
	m := pdf.MarginInfo{Top: 1, Right: 2, Bottom: 3, Left: 4}
	if m.Top != 1 || m.Right != 2 || m.Bottom != 3 || m.Left != 4 {
		t.Errorf("MarginInfo = %+v", m)
	}
}

func TestTable_NewIsEmpty(t *testing.T) {
	table := pdf.NewTable()
	if table == nil {
		t.Fatal("NewTable returned nil")
	}
	if table.RowCount() != 0 {
		t.Errorf("RowCount = %d, want 0", table.RowCount())
	}
	if len(table.ColumnWidths()) != 0 {
		t.Errorf("ColumnWidths length = %d, want 0", len(table.ColumnWidths()))
	}
	if table.Border().Sides != pdf.BorderSideNone {
		t.Errorf("Border.Sides = %v, want None", table.Border().Sides)
	}
}

func TestTable_SettersAndChaining(t *testing.T) {
	table := pdf.NewTable().
		SetColumnWidths([]float64{100, 200, 100}).
		SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1}).
		SetDefaultCellBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 0.5}).
		SetDefaultCellMargin(pdf.MarginInfo{Top: 4, Right: 6, Bottom: 4, Left: 6}).
		SetDefaultCellStyle(pdf.TextStyle{Size: 10})

	if got := table.ColumnWidths(); len(got) != 3 || got[1] != 200 {
		t.Errorf("ColumnWidths = %v, want [100 200 100]", got)
	}
	if table.Border().Width != 1 {
		t.Errorf("Border.Width = %g, want 1", table.Border().Width)
	}
	if table.DefaultCellBorder().Width != 0.5 {
		t.Errorf("DefaultCellBorder.Width = %g, want 0.5", table.DefaultCellBorder().Width)
	}
	if table.DefaultCellMargin().Left != 6 {
		t.Errorf("DefaultCellMargin.Left = %g, want 6", table.DefaultCellMargin().Left)
	}
	if table.DefaultCellStyle().Size != 10 {
		t.Errorf("DefaultCellStyle.Size = %g, want 10", table.DefaultCellStyle().Size)
	}
}

func TestTable_ColumnWidthsDefensiveCopy(t *testing.T) {
	widths := []float64{1, 2, 3}
	table := pdf.NewTable().SetColumnWidths(widths)
	widths[0] = 999 // mutate caller's slice
	if table.ColumnWidths()[0] == 999 {
		t.Error("SetColumnWidths should defensive-copy")
	}
}

func TestTable_AddRowAndCells(t *testing.T) {
	table := pdf.NewTable().SetColumnWidths([]float64{50, 50})
	row := table.AddRow()
	if row == nil {
		t.Fatal("AddRow returned nil")
	}
	if row.Table() != table {
		t.Error("Row.Table() != owning table")
	}
	if table.RowCount() != 1 {
		t.Errorf("RowCount after AddRow = %d, want 1", table.RowCount())
	}
	c1 := row.AddCell("hello")
	c2 := row.AddCell("world")
	if row.CellCount() != 2 {
		t.Errorf("CellCount = %d, want 2", row.CellCount())
	}
	if c1.Text() != "hello" || c2.Text() != "world" {
		t.Errorf("Cell texts = %q, %q", c1.Text(), c2.Text())
	}
	if c1.Row() != row {
		t.Error("Cell.Row() != owning row")
	}
}

func TestRow_AddCellsConvenience(t *testing.T) {
	table := pdf.NewTable().SetColumnWidths([]float64{50, 50, 50})
	row := table.AddRow()
	cells := row.AddCells("a", "b", "c")
	if len(cells) != 3 || cells[1].Text() != "b" {
		t.Errorf("AddCells = %v", cells)
	}
	if row.CellCount() != 3 {
		t.Errorf("CellCount after AddCells = %d, want 3", row.CellCount())
	}
}

func TestRow_SetHeight(t *testing.T) {
	row := pdf.NewTable().SetColumnWidths([]float64{50}).AddRow()
	if row.Height() != 0 {
		t.Errorf("default Height = %g, want 0 (auto)", row.Height())
	}
	row.SetHeight(25)
	if row.Height() != 25 {
		t.Errorf("Height after SetHeight(25) = %g", row.Height())
	}
}

func TestCell_SettersAndChaining(t *testing.T) {
	row := pdf.NewTable().SetColumnWidths([]float64{100}).AddRow()
	bg := &pdf.Color{R: 1, G: 1, B: 0, A: 1}
	cell := row.AddCell("x").
		SetText("y").
		SetTextStyle(pdf.TextStyle{Size: 12}).
		SetBackground(bg).
		SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideTop, Width: 2}).
		SetMargin(pdf.MarginInfo{Top: 1, Right: 2, Bottom: 3, Left: 4}).
		SetHAlign(pdf.HAlignCenter).
		SetVAlign(pdf.VAlignMiddle)

	if cell.Text() != "y" {
		t.Errorf("Text = %q, want y", cell.Text())
	}
	if cell.TextStyle() == nil || cell.TextStyle().Size != 12 {
		t.Errorf("TextStyle = %+v", cell.TextStyle())
	}
	if cell.Background() != bg {
		t.Error("Background pointer not preserved")
	}
	if cell.Border() == nil || cell.Border().Sides != pdf.BorderSideTop {
		t.Errorf("Border = %+v", cell.Border())
	}
	if cell.Margin() == nil || cell.Margin().Left != 4 {
		t.Errorf("Margin = %+v", cell.Margin())
	}
}

func TestCell_DefaultsAreNil(t *testing.T) {
	cell := pdf.NewTable().SetColumnWidths([]float64{50}).AddRow().AddCell("x")
	if cell.TextStyle() != nil {
		t.Error("default TextStyle should be nil (inherit)")
	}
	if cell.Background() != nil {
		t.Error("default Background should be nil")
	}
	if cell.Border() != nil {
		t.Error("default Border should be nil (inherit)")
	}
	if cell.Margin() != nil {
		t.Error("default Margin should be nil (inherit)")
	}
}
