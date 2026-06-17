// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"regexp"
	"sort"
	"unicode/utf8"
)

// SearchOptions tunes how SearchText interprets the query. The zero value is a
// case-sensitive literal search, matching the default of Aspose.PDF for .NET's
// TextFragmentAbsorber.
type SearchOptions struct {
	// CaseInsensitive folds case when matching (literal and regex alike).
	CaseInsensitive bool
	// Regex treats the query as an RE2 regular expression instead of a literal
	// string. Invalid patterns are reported as an error by SearchText.
	Regex bool
}

// TextMatch is a single occurrence located by SearchText.
type TextMatch struct {
	Text       string    // the matched text as it appears on the page
	PageNumber int       // 1-based page number the match was found on
	Rect       Rectangle // bounding box of the match in PDF user space (points)
}

// Rect precision: horizontal edges are taken from the recorded per-glyph start
// positions captured during extraction — exact for the left edge of any match
// and the right edge of any match that does not end at a fragment's final
// glyph (the common case for a word inside a line). When a match ends exactly
// on a fragment's last glyph, the right edge falls back to the fragment end
// and may be short by up to one glyph. Vertical edges come from the fragment's
// font ascent/descent.

// SearchText finds every occurrence of query on the page and returns the
// matches in visual reading order (top-to-bottom, left-to-right). Each match
// carries the matched text and a bounding rectangle in PDF user space.
//
// By default the query is a case-sensitive literal string; pass a SearchOptions
// to enable case-insensitive and/or regular-expression matching. At most one
// SearchOptions is honored (the last, if several are passed).
//
// This mirrors Aspose.PDF for .NET's TextFragmentAbsorber + page.Accept flow.
// Matches are located within a single text line: a query that would straddle a
// line break is not found. An empty query, or an invalid regular expression,
// returns an error.
func (p *Page) SearchText(query string, opts ...SearchOptions) ([]TextMatch, error) {
	re, err := compileSearch(query, lastSearchOption(opts))
	if err != nil {
		return nil, err
	}
	lines, err := p.ExtractTextWithLayout()
	if err != nil {
		return nil, err
	}
	return searchLines(lines, re, p.Number()), nil
}

// SearchText finds every occurrence of query across all pages of the document,
// returning matches page by page in reading order. Each TextMatch carries the
// page it was found on. See (*Page).SearchText for matching semantics.
func (d *Document) SearchText(query string, opts ...SearchOptions) ([]TextMatch, error) {
	re, err := compileSearch(query, lastSearchOption(opts))
	if err != nil {
		return nil, err
	}
	var out []TextMatch
	for _, p := range d.Pages() {
		lines, err := p.ExtractTextWithLayout()
		if err != nil {
			return nil, err
		}
		out = append(out, searchLines(lines, re, p.Number())...)
	}
	return out, nil
}

func lastSearchOption(opts []SearchOptions) SearchOptions {
	if len(opts) > 0 {
		return opts[len(opts)-1]
	}
	return SearchOptions{}
}

// compileSearch turns a query + options into a single RE2 matcher. Literal
// queries are quoted; case-insensitivity is applied uniformly via the (?i)
// flag, so Unicode case folding works for both literal and regex queries.
func compileSearch(query string, o SearchOptions) (*regexp.Regexp, error) {
	if query == "" {
		return nil, fmt.Errorf("empty search query")
	}
	pattern := query
	if !o.Regex {
		pattern = regexp.QuoteMeta(query)
	}
	if o.CaseInsensitive {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid search pattern: %w", err)
	}
	return re, nil
}

func searchLines(lines []TextLine, re *regexp.Regexp, pageNum int) []TextMatch {
	var out []TextMatch
	for i := range lines {
		out = append(out, searchLine(&lines[i], re, pageNum)...)
	}
	return out
}

// lineRuneMap flattens a line's fragments into one rune stream, recording for
// each rune its byte offset, owning fragment index (-1 for an inserted
// inter-fragment space), and rune index within that fragment. Shared by text
// search (searchLine) and text replace (collectReplaceMatchesLine).
type lineRuneMap struct {
	text       []byte
	runeByte   []int // byte offset where each rune starts
	owner      []int // fragment index per rune, or -1 for an inserted space
	local      []int // rune index within its owning fragment, or -1
	runeCounts []int // rune count per fragment
}

func buildLineRuneMap(line *TextLine) lineRuneMap {
	m := lineRuneMap{runeCounts: make([]int, len(line.Fragments))}
	var havePrev bool
	var prevEndX float64
	for fi := range line.Fragments {
		f := &line.Fragments[fi]
		if f.Text == "" {
			continue
		}
		m.runeCounts[fi] = utf8.RuneCountInString(f.Text)
		if havePrev {
			gap := f.X - prevEndX
			threshold := f.FontSize * 0.3
			if threshold < 1 {
				threshold = 1
			}
			if gap > threshold {
				m.runeByte = append(m.runeByte, len(m.text))
				m.owner = append(m.owner, -1)
				m.local = append(m.local, -1)
				m.text = append(m.text, ' ')
			}
		}
		k := 0
		for _, r := range f.Text {
			m.runeByte = append(m.runeByte, len(m.text))
			m.owner = append(m.owner, fi)
			m.local = append(m.local, k)
			m.text = utf8.AppendRune(m.text, r)
			k++
		}
		prevEndX = f.X + f.Width
		havePrev = true
	}
	return m
}

// searchLine matches re against a single line and maps each match back to a
// bounding rectangle. It reconstructs the line text from its fragments while
// recording, per rune, which fragment (and which rune within it) produced the
// character — replicating assembleLine's single-space-between-fragments rule —
// so a match's character span can be turned into page coordinates.
func searchLine(line *TextLine, re *regexp.Regexp, pageNum int) []TextMatch {
	if len(line.Fragments) == 0 {
		return nil
	}
	m := buildLineRuneMap(line)
	if len(m.owner) == 0 {
		return nil
	}

	var matches []TextMatch
	for _, loc := range re.FindAllIndex(m.text, -1) {
		b0, b1 := loc[0], loc[1]
		if b0 == b1 {
			continue // skip zero-width matches (e.g. "a*")
		}
		r0 := sort.SearchInts(m.runeByte, b0)
		r1 := sort.SearchInts(m.runeByte, b1)
		rect, ok := matchRect(line.Fragments, m.owner, m.local, m.runeCounts, r0, r1)
		if !ok {
			continue
		}
		matches = append(matches, TextMatch{
			Text:       string(m.text[b0:b1]),
			PageNumber: pageNum,
			Rect:       rect,
		})
	}
	return matches
}

// matchRect unions the per-rune cells of the match span [r0, r1) into one
// rectangle. A rune's horizontal extent comes from the fragment's recorded
// per-glyph start positions (runeX) when available — exact for the left edge
// of any rune and the right edge of any non-final rune; the final rune of a
// fragment falls back to the fragment end (X+Width). When runeX is absent the
// extent is interpolated uniformly from the fragment width. Inserted-space
// runes (owner < 0) contribute nothing.
func matchRect(frags []TextFragment, owner, local, runeCounts []int, r0, r1 int) (Rectangle, bool) {
	var (
		minX, minY, maxX, maxY float64
		found                  bool
	)
	for k := r0; k < r1 && k < len(owner); k++ {
		fi := owner[k]
		if fi < 0 {
			continue
		}
		f := &frags[fi]
		n := runeCounts[fi]
		if n <= 0 {
			continue
		}
		li := local[k]
		var x0, x1 float64
		if len(f.runeX) == n {
			x0 = f.runeX[li]
			if li+1 < n {
				x1 = f.runeX[li+1]
			} else {
				x1 = f.X + f.Width // fragment end (last glyph)
			}
		} else {
			x0 = f.X + float64(li)/float64(n)*f.Width
			x1 = f.X + float64(li+1)/float64(n)*f.Width
		}
		y0 := f.Y
		y1 := f.Y + f.Height
		if !found {
			minX, minY, maxX, maxY = x0, y0, x1, y1
			found = true
			continue
		}
		if x0 < minX {
			minX = x0
		}
		if x1 > maxX {
			maxX = x1
		}
		if y0 < minY {
			minY = y0
		}
		if y1 > maxY {
			maxY = y1
		}
	}
	if !found {
		return Rectangle{}, false
	}
	return Rectangle{LLX: minX, LLY: minY, URX: maxX, URY: maxY}, true
}
