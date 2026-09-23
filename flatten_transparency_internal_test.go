// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestFlattenTransparencyClearsPageGroup covers a page-level /Group. The
// public drawing API never emits one itself — ShapeStyle.Color.A < 1 only
// ever produces a resource-level ExtGState /ca — so this pokes the page
// dict directly (hence living in the internal test package); real-world
// PDFs (isolated/knockout page transparency groups from Adobe tools) do
// carry one, and pageNeedsTransparencyFlatten explicitly checks the page
// dict itself for this reason. Found by code review: an earlier version
// detected and rasterized such a page but never cleared /Group afterward,
// so ValidatePDFA's TRANSPARENCY check kept firing and a second
// FlattenTransparency call kept re-flattening the same page forever.
func TestFlattenTransparencyClearsPageGroup(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddText("hi", TextStyle{Size: 12}, Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720}); err != nil {
		t.Fatal(err)
	}
	p.pageDict()["/Group"] = pdfDict{"/S": pdfName("/Transparency")}

	n, err := doc.FlattenTransparency()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("FlattenTransparency() = %d, want 1", n)
	}
	if _, ok := p.pageDict()["/Group"]; ok {
		t.Error("page-level /Group survived flattening")
	}

	r := doc.ValidatePDFA(PDFA1B)
	for _, iss := range r.Issues {
		if iss.Rule == "TRANSPARENCY" {
			t.Errorf("TRANSPARENCY issue still present after flattening: %s", iss.Message)
		}
	}

	n2, err := doc.FlattenTransparency()
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 0 {
		t.Errorf("second FlattenTransparency() call flattened %d pages, want 0 (not idempotent)", n2)
	}
}
