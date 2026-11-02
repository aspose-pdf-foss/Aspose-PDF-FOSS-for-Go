// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/binary"
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"
	"strings"
)

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
//
// Several Type0 fonts may share one CIDFont, so the fonts are grouped by it: the
// one program embedded has to cover what all of them draw.
func (d *Document) embedCompositeFonts() {
	type group struct {
		fonts []pdfDict
		cids  map[uint16]bool
	}
	groups := map[int]*group{}
	used := d.pdfaUsedCIDs()
	for _, num := range slices.Sorted(maps.Keys(used)) {
		obj, ok := d.objects[num]
		if !ok {
			continue
		}
		dict, ok := obj.Value.(pdfDict)
		if !ok || pdfaFontEmbedded(d.objects, dict) {
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
		g := groups[ref.Num]
		if g == nil {
			g = &group{cids: map[uint16]bool{}}
			groups[ref.Num] = g
		}
		g.fonts = append(g.fonts, dict)
		maps.Copy(g.cids, used[num])
	}
	for _, descNum := range slices.Sorted(maps.Keys(groups)) {
		g := groups[descNum]
		fi := resolveFont(d.objects, g.fonts[0])
		sys := fontRepo.findCJK(fi, fi.ordering)
		if sys == nil || !sys.embeddingAllowed() {
			continue
		}
		// Latin codes a CMap sends to CIDs the ordering table leaves without
		// Unicode, which the renderer reaches the same way.
		latin := map[uint16]rune{}
		for _, font := range g.fonts {
			maps.Copy(latin, cjkASCIIFallback(resolveFont(d.objects, font).cidCMap, fi.cidToUni))
		}
		cidFont, ok := d.buildEmbeddedCompositeFont(g.fonts[0], fi, sys, g.cids, latin)
		if !ok {
			continue
		}
		d.objects[descNum].Value = cidFont
		for _, font := range g.fonts {
			font["/BaseFont"] = cidFont["/BaseFont"]
		}
	}
}

// buildEmbeddedCompositeFont subsets sys to the glyphs the used CIDs need and
// returns the replacement CIDFont dictionary: the subset program as
// /FontFile2, a /CIDToGIDMap translating the document CIDs into the subset
// glyphs, and /W widths taken from the subset itself, so the dictionary and
// the program agree as PDF/A requires.
func (d *Document) buildEmbeddedCompositeFont(type0 pdfDict, fi fontInfo, sys *ttfFont, cids map[uint16]bool, latin map[uint16]rune) (pdfDict, bool) {
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
		if r == 0 {
			r = latin[cid]
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
	// Text is laid out by the document's widths, so they stay and the subset's
	// advances follow; the lower CID decides for a glyph two CIDs share.
	sortedCIDs := slices.Sorted(maps.Keys(cidGID))
	docWidth := func(cid uint16) (float64, bool) {
		w, ok := fi.cidWidths[cid]
		return w, ok
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
	advances := map[uint16]float64{}
	for _, cid := range sortedCIDs {
		gid := cidGID[cid]
		binary.BigEndian.PutUint16(cidMap[int(cid)*2:], subsetGID(gid))
		var w any = defaultCIDWidth
		if int(gid) < len(sys.glyphWidths) {
			w = scale(sys.glyphWidths[gid])
		}
		if dw, ok := docWidth(cid); ok && dw > 0 {
			w = dw
			if _, seen := advances[subsetGID(gid)]; !seen {
				advances[subsetGID(gid)] = dw
			}
		}
		widths = append(widths, int(cid), pdfArray{w})
	}
	if subset, err := parseTTF(res.program); err == nil {
		if fitted := fitAdvances(subset, advances); fitted != subset {
			res.program = fitted.data
		}
	}

	fontFileID := d.addObject(buildFontFile2StreamBytes(res.program))
	descriptorFont := sys
	if subset, err := parseTTF(res.program); err == nil {
		subset.postScriptName = sys.postScriptName
		descriptorFont = subset
	}
	descID := d.addObject(buildFontDescriptor(descriptorFont, "/FontFile2", fontFileID, true))
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

// documentAdvances returns, per glyph of the substitute f, the advance (1/1000 em)
// the document's own /Widths give the codes it draws through it. When two codes
// reach one glyph with different widths the lower code decides, and the validator
// reports the other.
func (d *Document) documentAdvances(orig pdfDict, fi fontInfo, f *ttfFont, usedCodes map[uint32]bool) map[uint16]float64 {
	arr, ok := resolveRefToArray(d.objects, orig["/Widths"])
	if !ok {
		return nil // the standard metrics, which the bundled clones share
	}
	first := dictGetInt(orig, "/FirstChar")
	out := map[uint16]float64{}
	for _, code := range slices.Sorted(maps.Keys(usedCodes)) {
		i := int(code) - first
		if i < 0 || i >= len(arr) || code > 255 {
			continue
		}
		w := operandFloat(resolveRef(d.objects, arr[i]))
		gid := substituteGlyphFor(f, fi, int(code))
		if _, seen := out[gid]; !seen && gid != 0 && w > 0 {
			out[gid] = w
		}
	}
	return out
}

// fitAdvances returns f with the given glyphs' advances (1/1000 em) in its hmtx,
// or f itself when they already agree. Only the advance changes: the outlines stay
// as they are, and a viewer draws each one where /Widths puts it.
func fitAdvances(f *ttfFont, advances map[uint16]float64) *ttfFont {
	changed := false
	for gid, w := range advances {
		if int(gid) < len(f.glyphWidths) && math.Abs(float64(f.glyphWidths[gid])*1000/float64(f.unitsPerEm)-w) > 0.5 {
			changed = true
		}
	}
	if !changed {
		return f
	}
	data, err := setAdvances(f.data, f.glyphWidths, f.unitsPerEm, advances)
	if err != nil {
		return f
	}
	out, err := parseTTF(data)
	if err != nil {
		return f
	}
	out.postScriptName = f.postScriptName
	return out
}

// setAdvances rewrites an sfnt's hmtx with the given advances (1/1000 em), every
// glyph getting a full record, and hhea to match.
func setAdvances(data []byte, current []uint16, unitsPerEm uint16, advances map[uint16]float64) ([]byte, error) {
	dir := tableDir(data)
	hmtx, hhea := tableSlice(data, dir, "hmtx"), tableSlice(data, dir, "hhea")
	if len(hhea) < 36 || len(hmtx) == 0 {
		return nil, fmt.Errorf("font has no usable hmtx")
	}
	nLong := int(binary.BigEndian.Uint16(hhea[34:36]))
	out := make([]byte, len(current)*4)
	var maxAdvance uint16
	for g, adv := range current {
		if w, ok := advances[uint16(g)]; ok {
			adv = uint16(w*float64(unitsPerEm)/1000 + 0.5)
		}
		maxAdvance = max(maxAdvance, adv)
		var lsb []byte
		switch {
		case g < nLong && g*4+4 <= len(hmtx):
			lsb = hmtx[g*4+2 : g*4+4]
		case nLong*4+(g-nLong)*2+2 <= len(hmtx):
			lsb = hmtx[nLong*4+(g-nLong)*2 : nLong*4+(g-nLong)*2+2]
		default:
			lsb = []byte{0, 0}
		}
		binary.BigEndian.PutUint16(out[g*4:], adv)
		copy(out[g*4+2:], lsb)
	}
	newHhea := slices.Clone(hhea)
	binary.BigEndian.PutUint16(newHhea[10:12], maxAdvance)
	binary.BigEndian.PutUint16(newHhea[34:36], uint16(len(current)))
	tbls := make(map[string][]byte, len(dir))
	for tag := range dir {
		tbls[tag] = tableSlice(data, dir, tag)
	}
	tbls["hmtx"], tbls["hhea"] = out, newHhea
	return assembleSFNT(tbls), nil
}

// simpleToUnicode builds a /ToUnicode CMap from a simple font's encoding; nil when
// the encoding says nothing.
func simpleToUnicode(fi fontInfo) *pdfStream {
	var lines []string
	for c, r := range fi.encoding {
		if r != 0 && r != 0xFFFD {
			lines = append(lines, fmt.Sprintf("<%02X> <%04X>", c, r))
		}
	}
	if len(lines) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n1 begincodespacerange\n<00> <FF>\nendcodespacerange\n")
	for len(lines) > 0 {
		n := min(100, len(lines)) // a section holds at most 100 entries
		fmt.Fprintf(&b, "%d beginbfchar\n%s\nendbfchar\n", n, strings.Join(lines[:n], "\n"))
		lines = lines[n:]
	}
	b.WriteString("endcmap\nend\nend\n")
	return buildFlateStream([]byte(b.String()))
}

// embedPredefinedCMaps writes the CMaps of the used composite fonts into the file,
// which PDF/A-1 requires of every CMap but Identity (ISO 19005-1 §6.3.3.3).
func (d *Document) embedPredefinedCMaps() {
	embedded := map[string]pdfRef{}
	var embed func(name string) (pdfRef, bool)
	embed = func(name string) (pdfRef, bool) {
		if ref, ok := embedded[name]; ok {
			return ref, true
		}
		data, err := readGzipEmbed("cjk_cmaps/" + name + ".cmap.gz")
		if err != nil {
			return pdfRef{}, false
		}
		cm := predefinedCMap(name)
		if cm == nil {
			return pdfRef{}, false
		}
		st := buildFlateStream(data)
		st.Dict["/Type"] = pdfName("/CMap")
		st.Dict["/CMapName"] = pdfName("/" + name)
		st.Dict["/CIDSystemInfo"] = pdfDict{"/Registry": cm.registry, "/Ordering": cm.ordering, "/Supplement": cm.supplement}
		st.Dict["/WMode"] = cm.wmode
		if m := reCMapUse.FindSubmatch(data); m != nil {
			if parent, ok := embed(string(m[1])); ok {
				st.Dict["/UseCMap"] = parent
			}
		}
		ref := pdfRef{Num: d.addObject(st)}
		embedded[name] = ref
		return ref, true
	}
	fonts, _ := d.compositeFontsInUse()
	for _, font := range fonts {
		enc, ok := resolveRef(d.objects, font["/Encoding"]).(pdfName)
		if !ok || enc == "/Identity-H" || enc == "/Identity-V" {
			continue
		}
		if ref, ok := embed(strings.TrimPrefix(string(enc), "/")); ok {
			font["/Encoding"] = ref
		}
	}
}

// reCMapUse finds the CMap a CMap file builds on ("/H usecmap").
var reCMapUse = regexp.MustCompile(`/([A-Za-z0-9-]+)\s+usecmap`)
