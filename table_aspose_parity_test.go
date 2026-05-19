package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

// Aspose .NET sample:
//   Table table = new Table();
//   table.ColumnWidths = "100 200 100";
//   table.Border = new BorderInfo(BorderSide.All, 1f);
//   table.DefaultCellBorder = new BorderInfo(BorderSide.All, 0.5f);
//   Row row = table.Rows.Add();
//   row.Cells.Add("Header A");
//   row.Cells.Add("Header B");
//   row.Cells.Add("Header C");
//   page.Paragraphs.Add(table);
func TestAsposeParity_TableBasic(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	table := pdf.NewTable().
		SetColumnWidths([]float64{100, 200, 100}).
		SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1}).
		SetDefaultCellBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 0.5})

	row := table.AddRow()
	row.AddCell("Header A")
	row.AddCell("Header B")
	row.AddCell("Header C")

	if err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 600, URX: 450, URY: 750}); err != nil {
		t.Fatal(err)
	}
}

// Aspose .NET sample:
//   Cell cell = row.Cells.Add("colored");
//   cell.BackgroundColor = Color.Yellow;
//   cell.Alignment = HorizontalAlignment.Center;
//   cell.VerticalAlignment = VerticalAlignment.Center;
//   cell.Margin = new MarginInfo(5, 5, 5, 5);
func TestAsposeParity_CellOverrides(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	table := pdf.NewTable().SetColumnWidths([]float64{100})
	cell := table.AddRow().AddCell("colored")
	cell.SetBackground(&pdf.Color{R: 1, G: 1, B: 0, A: 1})
	cell.SetHAlign(pdf.HAlignCenter)
	cell.SetVAlign(pdf.VAlignMiddle)
	cell.SetMargin(pdf.MarginInfo{Top: 5, Right: 5, Bottom: 5, Left: 5})

	if err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 700}); err != nil {
		t.Fatal(err)
	}
}
