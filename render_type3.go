// SPDX-License-Identifier: MIT

package asposepdf

// Type3 fonts define each glyph as a small content stream (a /CharProcs entry)
// drawn with ordinary vector operators, rather than an outline. Rendering one is
// just running that stream with the right CTM: FontMatrix maps glyph space into
// text space, then the usual text-rendering matrix · text matrix · CTM apply.

type type3Font struct {
	charProcs  pdfDict     // glyph name (e.g. "/a1") → content stream
	enc        [256]string // character code → glyph name, from /Encoding/Differences
	fontMatrix [6]float64  // glyph space → text space
	resources  pdfDict     // resources for the char-proc streams
}

// buildType3Font collects the /CharProcs, /FontMatrix, /Resources and the
// code→name encoding of a Type3 font. Returns nil if it has no char procs.
func (rd *renderer) buildType3Font(objects map[int]*pdfObject, fontDict pdfDict) *type3Font {
	cp, ok := resolveRefToDict(objects, fontDict["/CharProcs"])
	if !ok {
		return nil
	}
	t3 := &type3Font{charProcs: cp, fontMatrix: identityMatrix()}
	if fm := shFloats(objects, fontDict["/FontMatrix"]); len(fm) == 6 {
		t3.fontMatrix = [6]float64{fm[0], fm[1], fm[2], fm[3], fm[4], fm[5]}
	}
	t3.resources, _ = resolveRefToDict(objects, fontDict["/Resources"])
	t3.enc = type3Encoding(objects, fontDict)
	return t3
}

// type3Encoding reads /Encoding/Differences into a code→glyph-name table. Unlike
// a normal font's encoding (code→rune), Type3 needs the raw glyph names to index
// /CharProcs.
func type3Encoding(objects map[int]*pdfObject, fontDict pdfDict) [256]string {
	var enc [256]string
	d, ok := resolveRefToDict(objects, fontDict["/Encoding"])
	if !ok {
		return enc
	}
	diffs, ok := resolveRefToArray(objects, d["/Differences"])
	if !ok {
		return enc
	}
	code := 0
	for _, e := range diffs {
		switch v := resolveRef(objects, e).(type) {
		case int:
			code = v
		case float64:
			code = int(v)
		case pdfName:
			if code >= 0 && code < 256 {
				enc[code] = string(v)
				code++
			}
		}
	}
	return enc
}

// drawType3Glyph executes the char-proc content stream for one code, mapped into
// device space by FontMatrix · text-rendering matrix · text matrix · CTM. The
// current fill colour is kept so uncolored (d1) glyphs paint correctly.
func (rd *renderer) drawType3Glyph(f *renderFont, code uint32) {
	if rd.depth >= 8 || code >= 256 {
		return
	}
	t3 := f.type3
	name := t3.enc[code]
	if name == "" {
		return
	}
	stream, ok := resolveRef(rd.page.doc.objects, t3.charProcs[name]).(*pdfStream)
	if !ok {
		return
	}
	ops, err := parseContentStream(decodedStreamData(stream))
	if err != nil {
		return
	}

	trm := matMul([6]float64{rd.ts.fontSize * rd.ts.hScale, 0, 0, rd.ts.fontSize, 0, rd.ts.rise}, rd.ts.tm)
	glyphCTM := matMul(t3.fontMatrix, matMul(trm, rd.gs.ctm))

	savedGS, savedRes, savedFl, savedStack := rd.gs, rd.res, rd.fl, len(rd.stack)
	rd.gs.ctm = glyphCTM
	if t3.resources != nil {
		rd.res = t3.resources
	}
	rd.fl = newFlattener(0.2)
	rd.depth++
	rd.exec(ops)
	rd.depth--
	rd.gs, rd.res, rd.fl = savedGS, savedRes, savedFl
	if len(rd.stack) > savedStack {
		rd.stack = rd.stack[:savedStack]
	}
}
