// SPDX-License-Identifier: MIT

package asposepdf

import (
	"sort"
	"strings"
	"unicode"
)

// wordToken is one word of a page together with the rectangle its glyphs
// occupy. Comparison works on these: the key drives matching, the rectangle
// drives the markup.
type wordToken struct {
	text string    // the word as it appears in the document
	key  string    // the comparison key (lower-cased when IgnoreCase is set)
	page int       // 1-based page number
	line int       // index of the layout line the word came from
	rect Rectangle // page-space bounding box
}

// pageWordTokens extracts the page's words in reading order.
func pageWordTokens(p *Page, ignoreCase bool) ([]wordToken, error) {
	lines, err := p.ExtractTextWithLayout()
	if err != nil {
		return nil, err
	}
	return lineWordTokens(lines, p.Number(), ignoreCase), nil
}

// lineWordTokens splits each line's text at whitespace and maps every word
// back to a rectangle through the same rune map SearchText uses, so
// right-to-left lines and sub-fragment boundaries are handled identically.
func lineWordTokens(lines []TextLine, pageNum int, ignoreCase bool) []wordToken {
	var out []wordToken
	for li := range lines {
		line := &lines[li]
		if len(line.Fragments) == 0 {
			continue
		}
		m := buildLineRuneMap(line)
		if len(m.owner) == 0 {
			continue
		}
		for _, span := range wordSpans(string(m.text)) {
			r0 := sort.SearchInts(m.runeByte, span[0])
			r1 := sort.SearchInts(m.runeByte, span[1])
			rect, ok := matchRect(line.Fragments, m.owner, m.local, m.runeCounts, r0, r1)
			if !ok {
				continue
			}
			text := string(m.text[span[0]:span[1]])
			out = append(out, wordToken{
				text: text,
				key:  tokenKey(text, ignoreCase),
				page: pageNum,
				line: li,
				rect: rect,
			})
		}
	}
	return out
}

// wordSpans returns the [start, end) byte spans of the whitespace-separated
// words of s.
func wordSpans(s string) [][2]int {
	var spans [][2]int
	start := -1
	for i, r := range s {
		if unicode.IsSpace(r) {
			if start >= 0 {
				spans = append(spans, [2]int{start, i})
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		spans = append(spans, [2]int{start, len(s)})
	}
	return spans
}

// tokenKey is the string two words are matched on.
func tokenKey(text string, ignoreCase bool) string {
	if ignoreCase {
		return strings.ToLower(text)
	}
	return text
}

// filterTokens keeps the words whose rectangle midpoint lies inside area
// (when non-nil) and outside every excluded rectangle. Deciding by the
// midpoint means a word on a boundary belongs to exactly one region.
func filterTokens(tokens []wordToken, area *Rectangle, exclude []Rectangle) []wordToken {
	if area == nil && len(exclude) == 0 {
		return tokens
	}
	out := make([]wordToken, 0, len(tokens))
	for _, tk := range tokens {
		if area != nil && !midpointIn(tk.rect, *area) {
			continue
		}
		skip := false
		for _, ex := range exclude {
			if midpointIn(tk.rect, ex) {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, tk)
		}
	}
	return out
}

// midpointIn reports whether the centre of r lies within area.
func midpointIn(r, area Rectangle) bool {
	cx := (r.LLX + r.URX) / 2
	cy := (r.LLY + r.URY) / 2
	return cx >= area.LLX && cx < area.URX && cy >= area.LLY && cy < area.URY
}

// tableRects returns the bounding rectangles of the tables detected on the
// page — ruled and borderless alike, since the absorber runs both passes.
func tableRects(p *Page) ([]Rectangle, error) {
	ta := NewTableAbsorber()
	if err := ta.Visit(p); err != nil {
		return nil, err
	}
	tables := ta.TableList()
	rects := make([]Rectangle, 0, len(tables))
	for _, t := range tables {
		rects = append(rects, t.Rect)
	}
	return rects, nil
}
