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
				dict, imgData := parseInlineImage(l)
				var biOperands []pdfValue
				if dict != nil {
					biOperands = []pdfValue{pdfValue(dict), pdfValue(string(imgData))}
				}
				ops = append(ops, contentOp{Operator: "BI", Operands: biOperands})
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

