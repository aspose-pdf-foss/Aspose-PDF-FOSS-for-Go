// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestMarkdownExportGFMTable(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	tbl := pdf.NewTable().
		SetColumnWidths([]float64{140, 100, 100}).
		SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1}).
		SetDefaultCellBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1})
	tbl.AddRow().AddCells("Item", "Qty", "Price")
	tbl.AddRow().AddCells("Apples", "3", "4.50")
	tbl.AddRow().AddCells("Pears | raw", "7", "9.10")
	if _, err := p.AddTable(tbl, pdf.Rectangle{LLX: 60, LLY: 500, URX: 400, URY: 700}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := doc.WriteMarkdown(&buf, pdf.MarkdownSaveOptions{NoImages: true}); err != nil {
		t.Fatal(err)
	}
	md := buf.String()
	if !strings.Contains(md, "| Item | Qty | Price |") {
		t.Errorf("header row missing:\n%s", md)
	}
	if !strings.Contains(md, "| --- |") {
		t.Errorf("separator row missing:\n%s", md)
	}
	if !strings.Contains(md, "| Apples | 3 | 4.50 |") {
		t.Errorf("data row missing:\n%s", md)
	}
	if !strings.Contains(md, `Pears \| raw`) {
		t.Errorf("pipe not escaped in cell:\n%s", md)
	}
	// Table text must not ALSO flow as paragraphs.
	if strings.Contains(strings.ReplaceAll(md, "| Apples", ""), "\nApples") {
		t.Errorf("table text duplicated as paragraphs:\n%s", md)
	}
}
