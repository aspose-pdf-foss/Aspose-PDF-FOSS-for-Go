// SPDX-License-Identifier: MIT

package asposepdf

import (
	"strings"
	"testing"
)

// twoPageTextDoc is a document of two pages, each with one line of text in
// Helvetica, saved and reopened so the text is found rather than drawn.
func twoPageTextDoc(t *testing.T, first, second string) *Document {
	t.Helper()
	d := NewDocumentFromFormat(PageFormatA4)
	if err := d.AddBlankPageFromFormat(PageFormatA4); err != nil {
		t.Fatal(err)
	}
	for i, text := range []string{first, second} {
		p, err := d.Page(i + 1)
		if err != nil {
			t.Fatal(err)
		}
		f := NewTextFragment(text)
		f.Position = Position{XIndent: 100, YIndent: 600}
		f.TextState = TextState{Font: FontHelvetica, FontSize: 12}
		if err := NewTextBuilder(p).AppendText(f); err != nil {
			t.Fatal(err)
		}
	}
	return reopenDoc(t, d)
}

// pdf-go-29ba: Page, Rectangle, BaselinePosition.
func TestTextFragmentPageRectangleBaseline(t *testing.T) {
	d := twoPageTextDoc(t, "First page", "Second page")
	f := absorbOne(t, d, "Second page")
	if p := f.Page(); p == nil || p.Number() != 2 {
		t.Fatalf("Page() = %v, want page 2", p)
	}
	// The page is followed when pages are reordered.
	if err := d.Reorder([]int{2, 1}); err != nil {
		t.Fatal(err)
	}
	if p := f.Page(); p == nil || p.Number() != 1 {
		t.Errorf("after reordering, Page() = %v, want page 1", p)
	}

	r := f.Rectangle()
	if !nearPt(r.LLX, 100) || r.URX-r.LLX < 50 || r.LLY >= 600 || r.URY <= 600 {
		t.Errorf("Rectangle() = %+v, want the text box around the baseline at (100, 600)", r)
	}
	if b := f.BaselinePosition(); !nearPt(b.XIndent, 100) || !nearPt(b.YIndent, 600) {
		t.Errorf("BaselinePosition() = %+v, want (100, 600)", b)
	}

	// A fragment on no page has none.
	if p := NewTextFragment("x").Page(); p != nil {
		t.Errorf("a new fragment's Page() = %v, want nil", p)
	}
	// A drawn fragment is on the page it was drawn on.
	nd := NewDocumentFromFormat(PageFormatA4)
	g := NewTextFragment("drawn")
	g.Position = Position{XIndent: 50, YIndent: 50}
	if err := NewTextBuilder(mustPage(t, nd)).AppendText(g); err != nil {
		t.Fatal(err)
	}
	if p := g.Page(); p == nil || p.Number() != 1 {
		t.Errorf("a drawn fragment's Page() = %v, want page 1", p)
	}
	// One returned by ExtractTextWithLayout takes its box from its fields.
	lines, err := mustPage(t, d).ExtractTextWithLayout()
	if err != nil || len(lines) == 0 || len(lines[0].Fragments) == 0 {
		t.Fatalf("layout: %v", err)
	}
	if lr := lines[0].Fragments[0].Rectangle(); lr.URX <= lr.LLX || lr.URY <= lr.LLY {
		t.Errorf("layout fragment Rectangle() = %+v", lr)
	}
}

// pdf-go-29ba: CloneWithSegments copies the segments and is on no page.
func TestTextFragmentCloneWithSegments(t *testing.T) {
	f := NewTextFragment("some text")
	seg := NewTextSegment("some segment")
	seg.TextState.FontSize = 40
	seg.Hyperlink = NewWebHyperlink("https://example.com/")
	f.Segments = append(f.Segments, seg)
	c := f.CloneWithSegments()
	if len(c.Segments) != 2 || c.Segments[1].TextState.FontSize != 40 {
		t.Fatalf("clone segments %+v", c.Segments)
	}
	c.Segments[1].TextState.FontSize = 10
	c.Segments[1].Hyperlink.(*WebHyperlink).URL = "https://other/"
	if seg.TextState.FontSize != 40 || seg.Hyperlink.(*WebHyperlink).URL != "https://example.com/" {
		t.Error("editing the clone changed the original")
	}

	// A clone of found text is on no page and can be drawn anew.
	d := twoPageTextDoc(t, "Found text", "Other")
	found := absorbOne(t, d, "Found text")
	fc := found.CloneWithSegments()
	if fc.Page() != nil {
		t.Error("the clone of a found fragment is on a page")
	}
	fc.Position = Position{XIndent: 100, YIndent: 300}
	if err := NewTextBuilder(mustPage(t, d)).AppendText(fc); err != nil {
		t.Fatal(err)
	}
	if n := absorbCount(t, reopenDoc(t, d), "Found text"); n != 2 {
		t.Errorf("%d occurrences after drawing the clone, want 2", n)
	}
}

// pdf-go-xiru: TextFragmentAbsorber.Remove deletes the text on save.
func TestTextFragmentAbsorberRemove(t *testing.T) {
	d := twoPageTextDoc(t, "Keep this", "Drop this")
	a := NewTextFragmentAbsorber("Drop this")
	if err := a.VisitDocument(d); err != nil {
		t.Fatal(err)
	}
	f := a.TextFragments()[0]
	if err := a.Remove(f); err != nil {
		t.Fatal(err)
	}
	if n := len(a.TextFragments()); n != 0 {
		t.Errorf("%d fragments left in the absorber", n)
	}
	if err := a.Remove(f); err == nil {
		t.Error("removing a fragment twice succeeded")
	}
	if err := NewTextFragmentAbsorber("x").Remove(NewTextFragment("x")); err == nil {
		t.Error("removing a fragment the absorber did not find succeeded")
	}
	out := reopenDoc(t, d)
	if n := absorbCount(t, out, "Drop this"); n != 0 {
		t.Errorf("removed text found %d times after save", n)
	}
	if n := absorbCount(t, out, "Keep this"); n != 1 {
		t.Errorf("other text found %d times, want 1", n)
	}
}

// linkActions lists the actions of the link annotations on page n.
func linkActions(t *testing.T, d *Document, n int) []Action {
	t.Helper()
	p, err := d.Page(n)
	if err != nil {
		t.Fatal(err)
	}
	var out []Action
	for _, a := range p.Annotations().All() {
		if l, ok := a.(*LinkAnnotation); ok {
			out = append(out, l.Action())
		}
	}
	return out
}

// pdf-go-xe2v: a local hyperlink on found text becomes a GoTo link to its
// page; a fragment's hyperlink reaches every segment without one; an edit
// of the hyperlink's field after drawing is applied.
func TestTextHyperlinks(t *testing.T) {
	d := twoPageTextDoc(t, "Go to page two", "Target")
	f := absorbOne(t, d, "Go to page two")
	f.Hyperlink = NewLocalHyperlink(2)
	out := reopenDoc(t, d)
	acts := linkActions(t, out, 1)
	if len(acts) != 1 {
		t.Fatalf("%d links on page 1, want 1", len(acts))
	}
	if g, ok := acts[0].(*GoToAction); !ok || g.PageNum() != 2 {
		t.Errorf("link action %#v, want GoTo page 2", acts[0])
	}
	if n := absorbCount(t, out, "Go to page two"); n != 1 {
		t.Errorf("linked text found %d times, want 1", n)
	}

	// A fragment's web hyperlink; a segment with its own keeps it.
	nd := NewDocumentFromFormat(PageFormatA4)
	g := NewTextFragment("first")
	g.Position = Position{XIndent: 100, YIndent: 600}
	g.Hyperlink = NewWebHyperlink("https://fragment/")
	own := NewTextSegment(" second")
	own.Hyperlink = NewLocalHyperlink(1)
	g.Segments = append(g.Segments, own)
	if err := NewTextBuilder(mustPage(t, nd)).AppendText(g); err != nil {
		t.Fatal(err)
	}
	var uris, gotos int
	for _, a := range linkActions(t, reopenDoc(t, nd), 1) {
		switch v := a.(type) {
		case *GoToURIAction:
			if v.URI() == "https://fragment/" {
				uris++
			}
		case *GoToAction:
			gotos++
		}
	}
	if uris != 1 || gotos != 1 {
		t.Errorf("%d fragment links and %d segment links, want 1 and 1", uris, gotos)
	}

	// Changing the URL in place after drawing redraws the link.
	g.Hyperlink.(*WebHyperlink).URL = "https://changed/"
	found := false
	for _, a := range linkActions(t, reopenDoc(t, nd), 1) {
		if u, ok := a.(*GoToURIAction); ok && u.URI() == "https://changed/" {
			found = true
		}
	}
	if !found {
		t.Error("the changed URL was not applied")
	}

	// A link to a page the document does not have is refused.
	bad := NewTextFragment("bad")
	bad.Position = Position{XIndent: 100, YIndent: 100}
	bad.Hyperlink = NewLocalHyperlink(9)
	if err := NewTextBuilder(mustPage(t, NewDocumentFromFormat(PageFormatA4))).AppendText(bad); err == nil || !strings.Contains(err.Error(), "page 9") {
		t.Errorf("link to page 9 of 1: %v", err)
	}
}

// pdf-go-1di0: TextEditOptions.NoCharacterAction when the page's font lacks
// a character of the new text.
func TestTextEditNoCharacterAction(t *testing.T) {
	edit := func(action NoCharacterAction, text string) (*Document, error) {
		d := twoPageTextDoc(t, "Hello there", "Other")
		a := NewTextFragmentAbsorber("Hello there")
		a.TextEditOptions = TextEditOptions{NoCharacterAction: action}
		if err := a.VisitDocument(d); err != nil {
			t.Fatal(err)
		}
		a.TextFragments()[0].Text = text
		return d, d.ApplyTextChanges()
	}
	// Helvetica (WinAnsi) has no Cyrillic.
	if _, err := edit(NoCharacterActionThrowException, "Hi Жx"); err == nil || !strings.Contains(err.Error(), "Ж") {
		t.Errorf("ThrowException: %v, want an error naming Ж", err)
	}
	// Text the font has is drawn in it, whatever the action.
	d, err := edit(NoCharacterActionThrowException, "Hi there")
	if err != nil {
		t.Fatalf("ThrowException with text the font has: %v", err)
	}
	if n := absorbCount(t, reopenDoc(t, d), "Hi there"); n != 1 {
		t.Errorf("edited text found %d times", n)
	}
	d, err = edit(NoCharacterActionReplaceAnyway, "Hi Жx")
	if err != nil {
		t.Fatalf("ReplaceAnyway: %v", err)
	}
	out := reopenDoc(t, d)
	text, err := mustPage(t, out).ExtractText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Hi x") || strings.Contains(text, "Ж") {
		t.Errorf("ReplaceAnyway page reads %q, want the text without the character the font lacks", text)
	}
	a := NewTextFragmentAbsorber("Hi x")
	if err := a.Visit(mustPage(t, out)); err != nil || len(a.TextFragments()) != 1 {
		t.Fatalf("ReplaceAnyway text not found: %v", err)
	}
	if fn := a.TextFragments()[0].FontName; !strings.Contains(fn, "Helvetica") {
		t.Errorf("ReplaceAnyway drew in %q, want the page's Helvetica", fn)
	}
}

// uriLinks counts page n's links to each URI (and GoTo links under "goto").
func uriLinks(t *testing.T, d *Document, n int) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, a := range linkActions(t, d, n) {
		switch v := a.(type) {
		case *GoToURIAction:
			out[v.URI()]++
		case *GoToAction:
			out["goto"]++
		}
	}
	return out
}

// A fragment's hyperlink can be replaced and cleared after an edit has been
// applied, on found and on drawn fragments; a found fragment's original
// links stay as they are; a link survives a text edit of a fragment with no
// segments.
func TestTextHyperlinkEditsAfterApply(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		d := twoPageTextDoc(t, "Linked words", "Other")
		f := absorbOne(t, d, "Linked words")
		f.Hyperlink = NewWebHyperlink("https://a/")
		if err := d.ApplyTextChanges(); err != nil {
			t.Fatal(err)
		}
		f.Hyperlink = NewWebHyperlink("https://b/")
		if got := uriLinks(t, reopenDoc(t, d), 1); got["https://a/"] != 0 || got["https://b/"] != 1 || len(got) != 1 {
			t.Errorf("after replacing the link: %v, want one link to b", got)
		}
		f.Hyperlink = nil
		if got := uriLinks(t, reopenDoc(t, d), 1); len(got) != 0 {
			t.Errorf("after clearing the link: %v, want none", got)
		}
	})
	t.Run("found with a link of its own", func(t *testing.T) {
		// The page already links the text; editing the fragment's link
		// adds and removes only what the edits drew.
		src := NewDocumentFromFormat(PageFormatA4)
		g := NewTextFragment("Linked words")
		g.Position = Position{XIndent: 100, YIndent: 600}
		g.Hyperlink = NewWebHyperlink("https://page/")
		if err := NewTextBuilder(mustPage(t, src)).AppendText(g); err != nil {
			t.Fatal(err)
		}
		d := reopenDoc(t, src)
		f := absorbOne(t, d, "Linked words")
		f.Hyperlink = NewWebHyperlink("https://a/")
		if err := d.ApplyTextChanges(); err != nil {
			t.Fatal(err)
		}
		f.Hyperlink = NewWebHyperlink("https://b/")
		if got := uriLinks(t, reopenDoc(t, d), 1); got["https://page/"] != 1 || got["https://b/"] != 1 || len(got) != 2 {
			t.Errorf("links %v, want the page's own and one to b", got)
		}
		f.Hyperlink = nil
		if got := uriLinks(t, reopenDoc(t, d), 1); got["https://page/"] != 1 || len(got) != 1 {
			t.Errorf("links %v, want only the page's own", got)
		}
	})
	t.Run("found, text and link changed together", func(t *testing.T) {
		d := twoPageTextDoc(t, "Linked words", "Other")
		f := absorbOne(t, d, "Linked words")
		f.Text = "Other words"
		f.Hyperlink = NewWebHyperlink("https://a/")
		if err := d.ApplyTextChanges(); err != nil {
			t.Fatal(err)
		}
		f.Hyperlink = nil
		if got := uriLinks(t, reopenDoc(t, d), 1); len(got) != 0 {
			t.Errorf("after clearing the link: %v, want none", got)
		}
	})
	t.Run("drawn without segments", func(t *testing.T) {
		d := NewDocumentFromFormat(PageFormatA4)
		f := &TextFragment{Text: "Hi", Position: Position{XIndent: 100, YIndent: 600}, Hyperlink: NewWebHyperlink("https://a/")}
		if err := NewTextBuilder(mustPage(t, d)).AppendText(f); err != nil {
			t.Fatal(err)
		}
		f.Text = "Hello"
		if got := uriLinks(t, reopenDoc(t, d), 1); got["https://a/"] != 1 || len(got) != 1 {
			t.Errorf("after a text edit: %v, want one link to a", got)
		}
		f.Hyperlink = nil
		if got := uriLinks(t, reopenDoc(t, d), 1); len(got) != 0 {
			t.Errorf("after clearing the link: %v, want none", got)
		}
	})
}

// Remove refuses a fragment that cannot be edited, leaving the document
// savable.
func TestTextFragmentAbsorberRemoveReadOnly(t *testing.T) {
	d := openBytes(t, rotatedDocBytes(t))
	a := NewTextFragmentAbsorber("tilted")
	if err := a.VisitDocument(d); err != nil || len(a.TextFragments()) != 1 {
		t.Fatalf("visit: %d, %v", len(a.TextFragments()), err)
	}
	if err := a.Remove(a.TextFragments()[0]); err == nil || !strings.Contains(err.Error(), "rotated") {
		t.Errorf("Remove on rotated text: %v, want a refusal", err)
	}
	if n := len(a.TextFragments()); n != 1 {
		t.Errorf("%d fragments after the refused Remove, want 1", n)
	}
	if n := absorbCount(t, reopenDoc(t, d), "tilted"); n != 1 {
		t.Errorf("rotated text found %d times after save, want 1", n)
	}
}

// A refused ThrowException edit leaves the page as it was.
func TestTextEditThrowExceptionLeavesPage(t *testing.T) {
	d := twoPageTextDoc(t, "Hello there", "Other")
	a := NewTextFragmentAbsorber("Hello there")
	a.TextEditOptions = TextEditOptions{NoCharacterAction: NoCharacterActionThrowException}
	if err := a.VisitDocument(d); err != nil {
		t.Fatal(err)
	}
	a.TextFragments()[0].Text = "Hi Жx"
	if err := d.ApplyTextChanges(); err == nil {
		t.Fatal("the edit was applied")
	}
	text, err := mustPage(t, d).ExtractText()
	if err != nil || !strings.Contains(text, "Hello there") {
		t.Errorf("page reads %q (%v), want it unchanged", text, err)
	}
}
