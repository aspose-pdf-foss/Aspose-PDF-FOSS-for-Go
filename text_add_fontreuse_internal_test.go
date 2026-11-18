// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

// A page font with the BaseFont AddText wants but another encoding (an OCR
// producer's custom /Differences) must not be reused: the text would show
// other characters.
func TestAddTextDoesNotReuseFontWithOtherEncoding(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	p, _ := doc.Page(1)
	custom := pdfDict{
		"/Type": pdfName("/Font"), "/Subtype": pdfName("/Type1"), "/BaseFont": pdfName("/Helvetica"),
		"/Encoding": pdfDict{
			"/BaseEncoding": pdfName("/WinAnsiEncoding"),
			"/Differences":  pdfArray{1, pdfName("/Z")},
		},
	}
	id := doc.addObject(custom)
	res := p.pageResources()
	if res == nil {
		res = pdfDict{}
		p.pageDict()["/Resources"] = res
	}
	res["/Font"] = pdfDict{"/F0": pdfRef{Num: id}}

	name, err := p.ensureStandardFontResource("/Helvetica")
	if err != nil {
		t.Fatal(err)
	}
	if name == "/F0" {
		t.Fatal("reused the font with custom /Differences")
	}
	// A plain WinAnsi font is still reused.
	again, _ := p.ensureStandardFontResource("/Helvetica")
	if again != name {
		t.Fatalf("plain WinAnsi font not reused: %s vs %s", again, name)
	}
	if err := p.AddText("EDITED", TextStyle{}, Rectangle{LLX: 10, LLY: 10, URX: 300, URY: 100}); err != nil {
		t.Fatal(err)
	}
	text, _ := p.ExtractText()
	if !strings.Contains(text, "EDITED") {
		t.Fatalf("extracted %q", text)
	}
}
