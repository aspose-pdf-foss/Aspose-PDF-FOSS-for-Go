// SPDX-License-Identifier: MIT

package asposepdf

import "strings"

// fontInfo holds the resolved encoding for a PDF font.
type fontInfo struct {
	name      string             // /BaseFont value, e.g. "/Helvetica"
	encoding  [256]rune          // character code → Unicode rune (single-byte fonts)
	widths    [256]float64       // character code → width in 1/1000 text space units
	toUnicode map[uint16]rune    // ToUnicode CMap mapping (glyph ID -> Unicode)
	cidWidths map[uint16]float64 // CID widths from /W array
	defaultW  float64            // /DW default width for CIDFont (1000 if absent)
	isType0   bool               // true = two-byte character codes (composite font)
	known     bool               // false if encoding could not be determined
	bold      bool
	italic    bool
	ascent    float64 // from /FontDescriptor /Ascent (in 1/1000 text space)
	descent   float64 // from /FontDescriptor /Descent (negative, in 1/1000 text space)
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
			fi.toUnicode = parseCMap(stream.Data)
			if len(fi.toUnicode) > 0 {
				fi.known = true
			}
		}
	}

	// Resolve /FontDescriptor for bold/italic flags and ascent/descent.
	resolveFontDescriptor(objects, fontDict, name, &fi)

	// For Type0: toUnicode and known are already set above.
	// Resolve descendant CIDFont for widths, then return.
	if fi.isType0 {
		fi.cidWidths, fi.defaultW = resolveCIDWidths(objects, fontDict)
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
			}
		}
		fi.encoding = base
		fi.known = true
	}

	if !fi.known && !hasEncoding {
		if isStandard14(name) {
			fi.encoding = defaultEncodingForFont(name)
			fi.known = true
		} else {
			for i := range fi.encoding {
				fi.encoding[i] = '\uFFFD'
			}
		}
	}

	fi.widths = resolveWidths(objects, fontDict, name)
	return fi
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
			parseCIDWidthArray(wArr, widths)
		}
	}

	return widths, defaultW
}

// parseCIDWidthArray parses a /W array into a map.
// The /W array has two forms:
//   - c [w1 w2 ...] — individual widths starting at CID c
//   - c_first c_last w — all CIDs in [c_first, c_last] get width w
func parseCIDWidthArray(arr pdfArray, widths map[uint16]float64) {
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
		switch v := arr[i].(type) {
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

// resolveFontDescriptor reads /FontDescriptor to set bold, italic, ascent, descent.
// Bold/italic also use font name heuristics as fallback.
// PDF spec Table 123: bit 3 = Italic, bit 19 = ForceBold.
func resolveFontDescriptor(objects map[int]*pdfObject, fontDict pdfDict, baseFontName string, fi *fontInfo) {
	lname := strings.ToLower(baseFontName)
	fi.bold = strings.Contains(lname, "bold")
	fi.italic = strings.Contains(lname, "italic") || strings.Contains(lname, "oblique")

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

	flags := dictGetInt(fdDict, "/Flags")
	if flags&(1<<2) != 0 {
		fi.italic = true
	}
	if flags&(1<<18) != 0 {
		fi.bold = true
	}

	// Read ascent/descent metrics.
	if v, ok := fdDict["/Ascent"]; ok {
		fi.ascent = operandFloat(v)
	}
	if v, ok := fdDict["/Descent"]; ok {
		fi.descent = operandFloat(v)
	}
}
