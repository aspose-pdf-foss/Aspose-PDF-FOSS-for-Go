package asposepdf

// fontInfo holds the resolved encoding for a PDF font.
type fontInfo struct {
	name     string        // /BaseFont value, e.g. "/Helvetica"
	encoding [256]rune     // character code → Unicode rune
	widths   [256]float64  // character code → width in 1/1000 text space units
	known    bool          // false if encoding could not be determined
}

// resolveFont resolves a font dictionary to a fontInfo.
// objects is needed to resolve indirect references in /Encoding.
func resolveFont(objects map[int]*pdfObject, fontDict pdfDict) fontInfo {
	name := dictGetName(fontDict, "/BaseFont")
	fi := fontInfo{name: name}

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
