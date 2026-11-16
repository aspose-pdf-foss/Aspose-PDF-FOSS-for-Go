// SPDX-License-Identifier: MIT

package asposepdf

// Text background colour (pdf-go-qjao), mirroring Aspose.PDF for .NET's
// TextSearchOptions.SearchForTextRelatedGraphics and
// TextState.BackgroundColor: the colour of the filled shape a match is drawn
// over — a highlight, a shaded table cell — found from the page's own
// content, the way TableAbsorber finds cell shading.

// pageFills returns the page's filled shapes in drawing order, Form XObjects
// included: each rectangle drawn with re, or the bounding box of any other
// filled path, with the fill colour in effect when it was painted.
func pageFills(p *Page) []shadedRect {
	data, err := p.contentStreams()
	if err != nil || len(data) == 0 {
		return nil
	}
	ops, err := parseContentStream(data)
	if err != nil {
		return nil
	}
	var fills []shadedRect
	visitPaths(p.doc.objects, ops, p.pageResources(), true, func(pv pathVisit) {
		if pv.paint != paintFill && pv.paint != paintBoth {
			return
		}
		if len(pv.rects) == 0 {
			fills = append(fills, shadedRect{Rectangle: pv.bbox, col: pv.fill})
			return
		}
		for _, r := range pv.rects {
			fills = append(fills, shadedRect{Rectangle: orderedRect(r), col: pv.fill})
		}
	})
	return fills
}

// backgroundAt returns the colour of the last shape drawn under (x, y), or
// nil when no filled shape covers it.
func backgroundAt(fills []shadedRect, x, y float64) *Color {
	for i := len(fills) - 1; i >= 0; i-- {
		f := fills[i]
		if x >= f.LLX && x <= f.URX && y >= f.LLY && y <= f.URY {
			c := f.col
			return &c
		}
	}
	return nil
}

// setMatchBackgrounds fills in BackgroundColor for the content matches (not
// those in annotations, which have no background in the page's content),
// reading each page's shapes once. The point tested is the centre of the
// match's first line.
func setMatchBackgrounds(d *Document, matches []TextMatch) {
	cache := map[int][]shadedRect{}
	for i := range matches {
		m := &matches[i]
		if m.annotation {
			continue
		}
		r := m.Rect
		if len(m.Segments) > 0 {
			r = m.Segments[0].Rect
		}
		fills, ok := cache[m.PageNumber]
		if !ok {
			if p, err := d.Page(m.PageNumber); err == nil {
				fills = pageFills(p)
			}
			cache[m.PageNumber] = fills
		}
		m.BackgroundColor = backgroundAt(fills, (r.LLX+r.URX)/2, (r.LLY+r.URY)/2)
	}
}
