// SPDX-License-Identifier: MIT

package asposepdf

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestParseCellNumber(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	cases := []struct {
		in     string
		want   *float64
		numFmt string
	}{
		{"1234", f(1234), ""},
		{"1234.56", f(1234.56), "#,##0.00"},
		{"1 234,56", f(1234.56), "#,##0.00"},
		{"1,234.56", f(1234.56), "#,##0.00"},
		{"-7", f(-7), ""},
		{"+42", f(42), ""},
		{"42%", f(0.42), "0%"},
		{"12.50%", f(0.125), "0.00%"},
		{"€17.00", f(17), `#,##0.00\ "€"`},
		{"17.00 €", f(17), `#,##0.00\ "€"`},
		{"$1,240.00", f(1240), `"$"#,##0.00`},
		{"1.234", f(1234), ""}, // trailing group of three = thousands
		{"", nil, ""},
		{"Item", nil, ""},
		{"3 apples", nil, ""},
		{"1,23,456", nil, ""}, // malformed grouping
		{"12.3456", nil, ""},  // 4 decimals: ambiguous, stays text
		{"v1.2", nil, ""},
		{"2026-05-19", nil, ""},
	}
	for _, c := range cases {
		got, numFmt := parseCellNumber(c.in)
		switch {
		case c.want == nil && got != nil:
			t.Errorf("%q: parsed %v, want text", c.in, *got)
		case c.want != nil && got == nil:
			t.Errorf("%q: stayed text, want %v", c.in, *c.want)
		case c.want != nil && (*got < *c.want-1e-9 || *got > *c.want+1e-9):
			t.Errorf("%q: got %v, want %v", c.in, *got, *c.want)
		case got != nil && numFmt != c.numFmt:
			t.Errorf("%q: numFmt %q, want %q", c.in, numFmt, c.numFmt)
		}
	}
}

func xlsxParts(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		buf := make([]byte, 64*1024)
		for {
			n, err := rc.Read(buf)
			b.Write(buf[:n])
			if err != nil {
				break
			}
		}
		rc.Close()
		out[f.Name] = b.String()
	}
	return out
}

// A drawn table round-trips into a workbook: numeric cells, merges, header
// fill, and the package invariants.
func TestWriteXlsxTablesOnly(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	p, _ := doc.Page(1)
	navy := Color{R: 0.1, G: 0.15, B: 0.4, A: 1}
	tbl := NewTable().
		SetColumnWidths([]float64{140, 80, 100}).
		SetBorder(BorderInfo{Sides: BorderSideAll, Width: 1}).
		SetDefaultCellBorder(BorderInfo{Sides: BorderSideAll, Width: 1})
	hdr := tbl.AddRow()
	for _, s := range []string{"Item", "Qty", "Price"} {
		hdr.AddCell(s).SetBackground(&navy).SetTextStyle(TextStyle{Font: FontHelveticaBold, Size: 10, Color: &Color{R: 1, G: 1, B: 1, A: 1}})
	}
	tbl.AddRow().AddCells("Apples", "3", "€4.50")
	tbl.AddRow().AddCells("Pears", "2", "€5.25")
	sum := tbl.AddRow()
	sum.AddCell("Total").SetColSpan(2)
	sum.AddCell("€9.75")
	if _, err := p.AddTable(tbl, Rectangle{LLX: 60, LLY: 500, URX: 420, URY: 700}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := doc.WriteXlsx(&buf); err != nil {
		t.Fatal(err)
	}
	parts := xlsxParts(t, buf.Bytes())
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "xl/workbook.xml",
		"xl/_rels/workbook.xml.rels", "xl/worksheets/sheet1.xml", "xl/styles.xml"} {
		if parts[name] == "" {
			t.Fatalf("missing part %s", name)
		}
	}
	sheet := parts["xl/worksheets/sheet1.xml"]
	for _, want := range []string{
		"<is><t>Item</t></is>", "<is><t>Apples</t></is>",
		"<v>3</v>", "<v>4.5</v>", "<v>9.75</v>",
		"<mergeCells", // the Total colspan
	} {
		if !strings.Contains(sheet, want) {
			t.Errorf("sheet1 missing %q", want)
		}
	}
	if !strings.Contains(parts["xl/styles.xml"], "formatCode") {
		t.Error("styles missing the currency numFmt")
	}
	if !strings.Contains(parts["xl/styles.xml"], "patternType=\"solid\"") {
		t.Error("styles missing the header fill")
	}
}

// FullPage mode: plain page text lands in cells, one sheet per page.
func TestWriteXlsxFullPage(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	p, _ := doc.Page(1)
	style := TextStyle{Font: FontHelvetica, Size: 11}
	mustT := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	mustT(p.AddText("Report Title", TextStyle{Font: FontHelveticaBold, Size: 16}, Rectangle{LLX: 60, LLY: 740, URX: 400, URY: 770}))
	mustT(p.AddText("Revenue", style, Rectangle{LLX: 60, LLY: 700, URX: 200, URY: 715}))
	mustT(p.AddText("1200", style, Rectangle{LLX: 300, LLY: 700, URX: 400, URY: 715}))
	mustT(doc.AddBlankPageFromFormat(PageFormatA4))
	p2, _ := doc.Page(2)
	mustT(p2.AddText("Second page", style, Rectangle{LLX: 60, LLY: 700, URX: 300, URY: 715}))

	var buf bytes.Buffer
	if err := doc.WriteXlsx(&buf, XlsxSaveOptions{Mode: XlsxFullPage}); err != nil {
		t.Fatal(err)
	}
	parts := xlsxParts(t, buf.Bytes())
	if parts["xl/worksheets/sheet2.xml"] == "" {
		t.Fatal("expected two sheets")
	}
	s1 := parts["xl/worksheets/sheet1.xml"]
	if !strings.Contains(s1, "Report Title") || !strings.Contains(s1, "<v>1200</v>") {
		t.Errorf("page 1 content missing: %.200s", s1)
	}
	if !strings.Contains(parts["xl/worksheets/sheet2.xml"], "Second page") {
		t.Error("page 2 content missing")
	}
	if !strings.Contains(parts["xl/workbook.xml"], `name="Page 1"`) {
		t.Error("sheet naming")
	}
}

// An empty document still yields a valid single-sheet workbook.
func TestWriteXlsxEmpty(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	var buf bytes.Buffer
	if err := doc.WriteXlsx(&buf); err != nil {
		t.Fatal(err)
	}
	parts := xlsxParts(t, buf.Bytes())
	if !strings.Contains(parts["xl/workbook.xml"], "<sheet ") {
		t.Error("workbook must carry at least one sheet")
	}
}
