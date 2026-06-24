// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"testing"
)

// TestTaggedTableHeadersAndArtifacts checks that header-row cells become /TH
// (the rest /TD) and that cell backgrounds/borders are bracketed as /Artifact in
// the content stream.
func TestTaggedTableHeadersAndArtifacts(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	tc := doc.TaggedContent()
	tc.SetTitle("T")
	tc.SetLanguage("en")
	p, _ := doc.Page(1)

	tbl := NewTable().SetColumnWidths([]float64{150, 150})
	tbl.SetRepeatingRowsCount(1)
	tbl.SetBorder(BorderInfo{Sides: BorderSideAll, Width: 1})
	tbl.AddRow().AddCells("H1", "H2")
	tbl.AddRow().AddCells("a", "b")
	if _, _, err := p.AddTaggedTable(tc, tc.Root(), tbl,
		Rectangle{LLX: 50, LLY: 500, URX: 550, URY: 760}); err != nil {
		t.Fatal(err)
	}

	// Count /TH and /TD structure elements in the object table.
	th, td := 0, 0
	for _, obj := range doc.objects {
		if d, ok := obj.Value.(pdfDict); ok && dictGetName(d, "/Type") == "/StructElem" {
			switch dictGetName(d, "/S") {
			case "/TH":
				th++
			case "/TD":
				td++
			}
		}
	}
	if th != 2 {
		t.Errorf("got %d /TH elements, want 2 (the header row)", th)
	}
	if td != 2 {
		t.Errorf("got %d /TD elements, want 2 (the body row)", td)
	}

	// The decoded page content brackets decoration as /Artifact and tags cells.
	content := decodedStreamData(resolveRef(doc.objects, p.pageDict()["/Contents"]).(*pdfStream))
	if !bytes.Contains(content, []byte("/Artifact BMC")) {
		t.Error("cell backgrounds/borders not bracketed as /Artifact")
	}
	if !bytes.Contains(content, []byte("/TH <</MCID")) {
		t.Error("header cell not tagged with an /TH marked-content sequence")
	}
}
