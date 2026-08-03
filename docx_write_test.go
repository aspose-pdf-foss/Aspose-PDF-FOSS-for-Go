// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// docxParts opens the generated package and returns part name → bytes.
func docxParts(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("output is not a zip: %v", err)
	}
	parts := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		parts[f.Name] = b
	}
	return parts
}

// assertWellFormed runs every XML part through the decoder.
func assertWellFormed(t *testing.T, parts map[string][]byte) {
	t.Helper()
	for name, data := range parts {
		if !strings.HasSuffix(name, ".xml") && !strings.HasSuffix(name, ".rels") {
			continue
		}
		dec := xml.NewDecoder(bytes.NewReader(data))
		for {
			_, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("%s: malformed XML: %v", name, err)
				break
			}
		}
	}
}

// docxText concatenates all w:t contents of document.xml.
func docxText(t *testing.T, parts map[string][]byte) string {
	t.Helper()
	doc := parts["word/document.xml"]
	if doc == nil {
		t.Fatal("word/document.xml missing")
	}
	var b strings.Builder
	dec := xml.NewDecoder(bytes.NewReader(doc))
	inT := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("document.xml: %v", err)
		}
		switch el := tok.(type) {
		case xml.StartElement:
			if el.Name.Local == "t" {
				inT = true
			}
		case xml.EndElement:
			if el.Name.Local == "t" {
				inT = false
				b.WriteString("\n")
			}
		case xml.CharData:
			if inT {
				b.Write(el)
			}
		}
	}
	return b.String()
}

// buildDocxSource builds a PDF exercising headings, styled runs, lists,
// links and an image.
func buildDocxSource(t *testing.T) *pdf.Document {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	add := func(text string, style pdf.TextStyle, y float64) {
		t.Helper()
		if err := p.AddText(text, style, pdf.Rectangle{LLX: 60, LLY: y, URX: 535, URY: y + 40}); err != nil {
			t.Fatal(err)
		}
	}
	add("Annual Report", pdf.TextStyle{Size: 24, Font: pdf.FontHelveticaBold}, 760)
	add("The quarterly revenue grew by twelve percent.", pdf.TextStyle{Size: 12}, 700)
	add("Key highlights", pdf.TextStyle{Size: 17, Font: pdf.FontHelveticaBold}, 650)
	add("• Revenue up", pdf.TextStyle{Size: 12}, 610)
	add("• Costs down", pdf.TextStyle{Size: 12}, 580)
	add("1. First step", pdf.TextStyle{Size: 12}, 540)
	add("2. Second step", pdf.TextStyle{Size: 12}, 510)
	add("code_sample()", pdf.TextStyle{Size: 11, Font: pdf.FontCourier}, 460)
	add("visit example", pdf.TextStyle{Size: 12, Color: &pdf.Color{B: 0.8, A: 1}}, 420)
	link := pdf.NewLinkAnnotation(p, pdf.Rectangle{LLX: 55, LLY: 400, URX: 300, URY: 465})
	link.SetAction(pdf.NewGoToURIAction("https://example.com/"))
	if err := p.Annotations().Add(link); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestWriteDocxPackageInvariants(t *testing.T) {
	doc := buildDocxSource(t)
	var buf bytes.Buffer
	if err := doc.WriteDocx(&buf); err != nil {
		t.Fatalf("WriteDocx: %v", err)
	}
	parts := docxParts(t, buf.Bytes())

	for _, name := range []string{"[Content_Types].xml", "_rels/.rels",
		"word/document.xml", "word/_rels/document.xml.rels",
		"word/styles.xml", "word/numbering.xml"} {
		if parts[name] == nil {
			t.Errorf("required part %s missing", name)
		}
	}
	assertWellFormed(t, parts)

	// Every part must be covered by [Content_Types].xml.
	ct := string(parts["[Content_Types].xml"])
	for name := range parts {
		if name == "[Content_Types].xml" {
			continue
		}
		if strings.Contains(ct, `PartName="/`+name+`"`) {
			continue
		}
		dot := strings.LastIndex(name, ".")
		if dot < 0 || !strings.Contains(ct, `Extension="`+name[dot+1:]+`"`) {
			t.Errorf("part %s not covered by content types", name)
		}
	}

	docXML := string(parts["word/document.xml"])
	rels := string(parts["word/_rels/document.xml.rels"])

	// Every r:id / r:embed reference resolves in the rels part.
	for _, m := range regexp.MustCompile(`r:(?:id|embed)="([^"]+)"`).FindAllStringSubmatch(docXML, -1) {
		if !strings.Contains(rels, `Id="`+m[1]+`"`) {
			t.Errorf("unresolved relationship %s", m[1])
		}
	}
	// The hyperlink relationship must be external.
	if !regexp.MustCompile(`Target="https://example.com/"[^/]*TargetMode="External"`).MatchString(rels) {
		t.Errorf("hyperlink relationship missing or not external: %s", rels)
	}
	// sectPr is the last child of w:body.
	if !regexp.MustCompile(`</w:sectPr></w:body></w:document>$`).MatchString(strings.TrimSpace(docXML)) {
		t.Error("w:sectPr is not the last element of w:body")
	}
	// Every numId resolves in numbering.xml.
	numbering := string(parts["word/numbering.xml"])
	for _, m := range regexp.MustCompile(`<w:numId w:val="(\d+)"/>`).FindAllStringSubmatch(docXML, -1) {
		if !strings.Contains(numbering, `<w:num w:numId="`+m[1]+`">`) {
			t.Errorf("numId %s not defined in numbering.xml", m[1])
		}
	}
	// docPr ids unique.
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(`<wp:docPr id="(\d+)"`).FindAllStringSubmatch(docXML, -1) {
		if seen[m[1]] {
			t.Errorf("duplicate wp:docPr id %s", m[1])
		}
		seen[m[1]] = true
	}
}

func TestWriteDocxContent(t *testing.T) {
	doc := buildDocxSource(t)
	var buf bytes.Buffer
	if err := doc.WriteDocx(&buf); err != nil {
		t.Fatal(err)
	}
	parts := docxParts(t, buf.Bytes())
	docXML := string(parts["word/document.xml"])
	text := docxText(t, parts)

	for _, want := range []string{"Annual Report", "quarterly revenue", "Key highlights",
		"Revenue up", "First step", "code_sample()", "visit example"} {
		if !strings.Contains(text, want) {
			t.Errorf("document text missing %q", want)
		}
	}
	// The title and section become named heading styles.
	if !strings.Contains(docXML, `<w:pStyle w:val="Heading1"/>`) {
		t.Error("no Heading1 paragraph")
	}
	// Bullet markers are stripped and items carry numbering.
	if strings.Contains(text, "•") {
		t.Error("bullet glyph leaked into list text")
	}
	if !strings.Contains(docXML, `<w:numPr>`) {
		t.Error("no numbered/bulleted paragraphs")
	}
	// Ordered list items keep their text without the "N." prefix.
	if strings.Contains(text, "1. First step") {
		t.Error("ordered marker not stripped")
	}
	// The link wraps its runs.
	if !strings.Contains(docXML, `<w:hyperlink r:id=`) {
		t.Error("no hyperlink element")
	}
	// Monospace text uses Courier New.
	if !regexp.MustCompile(`<w:rFonts w:ascii="Courier New"[^/]*/>[^<]*(<[^>]+>)*`).MatchString(docXML) {
		t.Error("code run not in Courier New")
	}
}

func TestSaveDocxWithImage(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddText("Before image", pdf.TextStyle{Size: 12},
		pdf.Rectangle{LLX: 60, LLY: 720, URX: 535, URY: 760}); err != nil {
		t.Fatal(err)
	}
	if err := p.AddImage("testdata/Koala.jpg", pdf.Rectangle{LLX: 100, LLY: 400, URX: 400, URY: 620}); err != nil {
		t.Skip("no test image available:", err)
	}
	var buf bytes.Buffer
	if err := doc.WriteDocx(&buf); err != nil {
		t.Fatal(err)
	}
	parts := docxParts(t, buf.Bytes())
	found := false
	for name := range parts {
		if strings.HasPrefix(name, "word/media/image1.") {
			found = true
		}
	}
	if !found {
		t.Error("no media part written")
	}
	docXML := string(parts["word/document.xml"])
	if !strings.Contains(docXML, "<wp:extent cx=") || !strings.Contains(docXML, "<a:blip r:embed=") {
		t.Error("no inline drawing markup")
	}
}

func TestWriteDocxPageSubset(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(doc.AddBlankPageFromFormat(pdf.PageFormatA4))
	for i := 1; i <= 2; i++ {
		p, err := doc.Page(i)
		must(err)
		must(p.AddText("Page text "+strings.Repeat("x", i), pdf.TextStyle{Size: 12},
			pdf.Rectangle{LLX: 60, LLY: 700, URX: 535, URY: 760}))
	}
	var buf bytes.Buffer
	if err := doc.WriteDocx(&buf, pdf.DocSaveOptions{Pages: []int{2}}); err != nil {
		t.Fatal(err)
	}
	text := docxText(t, docxParts(t, buf.Bytes()))
	if !strings.Contains(text, "Page text xx") || strings.Contains(text, "Page text x\n") {
		t.Errorf("page subset wrong: %q", text)
	}
	if err := doc.SaveDocx(filepath.Join(t.TempDir(), "bad.docx"), pdf.DocSaveOptions{Pages: []int{9}}); err == nil {
		t.Error("out-of-range page must error")
	}
}

func TestWriteDocxPageBreaks(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	// Three pages: text, blank, text — pagination must survive, including
	// the blank page in the middle.
	must(doc.AddBlankPageFromFormat(pdf.PageFormatA4))
	must(doc.AddBlankPageFromFormat(pdf.PageFormatA4))
	for _, n := range []int{1, 3} {
		p, err := doc.Page(n)
		must(err)
		must(p.AddText("Content", pdf.TextStyle{Size: 12},
			pdf.Rectangle{LLX: 60, LLY: 700, URX: 535, URY: 760}))
	}

	var buf bytes.Buffer
	must(doc.WriteDocx(&buf))
	docXML := string(docxParts(t, buf.Bytes())["word/document.xml"])
	if got := strings.Count(docXML, "<w:pageBreakBefore/>"); got != 2 {
		t.Errorf("page breaks = %d; want 2 (pages 2 and 3)", got)
	}

	buf.Reset()
	must(doc.WriteDocx(&buf, pdf.DocSaveOptions{NoPageBreaks: true}))
	docXML = string(docxParts(t, buf.Bytes())["word/document.xml"])
	if strings.Contains(docXML, "<w:pageBreakBefore/>") {
		t.Error("NoPageBreaks output still contains page breaks")
	}
}

func TestWriteDocxEnhancedFlowTables(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	tbl := pdf.NewTable().
		SetColumnWidths([]float64{120, 120, 120}).
		SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1}).
		SetDefaultCellBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1})
	tbl.AddRow().AddCells("Item", "Qty", "Price")
	tbl.AddRow().AddCells("Apples", "3", "4.50")
	r3 := tbl.AddRow()
	r3.AddCell("Total").SetColSpan(2)
	r3.AddCell("13.60")
	if _, err := p.AddTable(tbl, pdf.Rectangle{LLX: 60, LLY: 500, URX: 420, URY: 700}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := doc.WriteDocx(&buf); err != nil { // default = EnhancedFlow
		t.Fatal(err)
	}
	docXML := string(docxParts(t, buf.Bytes())["word/document.xml"])
	if !strings.Contains(docXML, "<w:tbl>") {
		t.Fatal("no w:tbl in EnhancedFlow output")
	}
	if got := strings.Count(docXML, "<w:tr>"); got != 3 {
		t.Errorf("rows = %d; want 3", got)
	}
	if !strings.Contains(docXML, `<w:gridSpan w:val="2"/>`) {
		t.Error("colspan not emitted as gridSpan")
	}
	for _, want := range []string{"Item", "Apples", "Total", "13.60"} {
		if !strings.Contains(docXML, ">"+want+"<") {
			t.Errorf("cell text %q missing", want)
		}
	}
	// The same table must NOT also appear as a picture: EnhancedFlow output
	// carries no media for this vector-only page.
	for name := range docxParts(t, buf.Bytes()) {
		if strings.HasPrefix(name, "word/media/") {
			t.Errorf("unexpected media part %s (table doubled as picture?)", name)
		}
	}

	// Flow mode keeps the picture behaviour.
	buf.Reset()
	if err := doc.WriteDocx(&buf, pdf.DocSaveOptions{Mode: pdf.DocFlow}); err != nil {
		t.Fatal(err)
	}
	docXML = string(docxParts(t, buf.Bytes())["word/document.xml"])
	if strings.Contains(docXML, "<w:tbl>") {
		t.Error("DocFlow must not emit tables")
	}
}
