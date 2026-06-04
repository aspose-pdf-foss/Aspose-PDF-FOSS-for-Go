// SPDX-License-Identifier: MIT

package asposepdf

// textState holds the PDF text-object state used while rendering.
type textState struct {
	tm, lm    [6]float64 // text matrix, line matrix
	font      *renderFont
	fontSize  float64
	charSpace float64 // Tc
	wordSpace float64 // Tw
	hScale    float64 // Th = Tz/100
	leading   float64 // TL
	rise      float64 // Ts
}

// renderFont carries what the renderer needs to draw glyphs of one font: the
// TrueType program (outlines), CID→GID mapping, and per-code advance widths.
// Only embedded TrueType (Type0/CIDFontType2 with /FontFile2, and simple
// /TrueType) fonts are renderable here; Standard-14 outlines arrive in P4.
type renderFont struct {
	prog     *ttfFont
	em       float64
	isType0  bool
	cidToGID []uint16 // nil → identity (GID = CID)
	fi       fontInfo
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
	if descriptor == nil {
		return rf // no descriptor → not renderable here
	}
	stream, ok := resolveRef(objects, descriptor["/FontFile2"]).(*pdfStream)
	if !ok {
		return rf // no TrueType program (Type1/Standard-14 → P4)
	}
	prog, err := parseTTF(decodedStreamData(stream))
	if err != nil {
		return rf
	}
	rf.prog = prog
	if prog.unitsPerEm != 0 {
		rf.em = float64(prog.unitsPerEm)
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

	if f != nil && f.prog != nil && !rd.gs.fillPattern {
		if gid := f.gid(code); true {
			rd.fillGlyph(f, gid)
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

// fillGlyph rasterizes the glyph outline into the page with the fill colour.
func (rd *renderer) fillGlyph(f *renderFont, gid uint16) {
	contours := f.prog.glyphContours(gid)
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
	cov := rd.ras.coverage(fl.path(), fillNonZero)
	compositeCoverage(rd.img, rd.w, cov, rd.gs.fillR, rd.gs.fillG, rd.gs.fillB, rd.gs.fillA, rd.gs.clip)
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
		if f.cidToGID == nil {
			return cid
		}
		if int(cid) < len(f.cidToGID) {
			return f.cidToGID[cid]
		}
		return 0
	}
	// Simple TrueType: map code → rune (encoding) → GID via the font cmap.
	if code < 256 {
		if r := f.fi.encoding[code]; r != 0 {
			return f.prog.glyphID(r)
		}
	}
	return 0
}
