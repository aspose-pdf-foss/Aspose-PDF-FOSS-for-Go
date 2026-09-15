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
