package asposepdf

import (
	"math"
	"sort"
	"strings"
)

// TextColor represents an RGB color with values in [0, 1].
type TextColor struct {
	R float64
	G float64
	B float64
}

// TextFragment represents a contiguous run of text with uniform font.
type TextFragment struct {
	Text        string
	X           float64   // horizontal position in points (from left edge)
	Y           float64   // vertical position in points (from bottom edge)
	Width       float64   // width in points
	FontName    string    // e.g. "Helvetica", "Arial-BoldMT"
	FontSize    float64   // effective size in points
	Height      float64   // text height in points (from ascent/descent metrics)
	Bold        bool
	Italic      bool
	CharSpacing  float64   // character spacing (Tc operator) in text space units
	Color        TextColor // fill color
	IsSubscript  bool      // Y is below the line baseline
	IsSuperscript bool     // Y is above the line baseline
}

// TextLine represents a horizontal line of text fragments at a common Y position.
type TextLine struct {
	Text      string         // concatenated text of all fragments (with spaces)
	Y         float64        // vertical position in points (from bottom edge)
	Fragments []TextFragment
}

// groupFragmentsIntoLines groups text fragments into lines sorted in visual
// reading order (top-to-bottom, left-to-right).
func groupFragmentsIntoLines(frags []textFragment) []TextLine {
	if len(frags) == 0 {
		return nil
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

	return lines
}

// assembleLine builds a TextLine from fragments on the same line.
func assembleLine(frags []textFragment) TextLine {
	sort.Slice(frags, func(i, j int) bool {
		return frags[i].x < frags[j].x
	})

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
			gap := f.x - frags[lastEmitted].endX
			spaceThreshold := f.fontSize * 0.3
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
			Color:       TextColor{R: f.colorR, G: f.colorG, B: f.colorB},
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
	return line
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
