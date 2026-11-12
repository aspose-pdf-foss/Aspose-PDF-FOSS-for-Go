// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// ReplaceOptions tunes how ReplaceText matches the search text and lays out
// the replacement. The zero value matches the literal string,
// case-sensitively, and leaves the rest of the line where it is. Mirrors
// Aspose.PDF for .NET's TextReplaceOptions / TextSearchOptions.
type ReplaceOptions struct {
	// CaseInsensitive folds case when matching.
	CaseInsensitive bool
	// Regex treats the search string as an RE2 regular expression.
	Regex bool
	// Adjustment says what happens to the text after a replacement of a
	// different width. ReplaceAdjustmentNone (the zero value) leaves it in
	// place; ReplaceAdjustmentShiftRestOfLine moves it.
	Adjustment ReplaceAdjustment
	// Font draws the replacement. Nil chooses one per match: the
	// Standard-14 face matching the original's family and style when it
	// covers every character of the replacement, else the original's family
	// through the FontRepository and the installed fonts (embedded), as
	// Aspose.PDF for .NET substitutes a font that has the glyphs. With no
	// font covering it the call fails, naming the character.
	Font Font

	// Rectangle lays the replacement out as a paragraph in this rectangle
	// instead of on the match's line (pdf-go-2tdw): it is wrapped at whole
	// words to the rectangle's width, its first line's text box touching
	// the rectangle's top, in the match's font, size and line pitch. Text
	// longer than the rectangle runs on below it unless FontSizeAdjustment
	// says otherwise. The same rectangle is used for every match, so this
	// is meant for a call replacing one. Mirrors Aspose.PDF for .NET's
	// TextReplaceOptions.Rectangle.
	//
	// A paragraph layout is also chosen, in the match's own rectangle (the
	// union of its lines; for a match on one line, widened to the page's
	// right edge less the margin left of the match), by FontSizeAdjustment, LeftAdjustment,
	// RightAdjustment, or an Adjustment of ReplaceAdjustmentAdjustSpaceWidth
	// or ReplaceAdjustmentWholeWordsHyphenation.
	Rectangle *Rectangle
	// FontSizeAdjustment changes the font size of a paragraph layout to fit
	// its rectangle: ShrinkToFit makes it smaller when the text would run
	// on below, ScaleToFill makes it the largest that fits. Mirrors
	// TextReplaceOptions.FontSizeAdjustmentAction.
	FontSizeAdjustment FontSizeAdjustment
	// LeftAdjustment and RightAdjustment move the left and the right edge of
	// a paragraph layout's rectangle, in points (a negative LeftAdjustment
	// and a positive RightAdjustment widen it). Mirror
	// TextReplaceOptions.LeftAdjustment and RightAdjustment.
	LeftAdjustment, RightAdjustment float64
}

// FontSizeAdjustment selects how a replacement laid out in a rectangle
// changes size to fit it. Mirrors Aspose.PDF for .NET's
// TextReplaceOptions.FontSizeAdjustment.
type FontSizeAdjustment int

const (
	// FontSizeAdjustmentNone keeps the match's font size; a longer text runs
	// on below the rectangle.
	FontSizeAdjustmentNone FontSizeAdjustment = iota
	// FontSizeAdjustmentShrinkToFit lowers the size until the text fits
	// the rectangle's height, and leaves it when it fits already.
	FontSizeAdjustmentShrinkToFit
	// FontSizeAdjustmentScaleToFill sets the largest size at which the text
	// fits the rectangle, larger or smaller than the match's.
	FontSizeAdjustmentScaleToFill
)

// ReplaceAdjustment selects how ReplaceText makes room for (or closes the
// gap after) a replacement whose width differs from the text it replaces.
// Mirrors Aspose.PDF for .NET's TextReplaceOptions.ReplaceAdjustment; note
// Aspose's default is ShiftRestOfLine, while the zero value here keeps the
// line as it is.
type ReplaceAdjustment int

const (
	// ReplaceAdjustmentNone leaves the rest of the line in place: a longer
	// replacement may overlap the text after it.
	ReplaceAdjustmentNone ReplaceAdjustment = 0
	// ReplaceAdjustmentAdjustSpaceWidth lays the replacement out as a
	// paragraph (see ReplaceOptions.Rectangle) and justifies it: every line
	// but a paragraph's last is spread to the rectangle's width by the
	// space between its words.
	ReplaceAdjustmentAdjustSpaceWidth ReplaceAdjustment = 1
	// ReplaceAdjustmentWholeWordsHyphenation lays the replacement out as a
	// paragraph wrapped at whole words (see ReplaceOptions.Rectangle); a
	// word longer than a line is broken. It may be combined with
	// ReplaceAdjustmentAdjustSpaceWidth (|).
	ReplaceAdjustmentWholeWordsHyphenation ReplaceAdjustment = 2
	// ReplaceAdjustmentShiftRestOfLine moves the text after the
	// replacement on the same line by the difference in width, so nothing
	// overlaps and no gap is left; the line gets longer or shorter. On a
	// right-to-left line that text lies to the left and moves left or right
	// while the replacement keeps the match's right edge. Text on other
	// lines, and graphics, do not move.
	ReplaceAdjustmentShiftRestOfLine ReplaceAdjustment = 8
)

func replaceToSearchOptions(opts []ReplaceOptions) SearchOptions {
	if len(opts) == 0 {
		return SearchOptions{}
	}
	o := opts[len(opts)-1]
	return SearchOptions{CaseInsensitive: o.CaseInsensitive, Regex: o.Regex}
}

func replaceAdjustment(opts []ReplaceOptions) (ReplaceAdjustment, *replaceLayout, error) {
	if len(opts) == 0 {
		return ReplaceAdjustmentNone, nil, nil
	}
	o := opts[len(opts)-1]
	a := o.Adjustment
	paraBits := ReplaceAdjustmentAdjustSpaceWidth | ReplaceAdjustmentWholeWordsHyphenation
	if a != ReplaceAdjustmentShiftRestOfLine && a&^paraBits != 0 {
		return 0, nil, fmt.Errorf("replace text: adjustment %d is not supported", a)
	}
	if o.FontSizeAdjustment < FontSizeAdjustmentNone || o.FontSizeAdjustment > FontSizeAdjustmentScaleToFill {
		return 0, nil, fmt.Errorf("replace text: font size adjustment %d is not supported", o.FontSizeAdjustment)
	}
	if o.Rectangle == nil && o.FontSizeAdjustment == FontSizeAdjustmentNone && a&paraBits == 0 &&
		o.LeftAdjustment == 0 && o.RightAdjustment == 0 {
		return a, nil, nil
	}
	if a == ReplaceAdjustmentShiftRestOfLine {
		return 0, nil, fmt.Errorf("replace text: ReplaceAdjustmentShiftRestOfLine does not apply to a replacement laid out in a rectangle")
	}
	if o.Rectangle != nil {
		if err := o.Rectangle.validate(); err != nil {
			return 0, nil, fmt.Errorf("replace text: rectangle: %w", err)
		}
	}
	for _, v := range []float64{o.LeftAdjustment, o.RightAdjustment} {
		if nonFinite(v) {
			return 0, nil, fmt.Errorf("replace text: edge adjustment %v is not a finite number", v)
		}
	}
	return ReplaceAdjustmentNone, &replaceLayout{
		rect:    o.Rectangle,
		fit:     o.FontSizeAdjustment,
		justify: a&ReplaceAdjustmentAdjustSpaceWidth != 0,
		left:    o.LeftAdjustment,
		right:   o.RightAdjustment,
	}, nil
}

// ReplaceText replaces every occurrence of old with replacement on the page and
// returns the number of replacements made. The matched glyphs are removed and
// the replacement is drawn at the same baseline, size, and colour in a
// metric-compatible Standard-14 face chosen from the original's family and
// style (so it renders even when the original used an embedded subset font
// that lacks the new glyphs). A replacement that face cannot show, such as
// Cyrillic or Arabic, is drawn in ReplaceOptions.Font, else in the original's
// family found by name (see ReplaceOptions.Font); with neither the call fails
// and nothing is changed. The replacement is shaped and, when right-to-left,
// ends where the match ended. By default the line is not
// re-flowed: a much longer replacement may overrun following content; with
// ReplaceOptions.Adjustment = ReplaceAdjustmentShiftRestOfLine the text after
// each replacement on its line moves by the difference in width instead (the
// paragraph is not re-wrapped, so a line can run past its margin); with
// ReplaceOptions.Rectangle (or the other paragraph options) the replacement
// is laid out as a wrapped paragraph instead. Matches are located
// within a single line, like SearchText. Mirrors the find-and-replace idiom of
// Aspose.PDF for .NET's TextFragmentAbsorber + TextFragment.Text.
//
// Text drawn inside a Form XObject is replaced as well: the form is copied
// for this page with only the matched glyphs removed (its images and paths
// stay), so other placements of the same form are left unchanged. The
// ShiftRestOfLine adjustment moves page-level text only.
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
	adj, lay, err := replaceAdjustment(opts)
	if err != nil {
		return 0, err
	}
	n, err := applyReplaceToPage(p, re, replacement, adj, lay, newReplaceFonts(opts), replaceToSearchOptions(opts).Regex)
	if n > 0 {
		p.doc.sweepUnused()
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
	adj, lay, err := replaceAdjustment(opts)
	if err != nil {
		return 0, err
	}
	total := 0
	fonts := newReplaceFonts(opts)
	for i := range d.pages {
		p := &Page{doc: d, index: i}
		n, err := applyReplaceToPage(p, re, replacement, adj, lay, fonts, replaceToSearchOptions(opts).Regex)
		total += n
		if err != nil {
			// As with ApplyRedactions: any EARLIER page that already
			// replaced successfully left its pre-replacement content
			// stream dead but still in d.objects — sweep whatever
			// succeeded before surfacing the error, so it doesn't end
			// up written to the file anyway on the caller's next Save.
			if total > 0 {
				d.sweepUnused()
			}
			return total, fmt.Errorf("replace text: page %d: %w", i+1, err)
		}
	}
	if total > 0 {
		d.sweepUnused()
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

	fontName     string // the original's font, for a covering substitute
	bold, italic bool
	rtl          bool // the line reads right to left: what follows lies left

	// more holds the boxes of a match's later lines (a regular expression
	// matching across a line break, pdf-go-1kcx): their text is removed,
	// the replacement goes on the first line.
	more []Rectangle
	// pitch is the distance between the match's first and second lines'
	// baselines (0 for a match on one line).
	pitch float64
}

// replaceFonts picks the font each replacement is drawn in (ReplaceOptions.Font)
// and keeps the faces loaded by name, so each is embedded once per call.
type replaceFonts struct {
	explicit Font
	byName   map[string]Font // "" value: nothing covering was found
}

func newReplaceFonts(opts []ReplaceOptions) *replaceFonts {
	rf := &replaceFonts{byName: map[string]Font{}}
	for _, o := range opts {
		if o.Font != nil {
			rf.explicit = o.Font
		}
	}
	return rf
}

// firstUncovered returns the first character of text font has no glyph for
// (default-ignorables draw nothing and are never missing), or 0.
func firstUncovered(font Font, text string) rune {
	for _, r := range text {
		if !isDefaultIgnorable(r) && !fontHasGlyph(font, r) {
			return r
		}
	}
	return 0
}

// fontFor returns the font to draw replacement in for m (pdf-go-o4py): until
// then the Standard-14 face was used whatever the replacement, and a character
// outside its Latin encoding was written as '?'.
func (rf *replaceFonts) fontFor(d *Document, m replaceMatch, replacement string) (Font, error) {
	if rf.explicit != nil {
		return rf.explicit, nil
	}
	missing := firstUncovered(m.font, replacement)
	if missing == 0 {
		return m.font, nil
	}
	name := cleanFontName(m.fontName)
	key := fmt.Sprintf("%s|%v|%v", name, m.bold, m.italic)
	f, seen := rf.byName[key]
	if !seen {
		if name != "" {
			if loaded, err := d.LoadFontByName(name, m.bold, m.italic); err == nil {
				f = loaded
			}
		}
		rf.byName[key] = f
	}
	if f != nil {
		if r := firstUncovered(f, replacement); r == 0 {
			return f, nil
		} else {
			missing = r
		}
	}
	return nil, fmt.Errorf("replacement character %q (%U) is not in %s, and no installed or registered font of the original's family %q has it; set ReplaceOptions.Font", missing, missing, m.font.BaseFont(), name)
}

func applyReplaceToPage(p *Page, re *regexp.Regexp, replacement string, adj ReplaceAdjustment, lay *replaceLayout, fonts *replaceFonts, acrossLines bool) (int, error) {
	lines, err := p.ExtractTextWithLayout()
	if err != nil {
		return 0, err
	}
	var matches []replaceMatch
	if acrossLines {
		matches = collectReplaceMatchesAcrossLines(p.Number(), lines, re)
	} else {
		for i := range lines {
			matches = append(matches, collectReplaceMatchesLine(&lines[i], re)...)
		}
	}
	if len(matches) == 0 {
		return 0, nil
	}
	// Choose every replacement's font before anything changes, so a
	// replacement no font can show leaves the page as it was.
	if replacement != "" {
		for i := range matches {
			f, err := fonts.fontFor(p.doc, matches[i], replacement)
			if err != nil {
				return 0, err
			}
			matches[i].font = f
		}
	}

	// Remove the old glyphs with the redaction text rewriter (it drops glyphs
	// whose drawn position falls inside the given regions).
	regions := make([]QuadPoint, 0, len(matches))
	for _, m := range matches {
		regions = append(regions, rectAsQuadPoint(m.rect))
		for _, r := range m.more {
			regions = append(regions, rectAsQuadPoint(r))
		}
	}
	data, err := stripTextInRegions(p, regions)
	if err != nil {
		return 0, err
	}

	// ShiftRestOfLine: move what follows each match on its line by the
	// width difference, and each later replacement on that line with it.
	if adj == ReplaceAdjustmentShiftRestOfLine {
		shifts := make([]lineShift, 0, len(matches))
		for _, m := range matches {
			newW := 0.0
			if replacement != "" {
				w, err := MeasureText(replacement, TextStyle{Font: m.font, Size: m.size})
				if err != nil {
					return 0, err
				}
				newW = w
			}
			if m.rtl {
				// The rest of a right-to-left line lies left of the match,
				// which keeps its right edge (pdf-go-59bb).
				shifts = append(shifts, lineShift{y: m.y, tol: 0.3 * m.size, fromX: m.rect.LLX, dx: (m.rect.URX - m.rect.LLX) - newW, rtl: true})
				continue
			}
			shifts = append(shifts, lineShift{y: m.y, tol: 0.3 * m.size, fromX: m.rect.URX, dx: newW - (m.rect.URX - m.x)})
		}
		for i := range matches {
			matches[i].x += lineShiftAt(shifts, matches[i].x, matches[i].y)
			matches[i].rect.URX += lineShiftAt(shifts, matches[i].rect.URX, matches[i].y)
		}
		data, err = shiftLineText(data, p.doc.objects, p.pageResources(), shifts)
		if err != nil {
			return 0, fmt.Errorf("shift rest of line: %w", err)
		}
	}

	if err := replacePageContents(p, data); err != nil {
		return 0, fmt.Errorf("replace content: %w", err)
	}

	// Redraw the replacement at each match's baseline (skipped for an empty
	// replacement, which is then just a deletion). AddTextAt shapes it and
	// orders it for display; a right-to-left replacement ends where the
	// match ended.
	if replacement != "" {
		for _, m := range matches {
			if lay != nil {
				if err := drawReplacementParagraph(p, m, replacement, lay); err != nil {
					return 0, fmt.Errorf("draw replacement: %w", err)
				}
				continue
			}
			style := TextStyle{Font: m.font, Size: m.size, Color: &Color{R: m.color.R, G: m.color.G, B: m.color.B, A: 1}}
			x := m.x
			if m.rtl || bidiBaseLevel(replacement, false) == 1 {
				style.HAlign, x = HAlignRight, m.rect.URX
			}
			if err := p.AddTextAt(replacement, style, x, m.y); err != nil {
				return 0, fmt.Errorf("draw replacement: %w", err)
			}
		}
	}
	return len(matches), nil
}

// stripTextInRegions returns the page's content with the glyphs drawn inside
// regions removed, in Form XObjects too (pdf-go-hxi2): the redaction Do
// rewriter, limited to text, clones each form a region reaches into and
// redirects that one placement to the clone. That adds resource names, so
// the page gets its own copy of /Resources first, as ApplyRedactions does
// (another page may share them). The caller installs the content.
func stripTextInRegions(p *Page, regions []QuadPoint) ([]byte, error) {
	data, err := p.contentStreams()
	if err != nil {
		return nil, err
	}
	if data == nil {
		data = []byte{}
	}
	resources := privateXObjectResources(p.doc.objects, p.pageResources())
	p.pageDict()["/Resources"] = resources
	fontMap := resolveFontResources(p.doc.objects, resources)
	data, err = rewriteTextOperatorsInStream(data, regions, fontMap, p.doc.objects, resources, nil)
	if err != nil {
		return nil, fmt.Errorf("remove matched text: %w", err)
	}
	budget := &formRedactBudget{textOnly: true}
	preserve := pagePreservedXObjectNames(p, p.doc.objects, resources, budget)
	data, err = rewriteXObjectDoOperatorsInStream(data, regions, p.doc, resources, 0, budget, preserve, nil)
	if err != nil {
		return nil, fmt.Errorf("remove matched text in form: %w", err)
	}
	return data, nil
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
	rtlLine := bidiBaseLevel(line.Text, false) == 1

	var out []replaceMatch
	for _, loc := range re.FindAllIndex(m.text, -1) {
		b0, b1 := loc[0], loc[1]
		if b0 == b1 {
			continue
		}
		r0 := sort.SearchInts(m.runeByte, b0)
		r1 := sort.SearchInts(m.runeByte, b1)
		if rm, ok := replaceMatchAt(line, m, r0, r1, rtlLine); ok {
			out = append(out, rm)
		}
	}
	return out
}

// collectReplaceMatchesAcrossLines finds the matches of a regular expression
// over the page's lines joined by "\n" (pdf-go-1kcx): each is replaced on the
// line it starts on, and its text on later lines is removed.
func collectReplaceMatchesAcrossLines(page int, lines []TextLine, re *regexp.Regexp) []replaceMatch {
	var out []replaceMatch
	for _, sm := range findAcrossLines([]pageLines{{page, lines}}, re) {
		first := sm.spans[0]
		rm, ok := replaceMatchAt(first.line, first.m, first.r0, first.r1, bidiBaseLevel(first.line.Text, false) == 1)
		if !ok {
			continue
		}
		if n := len(sm.spans); n > 1 {
			rm.pitch = (first.line.Y - sm.spans[n-1].line.Y) / float64(n-1)
		}
		for _, sp := range sm.spans[1:] {
			if r, ok := matchRect(sp.line.Fragments, sp.m.owner, sp.m.local, sp.m.runeCounts, sp.r0, sp.r1); ok {
				rm.more = append(rm.more, r)
			}
		}
		out = append(out, rm)
	}
	return out
}

// replaceMatchAt describes the match of runes [r0, r1) of a line's rune map:
// its box, and the start position, size, colour and font of its first glyph.
func replaceMatchAt(line *TextLine, m lineRuneMap, r0, r1 int, rtlLine bool) (replaceMatch, bool) {
	rect, ok := matchRect(line.Fragments, m.owner, m.local, m.runeCounts, r0, r1)
	if !ok {
		return replaceMatch{}, false
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
		return replaceMatch{}, false
	}
	f := &line.Fragments[fi]
	x := f.X
	if li >= 0 && li < len(f.runeX) {
		x = f.runeX[li]
	}
	return replaceMatch{
		rect:  rect,
		x:     x,
		y:     f.Y,
		size:  f.FontSize,
		color: f.Color,
		font:  standard14ForFragment(f.FontName, f.Bold, f.Italic),

		fontName: f.FontName,
		bold:     f.Bold,
		italic:   f.Italic,
		rtl:      rtlLine,
	}, true
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

// lineShift moves text on one line: glyphs whose baseline is within tol of
// y and that start at or after fromX move by dx along the line.
type lineShift struct {
	y, tol, fromX, dx float64
	rtl               bool    // moves what lies left of fromX instead of right
	toX               float64 // when non-zero, glyphs right of it are not moved
}

// lineShiftAt is the total shift for a glyph at (x, y).
func lineShiftAt(shifts []lineShift, x, y float64) float64 {
	d := 0.0
	for _, s := range shifts {
		if math.Abs(y-s.y) > s.tol {
			continue
		}
		if s.toX != 0 && x > s.toX {
			continue
		}
		if (!s.rtl && x >= s.fromX-0.01) || (s.rtl && x <= s.fromX+0.01) {
			d += s.dx
		}
	}
	return d
}

// shiftLineText moves glyphs by the given line shifts by inserting TJ
// kerning numbers in front of them. That is exact: a TJ number moves every
// glyph after it, and the shift already applied is tracked through the
// text object until the next operator that sets the position afresh (BT,
// Td, TD, Tm, T*, ' and "). A show operator that needs a shift becomes a
// TJ. Glyphs on a rotated or skewed baseline are left alone.
func shiftLineText(data []byte, objects map[int]*pdfObject, resources pdfDict, shifts []lineShift) ([]byte, error) {
	items, err := atWalk(data, objects, resources)
	if err != nil {
		return nil, err
	}
	var out []contentOp
	applied := 0.0
	for _, it := range items {
		if len(it.ops) == 1 {
			switch it.ops[0].Operator {
			case "BT", "Td", "TD", "Tm", "T*":
				applied = 0
			}
		}
		if it.kind != atText {
			out = append(out, it.ops...)
			continue
		}
		if it.textOp == "'" || it.textOp == "\"" {
			applied = 0
		}
		if !it.horizontal || it.kernUnit == 0 {
			out = append(out, it.ops...)
			continue
		}
		var arr pdfArray
		changed := false
		for _, t := range it.toks {
			if !t.glyph {
				arr = append(arr, t.kern)
				continue
			}
			if want := lineShiftAt(shifts, t.x, t.y); math.Abs(want-applied) > 1e-6 {
				arr = append(arr, -(want-applied)/it.kernUnit)
				applied = want
				changed = true
			}
			if n := len(arr); n > 0 {
				if str, ok := arr[n-1].(string); ok {
					arr[n-1] = str + t.raw
					continue
				}
			}
			arr = append(arr, t.raw)
		}
		if !changed {
			out = append(out, it.ops...)
			continue
		}
		out = append(out, it.pre...)
		out = append(out, contentOp{Operator: "TJ", Operands: []pdfValue{arr}})
	}
	return serializeContentOps(out), nil
}
