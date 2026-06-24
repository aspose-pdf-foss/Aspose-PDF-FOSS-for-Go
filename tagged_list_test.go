// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"bytes"
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestAddTaggedList: bulleted and numbered lists build /L → /LI → /Lbl+/LBody,
// validate as PDF/UA and round-trip with text intact.
func TestAddTaggedList(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	tc := doc.TaggedContent()
	tc.SetTitle("Lists")
	tc.SetLanguage("en-US")
	p, _ := doc.Page(1)
	style := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12}

	ul, err := p.AddTaggedList(tc, tc.Root(),
		[]string{"Apples", "Oranges", "Pears"}, style,
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 545, URY: 760}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.AddTaggedList(tc, tc.Root(),
		[]string{"First", "Second", "Third"}, style,
		pdf.Rectangle{LLX: 50, LLY: 400, URX: 545, URY: 560}, true); err != nil {
		t.Fatal(err)
	}
	if ul == nil {
		t.Fatal("nil list element")
	}
	if rep := doc.ValidatePDFUA(); !rep.Conformant {
		t.Fatalf("tagged list not PDF/UA-conformant: %+v", rep.Issues)
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	for _, marker := range []string{"/L", "/LI", "/Lbl", "/LBody"} {
		if !bytes.Contains(buf.Bytes(), []byte(marker)) {
			t.Errorf("output missing structure type %s", marker)
		}
	}
	out, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if rep := out.ValidatePDFUA(); !rep.Conformant {
		t.Errorf("not conformant after round-trip: %+v", rep.Issues)
	}
	page, _ := out.Page(1)
	txt, _ := page.ExtractText()
	for _, want := range []string{"Oranges", "Second"} {
		if !bytes.Contains([]byte(txt), []byte(want)) {
			t.Errorf("list text %q lost: %q", want, txt)
		}
	}
}

// TestTagArtifact: artifact-marked decoration keeps the document conformant (the
// content is excluded from the structure tree rather than left untagged).
func TestTagArtifact(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	tc := doc.TaggedContent()
	tc.SetTitle("Doc")
	tc.SetLanguage("en")
	p, _ := doc.Page(1)

	if err := p.TagArtifact(func() error {
		return p.AddText("CONFIDENTIAL — page footer", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 9},
			pdf.Rectangle{LLX: 50, LLY: 30, URX: 545, URY: 45})
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.TagContent(tc.Root(), pdf.StructP, func() error {
		return p.AddText("Body.", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
			pdf.Rectangle{LLX: 50, LLY: 700, URX: 545, URY: 740})
	}); err != nil {
		t.Fatal(err)
	}
	if rep := doc.ValidatePDFUA(); !rep.Conformant {
		t.Errorf("artifact + tagged content not conformant: %+v", rep.Issues)
	}
}

// TestTaggedListAndArtifactRequireSetup: both error without TaggedContent().
func TestTaggedListAndArtifactRequireSetup(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	if _, err := p.AddTaggedList(nil, nil, []string{"x"},
		pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 600, URX: 545, URY: 700}, false); err == nil {
		t.Error("expected error: AddTaggedList before TaggedContent()")
	}
	if err := p.TagArtifact(func() error { return nil }); err == nil {
		t.Error("expected error: TagArtifact before TaggedContent()")
	}
}
