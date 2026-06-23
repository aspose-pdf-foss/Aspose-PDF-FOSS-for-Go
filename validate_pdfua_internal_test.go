// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

func hasUARule(rep *PDFUAValidationReport, rule string) bool {
	for _, is := range rep.Issues {
		if is.Rule == rule {
			return true
		}
	}
	return false
}

// makeTaggedDoc turns doc into a minimal Tagged/PDF-UA document with a Document
// → Figure structure tree. When figureAlt is false the figure lacks alt text.
func makeTaggedDoc(d *Document, figureAlt bool) {
	if d.catalog == nil {
		d.catalog = pdfDict{}
	}
	d.SetInfo(DocumentInfo{Title: "Accessible Test"})
	d.catalog["/Lang"] = "en-US"
	d.catalog["/MarkInfo"] = pdfDict{"/Marked": true}
	d.catalog["/ViewerPreferences"] = pdfDict{"/DisplayDocTitle": true}

	figure := pdfDict{"/Type": pdfName("/StructElem"), "/S": pdfName("/Figure")}
	if figureAlt {
		figure["/Alt"] = "a bar chart of quarterly sales"
	}
	figID := d.addObject(figure)
	docElemID := d.addObject(pdfDict{
		"/Type": pdfName("/StructElem"), "/S": pdfName("/Document"),
		"/K": pdfArray{pdfRef{Num: figID}},
	})
	ptID := d.addObject(pdfDict{"/Nums": pdfArray{}})
	rootID := d.addObject(pdfDict{
		"/Type":       pdfName("/StructTreeRoot"),
		"/K":          pdfRef{Num: docElemID},
		"/ParentTree": pdfRef{Num: ptID},
	})
	d.catalog["/StructTreeRoot"] = pdfRef{Num: rootID}
}

func TestValidatePDFUAUntagged(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	rep := doc.ValidatePDFUA()
	if rep.Conformant {
		t.Fatal("untagged document reported PDF/UA-conformant")
	}
	for _, want := range []string{"UA_NOT_TAGGED", "UA_NO_STRUCT_TREE", "UA_NO_LANG", "UA_NO_TITLE", "UA_DISPLAY_DOCTITLE"} {
		if !hasUARule(rep, want) {
			t.Errorf("expected %s for an untagged document", want)
		}
	}
}

func TestValidatePDFUATaggedConformant(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	makeTaggedDoc(doc, true)
	rep := doc.ValidatePDFUA()
	if !rep.Conformant {
		t.Fatalf("tagged document not conformant: %+v", rep.Issues)
	}
}

func TestValidatePDFUAFigureNoAlt(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	makeTaggedDoc(doc, false)
	rep := doc.ValidatePDFUA()
	if !hasUARule(rep, "UA_FIGURE_NO_ALT") {
		t.Errorf("expected UA_FIGURE_NO_ALT for a figure without alt text; got %+v", rep.Issues)
	}
	// The structural prerequisites should otherwise be satisfied.
	for _, unexpected := range []string{"UA_NOT_TAGGED", "UA_NO_STRUCT_TREE", "UA_NO_LANG", "UA_NO_TITLE"} {
		if hasUARule(rep, unexpected) {
			t.Errorf("unexpected %s on an otherwise-tagged document", unexpected)
		}
	}
}

func TestResolveStructType(t *testing.T) {
	roleMap := pdfDict{"/MyHeading": pdfName("/H1"), "/Loop": pdfName("/Loop")}
	if got := resolveStructType(roleMap, "/MyHeading"); got != "/H1" {
		t.Errorf("role map resolution = %q, want /H1", got)
	}
	if got := resolveStructType(roleMap, "/Figure"); got != "/Figure" {
		t.Errorf("standard type passthrough = %q, want /Figure", got)
	}
	if got := resolveStructType(roleMap, "/Loop"); got != "/Loop" {
		t.Errorf("self-referential role map should terminate, got %q", got)
	}
}
