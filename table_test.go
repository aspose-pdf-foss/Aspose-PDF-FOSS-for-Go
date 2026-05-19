package asposepdf_test

import (
	"bytes"
	"compress/zlib"
	"io"
	"regexp"
	"strings"
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

// renderedContent returns the concatenated, FlateDecoded content stream bytes
// produced by doc.WriteTo, so tests can grep for raw PDF operators (which the
// writer otherwise stores compressed).
func renderedContent(t *testing.T, doc *pdf.Document) string {
	t.Helper()
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	data := buf.Bytes()
	re := regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`)
	var out strings.Builder
	for _, m := range re.FindAllSubmatch(data, -1) {
		body := m[1]
		zr, err := zlib.NewReader(bytes.NewReader(body))
		if err != nil {
			// Not a flate stream — append as-is (e.g. uncompressed object).
			out.Write(body)
			continue
		}
		dec, err := io.ReadAll(zr)
		zr.Close()
		if err != nil {
			continue
		}
		out.Write(dec)
		out.WriteByte('\n')
	}
	return out.String()
}

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

func TestAddTable_NilTable(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.AddTable(nil, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100})
	if err == nil {
		t.Error("AddTable(nil) should error")
	}
}

func TestAddTable_EmptyTable(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable() // no columns, no rows
	err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100})
	if err != nil {
		t.Errorf("AddTable(empty) = %v, want nil (no-op)", err)
	}
}

func TestAddTable_NoRows(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{50, 50})
	err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100})
	if err != nil {
		t.Errorf("AddTable(no-rows) = %v, want nil", err)
	}
}

func TestAddTable_BadRect(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{50})
	table.AddRow().AddCell("x")
	// LLX >= URX
	err := page.AddTable(table, pdf.Rectangle{LLX: 100, LLY: 0, URX: 0, URY: 100})
	if err == nil {
		t.Error("AddTable with bad rect should error")
	}
}

func TestAddTable_MismatchedCellCount(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{50, 50, 50}) // 3 cols
	table.AddRow().AddCells("a", "b")                              // 2 cells
	err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 100})
	if err == nil {
		t.Fatal("AddTable with mismatched cell count should error")
	}
}

func TestAddTable_NonPositiveColumnWidth(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{50, 0, 50})
	table.AddRow().AddCells("a", "b", "c")
	err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 100})
	if err == nil {
		t.Fatal("AddTable with zero column width should error")
	}

	table2 := pdf.NewTable().SetColumnWidths([]float64{50, -1, 50})
	table2.AddRow().AddCells("a", "b", "c")
	err2 := page.AddTable(table2, pdf.Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 100})
	if err2 == nil {
		t.Fatal("AddTable with negative column width should error")
	}
}

func TestAddTable_CellTextAppears(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	table := pdf.NewTable().
		SetColumnWidths([]float64{100, 100, 100}).
		SetDefaultCellStyle(pdf.TextStyle{Size: 10}).
		SetDefaultCellMargin(pdf.MarginInfo{Top: 2, Right: 4, Bottom: 2, Left: 4})

	table.AddRow().AddCells("alpha", "beta", "gamma")
	table.AddRow().AddCells("delta", "epsilon", "zeta")

	err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 600, URX: 350, URY: 750})
	if err != nil {
		t.Fatal(err)
	}

	text, err := page.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta"} {
		if !strings.Contains(text, want) {
			t.Errorf("ExtractText missing %q. Got: %q", want, text)
		}
	}
}

func TestAddTable_RoundTripText(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{100, 100})
	table.AddRow().AddCells("hello", "world")

	if err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 700}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	page2, _ := doc2.Page(1)
	text, _ := page2.ExtractText()
	if !strings.Contains(text, "hello") || !strings.Contains(text, "world") {
		t.Errorf("roundtrip lost cell text: %q", text)
	}
}

func TestAddTable_BackgroundFillEmitted(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{50})
	cell := table.AddRow().AddCell("x")
	cell.SetBackground(&pdf.Color{R: 1, G: 0, B: 0, A: 1})

	if err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}); err != nil {
		t.Fatal(err)
	}

	s := renderedContent(t, doc)
	// Red fill: 1 0 0 rg ... re f
	if !strings.Contains(s, "1 0 0 rg") {
		t.Error("output missing red fill color")
	}
}

func TestAddTable_BorderSidesMask(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{50})
	table.AddRow().AddCell("x").
		SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideTop | pdf.BorderSideBottom, Width: 1})

	if err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}); err != nil {
		t.Fatal(err)
	}

	s := renderedContent(t, doc)
	// Two strokes from the cell border (Top + Bottom).
	strokeCount := strings.Count(s, " S\n")
	if strokeCount < 2 {
		t.Errorf("strokes = %d, want >= 2 (top + bottom)", strokeCount)
	}
}

func TestAddTable_ZeroWidthBorderNotDrawn(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{50})
	table.AddRow().AddCell("x").
		SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 0})

	if err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}); err != nil {
		t.Fatal(err)
	}

	s := renderedContent(t, doc)
	if strings.Contains(s, " S\n") {
		t.Error("zero-width border should not emit stroke")
	}
}

func TestAddTable_CellOverridesDefaultBorder(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().
		SetColumnWidths([]float64{50, 50}).
		SetDefaultCellBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 0.5})
	row := table.AddRow()
	row.AddCell("a") // inherits default
	row.AddCell("b").SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 3})

	if err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}); err != nil {
		t.Fatal(err)
	}

	s := renderedContent(t, doc)
	if !strings.Contains(s, "0.5 w") {
		t.Error("default border width 0.5 missing")
	}
	if !strings.Contains(s, "3 w") {
		t.Error("cell-override border width 3 missing")
	}
}

func TestAddTable_OuterBorderDrawn(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().
		SetColumnWidths([]float64{50, 50}).
		SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 2})
	table.AddRow().AddCells("a", "b")

	if err := page.AddTable(table, pdf.Rectangle{LLX: 100, LLY: 100, URX: 200, URY: 150}); err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	if !strings.Contains(s, "2 w") {
		t.Error("outer border width 2 missing")
	}
	// Outer border = 4 strokes minimum.
	if strings.Count(s, " S\n") < 4 {
		t.Errorf("strokes = %d, want >= 4 (outer border)", strings.Count(s, " S\n"))
	}
}

func TestAddTable_OuterBorderNoneNoStrokes(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	// No table border, no cell borders → no strokes at all.
	table := pdf.NewTable().SetColumnWidths([]float64{50})
	table.AddRow().AddCell("x")

	if err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}); err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	if strings.Contains(s, " S\n") {
		t.Error("no border configured but stroke present")
	}
}

func TestAddTable_OuterBorderClampedToDrawnRows(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	// 3 rows × ~22pt each, but rect height only fits 1 row (~25pt).
	table := pdf.NewTable().
		SetColumnWidths([]float64{100}).
		SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideBottom, Width: 1}).
		SetDefaultCellStyle(pdf.TextStyle{Size: 12}).
		SetDefaultCellMargin(pdf.MarginInfo{Top: 4, Right: 4, Bottom: 4, Left: 4})
	table.AddRow().AddCell("one")
	table.AddRow().AddCell("two")
	table.AddRow().AddCell("three")

	if err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 25}); err != nil {
		t.Fatal(err)
	}

	s := renderedContent(t, doc)

	// Outer border bottom stroke should be at Y near rect.URY - drawnRowHeight,
	// NOT at rect.LLY = 0. The bottom-side stroke is emitted by drawBorderSides
	// as "URX LLY m LLX LLY l S\n", so with the bug it would be
	// "100 0 m 0 0 l S\n". With the fix LLY > 0 so that exact sequence must not
	// appear.
	if strings.Contains(s, "100 0 m 0 0 l S\n") {
		t.Error("outer border bottom at rect.LLY=0; expected at drawn-rows boundary")
	}
}

func TestAddTable_RowsBeyondRectAreClipped(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	// 3 rows, each ~22pt with default 12pt font + default margins. Rect is
	// only 30pt tall, so only the first row fits.
	table := pdf.NewTable().
		SetColumnWidths([]float64{100}).
		SetDefaultCellStyle(pdf.TextStyle{Size: 12}).
		SetDefaultCellMargin(pdf.MarginInfo{Top: 4, Right: 4, Bottom: 4, Left: 4})
	table.AddRow().AddCell("rowOne")
	table.AddRow().AddCell("rowTwo")
	table.AddRow().AddCell("rowThree")

	if err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 700, URX: 200, URY: 730}); err != nil {
		t.Fatal(err)
	}

	text, _ := page.ExtractText()
	if !strings.Contains(text, "rowOne") {
		t.Error("rowOne should have been drawn")
	}
	if strings.Contains(text, "rowThree") {
		t.Error("rowThree should have been clipped (rect too short)")
	}
}

func TestAddTable_AES128Roundtrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{100, 100})
	table.AddRow().AddCells("secret", "data")

	if err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 700}); err != nil {
		t.Fatal(err)
	}
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword: "x",
		Algorithm:    pdf.EncryptionAlgAES128,
	})

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	doc2, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "x")
	if err != nil {
		t.Fatal(err)
	}
	page2, _ := doc2.Page(1)
	text, _ := page2.ExtractText()
	if !strings.Contains(text, "secret") || !strings.Contains(text, "data") {
		t.Errorf("AES-128 roundtrip lost table text: %q", text)
	}
}

func TestAddTable_WithEmbeddedTTF(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	// Use the project's existing TTF fixture.
	font, err := doc.LoadFont("testdata/DejaVuSans.ttf")
	if err != nil {
		t.Fatalf("LoadFont: %v", err)
	}

	table := pdf.NewTable().
		SetColumnWidths([]float64{120, 120}).
		SetDefaultCellStyle(pdf.TextStyle{Font: font, Size: 11}).
		SetDefaultCellMargin(pdf.MarginInfo{Top: 3, Right: 5, Bottom: 3, Left: 5})

	// Unicode payload — exercises CID/Identity-H path that only embedded fonts handle.
	table.AddRow().AddCells("Cyrillic Привет", "Greek Γειά")

	if err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 600, URX: 290, URY: 700}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	page2, _ := doc2.Page(1)
	text, err := page2.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Привет") {
		t.Errorf("Cyrillic lost through TTF+table roundtrip: %q", text)
	}
	if !strings.Contains(text, "Γειά") {
		t.Errorf("Greek lost through TTF+table roundtrip: %q", text)
	}
}

func TestAddTable_HAlignCenter(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().
		SetColumnWidths([]float64{200}).
		SetDefaultCellMargin(pdf.MarginInfo{Top: 2, Right: 2, Bottom: 2, Left: 2})
	table.AddRow().AddCell("X").SetHAlign(pdf.HAlignCenter)

	if err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 600, URX: 200, URY: 700}); err != nil {
		t.Fatal(err)
	}

	layout, err := page.ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	if len(layout) == 0 || len(layout[0].Fragments) == 0 {
		t.Fatal("no text fragments extracted")
	}
	f := layout[0].Fragments[0]
	// For HAlignCenter, the "X" glyph should be near the column midpoint:
	// column [0, 200], interior [2, 198], midpoint ~100. Glyph width ~6pt at 12pt,
	// so X should be at ~97.
	midpoint := (0.0 + 200.0) / 2
	if f.X < midpoint-30 || f.X > midpoint+30 {
		t.Errorf("HAlignCenter glyph X = %g, want near midpoint %g (±30)", f.X, midpoint)
	}
}

func TestAddTable_CellStyleOverridesDefault(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().
		SetColumnWidths([]float64{100, 100}).
		SetDefaultCellStyle(pdf.TextStyle{Size: 10})
	row := table.AddRow()
	row.AddCell("default")
	row.AddCell("big").SetTextStyle(pdf.TextStyle{Size: 18})

	if err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 600, URX: 200, URY: 700}); err != nil {
		t.Fatal(err)
	}

	layout, err := page.ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	// Find fragments for "default" and "big" and check FontSize differs.
	var defaultSize, bigSize float64
	for _, line := range layout {
		for _, f := range line.Fragments {
			if strings.Contains(f.Text, "default") {
				defaultSize = f.FontSize
			}
			if strings.Contains(f.Text, "big") {
				bigSize = f.FontSize
			}
		}
	}
	if defaultSize != 10 {
		t.Errorf("default cell font size = %g, want 10", defaultSize)
	}
	if bigSize != 18 {
		t.Errorf("override cell font size = %g, want 18", bigSize)
	}
}
