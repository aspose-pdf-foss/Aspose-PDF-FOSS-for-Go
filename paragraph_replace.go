// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"strings"
)

// Replacing a paragraph's text (pdf-go-tjxp), mirroring setting
// MarkupParagraph.Text in Aspose.PDF for .NET: the paragraph's glyphs are
// removed from the page — only its text, so a background or rule under it
// stays — and the new text is set in the paragraph's place, wrapped to its
// width, in its first glyph's font, size and colour and at its line spacing.

// SetText replaces the paragraph's text on the page it was found on. The new
// text starts on the paragraph's first baseline and is wrapped to the
// paragraph's width at its line spacing; a text needing more lines than the
// paragraph had is set smaller, down to half size, to stay within them, and
// runs on below only past that. The font
// follows ReplaceText's choice: a Standard-14 face matching the original's
// family when it covers the text, else the original's family by name, else
// an error, raised before the page is touched.
func (mp *MarkupParagraph) SetText(text string) error {
	if mp.page == nil {
		return fmt.Errorf("set paragraph text: not a paragraph Paragraphs found")
	}
	var first *TextFragment
	var regions []QuadPoint
	for li := range mp.Lines {
		var box Rectangle
		have := false
		for fi := range mp.Lines[li].Fragments {
			f := &mp.Lines[li].Fragments[fi]
			if strings.TrimSpace(f.Text) == "" {
				continue
			}
			if first == nil {
				first = f
			}
			r := Rectangle{LLX: f.X, LLY: f.Y + f.descent, URX: f.X + f.Width, URY: f.Y + f.descent + f.Height}
			if have {
				box = unionRect(box, r)
			} else {
				box, have = r, true
			}
		}
		if have {
			const pad = 0.5
			regions = append(regions, rectAsQuadPoint(Rectangle{LLX: box.LLX - pad, LLY: box.LLY - pad, URX: box.URX + pad, URY: box.URY + pad}))
		}
	}
	if first == nil {
		return fmt.Errorf("set paragraph text: the paragraph shows no text")
	}
	p := mp.page
	size := first.FontSize
	font := Font(nil)
	if text != "" {
		rf := newReplaceFonts(nil)
		f, err := rf.fontFor(p.doc, replaceMatch{
			font:     standard14ForFragment(first.FontName, first.Bold, first.Italic),
			fontName: first.FontName, bold: first.Bold, italic: first.Italic,
		}, text)
		if err != nil {
			return fmt.Errorf("set paragraph text: %w", err)
		}
		font = f
	}

	old := contentStreamIDs(p.pageDict()["/Contents"])
	data, err := stripTextInRegions(p, regions)
	if err != nil {
		return fmt.Errorf("set paragraph text: %w", err)
	}
	if err := replacePageContents(p, data); err != nil {
		return fmt.Errorf("set paragraph text: %w", err)
	}
	p.doc.dropUnreachable(old)
	mp.Text = text
	if text == "" {
		return nil
	}

	spacing := 1.2
	if n := len(mp.Lines); n > 1 && size > 0 {
		if pitch := (mp.Lines[0].Y - mp.Lines[n-1].Y) / float64(n-1); pitch > 0 {
			spacing = pitch / size
		}
	}
	style := TextStyle{Font: font, Size: size, LineSpacing: spacing,
		Color: &Color{R: first.Color.R, G: first.Color.G, B: first.Color.B, A: 1}}
	width := mp.Rectangle.URX - mp.Rectangle.LLX
	// The paragraph keeps the lines it had: a text that needs more is set
	// smaller, down to half size, so it does not run into what follows.
	lines, lineHeight, err := measureText(text, style, width)
	if err != nil {
		return fmt.Errorf("set paragraph text: %w", err)
	}
	room := float64(len(mp.Lines)) * size * spacing
	for s := size - 0.25; float64(lines)*lineHeight > room+0.01 && s >= size/2; s -= 0.25 {
		style.Size = s
		if lines, lineHeight, err = measureText(text, style, width); err != nil {
			return fmt.Errorf("set paragraph text: %w", err)
		}
	}
	size = style.Size
	metrics, err := GetFontMetrics(font, size)
	if err != nil {
		return fmt.Errorf("set paragraph text: %w", err)
	}
	top := mp.Lines[0].Y + metrics.Ascent
	rect := Rectangle{LLX: mp.Rectangle.LLX, URX: mp.Rectangle.URX, URY: top,
		LLY: top - float64(lines)*lineHeight - size}
	if err := p.AddText(text, style, rect); err != nil {
		return fmt.Errorf("set paragraph text: %w", err)
	}
	return nil
}
