// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"math"
	"regexp"
	"unicode/utf8"
)

// documentFont is a font found on a page: its name as text extraction reports
// it. Usable as TextState.Font for reading; drawing with it goes through the
// replacement fallback.
type documentFont struct {
	name     string
	embedded bool
}

func (f documentFont) BaseFont() string { return f.name }
func (f documentFont) IsEmbedded() bool { return f.embedded }

// TextFragmentAbsorber finds text and returns it as TextFragments whose
// segments are runs of one font, size, colour and style. Mirrors Aspose.PDF
// for .NET's TextFragmentAbsorber. An empty phrase finds every line's text.
//
// Each Visit replaces the fragments of the previous one (as
// TextParagraphAbsorber and TableAbsorber do); Reset clears them. A visit
// first applies the document's pending text edits (ApplyTextChanges), so it
// reads the page as it will be saved, and binds every fragment it finds to
// the document so an edit to it can be applied later: a visit changes the
// document and must not run concurrently with other calls on it. Bound
// fragments live as long as the document does.
type TextFragmentAbsorber struct {
	phrase string
	opts   SearchOptions
	frags  []*TextFragment
}

// NewTextFragmentAbsorber returns an absorber for phrase (literal unless
// SearchOptions.Regex; the last options value wins).
func NewTextFragmentAbsorber(phrase string, opts ...SearchOptions) *TextFragmentAbsorber {
	return &TextFragmentAbsorber{phrase: phrase, opts: lastSearchOption(opts)}
}

// TextFragments returns the fragments the last Visit found (a copy of the
// list; the fragments themselves are shared, so editing one edits the page
// on the next ApplyTextChanges or save).
func (a *TextFragmentAbsorber) TextFragments() []*TextFragment {
	return append([]*TextFragment(nil), a.frags...)
}

// Reset clears the fragments found. Mirrors Aspose.PDF for .NET's
// TextFragmentAbsorber.Reset. Edits already made to them still apply.
func (a *TextFragmentAbsorber) Reset() { a.frags = nil }

// Visit finds the phrase on page p, replacing the fragments found before. It
// applies the document's pending text edits first and returns their error.
func (a *TextFragmentAbsorber) Visit(p *Page) error {
	if p == nil {
		return fmt.Errorf("text fragment absorber: nil page")
	}
	if err := p.doc.ApplyTextChanges(); err != nil {
		return err
	}
	return a.visit([]*Page{p})
}

// VisitDocument finds the phrase on every page of d (a regular expression may
// match across pages), replacing the fragments found before. It applies the
// document's pending text edits first and returns their error.
func (a *TextFragmentAbsorber) VisitDocument(d *Document) error {
	if err := d.ApplyTextChanges(); err != nil {
		return err
	}
	return a.visit(d.Pages())
}

func (a *TextFragmentAbsorber) visit(pages []*Page) error {
	var re *regexp.Regexp
	if a.phrase != "" {
		r, err := compileSearch(a.phrase, a.opts)
		if err != nil {
			return err
		}
		re = r
	}
	byNum := map[int]*Page{}
	var all []pageLines
	for _, p := range pages {
		lines, err := p.ExtractTextWithLayout()
		if err != nil {
			return err
		}
		byNum[p.Number()] = p
		all = append(all, pageLines{p.Number(), lines})
	}
	var matches []spanMatch
	switch {
	case re == nil:
		for _, pl := range all {
			for i := range pl.lines {
				line := &pl.lines[i]
				if len(line.Fragments) == 0 {
					continue
				}
				m := buildLineRuneMap(line)
				if len(m.owner) == 0 {
					continue
				}
				matches = append(matches, spanMatch{text: string(m.text),
					spans: []lineSpan{{pl.page, line, m, 0, len(m.runeByte)}}})
			}
		}
	case a.opts.Regex:
		matches = findAcrossLines(all, re)
	default:
		for _, pl := range all {
			for i := range pl.lines {
				matches = append(matches, findAcrossLines([]pageLines{{pl.page, pl.lines[i : i+1]}}, re)...)
			}
		}
	}
	fills := map[int][]shadedRect{}
	var found []*TextFragment
	for _, sm := range matches {
		f := a.build(sm, byNum, fills)
		if f == nil {
			continue
		}
		found = append(found, f)
		f.bind.page.doc.bindFragment(f)
	}
	a.frags = found
	return nil
}

type runKey struct {
	font         string
	size         int
	r, g, b      float64
	bold, italic bool
}

// keyOf is the run key of rune li of fragment f: its look with the rune's
// own fill colour (extraction keeps a colour change inside one fragment, so
// line text stays as it was; segments split there).
func keyOf(f *TextFragment, li int) runKey {
	c := runeColorOf(f, li)
	return runKey{f.FontName, int(math.Round(f.FontSize * 100)), c.R, c.G, c.B, f.Bold, f.Italic}
}

// runeColorOf is the fill colour of rune li of f.
func runeColorOf(f *TextFragment, li int) Color {
	if li >= 0 && li < len(f.runeColor) {
		return f.runeColor[li]
	}
	return f.Color
}

type runGroup struct {
	a, b  int
	key   runKey
	src   *TextFragment
	color Color // the run's fill colour (its first rune's)
	span  *lineSpan
	text  []rune
}

func (a *TextFragmentAbsorber) build(sm spanMatch, byNum map[int]*Page, fills map[int][]shadedRect) *TextFragment {
	if len(sm.spans) == 0 {
		return nil
	}
	pg := byNum[sm.spans[0].page]
	if pg == nil {
		return nil
	}
	f := &TextFragment{Text: sm.text}
	var drawn []QuadPoint
	noEdit := ""
	var union Rectangle
	haveUnion := false
	var firstLine Rectangle
	haveFirst := false
	for si := range sm.spans {
		sp := &sm.spans[si]
		frags := sp.line.Fragments
		var groups []*runGroup
		var cur *runGroup
		pending := -1
		var pendText []rune
		for k := sp.r0; k < sp.r1; k++ {
			r, _ := utf8.DecodeRune(sp.m.text[sp.m.runeByte[k]:])
			fi := sp.m.owner[k]
			if fi < 0 {
				if cur != nil {
					cur.b = k + 1
					cur.text = append(cur.text, r)
				} else {
					if pending < 0 {
						pending = k
					}
					pendText = append(pendText, r)
				}
				continue
			}
			src := &frags[fi]
			key := keyOf(src, sp.m.local[k])
			if cur == nil || cur.key != key {
				start := k
				text := []rune(nil)
				if pending >= 0 {
					start, text = pending, pendText
					pending, pendText = -1, nil
				}
				cur = &runGroup{a: start, key: key, src: src, color: runeColorOf(src, sp.m.local[k]), span: sp, text: text}
				cur.text = append(cur.text, r)
				groups = append(groups, cur)
				cur.b = k + 1
				continue
			}
			cur.b = k + 1
			cur.text = append(cur.text, r)
		}
		for _, g := range groups {
			rect, ok := matchRect(frags, sp.m.owner, sp.m.local, sp.m.runeCounts, g.a, g.b)
			if !ok {
				continue
			}
			if math.Abs(g.src.Rotation) > 1 && noEdit == "" {
				noEdit = fmt.Sprintf("text on a rotated baseline (%.0f degrees)", g.src.Rotation)
			}
			// The run's visual left edge on its baseline, for either
			// direction of text.
			x := rect.LLX
			// A negative Tf size (or Tz) mirrors the glyphs and reports a
			// negative effective size; the state carries its magnitude,
			// as a size to draw with must be positive, so a mirrored run
			// is redrawn upright when edited.
			st := TextState{
				Font:            documentFont{name: cleanFontName(g.src.FontName), embedded: fontEmbedded(pg, g.src.FontName)},
				FontSize:        math.Abs(g.src.FontSize),
				ForegroundColor: copyColor(&g.color),
				Rotation:        g.src.Rotation,
			}
			if g.src.Bold {
				st.FontStyle |= FontStyleBold
			}
			if g.src.Italic {
				st.FontStyle |= FontStyleItalic
			}
			if a.opts.SearchForTextRelatedGraphics {
				ff, ok := fills[sp.page]
				if !ok {
					ff = pageFills(byNum[sp.page])
					fills[sp.page] = ff
				}
				st.BackgroundColor = backgroundAt(ff, (rect.LLX+rect.URX)/2, (rect.LLY+rect.URY)/2)
			}
			seg := &TextSegment{Text: string(g.text), TextState: st,
				Position: Position{XIndent: x, YIndent: g.src.Y}, Rectangle: rect}
			if len(f.Segments) == 0 {
				src := *g.src
				f.X, f.Y, f.FontName, f.FontSize, f.Height = x, src.Y, src.FontName, src.FontSize, src.Height
				f.Bold, f.Italic, f.CharSpacing, f.Color = src.Bold, src.Italic, src.CharSpacing, g.color
				f.IsSubscript, f.IsSuperscript, f.Rotation = src.IsSubscript, src.IsSuperscript, src.Rotation
				f.descent = src.descent
			}
			if si == 0 {
				if !haveFirst {
					firstLine, haveFirst = rect, true
				} else {
					firstLine = unionRect(firstLine, rect)
				}
			}
			f.Segments = append(f.Segments, seg)
			drawn = append(drawn, QuadPoint{X1: rect.LLX, Y1: rect.URY, X2: rect.URX, Y2: rect.URY,
				X3: rect.LLX, Y3: rect.LLY, X4: rect.URX, Y4: rect.LLY})
			if !haveUnion {
				union, haveUnion = rect, true
			} else {
				union = unionRect(union, rect)
			}
		}
	}
	if len(f.Segments) == 0 {
		return nil
	}
	if a.opts.Rectangle != nil {
		hit := rectsIntersect(union, *a.opts.Rectangle)
		for _, s := range f.Segments {
			hit = hit || rectsIntersect(s.Rectangle, *a.opts.Rectangle)
		}
		if !hit {
			return nil
		}
	}
	f.Width = firstLine.URX - firstLine.LLX
	f.TextState = copyState(f.Segments[0].TextState)
	f.Position = f.Segments[0].Position
	for _, sp := range sm.spans[1:] {
		if sp.page != sm.spans[0].page && noEdit == "" {
			noEdit = "match spans several pages"
		}
	}
	f.bind = &fragmentBinding{page: pg, drawn: drawn, noEdit: noEdit, found: true}
	f.bind.snap = snapshotOf(f)
	return f
}

// fontEmbedded reports whether the page's font resource named by the
// extraction's font name carries an embedded program (FontFile, FontFile2 or
// FontFile3 on the descriptor, or on the Type0 descendant's). A font the page
// resources do not name (one only a Form XObject uses) falls back to "not a
// Standard-14 name".
func fontEmbedded(p *Page, name string) bool {
	want := cleanFontName(name)
	objs := p.doc.objects
	if fonts, ok := resolveRefToDict(objs, p.pageResources()["/Font"]); ok {
		for _, v := range fonts {
			fd, ok := resolveRefToDict(objs, v)
			if !ok {
				continue
			}
			if bf, ok := resolveRef(objs, fd["/BaseFont"]).(pdfName); !ok || cleanFontName(string(bf)) != want {
				continue
			}
			return descriptorEmbedded(objs, fd)
		}
	}
	_, _, std := standard14VerticalMetrics("/" + want)
	return !std
}

func descriptorEmbedded(objs map[int]*pdfObject, fd pdfDict) bool {
	if arr, ok := resolveRefToArray(objs, fd["/DescendantFonts"]); ok && len(arr) > 0 {
		if cd, ok := resolveRefToDict(objs, arr[0]); ok {
			return descriptorEmbedded(objs, cd)
		}
	}
	desc, ok := resolveRefToDict(objs, fd["/FontDescriptor"])
	if !ok {
		return false
	}
	for _, k := range []string{"/FontFile", "/FontFile2", "/FontFile3"} {
		if _, ok := desc[k]; ok {
			return true
		}
	}
	return false
}
