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
	prog     *ttfFont                    // embedded/substitute TrueType (glyf) program
	cff      *cffFont                    // embedded CFF program (/FontFile3); mutually exclusive with prog
	t1       *type1Font                  // embedded Type1 program (/FontFile)
	type3    *type3Font                  // Type3 font: glyphs are content streams, not outlines
	synth    func(uint32) []glyphContour // synthesized outlines by code (non-embedded ZapfDingbats)
	em       float64
	isType0  bool
	cidToGID []uint16        // nil → identity (GID = CID)
	fallback bool            // prog is the bundled substitute (non-embedded font)
	cjkUni   map[uint16]rune // non-embedded CJK: CID → Unicode (glyph via prog's cmap)
	cjkASCII map[uint16]rune // CID → ASCII rune for Latin codes the CMap leaves Unicode-less
	fi       fontInfo
}

// hasOutlines reports whether this font can draw glyph outlines.
func (f *renderFont) hasOutlines() bool {
	return f != nil && (f.prog != nil || f.cff != nil || f.t1 != nil)
}

// glyphOutline returns the glyph's flattened contours from whichever program
// backs this font.
func (f *renderFont) glyphOutline(gid uint16) []glyphContour {
	if f.cff != nil {
		return f.cff.glyphContours(gid)
	}
	if f.t1 != nil {
		return f.t1.glyphContours(gid)
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
	var fontDict pdfDict
	if fontsDict, ok := resolveRefToDict(objects, rd.res["/Font"]); ok {
		if fd, ok := resolveRefToDict(objects, fontsDict[name]); ok {
			fontDict = fd
		}
	}
	if fontDict == nil {
		// The content stream names a font the resources don't declare (e.g.
		// an empty /Font dict, 44963.pdf). Viewers substitute a default text
		// face instead of dropping the text — resolve a synthetic Helvetica
		// through the normal substitution chain, as MuPDF does.
		fontDict = pdfDict{
			"/Type":     pdfName("/Font"),
			"/Subtype":  pdfName("/Type1"),
			"/BaseFont": pdfName("/Helvetica"),
		}
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
	// Embedded classic Type1 outlines (/FontFile: eexec-encrypted PostScript
	// charstrings). Glyphs are keyed by name; gid() resolves code → name → GID.
	if stream, ok := resolveRef(objects, descriptor["/FontFile"]).(*pdfStream); ok {
		len1 := dictGetInt(stream.Dict, "/Length1")
		len2 := dictGetInt(stream.Dict, "/Length2")
		if v := resolveRef(objects, stream.Dict["/Length1"]); v != nil {
			len1 = int(operandFloat(v))
		}
		if v := resolveRef(objects, stream.Dict["/Length2"]); v != nil {
			len2 = int(operandFloat(v))
		}
		if t1 := parseType1(decodedStreamData(stream), len1, len2); t1 != nil {
			rf.t1 = t1
			if t1.unitsPerEm != 0 {
				rf.em = t1.unitsPerEm
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
			rf.cjkASCII = cjkASCIIFallback(fi.cidCMap, fi.cidToUni)
			rf.fallback = true
			if cjk.unitsPerEm != 0 {
				rf.em = float64(cjk.unitsPerEm)
			}
		}
	} else {
		// Non-embedded Type0 / CIDFontType2 with no Adobe CJK ordering.
		// Preference order:
		//  1. Identity /CIDToGIDMap + the exact named font installed — its glyph
		//     IDs are exactly the document's CIDs (e.g. Yu Gothic / YuGothM.ttc),
		//     an exact render via GID-as-CID.
		//  2. A /ToUnicode CMap (Identity-H) — substitute a metric-compatible
		//     face and reach glyphs through Unicode (code=CID → /ToUnicode rune →
		//     substitute glyph). This covers an explicit (non-Identity)
		//     /CIDToGIDMap, e.g. Arial in a visible signature appearance, whose
		//     original GIDs are useless against a substitute. Reuses the cjkUni
		//     machinery, which gid() consults before the /CIDToGIDMap.
		//  3. Identity /CIDToGIDMap, no ToUnicode, font not installed — GID-as-CID
		//     against a substitute: approximate, but not blank.
		var fb *ttfFont
		viaUnicode := false
		if rf.cidToGID == nil {
			fb = fontRepo.findSystemExact(fi)
		}
		if fb == nil && fi.toUnicode != nil && fi.cidCMap == nil {
			fb = fontRepo.find(fi)
			if fb == nil && isStd14Alias(fi.name) {
				fb = fontRepo.findSystemStd14(fi)
			}
			if fb == nil {
				fb = fallbackFontFor(fi)
			}
			viaUnicode = fb != nil
		}
		if fb == nil && rf.cidToGID == nil {
			fb = fallbackFontFor(fi)
		}
		if fb != nil {
			rf.prog = fb
			rf.fallback = true
			if viaUnicode {
				rf.cjkUni = fi.toUnicode
			}
			if fb.unitsPerEm != 0 {
				rf.em = float64(fb.unitsPerEm)
			}
		}
	}
	return rf
}

// cjkASCIIFallback maps the CIDs that a CMap assigns to single-byte ASCII codes
// but Adobe's CID→Unicode table leaves unmapped (the proportional-Latin glyphs)
// back to the ASCII rune, so Latin runs inside a CJK font still render.
func cjkASCIIFallback(cm *cidCMap, cidToUni map[uint16]rune) map[uint16]rune {
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

	// Clip modes (4-7) paint no fill/stroke for mode 7 but still contribute the
	// glyph outline to the text clip accumulated for ET, so process them too.
	if f != nil && (textHasPaint(rd.ts.renderMode) || rd.ts.renderMode >= 4) {
		switch {
		case f.hasOutlines():
			// Skip glyph 0 (.notdef): an unmapped code renders nothing rather
			// than the font's "missing glyph" box (tofu).
			if g := f.gid(code); g != 0 {
				rd.paintGlyphX(f, g, substituteXScale(f, code, g))
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
	rd.paintGlyphX(f, gid, 1)
}

// paintGlyphX is paintGlyph with a horizontal condensation factor applied in
// glyph design space (see substituteXScale).
func (rd *renderer) paintGlyphX(f *renderFont, gid uint16, xScale float64) {
	rd.paintContoursX(f, f.glyphOutline(gid), xScale)
}

// substituteXScale returns the horizontal scale that condenses (or expands) a
// substituted simple-font glyph so its painted width matches the document's
// declared /Widths advance. Without it, a substitute wider than the original
// face (e.g. Arimo standing in for a narrow script font) overlaps the next
// glyph, because advances honor /Widths while outlines paint at natural
// width. Acrobat (Adobe Sans MM) and MuPDF condense substituted glyphs the
// same way. Metric-compatible substitutes (within 2%) are left untouched.
func substituteXScale(f *renderFont, code uint32, gid uint16) float64 {
	if !f.fallback || f.isType0 || f.prog == nil || code >= 256 {
		return 1
	}
	declared := f.fi.widths[code]
	if declared <= 0 || int(gid) >= len(f.prog.glyphWidths) || f.em == 0 {
		return 1
	}
	natural := float64(f.prog.glyphWidths[gid]) / f.em * 1000
	if natural <= 0 {
		return 1
	}
	s := declared / natural
	if s > 0.98 && s < 1.02 {
		return 1
	}
	return s
}

// paintContours rasterizes a set of glyph design-unit contours, filling and/or
// stroking per the current text rendering mode (Tr). Shared by real glyph
// outlines and synthesized ones (ZapfDingbats marks).
func (rd *renderer) paintContours(f *renderFont, contours []glyphContour) {
	rd.paintContoursX(f, contours, 1)
}

// paintContoursX is paintContours with a horizontal pre-scale in glyph design
// space (substituted-font width matching).
func (rd *renderer) paintContoursX(f *renderFont, contours []glyphContour, xScale float64) {
	if len(contours) == 0 {
		return
	}
	// Glyph design units → device: scale by 1/em, then text-rendering matrix
	// (font size, horizontal scaling, rise) · text matrix · CTM · device base.
	trm := matMul([6]float64{rd.ts.fontSize * rd.ts.hScale, 0, 0, rd.ts.fontSize, 0, rd.ts.rise}, rd.ts.tm)
	if xScale != 1 {
		trm = matMul([6]float64{xScale, 0, 0, 1, 0, 0}, trm)
	}
	m := matMul(trm, matMul(rd.gs.ctm, rd.base))

	fl := newFlattener(0.2)
	for _, c := range contours {
		renderGlyphContour(fl, c, m, f.em)
	}
	path := fl.path()
	mode := rd.ts.renderMode

	if mode >= 4 { // clip modes 4-7: accumulate glyph outlines for the ET clip
		rd.textClip = append(rd.textClip, path.subs...)
	}
	if rd.suppressText {
		return
	}
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
		cid := uint16(code)  // showText passes the resolved CID as the code
		if f.cjkUni != nil { // non-embedded CJK: CID → Unicode → GID via cmap
			r := f.cjkUni[cid]
			if r == 0 {
				r = f.cjkASCII[cid] // Latin codes the CMap leaves Unicode-less
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
	// Embedded Type1 (/FontFile): charstrings are keyed by glyph name. Resolve
	// the code to a name — the PDF /Differences name first, then the base
	// encoding's standard name, then the font's own built-in encoding — and
	// look up the synthetic GID (ISO 32000-1 §9.6.6.1).
	if code < 256 && f.t1 != nil {
		var name string
		if f.fi.glyphNames != nil {
			name = f.fi.glyphNames[byte(code)]
		}
		if name == "" && f.fi.known {
			if r := f.fi.encoding[code]; r != 0 && r != 0xFFFD {
				name = runeToStdGlyphName(r)
			}
		}
		if name == "" {
			name = f.t1.builtinEnc[code]
		}
		if g, ok := f.t1.nameToGID[name]; ok {
			return g
		}
		return 0
	}
	// Simple CFF (Type1C) glyph selection (ISO 32000-1 §9.6.6.2). When the PDF
	// font dict carries its own /Encoding (WinAnsi etc., resolved into
	// fi.encoding) it overrides the font's built-in encoding: the code's glyph
	// name reaches the glyph through the charset (rune → name-derived GID).
	// Subset fonts often embed a built-in encoding covering only a few codes, so
	// without this the page renders nearly blank. The font's own encoding is the
	// fallback for codes the PDF encoding leaves unmapped.
	if code < 256 && f.cff != nil {
		// A /Differences name with no Unicode meaning (ZapfDingbats a71 etc.)
		// reaches the glyph by name through the charset.
		if f.fi.glyphNames != nil {
			if n := f.fi.glyphNames[byte(code)]; n != "" {
				if g := f.cff.nameToGID[n]; g != 0 {
					return g
				}
			}
		}
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
		if f.fallback {
			// Substituted font: the document's raw codes mean nothing in the
			// substitute's own (1,0)/(3,0) cmap — a custom /Differences code
			// like 35 would hit the substitute's '#' glyph. Resolve through
			// /ToUnicode first (obfuscated subsets lie in /Differences but
			// tell the truth there — e.g. a quote glyph named /numbersign),
			// then the PDF encoding; raw-code maps are for embedded programs.
			if r := f.fi.toUnicode[uint16(code)]; r != 0 {
				if g := f.prog.glyphID(r); g != 0 {
					return g
				}
			}
			if r := f.fi.encoding[code]; r != 0 && r != 0xFFFD {
				if g := f.prog.glyphID(r); g != 0 {
					return g
				}
			}
		}
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
