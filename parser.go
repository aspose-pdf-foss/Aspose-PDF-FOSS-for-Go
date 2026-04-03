package asposepdf

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// parseValue parses one PDF value from the lexer.
func parseValue(l *lexer) (pdfValue, error) {
	tok, err := l.Next()
	if err != nil {
		return nil, err
	}
	return parseValueFromToken(tok, l)
}

func parseValueFromToken(tok token, l *lexer) (pdfValue, error) {
	switch tok.kind {
	case tokEOF:
		return nil, io.EOF
	case tokNull:
		return pdfNull{}, nil
	case tokBool:
		return string(tok.raw) == "true", nil
	case tokInt:
		n, err := strconv.Atoi(string(tok.raw))
		if err != nil {
			return nil, err
		}
		// Could be start of a reference "n g R" — peek ahead.
		return tryParseRef(n, l)
	case tokReal:
		f, err := strconv.ParseFloat(string(tok.raw), 64)
		if err != nil {
			return nil, err
		}
		return f, nil
	case tokName:
		return pdfName(tok.raw), nil
	case tokString:
		return decodeLiteralString(tok.raw), nil
	case tokHexStr:
		return decodeHexString(tok.raw), nil
	case tokArrayOpen:
		return parseArray(l)
	case tokDictOpen:
		return parseDictOrStream(l)
	case tokKeyword:
		return pdfName(tok.raw), nil // treat unknown keywords as names
	default:
		return nil, fmt.Errorf("unexpected token %q", tok.raw)
	}
}

// tryParseRef tries to parse "n g R"; if next two tokens don't fit, returns n as int.
func tryParseRef(n int, l *lexer) (pdfValue, error) {
	savedPos := l.Pos()
	tok2, err := l.Next()
	if err != nil || tok2.kind != tokInt {
		l.pos = savedPos
		return n, nil
	}
	gen, err2 := strconv.Atoi(string(tok2.raw))
	if err2 != nil {
		l.pos = savedPos
		return n, nil
	}
	tok3, err3 := l.Next()
	if err3 != nil || tok3.kind != tokKeyword || string(tok3.raw) != "R" {
		l.pos = savedPos
		return n, nil
	}
	return pdfRef{Num: n, Gen: gen}, nil
}

func parseArray(l *lexer) (pdfArray, error) {
	var arr pdfArray
	for {
		tok, err := l.Next()
		if err != nil {
			return nil, err
		}
		if tok.kind == tokArrayClose {
			break
		}
		if tok.kind == tokEOF {
			return nil, fmt.Errorf("unexpected EOF in array")
		}
		v, err := parseValueFromToken(tok, l)
		if err != nil {
			return nil, err
		}
		arr = append(arr, v)
	}
	return arr, nil
}

func parseDictOrStream(l *lexer) (pdfValue, error) {
	d, err := parseDictBody(l)
	if err != nil {
		return nil, err
	}

	// Check if this dict is followed by "stream"
	if l.peekKeyword() == "stream" {
		l.skipToStreamData()
		streamData, err := readStreamData(l, d)
		if err != nil {
			return nil, err
		}
		decoded, err := decodeStream(d, streamData)
		if err != nil {
			// Unsupported filter (e.g. DCTDecode/JPEG): keep raw bytes and
			// preserve the original /Filter so the writer copies it as-is.
			return &pdfStream{Dict: d, Data: streamData, Decoded: false}, nil
		}
		return &pdfStream{Dict: d, Data: decoded, Decoded: true}, nil
	}
	return d, nil
}

func parseDictBody(l *lexer) (pdfDict, error) {
	d := make(pdfDict)
	for {
		tok, err := l.Next()
		if err != nil {
			return nil, err
		}
		if tok.kind == tokDictClose {
			break
		}
		if tok.kind == tokEOF {
			return nil, fmt.Errorf("unexpected EOF in dict")
		}
		if tok.kind != tokName {
			return nil, fmt.Errorf("expected name in dict, got %q", tok.raw)
		}
		key := string(tok.raw)
		val, err := parseValue(l)
		if err != nil {
			return nil, fmt.Errorf("dict value for %s: %w", key, err)
		}
		d[key] = val
	}
	return d, nil
}

// readStreamData reads raw stream bytes using /Length from the dict.
func readStreamData(l *lexer, d pdfDict) ([]byte, error) {
	lengthVal, ok := d["/Length"]
	if !ok {
		return nil, fmt.Errorf("stream missing /Length")
	}
	var length int
	switch v := lengthVal.(type) {
	case int:
		length = v
	case float64:
		length = int(v)
	default:
		return nil, fmt.Errorf("unexpected /Length type %T", lengthVal)
	}
	if l.pos+length > len(l.data) {
		return nil, fmt.Errorf("stream length %d exceeds file size", length)
	}
	data := l.data[l.pos : l.pos+length]
	l.pos += length
	return data, nil
}

// decodeStream decompresses stream data based on /Filter and /DecodeParms.
func decodeStream(d pdfDict, raw []byte) ([]byte, error) {
	filterVal, ok := d["/Filter"]
	if !ok {
		return raw, nil // uncompressed
	}

	filters := toFilterList(filterVal)
	params := toParamsList(d["/DecodeParms"], len(filters))

	data := raw
	for i, f := range filters {
		var err error
		data, err = applyFilter(f, data)
		if err != nil {
			return nil, err
		}
		if params[i] != nil {
			predictor := dictGetInt(params[i], "/Predictor")
			if predictor >= 10 { // PNG predictor
				columns := dictGetInt(params[i], "/Columns")
				if columns == 0 {
					columns = 1
				}
				data, err = applyPNGPredictor(data, columns)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	return data, nil
}

// toParamsList returns a slice of DecodeParms dicts (one per filter).
func toParamsList(v pdfValue, n int) []pdfDict {
	params := make([]pdfDict, n)
	switch dv := v.(type) {
	case pdfDict:
		if n > 0 {
			params[0] = dv
		}
	case pdfArray:
		for i, item := range dv {
			if i >= n {
				break
			}
			if d, ok := item.(pdfDict); ok {
				params[i] = d
			}
		}
	}
	return params
}

// applyPNGPredictor reverses the PNG predictor applied before compression.
// data is the post-zlib bytes; each row is 1 filter-type byte + columns data bytes.
func applyPNGPredictor(data []byte, columns int) ([]byte, error) {
	stride := columns + 1
	if len(data)%stride != 0 {
		return nil, fmt.Errorf("PNG predictor: data length %d not divisible by stride %d", len(data), stride)
	}
	rows := len(data) / stride
	out := make([]byte, rows*columns)
	prev := make([]byte, columns)

	for i := 0; i < rows; i++ {
		row := data[i*stride : (i+1)*stride]
		filterType := row[0]
		curr := row[1:]
		outRow := out[i*columns : (i+1)*columns]

		switch filterType {
		case 0: // None
			copy(outRow, curr)
		case 1: // Sub
			for j := 0; j < columns; j++ {
				a := byte(0)
				if j > 0 {
					a = outRow[j-1]
				}
				outRow[j] = curr[j] + a
			}
		case 2: // Up
			for j := 0; j < columns; j++ {
				outRow[j] = curr[j] + prev[j]
			}
		case 3: // Average
			for j := 0; j < columns; j++ {
				a := byte(0)
				if j > 0 {
					a = outRow[j-1]
				}
				outRow[j] = curr[j] + byte((int(a)+int(prev[j]))/2)
			}
		case 4: // Paeth
			for j := 0; j < columns; j++ {
				a := byte(0)
				c := byte(0)
				if j > 0 {
					a = outRow[j-1]
					c = prev[j-1]
				}
				outRow[j] = curr[j] + paethPredictor(a, prev[j], c)
			}
		default:
			return nil, fmt.Errorf("unknown PNG row filter type %d", filterType)
		}
		copy(prev, outRow)
	}
	return out, nil
}

func paethPredictor(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	pa := p - int(a)
	if pa < 0 {
		pa = -pa
	}
	pb := p - int(b)
	if pb < 0 {
		pb = -pb
	}
	pc := p - int(c)
	if pc < 0 {
		pc = -pc
	}
	if pa <= pb && pa <= pc {
		return a
	}
	if pb <= pc {
		return b
	}
	return c
}

func toFilterList(v pdfValue) []string {
	switch fv := v.(type) {
	case pdfName:
		return []string{string(fv)}
	case pdfArray:
		var list []string
		for _, item := range fv {
			if n, ok := item.(pdfName); ok {
				list = append(list, string(n))
			}
		}
		return list
	}
	return nil
}

func applyFilter(filter string, data []byte) ([]byte, error) {
	switch filter {
	case "/FlateDecode", "/Fl":
		return flateDecode(data)
	case "/ASCIIHexDecode", "/AHx":
		return asciiHexDecode(data)
	case "/ASCII85Decode", "/A85":
		return ascii85Decode(data)
	default:
		return data, fmt.Errorf("unsupported filter: %s", filter)
	}
}

func flateDecode(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}


func asciiHexDecode(data []byte) ([]byte, error) {
	// Remove whitespace and '>' terminator
	var clean strings.Builder
	for _, b := range data {
		if b == '>' {
			break
		}
		if !isWhitespace(b) {
			clean.WriteByte(b)
		}
	}
	s := clean.String()
	if len(s)%2 != 0 {
		s += "0"
	}
	return hex.DecodeString(s)
}

func ascii85Decode(data []byte) ([]byte, error) {
	// Find end marker ~>
	end := bytes.Index(data, []byte("~>"))
	if end >= 0 {
		data = data[:end]
	}
	var out []byte
	var buf [5]byte
	var n int
	for _, b := range data {
		if isWhitespace(b) {
			continue
		}
		if b == 'z' && n == 0 {
			out = append(out, 0, 0, 0, 0)
			continue
		}
		buf[n] = b - '!'
		n++
		if n == 5 {
			val := uint32(buf[0])*52200625 + uint32(buf[1])*614125 +
				uint32(buf[2])*7225 + uint32(buf[3])*85 + uint32(buf[4])
			out = append(out, byte(val>>24), byte(val>>16), byte(val>>8), byte(val))
			n = 0
		}
	}
	if n > 0 {
		// Partial group
		for i := n; i < 5; i++ {
			buf[i] = 84 // '~' - '!'
		}
		val := uint32(buf[0])*52200625 + uint32(buf[1])*614125 +
			uint32(buf[2])*7225 + uint32(buf[3])*85 + uint32(buf[4])
		b := [4]byte{byte(val >> 24), byte(val >> 16), byte(val >> 8), byte(val)}
		out = append(out, b[:n-1]...)
	}
	return out, nil
}

func decodeLiteralString(raw []byte) string {
	// raw includes outer parens
	if len(raw) < 2 {
		return ""
	}
	inner := raw[1 : len(raw)-1]
	var buf bytes.Buffer
	for i := 0; i < len(inner); i++ {
		if inner[i] == '\\' && i+1 < len(inner) {
			i++
			switch inner[i] {
			case 'n':
				buf.WriteByte('\n')
			case 'r':
				buf.WriteByte('\r')
			case 't':
				buf.WriteByte('\t')
			case 'b':
				buf.WriteByte('\b')
			case 'f':
				buf.WriteByte('\f')
			case '(':
				buf.WriteByte('(')
			case ')':
				buf.WriteByte(')')
			case '\\':
				buf.WriteByte('\\')
			default:
				buf.WriteByte(inner[i])
			}
		} else {
			buf.WriteByte(inner[i])
		}
	}
	return buf.String()
}

func decodeHexString(raw []byte) string {
	// raw includes < and >
	if len(raw) < 2 {
		return ""
	}
	inner := raw[1 : len(raw)-1]
	var clean strings.Builder
	for _, b := range inner {
		if !isWhitespace(b) {
			clean.WriteByte(b)
		}
	}
	s := clean.String()
	if len(s)%2 != 0 {
		s += "0"
	}
	decoded, _ := hex.DecodeString(s)
	return string(decoded)
}

// parseIndirectObject parses "n g obj <value> endobj" starting at the given offset.
func parseIndirectObject(data []byte, offset int64) (*pdfObject, error) {
	l := newLexerAt(data, int(offset))

	tok1, err := l.Next()
	if err != nil || tok1.kind != tokInt {
		return nil, fmt.Errorf("expected obj number at %d", offset)
	}
	num, _ := strconv.Atoi(string(tok1.raw))

	tok2, err := l.Next()
	if err != nil || tok2.kind != tokInt {
		return nil, fmt.Errorf("expected gen number at %d", offset)
	}
	gen, _ := strconv.Atoi(string(tok2.raw))

	tok3, err := l.Next()
	if err != nil || tok3.kind != tokKeyword || string(tok3.raw) != "obj" {
		return nil, fmt.Errorf("expected 'obj' keyword at %d", offset)
	}

	val, err := parseValue(l)
	if err != nil {
		return nil, fmt.Errorf("object %d value: %w", num, err)
	}

	return &pdfObject{Num: num, Gen: gen, Value: val}, nil
}

// toFloat converts a pdfValue numeric (int or float64) to float64.
func toFloat(v pdfValue) (float64, error) {
	switch n := v.(type) {
	case int:
		return float64(n), nil
	case float64:
		return n, nil
	}
	return 0, fmt.Errorf("expected number, got %T", v)
}

func dictGetName(d pdfDict, key string) string {
	if n, ok := d[key].(pdfName); ok {
		return string(n)
	}
	return ""
}

func dictGetInt(d pdfDict, key string) int {
	switch n := d[key].(type) {
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}
