// SPDX-License-Identifier: MIT

package asposepdf_test

import (
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

const (
	searchPageW = 400.0
	searchPageH = 300.0
)

// pageWithText returns a single-page document with one line of Helvetica text
// drawn near the top, used to exercise SearchText against known content.
func pageWithText(t *testing.T, text string) *asposepdf.Document {
	t.Helper()
	doc := asposepdf.NewDocument(searchPageW, searchPageH)
	p, err := doc.Page(1)
	if err != nil {
		t.Fatalf("Page(1): %v", err)
	}
	if err := p.AddText(text,
		asposepdf.TextStyle{Font: asposepdf.FontHelvetica, Size: 12},
		asposepdf.Rectangle{LLX: 20, LLY: 250, URX: searchPageW - 20, URY: 280}); err != nil {
		t.Fatalf("AddText: %v", err)
	}
	return doc
}

func assertRectInPage(t *testing.T, m asposepdf.TextMatch) {
	t.Helper()
	r := m.Rect
	if !(r.URX > r.LLX && r.URY > r.LLY) {
		t.Errorf("match %q: empty rect %+v", m.Text, r)
	}
	if r.LLX < 0 || r.URX > searchPageW || r.LLY < 0 || r.URY > searchPageH {
		t.Errorf("match %q: rect %+v out of page bounds %gx%g", m.Text, r, searchPageW, searchPageH)
	}
}

func TestSearchTextLiteral(t *testing.T) {
	doc := pageWithText(t, "Find the cat. The cat sat.")
	p, _ := doc.Page(1)

	got, err := p.SearchText("cat")
	if err != nil {
		t.Fatalf("SearchText: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf(`SearchText("cat"): got %d matches, want 2`, len(got))
	}
	for _, m := range got {
		if m.Text != "cat" {
			t.Errorf("match text: got %q, want %q", m.Text, "cat")
		}
		if m.PageNumber != 1 {
			t.Errorf("PageNumber: got %d, want 1", m.PageNumber)
		}
		assertRectInPage(t, m)
	}

	// Left-to-right ordering: first match precedes the second.
	if got[0].Rect.LLX >= got[1].Rect.LLX {
		t.Errorf("matches not in reading order: %v then %v", got[0].Rect, got[1].Rect)
	}

	none, err := p.SearchText("dog")
	if err != nil {
		t.Fatalf("SearchText: %v", err)
	}
	if len(none) != 0 {
		t.Errorf(`SearchText("dog"): got %d matches, want 0`, len(none))
	}
}

func TestSearchTextCaseSensitivity(t *testing.T) {
	doc := pageWithText(t, "Find the cat. The cat sat.")
	p, _ := doc.Page(1)

	if got, _ := p.SearchText("CAT"); len(got) != 0 {
		t.Errorf(`case-sensitive "CAT": got %d, want 0`, len(got))
	}
	if got, _ := p.SearchText("CAT", asposepdf.SearchOptions{CaseInsensitive: true}); len(got) != 2 {
		t.Errorf(`case-insensitive "CAT": got %d, want 2`, len(got))
	}
}

func TestSearchTextMultiFragment(t *testing.T) {
	doc := pageWithText(t, "Find the cat. The cat sat.")
	p, _ := doc.Page(1)

	// "the cat" spans two fragments plus the inserted space.
	cs, _ := p.SearchText("the cat")
	if len(cs) != 1 {
		t.Fatalf(`case-sensitive "the cat": got %d, want 1`, len(cs))
	}
	// The union box must be wider than a single word.
	if w := cs[0].Rect.URX - cs[0].Rect.LLX; w < 25 {
		t.Errorf("multi-fragment match too narrow: width %g", w)
	}
	assertRectInPage(t, cs[0])

	ci, _ := p.SearchText("the cat", asposepdf.SearchOptions{CaseInsensitive: true})
	if len(ci) != 2 {
		t.Errorf(`case-insensitive "the cat": got %d, want 2`, len(ci))
	}
}

func TestSearchTextRegex(t *testing.T) {
	doc := pageWithText(t, "Find the cat. The cat sat.")
	p, _ := doc.Page(1)

	// c.t matches both "cat"s.
	if got, _ := p.SearchText("c.t", asposepdf.SearchOptions{Regex: true}); len(got) != 2 {
		t.Errorf(`regex "c.t": got %d, want 2`, len(got))
	}
	// Case-sensitive [Tt]he matches "the" and "The".
	if got, _ := p.SearchText("[Tt]he", asposepdf.SearchOptions{Regex: true}); len(got) != 2 {
		t.Errorf(`regex "[Tt]he": got %d, want 2`, len(got))
	}
	// Regex honors case-insensitivity too.
	if got, _ := p.SearchText("THE", asposepdf.SearchOptions{Regex: true, CaseInsensitive: true}); len(got) != 2 {
		t.Errorf(`regex "THE" (i): got %d, want 2`, len(got))
	}
}

func TestSearchTextErrors(t *testing.T) {
	doc := pageWithText(t, "anything")
	p, _ := doc.Page(1)

	if _, err := p.SearchText(""); err == nil {
		t.Error("empty query: expected error")
	}
	if _, err := p.SearchText("(", asposepdf.SearchOptions{Regex: true}); err == nil {
		t.Error("invalid regex: expected error")
	}
	// A literal "(" is not a regex and must match nothing without erroring.
	if _, err := p.SearchText("("); err != nil {
		t.Errorf("literal %q should not error: %v", "(", err)
	}
}

func TestDocumentSearchTextAcrossPages(t *testing.T) {
	doc := asposepdf.NewDocument(searchPageW, searchPageH)
	p1, _ := doc.Page(1)
	if err := p1.AddText("alpha beta gamma",
		asposepdf.TextStyle{Font: asposepdf.FontHelvetica, Size: 12},
		asposepdf.Rectangle{LLX: 20, LLY: 250, URX: searchPageW - 20, URY: 280}); err != nil {
		t.Fatalf("AddText p1: %v", err)
	}
	if err := doc.AddBlankPage(searchPageW, searchPageH); err != nil {
		t.Fatalf("AddBlankPage: %v", err)
	}
	p2, _ := doc.Page(2)
	if err := p2.AddText("beta gamma delta",
		asposepdf.TextStyle{Font: asposepdf.FontHelvetica, Size: 12},
		asposepdf.Rectangle{LLX: 20, LLY: 250, URX: searchPageW - 20, URY: 280}); err != nil {
		t.Fatalf("AddText p2: %v", err)
	}

	got, err := doc.SearchText("beta")
	if err != nil {
		t.Fatalf("SearchText: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf(`SearchText("beta"): got %d, want 2`, len(got))
	}
	// Returned page by page in order.
	if got[0].PageNumber != 1 || got[1].PageNumber != 2 {
		t.Errorf("page numbers: got %d,%d want 1,2", got[0].PageNumber, got[1].PageNumber)
	}

	if only, _ := doc.SearchText("alpha"); len(only) != 1 || only[0].PageNumber != 1 {
		t.Errorf(`SearchText("alpha"): got %d matches (want 1 on page 1)`, len(only))
	}
}
