// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestTaggedLinkObjectReferenceRoundTrip: a /Link structure element built
// with TagContent (visible text) + AddObjectReference (the OBJR pointing at
// the LinkAnnotation) survives Save/WriteTo and stays PDF/UA-conformant.
func TestTaggedLinkObjectReferenceRoundTrip(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	tc := doc.TaggedContent()
	tc.SetTitle("Link Test")
	tc.SetLanguage("en")
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}

	link := pdf.NewLinkAnnotation(p, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	link.SetAction(pdf.NewGoToURIAction("https://example.com"))
	if err := p.Annotations().Add(link); err != nil {
		t.Fatal(err)
	}

	linkElem, err := p.TagContent(tc.Root(), pdf.StructLink, func() error {
		return p.AddText("Visit our site", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12}, link.Rect())
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := linkElem.AddObjectReference(link); err != nil {
		t.Fatal(err)
	}

	if rep := doc.ValidatePDFUA(); !rep.Conformant {
		t.Fatalf("authored document not PDF/UA-conformant: %+v", rep.Issues)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"/OBJR", "/StructParent", "/Link"} {
		if !bytes.Contains(buf.Bytes(), []byte(marker)) {
			t.Errorf("output missing %s", marker)
		}
	}

	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if rep := out.ValidatePDFUA(); !rep.Conformant {
		t.Errorf("not conformant after round-trip: %+v", rep.Issues)
	}
	page, err := out.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	txt, err := page.ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(txt), []byte("Visit our site")) {
		t.Errorf("tagged link text lost after round-trip: %q", txt)
	}
	if page.Annotations().Count() != 1 {
		t.Errorf("annotation lost after round-trip: %d annotations, want 1", page.Annotations().Count())
	}
}

// TestAddObjectReferenceComposesWithBareAddChild: a link with no
// separately-tagged visible text (e.g. an image link tagged /Figure
// elsewhere) can attach the OBJR to a bare AddChild(StructLink) element.
func TestAddObjectReferenceComposesWithBareAddChild(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	tc := doc.TaggedContent()
	tc.SetTitle("T")
	tc.SetLanguage("en")
	p, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}
	link := pdf.NewLinkAnnotation(p, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	if err := p.Annotations().Add(link); err != nil {
		t.Fatal(err)
	}
	elem := tc.Root().AddChild(pdf.StructLink)
	if err := elem.AddObjectReference(link); err != nil {
		t.Fatal(err)
	}
	if rep := doc.ValidatePDFUA(); !rep.Conformant {
		t.Errorf("not conformant: %+v", rep.Issues)
	}
}
