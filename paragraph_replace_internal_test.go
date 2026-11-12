// SPDX-License-Identifier: MIT

package asposepdf

import "testing"

// TestSetTextDropsOldContent: the content stream the paragraph was drawn in
// does not stay in the document once SetText replaces it.
func TestSetTextDropsOldContent(t *testing.T) {
	d := NewDocument(400, 400)
	p, _ := d.Page(1)
	if err := p.AddTextAt("OldParagraphWords here", TextStyle{Font: FontHelvetica, Size: 12}, 50, 300); err != nil {
		t.Fatal(err)
	}
	m, err := p.Paragraphs()
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Sections[0].Paragraphs[0].SetText("New words"); err != nil {
		t.Fatal(err)
	}
	if found, _ := documentContainsBytes(d, "OldParagraphWords"); found {
		t.Error("the old text is still in the document")
	}
}
