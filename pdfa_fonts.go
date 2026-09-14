// SPDX-License-Identifier: MIT

package asposepdf

import "encoding/binary"

// Font work for PDF/A conversion: finding out which fonts a document actually
// draws with, and giving a composite (CJK) font a program of its own.

// forEachContentStream calls fn for every content stream in the document
// together with the resources it draws against: page contents, Form XObjects
// (annotation appearances and patterns among them) and the glyph procedures of
// Type3 fonts.
func (d *Document) forEachContentStream(fn func(data []byte, res pdfDict)) {
	for _, p := range d.Pages() {
		if data, err := p.contentStreams(); err == nil {
			fn(data, p.pageResources())
		}
	}
	for _, obj := range d.objects {
		switch v := obj.Value.(type) {
		case *pdfStream:
			if res, ok := resolveRefToDict(d.objects, v.Dict["/Resources"]); ok {
				fn(decodedStreamData(v), res)
			}
		case pdfDict:
			if dictGetName(v, "/Subtype") != "/Type3" {
				continue
			}
			res, ok := resolveRefToDict(d.objects, v["/Resources"])
			if !ok {
				continue
			}
			procs, ok := resolveRefToDict(d.objects, v["/CharProcs"])
			if !ok {
				continue
			}
			for _, pv := range procs {
				if st, ok := resolveRef(d.objects, pv).(*pdfStream); ok {
					fn(decodedStreamData(st), res)
				}
			}
		}
	}
}

// pdfaUsedCIDs returns, per composite (Type0) font object, the character
// identifiers its text actually selects — the set an embedded subset has to
// cover. Codes are decoded through the font own CMap, so a predefined CJK
// encoding yields real CIDs rather than raw bytes.
func (d *Document) pdfaUsedCIDs() map[int]map[uint16]bool {
	out := map[int]map[uint16]bool{}
	d.forEachContentStream(func(data []byte, res pdfDict) {
		fonts, ok := resolveRefToDict(d.objects, res["/Font"])
		if !ok {
			return
		}
		ops, err := parseContentStream(data)
		if err != nil {
			return
		}
		var (
			curNum int
			curFI  fontInfo
			have   bool
		)
		show := func(v pdfValue) {
			s, ok := v.(string)
			if !ok || !have {
				return
			}
			set := out[curNum]
			if set == nil {
				set = map[uint16]bool{}
				out[curNum] = set
			}
			if curFI.cidCMap != nil {
				b := []byte(s)
				for len(b) > 0 {
					_, cid, n := curFI.cidCMap.next(b)
					if n <= 0 {
						break
					}
					set[cid] = true
					b = b[n:]
				}
				return
			}
			for i := 0; i+1 < len(s); i += 2 { // Identity: the code is the CID
				set[uint16(s[i])<<8|uint16(s[i+1])] = true
			}
		}
		for _, op := range ops {
			switch op.Operator {
			case "Tf":
				have = false
				if len(op.Operands) == 0 {
					continue
				}
				name, ok := op.Operands[0].(pdfName)
				if !ok {
					continue
				}
				ref, ok := fonts[string(name)].(pdfRef)
				if !ok {
					continue
				}
				fd, ok := resolveRefToDict(d.objects, ref)
				if !ok || dictGetName(fd, "/Subtype") != "/Type0" {
					continue
				}
				curNum, curFI, have = ref.Num, resolveFont(d.objects, fd), true
			case "Tj", "'", "\"":
				if len(op.Operands) > 0 {
					show(op.Operands[len(op.Operands)-1])
				}
			case "TJ":
				if len(op.Operands) == 0 {
					continue
				}
				arr, ok := op.Operands[0].(pdfArray)
				if !ok {
					continue
				}
				for _, el := range arr {
					show(el)
				}
			}
		}
	})
	return out
}

// embedCompositeFonts gives every non-embedded composite (Type0) font the
// document draws with a font program of its own, so the PDF/A embedding rule
// is satisfied for CJK text. The face comes from the font repository — a
// registered one first, then the installed face the renderer would use for the
// same document — and only the glyphs actually drawn are embedded, since a
// whole CJK font runs to tens of megabytes. Glyphs are reached the way the
// renderer reaches them: CID → Unicode (through the Adobe ordering table or
// the /ToUnicode CMap) → glyph in the substitute.
//
// A font whose licensing bits forbid embedding, one with no installed face,
// and one whose CIDs do not resolve to Unicode are left alone and stay a
// reported violation.
func (d *Document) embedCompositeFonts() {
	for num, cids := range d.pdfaUsedCIDs() {
		obj, ok := d.objects[num]
		if !ok {
			continue
		}
		dict, ok := obj.Value.(pdfDict)
		if !ok || pdfaFontEmbedded(d.objects, dict) {
			continue
		}
		fi := resolveFont(d.objects, dict)
		sys := fontRepo.findCJK(fi, fi.ordering)
		if sys == nil || !sys.embeddingAllowed() {
			continue
		}
		cidFont, ok := d.buildEmbeddedCompositeFont(dict, fi, sys, cids)
		if !ok {
			continue
		}
		descs, ok := resolveRefToArray(d.objects, dict["/DescendantFonts"])
		if !ok || len(descs) == 0 {
			continue
		}
		ref, ok := descs[0].(pdfRef)
		if !ok {
			continue
		}
		d.objects[ref.Num].Value = cidFont
		dict["/BaseFont"] = cidFont["/BaseFont"]
	}
}

// buildEmbeddedCompositeFont subsets sys to the glyphs the used CIDs need and
// returns the replacement CIDFont dictionary: the subset program as
// /FontFile2, a /CIDToGIDMap translating the document CIDs into the subset
// glyphs, and /W widths taken from the subset itself, so the dictionary and
// the program agree as PDF/A requires.
func (d *Document) buildEmbeddedCompositeFont(type0 pdfDict, fi fontInfo, sys *ttfFont, cids map[uint16]bool) (pdfDict, bool) {
	// A collection sub-font tables do not start at the head of the file, so
	// re-wrap it as a standalone sfnt before subsetting.
	if len(sys.data) > 4 && binary.BigEndian.Uint32(sys.data[0:4]) == 0x74746366 {
		data, err := standaloneSFNT(sys)
		if err != nil {
			return nil, false
		}
		wrapped, err := parseTTF(data)
		if err != nil {
			return nil, false
		}
		wrapped.postScriptName = sys.postScriptName
		sys = wrapped
	}

	cidGID := make(map[uint16]uint16, len(cids))
	glyphs := map[uint16]bool{}
	var maxCID uint16
	for cid := range cids {
		r := rune(0)
		if fi.cidToUni != nil {
			r = fi.cidToUni[cid]
		}
		if r == 0 && fi.toUnicode != nil {
			r = fi.toUnicode[cid]
		}
		if r == 0 {
			continue
		}
		gid := sys.glyphID(r)
		if gid == 0 {
			continue
		}
		cidGID[cid] = gid
		glyphs[gid] = true
		if cid > maxCID {
			maxCID = cid
		}
	}
	if len(cidGID) == 0 {
		return nil, false
	}

	res, err := subsetTTF(sys, glyphs)
	if err != nil {
		return nil, false
	}
	subsetGID := func(orig uint16) uint16 {
		if int(orig)*2+2 > len(res.cidToGID) {
			return 0
		}
		return binary.BigEndian.Uint16(res.cidToGID[int(orig)*2:])
	}
	scale := func(w uint16) int {
		return int(float64(w)*1000.0/float64(sys.unitsPerEm) + 0.5)
	}

	cidMap := make([]byte, (int(maxCID)+1)*2) // /CIDToGIDMap: two bytes per CID
	widths := make(pdfArray, 0, len(cidGID)*2)
	for cid, gid := range cidGID {
		binary.BigEndian.PutUint16(cidMap[int(cid)*2:], subsetGID(gid))
		w := defaultCIDWidth
		if int(gid) < len(sys.glyphWidths) {
			w = scale(sys.glyphWidths[gid])
		}
		widths = append(widths, int(cid), pdfArray{w})
	}

	fontFileID := d.addObject(buildFontFile2StreamBytes(res.program))
	descriptorFont := sys
	if subset, err := parseTTF(res.program); err == nil {
		subset.postScriptName = sys.postScriptName
		descriptorFont = subset
	}
	descID := d.addObject(buildFontDescriptor(descriptorFont, "/FontFile2", fontFileID))
	mapID := d.addObject(buildFlateStream(cidMap))

	out := pdfDict{
		"/Type":           pdfName("/Font"),
		"/Subtype":        pdfName("/CIDFontType2"),
		"/BaseFont":       pdfName("/" + sys.postScriptName),
		"/FontDescriptor": pdfRef{Num: descID},
		"/CIDToGIDMap":    pdfRef{Num: mapID},
		"/W":              widths,
		"/DW":             defaultCIDWidth,
	}
	// The CIDSystemInfo has to keep matching the encoding CMap.
	if descs, ok := resolveRefToArray(d.objects, type0["/DescendantFonts"]); ok && len(descs) > 0 {
		if old, ok := resolveRefToDict(d.objects, descs[0]); ok {
			if csi, ok := old["/CIDSystemInfo"]; ok {
				out["/CIDSystemInfo"] = csi
			}
		}
	}
	if _, ok := out["/CIDSystemInfo"]; !ok {
		out["/CIDSystemInfo"] = pdfDict{
			"/Registry": "Adobe", "/Ordering": "Identity", "/Supplement": 0,
		}
	}
	return out, true
}
