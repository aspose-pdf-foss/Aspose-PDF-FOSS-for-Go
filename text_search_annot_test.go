// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// TestSearchInAnnotations: a sticky-note's text is not in the page content
// stream, so it is found only when SearchOptions.SearchInAnnotations is set, and
// the match carries the annotation's rectangle.
func TestSearchInAnnotations(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	p, _ := doc.Page(1)
	mustNoErr(t, p.AddText("ordinary page body", pdf.TextStyle{Font: pdf.FontHelvetica, Size: 12},
		pdf.Rectangle{LLX: 50, LLY: 700, URX: 500, URY: 720}))

	note := pdf.NewTextAnnotation(p, pdf.Point{X: 120, Y: 650})
	note.SetContents("annotation secret keyword")
	if err := p.Annotations().Add(note); err != nil {
		t.Fatal(err)
	}

	// Default search does not look inside annotations.
	def, err := p.SearchText("secret")
	if err != nil {
		t.Fatal(err)
	}
	if len(def) != 0 {
		t.Errorf("annotation text should not be found by default, got %d matches", len(def))
	}

	// With the option, the annotation text is searched.
	got, err := p.SearchText("secret", pdf.SearchOptions{SearchInAnnotations: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 annotation match, got %d", len(got))
	}
	if got[0].Text != "secret" {
		t.Errorf("match text = %q, want secret", got[0].Text)
	}
	if got[0].Rect != note.Rect() {
		t.Errorf("match rect = %+v, want annotation rect %+v", got[0].Rect, note.Rect())
	}
	if got[0].PageNumber != 1 {
		t.Errorf("match page = %d, want 1", got[0].PageNumber)
	}

	// Page body is still found normally, and combines with annotation matches.
	both, _ := p.SearchText("o", pdf.SearchOptions{SearchInAnnotations: true, CaseInsensitive: true})
	if len(both) == 0 {
		t.Error("expected matches across body and annotation")
	}
}
