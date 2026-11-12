// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"strings"
)

// replaceLayout is how ReplaceText lays a replacement out as a paragraph in
// a rectangle (pdf-go-2tdw): ReplaceOptions.Rectangle, FontSizeAdjustment,
// LeftAdjustment/RightAdjustment and the paragraph adjustments.
type replaceLayout struct {
	rect        *Rectangle // nil: the match's own rectangle
	fit         FontSizeAdjustment
	justify     bool
	left, right float64
}

// region is the rectangle a paragraph replacement of m goes into: the
// Rectangle option, else the union of the match's lines; a match on one line
// runs on to the page's right edge less the margin on its left, as Aspose
// lays a replacement out in the free space after it.
func (lay *replaceLayout) region(m replaceMatch, page Rectangle) (Rectangle, error) {
	r := m.rect
	for _, o := range m.more {
		r = unionRect(r, o)
	}
	if len(m.more) == 0 {
		if right := page.URX - (r.LLX - page.LLX); right > r.URX {
			r.URX = right
		}
	}
	if lay.rect != nil {
		r = *lay.rect
	}
	r.LLX += lay.left
	r.URX += lay.right
	if !(r.URX > r.LLX) {
		return r, fmt.Errorf("rectangle %v has no width after the edge adjustments", r)
	}
	return r, nil
}

// textBoxEm returns a font's ascender and descender per em (the descender
// negative), the text box SearchText reports for its glyphs.
func textBoxEm(f Font) (asc, desc float64) {
	switch f := f.(type) {
	case standardFont:
		if a, d, ok := standard14VerticalMetrics("/" + f.name); ok {
			return a / 1000, d / 1000
		}
	case *embeddedFont:
		if u := float64(f.ttf.unitsPerEm); u > 0 && f.ttf.ascent > 0 {
			return float64(f.ttf.ascent) / u, float64(f.ttf.descent) / u
		}
	}
	return 0.8, -0.2
}

// paragraphLine is one wrapped line of a paragraph replacement; last marks
// the last line of a paragraph of the replacement (before a "\n" or at its
// end), which justification leaves as it is.
type paragraphLine struct {
	text string
	last bool
}

// wrapReplacement wraps text at whole words to width at the given size.
// broken reports whether a word longer than a line had to be split.
func wrapReplacement(font Font, size float64, text string, rtl bool, width float64) (lines []paragraphLine, broken bool, err error) {
	wf, _, err := fontWidthAndAscent(font, size)
	if err != nil {
		return nil, false, err
	}
	for _, para := range strings.Split(text, "\n") {
		m := lineMeasurerForFont(font, size, para, rtl, wf)
		for _, w := range strings.Fields(para) {
			if m.word(w) > width+0.01 {
				broken = true
			}
		}
		wrapped := wrapText(para, m, width)
		if len(wrapped) == 0 {
			wrapped = []string{""}
		}
		for i, l := range wrapped {
			lines = append(lines, paragraphLine{text: l, last: i == len(wrapped)-1})
		}
	}
	return lines, broken, nil
}

// drawReplacementParagraph draws replacement for m as a paragraph in the
// layout's rectangle: wrapped to its width, the first line's text box at its
// top, in the match's line pitch (1.2 for a match on one line), at a size
// the layout's FontSizeAdjustment chooses, justified when asked.
func drawReplacementParagraph(p *Page, m replaceMatch, replacement string, lay *replaceLayout) error {
	page, err := p.CropBox()
	if err != nil {
		return err
	}
	r, err := lay.region(m, page)
	if err != nil {
		return err
	}
	width, height := r.URX-r.LLX, r.URY-r.LLY
	spacing := 1.2
	if m.pitch > 0 && m.size > 0 {
		spacing = m.pitch / m.size
	}
	asc, desc := textBoxEm(m.font)
	rtl := m.rtl || bidiBaseLevel(replacement, false) == 1

	layout := func(s float64) ([]paragraphLine, bool, error) {
		lines, broken, err := wrapReplacement(m.font, s, replacement, rtl, width)
		if err != nil {
			return nil, false, err
		}
		h := float64(len(lines)-1)*s*spacing + s*(asc-desc)
		return lines, !broken && h <= height+0.01, nil
	}
	// largestFitting finds by bisection the largest size in [lo, hi] at
	// which the text fits, or lo when none does.
	largestFitting := func(lo, hi float64) (float64, error) {
		if _, ok, err := layout(hi); err != nil || ok {
			return hi, err
		}
		for i := 0; i < 40 && hi-lo > 0.01; i++ {
			mid := (lo + hi) / 2
			_, ok, err := layout(mid)
			if err != nil {
				return 0, err
			}
			if ok {
				lo = mid
			} else {
				hi = mid
			}
		}
		return lo, nil
	}

	size := m.size
	switch lay.fit {
	case FontSizeAdjustmentShrinkToFit:
		if size, err = largestFitting(minf(1, size), size); err != nil {
			return err
		}
	case FontSizeAdjustmentScaleToFill:
		hi := maxf(size, height/(asc-desc))
		if size, err = largestFitting(minf(1, size), hi); err != nil {
			return err
		}
	}
	lines, _, err := layout(size)
	if err != nil {
		return err
	}

	style := TextStyle{Font: m.font, Size: size, Color: &Color{R: m.color.R, G: m.color.G, B: m.color.B, A: 1}}
	y := r.URY - asc*size
	for _, l := range lines {
		if err := drawParagraphLine(p, l, style, r, y, rtl, lay.justify); err != nil {
			return err
		}
		y -= size * spacing
	}
	return nil
}

// drawParagraphLine draws one line at baseline y in r: from the left edge, or
// ending at the right one for right-to-left text. A justified line (not the
// last of its paragraph, more than one word, left to right) is drawn word by
// word, the space between them stretched so the line spans r's width.
func drawParagraphLine(p *Page, l paragraphLine, style TextStyle, r Rectangle, y float64, rtl, justify bool) error {
	if l.text == "" {
		return nil
	}
	if rtl {
		style.HAlign = HAlignRight
		return p.AddTextAt(l.text, style, r.URX, y)
	}
	words := strings.Fields(l.text)
	if !justify || l.last || len(words) < 2 {
		return p.AddTextAt(l.text, style, r.LLX, y)
	}
	widths := make([]float64, len(words))
	total := 0.0
	for i, w := range words {
		wd, err := MeasureText(w, style)
		if err != nil {
			return err
		}
		widths[i] = wd
		total += wd
	}
	gap := (r.URX - r.LLX - total) / float64(len(words)-1)
	x := r.LLX
	for i, w := range words {
		if err := p.AddTextAt(w, style, x, y); err != nil {
			return err
		}
		x += widths[i] + gap
	}
	return nil
}
