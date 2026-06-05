// SPDX-License-Identifier: MIT

package asposepdf

import "math"

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
// caching the result. A nil program means the font has no embedded TrueType
// outline (e.g. Standard-14) and its glyphs are skipped until P4.
func (rd *renderer) resolveRenderFont(name string) *renderFont {
	if rf, ok := rd.fontCache[name]; ok {
		return rf
	}
	rf := rd.buildRenderFont(name)
	rd.fontCache[name] = rf
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
	} else if rf.cidToGID == nil {
		// Non-embedded Type0 CIDFontType2 with Identity CIDToGIDMap: producers
		// commonly emit the original font's glyph IDs as CIDs (an anti-extraction
		// pattern — no ToUnicode, GID-as-CID). Substitute a metric-compatible
		// family whose Latin glyph order matches and use the CID as a GID
		// directly (gid() returns the CID for Identity). Better an approximate
		// render than blank text.
		fb := fontRepo.find(fi)
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
		// Substitute font: advance by its own natural glyph width so letterforms
		// and spacing stay self-consistent. The document's Standard-14 metrics do
		// not match these (DejaVu) shapes — matching them instead distorts narrow
		// glyphs (i, l) and their side bearings. Exact metrics await a
		// metric-compatible bundled family (backlog).
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
		cid := uint16(code) // Identity-H: code == CID
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
	// Simple CFF (Type1C) glyph selection: the CFF charset+encoding gives a
	// direct code→GID map (ISO 32000-1 §9.6.6.2).
	if code < 256 && f.cff != nil {
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
