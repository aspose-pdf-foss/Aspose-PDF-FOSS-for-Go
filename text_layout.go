// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

// TextFragment represents a contiguous run of text with uniform font.
type TextFragment struct {
	Text          string
	X             float64 // horizontal position in points (from left edge)
	Y             float64 // vertical position in points (from bottom edge)
	Width         float64 // width in points
	FontName      string  // e.g. "Helvetica", "Arial-BoldMT"
	FontSize      float64 // effective size in points
	Height        float64 // text height in points (from ascent/descent metrics)
	Bold          bool
	Italic        bool
	CharSpacing   float64 // character spacing (Tc operator) in text space units
	Color         Color   // fill color
	IsSubscript   bool    // Y is below the line baseline
	IsSuperscript bool    // Y is above the line baseline
	Rotation      float64 // baseline angle in degrees CCW; 0 = horizontal (diagonal watermarks, axis labels, …)

	// runeX holds the exact device-space start X of each rune (len == rune
	// count of Text). Unexported: used by SearchText for precise sub-fragment
	// match rectangles. Empty when positions were not recorded.
	runeX []float64
	// descent is how far the text box reaches below the baseline (≤ 0), so
	// the box spans Y+descent .. Y+descent+Height. Unexported: used by
	// SearchText and the comparer for match rectangles.
	descent float64
	// endY is the device-space y after the last glyph's advance (the end
	// point is (X+Width, endY)): the gap to the next fragment of a rotated
	// line is measured along the baseline.
	endY float64
}

// TextLine represents a horizontal line of text fragments at a common Y position.
type TextLine struct {
	Text      string  // concatenated text of all fragments (with spaces)
	Y         float64 // vertical position in points (from bottom edge)
	Fragments []TextFragment
}

// groupFragmentsIntoLines groups text fragments into lines sorted in visual
// reading order (top-to-bottom, left-to-right).
func groupFragmentsIntoLines(frags []textFragment) []TextLine {
	if len(frags) == 0 {
		return nil
	}

	// Rotated text that runs on from one fragment to the next (text along a
	// curve, each glyph at its own angle; a watermark split by a font change)
	// is one line, in the order it was drawn.
	chains, frags := chainRotatedFragments(frags)

	// Other rotated fragments (diagonal watermarks, axis labels) must not join
	// horizontal lines: each becomes its own line, merged back by Y below.
	var rotated []textFragment
	upright := frags[:0]
	for _, f := range frags {
		if math.Abs(f.rotation) > 1 {
			rotated = append(rotated, f)
		} else {
			upright = append(upright, f)
		}
	}
	frags = upright
	var chainLines []TextLine
	for _, c := range chains {
		chainLines = append(chainLines, lineFromOrdered(c))
	}
	if len(frags) == 0 {
		// Only rotated text on the page: emit each as its own line.
		lines := chainLines
		for _, f := range rotated {
			lines = append(lines, assembleLine([]textFragment{f}))
		}
		sort.SliceStable(lines, func(i, j int) bool { return lines[i].Y > lines[j].Y })
		return lines
	}

	// Sort by Y descending (top first), then X ascending (left first).
	sort.Slice(frags, func(i, j int) bool {
		if math.Abs(frags[i].y-frags[j].y) > 0.5 {
			return frags[i].y > frags[j].y
		}
		return frags[i].x < frags[j].x
	})

	// Group into lines by Y proximity.
	var lines []TextLine
	var curFrags []textFragment
	curY := frags[0].y

	for _, f := range frags {
		if f.text.Len() == 0 {
			continue
		}
		threshold := f.fontSize * 0.3
		if threshold < 1 {
			threshold = 1
		}
		if len(curFrags) > 0 && math.Abs(f.y-curY) > threshold {
			lines = append(lines, assembleLine(curFrags))
			curFrags = nil
		}
		curFrags = append(curFrags, f)
		curY = f.y
	}
	if len(curFrags) > 0 {
		lines = append(lines, assembleLine(curFrags))
	}

	// Merge rotated fragments back as standalone lines in Y order.
	for _, f := range rotated {
		lines = append(lines, assembleLine([]textFragment{f}))
	}
	lines = append(lines, chainLines...)
	if len(rotated) > 0 || len(chainLines) > 0 {
		sort.SliceStable(lines, func(i, j int) bool { return lines[i].Y > lines[j].Y })
	}

	return lines
}

// chainRotatedFragments takes out of frags (in drawing order) the runs of
// consecutive fragments that continue one another along their baselines and
// include rotated text: each next fragment starts where the previous one
// ended, give or take half an em across the baseline and up to an em of word
// gap along it, at nearly the same size and within 30 degrees of its angle.
// It returns those runs and the fragments left over (pdf-go-3iwy).
func chainRotatedFragments(frags []textFragment) (chains [][]textFragment, rest []textFragment) {
	follows := func(prev, f *textFragment) bool {
		if math.Abs(prev.rotation) <= 1 && math.Abs(f.rotation) <= 1 {
			return false // upright text is grouped into lines by position
		}
		em := prev.fontSize
		if em <= 0 || math.Abs(f.fontSize-em) > 0.2*em {
			return false
		}
		turn := math.Mod(f.rotation-prev.rotation+540, 360) - 180
		if math.Abs(turn) > 30 {
			return false
		}
		along, across := baselineGap(prev.endX, prev.endY, prev.rotation, f.x, f.y)
		return along >= -0.5*em && along <= em && math.Abs(across) <= 0.5*em
	}
	var cur []textFragment
	flush := func() {
		rotated := false
		for i := range cur {
			rotated = rotated || math.Abs(cur[i].rotation) > 1
		}
		if len(cur) > 1 && rotated {
			chains = append(chains, cur)
		} else {
			rest = append(rest, cur...)
		}
		cur = nil
	}
	for _, f := range frags {
		if f.text.Len() == 0 {
			rest = append(rest, f)
			continue
		}
		if len(cur) > 0 && !follows(&cur[len(cur)-1], &f) {
			flush()
		}
		cur = append(cur, f)
	}
	flush()
	return chains, rest
}

// baselineGap splits the step from a fragment's end point to the next
// fragment's start into its parts along and across the first one's baseline.
func baselineGap(endX, endY, rotation, x, y float64) (along, across float64) {
	sin, cos := math.Sincos(rotation * math.Pi / 180)
	dx, dy := x-endX, y-endY
	return dx*cos + dy*sin, dy*cos - dx*sin
}

// fragmentGap is the space between two fragments of one line: horizontal for
// upright text, along the first one's baseline for rotated text.
func fragmentGap(prevEndX, prevEndY, prevRot, x, y, rot float64) float64 {
	if math.Abs(prevRot) <= 1 && math.Abs(rot) <= 1 {
		return x - prevEndX
	}
	along, _ := baselineGap(prevEndX, prevEndY, prevRot, x, y)
	return along
}

// assembleLine builds a TextLine from fragments on the same line.
func assembleLine(frags []textFragment) TextLine {
	sort.Slice(frags, func(i, j int) bool {
		return frags[i].x < frags[j].x
	})
	return lineFromOrdered(frags)
}

// lineFromOrdered builds a TextLine from fragments already in reading order.
func lineFromOrdered(frags []textFragment) TextLine {

	// Determine baseline Y from the fragment with the largest font size.
	baselineY := frags[0].y
	maxFontSize := frags[0].fontSize
	for _, f := range frags[1:] {
		if f.fontSize > maxFontSize {
			maxFontSize = f.fontSize
			baselineY = f.y
		}
	}

	line := TextLine{
		Y: baselineY,
	}

	var buf strings.Builder
	lastEmitted := -1
	for i, f := range frags {
		text := f.text.String()
		if text == "" {
			continue
		}

		if lastEmitted >= 0 {
			p := &frags[lastEmitted]
			gap := fragmentGap(p.endX, p.endY, p.rotation, f.x, f.y, f.rotation)
			// A real inter-word space advances 0.25–0.28 em in proportional
			// faces, while kerning/tracking gaps stay well under 0.15 em —
			// so 0.2 em separates them. The old 0.3 em threshold sat ABOVE
			// a genuine space's width and swallowed single spaces between
			// styled runs ("Revenue grew" + bold "12%" → "Revenue grew12%").
			// The em is taken from the SMALLER of the adjacent fragments: a
			// space typed in either font justifies the separation (a 10pt
			// space next to a 24pt word must still count).
			minSize := f.fontSize
			if s := frags[lastEmitted].fontSize; s < minSize {
				minSize = s
			}
			spaceThreshold := minSize * 0.2
			if spaceThreshold < 1 {
				spaceThreshold = 1
			}
			if gap > spaceThreshold {
				buf.WriteByte(' ')
			}
		}

		buf.WriteString(text)
		lastEmitted = i
		tf := TextFragment{
			Text:        text,
			X:           f.x,
			Y:           f.y,
			Width:       f.endX - f.x,
			FontName:    cleanFontName(f.fontName),
			FontSize:    f.fontSize,
			Height:      f.height,
			Bold:        f.bold,
			Italic:      f.italic,
			CharSpacing: f.charSpacing,
			Color:       Color{R: f.colorR, G: f.colorG, B: f.colorB, A: 1},
			Rotation:    f.rotation,
			runeX:       f.runeX,
			descent:     f.descent,
			endY:        f.endY,
		}
		// Detect sub/superscript: smaller font with Y offset from baseline.
		if f.fontSize < maxFontSize*0.85 {
			dy := f.y - baselineY
			threshold := maxFontSize * 0.2
			if dy > threshold {
				tf.IsSuperscript = true
			} else if dy < -threshold {
				tf.IsSubscript = true
			}
		}
		line.Fragments = append(line.Fragments, tf)
	}

	line.Text = buf.String()
	// Glyphs are collected in visual order (left to right on the page), which
	// reads backwards for right-to-left scripts; put the line back into the
	// order its characters were typed.
	if bidiHasStrongRTL(line.Text) {
		logical, _ := bidiVisualToLogical([]rune(line.Text))
		line.Text = string(logical)
	}
	return line
}

// buildFlattenedTextFromFragments lays the page out on a fixed-pitch
// character grid: each fragment starts at the column its x position maps to,
// the space between fragments is padding, and a wide vertical gap becomes a
// blank line. The result reads like the page in a monospace font, which is
// what makes it diffable and what keeps columns aligned without a table
// detector (TextExtractFlatten).
func buildFlattenedTextFromFragments(frags []textFragment) string {
	lines := groupFragmentsIntoLines(frags)
	if len(lines) == 0 {
		return ""
	}
	cell := flattenCellWidth(lines)
	originX := flattenOriginX(lines)

	var buf strings.Builder
	for i, line := range lines {
		if i > 0 {
			buf.WriteByte(0x0A)
			gap := lines[i-1].Y - line.Y
			size := 12.0
			if len(line.Fragments) > 0 {
				size = line.Fragments[0].FontSize
			}
			if gap > size*1.5 {
				buf.WriteByte(0x0A)
			}
		}
		buf.WriteString(flattenLine(line, originX, cell))
	}
	return buf.String()
}

// flattenLine places one line on the grid, never overwriting text already
// placed: a fragment whose column is behind the cursor is separated by a
// single space instead.
func flattenLine(line TextLine, originX, cell float64) string {
	var out []rune
	prevEnd := 0.0
	for _, f := range line.Fragments {
		if f.Text == "" {
			continue
		}
		col := int(math.Round((f.X - originX) / cell))
		if col < 0 {
			col = 0
		}
		if len(out) > 0 && col <= len(out) {
			// Behind or right at the text already placed: a word gap on the
			// page still gets its space (pdf-go-a2id).
			joined := out[len(out)-1] == ' ' || strings.HasPrefix(f.Text, " ")
			if col < len(out) || (!joined && gapIsSpace(prevEnd, f)) {
				col = len(out) + 1
			}
		}
		for len(out) < col {
			out = append(out, 0x20)
		}
		out = append(out, []rune(f.Text)...)
		prevEnd = f.X + f.Width
	}
	return strings.TrimRight(string(out), " ")
}

// flattenCellWidth is the width of one grid cell: the median advance per
// character across the page. The median rather than the minimum, so one
// narrow superscript cannot stretch the whole page into ribbons.
func flattenCellWidth(lines []TextLine) float64 {
	var widths []float64
	for _, line := range lines {
		for _, f := range line.Fragments {
			n := utf8.RuneCountInString(f.Text)
			if n == 0 || f.Width <= 0 {
				continue
			}
			widths = append(widths, f.Width/float64(n))
		}
	}
	if len(widths) == 0 {
		return 6 // a sane default; the page has no measurable text anyway
	}
	sort.Float64s(widths)
	cell := widths[len(widths)/2]
	if cell < 0.5 {
		cell = 0.5
	}
	return cell
}

// flattenOriginX is the leftmost text position, so the leftmost column is 0
// rather than however far the page margin happens to be.
func flattenOriginX(lines []TextLine) float64 {
	origin := math.Inf(1)
	for _, line := range lines {
		for _, f := range line.Fragments {
			if f.Text != "" && f.X < origin {
				origin = f.X
			}
		}
	}
	if math.IsInf(origin, 1) {
		return 0
	}
	return origin
}

// buildTextFromFragments groups fragments into lines and joins them as plain text.
func buildTextFromFragments(frags []textFragment) string {
	lines := groupFragmentsIntoLines(frags)
	if len(lines) == 0 {
		return ""
	}

	var buf strings.Builder
	for i, line := range lines {
		if i > 0 {
			buf.WriteByte('\n')
			prevY := lines[i-1].Y
			gap := prevY - line.Y
			avgFontSize := 12.0
			if len(line.Fragments) > 0 {
				avgFontSize = line.Fragments[0].FontSize
			}
			if gap > avgFontSize*1.5 {
				buf.WriteByte('\n')
			}
		}
		buf.WriteString(line.Text)
	}
	return buf.String()
}

// cleanFontName strips the PDF name prefix "/" and subset prefix "ABCDEF+".
func cleanFontName(name string) string {
	if name == "" {
		return ""
	}
	if name[0] == '/' {
		name = name[1:]
	}
	if len(name) > 7 && name[6] == '+' {
		allUpper := true
		for i := 0; i < 6; i++ {
			if name[i] < 'A' || name[i] > 'Z' {
				allUpper = false
				break
			}
		}
		if allUpper {
			name = name[7:]
		}
	}
	return name
}

// ExtractTextWithLayout returns structured text lines sorted in visual
// (top-to-bottom, left-to-right) reading order. Each line contains
// its concatenated text and individual fragments with positions.
func (p *Page) ExtractTextWithLayout() ([]TextLine, error) {
	data, err := p.contentStreams()
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}

	ops, err := parseContentStream(data)
	if err != nil {
		return nil, err
	}

	resources := p.pageResources()
	fonts := resolveFontResources(p.doc.objects, resources)

	ext := newTextExtractor(p.doc.objects, fonts)
	ext.process(ops, resources)
	ext.flushFragment()

	return groupFragmentsIntoLines(ext.fragments), nil
}

// ExtractTextWithLayout returns structured text lines for each page.
// The returned slice has one entry per page (0-indexed).
func (d *Document) ExtractTextWithLayout() ([][]TextLine, error) {
	pages := d.Pages()
	result := make([][]TextLine, len(pages))
	for i, p := range pages {
		lines, err := p.ExtractTextWithLayout()
		if err != nil {
			return nil, err
		}
		result[i] = lines
	}
	return result, nil
}
