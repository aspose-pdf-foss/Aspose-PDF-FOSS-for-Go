// SPDX-License-Identifier: MIT

package asposepdf

import (
	"maps"
	"strings"
)

// fontInfo holds the resolved encoding for a PDF font.
type fontInfo struct {
	name      string             // /BaseFont value, e.g. "/Helvetica"
	encoding  [256]rune          // character code → Unicode rune (single-byte fonts)
	widths    [256]float64       // character code → width in 1/1000 text space units
	toUnicode map[uint16]rune    // ToUnicode CMap mapping (glyph ID -> Unicode)
	cidWidths map[uint16]float64 // CID widths from /W array
	// toUnicodeSeq holds the codes whose ToUnicode destination is several
	// characters — a ligature glyph mapped to "fi" — with the full sequence.
	toUnicodeSeq map[uint16][]rune
	defaultW     float64         // /DW default width for CIDFont (1000 if absent)

	// Vertical writing (ISO 32000-1 §9.7.4.3): a composite font whose CMap has
	// WMode 1 advances down the page, each glyph hung from its position vector.
	vertical  bool
	dw2       [2]float64            // /DW2: the position vector's y and the advance
	w2        map[uint16][3]float64 // /W2 per CID: advance, position vector x and y
	vertForms map[uint16]rune       // vertical CID -> its presentation form, for drawing
	isType0      bool            // true = two-byte character codes (composite font)
	cidCMap      *cidCMap        // Type0 /Encoding CMap (predefined or embedded); nil = Identity
	cidToUni     map[uint16]rune // CID → Unicode (Adobe ordering table); nil if unknown
	ordering     string          // CIDSystemInfo /Ordering (e.g. "GB1"); "" = Identity/unknown
	known        bool            // false if encoding could not be determined
	// glyphNames maps codes remapped by /Encoding /Differences to their raw
	// glyph names — needed when a name has no Unicode meaning (ZapfDingbats
	// a1..a191) and the glyph must be reached by name through the font's
	// charset rather than through a rune. nil when no /Differences.
	glyphNames map[byte]string
	bold       bool
	italic     bool
	serif      bool    // /FontDescriptor /Flags Serif bit (used to pick a substitute)
	ascent     float64 // from /FontDescriptor /Ascent (in 1/1000 text space)
	descent    float64 // from /FontDescriptor /Descent (negative, in 1/1000 text space)
}

// defaultFontInfo returns the fontInfo viewers substitute when a content
// stream names a font that the resources do not declare (e.g. an empty
// /Resources/Font dict, 44963.pdf): Standard-14 Helvetica with AFM metrics.
func defaultFontInfo(objects map[int]*pdfObject) fontInfo {
	return resolveFont(objects, pdfDict{
		"/Type":     pdfName("/Font"),
		"/Subtype":  pdfName("/Type1"),
		"/BaseFont": pdfName("/Helvetica"),
	})
}

// resolveFont resolves a font dictionary to a fontInfo.
// objects is needed to resolve indirect references in /Encoding.
func resolveFont(objects map[int]*pdfObject, fontDict pdfDict) fontInfo {
	name := dictGetName(fontDict, "/BaseFont")
	fi := fontInfo{name: name, defaultW: 1000}

	// Detect Type0 (composite) font.
	subtype := dictGetName(fontDict, "/Subtype")
	if subtype == "/Type0" {
		fi.isType0 = true
	}

	// Parse /ToUnicode CMap if present (works for any font type).
	if tuVal, ok := fontDict["/ToUnicode"]; ok {
		resolved := resolveRef(objects, tuVal)
		if stream, ok := resolved.(*pdfStream); ok {
			fi.toUnicode, fi.toUnicodeSeq = parseCMapFull(stream.Data)
			if len(fi.toUnicode) > 0 {
				fi.known = true
			}
		}
	}

	// Resolve /FontDescriptor for bold/italic flags and ascent/descent.
	resolveFontDescriptor(objects, fontDict, name, &fi)
	if fi.ascent == 0 && fi.descent == 0 {
		if a, d, ok := standard14VerticalMetrics(name); ok {
			fi.ascent, fi.descent = a, d
		}
	}

	// For Type0: toUnicode and known are already set above.
	// Resolve descendant CIDFont for widths, then return.
	if fi.isType0 {
		fi.cidWidths, fi.defaultW = resolveCIDWidths(objects, fontDict)
		resolveType0Encoding(objects, fontDict, &fi)
		return fi
	}

	// --- single-byte encoding logic ---
	encVal, hasEncoding := fontDict["/Encoding"]
	if hasEncoding {
		encVal = resolveRef(objects, encVal)
	}

	switch enc := encVal.(type) {
	case pdfName:
		if tbl, ok := lookupEncoding(string(enc)); ok {
			fi.encoding = tbl
			fi.known = true
		}
	case pdfDict:
		baseName := dictGetName(enc, "/BaseEncoding")
		base, ok := lookupEncoding(baseName)
		if !ok {
			base = standardEncoding
		}
		if diffs, ok := enc["/Differences"]; ok {
			if arr, ok := diffs.(pdfArray); ok {
				base = applyDifferences(base, arr)
				fi.glyphNames = differencesGlyphNames(arr)
			}
		}
		fi.encoding = base
		fi.known = true
	}

	if !fi.known && !hasEncoding {
		if isStandard14(name) {
			fi.encoding = defaultEncodingForFont(name)
			fi.known = true
		} else if canon, ok := standard14WidthAlias(name); ok && !fontDescriptorHasFile(objects, fontDict) {
			// A non-embedded font named by a common alias of a Standard-14
			// face (Arial, Times New Roman, Courier New, …) is drawn by
			// every viewer — and by our own renderer and width tables — as
			// that face, so it reads with the same built-in encoding
			// (pdf-go-tnsb). An embedded program is excluded: its own
			// built-in encoding is unrelated to its name, and marking it
			// known would make the renderer prefer the guess over the
			// program's cmap.
			fi.encoding = defaultEncodingForFont(canon)
			fi.known = true
		} else {
			for i := range fi.encoding {
				fi.encoding[i] = '\uFFFD'
			}
		}
	}

	fi.widths = resolveWidths(objects, fontDict, name)

	// Type3 widths are expressed in glyph space and must be mapped through
	// /FontMatrix (ISO 32000-1 §9.6.5) — e.g. matrix .0133 with width 38 is
	// half an em, not 38/1000. Normalize them to the standard 1000-units-per-em
	// convention every consumer (extractor advance, renderer tx) assumes.
	if dictGetName(fontDict, "/Subtype") == "/Type3" {
		if fm := shFloats(objects, fontDict["/FontMatrix"]); len(fm) == 6 && fm[0] != 0 {
			scale := fm[0] * 1000
			for i := range fi.widths {
				fi.widths[i] *= scale
			}
		}
	}
	return fi
}

// resolveType0Encoding resolves a composite font's /Encoding (a predefined
// CMap name like /GBK-EUC-H, or an embedded CMap stream) into a code→CID map,
// and the descendant's CIDSystemInfo /Ordering into a CID→Unicode table. This
// is what lets a non-embedded CJK font (no /FontFile2, no /ToUnicode) extract
// and render: codes → CIDs → Unicode → glyphs from a substitute/system font.
// Identity-H/V keep code == CID (cidCMap stays nil), matching prior behavior.
func resolveType0Encoding(objects map[int]*pdfObject, type0Dict pdfDict, fi *fontInfo) {
	encVal, ok := type0Dict["/Encoding"]
	if ok {
		encVal = resolveRef(objects, encVal)
	}
	switch enc := encVal.(type) {
	case pdfName:
		name := string(enc)
		if name != "/Identity-H" && name != "/Identity-V" && name != "/Identity" {
			fi.cidCMap = predefinedCMap(strings.TrimPrefix(name, "/"))
		}
	case *pdfStream:
		data := enc.Data
		if !enc.Decoded {
			if d, err := decodeStream(enc.Dict, enc.Data); err == nil {
				data = d
			}
		}
		fi.cidCMap = parseCIDCMap(data, predefinedCMap)
	}

	// CIDSystemInfo /Ordering → CID→Unicode table (Adobe ordering). Valid both
	// for a predefined/embedded CMap (codes → Adobe CIDs) and for Identity-H when
	// the descendant is a real Adobe-ordered CIDFont (the 2-byte code is the
	// Adobe CID), e.g. a non-embedded Yu Gothic with /Ordering (Japan1).
	fi.ordering = cidSystemOrdering(objects, type0Dict)
	if fi.ordering != "" {
		fi.cidToUni = cidToUnicodeForOrdering(fi.ordering)
	}
	if (fi.cidCMap != nil || fi.cidToUni != nil) && fi.toUnicode == nil {
		fi.known = true
	}
	resolveVertical(objects, type0Dict, encVal, fi)
}

// resolveVertical reads the vertical writing mode and metrics. A vertical CID
// extracts as the character it stands for, so "（" finds it, while its presentation
// form is kept for the glyph.
func resolveVertical(objects map[int]*pdfObject, type0Dict pdfDict, enc pdfValue, fi *fontInfo) {
	switch v := enc.(type) {
	case pdfName:
		fi.vertical = v == "/Identity-V"
	case *pdfStream:
		fi.vertical = dictGetInt(v.Dict, "/WMode") == 1
	}
	if fi.cidCMap != nil && fi.cidCMap.wmode == 1 {
		fi.vertical = true
	}
	if !fi.vertical {
		return
	}
	fi.dw2 = [2]float64{880, -1000}
	if descs, ok := resolveRefToArray(objects, type0Dict["/DescendantFonts"]); ok && len(descs) > 0 {
		if cidDict, ok := resolveRefToDict(objects, descs[0]); ok {
			if a, ok := resolveRefToArray(objects, cidDict["/DW2"]); ok && len(a) >= 2 {
				fi.dw2 = [2]float64{operandFloat(resolveRef(objects, a[0])), operandFloat(resolveRef(objects, a[1]))}
			}
			if a, ok := resolveRefToArray(objects, cidDict["/W2"]); ok {
				fi.w2 = parseCIDVerticalArray(objects, a)
			}
		}
	}
	if fi.cidCMap == nil || fi.cidToUni == nil {
		return
	}
	bases := fi.cidCMap.verticalBases()
	if len(bases) == 0 {
		return
	}
	uni := make(map[uint16]rune, len(fi.cidToUni)+len(bases))
	maps.Copy(uni, fi.cidToUni)
	fi.vertForms = map[uint16]rune{}
	for cid, base := range bases {
		if r := fi.cidToUni[cid]; r != 0 {
			fi.vertForms[cid] = r
		}
		if r := fi.cidToUni[base]; r != 0 {
			uni[cid] = r
		}
	}
	fi.cidToUni = uni
}

// parseCIDVerticalArray parses a /W2 array: "c [w1y vx vy ...]" or
// "cfirst clast w1y vx vy" (ISO 32000-1 §9.7.4.3).
func parseCIDVerticalArray(objects map[int]*pdfObject, arr pdfArray) map[uint16][3]float64 {
	out := map[uint16][3]float64{}
	num := func(v pdfValue) float64 { return operandFloat(resolveRef(objects, v)) }
	for i := 0; i+1 < len(arr); {
		first := int(num(arr[i]))
		if list, ok := resolveRef(objects, arr[i+1]).(pdfArray); ok {
			for j := 0; j+2 < len(list); j += 3 {
				out[uint16(first+j/3)] = [3]float64{num(list[j]), num(list[j+1]), num(list[j+2])}
			}
			i += 2
			continue
		}
		if i+4 >= len(arr) {
			break
		}
		last := int(num(arr[i+1]))
		m := [3]float64{num(arr[i+2]), num(arr[i+3]), num(arr[i+4])}
		for c := first; c <= last && c <= 0xFFFF; c++ {
			out[uint16(c)] = m
		}
		i += 5
	}
	return out
}

// verticalMetrics returns a CID's advance and position vector, in 1/1000 em. The
// default vector's x is half the glyph's width, centring it on the line.
func (fi *fontInfo) verticalMetrics(cid uint16) (w1y, vx, vy float64) {
	if m, ok := fi.w2[cid]; ok {
		return m[0], m[1], m[2]
	}
	w0 := fi.defaultW
	if w, ok := fi.cidWidths[cid]; ok {
		w0 = w
	}
	return fi.dw2[1], w0 / 2, fi.dw2[0]
}

// verticalDisplacement is the move from a text position to the glyph's origin.
func (fi *fontInfo) verticalDisplacement(cid uint16, fontSize, horizScale float64) [6]float64 {
	_, vx, vy := fi.verticalMetrics(cid)
	return translateMatrix(-vx/1000*fontSize*horizScale, -vy/1000*fontSize)
}

// textAdvanceMatrix moves a text matrix along the writing direction.
func textAdvanceMatrix(fi *fontInfo, adv float64) [6]float64 {
	if fi != nil && fi.vertical {
		return translateMatrix(0, adv)
	}
	return translateMatrix(adv, 0)
}

// glyphCentre is the point of a glyph a distance along the writing direction from
// the text position: horizontally the baseline; vertically the glyph hangs centred
// on the line, as the default position vector places it.
func glyphCentre(fi *fontInfo, m [6]float64, along, rise float64) (float64, float64) {
	if fi != nil && fi.vertical {
		return matApplyPoint(m, 0, along+rise)
	}
	return matApplyPoint(m, along, rise)
}

// cidSystemOrdering returns the descendant CIDFont's CIDSystemInfo /Ordering
// (e.g. "GB1", "Japan1"), or "" if absent.
func cidSystemOrdering(objects map[int]*pdfObject, type0Dict pdfDict) string {
	descVal, ok := type0Dict["/DescendantFonts"]
	if !ok {
		return ""
	}
	descArr, ok := resolveRef(objects, descVal).(pdfArray)
	if !ok || len(descArr) == 0 {
		return ""
	}
	cidDict, ok := resolveRefToDict(objects, descArr[0])
	if !ok {
		return ""
	}
	csiDict, ok := resolveRefToDict(objects, cidDict["/CIDSystemInfo"])
	if !ok {
		return ""
	}
	if s, ok := csiDict["/Ordering"].(string); ok {
		return s
	}
	return ""
}

// resolveCIDWidths extracts /DW and /W from the CIDFont descendant.
func resolveCIDWidths(objects map[int]*pdfObject, type0Dict pdfDict) (map[uint16]float64, float64) {
	widths := make(map[uint16]float64)
	defaultW := 1000.0

	descVal, ok := type0Dict["/DescendantFonts"]
	if !ok {
		return widths, defaultW
	}
	descResolved := resolveRef(objects, descVal)
	descArr, ok := descResolved.(pdfArray)
	if !ok || len(descArr) == 0 {
		return widths, defaultW
	}
	cidDict, ok := resolveRefToDict(objects, descArr[0])
	if !ok {
		return widths, defaultW
	}

	if dw, ok := cidDict["/DW"]; ok {
		defaultW = operandFloat(dw)
	}

	if wVal, ok := cidDict["/W"]; ok {
		wResolved := resolveRef(objects, wVal)
		if wArr, ok := wResolved.(pdfArray); ok {
			parseCIDWidthArray(objects, wArr, widths)
		}
	}

	return widths, defaultW
}

// parseCIDWidthArray parses a /W array into a map.
// The /W array has two forms:
//   - c [w1 w2 ...] — individual widths starting at CID c
//   - c_first c_last w — all CIDs in [c_first, c_last] get width w
//
// Elements may be indirect references (notably the width sub-array,
// e.g. /W [0 9 0 R]), so each is resolved before inspection.
func parseCIDWidthArray(objects map[int]*pdfObject, arr pdfArray, widths map[uint16]float64) {
	i := 0
	for i < len(arr) {
		cidStart, ok := pdfValueToInt(arr[i])
		if !ok {
			i++
			continue
		}
		i++
		if i >= len(arr) {
			break
		}
		switch v := resolveRef(objects, arr[i]).(type) {
		case pdfArray:
			for j, w := range v {
				widths[uint16(cidStart+j)] = operandFloat(w)
			}
			i++
		default:
			cidEnd, ok := pdfValueToInt(arr[i])
			if !ok {
				i++
				continue
			}
			i++
			if i >= len(arr) {
				break
			}
			w := operandFloat(arr[i])
			i++
			for c := cidStart; c <= cidEnd; c++ {
				widths[uint16(c)] = w
			}
		}
	}
}

// pdfValueToInt converts a pdfValue to int.
func pdfValueToInt(v pdfValue) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	}
	return 0, false
}

// resolveWidths extracts glyph widths from a font dictionary.
// It tries /Widths+/FirstChar+/LastChar first, then Standard 14 metrics,
// then falls back to monospaced 600 units.
func resolveWidths(objects map[int]*pdfObject, fontDict pdfDict, baseFontName string) [256]float64 {
	var widths [256]float64

	// Try /Widths + /FirstChar + /LastChar from font dict.
	if wVal, ok := fontDict["/Widths"]; ok {
		firstChar := dictGetInt(fontDict, "/FirstChar")
		lastChar := dictGetInt(fontDict, "/LastChar")
		wResolved := resolveRef(objects, wVal)
		if arr, ok := wResolved.(pdfArray); ok {
			for i, v := range arr {
				code := firstChar + i
				if code >= 0 && code < 256 && i <= lastChar-firstChar {
					widths[code] = operandFloat(v)
				}
			}
			return widths
		}
	}

	// Fallback: Standard 14 built-in metrics.
	if std, ok := standard14Widths(baseFontName); ok {
		return std
	}

	// Last resort: monospaced fallback.
	for i := 32; i < 256; i++ {
		widths[i] = 600
	}
	return widths
}

// lookupEncoding returns the encoding table for a named encoding.
func lookupEncoding(name string) ([256]rune, bool) {
	switch name {
	case "/WinAnsiEncoding":
		return winAnsiEncoding, true
	case "/MacRomanEncoding":
		return macRomanEncoding, true
	case "/StandardEncoding":
		return standardEncoding, true
	case "/MacExpertEncoding":
		return macExpertEncoding, true
	default:
		return [256]rune{}, false
	}
}

// isStandard14 reports whether the font name is one of the 14 standard PDF fonts.
func isStandard14(name string) bool {
	switch name {
	case "/Courier", "/Courier-Bold", "/Courier-Oblique", "/Courier-BoldOblique",
		"/Helvetica", "/Helvetica-Bold", "/Helvetica-Oblique", "/Helvetica-BoldOblique",
		"/Times-Roman", "/Times-Bold", "/Times-Italic", "/Times-BoldItalic",
		"/Symbol", "/ZapfDingbats":
		return true
	}
	return false
}

// defaultEncodingForFont returns the default encoding for a standard 14 font.
func defaultEncodingForFont(name string) [256]rune {
	switch name {
	case "/Symbol":
		return symbolEncoding
	case "/ZapfDingbats":
		return zapfDingbatsEncoding
	default:
		return standardEncoding
	}
}

// nameImpliesBold reports whether a (lower-cased) font name carries a weight
// keyword that reads as bold or heavier — including the foundry weight words
// (demi, semibold, black, heavy) that "bold" alone would miss (e.g. the
// URWGothicL-Demi family).
func nameImpliesBold(lname string) bool {
	for _, kw := range []string{"bold", "demi", "semibold", "black", "heavy", "extrabold", "ultra"} {
		if strings.Contains(lname, kw) {
			return true
		}
	}
	return false
}

// resolveFontDescriptor reads /FontDescriptor to set bold, italic, ascent, descent.
// Bold/italic also use font name heuristics as fallback.
// PDF spec Table 123: bit 3 = Italic, bit 19 = ForceBold.
func resolveFontDescriptor(objects map[int]*pdfObject, fontDict pdfDict, baseFontName string, fi *fontInfo) {
	lname := strings.ToLower(baseFontName)
	fi.bold = nameImpliesBold(lname)
	fi.italic = strings.Contains(lname, "italic") || strings.Contains(lname, "obli")

	// Find /FontDescriptor dict.
	fdVal, ok := fontDict["/FontDescriptor"]
	if !ok {
		// For Type0, check descendant font's FontDescriptor.
		if descVal, ok := fontDict["/DescendantFonts"]; ok {
			descResolved := resolveRef(objects, descVal)
			if descArr, ok := descResolved.(pdfArray); ok && len(descArr) > 0 {
				if cidDict, ok := resolveRefToDict(objects, descArr[0]); ok {
					fdVal, ok = cidDict["/FontDescriptor"]
					if !ok {
						return
					}
				}
			}
		}
		if fdVal == nil {
			return
		}
	}

	fdDict, ok := resolveRefToDict(objects, fdVal)
	if !ok {
		return
	}

	// /Flags bits per ISO 32000-1 Table 121 (1-based bit numbers): bit 2 Serif,
	// bit 7 Italic, bit 19 ForceBold.
	flags := dictGetInt(fdDict, "/Flags")
	if flags&(1<<1) != 0 {
		fi.serif = true
	}
	if flags&(1<<6) != 0 {
		fi.italic = true
	}
	if flags&(1<<18) != 0 {
		fi.bold = true
	}
	// A non-zero /ItalicAngle means the face is slanted (oblique), even when
	// the italic flag and name keyword are absent (URWGothicL-DemiObli has
	// ItalicAngle −11, Flags 32). Pick an oblique substitute for it.
	if !fi.italic {
		if ia := operandFloat(fdDict["/ItalicAngle"]); ia != 0 {
			fi.italic = true
		}
	}

	// Read ascent/descent metrics.
	if v, ok := fdDict["/Ascent"]; ok {
		fi.ascent = operandFloat(v)
	}
	if v, ok := fdDict["/Descent"]; ok {
		fi.descent = operandFloat(v)
	}
}
