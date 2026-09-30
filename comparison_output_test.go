// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func sampleDiffOps() []pdf.DiffOperation {
	return []pdf.DiffOperation{
		{Operation: pdf.OperationEqual, Text: "hello", SourcePage: 1, DestPage: 1},
		{Operation: pdf.OperationInsert, Text: "world", DestPage: 1,
			DestRects: []pdf.Rectangle{{LLX: 1, LLY: 2, URX: 3, URY: 4}}},
		{Operation: pdf.OperationDelete, Text: "old", SourcePage: 1,
			SourceRects: []pdf.Rectangle{{LLX: 5, LLY: 6, URX: 7, URY: 8}}},
	}
}

// --- HTML ---

func TestHTMLDiffOutputGeneratorGenerateOutput(t *testing.T) {
	gen := pdf.NewHTMLDiffOutputGenerator()
	out, err := gen.GenerateOutput(sampleDiffOps())
	if err != nil {
		t.Fatal(err)
	}
	// Equal text is unwrapped by default (no EqualColor/EqualBackground set),
	// so it appears as plain text, not inside a <span>.
	if !strings.Contains(out, "hello") || strings.Contains(out, "<span") && strings.Contains(out, ">hello<") {
		t.Errorf("output = %q, want plain (unwrapped) equal text %q", out, "hello")
	}
	if !strings.Contains(out, `color:rgb(51,184,89)`) || !strings.Contains(out, ">world<") {
		t.Errorf("output = %q, missing a green span wrapping the insertion", out)
	}
	if !strings.Contains(out, `color:rgb(224,56,56)`) || !strings.Contains(out, ">old<") {
		t.Errorf("output = %q, missing a red span wrapping the deletion", out)
	}
	if strings.Contains(out, "line-through") {
		t.Errorf("output = %q, should not strike through deletions by default", out)
	}
	if strings.Contains(out, "Page ") {
		t.Errorf("output = %q, single-list GenerateOutput should not add page headings", out)
	}
}

func TestHTMLDiffOutputGeneratorStrikethroughDelete(t *testing.T) {
	gen := pdf.NewHTMLDiffOutputGenerator(pdf.DiffOutputStyle{StrikethroughDelete: true})
	out, err := gen.GenerateOutput(sampleDiffOps())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "text-decoration:line-through") {
		t.Errorf("output = %q, want line-through on the deletion", out)
	}
	insStart := strings.Index(out, ">world<")
	lineThrough := strings.Index(out, "line-through")
	if insStart >= 0 && lineThrough >= 0 && lineThrough < insStart {
		t.Error("line-through style leaked onto the insertion span")
	}
}

func TestHTMLDiffOutputGeneratorGenerateOutputPages(t *testing.T) {
	gen := pdf.NewHTMLDiffOutputGenerator()
	pages := [][]pdf.DiffOperation{
		{{Operation: pdf.OperationEqual, Text: "one"}},
		{{Operation: pdf.OperationInsert, Text: "two"}},
	}
	out, err := gen.GenerateOutputPages(pages)
	if err != nil {
		t.Fatal(err)
	}
	p1 := strings.Index(out, "Page 1")
	p2 := strings.Index(out, "Page 2")
	if p1 < 0 || p2 < 0 || p2 < p1 {
		t.Fatalf("output = %q, want ordered Page 1 / Page 2 headings", out)
	}
	if strings.Index(out, "one") > p2 || strings.Index(out, "two") < p2 {
		t.Errorf("output = %q, page content not grouped under the right heading", out)
	}
}

func TestHTMLDiffOutputGeneratorEscapesHTML(t *testing.T) {
	gen := pdf.NewHTMLDiffOutputGenerator()
	out, err := gen.GenerateOutput([]pdf.DiffOperation{{Operation: pdf.OperationEqual, Text: "a < b & c"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "a < b") || !strings.Contains(out, "&lt;") || !strings.Contains(out, "&amp;") {
		t.Errorf("output = %q, want HTML-escaped text", out)
	}
}

func TestHTMLDiffOutputGeneratorWriteAndSaveOutput(t *testing.T) {
	gen := pdf.NewHTMLDiffOutputGenerator()
	ops := sampleDiffOps()
	want, err := gen.GenerateOutput(ops)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := gen.WriteOutput(&buf, ops); err != nil {
		t.Fatal(err)
	}
	if buf.String() != want {
		t.Errorf("WriteOutput = %q, want %q", buf.String(), want)
	}

	path := tmpOut(t, "diff.html")
	if err := gen.SaveOutput(path, ops); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Errorf("SaveOutput file = %q, want %q", string(data), want)
	}
}

// --- JSON ---

func TestJSONDiffOutputGeneratorGenerateOutput(t *testing.T) {
	gen := pdf.NewJSONDiffOutputGenerator()
	out, err := gen.GenerateOutput(sampleDiffOps())
	if err != nil {
		t.Fatal(err)
	}
	var decoded []struct {
		Operation   string `json:"operation"`
		Text        string `json:"text"`
		SourcePage  int    `json:"sourcePage"`
		DestPage    int    `json:"destPage"`
		SourceRects []struct {
			LLX, LLY, URX, URY float64
		} `json:"sourceRects"`
		DestRects []struct {
			LLX, LLY, URX, URY float64
		} `json:"destRects"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(decoded) != 3 {
		t.Fatalf("decoded %d operations, want 3", len(decoded))
	}
	if decoded[0].Operation != "equal" || decoded[0].Text != "hello" {
		t.Errorf("decoded[0] = %+v, want equal/hello", decoded[0])
	}
	if decoded[1].Operation != "insert" || decoded[1].Text != "world" || decoded[1].DestPage != 1 {
		t.Errorf("decoded[1] = %+v, want insert/world/destPage=1", decoded[1])
	}
	if len(decoded[1].DestRects) != 1 || decoded[1].DestRects[0].URX != 3 {
		t.Errorf("decoded[1].DestRects = %+v, want the insertion's rect", decoded[1].DestRects)
	}
	if decoded[2].Operation != "delete" || decoded[2].Text != "old" || decoded[2].SourcePage != 1 {
		t.Errorf("decoded[2] = %+v, want delete/old/sourcePage=1", decoded[2])
	}
}

func TestJSONDiffOutputGeneratorGenerateOutputPages(t *testing.T) {
	gen := pdf.NewJSONDiffOutputGenerator()
	pages := [][]pdf.DiffOperation{
		{{Operation: pdf.OperationEqual, Text: "one"}},
		{{Operation: pdf.OperationInsert, Text: "two"}},
	}
	out, err := gen.GenerateOutputPages(pages)
	if err != nil {
		t.Fatal(err)
	}
	var decoded []struct {
		Page       int `json:"page"`
		Operations []struct {
			Operation string `json:"operation"`
			Text      string `json:"text"`
		} `json:"operations"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(decoded) != 2 || decoded[0].Page != 1 || decoded[1].Page != 2 {
		t.Fatalf("decoded pages = %+v, want page 1 then page 2", decoded)
	}
	if len(decoded[0].Operations) != 1 || decoded[0].Operations[0].Text != "one" {
		t.Errorf("page 1 operations = %+v, want one op carrying %q", decoded[0].Operations, "one")
	}
	if len(decoded[1].Operations) != 1 || decoded[1].Operations[0].Text != "two" {
		t.Errorf("page 2 operations = %+v, want one op carrying %q", decoded[1].Operations, "two")
	}
}

// --- Markdown ---

func TestMarkdownDiffOutputGeneratorGenerateOutput(t *testing.T) {
	gen := pdf.NewMarkdownDiffOutputGenerator()
	out, err := gen.GenerateOutput(sampleDiffOps())
	if err != nil {
		t.Fatal(err)
	}
	want := "hello **world** ~~old~~"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestMarkdownDiffOutputGeneratorEscapesSpecialCharacters(t *testing.T) {
	gen := pdf.NewMarkdownDiffOutputGenerator()
	out, err := gen.GenerateOutput([]pdf.DiffOperation{{Operation: pdf.OperationEqual, Text: "a*b_c"}})
	if err != nil {
		t.Fatal(err)
	}
	if out != `a\*b\_c` {
		t.Errorf("output = %q, want escaped markdown specials", out)
	}
}

func TestMarkdownDiffOutputGeneratorGenerateOutputPages(t *testing.T) {
	gen := pdf.NewMarkdownDiffOutputGenerator()
	pages := [][]pdf.DiffOperation{
		{{Operation: pdf.OperationEqual, Text: "one"}},
		{{Operation: pdf.OperationInsert, Text: "two"}},
	}
	out, err := gen.GenerateOutputPages(pages)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "## Page 1") || !strings.Contains(out, "## Page 2") {
		t.Fatalf("output = %q, want Page 1 / Page 2 headings", out)
	}
	if strings.Index(out, "one") > strings.Index(out, "## Page 2") {
		t.Errorf("output = %q, page 1 content should precede the page 2 heading", out)
	}
	if !strings.Contains(out, "**two**") {
		t.Errorf("output = %q, want the insertion bolded", out)
	}
}

// --- ComparisonResult.Pages ---

func TestComparisonResultPagesFlatMode(t *testing.T) {
	d1 := buildComparisonDoc(t, "alpha", "gamma")
	d2 := buildComparisonDoc(t, "alpha", "delta")
	res, err := pdf.CompareFlatDocuments(d1, d2)
	if err != nil {
		t.Fatal(err)
	}
	pages := res.Pages()
	if len(pages) == 0 {
		t.Fatal("Pages() returned nothing for a flat comparison with changes")
	}
	// Every operation from Operations() must show up under the page
	// PageOperations would place it under.
	for _, op := range res.Operations() {
		found := false
		for _, pageOps := range pages {
			for _, po := range pageOps {
				if po.Operation == op.Operation && po.Text == op.Text &&
					po.SourcePage == op.SourcePage && po.DestPage == op.DestPage {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("operation %+v missing from Pages()", op)
		}
	}
}

func TestComparisonResultPagesPageByPageMode(t *testing.T) {
	d1 := buildComparisonDoc(t, "alpha", "gamma")
	d2 := buildComparisonDoc(t, "alpha", "delta")
	res, err := pdf.CompareDocumentsPageByPage(d1, d2)
	if err != nil {
		t.Fatal(err)
	}
	pages := res.Pages()
	if len(pages) != 2 {
		t.Fatalf("Pages() = %d pages, want 2", len(pages))
	}
	for i, pageOps := range pages {
		want := res.PageOperations(i + 1)
		if len(pageOps) != len(want) {
			t.Errorf("Pages()[%d] has %d ops, PageOperations(%d) has %d", i, len(pageOps), i+1, len(want))
		}
	}
}

// --- PDF report ---

// TestPDFDiffOutputGeneratorRendersColors renders the generated report and
// checks for the actual insert/delete colors in the pixels, the same style of
// check comparison_sidebyside_test.go uses — a word-presence check alone would
// pass even if diffRuns forgot to set TextStyle.Color, since AddText draws the
// glyphs either way.
func TestPDFDiffOutputGeneratorRendersColors(t *testing.T) {
	ops := []pdf.DiffOperation{
		{Operation: pdf.OperationInsert, Text: "insertedword"},
		{Operation: pdf.OperationDelete, Text: "deletedword"},
	}
	gen := pdf.NewPDFDiffOutputGenerator()
	var buf bytes.Buffer
	if err := gen.WriteOutput(&buf, ops); err != nil {
		t.Fatal(err)
	}
	doc, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	img, err := page.RenderImage(pdf.RenderOptions{DPI: 150})
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	green := [3]int{51, 184, 89}
	red := [3]int{224, 56, 56}
	hasColor := func(want [3]int) bool {
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, bl, _ := img.At(x, y).RGBA()
				rr, gg, bb := int(r>>8), int(g>>8), int(bl>>8)
				if sbsAbsDiff(rr, want[0]) <= 20 && sbsAbsDiff(gg, want[1]) <= 20 && sbsAbsDiff(bb, want[2]) <= 20 {
					return true
				}
			}
		}
		return false
	}
	if !hasColor(green) {
		t.Error("no insert-green pixel found in the rendered report")
	}
	if !hasColor(red) {
		t.Error("no delete-red pixel found in the rendered report")
	}
}

func TestPDFDiffOutputGeneratorGenerateOutputPages(t *testing.T) {
	pages := [][]pdf.DiffOperation{
		{{Operation: pdf.OperationEqual, Text: "one"}},
		{{Operation: pdf.OperationInsert, Text: "two"}},
	}
	gen := pdf.NewPDFDiffOutputGenerator()
	var buf bytes.Buffer
	if err := gen.WriteOutputPages(&buf, pages); err != nil {
		t.Fatal(err)
	}
	doc, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	text, err := doc.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	full := strings.Join(text, "\n")
	if !strings.Contains(full, "Page 1") || !strings.Contains(full, "Page 2") {
		t.Errorf("report text = %q, want Page 1 / Page 2 headings", full)
	}
	if !strings.Contains(full, "one") || !strings.Contains(full, "two") {
		t.Errorf("report text = %q, missing operation text", full)
	}
}

func TestPDFDiffOutputGeneratorEmptyPageOperations(t *testing.T) {
	pages := [][]pdf.DiffOperation{
		{{Operation: pdf.OperationEqual, Text: "one"}},
		{}, // a page with no differences at all
	}
	gen := pdf.NewPDFDiffOutputGenerator()
	var buf bytes.Buffer
	if err := gen.WriteOutputPages(&buf, pages); err != nil {
		t.Fatalf("WriteOutputPages with an empty page's ops: %v", err)
	}
}

func TestPDFDiffReportOptionsTitleAndFormat(t *testing.T) {
	gen := pdf.NewPDFDiffOutputGenerator(pdf.PDFDiffReportOptions{
		Title:  "My Report",
		Format: pdf.PageFormatLetter,
	})
	var buf bytes.Buffer
	if err := gen.WriteOutput(&buf, []pdf.DiffOperation{{Operation: pdf.OperationEqual, Text: "hi"}}); err != nil {
		t.Fatal(err)
	}
	doc, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	text, err := doc.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(text, "\n"), "My Report") {
		t.Errorf("report text = %q, missing custom title", text)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	size, err := page.Size()
	if err != nil {
		t.Fatal(err)
	}
	if size.Width != pdf.PageFormatLetter.Width || size.Height != pdf.PageFormatLetter.Height {
		t.Errorf("page size = %+v, want Letter format", size)
	}
}
