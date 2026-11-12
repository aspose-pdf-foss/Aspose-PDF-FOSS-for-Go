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
	// Rectangle, when non-nil, limits results to matches whose bounding box
	// intersects this region (PDF user space, applied to every page). nil
	// searches the whole page. Mirrors Aspose.PDF for .NET's
	// TextSearchOptions.Rectangle.
	Rectangle *Rectangle
	// SearchInAnnotations also searches the text content (/Contents) of the
	// page's annotations — sticky notes, free text, markup, etc. Each match's
	// Rect is the annotation's rectangle. Mirrors Aspose.PDF for .NET's
	// TextSearchOptions.SearchInAnnotations.
	SearchInAnnotations bool
}

// TextMatch is a single occurrence located by SearchText.
type TextMatch struct {
	Text       string    // the matched text as it appears on the page; lines it spans are joined by "\n"
	PageNumber int       // 1-based page number the match starts on
	Rect       Rectangle // bounding box of the match on that page, in PDF user space (points)
	// Segments are the match's parts, one per line it covers, each with
	// its page and box: one segment for a match within a line. Mirrors the
	// segments of an Aspose.PDF for .NET TextFragment.
	Segments []TextMatchSegment
}

// TextMatchSegment is the part of a match on one line.
type TextMatchSegment struct {
	Text       string
	PageNumber int
	Rect       Rectangle
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
// A literal query is matched within a single text line. A regular expression
// is matched against the page's lines in reading order joined by "\n", so one
// such as "first\s+second" finds a phrase broken across lines; the match then
// has a segment per line. An empty query, or an invalid regular expression,
// returns an error.
func (p *Page) SearchText(query string, opts ...SearchOptions) ([]TextMatch, error) {
	opt := lastSearchOption(opts)
	re, err := compileSearch(query, opt)
	if err != nil {
		return nil, err
	}
	lines, err := p.ExtractTextWithLayout()
	if err != nil {
		return nil, err
	}
	var matches []TextMatch
	if opt.Regex {
		matches = searchAcrossLines([]pageLines{{p.Number(), lines}}, re)
	} else {
		matches = searchLines(lines, re, p.Number())
	}
	if opt.SearchInAnnotations {
		matches = append(matches, searchAnnotations(p, re)...)
	}
	return filterMatchesByRect(matches, opt.Rectangle), nil
}

// SearchText finds every occurrence of query across all pages of the document,
// returning matches page by page in reading order. Each TextMatch carries the
// page it was found on. See (*Page).SearchText for matching semantics; a
// regular expression here also matches across pages, the last line of one
// page joined to the first of the next by "\n", with a segment per line.
func (d *Document) SearchText(query string, opts ...SearchOptions) ([]TextMatch, error) {
	opt := lastSearchOption(opts)
	re, err := compileSearch(query, opt)
	if err != nil {
		return nil, err
	}
	var all []pageLines
	var out []TextMatch
	for _, p := range d.Pages() {
		lines, err := p.ExtractTextWithLayout()
		if err != nil {
			return nil, err
		}
		if opt.Regex {
			all = append(all, pageLines{p.Number(), lines})
		} else {
			out = append(out, searchLines(lines, re, p.Number())...)
		}
		if opt.SearchInAnnotations && !opt.Regex {
			out = append(out, searchAnnotations(p, re)...)
		}
	}
	if opt.Regex {
		out = searchAcrossLines(all, re)
		if opt.SearchInAnnotations {
			for _, p := range d.Pages() {
				out = append(out, searchAnnotations(p, re)...)
			}
		}
	}
	return filterMatchesByRect(out, opt.Rectangle), nil
}

// pageLines is a page's layout lines.
type pageLines struct {
	page  int
	lines []TextLine
}

// lineSpan is the part of a match on one line: runes [r0, r1) of the
// line's rune map.
type lineSpan struct {
	page   int
	line   *TextLine
	m      lineRuneMap
	r0, r1 int
}

// spanMatch is a match found across lines: its text and its part on each
// line it covers.
type spanMatch struct {
	text  string
	spans []lineSpan
}

// findAcrossLines matches re against the lines of the given pages in order,
// joined by "\n", splitting each match into the lines it covers
// (pdf-go-1kcx). Zero-width matches and matches of nothing but line joins
// are left out.
func findAcrossLines(pages []pageLines, re *regexp.Regexp) []spanMatch {
	type lineRef struct {
		page int
		line *TextLine
		m    lineRuneMap
	}
	var (
		text     []byte
		runeByte []int
		which    []int // per rune: index into refs, -1 for a joining "\n"
		local    []int // per rune: its index within the line's rune map
		refs     []lineRef
	)
	for _, pl := range pages {
		for i := range pl.lines {
			line := &pl.lines[i]
			if len(line.Fragments) == 0 {
				continue
			}
			m := buildLineRuneMap(line)
			if len(m.owner) == 0 {
				continue
			}
			if len(refs) > 0 {
				runeByte = append(runeByte, len(text))
				which = append(which, -1)
				local = append(local, -1)
				text = append(text, '\n')
			}
			base := len(text)
			for k, b := range m.runeByte {
				runeByte = append(runeByte, base+b)
				which = append(which, len(refs))
				local = append(local, k)
			}
			text = append(text, m.text...)
			refs = append(refs, lineRef{pl.page, line, m})
		}
	}
	var out []spanMatch
	for _, loc := range re.FindAllIndex(text, -1) {
		b0, b1 := loc[0], loc[1]
		if b0 == b1 {
			continue // skip zero-width matches (e.g. "a*")
		}
		r0 := sort.SearchInts(runeByte, b0)
		r1 := sort.SearchInts(runeByte, b1)
		sm := spanMatch{text: string(text[b0:b1])}
		for k := r0; k < r1; {
			if which[k] < 0 {
				k++
				continue
			}
			j := k
			for j < r1 && which[j] == which[k] {
				j++
			}
			ref := refs[which[k]]
			sm.spans = append(sm.spans, lineSpan{ref.page, ref.line, ref.m, local[k], local[j-1] + 1})
			k = j
		}
		if len(sm.spans) > 0 {
			out = append(out, sm)
		}
	}
	return out
}

// searchAcrossLines is findAcrossLines as TextMatches, with a segment per
// line. A match within one line gets exactly the result searchLine gives it.
func searchAcrossLines(pages []pageLines, re *regexp.Regexp) []TextMatch {
	var out []TextMatch
	for _, sm := range findAcrossLines(pages, re) {
		var match TextMatch
		for _, sp := range sm.spans {
			rect, ok := matchRect(sp.line.Fragments, sp.m.owner, sp.m.local, sp.m.runeCounts, sp.r0, sp.r1)
			if !ok {
				continue
			}
			e := len(sp.m.text)
			if sp.r1 < len(sp.m.runeByte) {
				e = sp.m.runeByte[sp.r1]
			}
			seg := TextMatchSegment{Text: string(sp.m.text[sp.m.runeByte[sp.r0]:e]), PageNumber: sp.page, Rect: rect}
			if len(match.Segments) == 0 {
				match.PageNumber, match.Rect = seg.PageNumber, seg.Rect
			} else if seg.PageNumber == match.PageNumber {
				match.Rect = unionRect(match.Rect, seg.Rect)
			}
			match.Segments = append(match.Segments, seg)
		}
		if len(match.Segments) == 0 {
			continue
		}
		match.Text = sm.text
		out = append(out, match)
	}
	return out
}

// searchAnnotations returns matches of re within the /Contents text of the
// page's annotations; each match's Rect is the annotation's rectangle.
func searchAnnotations(p *Page, re *regexp.Regexp) []TextMatch {
	var out []TextMatch
	for _, a := range p.Annotations().All() {
		content := a.Contents()
		if content == "" {
			continue
		}
		for _, loc := range re.FindAllStringIndex(content, -1) {
			out = append(out, TextMatch{
				Text:       content[loc[0]:loc[1]],
				PageNumber: p.Number(),
				Rect:       a.Rect(),
			})
		}
	}
	return out
}

// filterMatchesByRect keeps only matches whose bounding box intersects r. A nil
// r returns the matches unchanged.
func filterMatchesByRect(matches []TextMatch, r *Rectangle) []TextMatch {
	if r == nil {
		return matches
	}
	out := matches[:0]
	for _, m := range matches {
		hit := rectsIntersect(m.Rect, *r)
		for _, s := range m.Segments {
			hit = hit || rectsIntersect(s.Rect, *r)
		}
		if hit {
			out = append(out, m)
		}
	}
	return out
}

// rectsIntersect reports whether two axis-aligned rectangles overlap with
// positive area (edge-only contact does not count).
func rectsIntersect(a, b Rectangle) bool {
	return a.LLX < b.URX && a.URX > b.LLX && a.LLY < b.URY && a.URY > b.LLY
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
	var prevEndX, prevEndY, prevRot float64
	for fi := range line.Fragments {
		f := &line.Fragments[fi]
		if f.Text == "" {
			continue
		}
		m.runeCounts[fi] = utf8.RuneCountInString(f.Text)
		if havePrev {
			gap := fragmentGap(prevEndX, prevEndY, prevRot, f.X, f.Y, f.Rotation)
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
		prevEndX, prevEndY, prevRot = f.X+f.Width, f.endY, f.Rotation
		havePrev = true
	}
	if bidiHasStrongRTL(string(m.text)) {
		m = m.logicalOrder()
	}
	return m
}

// logicalOrder puts the line's runes — and the per-rune fragment mapping that
// turns a match back into a rectangle — into logical order, so a search for
// right-to-left text matches the way the text is typed rather than the way its
// glyphs sit on the page. The fragments themselves are left in visual order.
func (m lineRuneMap) logicalOrder() lineRuneMap {
	runes := []rune(string(m.text))
	if len(runes) != len(m.owner) {
		return m
	}
	logical, order := bidiVisualToLogical(runes)
	out := lineRuneMap{
		runeByte:   make([]int, 0, len(order)),
		owner:      make([]int, 0, len(order)),
		local:      make([]int, 0, len(order)),
		runeCounts: m.runeCounts,
	}
	for i, vis := range order {
		out.runeByte = append(out.runeByte, len(out.text))
		out.owner = append(out.owner, m.owner[vis])
		out.local = append(out.local, m.local[vis])
		out.text = utf8.AppendRune(out.text, logical[i])
	}
	return out
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
		text := string(m.text[b0:b1])
		matches = append(matches, TextMatch{
			Text:       text,
			PageNumber: pageNum,
			Rect:       rect,
			Segments:   []TextMatchSegment{{Text: text, PageNumber: pageNum, Rect: rect}},
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
		// The text box: from the descender to the ascender, the convention
		// viewers use for highlight and redaction quads.
		y0 := f.Y + f.descent
		y1 := y0 + f.Height
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
