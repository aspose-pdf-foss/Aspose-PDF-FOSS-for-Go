// SPDX-License-Identifier: MIT

package asposepdf

import "strings"

// Extraction limited to the page or to a rectangle (pdf-go-c0t0), mirroring
// Aspose.PDF for .NET's TextSearchOptions.LimitToPageBounds and Rectangle on
// extraction: the region is applied glyph by glyph, so a word the page edge
// cuts keeps the letters that show.

// extractRegion returns the region opt limits extraction to; limited is
// false when it sets none.
func (p *Page) extractRegion(opt TextExtractOptions) (region Rectangle, limited bool, err error) {
	if opt.LimitToPageBounds {
		box, err := p.CropBox()
		if err != nil {
			return Rectangle{}, false, err
		}
		region, limited = orderedRect(box), true
	}
	if opt.Rectangle != nil {
		r := orderedRect(*opt.Rectangle)
		if limited {
			r = Rectangle{LLX: maxf(r.LLX, region.LLX), LLY: maxf(r.LLY, region.LLY),
				URX: minf(r.URX, region.URX), URY: minf(r.URY, region.URY)}
		}
		region, limited = r, true
	}
	return region, limited, nil
}

func orderedRect(r Rectangle) Rectangle {
	return Rectangle{LLX: minf(r.LLX, r.URX), LLY: minf(r.LLY, r.URY), URX: maxf(r.LLX, r.URX), URY: maxf(r.LLY, r.URY)}
}

// clipFragments keeps the glyphs whose advance lies inside r: both its start
// and its end — the next glyph's start, or the fragment's end for the last —
// within r. A fragment is split where glyphs fall out of it.
func clipFragments(frags []textFragment, r Rectangle) []textFragment {
	const tol = 0.01
	inside := func(x, y float64) bool {
		return x >= r.LLX-tol && x <= r.URX+tol && y >= r.LLY-tol && y <= r.URY+tol
	}
	var out []textFragment
	for i := range frags {
		f := &frags[i]
		runes := []rune(f.text.String())
		if len(runes) != len(f.runeX) || len(runes) != len(f.runeY) {
			// No per-glyph positions (a span emitted as one): all or nothing.
			if inside(f.x, f.y) && inside(f.endX, f.endY) {
				out = append(out, cloneFragment(f, string(runes), f.runeX, f.runeY, f.x, f.y, f.endX, f.endY))
			}
			continue
		}
		start := -1
		flush := func(end int) {
			if start < 0 {
				return
			}
			ex, ey := f.endX, f.endY
			if end < len(runes) {
				ex, ey = f.runeX[end], f.runeY[end]
			}
			out = append(out, cloneFragment(f, string(runes[start:end]), f.runeX[start:end], f.runeY[start:end],
				f.runeX[start], f.runeY[start], ex, ey))
			start = -1
		}
		for k := range runes {
			ex, ey := f.endX, f.endY
			if k+1 < len(runes) {
				ex, ey = f.runeX[k+1], f.runeY[k+1]
			}
			if inside(f.runeX[k], f.runeY[k]) && inside(ex, ey) {
				if start < 0 {
					start = k
				}
			} else {
				flush(k)
			}
		}
		flush(len(runes))
	}
	return out
}

// cloneFragment is f with other text and extent; its style is kept.
func cloneFragment(f *textFragment, text string, runeX, runeY []float64, x, y, endX, endY float64) textFragment {
	var b strings.Builder
	b.WriteString(text)
	return textFragment{
		text: b, x: x, y: y, endX: endX, endY: endY,
		runeX: append([]float64(nil), runeX...), runeY: append([]float64(nil), runeY...),
		fontName: f.fontName, fontSize: f.fontSize, height: f.height, descent: f.descent,
		bold: f.bold, italic: f.italic, charSpacing: f.charSpacing,
		colorR: f.colorR, colorG: f.colorG, colorB: f.colorB, rotation: f.rotation,
	}
}
