package asposepdf

import (
	"bytes"
	"fmt"
	"testing"
)

type testObj struct {
	num  int
	body []byte
}

func testMakeStream(data []byte) []byte {
	return []byte(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(data), data))
}

func assemblePDF(objs []testObj) []byte {
	var buf []byte
	buf = append(buf, "%PDF-1.4\n"...)
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = len(buf)
		buf = append(buf, fmt.Sprintf("%d 0 obj\n", o.num)...)
		buf = append(buf, o.body...)
		buf = append(buf, "\nendobj\n"...)
	}
	xrefOffset := len(buf)
	buf = append(buf, "xref\n"...)
	buf = append(buf, fmt.Sprintf("0 %d\n", len(objs)+1)...)
	buf = append(buf, "0000000000 65535 f \r\n"...)
	for _, off := range offsets {
		buf = append(buf, fmt.Sprintf("%010d 00000 n \r\n", off)...)
	}
	buf = append(buf, "trailer\n"...)
	buf = append(buf, fmt.Sprintf("<< /Size %d /Root 1 0 R >>\n", len(objs)+1)...)
	buf = append(buf, "startxref\n"...)
	buf = append(buf, fmt.Sprintf("%d\n", xrefOffset)...)
	buf = append(buf, "%%EOF\n"...)
	return buf
}

// buildTestPDF creates a minimal 2-page PDF with known content for internal tests.
func buildTestPDF() []byte {
	return assemblePDF([]testObj{
		{1, []byte("<< /Type /Catalog /Pages 2 0 R >>")},
		{2, []byte("<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>")},
		{3, []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 7 0 R >> >> >>")},
		{4, testMakeStream([]byte("BT /F1 12 Tf 100 700 Td (Page 1) Tj ET"))},
		{5, []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 6 0 R /Resources << /Font << /F1 7 0 R >> >> >>")},
		{6, testMakeStream([]byte("BT /F1 12 Tf 100 700 Td (Page 2) Tj ET"))},
		{7, []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")},
	})
}

// buildTestPDFWithContent creates a single-page PDF with custom content and a Helvetica/WinAnsi font at /F1.
func buildTestPDFWithContent(content []byte) []byte {
	return assemblePDF([]testObj{
		{1, []byte("<< /Type /Catalog /Pages 2 0 R >>")},
		{2, []byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>")},
		{3, []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>")},
		{4, testMakeStream(content)},
		{5, []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")},
	})
}

func TestPageContentStreams(t *testing.T) {
	pdf := buildTestPDF()
	doc, err := OpenStream(bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page := &Page{doc: doc, index: 0}
	data, err := page.contentStreams()
	if err != nil {
		t.Fatalf("contentStreams: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty content stream data")
	}
	if !bytes.Contains(data, []byte("Page 1")) {
		t.Error("content stream should contain 'Page 1'")
	}
}

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

func TestStandard14Widths(t *testing.T) {
	// Helvetica: 'A' = 667, 'i' = 222, space = 278
	w, ok := standard14Widths("/Helvetica")
	if !ok {
		t.Fatal("expected Helvetica to be a standard 14 font")
	}
	if w[65] != 667 {
		t.Errorf("Helvetica 'A': got %v, want 667", w[65])
	}
	if w[105] != 222 {
		t.Errorf("Helvetica 'i': got %v, want 222", w[105])
	}
	if w[32] != 278 {
		t.Errorf("Helvetica space: got %v, want 278", w[32])
	}

	// Courier: all printable = 600
	w, ok = standard14Widths("/Courier")
	if !ok {
		t.Fatal("expected Courier to be a standard 14 font")
	}
	if w[65] != 600 {
		t.Errorf("Courier 'A': got %v, want 600", w[65])
	}

	// Times-Roman: 'A' = 722
	w, ok = standard14Widths("/Times-Roman")
	if !ok {
		t.Fatal("expected Times-Roman to be a standard 14 font")
	}
	if w[65] != 722 {
		t.Errorf("Times-Roman 'A': got %v, want 722", w[65])
	}

	// Unknown font returns false.
	_, ok = standard14Widths("/CustomFont+XYZ")
	if ok {
		t.Error("expected ok=false for unknown font")
	}
}

func TestResolveFontWidthsFromDict(t *testing.T) {
	objects := map[int]*pdfObject{}
	fontDict := pdfDict{
		"/Type":      pdfName("/Font"),
		"/Subtype":   pdfName("/Type1"),
		"/BaseFont":  pdfName("/Helvetica"),
		"/Encoding":  pdfName("/WinAnsiEncoding"),
		"/FirstChar": 32,
		"/LastChar":  34,
		"/Widths":    pdfArray{250, 300, 350},
	}
	fi := resolveFont(objects, fontDict)
	if fi.widths[32] != 250 {
		t.Errorf("widths[32]: got %v, want 250", fi.widths[32])
	}
	if fi.widths[33] != 300 {
		t.Errorf("widths[33]: got %v, want 300", fi.widths[33])
	}
	if fi.widths[34] != 350 {
		t.Errorf("widths[34]: got %v, want 350", fi.widths[34])
	}
}

func TestResolveFontWidthsStandard14Fallback(t *testing.T) {
	objects := map[int]*pdfObject{}
	fontDict := pdfDict{
		"/Type":     pdfName("/Font"),
		"/Subtype":  pdfName("/Type1"),
		"/BaseFont": pdfName("/Helvetica"),
		"/Encoding": pdfName("/WinAnsiEncoding"),
	}
	fi := resolveFont(objects, fontDict)
	if fi.widths[65] != 667 {
		t.Errorf("widths[65] (Helvetica 'A'): got %v, want 667", fi.widths[65])
	}
}

func TestResolveFontWidthsUnknownFallback(t *testing.T) {
	objects := map[int]*pdfObject{}
	fontDict := pdfDict{
		"/Type":     pdfName("/Font"),
		"/Subtype":  pdfName("/Type1"),
		"/BaseFont": pdfName("/CustomFont+ABC"),
	}
	fi := resolveFont(objects, fontDict)
	if fi.widths[65] != 600 {
		t.Errorf("widths[65]: got %v, want 600 (fallback)", fi.widths[65])
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
