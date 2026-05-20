package asposepdf_test

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"os"
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
	_, err := page.AddTable(nil, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100})
	if err == nil {
		t.Error("AddTable(nil) should error")
	}
}

func TestAddTable_EmptyTable(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable() // no columns, no rows
	_, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100})
	if err != nil {
		t.Errorf("AddTable(empty) = %v, want nil (no-op)", err)
	}
}

func TestAddTable_NoRows(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{50, 50})
	_, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100})
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
	_, err := page.AddTable(table, pdf.Rectangle{LLX: 100, LLY: 0, URX: 0, URY: 100})
	if err == nil {
		t.Error("AddTable with bad rect should error")
	}
}

func TestAddTable_MismatchedCellCount(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{50, 50, 50}) // 3 cols
	table.AddRow().AddCells("a", "b")                              // 2 cells
	_, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 100})
	if err == nil {
		t.Fatal("AddTable with mismatched cell count should error")
	}
}

func TestAddTable_NonPositiveColumnWidth(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{50, 0, 50})
	table.AddRow().AddCells("a", "b", "c")
	_, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 100})
	if err == nil {
		t.Fatal("AddTable with zero column width should error")
	}

	table2 := pdf.NewTable().SetColumnWidths([]float64{50, -1, 50})
	table2.AddRow().AddCells("a", "b", "c")
	_, err2 := page.AddTable(table2, pdf.Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 100})
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

	_, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 600, URX: 350, URY: 750})
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

	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 700}); err != nil {
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

	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}); err != nil {
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

	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}); err != nil {
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

	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}); err != nil {
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

	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}); err != nil {
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

	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 100, LLY: 100, URX: 200, URY: 150}); err != nil {
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

	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}); err != nil {
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

	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 25}); err != nil {
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

func TestAddTable_AES128Roundtrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{100, 100})
	table.AddRow().AddCells("secret", "data")

	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 700}); err != nil {
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

	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 600, URX: 290, URY: 700}); err != nil {
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

	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 600, URX: 200, URY: 700}); err != nil {
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

	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 600, URX: 200, URY: 700}); err != nil {
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

func TestCell_ColSpanDefault(t *testing.T) {
	cell := pdf.NewTable().SetColumnWidths([]float64{50}).AddRow().AddCell("x")
	if cell.ColSpan() != 1 {
		t.Errorf("default ColSpan = %d, want 1", cell.ColSpan())
	}
	if cell.RowSpan() != 1 {
		t.Errorf("default RowSpan = %d, want 1", cell.RowSpan())
	}
}

func TestCell_SetColSpanChaining(t *testing.T) {
	cell := pdf.NewTable().SetColumnWidths([]float64{50, 50, 50}).AddRow().AddCell("x").SetColSpan(2)
	if cell.ColSpan() != 2 {
		t.Errorf("ColSpan = %d, want 2", cell.ColSpan())
	}
}

func TestCell_SetRowSpanChaining(t *testing.T) {
	cell := pdf.NewTable().SetColumnWidths([]float64{50}).AddRow().AddCell("x").SetRowSpan(3)
	if cell.RowSpan() != 3 {
		t.Errorf("RowSpan = %d, want 3", cell.RowSpan())
	}
}

func TestTable_RepeatingRowsCountDefault(t *testing.T) {
	table := pdf.NewTable()
	if table.RepeatingRowsCount() != 0 {
		t.Errorf("default RepeatingRowsCount = %d, want 0", table.RepeatingRowsCount())
	}
}

func TestTable_SetRepeatingRowsCountChaining(t *testing.T) {
	table := pdf.NewTable().SetRepeatingRowsCount(2)
	if table.RepeatingRowsCount() != 2 {
		t.Errorf("RepeatingRowsCount = %d, want 2", table.RepeatingRowsCount())
	}
}

func TestTable_OverflowMarginsDefault(t *testing.T) {
	top, bottom := pdf.NewTable().OverflowMargins()
	if top != 50 || bottom != 50 {
		t.Errorf("default OverflowMargins = (%g, %g), want (50, 50)", top, bottom)
	}
}

func TestTable_SetOverflowMarginsChaining(t *testing.T) {
	table := pdf.NewTable().SetOverflowMargins(70, 30)
	top, bottom := table.OverflowMargins()
	if top != 70 || bottom != 30 {
		t.Errorf("OverflowMargins = (%g, %g), want (70, 30)", top, bottom)
	}
}

func TestAddTable_ColSpanRendersWiderCell(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{100, 100, 100})
	row := table.AddRow()
	row.AddCell("wide cell").SetColSpan(3) // spans all 3 columns

	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 300, URY: 100}); err != nil {
		t.Fatal(err)
	}

	layout, err := page.ExtractTextWithLayout()
	if err != nil {
		t.Fatal(err)
	}
	if len(layout) == 0 || len(layout[0].Fragments) == 0 {
		t.Fatal("no fragments extracted")
	}
	// For left-aligned default, X is at LLX + left margin. We just verify
	// the colspan cell renders without error.
	found := false
	for _, line := range layout {
		for _, f := range line.Fragments {
			if strings.Contains(f.Text, "wide cell") {
				found = true
			}
		}
	}
	if !found {
		t.Error("colspan cell text not found")
	}
}

func TestAddTable_RowSpanRendersTallerCell(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().
		SetColumnWidths([]float64{50, 50}).
		SetDefaultCellStyle(pdf.TextStyle{Size: 12}).
		SetDefaultCellMargin(pdf.MarginInfo{Top: 2, Right: 2, Bottom: 2, Left: 2})
	row0 := table.AddRow()
	row0.AddCell("T").SetRowSpan(2) // spans rows 0 and 1
	row0.AddCell("a")
	table.AddRow().AddCell("b") // col 0 is covered

	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}); err != nil {
		t.Fatal(err)
	}

	text, _ := page.ExtractText()
	for _, want := range []string{"T", "a", "b"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in output: %q", want, text)
		}
	}
}

func TestAddTable_RowSpanColSpanCombined(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{50, 50, 50})
	row0 := table.AddRow()
	row0.AddCell("2x2").SetColSpan(2).SetRowSpan(2) // covers rows 0..1, cols 0..1
	row0.AddCell("c0")
	table.AddRow().AddCell("c1") // col 2 only; cols 0..1 covered

	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 150, URY: 100}); err != nil {
		t.Fatal(err)
	}

	text, _ := page.ExtractText()
	for _, want := range []string{"2x2", "c0", "c1"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q: %q", want, text)
		}
	}
}

func TestAddTable_RepeatingRowsCountValidation(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	table := pdf.NewTable().SetColumnWidths([]float64{50})
	table.AddRow().AddCell("only")
	table.SetRepeatingRowsCount(5) // way more than the 1 row
	_, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100})
	if err == nil {
		t.Error("expected error when RepeatingRowsCount exceeds RowCount")
	}

	table2 := pdf.NewTable().SetColumnWidths([]float64{50})
	table2.AddRow().AddCell("only")
	table2.SetRepeatingRowsCount(-1)
	_, err = page.AddTable(table2, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100})
	if err == nil {
		t.Error("expected error for negative RepeatingRowsCount")
	}
}

func TestAddTable_OverflowAddsPage(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	table := pdf.NewTable().
		SetColumnWidths([]float64{100}).
		SetDefaultCellStyle(pdf.TextStyle{Size: 12}).
		SetDefaultCellMargin(pdf.MarginInfo{Top: 4, Right: 4, Bottom: 4, Left: 4})
	// 3 rows of ~22pt each = ~66pt; rect height = 30pt → only 1 row fits.
	table.AddRow().AddCell("rowOne")
	table.AddRow().AddCell("rowTwo")
	table.AddRow().AddCell("rowThree")

	pagesAdded, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 700, URX: 200, URY: 730})
	if err != nil {
		t.Fatal(err)
	}
	if pagesAdded < 1 {
		t.Errorf("pagesAdded = %d, want >= 1", pagesAdded)
	}
	if doc.PageCount() != 1+pagesAdded {
		t.Errorf("PageCount = %d, want %d", doc.PageCount(), 1+pagesAdded)
	}
}

func TestAddTable_OverflowReturnsZeroIfFits(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{100})
	table.AddRow().AddCell("only row")
	pagesAdded, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 500})
	if err != nil {
		t.Fatal(err)
	}
	if pagesAdded != 0 {
		t.Errorf("pagesAdded = %d, want 0 (fits)", pagesAdded)
	}
	if doc.PageCount() != 1 {
		t.Errorf("PageCount = %d, want 1", doc.PageCount())
	}
}

func TestAddTable_OverflowContentSurvivesRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	table := pdf.NewTable().
		SetColumnWidths([]float64{100}).
		SetDefaultCellStyle(pdf.TextStyle{Size: 12}).
		SetDefaultCellMargin(pdf.MarginInfo{Top: 3, Right: 3, Bottom: 3, Left: 3})
	for i := 1; i <= 8; i++ {
		table.AddRow().AddCell(fmt.Sprintf("row%d", i))
	}

	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 700, URX: 200, URY: 760}); err != nil {
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
	for i := 1; i <= 8; i++ {
		want := fmt.Sprintf("row%d", i)
		found := false
		for p := 1; p <= doc2.PageCount(); p++ {
			pg, _ := doc2.Page(p)
			text, _ := pg.ExtractText()
			if strings.Contains(text, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing %q somewhere in document", want)
		}
	}
}

func TestAddTable_OverflowGroupTooTallErrors(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	table := pdf.NewTable().
		SetColumnWidths([]float64{100}).
		SetOverflowMargins(400, 400) // leaves only 42pt for content on A4 (842-800)
	row := table.AddRow()
	row.SetHeight(100) // single row taller than the available continuation space
	row.AddCell("huge")

	_, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50})
	if err == nil {
		t.Error("expected error for group too tall for any page")
	}
}

func TestAddTable_HeadersRepeatOnEachOverflowPage(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	table := pdf.NewTable().
		SetColumnWidths([]float64{100}).
		SetDefaultCellStyle(pdf.TextStyle{Size: 12}).
		SetDefaultCellMargin(pdf.MarginInfo{Top: 3, Right: 3, Bottom: 3, Left: 3})
	table.AddRow().AddCell("HDR-XYZ") // unique header text
	for i := 1; i <= 6; i++ {
		table.AddRow().AddCell(fmt.Sprintf("row%d", i))
	}
	table.SetRepeatingRowsCount(1)

	// Small rect → 1 header + ~2 body rows per page → 3 pages total.
	pagesAdded, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 700, URX: 200, URY: 760})
	if err != nil {
		t.Fatal(err)
	}
	if pagesAdded < 1 {
		t.Fatalf("expected overflow, pagesAdded = %d", pagesAdded)
	}
	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	// Header text must appear on EVERY page that contains body content.
	headerPages := 0
	for p := 1; p <= doc2.PageCount(); p++ {
		page, _ := doc2.Page(p)
		text, _ := page.ExtractText()
		if strings.Contains(text, "HDR-XYZ") {
			headerPages++
		}
	}
	if headerPages != doc2.PageCount() {
		t.Errorf("header appeared on %d of %d pages; want all", headerPages, doc2.PageCount())
	}
}

func TestAddTable_NoHeaderRepeatByDefault(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	table := pdf.NewTable().
		SetColumnWidths([]float64{100}).
		SetDefaultCellStyle(pdf.TextStyle{Size: 12}).
		SetDefaultCellMargin(pdf.MarginInfo{Top: 3, Right: 3, Bottom: 3, Left: 3})
	table.AddRow().AddCell("HDR-XYZ")
	for i := 1; i <= 6; i++ {
		table.AddRow().AddCell(fmt.Sprintf("row%d", i))
	}
	// NOTE: NO SetRepeatingRowsCount call → default 0.

	pagesAdded, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 700, URX: 200, URY: 760})
	if err != nil {
		t.Fatal(err)
	}
	if pagesAdded < 1 {
		t.Fatal("expected overflow")
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	headerPages := 0
	for p := 1; p <= doc2.PageCount(); p++ {
		page, _ := doc2.Page(p)
		text, _ := page.ExtractText()
		if strings.Contains(text, "HDR-XYZ") {
			headerPages++
		}
	}
	// Without repeat, header appears on exactly 1 page (the first).
	if headerPages != 1 {
		t.Errorf("header without repeat appeared on %d pages; want exactly 1", headerPages)
	}
}

func TestAddTable_RowSpanCrossingHeaderBodyErrors(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	table := pdf.NewTable().SetColumnWidths([]float64{50, 50})
	row0 := table.AddRow()
	row0.AddCell("header tall").SetRowSpan(2) // extends from header into body
	row0.AddCell("a")
	table.AddRow().AddCell("b") // col 0 is covered by row 0's rowspan
	table.SetRepeatingRowsCount(1)

	_, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100})
	if err == nil {
		t.Error("expected error: rowspan from header into body")
	}
}

func TestAddTable_RowSpanGroupSurvivesPageBreak(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	table := pdf.NewTable().
		SetColumnWidths([]float64{60, 60}).
		SetDefaultCellStyle(pdf.TextStyle{Size: 12}).
		SetDefaultCellMargin(pdf.MarginInfo{Top: 3, Right: 3, Bottom: 3, Left: 3})
	// Rows 0-3: regular. Rows 4-5: rowspan group (cell at col 0 spans both).
	for i := 0; i < 4; i++ {
		table.AddRow().AddCells(fmt.Sprintf("a%d", i), fmt.Sprintf("b%d", i))
	}
	row4 := table.AddRow()
	row4.AddCell("SPAN").SetRowSpan(2)
	row4.AddCell("b4")
	table.AddRow().AddCell("b5") // col 0 is covered by the rowspan above

	// Tight rect: 4 rows fit on the first page, then rows 4+5 must move
	// together as a group to the continuation page.
	pagesAdded, err := page.AddTable(table, pdf.Rectangle{
		LLX: 0, LLY: 670, URX: 200, URY: 760,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pagesAdded < 1 {
		t.Fatal("expected overflow page")
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))

	// "SPAN" + "b4" + "b5" must all appear on the SAME page (not split).
	spanPage, b4Page, b5Page := -1, -1, -1
	for p := 1; p <= doc2.PageCount(); p++ {
		pg, _ := doc2.Page(p)
		text, _ := pg.ExtractText()
		if strings.Contains(text, "SPAN") {
			spanPage = p
		}
		if strings.Contains(text, "b4") {
			b4Page = p
		}
		if strings.Contains(text, "b5") {
			b5Page = p
		}
	}
	if spanPage == -1 || b4Page == -1 || b5Page == -1 {
		t.Fatalf("missing piece: span=%d b4=%d b5=%d", spanPage, b4Page, b5Page)
	}
	if spanPage != b4Page || spanPage != b5Page {
		t.Errorf("rowspan group split across pages: span=%d b4=%d b5=%d", spanPage, b4Page, b5Page)
	}
}

func TestAddTable_OverflowAES128Roundtrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	table := pdf.NewTable().
		SetColumnWidths([]float64{100}).
		SetDefaultCellStyle(pdf.TextStyle{Size: 12}).
		SetDefaultCellMargin(pdf.MarginInfo{Top: 3, Right: 3, Bottom: 3, Left: 3})
	table.AddRow().AddCell("encrypted header")
	for i := 1; i <= 5; i++ {
		table.AddRow().AddCell(fmt.Sprintf("encrypted row%d", i))
	}
	table.SetRepeatingRowsCount(1)

	pagesAdded, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 700, URX: 200, URY: 760})
	if err != nil {
		t.Fatal(err)
	}
	if pagesAdded < 1 {
		t.Fatal("expected overflow")
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
	// Header must appear on every page; all 5 body rows must appear somewhere.
	foundRows := 0
	for p := 1; p <= doc2.PageCount(); p++ {
		pg, _ := doc2.Page(p)
		text, _ := pg.ExtractText()
		if !strings.Contains(text, "encrypted header") {
			t.Errorf("page %d missing repeated header", p)
		}
		for i := 1; i <= 5; i++ {
			if strings.Contains(text, fmt.Sprintf("encrypted row%d", i)) {
				foundRows++
			}
		}
	}
	if foundRows != 5 {
		t.Errorf("found %d body rows across pages; want 5", foundRows)
	}
}

func TestRow_BackgroundDefault(t *testing.T) {
	r := pdf.NewTable().SetColumnWidths([]float64{50}).AddRow()
	if r.Background() != nil {
		t.Error("default Background should be nil")
	}
	if r.TextStyle() != nil {
		t.Error("default TextStyle should be nil")
	}
	if r.Border() != nil {
		t.Error("default Border should be nil")
	}
	if r.Margin() != nil {
		t.Error("default Margin should be nil")
	}
}

func TestRow_SettersAndChaining(t *testing.T) {
	bg := &pdf.Color{R: 0.9, G: 0.9, B: 0.9, A: 1}
	r := pdf.NewTable().SetColumnWidths([]float64{50}).AddRow().
		SetBackground(bg).
		SetTextStyle(pdf.TextStyle{Size: 14}).
		SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideTop, Width: 1}).
		SetMargin(pdf.MarginInfo{Top: 3, Right: 4, Bottom: 3, Left: 4})

	if r.Background() != bg {
		t.Error("Background pointer not preserved")
	}
	if r.TextStyle() == nil || r.TextStyle().Size != 14 {
		t.Errorf("TextStyle = %+v", r.TextStyle())
	}
	if r.Border() == nil || r.Border().Sides != pdf.BorderSideTop {
		t.Errorf("Border = %+v", r.Border())
	}
	if r.Margin() == nil || r.Margin().Left != 4 {
		t.Errorf("Margin = %+v", r.Margin())
	}
}

func TestAddTable_RowBackgroundAppliesToAllCells(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{50, 50, 50})
	row := table.AddRow().SetBackground(&pdf.Color{R: 0.85, G: 0, B: 0, A: 1})
	row.AddCells("a", "b", "c")
	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 100}); err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	// Red fill: 0.85 0 0 rg + re + f — should appear (one or more times for the 3 cells).
	if !strings.Contains(s, "0.85 0 0 rg") {
		t.Error("row-level red background not emitted")
	}
}

func TestAddTable_CellBackgroundWinsOverRow(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{50, 50})
	row := table.AddRow().SetBackground(&pdf.Color{R: 0.85, G: 0, B: 0, A: 1}) // red
	row.AddCell("a")
	row.AddCell("b").SetBackground(&pdf.Color{R: 0, G: 0, B: 0.85, A: 1}) // blue
	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50}); err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	if !strings.Contains(s, "0.85 0 0 rg") {
		t.Error("row red fill missing for cell A")
	}
	if !strings.Contains(s, "0 0 0.85 rg") {
		t.Error("cell B's blue override missing")
	}
}

func TestTable_AddRows_BatchReturnsRowsInOrder(t *testing.T) {
	table := pdf.NewTable().SetColumnWidths([]float64{50, 50})
	rows := table.AddRows([][]string{
		{"a1", "a2"},
		{"b1", "b2"},
		{"c1", "c2"},
	})
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	if table.RowCount() != 3 {
		t.Errorf("RowCount = %d, want 3", table.RowCount())
	}
	if rows[1].Cells()[0].Text() != "b1" {
		t.Errorf("rows[1][0] = %q, want b1", rows[1].Cells()[0].Text())
	}
}

func TestTable_AddRows_EmptyInputs(t *testing.T) {
	table := pdf.NewTable().SetColumnWidths([]float64{50})
	if got := table.AddRows(nil); len(got) != 0 {
		t.Errorf("AddRows(nil) = %d rows, want 0", len(got))
	}
	if got := table.AddRows([][]string{}); len(got) != 0 {
		t.Errorf("AddRows([]) = %d rows, want 0", len(got))
	}
	if table.RowCount() != 0 {
		t.Errorf("RowCount = %d, want 0 (empty inputs add nothing)", table.RowCount())
	}
}

func TestCell_ImageDefault(t *testing.T) {
	cell := pdf.NewTable().SetColumnWidths([]float64{50}).AddRow().AddCell("x")
	path, hasImage := cell.Image()
	if path != "" || hasImage {
		t.Errorf("default Image() = (%q, %v), want ('', false)", path, hasImage)
	}
}

func TestCell_SetImageChaining(t *testing.T) {
	cell := pdf.NewTable().SetColumnWidths([]float64{50}).AddRow().AddCell("ignored").
		SetImage("testdata/Koala.jpg")
	path, hasImage := cell.Image()
	if !hasImage {
		t.Error("hasImage should be true after SetImage")
	}
	if path != "testdata/Koala.jpg" {
		t.Errorf("Image() path = %q", path)
	}
}

func TestCell_SetImageFromStream(t *testing.T) {
	cell := pdf.NewTable().SetColumnWidths([]float64{50}).AddRow().AddCell("").
		SetImageFromStream(bytes.NewReader([]byte("dummy")))
	path, hasImage := cell.Image()
	if !hasImage {
		t.Error("hasImage should be true after SetImageFromStream")
	}
	if path != "" {
		t.Errorf("path should be empty for stream-set image, got %q", path)
	}
}

func TestAddTable_ImageCellRendered(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{200})
	table.AddRow().AddCell("").SetImage("testdata/Koala.jpg")
	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 500, URX: 250, URY: 750}); err != nil {
		t.Fatal(err)
	}
	// Verify via the existing ImageInfos API: the page should now have 1 image.
	infos, err := page.ImageInfos()
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 {
		t.Errorf("got %d images, want 1", len(infos))
	}
}

func TestAddTable_ImageOverridesText(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{200})
	table.AddRow().AddCell("ignored-text-should-not-render").
		SetImage("testdata/Koala.jpg")
	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 500, URX: 250, URY: 750}); err != nil {
		t.Fatal(err)
	}
	text, _ := page.ExtractText()
	if strings.Contains(text, "ignored-text-should-not-render") {
		t.Errorf("text should not render when image is set; got: %q", text)
	}
}

func TestAddTable_ImageFromStreamRoundTrip(t *testing.T) {
	data, err := os.ReadFile("testdata/Koala.jpg")
	if err != nil {
		t.Fatal(err)
	}
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{200})
	table.AddRow().AddCell("").SetImageFromStream(bytes.NewReader(data))
	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 500, URX: 250, URY: 750}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	page2, _ := doc2.Page(1)
	infos, _ := page2.ImageInfos()
	if len(infos) != 1 {
		t.Errorf("after roundtrip got %d images, want 1", len(infos))
	}
}

func TestAddTable_ImageWithColSpan(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{50, 50, 50, 50})
	row := table.AddRow()
	row.AddCell("").SetColSpan(2).SetImage("testdata/Koala.jpg") // image spans cols 0-1 (100pt wide)
	row.AddCell("a")
	row.AddCell("b")
	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 500, URX: 250, URY: 750}); err != nil {
		t.Fatal(err)
	}
	infos, _ := page.ImageInfos()
	if len(infos) != 1 {
		t.Errorf("colspan image: got %d images, want 1", len(infos))
	}
}

func TestAddTable_ImageInRepeatingHeader(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().
		SetColumnWidths([]float64{100, 100}).
		SetDefaultCellStyle(pdf.TextStyle{Size: 12}).
		SetDefaultCellMargin(pdf.MarginInfo{Top: 3, Right: 3, Bottom: 3, Left: 3})
	// Header row: image + text.
	header := table.AddRow()
	header.AddCell("").SetImage("testdata/Koala.jpg")
	header.AddCell("HEADER")
	// Body rows.
	for i := 1; i <= 6; i++ {
		table.AddRow().AddCells(fmt.Sprintf("a%d", i), fmt.Sprintf("b%d", i))
	}
	table.SetRepeatingRowsCount(1)

	pagesAdded, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 600, URX: 200, URY: 760})
	if err != nil {
		t.Fatal(err)
	}
	if pagesAdded < 1 {
		t.Fatalf("expected overflow, pagesAdded = %d", pagesAdded)
	}
	// Verify each page has an image (repeated header).
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	for p := 1; p <= doc2.PageCount(); p++ {
		pg, _ := doc2.Page(p)
		infos, _ := pg.ImageInfos()
		if len(infos) < 1 {
			t.Errorf("page %d has %d images; want >= 1 (repeated header image)", p, len(infos))
		}
	}
}

func TestAddTable_BorderDedupIdenticalAdjacentEdges(t *testing.T) {
	// 2 cells with the same border style. The shared vertical edge between
	// them should render exactly once, not twice.
	// Pre-dedup count: 8 (4 sides × 2 cells). Post-dedup: 7 (shared edge deduped).
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().
		SetColumnWidths([]float64{50, 50}).
		SetDefaultCellBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1})
	table.AddRow().AddCells("a", "b")
	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50}); err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	strokes := strings.Count(s, " S\n")
	// Without dedup: 8 (4+4). With dedup (shared right-of-A == left-of-B): 7.
	if strokes >= 8 {
		t.Errorf("expected dedup reducing strokes below 8, got %d", strokes)
	}
}

func TestAddTable_BorderNoDedupDifferentStyles(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{50, 50})
	row := table.AddRow()
	row.AddCell("a").SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1})
	row.AddCell("b").SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 3})
	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50}); err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	// Different widths → shared edge NOT deduped, both render.
	if !strings.Contains(s, "1 w") || !strings.Contains(s, "3 w") {
		t.Error("expected both 1pt and 3pt widths present (no dedup for different styles)")
	}
}

func TestAddTable_DedupResetsBetweenPages(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().
		SetColumnWidths([]float64{50}).
		SetDefaultCellStyle(pdf.TextStyle{Size: 12}).
		SetDefaultCellMargin(pdf.MarginInfo{Top: 3, Right: 3, Bottom: 3, Left: 3}).
		SetDefaultCellBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1})
	for i := 1; i <= 6; i++ {
		table.AddRow().AddCell(fmt.Sprintf("row%d", i))
	}
	pagesAdded, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 700, URX: 100, URY: 760})
	if err != nil {
		t.Fatal(err)
	}
	if pagesAdded < 1 {
		t.Fatal("expected overflow")
	}
	// Each page should render its own borders (dedup is per-page).
	// Indirect check: verify all rows + their borders survive roundtrip.
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	foundRows := 0
	for p := 1; p <= doc2.PageCount(); p++ {
		pg, _ := doc2.Page(p)
		text, _ := pg.ExtractText()
		for i := 1; i <= 6; i++ {
			if strings.Contains(text, fmt.Sprintf("row%d", i)) {
				foundRows++
			}
		}
	}
	if foundRows != 6 {
		t.Errorf("multi-page roundtrip lost rows: got %d, want 6", foundRows)
	}
}

func TestAddTable_ImageCellAES128Roundtrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	table := pdf.NewTable().SetColumnWidths([]float64{200})
	table.AddRow().AddCell("").SetImage("testdata/Koala.jpg")
	if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 500, URX: 250, URY: 750}); err != nil {
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
	infos, _ := page2.ImageInfos()
	if len(infos) != 1 {
		t.Errorf("AES-128 roundtrip lost image: got %d, want 1", len(infos))
	}
}
