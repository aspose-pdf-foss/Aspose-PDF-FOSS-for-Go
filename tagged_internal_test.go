// SPDX-License-Identifier: MIT

package asposepdf

import (
	"bytes"
	"testing"
)

// TestTaggedMarkedContentEmitted verifies the page content stream actually
// carries the marked-content operators (BDC … EMC with an /MCID) that link the
// drawn content to the structure tree.
func TestTaggedMarkedContentEmitted(t *testing.T) {
	doc := NewDocumentFromFormat(PageFormatA4)
	tc := doc.TaggedContent()
	tc.SetTitle("T")
	tc.SetLanguage("en")
	p, _ := doc.Page(1)
	if _, err := p.TagContent(tc.Root(), StructP, func() error {
		return p.AddText("hi", TextStyle{Font: FontHelvetica, Size: 12},
			Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 730})
	}); err != nil {
		t.Fatal(err)
	}

	// The page's content stream(s) hold the marked content (decoded in memory).
	pageDict := p.pageObj().Value.(pdfDict)
	var content []byte
	switch c := pageDict["/Contents"].(type) {
	case *pdfStream:
		content = c.Data
	case pdfRef:
		if s, ok := resolveRef(doc.objects, c).(*pdfStream); ok {
			content = s.Data
		}
	case pdfArray:
		for _, e := range c {
			if s, ok := resolveRef(doc.objects, e).(*pdfStream); ok {
				content = append(content, s.Data...)
			}
		}
	}
	for _, op := range []string{"/P <</MCID 0>> BDC", "EMC"} {
		if !bytes.Contains(content, []byte(op)) {
			t.Errorf("content stream missing %q\n--- content ---\n%s", op, content)
		}
	}

	// The page is wired into the /ParentTree via /StructParents.
	if _, ok := pageDict["/StructParents"]; !ok {
		t.Error("page has no /StructParents entry")
	}
}
