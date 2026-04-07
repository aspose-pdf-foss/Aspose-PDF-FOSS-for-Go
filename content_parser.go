package asposepdf

import "strconv"

// contentOp is a single operator from a PDF content stream with its operands.
type contentOp struct {
	Operator string
	Operands []pdfValue
}

// parseContentStream parses decoded content stream bytes into a sequence of operators.
// Operands (numbers, strings, names) are collected on a stack; when a keyword (operator)
// is encountered, a contentOp is emitted with the accumulated operands.
func parseContentStream(data []byte) ([]contentOp, error) {
	l := newLexer(data)
	var ops []contentOp
	var operands []pdfValue

	for {
		tok, err := l.Next()
		if err != nil {
			return nil, err
		}
		if tok.kind == tokEOF {
			break
		}

		switch tok.kind {
		case tokKeyword:
			kw := string(tok.raw)
			if kw == "BI" {
				skipInlineImage(l)
				ops = append(ops, contentOp{Operator: "BI"})
				operands = nil
				continue
			}
			ops = append(ops, contentOp{
				Operator: kw,
				Operands: operands,
			})
			operands = nil

		case tokInt:
			n, _ := strconv.Atoi(string(tok.raw))
			operands = append(operands, n)

		case tokReal:
			f, _ := strconv.ParseFloat(string(tok.raw), 64)
			operands = append(operands, f)

		case tokName:
			operands = append(operands, pdfName(tok.raw))

		case tokString:
			operands = append(operands, decodeLiteralString(tok.raw))

		case tokHexStr:
			operands = append(operands, decodeHexString(tok.raw))

		case tokArrayOpen:
			arr, err := parseContentArray(l)
			if err != nil {
				return nil, err
			}
			operands = append(operands, arr)

		case tokDictOpen:
			d, err := parseDictBody(l)
			if err != nil {
				return nil, err
			}
			operands = append(operands, d)

		case tokBool:
			operands = append(operands, string(tok.raw) == "true")

		case tokNull:
			operands = append(operands, pdfNull{})
		}
	}
	return ops, nil
}

// parseContentArray parses a content stream array (used in TJ operator).
// Does not attempt to parse indirect references (they don't exist in content streams).
func parseContentArray(l *lexer) (pdfArray, error) {
	var arr pdfArray
	for {
		tok, err := l.Next()
		if err != nil {
			return nil, err
		}
		if tok.kind == tokArrayClose || tok.kind == tokEOF {
			break
		}
		switch tok.kind {
		case tokInt:
			n, _ := strconv.Atoi(string(tok.raw))
			arr = append(arr, n)
		case tokReal:
			f, _ := strconv.ParseFloat(string(tok.raw), 64)
			arr = append(arr, f)
		case tokString:
			arr = append(arr, decodeLiteralString(tok.raw))
		case tokHexStr:
			arr = append(arr, decodeHexString(tok.raw))
		case tokName:
			arr = append(arr, pdfName(tok.raw))
		}
	}
	return arr, nil
}

// skipInlineImage skips past the binary data of an inline image (BI...ID...EI).
// The lexer is positioned just after the "BI" keyword.
func skipInlineImage(l *lexer) {
	// Skip key-value pairs until "ID" keyword.
	for {
		tok, err := l.Next()
		if err != nil || tok.kind == tokEOF {
			return
		}
		if tok.kind == tokKeyword && string(tok.raw) == "ID" {
			break
		}
	}
	// Skip one whitespace byte after ID.
	if l.pos < len(l.data) {
		l.pos++
	}
	// Scan for whitespace + "EI" + delimiter.
	for l.pos < len(l.data)-2 {
		if isWhitespace(l.data[l.pos]) &&
			l.data[l.pos+1] == 'E' && l.data[l.pos+2] == 'I' &&
			(l.pos+3 >= len(l.data) || isDelimiter(l.data[l.pos+3])) {
			l.pos += 3
			return
		}
		l.pos++
	}
	l.pos = len(l.data)
}
