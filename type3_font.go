// SPDX-License-Identifier: MIT

package asposepdf

import (
	"fmt"
	"sort"
	"strings"
)

// Type3 font authoring (epic pdf-go-nul, write side): define custom glyphs
// with the full vector drawing API and use the result as a TextStyle.Font.
// A Type3 font's glyphs are small PDF content streams (ISO 32000-1 §9.6.5)
// rather than TrueType/CFF outlines — the classic vehicle for custom
// symbols, ornaments and math marks. The read side (extraction + rendering
// of existing Type3 fonts) has long shipped; this is the authoring
// counterpart, a capability Aspose.PDF for .NET does not expose.
//
//	t3 := doc.CreateType3Font()
//	star, _ := t3.AddGlyph('★', 900)         // canvas in a 1000-unit em square
//	star.DrawPath(...)                        // any Draw*/AddImage ops
//	page.AddText("★ rating", pdf.TextStyle{Font: t3, Size: 24}, rect)
//
// Glyph canvases use glyph space: 1000 units per em, baseline at y=0 (so
// descenders go below zero; the canvas MediaBox spans [0,-300..1000,1000]).
// The font embeds a /ToUnicode CMap and uniXXXX glyph names, so text drawn
// with it stays extractable and searchable. Runes missing from the font are
// skipped when drawing. At most 255 glyphs per font (single-byte codes).

// Type3Font is a user-defined glyph-stream font. Create with
// Document.CreateType3Font, add glyphs, then use as TextStyle.Font.
type Type3Font struct {
	doc    *Document
	glyphs []*type3Glyph
	byRune map[rune]*type3Glyph
	fontID int // font dict object number once built (0 = not built)
}

type type3Glyph struct {
	r      rune
	code   byte
	width  float64 // advance in 1/1000 em
	canvas *Page
}

// CreateType3Font returns an empty Type3 font bound to the document.
func (d *Document) CreateType3Font() *Type3Font {
	return &Type3Font{doc: d, byRune: map[rune]*type3Glyph{}}
}

// BaseFont implements Font.
func (t *Type3Font) BaseFont() string { return "Type3" }

// IsEmbedded implements Font (glyph streams are embedded by construction).
func (t *Type3Font) IsEmbedded() bool { return true }

// AddGlyph registers a glyph for r with the given advance width (in 1/1000
// em, like all font metrics) and returns its drawing canvas: a detached page
// covering the em square (x 0..1000, y -300..1000, baseline at y=0) on which
// the whole drawing API works. Draw before the first AddText using the font
// — glyph streams are frozen when the font is first used. Errors on a
// duplicate rune, a non-positive width, a font already frozen, or a full
// font (255 glyphs).
func (t *Type3Font) AddGlyph(r rune, width float64) (*Page, error) {
	if t.fontID != 0 {
		return nil, fmt.Errorf("AddGlyph: font already frozen by first use")
	}
	if width <= 0 {
		return nil, fmt.Errorf("AddGlyph: width must be positive, got %g", width)
	}
	if _, dup := t.byRune[r]; dup {
		return nil, fmt.Errorf("AddGlyph: rune %q already defined", r)
	}
	if len(t.glyphs) >= 255 {
		return nil, fmt.Errorf("AddGlyph: a Type3 font holds at most 255 glyphs")
	}
	canvasDict := pdfDict{
		"/Type":      pdfName("/Page"),
		"/MediaBox":  pdfArray{0.0, -300.0, 1000.0, 1000.0},
		"/Resources": pdfDict{},
	}
	g := &type3Glyph{
		r:      r,
		code:   byte(len(t.glyphs) + 1), // codes 1..255; 0 stays unused
		width:  width,
		canvas: &Page{doc: t.doc, obj: &pdfObject{Value: canvasDict}},
	}
	t.glyphs = append(t.glyphs, g)
	t.byRune[r] = g
	return g.canvas, nil
}

// glyphName returns the uniXXXX AGL-style name, so extractors that resolve
// /Differences names recover the rune even without /ToUnicode.
func (g *type3Glyph) glyphName() string {
	if g.r <= 0xFFFF {
		return fmt.Sprintf("uni%04X", g.r)
	}
	return fmt.Sprintf("u%06X", g.r)
}

// ensureBuilt freezes the font: harvests every glyph canvas into a
// /CharProcs stream, merges canvas resources into the font /Resources, and
// writes the font dict (+ /ToUnicode). Returns the font object number.
func (t *Type3Font) ensureBuilt() (int, error) {
	if t.fontID != 0 {
		return t.fontID, nil
	}
	if len(t.glyphs) == 0 {
		return 0, fmt.Errorf("type3 font: no glyphs defined")
	}

	charProcs := pdfDict{}
	fontRes := pdfDict{}
	maxCode := byte(0)
	for _, g := range t.glyphs {
		content, err := g.canvas.contentStreams()
		if err != nil {
			return 0, fmt.Errorf("type3 glyph %q: %w", g.r, err)
		}
		// d0 declares the advance; it must precede any drawing operator
		// (ISO 32000-1 §9.6.5, Table 113).
		proc := fmt.Sprintf("%s 0 d0\n", trimFloat(g.width)) + string(content)
		stream := &pdfStream{Dict: pdfDict{}, Data: []byte(proc), Decoded: true}
		charProcs["/"+g.glyphName()] = pdfRef{Num: t.doc.addObject(stream)}
		mergeResourceDict(fontRes, g.canvas.pageResources())
		// Free this glyph's canvas's own content-stream objects now that
		// their bytes are copied into the CharProc stream above, and
		// reject any further drawing on it (pdf-go-nvpo / GitHub issue
		// #17 point 1).
		freezeDetachedCanvas(g.canvas)
		if g.code > maxCode {
			maxCode = g.code
		}
	}

	// /Encoding /Differences: [ firstCode /name /name ... ] — codes are
	// consecutive by construction.
	diffs := pdfArray{float64(t.glyphs[0].code)}
	for _, g := range t.glyphs {
		diffs = append(diffs, pdfName("/"+g.glyphName()))
	}
	widths := pdfArray{}
	for _, g := range t.glyphs {
		widths = append(widths, g.width)
	}

	fontDict := pdfDict{
		"/Type":       pdfName("/Font"),
		"/Subtype":    pdfName("/Type3"),
		"/FontBBox":   pdfArray{0.0, -300.0, 1000.0, 1000.0},
		"/FontMatrix": pdfArray{0.001, 0.0, 0.0, 0.001, 0.0, 0.0},
		"/CharProcs":  charProcs,
		"/Encoding": pdfDict{
			"/Type":        pdfName("/Encoding"),
			"/Differences": diffs,
		},
		"/FirstChar": float64(t.glyphs[0].code),
		"/LastChar":  float64(maxCode),
		"/Widths":    widths,
		"/ToUnicode": t.buildToUnicode(),
	}
	if len(fontRes) > 0 {
		fontDict["/Resources"] = fontRes
	}
	t.fontID = t.doc.addObject(fontDict)
	return t.fontID, nil
}

// buildToUnicode writes a minimal bfchar CMap covering the glyph codes.
func (t *Type3Font) buildToUnicode() pdfRef {
	var b strings.Builder
	b.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n")
	b.WriteString("/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n")
	b.WriteString("/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n")
	b.WriteString("1 begincodespacerange\n<00> <FF>\nendcodespacerange\n")
	fmt.Fprintf(&b, "%d beginbfchar\n", len(t.glyphs))
	sorted := append([]*type3Glyph(nil), t.glyphs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].code < sorted[j].code })
	for _, g := range sorted {
		if g.r <= 0xFFFF {
			fmt.Fprintf(&b, "<%02X> <%04X>\n", g.code, g.r)
		} else {
			r1, r2 := utf16Surrogates(g.r)
			fmt.Fprintf(&b, "<%02X> <%04X%04X>\n", g.code, r1, r2)
		}
	}
	b.WriteString("endbfchar\nendcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n")
	stream := &pdfStream{Dict: pdfDict{}, Data: []byte(b.String()), Decoded: true}
	return pdfRef{Num: t.doc.addObject(stream)}
}

// utf16Surrogates splits a supplementary-plane rune into a UTF-16 pair.
func utf16Surrogates(r rune) (uint16, uint16) {
	v := uint32(r) - 0x10000
	return uint16(0xD800 + (v >> 10)), uint16(0xDC00 + (v & 0x3FF))
}

// mergeResourceDict shallow-merges src's resource categories into dst
// (later glyphs win on a same-name collision; canvases get distinct names
// from their own registrars, so collisions only occur for identical refs).
func mergeResourceDict(dst, src pdfDict) {
	for cat, v := range src {
		sub, ok := v.(pdfDict)
		if !ok {
			continue
		}
		dsub, ok := dst[cat].(pdfDict)
		if !ok {
			dsub = pdfDict{}
			dst[cat] = dsub
		}
		for name, val := range sub {
			dsub[name] = val
		}
	}
}

// type3WidthFn returns the width function for layout (missing runes → 0,
// the glyph is skipped when encoding).
func (t *Type3Font) widthFn(size float64) widthFn {
	return func(r rune) float64 {
		if g, ok := t.byRune[r]; ok {
			return g.width / 1000.0 * size
		}
		return 0
	}
}

// encodeString writes the PDF string operand for s (unmapped runes are
// skipped — a Type3 font has no notdef fallback worth drawing).
func (t *Type3Font) encodeString(s string) string {
	var b strings.Builder
	b.WriteByte('(')
	for _, r := range s {
		g, ok := t.byRune[r]
		if !ok {
			continue
		}
		switch g.code {
		case '(', ')', '\\':
			b.WriteByte('\\')
		}
		b.WriteByte(g.code)
	}
	b.WriteByte(')')
	return b.String()
}

// ensureType3FontResource registers the built font in the page resources
// under a stable object-number-derived name.
func (p *Page) ensureType3FontResource(t *Type3Font) (string, error) {
	if t.doc != p.doc {
		return "", fmt.Errorf("type3 font belongs to a different document")
	}
	id, err := t.ensureBuilt()
	if err != nil {
		return "", err
	}
	pageDict := p.pageDict()
	if pageDict == nil {
		return "", fmt.Errorf("type3 font: page has no dict")
	}
	resources := p.pageResources()
	if resources == nil {
		resources = pdfDict{}
		pageDict["/Resources"] = resources
	}
	fontDict, _ := resolveRef(p.doc.objects, resources["/Font"]).(pdfDict)
	if fontDict == nil {
		fontDict = pdfDict{}
		resources["/Font"] = fontDict
	}
	name := fmt.Sprintf("/T3F%d", id)
	fontDict[name] = pdfRef{Num: id}
	return name, nil
}
