package asposepdf_test

import (
	"fmt"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
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

	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 600, URX: 450, URY: 750}); err != nil {
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

	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 700}); err != nil {
		t.Fatal(err)
	}
}

// Aspose .NET sample:
//   Table table = new Table();
//   table.ColumnWidths = "80 160 80";
//   table.RepeatingRowsCount = 1;
//   Row header = table.Rows.Add();
//   header.Cells.Add("Header A");
//   header.Cells.Add("Header B");
//   header.Cells.Add("Header C");
//   for (int i = 0; i < 30; i++) {
//       Row r = table.Rows.Add();
//       r.Cells.Add("a"+i); r.Cells.Add("b"+i); r.Cells.Add("c"+i);
//   }
//   page.Paragraphs.Add(table); // auto-flows across pages
func TestAsposeParity_TableRepeatingRows(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	table := pdf.NewTable().
		SetColumnWidths([]float64{80, 160, 80}).
		SetRepeatingRowsCount(1)
	header := table.AddRow()
	header.AddCells("Header A", "Header B", "Header C")
	for i := 0; i < 30; i++ {
		table.AddRow().AddCells(
			fmt.Sprintf("a%d", i),
			fmt.Sprintf("b%d", i),
			fmt.Sprintf("c%d", i),
		)
	}

	pagesAdded, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 100, URX: 370, URY: 320})
	if err != nil {
		t.Fatal(err)
	}
	if pagesAdded < 1 {
		t.Fatalf("expected overflow, pagesAdded = %d", pagesAdded)
	}
}

// Aspose .NET sample:
//   Cell cell = row.Cells.Add("TOTAL");
//   cell.ColSpan = 2;
//   cell.Alignment = HorizontalAlignment.Right;
func TestAsposeParity_CellColSpan(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	table := pdf.NewTable().SetColumnWidths([]float64{80, 80, 80, 80})
	row := table.AddRow()
	row.AddCell("Item 1").SetHAlign(pdf.HAlignLeft)
	row.AddCell("Item 2").SetHAlign(pdf.HAlignLeft)
	row.AddCell("TOTAL").SetColSpan(2).SetHAlign(pdf.HAlignRight)

	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 600, URX: 370, URY: 700}); err != nil {
		t.Fatal(err)
	}
}

// Aspose .NET sample:
//   Cell cell = row.Cells.Add("Category");
//   cell.RowSpan = 3;
func TestAsposeParity_CellRowSpan(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	table := pdf.NewTable().SetColumnWidths([]float64{100, 100})
	row0 := table.AddRow()
	row0.AddCell("Category").SetRowSpan(3)
	row0.AddCell("Item 1")
	table.AddRow().AddCell("Item 2") // col 0 covered by rowspan
	table.AddRow().AddCell("Item 3") // col 0 covered by rowspan

	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 700}); err != nil {
		t.Fatal(err)
	}
}

// Aspose .NET sample:
//   Cell cell = row.Cells.Add();
//   cell.Image = new Image { File = "logo.png" };
func TestAsposeParity_CellImage(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{200})
	table.AddRow().AddCell("").SetImage("testdata/Koala.jpg")
	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 500, URX: 250, URY: 750}); err != nil {
		t.Fatal(err)
	}
}

// Aspose .NET sample:
//   Row row = table.Rows.Add();
//   row.BackgroundColor = Color.LightGray;
//   row.DefaultCellTextState = new TextState { FontSize = 14 };
func TestAsposeParity_RowStyling(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{100, 100})
	table.AddRow().
		SetBackground(&pdf.Color{R: 0.83, G: 0.83, B: 0.83, A: 1}).
		SetTextStyle(pdf.TextStyle{Size: 14}).
		AddCells("Header A", "Header B")
	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 700}); err != nil {
		t.Fatal(err)
	}
}

// Aspose .NET-style batch row construction (no exact 1:1 in .NET — closest is
// LINQ enumeration with explicit Row construction). Our AddRows is the
// Go-idiomatic equivalent, returning rows for per-row customization.
func TestAsposeParity_AddRowsBatch(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{80, 80, 80})
	rows := table.AddRows([][]string{
		{"Alice", "Engineering", "23"},
		{"Bob", "Marketing", "17"},
		{"Carol", "Operations", "9"},
	})
	for _, r := range rows {
		r.SetBackground(&pdf.Color{R: 0.97, G: 0.97, B: 0.97, A: 1})
	}
	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 600, URX: 290, URY: 700}); err != nil {
		t.Fatal(err)
	}
}
