// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"math"
	"strings"
)

// The seam between the OpenType shaper (shape.go) and text layout in
// renderTextInBuilder (RTL support, phase 3; epic pdf-go-26u4). A wrapped
// line is split into bidi level runs, each run is shaped in logical order
// with its own direction, and the runs are laid out left to right in visual
// order as positioned glyphs.

// bidiRun is a maximal stretch of a line at one embedding level, in logical
// order.
type bidiRun struct {
	text  string
	level int
}

// bidiLevelRuns splits one line into its level runs and returns them in
// visual order. Characters in right-to-left runs are mirrored (rule L4), so
// a parenthesis in Arabic text draws facing the right way.
func bidiLevelRuns(s string, baseLevel int) []bidiRun {
	runes := []rune(s)
	if len(runes) == 0 {
		return nil
	}
	levels := bidiResolve(runes, baseLevel)
	starts := []int{0}
	runOf := make([]int, len(runes))
	for i := 1; i < len(runes); i++ {
		if levels[i] != levels[i-1] {
			starts = append(starts, i)
		}
		runOf[i] = len(starts) - 1
	}
	starts = append(starts, len(runes))

	// Reordering reverses whole runs, so each run's characters stay together
	// in visual order: the first time a run is met is where it goes.
	var out []bidiRun
	prev := -1
	for _, idx := range bidiReorderIndices(levels) {
		id := runOf[idx]
		if id == prev {
			continue
		}
		prev = id
		seg := append([]rune(nil), runes[starts[id]:starts[id+1]]...)
		level := levels[starts[id]]
		if level%2 == 1 {
			for k, r := range seg {
				seg[k] = bidiMirror(r)
			}
		}
		out = append(out, bidiRun{text: string(seg), level: level})
	}
	return out
}

// shapedLine is one line shaped into positioned glyphs, in drawing order.
type shapedLine struct {
	glyphs []shapedGlyph
	upem   float64
}

// shapeLine shapes one wrapped line. Without bidi the line is a single
// left-to-right run.
func shapeLine(f *ttfFont, line string, baseLevel int, bidi bool) shapedLine {
	sl := shapedLine{upem: float64(f.unitsPerEm)}
	if sl.upem == 0 {
		sl.upem = 1000
	}
	runs := []bidiRun{{text: line}}
	if bidi {
		runs = bidiLevelRuns(line, baseLevel)
	}
	for _, run := range runs {
		rtl := run.level%2 == 1
		gs := shapeRun(f, run.text, rtl)
		if rtl {
			for i, j := 0, len(gs)-1; i < j; i, j = i+1, j-1 {
				gs[i], gs[j] = gs[j], gs[i]
			}
		}
		for i := range gs {
			gs[i].rtl = rtl
		}
		sl.glyphs = append(sl.glyphs, gs...)
	}
	return sl
}

// width is the line's total advance in points.
func (l shapedLine) width(size float64) float64 {
	var adv int64
	for _, g := range l.glyphs {
		adv += int64(g.advance)
	}
	return float64(adv) / l.upem * size
}

// pdfGlyphWidth is the advance the embedded font's /W array declares for
// gid, in thousandths of an em — rounded exactly as buildWArray rounds it,
// so positioning corrections are measured against what a viewer will use.
func pdfGlyphWidth(f *ttfFont, gid uint16) float64 {
	if int(gid) >= len(f.glyphWidths) || f.unitsPerEm == 0 {
		return defaultCIDWidth
	}
	return float64(int(float64(f.glyphWidths[gid])*1000.0/float64(f.unitsPerEm) + 0.5))
}

// actualTextSpans finds the glyphs whose text cannot ride on /ToUnicode and
// groups each into a span to be written with an /ActualText: a hidden
// joiner (its space glyph must not read as a space), a glyph whose mapping
// already belongs to other text (mappable[i] false), and the pieces of a
// GSUB decomposition that stand for no character of their own, grouped
// with the glyph that carries their cluster's text. It returns, per glyph
// index, the last index of the span starting there, or -1.
func (l shapedLine) actualTextSpans(mappable []bool) []int {
	n := len(l.glyphs)
	spans := make([]int, n)
	for i := range spans {
		spans[i] = -1
	}
	lastEnd := -1
	for i := 0; i < n; {
		g := l.glyphs[i]
		switch {
		case g.ignorable:
			spans[i] = i
			lastEnd = i
			i++
		case g.text == "":
			j := i
			for j < n && l.glyphs[j].text == "" && !l.glyphs[j].ignorable {
				j++
			}
			a, b := i, j-1
			// The text-carrying glyph precedes its pieces in logical order,
			// which puts it after them once an RTL run is reversed.
			if g.rtl {
				if j < n && !l.glyphs[j].ignorable {
					b = j
				}
			} else if i > 0 && i-1 > lastEnd && !l.glyphs[i-1].ignorable {
				a = i - 1
			}
			spans[a] = b
			lastEnd = b
			i = b + 1
		case !mappable[i]:
			// A left-to-right owner of decomposition pieces is picked up by
			// the pieces' span that follows.
			if !g.rtl && i+1 < n && l.glyphs[i+1].text == "" && !l.glyphs[i+1].ignorable {
				i++
				continue
			}
			spans[i] = i
			lastEnd = i
			i++
		default:
			i++
		}
	}
	return spans
}

// spanText is the text of glyphs a..b in logical order, as a PDF
// UTF-16BE hex string body.
func (l shapedLine) spanText(a, b int) string {
	parts := make([]string, 0, b-a+1)
	for i := a; i <= b; i++ {
		parts = append(parts, l.glyphs[i].text)
	}
	if l.glyphs[a].rtl {
		for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
			parts[i], parts[j] = parts[j], parts[i]
		}
	}
	return "FEFF" + stringToUTF16BEHex(strings.Join(parts, ""))
}

// showOps returns the operators that draw the line from the current text
// position. Glyph ids go out as one Tj when the shaped positions agree with
// the advances the font declares; otherwise as TJ arrays with a kerning
// number wherever they differ. Glyphs placed off the baseline, such as
// marks, are bracketed by a text rise (Ts).
func (l shapedLine) showOps(ef *embeddedFont, size float64) string {
	return l.showOpsSpaced(ef, size, 0)
}

// showOpsSpaced is showOps with word spacing tw (points): a two-byte font
// ignores the Tw operator, so the pen is advanced by tw after every glyph
// standing for a space through a TJ number.
func (l shapedLine) showOpsSpaced(ef *embeddedFont, size, tw float64) string {
	perUnit := 1000 / l.upem
	mappable := make([]bool, len(l.glyphs))
	for i, g := range l.glyphs {
		if !g.ignorable && g.text != "" {
			mappable[i] = ef.claimGlyphText(g.gid, g.text)
		}
	}
	spans := l.actualTextSpans(mappable)
	var out, arr, run strings.Builder
	adjust := 0.0    // pending pen movement, thousandths of an em; positive moves right
	numbers := false // arr holds a kerning number, so it must be shown with TJ
	rise := int32(0)
	spanEnd := -1

	flushRun := func() {
		if run.Len() > 0 {
			arr.WriteByte('<')
			arr.WriteString(run.String())
			arr.WriteByte('>')
			run.Reset()
		}
	}
	flushAdjust := func() {
		// Corrections under a thousandth of an em are rounding in /W; they
		// are carried forward rather than written, so plain text stays a
		// plain Tj and no error accumulates along the line.
		if math.Abs(adjust) < 1 {
			return
		}
		flushRun()
		if arr.Len() > 0 {
			arr.WriteByte(' ')
		}
		// A TJ number moves the pen left by n/1000 em, hence the sign.
		arr.WriteString(formatFloat(-adjust))
		arr.WriteByte(' ')
		numbers = true
		adjust = 0
	}
	flushArray := func() {
		flushRun()
		switch body := strings.TrimSpace(arr.String()); {
		case body == "":
		case numbers:
			fmt.Fprintf(&out, "[%s] TJ\n", body)
		default:
			fmt.Fprintf(&out, "%s Tj\n", body)
		}
		arr.Reset()
		numbers = false
	}

	for i, g := range l.glyphs {
		if end := spans[i]; end >= 0 {
			flushArray()
			fmt.Fprintf(&out, "/Span <</ActualText <%s>>> BDC\n", l.spanText(i, end))
			spanEnd = end
		}
		if g.yOff != rise {
			flushArray()
			fmt.Fprintf(&out, "%s Ts\n", formatFloat(float64(g.yOff)/l.upem*size))
			rise = g.yOff
		}
		adjust += float64(g.xOff) * perUnit
		flushAdjust()
		fmt.Fprintf(&run, "%04X", g.gid)
		ef.useGlyph(g.gid)
		// After the glyph the pen sits at its offset plus the declared
		// width; the shaped position wants its origin plus its advance.
		adjust += float64(g.advance-g.xOff)*perUnit - pdfGlyphWidth(ef.ttf, g.gid)
		if tw != 0 && g.text == " " {
			adjust += tw / size * 1000
		}
		if i == spanEnd {
			flushArray()
			out.WriteString("EMC\n")
			spanEnd = -1
		}
	}
	flushArray()
	if rise != 0 {
		out.WriteString("0 Ts\n")
	}
	return out.String()
}
