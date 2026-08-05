// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// Textbox mode: every paragraph line-cell becomes an absolutely positioned
// wps text box, pages become sections with their own size.
func TestWriteDocxTextbox(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	style := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12}
	if err := p.AddText("Hello textbox world", style, pdf.Rectangle{LLX: 100, LLY: 700, URX: 400, URY: 720}); err != nil {
		t.Fatal(err)
	}
	if err := doc.AddBlankPageFromFormat(pdf.PageFormatA4.Landscape()); err != nil {
		t.Fatal(err)
	}
	p2, _ := doc.Page(2)
	if err := p2.AddText("Landscape page", style, pdf.Rectangle{LLX: 60, LLY: 500, URX: 300, URY: 520}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := doc.WriteDocx(&buf, pdf.DocSaveOptions{Mode: pdf.DocTextbox}); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var docXML string
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, _ := f.Open()
			var sb strings.Builder
			b := make([]byte, 64*1024)
			for {
				n, err := rc.Read(b)
				sb.Write(b[:n])
				if err != nil {
					break
				}
			}
			rc.Close()
			docXML = sb.String()
		}
	}
	if docXML == "" {
		t.Fatal("no document.xml")
	}
	for _, want := range []string{
		"<v:textbox", "<v:shape ", "Hello textbox world", "Landscape page",
		"mso-position-horizontal-relative:page", `w:orient="landscape"`,
	} {
		if !strings.Contains(docXML, want) {
			t.Errorf("document.xml missing %q", want)
		}
	}
	if got := strings.Count(docXML, "<w:sectPr>"); got != 2 {
		t.Errorf("sections = %d; want 2 (one per page)", got)
	}
	if strings.Contains(docXML, "<w:tbl>") {
		t.Error("textbox mode must not emit tables")
	}
}
