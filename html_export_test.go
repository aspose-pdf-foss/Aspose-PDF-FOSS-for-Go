// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func makeHTMLDoc(t *testing.T) *pdf.Document {
	t.Helper()
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	mustNoErr(t, p.AddText("Hello HTML export", pdf.TextStyle{Font: pdf.FontHelveticaBold, Size: 24},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 740}))
	mustNoErr(t, p.AddText("Second line in Courier", pdf.TextStyle{Font: pdf.FontCourier, Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 650, URX: 545, URY: 680}))
	mustNoErr(t, doc.AddBlankPageFromFormat(pdf.PageFormatA4))
	p2, _ := doc.Page(2)
	mustNoErr(t, p2.AddText("Page two", pdf.TextStyle{Font: pdf.FontTimesRoman, Size: 14},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 730}))
	return doc
}

// TestWriteHTMLStructure: the output carries one .page div per page, a valid
// base64 PNG background each, and transparent text spans with the page text.
func TestWriteHTMLStructure(t *testing.T) {
	var buf bytes.Buffer
	if err := makeHTMLDoc(t).WriteHTML(&buf, pdf.HTMLSaveOptions{DPI: 72, Title: "T"}); err != nil {
		t.Fatal(err)
	}
	s := buf.String()

	if got := strings.Count(s, `<div class="page"`); got != 2 {
		t.Errorf("page divs = %d, want 2", got)
	}
	for _, want := range []string{
		"<title>T</title>",
		"Hello HTML export",
		"Second line in Courier",
		"Page two",
		`class="f-mono"`,  // Courier maps to the mono family
		`class="f-serif"`, // Times maps to the serif family
		"font-weight:bold",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("HTML missing %q", want)
		}
	}

	// Every background decodes as a real PNG.
	re := regexp.MustCompile(`data:image/png;base64,([A-Za-z0-9+/=]+)`)
	ms := re.FindAllStringSubmatch(s, -1)
	if len(ms) != 2 {
		t.Fatalf("embedded PNGs = %d, want 2", len(ms))
	}
	for i, m := range ms {
		raw, err := base64.StdEncoding.DecodeString(m[1])
		if err != nil {
			t.Fatalf("page %d: bad base64: %v", i+1, err)
		}
		if _, err := png.Decode(bytes.NewReader(raw)); err != nil {
			t.Fatalf("page %d: bad PNG: %v", i+1, err)
		}
	}
}

// TestWriteHTMLEscaping: text with HTML metacharacters is escaped.
func TestWriteHTMLEscaping(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	mustNoErr(t, p.AddText("a < b & c > d", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 720}))
	var buf bytes.Buffer
	if err := doc.WriteHTML(&buf, pdf.HTMLSaveOptions{DPI: 72}); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if !strings.Contains(s, "a &lt; b &amp; c &gt; d") {
		t.Error("metacharacters not escaped in the text layer")
	}
}

// TestSaveHTMLRealFile: a real PDF exports to a well-formed non-empty file.
func TestSaveHTMLRealFile(t *testing.T) {
	doc, err := pdf.Open(testFile(t))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join("result_files", "TestSaveHTMLRealFile")
	mustNoErr(t, os.MkdirAll(dir, 0o755))
	out := filepath.Join(dir, "out.html")
	if err := doc.SaveHTML(out, pdf.HTMLSaveOptions{DPI: 96}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.HasPrefix(s, "<!DOCTYPE html>") || !strings.HasSuffix(strings.TrimSpace(s), "</html>") {
		t.Error("output is not a complete HTML document")
	}
	if strings.Count(s, `<div class="page"`) != doc.PageCount() {
		t.Errorf("page divs = %d, want %d", strings.Count(s, `<div class="page"`), doc.PageCount())
	}
}
