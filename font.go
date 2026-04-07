package asposepdf

// fontInfo holds the resolved encoding for a PDF font.
type fontInfo struct {
	name     string    // /BaseFont value, e.g. "/Helvetica"
	encoding [256]rune // character code → Unicode rune
	known    bool      // false if encoding could not be determined
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
			return fi
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
		return fi
	}

	if !hasEncoding {
		if isStandard14(name) {
			fi.encoding = defaultEncodingForFont(name)
			fi.known = true
			return fi
		}
	}

	// Unknown encoding — fill with U+FFFD.
	for i := range fi.encoding {
		fi.encoding[i] = '\uFFFD'
	}
	fi.known = false
	return fi
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
