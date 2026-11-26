// SPDX-License-Identifier: MIT

package asposepdf

// Accessors of a TextFragment mirroring Aspose.PDF for .NET's
// TextFragment.Page, Rectangle, BaselinePosition and CloneWithSegments
// (pdf-go-29ba).

// Page returns the page the fragment was found or drawn on, followed through
// page reordering; nil for a fragment on no page (built but not drawn,
// cloned, drawn on a form canvas, or on a page removed since).
func (f *TextFragment) Page() *Page {
	b := f.bind
	if b == nil || b.page == nil || b.page.obj != nil || b.page.doc == nil {
		return nil
	}
	d := b.page.doc
	for i, o := range d.pages {
		if o == b.obj {
			if p, err := d.Page(i + 1); err == nil {
				return p
			}
		}
	}
	return nil
}

// Rectangle returns the fragment's text box on its page: the union of its
// segments' rectangles for a fragment found or drawn, else the box its
// position, width and height give (a fragment ExtractTextWithLayout
// returned). Zero when neither is known.
func (f *TextFragment) Rectangle() Rectangle {
	var r Rectangle
	have := false
	for _, s := range f.Segments {
		sr := s.Rectangle
		if sr == (Rectangle{}) {
			continue
		}
		if !have {
			r, have = sr, true
			continue
		}
		r = Rectangle{LLX: minf(r.LLX, sr.LLX), LLY: minf(r.LLY, sr.LLY), URX: maxf(r.URX, sr.URX), URY: maxf(r.URY, sr.URY)}
	}
	if have {
		return r
	}
	if f.Width == 0 && f.Height == 0 {
		return Rectangle{}
	}
	return Rectangle{LLX: f.X, LLY: f.Y + f.descent, URX: f.X + f.Width, URY: f.Y + f.descent + f.Height}
}

// BaselinePosition returns where the fragment's baseline starts. Here
// Position is already that point (in Aspose.PDF for .NET Position is the
// bottom of the text box and BaselinePosition the baseline); for a fragment
// ExtractTextWithLayout returned it is (X, Y).
func (f *TextFragment) BaselinePosition() Position {
	if f.Position != (Position{}) {
		return f.Position
	}
	return Position{XIndent: f.X, YIndent: f.Y}
}

// CloneWithSegments returns a copy of the fragment and of each of its
// segments, on no page: editing the copy changes neither the original nor
// the page, and it can be drawn with a TextBuilder like a new fragment.
func (f *TextFragment) CloneWithSegments() *TextFragment {
	c := *f
	c.bind = nil
	c.TextState = copyState(f.TextState)
	c.Hyperlink = cloneHyperlink(f.Hyperlink)
	if r := f.ReplaceOptions.Rectangle; r != nil {
		rc := *r
		c.ReplaceOptions.Rectangle = &rc
	}
	c.runeX = append([]float64(nil), f.runeX...)
	c.runeColor = append([]Color(nil), f.runeColor...)
	c.runeHidden = append([]bool(nil), f.runeHidden...)
	c.runeRef = append([]int(nil), f.runeRef...)
	c.runeRun = append([]textRunState(nil), f.runeRun...)
	c.Segments = make([]*TextSegment, len(f.Segments))
	for i, s := range f.Segments {
		if s == nil {
			continue
		}
		cs := *s
		cs.TextState = copyState(s.TextState)
		cs.Hyperlink = cloneHyperlink(s.Hyperlink)
		cs.glyphX = nil
		c.Segments[i] = &cs
	}
	return &c
}

func cloneHyperlink(h Hyperlink) Hyperlink {
	switch v := h.(type) {
	case *WebHyperlink:
		if v != nil {
			c := *v
			return &c
		}
	case *LocalHyperlink:
		if v != nil {
			c := *v
			return &c
		}
	}
	return h
}
