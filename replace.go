// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ReplaceOptions tunes how ReplaceText matches the search text. The zero value
// matches the literal string, case-sensitively. Mirrors the matching half of
// Aspose.PDF for .NET's TextReplaceOptions / TextSearchOptions.
type ReplaceOptions struct {
	// CaseInsensitive folds case when matching.
	CaseInsensitive bool
	// Regex treats the search string as an RE2 regular expression.
	Regex bool
}

func replaceToSearchOptions(opts []ReplaceOptions) SearchOptions {
	if len(opts) == 0 {
		return SearchOptions{}
	}
	o := opts[len(opts)-1]
	return SearchOptions{CaseInsensitive: o.CaseInsensitive, Regex: o.Regex}
}

// ReplaceText replaces every occurrence of old with replacement on the page and
// returns the number of replacements made. The matched glyphs are removed and
// the replacement is drawn at the same baseline, size, and colour in a
// metric-compatible Standard-14 face chosen from the original's family and
// style (so any replacement text renders, even when the original used an
// embedded subset font that lacks the new glyphs). The line is not re-flowed: a
// much longer replacement may overrun following content. Matches are located
// within a single line, like SearchText. Mirrors the find-and-replace idiom of
// Aspose.PDF for .NET's TextFragmentAbsorber + TextFragment.Text.
//
// Removing the old text reuses the SAME content-stream rewriter
// ApplyRedactions does (rewriteTextOperatorsInStream), so it shares that
// function's one hard limit: a match drawn with a font this pipeline
// cannot resolve (an undeclared font name, or a simple font with no
// usable /Encoding/ToUnicode) makes this call return an error instead
// of replacing it — rather than, as before, silently leaving the OLD
// text underneath the new replacement text it still drew on top (a
// real, if cosmetic, bug this change trades for a clear error; found
// during an independent review of pdf-go-oq9i, the ApplyRedactions fix
// this inherited). Also removes the dead, now-unreferenced ORIGINAL
// content stream from the document once at least one match was
// replaced (RemoveUnusedObjects, the same reason ApplyRedactions calls
// it — pdf-go-2qkq's class of leak otherwise applies here too, since
// old text containing whatever was just "replaced" would stay
// recoverable in the saved file) — so, like ApplyRedactions, call
// ReplaceText only after placing everything else you mean to keep.
func (p *Page) ReplaceText(old, replacement string, opts ...ReplaceOptions) (int, error) {
	re, err := compileSearch(old, replaceToSearchOptions(opts))
	if err != nil {
		return 0, err
	}
	n, err := applyReplaceToPage(p, re, replacement)
	if n > 0 {
		p.doc.RemoveUnusedObjects()
	}
	return n, err
}

// ReplaceText replaces every occurrence of old with replacement across all
// pages, returning the total number of replacements. See (*Page).ReplaceText
// for the replacement strategy and limitations.
func (d *Document) ReplaceText(old, replacement string, opts ...ReplaceOptions) (int, error) {
	re, err := compileSearch(old, replaceToSearchOptions(opts))
	if err != nil {
		return 0, err
	}
	total := 0
	for i := range d.pages {
		p := &Page{doc: d, index: i}
		n, err := applyReplaceToPage(p, re, replacement)
		total += n
		if err != nil {
			// As with ApplyRedactions: any EARLIER page that already
			// replaced successfully left its pre-replacement content
			// stream dead but still in d.objects — sweep whatever
			// succeeded before surfacing the error, so it doesn't end
			// up written to the file anyway on the caller's next Save.
			if total > 0 {
				d.RemoveUnusedObjects()
			}
			return total, fmt.Errorf("replace text: page %d: %w", i+1, err)
		}
	}
	if total > 0 {
		d.RemoveUnusedObjects()
	}
	return total, nil
}

// replaceMatch is a located match plus the appearance needed to redraw it.
type replaceMatch struct {
	rect  Rectangle // bounding box of the matched glyphs (for removal)
	x, y  float64   // baseline start position of the replacement
	size  float64   // font size in points
	color Color     // fill colour of the original text
	font  Font      // Standard-14 face matched to the original's family/style
}

func applyReplaceToPage(p *Page, re *regexp.Regexp, replacement string) (int, error) {
	lines, err := p.ExtractTextWithLayout()
	if err != nil {
		return 0, err
	}
	var matches []replaceMatch
	for i := range lines {
		matches = append(matches, collectReplaceMatchesLine(&lines[i], re)...)
	}
	if len(matches) == 0 {
		return 0, nil
	}

	// Remove the old glyphs with the redaction text rewriter (it drops glyphs
	// whose drawn position falls inside the given regions).
	data, err := p.contentStreams()
	if err != nil {
		return 0, err
	}
	if data == nil {
		data = []byte{}
	}
	regions := make([]QuadPoint, 0, len(matches))
	for _, m := range matches {
		regions = append(regions, rectAsQuadPoint(m.rect))
	}
	fontMap := resolveFontResources(p.doc.objects, p.pageResources())
	data, err = rewriteTextOperatorsInStream(data, regions, fontMap, p.doc.objects, p.pageResources(), nil)
	if err != nil {
		return 0, fmt.Errorf("remove matched text: %w", err)
	}

	// Redraw the replacement at each match's baseline (skipped for an empty
	// replacement, which is then just a deletion).
	if replacement != "" {
		var buf strings.Builder
		for _, m := range matches {
			resName, _, encode, _, _, err := p.resolveFontForPage(m.font, m.size)
			if err != nil {
				return 0, err
			}
			buf.WriteString(fmt.Sprintf("\nq\n%s %s %s rg\nBT\n%s %s Tf\n%s %s Td\n%s Tj\nET\nQ\n",
				formatFloat(m.color.R), formatFloat(m.color.G), formatFloat(m.color.B),
				resName, formatFloat(m.size),
				formatFloat(m.x), formatFloat(m.y),
				encode(replacement)))
		}
		data = append(data, []byte(buf.String())...)
	}

	if err := replacePageContents(p, data); err != nil {
		return 0, fmt.Errorf("replace content: %w", err)
	}
	return len(matches), nil
}

// collectReplaceMatchesLine finds matches on one line and captures, for each,
// the removal rect plus the start position/size/colour/font of its first
// fragment (used to redraw the replacement).
func collectReplaceMatchesLine(line *TextLine, re *regexp.Regexp) []replaceMatch {
	if len(line.Fragments) == 0 {
		return nil
	}
	m := buildLineRuneMap(line)
	if len(m.owner) == 0 {
		return nil
	}

	var out []replaceMatch
	for _, loc := range re.FindAllIndex(m.text, -1) {
		b0, b1 := loc[0], loc[1]
		if b0 == b1 {
			continue
		}
		r0 := sort.SearchInts(m.runeByte, b0)
		r1 := sort.SearchInts(m.runeByte, b1)
		rect, ok := matchRect(line.Fragments, m.owner, m.local, m.runeCounts, r0, r1)
		if !ok {
			continue
		}
		// First real (non-inserted-space) rune of the match gives the start.
		fi, li := -1, 0
		for ri := r0; ri < r1 && ri < len(m.owner); ri++ {
			if m.owner[ri] >= 0 {
				fi, li = m.owner[ri], m.local[ri]
				break
			}
		}
		if fi < 0 {
			continue
		}
		f := &line.Fragments[fi]
		x := f.X
		if li >= 0 && li < len(f.runeX) {
			x = f.runeX[li]
		}
		out = append(out, replaceMatch{
			rect:  rect,
			x:     x,
			y:     f.Y,
			size:  f.FontSize,
			color: f.Color,
			font:  standard14ForFragment(f.FontName, f.Bold, f.Italic),
		})
	}
	return out
}

// standard14ForFragment picks the Standard-14 face closest to a fragment's
// font family and style. Serif → Times, monospace → Courier, else Helvetica.
func standard14ForFragment(name string, bold, italic bool) Font {
	n := strings.ToLower(name)
	mono := strings.Contains(n, "courier") || strings.Contains(n, "mono") || strings.Contains(n, "consol")
	serif := strings.Contains(n, "times") || strings.Contains(n, "serif") ||
		strings.Contains(n, "roman") || strings.Contains(n, "georgia") || strings.Contains(n, "minion")
	switch {
	case mono:
		switch {
		case bold && italic:
			return FontCourierBoldOblique
		case bold:
			return FontCourierBold
		case italic:
			return FontCourierOblique
		default:
			return FontCourier
		}
	case serif:
		switch {
		case bold && italic:
			return FontTimesBoldItalic
		case bold:
			return FontTimesBold
		case italic:
			return FontTimesItalic
		default:
			return FontTimesRoman
		}
	default:
		switch {
		case bold && italic:
			return FontHelveticaBoldOblique
		case bold:
			return FontHelveticaBold
		case italic:
			return FontHelveticaOblique
		default:
			return FontHelvetica
		}
	}
}
