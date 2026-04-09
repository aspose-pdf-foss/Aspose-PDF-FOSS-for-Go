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

	// Find end: whitespace + "EI" + delimiter.
	start := l.pos
	for l.pos < len(l.data)-2 {
		if isWhitespace(l.data[l.pos]) &&
			l.data[l.pos+1] == 'E' && l.data[l.pos+2] == 'I' &&
			(l.pos+3 >= len(l.data) || isDelimiter(l.data[l.pos+3])) {
			data := l.data[start:l.pos]
			l.pos += 3
			return normalizeInlineDict(dict), data
		}
		l.pos++
	}
	l.pos = len(l.data)
	return nil, nil
}
