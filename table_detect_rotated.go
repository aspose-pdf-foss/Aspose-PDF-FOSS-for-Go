// SPDX-License-Identifier: MIT

package asposepdf

import "math"

// Borderless tables of text set at a quarter turn (a page laid out sideways
// with a rotating cm, or a sideways block on an upright page). The stream pass
// works on horizontal layout lines, which such text never forms, so its
// fragments are turned into their own frame — the baseline running left to
// right, lines stacked top to bottom — grouped into lines there, put through
// the same detector, and the tables found are turned back (pdf-go-sirj).

// quarterFrame maps page space to the frame of text rotated by rot (90 or
// 270 degrees) on a page w by h, and back. Both frames keep coordinates
// positive so the column histogram of the prose guard sees them.
type quarterFrame struct {
	rot, w, h float64
}

func (q quarterFrame) to(x, y float64) (float64, float64) {
	if q.rot == 90 {
		return y, q.w - x
	}
	return q.h - y, x
}

func (q quarterFrame) from(x, y float64) (float64, float64) {
	if q.rot == 90 {
		return q.w - y, x
	}
	return y, q.h - x
}

func (q quarterFrame) rectTo(r Rectangle) Rectangle {
	x0, y0 := q.to(r.LLX, r.LLY)
	x1, y1 := q.to(r.URX, r.URY)
	return Rectangle{LLX: minf(x0, x1), LLY: minf(y0, y1), URX: maxf(x0, x1), URY: maxf(y0, y1)}
}

func (q quarterFrame) rectFrom(r Rectangle) Rectangle {
	x0, y0 := q.from(r.LLX, r.LLY)
	x1, y1 := q.from(r.URX, r.URY)
	return Rectangle{LLX: minf(x0, x1), LLY: minf(y0, y1), URX: maxf(x0, x1), URY: maxf(y0, y1)}
}

// detectRotatedStreamTables runs the borderless pass over the page's text
// rotated by rot degrees. skip lists regions already claimed by tables;
// vRules, the page's vertical rules, are the frame's horizontal ones.
func detectRotatedStreamTables(p *Page, lines []TextLine, skip []Rectangle, vRules []rule, rot float64) []*AbsorbedTable {
	sz, err := p.Size()
	if err != nil {
		return nil
	}
	q := quarterFrame{rot: rot, w: sz.Width, h: sz.Height}

	type key struct {
		x, y float64
		text string
	}
	orig := map[key]TextFragment{}
	var frags []textFragment
	for _, l := range lines {
		for _, f := range l.Fragments {
			turn := math.Mod(f.Rotation-rot+540, 360) - 180
			if math.Abs(turn) > 1 || f.Text == "" {
				continue
			}
			sx, sy := q.to(f.X, f.Y)
			ex, _ := q.to(f.X+f.Width, f.endY)
			tf := textFragment{
				x: sx, y: sy, endX: ex, endY: sy,
				fontName: f.FontName, fontSize: f.FontSize, height: f.Height, descent: f.descent,
				bold: f.Bold, italic: f.Italic, charSpacing: f.CharSpacing,
				colorR: f.Color.R, colorG: f.Color.G, colorB: f.Color.B,
			}
			tf.text.WriteString(f.Text)
			frags = append(frags, tf)
			orig[key{sx, sy, f.Text}] = f
		}
	}
	if len(frags) < 2*streamMinRows {
		return nil
	}

	frameLines := groupFragmentsIntoLines(append([]textFragment(nil), frags...))
	var frameSkip []Rectangle
	for _, r := range skip {
		frameSkip = append(frameSkip, q.rectTo(r))
	}
	var frameRules []rule
	for _, r := range vRules {
		x0, y := q.to(r.pos, r.lo)
		x1, _ := q.to(r.pos, r.hi)
		frameRules = append(frameRules, rule{pos: y, lo: minf(x0, x1), hi: maxf(x0, x1)})
	}
	// The frame is q.h wide: the page's height runs along the baseline.
	markup := func() (*PageMarkup, error) {
		return &PageMarkup{PageNumber: p.Number(), Sections: markupSections(append([]textFragment(nil), frags...), q.h)}, nil
	}

	tables := detectStreamTables(frameLines, frameSkip, frameRules, p.Number(), markup)
	for _, t := range tables {
		t.Rect = q.rectFrom(t.Rect)
		for _, row := range t.rows {
			row.Rect = q.rectFrom(row.Rect)
			for _, c := range row.cells {
				c.Rect = q.rectFrom(c.Rect)
				for i, fr := range c.fragments {
					if o, ok := orig[key{fr.X, fr.Y, fr.Text}]; ok {
						c.fragments[i] = o
					}
				}
			}
		}
	}
	return tables
}
