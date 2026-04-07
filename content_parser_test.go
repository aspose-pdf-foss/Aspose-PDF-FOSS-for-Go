package asposepdf

import "testing"

func TestParseContentStreamSimple(t *testing.T) {
	data := []byte("BT /F1 12 Tf 100 700 Td (Hello) Tj ET")
	ops, err := parseContentStream(data)
	if err != nil {
		t.Fatalf("parseContentStream: %v", err)
	}
	// BT, Tf, Td, Tj, ET = 5 operators
	if len(ops) != 5 {
		t.Fatalf("expected 5 ops, got %d", len(ops))
	}
	if ops[0].Operator != "BT" {
		t.Errorf("op[0]: got %q, want BT", ops[0].Operator)
	}
	if ops[1].Operator != "Tf" {
		t.Errorf("op[1]: got %q, want Tf", ops[1].Operator)
	}
	if len(ops[1].Operands) != 2 {
		t.Errorf("Tf operands: got %d, want 2", len(ops[1].Operands))
	}
	if ops[2].Operator != "Td" {
		t.Errorf("op[2]: got %q, want Td", ops[2].Operator)
	}
	if ops[3].Operator != "Tj" {
		t.Errorf("op[3]: got %q, want Tj", ops[3].Operator)
	}
	if len(ops[3].Operands) != 1 {
		t.Errorf("Tj operands: got %d, want 1", len(ops[3].Operands))
	}
	if ops[4].Operator != "ET" {
		t.Errorf("op[4]: got %q, want ET", ops[4].Operator)
	}
}

func TestParseContentStreamEmpty(t *testing.T) {
	ops, err := parseContentStream([]byte{})
	if err != nil {
		t.Fatalf("parseContentStream: %v", err)
	}
	if len(ops) != 0 {
		t.Errorf("expected 0 ops, got %d", len(ops))
	}
}

func TestParseContentStreamTJArray(t *testing.T) {
	data := []byte("BT [(He) -10 (llo)] TJ ET")
	ops, err := parseContentStream(data)
	if err != nil {
		t.Fatalf("parseContentStream: %v", err)
	}
	if len(ops) != 3 {
		t.Fatalf("expected 3 ops, got %d", len(ops))
	}
	if ops[1].Operator != "TJ" {
		t.Errorf("op[1]: got %q, want TJ", ops[1].Operator)
	}
	arr, ok := ops[1].Operands[0].(pdfArray)
	if !ok {
		t.Fatalf("TJ operand is not pdfArray: %T", ops[1].Operands[0])
	}
	if len(arr) != 3 {
		t.Fatalf("TJ array: expected 3 elements, got %d", len(arr))
	}
	if s, ok := arr[0].(string); !ok || s != "He" {
		t.Errorf("TJ[0]: got %v, want \"He\"", arr[0])
	}
	if n, ok := arr[1].(int); !ok || n != -10 {
		t.Errorf("TJ[1]: got %v, want -10", arr[1])
	}
}

func TestResolveFontWinAnsi(t *testing.T) {
	objects := map[int]*pdfObject{}
	fontDict := pdfDict{
		"/Type":     pdfName("/Font"),
		"/Subtype":  pdfName("/Type1"),
		"/BaseFont": pdfName("/Helvetica"),
		"/Encoding": pdfName("/WinAnsiEncoding"),
	}
	fi := resolveFont(objects, fontDict)
	if fi.name != "/Helvetica" {
		t.Errorf("name: got %q, want /Helvetica", fi.name)
	}
	if !fi.known {
		t.Error("expected known=true for WinAnsiEncoding")
	}
	if fi.encoding[65] != 'A' {
		t.Errorf("encoding[65]: got %c, want A", fi.encoding[65])
	}
}

func TestResolveFontStandard14Default(t *testing.T) {
	objects := map[int]*pdfObject{}
	fontDict := pdfDict{
		"/Type":     pdfName("/Font"),
		"/Subtype":  pdfName("/Type1"),
		"/BaseFont": pdfName("/Courier"),
	}
	fi := resolveFont(objects, fontDict)
	if !fi.known {
		t.Error("expected known=true for standard 14 font without /Encoding")
	}
}

func TestResolveFontUnknown(t *testing.T) {
	objects := map[int]*pdfObject{}
	fontDict := pdfDict{
		"/Type":     pdfName("/Font"),
		"/Subtype":  pdfName("/Type1"),
		"/BaseFont": pdfName("/CustomFont+ABC"),
	}
	fi := resolveFont(objects, fontDict)
	if fi.known {
		t.Error("expected known=false for unknown font without /Encoding")
	}
}

func TestResolveFontWithDifferences(t *testing.T) {
	objects := map[int]*pdfObject{}
	fontDict := pdfDict{
		"/Type":     pdfName("/Font"),
		"/Subtype":  pdfName("/Type1"),
		"/BaseFont": pdfName("/Helvetica"),
		"/Encoding": pdfDict{
			"/Type":         pdfName("/Encoding"),
			"/BaseEncoding": pdfName("/WinAnsiEncoding"),
			"/Differences":  pdfArray{32, pdfName("/Euro")},
		},
	}
	fi := resolveFont(objects, fontDict)
	if !fi.known {
		t.Error("expected known=true")
	}
	if fi.encoding[32] != '€' {
		t.Errorf("encoding[32]: got %c, want €", fi.encoding[32])
	}
	if fi.encoding[65] != 'A' {
		t.Errorf("encoding[65]: got %c, want A", fi.encoding[65])
	}
}

func TestApplyDifferences(t *testing.T) {
	base := standardEncoding
	diffs := pdfArray{
		32, pdfName("/Euro"),
		65, pdfName("/Omega"),
	}
	enc := applyDifferences(base, diffs)
	if enc[32] != '€' {
		t.Errorf("pos 32: got %c, want €", enc[32])
	}
	if enc[65] != 'Ω' {
		t.Errorf("pos 65: got %c, want Ω", enc[65])
	}
	if enc[66] != base[66] {
		t.Errorf("pos 66 should be unchanged")
	}
}

func TestParseContentStreamInlineImage(t *testing.T) {
	data := []byte("BT (Before) Tj ET BI /W 1 /H 1 /CS /G /BPC 8 ID \x00 EI BT (After) Tj ET")
	ops, err := parseContentStream(data)
	if err != nil {
		t.Fatalf("parseContentStream: %v", err)
	}
	var tjOps []contentOp
	for _, op := range ops {
		if op.Operator == "Tj" {
			tjOps = append(tjOps, op)
		}
	}
	if len(tjOps) != 2 {
		t.Fatalf("expected 2 Tj ops, got %d (total ops: %d)", len(tjOps), len(ops))
	}
}
