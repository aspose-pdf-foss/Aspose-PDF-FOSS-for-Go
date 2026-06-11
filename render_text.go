// SPDX-License-Identifier: MIT

package asposepdf

import (
	"math"
	"strconv"
)

// textState holds the PDF text-object state used while rendering.
type textState struct {
	tm, lm     [6]float64 // text matrix, line matrix
	font       *renderFont
	fontSize   float64
	charSpace  float64 // Tc
	wordSpace  float64 // Tw
	hScale     float64 // Th = Tz/100
	leading    float64 // TL
	rise       float64 // Ts
	renderMode int     // Tr (0 fill, 1 stroke, 2 fill+stroke, 3 invisible, 4-7 add clip)
}

// renderFont carries what the renderer needs to draw glyphs of one font: the
// TrueType program (outlines), CID→GID mapping, and per-code advance widths.
// Only embedded TrueType (Type0/CIDFontType2 with /FontFile2, and simple
// /TrueType) fonts are renderable here; Standard-14 outlines arrive in P4.
type renderFont struct {
	prog     *ttfFont   // embedded/substitute TrueType (glyf) program
	cff      *cffFont   // embedded CFF program (/FontFile3); mutually exclusive with prog
	type3    *type3Font // Type3 font: glyphs are content streams, not outlines
	synth    func(uint32) []glyphContour // synthesized outlines by code (non-embedded ZapfDingbats)
	em       float64
	isType0  bool
	cidToGID []uint16 // nil → identity (GID = CID)
	fallback bool     // prog is the bundled substitute (non-embedded font)
	cjkUni   map[uint16]rune // non-embedded CJK: CID → Unicode (glyph via prog's cmap)
	cjkAscii map[uint16]rune // CID → ASCII rune for Latin codes the CMap leaves Unicode-less
	fi       fontInfo
}

// hasOutlines reports whether this font can draw glyph outlines.
func (f *renderFont) hasOutlines() bool { return f != nil && (f.prog != nil || f.cff != nil) }

// glyphOutline returns the glyph's flattened contours from whichever program
// backs this font.
func (f *renderFont) glyphOutline(gid uint16) []glyphContour {
	if f.cff != nil {
		return f.cff.glyphContours(gid)
	}
	if f.prog != nil {
		return f.prog.glyphContours(gid)
	}
	return nil
}

// beginText / text operators -------------------------------------------------

func (rd *renderer) textBegin() {
	rd.ts.tm = identityMatrix()
	rd.ts.lm = identityMatrix()
}

func (rd *renderer) setFont(name string, size float64) {
	rd.ts.fontSize = size
	if rd.ts.hScale == 0 {
		rd.ts.hScale = 1
	}
	rd.ts.font = rd.resolveRenderFont(name)
}

func (rd *renderer) textMove(tx, ty float64) {
	rd.ts.lm = matMul(translateMatrix(tx, ty), rd.ts.lm)
	rd.ts.tm = rd.ts.lm
}

func (rd *renderer) textSetMatrix(m [6]float64) {
	rd.ts.tm = m
	rd.ts.lm = m
}

func (rd *renderer) textNextLine() { rd.textMove(0, -rd.ts.leading) }

// showText draws a PDF string and advances the text matrix.
func (rd *renderer) showText(s string) {
	f := rd.ts.font
	if rd.ts.hScale == 0 {
		rd.ts.hScale = 1
	}
	if f != nil && f.isType0 {
		if f.fi.cidCMap != nil {
			// Predefined/embedded CMap: the codespace dictates each code's byte
			// length (mixed 1-byte Latin / 2-byte CJK). showGlyph receives the
			// resolved CID as its code (gid()/width are CID-keyed downstream).
			b := []byte(s)
			for len(b) > 0 {
				_, cid, n := f.fi.cidCMap.next(b)
				rd.showGlyph(uint32(cid), n == 1 && b[0] == 32)
				b = b[n:]
			}
			return
		}
		for i := 0; i+1 < len(s); i += 2 {
			code := uint16(s[i])<<8 | uint16(s[i+1])
			rd.showGlyph(uint32(code), code == 32)
		}
		return
	}
	for i := 0; i < len(s); i++ {
		rd.showGlyph(uint32(s[i]), s[i] == 32)
	}
}

// showTJ handles the TJ operator (array of strings and position adjustments).
func (rd *renderer) showTJ(v pdfValue) {
	arr, ok := v.(pdfArray)
	if !ok {
		return
	}
	if rd.ts.hScale == 0 {
		rd.ts.hScale = 1
	}
	for _, el := range arr {
		if s, ok := el.(string); ok {
			rd.showText(s)
			continue
		}
		adj := operandFloat(el)
		tx := -adj / 1000 * rd.ts.fontSize * rd.ts.hScale
		rd.ts.tm = matMul(translateMatrix(tx, 0), rd.ts.tm)
	}
}

// resolveRenderFont returns the renderable font for a /Font resource name,
// caching the result. The cache key is the font's indirect object number, not
// the resource name: a page and a nested Form XObject may both define e.g.
// /T1_0 pointing at different fonts (56333.pdf swaps two faces this way), and
// a name-keyed cache would render the form's text with the page's font. A nil
// program means the font has no embedded outline and its glyphs are skipped.
func (rd *renderer) resolveRenderFont(name string) *renderFont {
	key := name
	if fontsDict, ok := resolveRefToDict(rd.page.doc.objects, rd.res["/Font"]); ok {
		if ref, ok := fontsDict[name].(pdfRef); ok {
			key = "obj:" + strconv.Itoa(ref.Num)
		}
	}
	if rf, ok := rd.fontCache[key]; ok {
		return rf
	}
	rf := rd.buildRenderFont(name)
	rd.fontCache[key] = rf
	return rf
}

func (rd *renderer) buildRenderFont(name string) *renderFont {
	objects := rd.page.doc.objects
	fontsDict, ok := resolveRefToDict(objects, rd.res["/Font"])
	if !ok {
		return nil
	}
	fontDict, ok := resolveRefToDict(objects, fontsDict[name])
	if !ok {
		return nil
	}
	fi := resolveFont(objects, fontDict)
	rf := &renderFont{fi: fi, isType0: fi.isType0, em: 1000}

	// Type3 fonts define each glyph as a content stream, not an outline.
	if dictGetName(fontDict, "/Subtype") == "/Type3" {
		rf.type3 = rd.buildType3Font(objects, fontDict)
		return rf
	}

	var descriptor pdfDict
	if fi.isType0 {
		if descArr, ok := resolveRefToArray(objects, fontDict["/DescendantFonts"]); ok && len(descArr) > 0 {
			if cidFont, ok := resolveRefToDict(objects, descArr[0]); ok {
				descriptor, _ = resolveRefToDict(objects, cidFont["/FontDescriptor"])
				rf.cidToGID = parseCIDToGIDMap(objects, cidFont["/CIDToGIDMap"])
			}
		}
	} else {
		descriptor, _ = resolveRefToDict(objects, fontDict["/FontDescriptor"])
	}
	if stream, ok := resolveRef(objects, descriptor["/FontFile2"]).(*pdfStream); ok {
		if prog, err := parseTTF(decodedStreamData(stream)); err == nil {
			rf.prog = prog
			if prog.unitsPerEm != 0 {
				rf.em = float64(prog.unitsPerEm)
			}
			return rf
		}
	}
	// Embedded CFF outlines (/FontFile3: Type1C, CIDFontType0C, or an OpenType
	// wrapper). The bytes may be a bare CFF or an sfnt with a 'CFF ' table.
	if stream, ok := resolveRef(objects, descriptor["/FontFile3"]).(*pdfStream); ok {
		if cff, err := parseCFFProgram(decodedStreamData(stream)); err == nil {
			rf.cff = cff
			if cff.unitsPerEm != 0 {
				rf.em = cff.unitsPerEm
			}
			return rf
		}
	}

	// No embedded program. For a simple (non-Type0) font — chiefly
	// the Standard 14 — substitute the bundled fallback font's glyph shapes,
	// keeping the resolved AFM metrics for positioning. Type0 fonts map codes
	// to GIDs that only match their own program, so they are left unrendered.
	if !fi.isType0 {
		// Resolution order: a caller-registered or system font (exact), then
		// the bundled metric-compatible substitute.
		fb := fontRepo.find(fi)
		if fb == nil && isCJKFamily(fi.name) {
			// A non-embedded simple font that is really a CJK face (e.g. SimSun
			// with WinAnsiEncoding for its Latin glyphs). Render from the actual
			// installed CJK font — its Latin forms are half-width and upright, and
			// the bundled Latin substitutes don't match — keeping it consistent
			// with the composite sibling of the same font.
			fb = fontRepo.findCJK(fi, "")
		}
		if fb == nil && isStd14Alias(fi.name) {
			// Standard-14 families: prefer the installed metric-equivalent face
			// (Courier New / Arial / Times New Roman) — the same substitution
			// Acrobat performs — over the bundled clone, whose letterforms differ
			// (e.g. Cousine's low-serif design vs Courier New's slab serifs).
			fb = fontRepo.findSystemStd14(fi)
		}
		if fb == nil {
			fb = fallbackFontFor(fi)
		}
		if fb != nil {
			rf.prog = fb
			rf.fallback = true
			if fb.unitsPerEm != 0 {
				rf.em = float64(fb.unitsPerEm)
			}
		} else if isZapfDingbats(fi) {
			// No bundled substitute for ZapfDingbats; synthesize the common marks
			// (checkbox/radio appearances) as vector outlines in 1000-em space.
			rf.synth = zapfDingbatsContours
			rf.em = 1000
		}
	} else if fi.cidToUni != nil {
		// Non-embedded composite CJK font (e.g. SimSun with /Encoding /GBK-EUC-H,
		// no /FontFile2). Render real glyphs from an installed CJK font: map the
		// CID → Unicode (Adobe ordering table) → GID via that font's cmap. We do
		// not use the bundled Latin substitutes here — they have no CJK glyphs.
		if cjk := fontRepo.findCJK(fi, fi.ordering); cjk != nil {
			rf.prog = cjk
			rf.cjkUni = fi.cidToUni
			rf.cjkAscii = cjkAsciiFallback(fi.cidCMap, fi.cidToUni)
			rf.fallback = true
			if cjk.unitsPerEm != 0 {
				rf.em = float64(cjk.unitsPerEm)
			}
		}
	} else if rf.cidToGID == nil {
		// Non-embedded Type0 CIDFontType2 with Identity CIDToGIDMap: producers
		// emit the original font's glyph IDs as CIDs (no ToUnicode, GID-as-CID).
		// If the actual named font is installed (e.g. Yu Gothic — YuGothM.ttc),
		// use it: its glyph IDs are exactly the document's CIDs, so the text
		// renders in the correct face. Otherwise substitute a metric-compatible
		// family whose Latin glyph order matches and use the CID as a GID
		// directly — an approximate render rather than blank text.
		fb := fontRepo.findSystemExact(fi)
		if fb == nil {
			fb = fallbackFontFor(fi)
		}
		if fb != nil {
			rf.prog = fb
			rf.fallback = true
			if fb.unitsPerEm != 0 {
				rf.em = float64(fb.unitsPerEm)
			}
		}
	}
	return rf
}

// cjkAsciiFallback maps the CIDs that a CMap assigns to single-byte ASCII codes
// but Adobe's CID→Unicode table leaves unmapped (the proportional-Latin glyphs)
// back to the ASCII rune, so Latin runs inside a CJK font still render.
func cjkAsciiFallback(cm *cidCMap, cidToUni map[uint16]rune) map[uint16]rune {
	if cm == nil {
		return nil
	}
	m := map[uint16]rune{}
	for code := 0x20; code < 0x7f; code++ {
		_, cid, n := cm.next([]byte{byte(code)})
		if n == 1 && cid != 0 && cidToUni[cid] == 0 {
			m[cid] = rune(code)
		}
	}
	return m
}

// decodedStreamData returns a stream's content with its filters applied.
func decodedStreamData(s *pdfStream) []byte {
	if s.Decoded {
		return s.Data
	}
	if out, err := decodeStream(s.Dict, s.Data); err == nil {
		return out
	}
	return s.Data
}

// parseCIDToGIDMap returns the CID→GID table, or nil for /Identity (or absent).
func parseCIDToGIDMap(objects map[int]*pdfObject, v pdfValue) []uint16 {
	stream, ok := resolveRef(objects, v).(*pdfStream)
	if !ok {
		return nil // /Identity or absent → GID = CID
	}
	data := decodedStreamData(stream)
	n := len(data) / 2
	m := make([]uint16, n)
	for i := 0; i < n; i++ {
		m[i] = uint16(data[i*2])<<8 | uint16(data[i*2+1])
	}
	return m
}

// showGlyph fills one glyph (if its font is renderable) and advances tm.
func (rd *renderer) showGlyph(code uint32, isSpace bool) {
	f := rd.ts.font
	w0 := rd.glyphWidth(code) // 1/1000 em

	if f != nil && textHasPaint(rd.ts.renderMode) {
		switch {
		case f.hasOutlines():
			// Skip glyph 0 (.notdef): an unmapped code renders nothing rather
			// than the font's "missing glyph" box (tofu).
			if g := f.gid(code); g != 0 {
				rd.paintGlyph(f, g)
			}
		case f.synth != nil:
			rd.paintContours(f, f.synth(code))
		case f.type3 != nil:
			rd.drawType3Glyph(f, code)
		}
	}

	// Advance: tx = (w0/1000 * fontSize + Tc + (Tw if space)) * Th.
	tx := (w0/1000*rd.ts.fontSize + rd.ts.charSpace)
	if isSpace {
		tx += rd.ts.wordSpace
	}
	tx *= rd.ts.hScale
	rd.ts.tm = matMul(translateMatrix(tx, 0), rd.ts.tm)
}

func (rd *renderer) glyphWidth(code uint32) float64 {
	f := rd.ts.font
	if f == nil {
		return 0
	}
	if f.fallback && f.prog != nil && !f.isType0 {
		// Substitute font: advance by the document's declared /Widths (or the
		// Standard-14 AFM width) so layout matches the producer's, even when the
		// substitute's own metrics differ — otherwise e.g. a Garamond mapped to a
		// wider sans overflows into the next field. Only when the document gives
		// no width for this code do we fall back to the substitute's natural
		// advance.
		if code < 256 && f.fi.widths[code] != 0 {
			return f.fi.widths[code]
		}
		gid := f.gid(code)
		if int(gid) < len(f.prog.glyphWidths) {
			return float64(f.prog.glyphWidths[gid]) / f.em * 1000
		}
	}
	if f.isType0 {
		if w, ok := f.fi.cidWidths[uint16(code)]; ok {
			return w
		}
		return f.fi.defaultW
	}
	if code < 256 {
		return f.fi.widths[code]
	}
	return 0
}

// Text rendering modes (Tr, ISO 32000-1 §9.3.6). Fill modes paint with the fill
// colour; stroke modes outline with the stroke colour; modes 3/7 paint nothing
// (3 = invisible, used for the hidden OCR layer over scanned pages).
func textFills(m int) bool    { return m == 0 || m == 2 || m == 4 || m == 6 }
func textStrokes(m int) bool  { return m == 1 || m == 2 || m == 5 || m == 6 }
func textHasPaint(m int) bool { return textFills(m) || textStrokes(m) }

// paintGlyph rasterizes the glyph outline, filling and/or stroking it per the
// current text rendering mode (Tr). Text clipping (modes 4-7) is not yet
// accumulated; those modes still paint their fill/stroke component.
func (rd *renderer) paintGlyph(f *renderFont, gid uint16) {
	rd.paintContours(f, f.glyphOutline(gid))
}

// paintContours rasterizes a set of glyph design-unit contours, filling and/or
// stroking per the current text rendering mode (Tr). Shared by real glyph
// outlines and synthesized ones (ZapfDingbats marks).
func (rd *renderer) paintContours(f *renderFont, contours []glyphContour) {
	if len(contours) == 0 {
		return
	}
	// Glyph design units → device: scale by 1/em, then text-rendering matrix
	// (font size, horizontal scaling, rise) · text matrix · CTM · device base.
	trm := matMul([6]float64{rd.ts.fontSize * rd.ts.hScale, 0, 0, rd.ts.fontSize, 0, rd.ts.rise}, rd.ts.tm)
	m := matMul(trm, matMul(rd.gs.ctm, rd.base))

	fl := newFlattener(0.2)
	for _, c := range contours {
		renderGlyphContour(fl, c, m, f.em)
	}
	path := fl.path()
	mode := rd.ts.renderMode

	if textFills(mode) && !rd.gs.fillPattern {
		rd.compositePath(path, fillNonZero, rd.gs.fillR, rd.gs.fillG, rd.gs.fillB, rd.gs.fillA)
	}
	if textStrokes(mode) {
		dm := rd.dmat()
		scale := math.Sqrt(math.Abs(dm[0]*dm[3] - dm[1]*dm[2]))
		dw := rd.gs.lineWidth * scale
		if dw < 1 {
			dw = 1
		}
		st := strokeStyle{hw: dw / 2, cap: rd.gs.lineCap, join: rd.gs.lineJoin, miterLimit: rd.gs.miterLimit}
		rd.compositePath(strokeToFill(path, st), fillNonZero, rd.gs.strokeR, rd.gs.strokeG, rd.gs.strokeB, rd.gs.strokeA)
	}
}

// renderGlyphContour emits one TrueType contour (on/off-curve quadratics) into
// the flattener, transforming each design-unit point through m (after dividing
// by the em size).
func renderGlyphContour(fl *flattener, c glyphContour, m [6]float64, em float64) {
	n := len(c)
	if n == 0 || em == 0 {
		return
	}
	pt := func(i int) (float64, float64) {
		p := c[((i%n)+n)%n]
		return applyPt(m, p.x/em, p.y/em)
	}
	on := func(i int) bool { return c[((i%n)+n)%n].on }

	start := 0
	for start < n && !on(start) {
		start++
	}
	if start == n {
		// All control points off-curve: start at the midpoint of the last and
		// first points and treat every point as a control point.
		ax, ay := pt(0)
		bx, by := pt(n - 1)
		sx, sy := (ax+bx)/2, (ay+by)/2
		fl.moveTo(sx, sy)
		hasOff := false
		var ox, oy float64
		for k := 0; k <= n; k++ {
			x, y := pt(k)
			if hasOff {
				mx, my := (ox+x)/2, (oy+y)/2
				fl.quadTo(ox, oy, mx, my)
			}
			ox, oy, hasOff = x, y, true
		}
		fl.close()
		return
	}

	sx, sy := pt(start)
	fl.moveTo(sx, sy)
	hasOff := false
	var ox, oy float64
	for k := 1; k <= n; k++ {
		i := start + k
		x, y := pt(i)
		if on(i) {
			if hasOff {
				fl.quadTo(ox, oy, x, y)
				hasOff = false
			} else {
				fl.lineTo(x, y)
			}
		} else {
			if hasOff {
				mx, my := (ox+x)/2, (oy+y)/2
				fl.quadTo(ox, oy, mx, my)
			}
			ox, oy, hasOff = x, y, true
		}
	}
	fl.close()
}

// gid maps a character code to a glyph ID for this font.
func (f *renderFont) gid(code uint32) uint16 {
	if f.isType0 {
		cid := uint16(code) // showText passes the resolved CID as the code
		if f.cjkUni != nil { // non-embedded CJK: CID → Unicode → GID via cmap
			r := f.cjkUni[cid]
			if r == 0 {
				r = f.cjkAscii[cid] // Latin codes the CMap leaves Unicode-less
			}
			if r != 0 && f.prog != nil {
				return f.prog.glyphID(r)
			}
			return 0
		}
		if f.cidToGID != nil { // CIDFontType2 explicit map
			if int(cid) < len(f.cidToGID) {
				return f.cidToGID[cid]
			}
			return 0
		}
		if f.cff != nil && f.cff.isCID { // CIDFontType0C: charset maps CID→GID
			return f.cff.gidForCID(cid)
		}
		return cid // Identity
	}
	// Simple CFF (Type1C) glyph selection (ISO 32000-1 §9.6.6.2). When the PDF
	// font dict carries its own /Encoding (WinAnsi etc., resolved into
	// fi.encoding) it overrides the font's built-in encoding: the code's glyph
	// name reaches the glyph through the charset (rune → name-derived GID).
	// Subset fonts often embed a built-in encoding covering only a few codes, so
	// without this the page renders nearly blank. The font's own encoding is the
	// fallback for codes the PDF encoding leaves unmapped.
	if code < 256 && f.cff != nil {
		if f.fi.known {
			if r := f.fi.encoding[code]; r != 0 && r != 0xFFFD {
				if g := f.cff.runeToGID[r]; g != 0 {
					return g
				}
			}
		}
		if g := f.cff.simpleGID[uint16(code)]; g != 0 {
			return g
		}
	}
	// Simple TrueType glyph selection (ISO 32000-1 §9.6.6.4). Embedded subset
	// fonts often carry only a (1,0) Mac or (3,0) symbol cmap keyed by the raw
	// byte code, so try that first; then the Unicode cmap via the PDF encoding;
	// then the symbol 0xF000 range.
	if code < 256 && f.prog != nil {
		c := uint16(code)
		if g := f.prog.codeToGlyph[c]; g != 0 {
			return g
		}
		if r := f.fi.encoding[code]; r != 0 && r != 0xFFFD {
			if g := f.prog.glyphID(r); g != 0 {
				return g
			}
		}
		if g := f.prog.codeToGlyph[0xF000|c]; g != 0 {
			return g
		}
	}
	return 0
}
