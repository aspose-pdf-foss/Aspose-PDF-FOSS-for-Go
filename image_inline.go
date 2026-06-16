// SPDX-License-Identifier: MIT

package asposepdf

// normalizeInlineDict expands abbreviated keys and values in an inline image dict.
func normalizeInlineDict(d pdfDict) pdfDict {
	norm := make(pdfDict, len(d))
	for k, v := range d {
		fullKey := expandInlineKey(k)
		norm[fullKey] = expandInlineValue(v)
	}
	return norm
}

func expandInlineKey(k string) string {
	switch k {
	case "/BPC":
		return "/BitsPerComponent"
	case "/CS":
		return "/ColorSpace"
	case "/D":
		return "/Decode"
	case "/DP":
		return "/DecodeParms"
	case "/F":
		return "/Filter"
	case "/H":
		return "/Height"
	case "/IM":
		return "/ImageMask"
	case "/W":
		return "/Width"
	default:
		return k
	}
}

func expandInlineValue(v pdfValue) pdfValue {
	n, ok := v.(pdfName)
	if !ok {
		return v
	}
	switch string(n) {
	case "/G":
		return pdfName("/DeviceGray")
	case "/RGB":
		return pdfName("/DeviceRGB")
	case "/CMYK":
		return pdfName("/DeviceCMYK")
	case "/I":
		return pdfName("/Indexed")
	case "/AHx":
		return pdfName("/ASCIIHexDecode")
	case "/A85":
		return pdfName("/ASCII85Decode")
	case "/LZW":
		return pdfName("/LZWDecode")
	case "/Fl":
		return pdfName("/FlateDecode")
	case "/RL":
		return pdfName("/RunLengthDecode")
	case "/CCF":
		return pdfName("/CCITTFaxDecode")
	case "/DCT":
		return pdfName("/DCTDecode")
	default:
		return v
	}
}

// inlineRawDataLen returns the exact byte length of an UNFILTERED inline image's
// sample data, or -1 when it cannot be determined (the image is filtered, or its
// colour space gives no fixed component count). Rows are padded to a byte
// boundary (ISO 32000-1 §8.9.5.2). The dict must already be normalized
// (full-length keys, expanded value names).
func inlineRawDataLen(d pdfDict) int {
	if f, ok := d["/Filter"]; ok {
		if arr, isArr := f.(pdfArray); !isArr || len(arr) > 0 {
			return -1 // any filter → encoded length is unknown here
		}
	}

	w := inlineInt(d["/Width"])
	h := inlineInt(d["/Height"])
	if w <= 0 || h <= 0 {
		return -1
	}

	bpc := inlineInt(d["/BitsPerComponent"])
	comps := 0
	if b, _ := d["/ImageMask"].(bool); b {
		comps, bpc = 1, 1 // stencil mask: 1 component, 1 bit
	} else {
		switch n := d["/ColorSpace"].(type) {
		case pdfName:
			switch string(n) {
			case "/DeviceGray", "/CalGray", "/G":
				comps = 1
			case "/DeviceRGB", "/CalRGB", "/RGB", "/Lab":
				comps = 3
			case "/DeviceCMYK", "/CMYK":
				comps = 4
			case "/Indexed", "/I":
				comps = 1
			}
		case pdfArray:
			if len(n) > 0 {
				if name, _ := n[0].(pdfName); name == "/Indexed" || name == "/I" {
					comps = 1
				}
			}
		}
		if comps == 0 || bpc <= 0 {
			return -1 // unknown colour space or bit depth
		}
	}

	bytesPerRow := (w*comps*bpc + 7) / 8
	return bytesPerRow * h
}

// inlineInt coerces an inline-dict numeric value to int (0 if absent/other).
func inlineInt(v pdfValue) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}

// parseInlineImage parses BI key-value pairs and image data (between ID and EI).
func parseInlineImage(l *lexer) (pdfDict, []byte) {
	dict := make(pdfDict)

	for {
		tok, err := l.Next()
		if err != nil || tok.kind == tokEOF {
			return nil, nil
		}
		if tok.kind == tokKeyword && string(tok.raw) == "ID" {
			break
		}
		if tok.kind != tokName {
			continue
		}
		key := string(tok.raw)
		valTok, err := l.Next()
		if err != nil || valTok.kind == tokEOF {
			return nil, nil
		}
		val, err := parseValueFromToken(valTok, l)
		if err != nil {
			continue
		}
		dict[key] = val
	}

	// Skip one whitespace byte after ID.
	if l.pos < len(l.data) {
		l.pos++
	}

	norm := normalizeInlineDict(dict)

	// Unfiltered image data has a known, exact byte length (rows are
	// byte-aligned). Consume precisely that many bytes and step over the
	// following EI. Scanning for EI in raw binary samples is unreliable —
	// the sample bytes routinely contain a literal "EI" (and even
	// "<whitespace>EI"), which truncates the image mid-data and swallows the
	// operators that follow (35862.pdf: 156 1-bpp mask bars, the byte before
	// each EI is image data, not whitespace).
	if n := inlineRawDataLen(norm); n >= 0 && l.pos+n <= len(l.data) {
		start := l.pos
		p := start + n
		for p < len(l.data) && isWhitespace(l.data[p]) { // optional EOL before EI
			p++
		}
		if p+1 < len(l.data) && l.data[p] == 'E' && l.data[p+1] == 'I' {
			l.pos = p + 2
			return norm, l.data[start:start+n]
		}
		// Length didn't line up with an EI (bad dimensions / unexpected
		// padding): fall through to the tolerant scan below.
	}

	// Find end: (whitespace | '>') + "EI" + delimiter. EI is normally
	// preceded by whitespace, but with an ASCIIHexDecode (/AHx) or ASCII85
	// (/A85) filter the data ends with the '>' EOD marker and "EI" follows it
	// directly (e.g. "...3F>EI"). Accept '>' as a terminator too, otherwise the
	// scan overruns the image and swallows every glyph up to the next
	// whitespace-delimited EI.
	start := l.pos
	for l.pos < len(l.data)-2 {
		if (isWhitespace(l.data[l.pos]) || l.data[l.pos] == '>') &&
			l.data[l.pos+1] == 'E' && l.data[l.pos+2] == 'I' &&
			(l.pos+3 >= len(l.data) || isDelimiter(l.data[l.pos+3])) {
			end := l.pos
			if l.data[l.pos] == '>' {
				end++ // keep the '>' EOD marker in the filtered data
			}
			data := l.data[start:end]
			l.pos += 3
			return normalizeInlineDict(dict), data
		}
		l.pos++
	}
	l.pos = len(l.data)
	return nil, nil
}
