// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"strings"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestFlowPaginates: a long flow spills onto additional pages automatically.
func TestFlowPaginates(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	flow := doc.NewFlow(pdf.FlowOptions{})
	flow.AddHeading(1, "Annual Report", pdf.TextStyle{})
	flow.AddParagraph(strings.Repeat("Flowing text that wraps and paginates across pages. ", 90), pdf.TextStyle{})

	tbl := pdf.NewTable().SetColumnWidths([]float64{150, 150, 150})
	tbl.SetRepeatingRowsCount(1)
	tbl.AddRow().AddCells("Region", "Q3", "Q4")
	for i := 0; i < 4; i++ {
		tbl.AddRow().AddCells("Row", "$1M", "$2M")
	}
	flow.AddTable(tbl)
	flow.AddList([]string{"First", "Second longer note that might wrap", "Third"}, true, pdf.TextStyle{})

	pages, err := flow.Render()
	if err != nil {
		t.Fatal(err)
	}
	if pages < 2 {
		t.Errorf("expected the long flow to paginate, got %d page(s)", pages)
	}
	if doc.PageCount() != pages {
		t.Errorf("doc page count %d != flow pages %d", doc.PageCount(), pages)
	}
	img, err := doc.RenderImage(1, pdf.RenderOptions{DPI: 60})
	if err != nil {
		t.Fatal(err)
	}
	if nonWhitePixels(img) == 0 {
		t.Error("first flow page rendered blank")
	}
}

// TestFlowReusesBlankPage: a short flow reuses the document's initial blank page
// rather than leaving a leading empty page, and the text is on page 1.
func TestFlowReusesBlankPage(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	if _, err := doc.NewFlow(pdf.FlowOptions{}).
		AddHeading(1, "Title", pdf.TextStyle{}).
		AddParagraph("Body.", pdf.TextStyle{}).Render(); err != nil {
		t.Fatal(err)
	}
	if doc.PageCount() != 1 {
		t.Errorf("short flow used %d pages, want 1 (the blank page reused)", doc.PageCount())
	}
	p, _ := doc.Page(1)
	if txt, _ := p.ExtractText(); !bytes.Contains([]byte(txt), []byte("Title")) {
		t.Errorf("flow content not on page 1: %q", txt)
	}
}

// TestFlowTagged: a tagged flow produces a PDF/UA-conformant document that
// round-trips.
func TestFlowTagged(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	tc := doc.TaggedContent()
	tc.SetTitle("Tagged Flow")
	tc.SetLanguage("en-US")
	flow := doc.NewFlow(pdf.FlowOptions{Tagged: tc})
	flow.AddHeading(1, "Report", pdf.TextStyle{})
	flow.AddParagraph("An accessible flowing document.", pdf.TextStyle{})
	flow.AddList([]string{"Alpha", "Beta"}, false, pdf.TextStyle{})
	if _, err := flow.Render(); err != nil {
		t.Fatal(err)
	}
	if rep := doc.ValidatePDFUA(); !rep.Conformant {
		t.Fatalf("tagged flow not PDF/UA-conformant: %+v", rep.Issues)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if rep := out.ValidatePDFUA(); !rep.Conformant {
		t.Errorf("not conformant after round-trip: %+v", rep.Issues)
	}
}

// TestFlowBadMargins: margins that leave no content area error.
func TestFlowBadMargins(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	if _, err := doc.NewFlow(pdf.FlowOptions{MarginLeft: 400, MarginRight: 400}).
		AddParagraph("x", pdf.TextStyle{}).Render(); err == nil {
		t.Error("expected an error when margins leave no content area")
	}
}
