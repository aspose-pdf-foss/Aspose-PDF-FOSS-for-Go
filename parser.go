// SPDX-License-Identifier: MIT

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
		// Only attempt filter decoding when a /Filter is present.
		// Streams without /Filter may still be encrypted (no filter to detect
		// that), so we must leave Decoded=false in that case so the decryption
		// pass in getObject can process the raw bytes before we mark them clean.
		if _, hasFilter := d["/Filter"]; !hasFilter {
			return &pdfStream{Dict: d, Data: streamData, Decoded: false}, nil
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

// readStreamData reads raw stream bytes for the current stream. It uses a
// direct integer /Length when present and in range; otherwise — when
// /Length is an indirect reference (valid per ISO 32000-1 §7.3.8.2),
// missing, or out of range — it falls back to scanning for the
// "endstream" keyword. The indirect-reference case cannot be resolved
// here (the lexer has no object table), so the scan is the robust path
// and also tolerates a wrong /Length.
func readStreamData(l *lexer, d pdfDict) ([]byte, error) {
	length := -1
	switch v := d["/Length"].(type) {
	case int:
		length = v
	case float64:
		length = int(v)
	}

	// A usable direct /Length is taken verbatim — but only when "endstream"
	// actually follows the claimed data (allowing the optional EOL before it,
	// ISO 32000-1 §7.3.8.1). A wrong direct /Length that merely fits in the file
	// (e.g. a literal "/Length 1" in front of a 5 KB content stream) would
	// otherwise truncate the stream; verifying the terminator catches that and
	// falls through to the endstream scan, matching Acrobat/MuPDF/pdf.js.
	if length >= 0 && l.pos+length <= len(l.data) && followedByEndstream(l.data, l.pos+length) {
		data := l.data[l.pos : l.pos+length]
		l.pos += length
		return data, nil
	}

	// Otherwise /Length is an indirect reference, missing, wrong, or out of range:
	// scan for the "endstream" keyword and take everything up to the single
	// end-of-line that precedes it.
	idx := streamEndIndex(l.data[l.pos:])
	if idx < 0 {
		return nil, fmt.Errorf("stream: no endstream marker found")
	}
	dataEnd := l.pos + idx
	if dataEnd > l.pos && l.data[dataEnd-1] == '\n' {
		dataEnd--
	}
	if dataEnd > l.pos && l.data[dataEnd-1] == '\r' {
		dataEnd--
	}
	data := l.data[l.pos:dataEnd]
	l.pos = l.pos + idx // leave the lexer at the "endstream" keyword
	return data, nil
}

// followedByEndstream reports whether the "endstream" keyword begins at pos,
// after skipping the optional end-of-line (and any stray whitespace) that may
// separate the stream data from the keyword. Used to validate a direct /Length
// before trusting it.
func followedByEndstream(data []byte, pos int) bool {
	for pos < len(data) && isWhitespace(data[pos]) {
		pos++
	}
	return bytes.HasPrefix(data[pos:], []byte("endstream"))
}

// streamEndIndex returns the offset within data of the "endstream" keyword that
// terminates the stream. Binary stream data (compressed or encrypted bytes) can
// contain a spurious "endstream" byte sequence; an indirect or wrong /Length
// forces us here without a reliable length, so a naive first-match scan can
// truncate the stream mid-data. The real terminator is "endstream" followed by
// the object's "endobj" (optionally separated by whitespace), so prefer the
// first match confirmed that way and fall back to the first match otherwise.
func streamEndIndex(data []byte) int {
	kw := []byte("endstream")
	first := -1
	for off := 0; ; {
		rel := bytes.Index(data[off:], kw)
		if rel < 0 {
			break
		}
		abs := off + rel
		if first < 0 {
			first = abs
		}
		j := abs + len(kw)
		for j < len(data) && isWhitespace(data[j]) {
			j++
		}
		if bytes.HasPrefix(data[j:], []byte("endobj")) {
			return abs
		}
		off = abs + 1
	}
	return first
}

// decodeStream decompresses stream data based on /Filter and /DecodeParms.
func decodeStream(d pdfDict, raw []byte) ([]byte, error) {
	filterVal, ok := d["/Filter"]
	if !ok {
		return raw, nil // uncompressed
	}
	// An indirect /Filter (e.g. /Filter 11 0 R) can't be resolved here — error
	// so the stream stays raw (Decoded=false) until resolveIndirectStreamFilters
	// materialises the reference after all objects are parsed. Returning raw
	// with nil error would wrongly mark the stream as decoded.
	if hasIndirectFilter(filterVal) {
		return nil, fmt.Errorf("indirect /Filter reference")
	}
	// Likewise an indirect /DecodeParms (or /DP), e.g. /DecodeParms [107 0 R]:
	// decoding here would run with empty params (wrong /Columns, /K, /BlackIs1
	// for CCITT) and mis-mark the stream Decoded, blocking the post-pass that
	// would resolve the reference and decode correctly (39952.pdf).
	if hasIndirectFilter(d["/DecodeParms"]) || hasIndirectFilter(d["/DP"]) {
		return nil, fmt.Errorf("indirect /DecodeParms reference")
	}

	filters := toFilterList(filterVal)
	params := toParamsList(d["/DecodeParms"], len(filters))

	data := raw
	for i, f := range filters {
		var err error
		if f == "/CCITTFaxDecode" || f == "/CCF" {
			data, err = ccittFilter(data, params[i], d)
			if err != nil {
				return nil, err
			}
			continue
		}
		if f == "/LZWDecode" || f == "/LZW" {
			// Handled here rather than in applyFilter because /EarlyChange
			// lives in DecodeParms; the predictor block below still applies.
			data, err = lzwDecode(data, lzwEarlyChange(params[i]))
		} else {
			data, err = applyFilter(f, data)
		}
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
				colors := dictGetInt(params[i], "/Colors")
				if colors == 0 {
					colors = 1
				}
				bpcP := dictGetInt(params[i], "/BitsPerComponent")
				if bpcP == 0 {
					bpcP = 8
				}
				rowBytes := (columns*colors*bpcP + 7) / 8
				bpp := colors * bpcP / 8
				if bpp < 1 {
					bpp = 1
				}
				data, err = applyPNGPredictor(data, rowBytes, bpp)
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
// data is the post-zlib bytes; each row is 1 filter-type byte + rowBytes data
// bytes (rowBytes = ceil(Columns·Colors·BitsPerComponent / 8)). bpp is the
// byte distance to the "left" reference sample (Colors·BitsPerComponent/8,
// min 1) — for an RGB8 image that is 3, not 1, per the PNG spec.
func applyPNGPredictor(data []byte, rowBytes, bpp int) ([]byte, error) {
	stride := rowBytes + 1
	if len(data)%stride != 0 {
		return nil, fmt.Errorf("PNG predictor: data length %d not divisible by stride %d", len(data), stride)
	}
	rows := len(data) / stride
	out := make([]byte, rows*rowBytes)
	prev := make([]byte, rowBytes)

	for i := 0; i < rows; i++ {
		row := data[i*stride : (i+1)*stride]
		filterType := row[0]
		curr := row[1:]
		outRow := out[i*rowBytes : (i+1)*rowBytes]

		switch filterType {
		case 0: // None
			copy(outRow, curr)
		case 1: // Sub
			for j := 0; j < rowBytes; j++ {
				a := byte(0)
				if j >= bpp {
					a = outRow[j-bpp]
				}
				outRow[j] = curr[j] + a
			}
		case 2: // Up
			for j := 0; j < rowBytes; j++ {
				outRow[j] = curr[j] + prev[j]
			}
		case 3: // Average
			for j := 0; j < rowBytes; j++ {
				a := byte(0)
				if j >= bpp {
					a = outRow[j-bpp]
				}
				outRow[j] = curr[j] + byte((int(a)+int(prev[j]))/2)
			}
		case 4: // Paeth
			for j := 0; j < rowBytes; j++ {
				a := byte(0)
				c := byte(0)
				if j >= bpp {
					a = outRow[j-bpp]
					c = prev[j-bpp]
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
	case "/RunLengthDecode", "/RL":
		return runLengthDecode(data)
	case "/LZWDecode", "/LZW":
		return lzwDecode(data, 1) // default /EarlyChange; decodeStream passes the real one
	default:
		return data, fmt.Errorf("unsupported filter: %s", filter)
	}
}

// runLengthDecode decodes RunLengthDecode data (ISO 32000-1 §7.4.5): a length
// byte n < 128 copies the next n+1 bytes literally; n > 128 repeats the next
// byte 257−n times; n == 128 is end-of-data.
func runLengthDecode(data []byte) ([]byte, error) {
	var out []byte
	i := 0
	for i < len(data) {
		n := int(data[i])
		i++
		switch {
		case n == 128:
			return out, nil
		case n < 128:
			end := i + n + 1
			if end > len(data) {
				end = len(data)
			}
			out = append(out, data[i:end]...)
			i = end
		default:
			if i >= len(data) {
				return out, nil
			}
			out = append(out, bytes.Repeat([]byte{data[i]}, 257-n)...)
			i++
		}
	}
	return out, nil
}

// hasIndirectFilter reports whether a value is (or, for an array, contains) an
// unresolved indirect reference. Used to detect indirect /Filter, /DecodeParms
// and /DP entries that must be resolved by the post-parse pass before decoding.
func hasIndirectFilter(v pdfValue) bool {
	switch fv := v.(type) {
	case pdfRef:
		return true
	case pdfArray:
		for _, el := range fv {
			if _, ok := el.(pdfRef); ok {
				return true
			}
		}
	}
	return false
}

func flateDecode(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	out, rerr := io.ReadAll(r)
	if rerr != nil && len(out) > 0 {
		// Tolerate a truncated stream or a bad trailing Adler-32 checksum (common
		// in real-world PDFs with a wrong /Length): keep the bytes we inflated.
		// Throwing them away would blank the whole page. The zlib header was
		// valid, so this is genuine DEFLATE output — unlike a header failure,
		// which we still reject (e.g. an undecrypted/random stream).
		return out, nil
	}
	return out, rerr
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

// validateASCII85 checks that every byte in data belongs to the ASCII85
// alphabet (ISO 32000-1 §7.4.3). Valid bytes are:
//   - '!' (0x21) … 'u' (0x75) — data alphabet
//   - 'z' (0x7A)              — shorthand for four zero bytes
//   - '~' (0x7E)              — start of end-of-data marker (~>)
//   - whitespace / NUL        — silently ignored per spec
//
// Any other byte signals that data is not valid ASCII85 (e.g. encrypted
// stream bytes) and causes an error so the decoder returns Decoded=false.
func validateASCII85(data []byte) error {
	for i, b := range data {
		switch {
		case b >= '!' && b <= 'u':
			// data alphabet — ok
		case b == 'z' || b == '~':
			// shorthand zero group or terminator — ok
		case b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' || b == '\v' || b == 0:
			// whitespace / NUL — ignored per spec
		default:
			return fmt.Errorf("ascii85: invalid byte 0x%02x at offset %d", b, i)
		}
	}
	return nil
}

func ascii85Decode(data []byte) ([]byte, error) {
	if err := validateASCII85(data); err != nil {
		return nil, err
	}
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

// decodeLiteralString decodes a PDF literal string token (including its
// outer parentheses) into raw bytes, per ISO 32000-1 §7.3.4.2. It handles
// the named escapes (\n \r \t \b \f \( \) \\), octal escapes \ddd (1–3
// octal digits, the byte value mod 256), and the backslash-before-EOL line
// continuation (the backslash and the line break are dropped). A backslash
// before any other character is dropped, keeping that character.
func decodeLiteralString(raw []byte) string {
	if len(raw) < 2 {
		return ""
	}
	inner := raw[1 : len(raw)-1]
	var buf bytes.Buffer
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		if c != '\\' || i+1 >= len(inner) {
			buf.WriteByte(c)
			continue
		}
		i++
		e := inner[i]
		switch e {
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
		case '\n':
			// Line continuation: backslash + LF — emit nothing.
		case '\r':
			// Line continuation: backslash + CR, optionally CRLF.
			if i+1 < len(inner) && inner[i+1] == '\n' {
				i++
			}
		case '0', '1', '2', '3', '4', '5', '6', '7':
			// Octal escape: this digit plus up to two more octal digits.
			val := int(e - '0')
			for k := 0; k < 2 && i+1 < len(inner) && inner[i+1] >= '0' && inner[i+1] <= '7'; k++ {
				i++
				val = val*8 + int(inner[i]-'0')
			}
			buf.WriteByte(byte(val))
		default:
			buf.WriteByte(e)
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

// dictGetString returns the string value of key from d.
// Handles plain string values and pdfName values (e.g. /V on a checkbox or radio field).
func dictGetString(d pdfDict, key string) string {
	switch v := d[key].(type) {
	case string:
		return v
	case pdfName:
		return string(v)
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
