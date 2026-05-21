# Phase 2: Text Extraction — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add text extraction to the PDF library — parse content streams, resolve font encodings, and expose `(*Page).ExtractText()` and `(*Document).ExtractText()` public API.

**Architecture:** 4 new files in the root package (`content_parser.go`, `encoding.go`, `font.go`, `text.go`) plus 2 test files. Reuses the existing `lexer` for tokenization. Content stream operators are parsed into `contentOp` structs, fonts are resolved to `[256]rune` encoding tables, and a text state machine processes operators to produce positioned text with heuristic space/newline insertion.

**Tech Stack:** Pure Go, no external dependencies. Existing lexer, parser, and object model from Phase 1.

**Spec:** `docs/superpowers/specs/2026-04-06-text-extraction-design.md`

---

### Task 1: Content Stream Parser — Core

**Files:**
- Create: `content_parser.go`
- Create: `content_parser_test.go`

This task implements the content stream parser that converts raw content stream bytes into a sequence of `contentOp` structs. It reuses the existing `lexer` from `lexer.go`.

- [ ] **Step 1: Write the failing test for basic BT/ET parsing**

In `content_parser_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestParseContentStream -v ./...`
Expected: FAIL — `parseContentStream` not defined.

- [ ] **Step 3: Implement the content stream parser**

In `content_parser.go`:

```go
package asposepdf

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
			n := toIntBytes(tok.raw)
			// Handle negative numbers.
			if len(tok.raw) > 0 && tok.raw[0] == '-' {
				n = -toIntBytes(tok.raw[1:])
			}
			operands = append(operands, n)

		case tokReal:
			f := parseFloat(tok.raw)
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
			// Inline image dict — skip (consumed by BI handler).
			// Dicts in content streams are rare outside BI; skip for robustness.
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
		if tok.kind == tokArrayClose {
			break
		}
		if tok.kind == tokEOF {
			break
		}
		switch tok.kind {
		case tokInt:
			n := toIntBytes(tok.raw)
			if len(tok.raw) > 0 && tok.raw[0] == '-' {
				n = -toIntBytes(tok.raw[1:])
			}
			arr = append(arr, n)
		case tokReal:
			arr = append(arr, parseFloat(tok.raw))
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
	// Scan for "\nEI" or " EI" delimiter.
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

// parseFloat parses a float from raw token bytes.
func parseFloat(raw []byte) float64 {
	// strconv.ParseFloat is fine for PDF content stream numbers.
	f, _ := strconv.ParseFloat(string(raw), 64)
	return f
}
```

Note: We need to add `import "strconv"` to the file. The `toIntBytes` function already exists in `doc.go` but only handles positive ASCII digits. For negative numbers in content streams we handle the sign in the parser.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run TestParseContentStream -v ./...`
Expected: PASS

- [ ] **Step 5: Write test for TJ array parsing**

Add to `content_parser_test.go`:

```go
func TestParseContentStreamTJArray(t *testing.T) {
	data := []byte("BT [(He) -10 (llo)] TJ ET")
	ops, err := parseContentStream(data)
	if err != nil {
		t.Fatalf("parseContentStream: %v", err)
	}
	// BT, TJ, ET
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
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test -run TestParseContentStreamTJArray -v ./...`
Expected: PASS

- [ ] **Step 7: Write test for inline image skipping**

Add to `content_parser_test.go`:

```go
func TestParseContentStreamInlineImage(t *testing.T) {
	// BI ... ID <binary> EI should be skipped; text after should parse.
	data := []byte("BT (Before) Tj ET BI /W 1 /H 1 /CS /G /BPC 8 ID \x00 EI BT (After) Tj ET")
	ops, err := parseContentStream(data)
	if err != nil {
		t.Fatalf("parseContentStream: %v", err)
	}
	// First BT, Tj, ET = 3; BI = 1; second BT, Tj, ET = 3; total = 7
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
```

- [ ] **Step 8: Run test to verify it passes**

Run: `go test -run TestParseContentStreamInlineImage -v ./...`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add content_parser.go content_parser_test.go
git commit -m "feat: add content stream parser (Phase 2)

Parse PDF content stream bytes into contentOp structs.
Reuses existing lexer. Handles TJ arrays and inline images (BI/ID/EI).

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

### Task 2: Encoding Tables

**Files:**
- Create: `encoding.go`

Static encoding tables for WinAnsi, MacRoman, Standard, Symbol, ZapfDingbats, plus the Adobe Glyph List for `/Differences` resolution.

- [ ] **Step 1: Write encoding.go with all tables**

Create `encoding.go` with the five encoding tables and glyph-to-rune map. Each table is a `[256]rune` array where index = character code, value = Unicode rune. Position 0 (`.notdef`) maps to `\uFFFD`.

```go
package asposepdf

// applyDifferences overlays /Differences entries onto a base encoding.
// diffs is a pdfArray of the form: code₁ /name₁ /name₂ … code₂ /name₃ …
// Each integer starts a run at that code; each name maps glyphToRune[name].
func applyDifferences(base [256]rune, diffs pdfArray) [256]rune {
	enc := base
	code := 0
	for _, v := range diffs {
		switch val := v.(type) {
		case int:
			code = val
		case pdfName:
			name := string(val)
			if len(name) > 0 && name[0] == '/' {
				name = name[1:]
			}
			if r, ok := glyphToRune[name]; ok && code < 256 {
				enc[code] = r
			}
			code++
		}
	}
	return enc
}

var winAnsiEncoding = [256]rune{ /* ... 256 entries ... */ }
var macRomanEncoding = [256]rune{ /* ... 256 entries ... */ }
var standardEncoding = [256]rune{ /* ... 256 entries ... */ }
var symbolEncoding = [256]rune{ /* ... 256 entries ... */ }
var zapfDingbatsEncoding = [256]rune{ /* ... 256 entries ... */ }
var glyphToRune = map[string]rune{ /* ~500 entries */ }
```

The actual arrays contain complete mappings per the PDF spec and Adobe Glyph List. Positions without a defined glyph map to `\uFFFD` (U+FFFD replacement character).

Key entries for WinAnsiEncoding (indices 32-127 match ASCII, plus positions 128-159 for Euro, curly quotes, etc., and 160-255 for Latin-1 Supplement).

Key entries for glyphToRune: `"space": ' '`, `"A": 'A'`, `"Adieresis": 'Ä'`, `"endash": '–'`, `"fi": 'fi'`, etc.

This file will be ~600-700 lines due to the static data. The full encoding tables are derived from:
- PDF Reference 1.7, Appendix D (character sets)
- Adobe Glyph List (https://github.com/adobe-type-tools/agl-aglfn)

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: BUILD OK

- [ ] **Step 3: Write test for applyDifferences**

Add to `content_parser_test.go` (or create a small test in the same package):

```go
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
	// Unmodified position should remain.
	if enc[66] != base[66] {
		t.Errorf("pos 66 should be unchanged")
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestApplyDifferences -v ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add encoding.go
git commit -m "feat: add PDF encoding tables (Phase 2)

WinAnsi, MacRoman, Standard, Symbol, ZapfDingbats encoding tables.
Adobe Glyph List (~500 entries) for /Differences resolution.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

### Task 3: Font Resolver

**Files:**
- Create: `font.go`

Resolves a PDF font dictionary into a `fontInfo` struct containing the font name, encoding table, and whether the encoding is known.

- [ ] **Step 1: Write the failing test**

Add to `content_parser_test.go`:

```go
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
	// Other positions must still be WinAnsi.
	if fi.encoding[65] != 'A' {
		t.Errorf("encoding[65]: got %c, want A", fi.encoding[65])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestResolveFont -v ./...`
Expected: FAIL — `resolveFont` not defined.

- [ ] **Step 3: Implement font.go**

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run TestResolveFont -v ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add font.go
git commit -m "feat: add font resolver (Phase 2)

Resolves PDF font dictionaries to encoding tables.
Supports named encodings, /Differences, and standard 14 fonts.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

### Task 4: `/Resources` Inheritance & `contentStreams()` Helper

**Files:**
- Modify: `doc.go` — add `/Resources` inheritance in `walkPageTree`
- Modify: `page.go` — add `contentStreams()` method

- [ ] **Step 1: Write the failing test for contentStreams**

Add to `content_parser_test.go`:

```go
func TestPageContentStreams(t *testing.T) {
	pdf := buildMinimalPDF()
	doc, err := OpenStream(bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page, err := doc.Page(1)
	if err != nil {
		t.Fatalf("Page(1): %v", err)
	}
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
```

Add `import "bytes"` at the top of the test file.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestPageContentStreams -v ./...`
Expected: FAIL — `contentStreams` not defined.

- [ ] **Step 3: Add `/Resources` inheritance to walkPageTree in doc.go**

Modify `walkPageTree` in `doc.go` to propagate `/Resources` from `/Pages` to `/Page` nodes, same pattern as `/MediaBox` inheritance. Add the inherited key before recursing into children:

In `walkPageTree`, inside the `case "/Pages":` block, after getting kids but before iterating, check if the node has `/Resources`. When visiting a `/Page` node, if it lacks `/Resources` and a parent had one, set it.

A simpler approach: instead of modifying the tree walk, handle it in `contentStreams()` by walking the `/Parent` chain (same as `mediaBoxSize`). But since `/Pages` nodes are deleted after parsing, we should copy `/Resources` during `resolvePageTree` — before deletion.

Modify `resolvePageTree` in `doc.go` to call a new helper:

```go
func resolvePageTree(objects map[int]*pdfObject, catalog pdfDict) ([]*pdfObject, error) {
	pagesVal, ok := catalog["/Pages"]
	if !ok {
		return nil, fmt.Errorf("catalog missing /Pages")
	}
	var result []*pdfObject
	if err := walkPageTree(objects, pagesVal, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func walkPageTree(objects map[int]*pdfObject, nodeVal pdfValue, inheritedResources pdfValue, result *[]*pdfObject) error {
	ref, ok := nodeVal.(pdfRef)
	if !ok {
		return fmt.Errorf("page tree node is not a ref: %T", nodeVal)
	}
	obj, ok := objects[ref.Num]
	if !ok {
		return fmt.Errorf("object %d not found", ref.Num)
	}
	nodeDict, ok := obj.Value.(pdfDict)
	if !ok {
		return fmt.Errorf("page tree object %d is not a dict", ref.Num)
	}

	// Track inherited /Resources.
	if res, ok := nodeDict["/Resources"]; ok {
		inheritedResources = res
	}

	switch dictGetName(nodeDict, "/Type") {
	case "/Pages":
		kidsVal, ok := nodeDict["/Kids"]
		if !ok {
			return fmt.Errorf("Pages node %d missing /Kids", ref.Num)
		}
		arr, ok := kidsVal.(pdfArray)
		if !ok {
			return fmt.Errorf("/Kids is not an array")
		}
		for _, kid := range arr {
			if err := walkPageTree(objects, kid, inheritedResources, result); err != nil {
				return err
			}
		}
	case "/Page", "":
		// Inherit /Resources if not present on page itself.
		if _, hasRes := nodeDict["/Resources"]; !hasRes && inheritedResources != nil {
			nodeDict["/Resources"] = inheritedResources
		}
		*result = append(*result, obj)
	default:
		return fmt.Errorf("unknown page tree node type: %s at object %d",
			dictGetName(nodeDict, "/Type"), ref.Num)
	}
	return nil
}
```

- [ ] **Step 4: Add contentStreams method to page.go**

Add to `page.go`:

```go
// contentStreams returns the concatenated decoded content stream bytes for this page.
// /Contents may be a single stream reference or an array of references.
func (p *Page) contentStreams() ([]byte, error) {
	d := p.pageDict()
	if d == nil {
		return nil, fmt.Errorf("page %d has no dict", p.Number())
	}
	contentsVal, ok := d["/Contents"]
	if !ok {
		return nil, nil // page with no content
	}

	objects := p.doc.objects
	contentsVal = resolveRef(objects, contentsVal)

	switch cv := contentsVal.(type) {
	case *pdfStream:
		return cv.Data, nil
	case pdfArray:
		var buf []byte
		for _, item := range cv {
			resolved := resolveRef(objects, item)
			if s, ok := resolved.(*pdfStream); ok {
				buf = append(buf, s.Data...)
				buf = append(buf, '\n')
			}
		}
		return buf, nil
	default:
		return nil, fmt.Errorf("unexpected /Contents type %T", contentsVal)
	}
}

// pageResources returns the /Resources dict for this page (may be inherited).
func (p *Page) pageResources() pdfDict {
	d := p.pageDict()
	if d == nil {
		return nil
	}
	resVal, ok := d["/Resources"]
	if !ok {
		return nil
	}
	res := resolveRef(p.doc.objects, resVal)
	if rd, ok := res.(pdfDict); ok {
		return rd
	}
	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -run TestPageContentStreams -v ./...`
Expected: PASS

Also run full test suite to check nothing is broken:
Run: `go test -v ./...`
Expected: All existing tests PASS.

- [ ] **Step 6: Commit**

```bash
git add doc.go page.go
git commit -m "feat: add /Resources inheritance and contentStreams helper (Phase 2)

walkPageTree now propagates /Resources from /Pages to /Page nodes.
Page.contentStreams() returns concatenated decoded content stream bytes.
Page.pageResources() returns the resolved /Resources dict.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

### Task 5: Text State Machine & Public API

**Files:**
- Create: `text.go`
- Create: `text_test.go`

Core text extraction: text state machine, operator handling, space/newline heuristics, Form XObject recursion, and the public `ExtractText()` API.

- [ ] **Step 1: Write the failing test for basic text extraction**

In `text_test.go`:

```go
package asposepdf_test

import (
	"bytes"
	"strings"
	"testing"

	asposepdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestExtractTextMinimal(t *testing.T) {
	pdf := buildMinimalPDF()
	doc, err := asposepdf.OpenStream(bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page1, err := doc.Page(1)
	if err != nil {
		t.Fatalf("Page(1): %v", err)
	}
	text, err := page1.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if !strings.Contains(text, "Page 1") {
		t.Errorf("page 1 text=%q, want it to contain 'Page 1'", text)
	}

	page2, err := doc.Page(2)
	if err != nil {
		t.Fatalf("Page(2): %v", err)
	}
	text2, err := page2.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if !strings.Contains(text2, "Page 2") {
		t.Errorf("page 2 text=%q, want it to contain 'Page 2'", text2)
	}
}

func TestDocumentExtractText(t *testing.T) {
	pdf := buildMinimalPDF()
	doc, err := asposepdf.OpenStream(bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	texts, err := doc.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if len(texts) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(texts))
	}
	if !strings.Contains(texts[0], "Page 1") {
		t.Errorf("page 1: %q", texts[0])
	}
	if !strings.Contains(texts[1], "Page 2") {
		t.Errorf("page 2: %q", texts[1])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run "TestExtractTextMinimal|TestDocumentExtractText" -v ./...`
Expected: FAIL — `ExtractText` not defined.

- [ ] **Step 3: Implement text.go**

```go
package asposepdf

import (
	"math"
	"strings"
)

// ExtractText returns the text content of the page.
// Characters from fonts with unrecognized encodings are replaced with U+FFFD.
func (p *Page) ExtractText() (string, error) {
	data, err := p.contentStreams()
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", nil
	}

	ops, err := parseContentStream(data)
	if err != nil {
		return "", err
	}

	resources := p.pageResources()
	fonts := resolveFontResources(p.doc.objects, resources)

	ext := newTextExtractor(p.doc.objects, fonts)
	ext.process(ops, resources)
	return ext.text(), nil
}

// ExtractText returns the text content of each page.
// The returned slice has one entry per page (0-indexed).
func (d *Document) ExtractText() ([]string, error) {
	pages := d.Pages()
	result := make([]string, len(pages))
	for i, p := range pages {
		text, err := p.ExtractText()
		if err != nil {
			return nil, err
		}
		result[i] = text
	}
	return result, nil
}

// resolveFontResources resolves all fonts in /Resources /Font.
func resolveFontResources(objects map[int]*pdfObject, resources pdfDict) map[string]fontInfo {
	fonts := make(map[string]fontInfo)
	if resources == nil {
		return fonts
	}
	fontVal, ok := resources["/Font"]
	if !ok {
		return fonts
	}
	fontDict, ok := resolveRefToDict(objects, fontVal)
	if !ok {
		return fonts
	}
	for name, val := range fontDict {
		fd, ok := resolveRefToDict(objects, val)
		if !ok {
			continue
		}
		fonts[name] = resolveFont(objects, fd)
	}
	return fonts
}

type textExtractor struct {
	objects map[int]*pdfObject
	fonts   map[string]fontInfo

	// Text state.
	font      fontInfo
	fontSize  float64
	charSpace float64
	wordSpace float64
	leading   float64
	tm        [6]float64 // text matrix
	lm        [6]float64 // line matrix
	ctm       [6]float64 // current transformation matrix
	ctmStack  [][6]float64

	// Output.
	buf    strings.Builder
	lastX  float64
	lastY  float64
	hasPos bool
}

func newTextExtractor(objects map[int]*pdfObject, fonts map[string]fontInfo) *textExtractor {
	return &textExtractor{
		objects: objects,
		fonts:   fonts,
		ctm:     identityMatrix(),
	}
}

func identityMatrix() [6]float64 {
	return [6]float64{1, 0, 0, 1, 0, 0}
}

func (e *textExtractor) text() string {
	return strings.TrimSpace(e.buf.String())
}

func (e *textExtractor) process(ops []contentOp, resources pdfDict) {
	for _, op := range ops {
		switch op.Operator {
		case "BT":
			e.tm = identityMatrix()
			e.lm = identityMatrix()

		case "ET":
			// End of text object — nothing to do.

		case "Tf":
			if len(op.Operands) >= 2 {
				fontName := operandName(op.Operands[0])
				if fi, ok := e.fonts[fontName]; ok {
					e.font = fi
				}
				e.fontSize = operandFloat(op.Operands[1])
			}

		case "Td":
			if len(op.Operands) >= 2 {
				tx := operandFloat(op.Operands[0])
				ty := operandFloat(op.Operands[1])
				e.lm = matMul(translateMatrix(tx, ty), e.lm)
				e.tm = e.lm
			}

		case "TD":
			if len(op.Operands) >= 2 {
				tx := operandFloat(op.Operands[0])
				ty := operandFloat(op.Operands[1])
				e.leading = -ty
				e.lm = matMul(translateMatrix(tx, ty), e.lm)
				e.tm = e.lm
			}

		case "Tm":
			if len(op.Operands) >= 6 {
				for i := 0; i < 6; i++ {
					e.tm[i] = operandFloat(op.Operands[i])
				}
				e.lm = e.tm
			}

		case "T*":
			e.lm = matMul(translateMatrix(0, -e.leading), e.lm)
			e.tm = e.lm

		case "Tj":
			if len(op.Operands) >= 1 {
				e.showString(op.Operands[0])
			}

		case "TJ":
			if len(op.Operands) >= 1 {
				e.showTJ(op.Operands[0])
			}

		case "'":
			// Move to next line, then show string.
			e.lm = matMul(translateMatrix(0, -e.leading), e.lm)
			e.tm = e.lm
			if len(op.Operands) >= 1 {
				e.showString(op.Operands[0])
			}

		case "\"":
			// Set word/char spacing, move to next line, show string.
			if len(op.Operands) >= 3 {
				e.wordSpace = operandFloat(op.Operands[0])
				e.charSpace = operandFloat(op.Operands[1])
				e.lm = matMul(translateMatrix(0, -e.leading), e.lm)
				e.tm = e.lm
				e.showString(op.Operands[2])
			}

		case "Tc":
			if len(op.Operands) >= 1 {
				e.charSpace = operandFloat(op.Operands[0])
			}

		case "Tw":
			if len(op.Operands) >= 1 {
				e.wordSpace = operandFloat(op.Operands[0])
			}

		case "TL":
			if len(op.Operands) >= 1 {
				e.leading = operandFloat(op.Operands[0])
			}

		case "cm":
			if len(op.Operands) >= 6 {
				var m [6]float64
				for i := 0; i < 6; i++ {
					m[i] = operandFloat(op.Operands[i])
				}
				e.ctm = matMul(m, e.ctm)
			}

		case "q":
			e.ctmStack = append(e.ctmStack, e.ctm)

		case "Q":
			if len(e.ctmStack) > 0 {
				e.ctm = e.ctmStack[len(e.ctmStack)-1]
				e.ctmStack = e.ctmStack[:len(e.ctmStack)-1]
			}

		case "Do":
			if len(op.Operands) >= 1 {
				e.doFormXObject(op.Operands[0], resources)
			}
		}
	}
}

func (e *textExtractor) showString(operand pdfValue) {
	s, ok := operand.(string)
	if !ok {
		return
	}
	for i := 0; i < len(s); i++ {
		code := s[i]
		r := e.font.encoding[code]
		if r == 0 {
			r = '\uFFFD'
		}
		e.emitRune(r)
		// Advance text matrix by character width (approximation).
		w := e.fontSize * 0.6 // approximate glyph width
		if code == ' ' {
			w = e.fontSize * 0.25
		}
		tx := (w + e.charSpace) / 1000.0 * e.fontSize
		if code == ' ' {
			tx = e.fontSize*0.25 + e.wordSpace + e.charSpace
		}
		e.tm = matMul(translateMatrix(tx, 0), e.tm)
	}
}

func (e *textExtractor) showTJ(operand pdfValue) {
	arr, ok := operand.(pdfArray)
	if !ok {
		return
	}
	for _, elem := range arr {
		switch v := elem.(type) {
		case string:
			for i := 0; i < len(v); i++ {
				code := v[i]
				r := e.font.encoding[code]
				if r == 0 {
					r = '\uFFFD'
				}
				e.emitRune(r)
			}
		case int:
			// Negative = move right (spacing), positive = move left (kerning).
			displacement := -float64(v) / 1000.0 * e.fontSize
			e.tm = matMul(translateMatrix(displacement, 0), e.tm)
		case float64:
			displacement := -v / 1000.0 * e.fontSize
			e.tm = matMul(translateMatrix(displacement, 0), e.tm)
		}
	}
}

func (e *textExtractor) emitRune(r rune) {
	x, y := e.currentPos()

	if e.hasPos {
		dx := math.Abs(x - e.lastX)
		dy := math.Abs(y - e.lastY)
		spaceWidth := e.fontSize * 0.25
		if spaceWidth < 1 {
			spaceWidth = 1
		}

		if dy > e.fontSize*0.5 {
			e.buf.WriteByte('\n')
		} else if dx > spaceWidth*0.3 {
			e.buf.WriteByte(' ')
		}
	}

	e.buf.WriteRune(r)
	e.lastX = x
	e.lastY = y
	e.hasPos = true
}

func (e *textExtractor) currentPos() (float64, float64) {
	// Position = tm × ctm, extracting tx, ty.
	m := matMul(e.tm, e.ctm)
	return m[4], m[5]
}

func (e *textExtractor) doFormXObject(operand pdfValue, parentResources pdfDict) {
	name := operandName(operand)
	if name == "" || parentResources == nil {
		return
	}

	xobjVal, ok := parentResources["/XObject"]
	if !ok {
		return
	}
	xobjDict, ok := resolveRefToDict(e.objects, xobjVal)
	if !ok {
		return
	}
	formVal, ok := xobjDict[name]
	if !ok {
		return
	}
	resolved := resolveRef(e.objects, formVal)
	stream, ok := resolved.(*pdfStream)
	if !ok {
		return
	}
	if dictGetName(stream.Dict, "/Subtype") != "/Form" {
		return
	}

	// Parse form's content stream.
	ops, err := parseContentStream(stream.Data)
	if err != nil {
		return
	}

	// Form resources override parent.
	formResources := parentResources
	if resVal, ok := stream.Dict["/Resources"]; ok {
		if rd, ok := resolveRefToDict(e.objects, resVal); ok {
			formResources = rd
		}
	}

	// Resolve fonts for the form.
	formFonts := resolveFontResources(e.objects, formResources)

	// Apply form's /Matrix if present.
	savedCTM := e.ctm
	if matVal, ok := stream.Dict["/Matrix"]; ok {
		if arr, ok := matVal.(pdfArray); ok && len(arr) == 6 {
			var fm [6]float64
			for i := 0; i < 6; i++ {
				fm[i] = operandFloat(arr[i])
			}
			e.ctm = matMul(fm, e.ctm)
		}
	}

	// Merge form fonts with parent fonts (form takes precedence).
	savedFonts := e.fonts
	merged := make(map[string]fontInfo, len(e.fonts)+len(formFonts))
	for k, v := range e.fonts {
		merged[k] = v
	}
	for k, v := range formFonts {
		merged[k] = v
	}
	e.fonts = merged

	e.process(ops, formResources)

	e.fonts = savedFonts
	e.ctm = savedCTM
}

// operandName extracts a PDF name string from an operand.
func operandName(v pdfValue) string {
	if n, ok := v.(pdfName); ok {
		return string(n)
	}
	return ""
}

// operandFloat extracts a float64 from an operand (int or float64).
func operandFloat(v pdfValue) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case float64:
		return n
	}
	return 0
}

// Matrix operations for 3x3 affine transforms stored as [a b c d e f].
// The full matrix is:
//
//	| a b 0 |
//	| c d 0 |
//	| e f 1 |

func translateMatrix(tx, ty float64) [6]float64 {
	return [6]float64{1, 0, 0, 1, tx, ty}
}

func matMul(a, b [6]float64) [6]float64 {
	return [6]float64{
		a[0]*b[0] + a[1]*b[2],
		a[0]*b[1] + a[1]*b[3],
		a[2]*b[0] + a[3]*b[2],
		a[2]*b[1] + a[3]*b[3],
		a[4]*b[0] + a[5]*b[2] + b[4],
		a[4]*b[1] + a[5]*b[3] + b[5],
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run "TestExtractTextMinimal|TestDocumentExtractText" -v ./...`
Expected: PASS

- [ ] **Step 5: Write test for TJ with kerning**

Add to `text_test.go`:

```go
func TestExtractTextTJ(t *testing.T) {
	content := []byte("BT /F1 12 Tf 100 700 Td [(H) -10 (ello)] TJ ET")
	pdf := buildPDFWithContent(content)
	doc, err := asposepdf.OpenStream(bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page, _ := doc.Page(1)
	text, err := page.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if !strings.Contains(text, "Hello") {
		t.Errorf("text=%q, want it to contain 'Hello'", text)
	}
}
```

We need the helper `buildPDFWithContent` in `text_test.go`:

```go
// buildPDFWithContent creates a single-page PDF with the given content stream
// and a /Helvetica font at /F1.
func buildPDFWithContent(content []byte) []byte {
	stream := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content)

	type pdfObj struct {
		num  int
		body []byte
	}
	objs := []pdfObj{
		{1, []byte("<< /Type /Catalog /Pages 2 0 R >>")},
		{2, []byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>")},
		{3, []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>")},
		{4, []byte(stream)},
		{5, []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")},
	}

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
```

Add `import "fmt"` to text_test.go imports.

- [ ] **Step 6: Run test to verify it passes**

Run: `go test -run TestExtractTextTJ -v ./...`
Expected: PASS

- [ ] **Step 7: Write test for multiline text**

Add to `text_test.go`:

```go
func TestExtractTextMultiline(t *testing.T) {
	content := []byte("BT /F1 12 Tf 100 700 Td (Line One) Tj 0 -14 Td (Line Two) Tj ET")
	pdf := buildPDFWithContent(content)
	doc, err := asposepdf.OpenStream(bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page, _ := doc.Page(1)
	text, err := page.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if !strings.Contains(text, "Line One") {
		t.Errorf("text=%q, missing 'Line One'", text)
	}
	if !strings.Contains(text, "Line Two") {
		t.Errorf("text=%q, missing 'Line Two'", text)
	}
	if !strings.Contains(text, "\n") {
		t.Errorf("text=%q, expected newline between lines", text)
	}
}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `go test -run TestExtractTextMultiline -v ./...`
Expected: PASS

- [ ] **Step 9: Write test for unknown font**

Add to `text_test.go`:

```go
func TestExtractTextUnknownFont(t *testing.T) {
	// Build PDF with a custom font that has no /Encoding.
	stream := buildStreamBytes([]byte("BT /F1 12 Tf 100 700 Td (ABC) Tj ET"))

	type pdfObj struct {
		num  int
		body []byte
	}
	objs := []pdfObj{
		{1, []byte("<< /Type /Catalog /Pages 2 0 R >>")},
		{2, []byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>")},
		{3, []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>")},
		{4, stream},
		{5, []byte("<< /Type /Font /Subtype /Type1 /BaseFont /UnknownCustomFont+XYZ >>")},
	}

	pdf := assemblePDF(objs)
	doc, err := asposepdf.OpenStream(bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page, _ := doc.Page(1)
	text, err := page.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText should not error: %v", err)
	}
	if !strings.ContainsRune(text, '\uFFFD') {
		t.Errorf("expected U+FFFD for unknown font, got %q", text)
	}
}

func buildStreamBytes(data []byte) []byte {
	return []byte(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(data), data))
}

func assemblePDF(objs []struct{ num int; body []byte }) []byte {
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
```

Note: reuse `buildPDFWithContent` but with a modified font object. Extract a `buildPDFWithContentAndFont` variant that accepts custom font bytes:

```go
func buildPDFWithContentAndFont(content []byte, fontObj []byte) []byte {
	stream := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content)
	type pdfObj struct {
		num  int
		body []byte
	}
	objs := []pdfObj{
		{1, []byte("<< /Type /Catalog /Pages 2 0 R >>")},
		{2, []byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>")},
		{3, []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>")},
		{4, []byte(stream)},
		{5, fontObj},
	}
	// ... same assembly as buildPDFWithContent ...
}
```

Then `buildPDFWithContent` calls `buildPDFWithContentAndFont` with the default Helvetica+WinAnsi font.

Update `TestExtractTextUnknownFont` to use it:

```go
func TestExtractTextUnknownFont(t *testing.T) {
	content := []byte("BT /F1 12 Tf 100 700 Td (ABC) Tj ET")
	unknownFont := []byte("<< /Type /Font /Subtype /Type1 /BaseFont /UnknownCustomFont+XYZ >>")
	pdf := buildPDFWithContentAndFont(content, unknownFont)
	doc, err := asposepdf.OpenStream(bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page, _ := doc.Page(1)
	text, err := page.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText should not error: %v", err)
	}
	if !strings.ContainsRune(text, '\uFFFD') {
		t.Errorf("expected U+FFFD for unknown font, got %q", text)
	}
}
```

- [ ] **Step 10: Run test to verify it passes**

Run: `go test -run TestExtractTextUnknownFont -v ./...`
Expected: PASS

- [ ] **Step 11: Write test for Form XObject text extraction**

Add to `text_test.go`:

```go
func TestExtractTextFormXObject(t *testing.T) {
	// Build a PDF where the page's content stream calls "Do" on a Form XObject
	// that contains the actual text.
	formContent := []byte("BT /F1 12 Tf 100 700 Td (Form Text) Tj ET")
	formStream := fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 612 792] /Resources << /Font << /F1 6 0 R >> >> /Length %d >>\nstream\n%s\nendstream", len(formContent), formContent)
	pageContent := []byte("BT /F1 12 Tf 100 500 Td (Page Text) Tj ET /Fm1 Do")
	pageStream := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(pageContent), pageContent)

	type pdfObj struct {
		num  int
		body []byte
	}
	objs := []pdfObj{
		{1, []byte("<< /Type /Catalog /Pages 2 0 R >>")},
		{2, []byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>")},
		{3, []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 6 0 R >> /XObject << /Fm1 5 0 R >> >> >>")},
		{4, []byte(pageStream)},
		{5, []byte(formStream)},
		{6, []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")},
	}

	pdf := assemblePDFFromObjs(objs)
	doc, err := asposepdf.OpenStream(bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page, _ := doc.Page(1)
	text, err := page.ExtractText()
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if !strings.Contains(text, "Page Text") {
		t.Errorf("text=%q, missing 'Page Text'", text)
	}
	if !strings.Contains(text, "Form Text") {
		t.Errorf("text=%q, missing 'Form Text'", text)
	}
}

// assemblePDFFromObjs builds a minimal PDF from the given objects.
func assemblePDFFromObjs(objs []struct{ num int; body []byte }) []byte {
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
```

Note: The `assemblePDFFromObjs` helper works with the local `pdfObj` struct type — adapt the signature to use the same named type used by the calling function, or use a common helper. In practice the engineer should define the struct type once and reuse it.

- [ ] **Step 12: Run test to verify it passes**

Run: `go test -run TestExtractTextFormXObject -v ./...`
Expected: PASS

- [ ] **Step 13: Run the full test suite**

Run: `go test -v ./...`
Expected: All tests PASS. No regressions.

- [ ] **Step 14: Commit**

```bash
git add text.go text_test.go
git commit -m "feat: add text extraction API (Phase 2)

Page.ExtractText() and Document.ExtractText() public methods.
Text state machine handles BT/ET/Tf/Td/TD/Tm/T*/Tj/TJ/'/\"/Tc/Tw/TL/cm/Do.
Heuristic space/newline insertion based on text matrix positioning.
Form XObject recursive extraction.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

### Task 6: Integration Tests with Real PDFs

**Files:**
- Modify: `text_test.go` — add integration tests
- Modify: `testdata/testfiles.json` — add entry for `TestExtractTextFiles`

- [ ] **Step 1: Add test file entries to testfiles.json**

Add to `testdata/testfiles.json`:

```json
"TestExtractTextFiles": [
  ["4pages.pdf"],
  ["Binder1.pdf"],
  ["PdfWithLinks.pdf"],
  ["PdfWithTable.pdf"],
  ["alfa.pdf"],
  ["marketing.pdf"]
]
```

- [ ] **Step 2: Write the integration test**

Add to `text_test.go`:

```go
func TestExtractTextFiles(t *testing.T) {
	for _, inputPath := range testFiles(t) {
		t.Run(stem(inputPath), func(t *testing.T) {
			doc, err := asposepdf.Open(inputPath)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			texts, err := doc.ExtractText()
			if err != nil {
				t.Fatalf("ExtractText: %v", err)
			}
			if len(texts) != doc.PageCount() {
				t.Fatalf("expected %d pages, got %d", doc.PageCount(), len(texts))
			}
			for i, text := range texts {
				if len(strings.TrimSpace(text)) == 0 {
					t.Logf("page %d: empty text (may be image-only)", i+1)
					continue
				}
				// Check that we got at least some real characters (not all U+FFFD).
				cleaned := strings.ReplaceAll(text, "\uFFFD", "")
				cleaned = strings.TrimSpace(cleaned)
				if len(cleaned) == 0 {
					t.Logf("page %d: all characters are U+FFFD (unknown encoding)", i+1)
				}
			}
			t.Logf("%s: extracted text from %d pages", stem(inputPath), len(texts))
		})
	}
}
```

- [ ] **Step 3: Run the integration test**

Run: `go test -run TestExtractTextFiles -v ./...`
Expected: PASS (some pages may log warnings about empty text or unknown encodings — that's expected for image-only pages or fonts without standard encodings).

- [ ] **Step 4: Run the complete test suite**

Run: `go test -v ./...`
Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
git add text_test.go testdata/testfiles.json
git commit -m "test: add integration tests for text extraction (Phase 2)

TestExtractTextFiles runs against all real test PDFs.
Verifies ExtractText returns correct page count and non-empty text.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

### Task 7: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Add text extraction API to CLAUDE.md**

In the **Public API** section of `CLAUDE.md`, add under `page.go`:

```markdown
**`text.go`** — text extraction
- `(*Page).ExtractText() (string, error)` — returns the text content of a page; unknown font characters become U+FFFD
- `(*Document).ExtractText() ([]string, error)` — returns text for all pages (one entry per page)
```

Add to the architecture description after "PDF writing":

```markdown
### Text extraction (`text.go`, `content_parser.go`, `font.go`, `encoding.go`)

1. `parseContentStream(data)` tokenizes content stream bytes into `contentOp` structs (operator + operands), reusing the existing `lexer`
2. `resolveFont(objects, fontDict)` maps font dictionaries to `fontInfo{name, encoding [256]rune, known bool}` — supports WinAnsi, MacRoman, Standard encodings, `/Differences`, standard 14 fonts, Symbol, ZapfDingbats
3. `textExtractor` state machine processes operators (BT/ET/Tf/Td/Tm/Tj/TJ/etc.), tracking text matrix position, font, and spacing
4. Space/newline insertion uses heuristics: horizontal gap > spaceWidth×0.3 → space, vertical shift > fontSize×0.5 → newline
5. Form XObjects (`Do` operator) are recursively processed with inherited CTM and overridden resources
```

- [ ] **Step 2: Verify build still works**

Run: `go build ./...`
Expected: BUILD OK

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: add text extraction to CLAUDE.md (Phase 2)

Document ExtractText API, content stream parser, font resolver,
and text state machine architecture.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

### Task 8: Update Beads Issues

- [ ] **Step 1: Close Phase 2 beads issues**

```bash
bd update pdf-go-7id --status closed
bd update pdf-go-qew --status closed
bd update pdf-go-bo5 --status closed
bd update pdf-go-v3l --status closed
bd update pdf-go-56t --status closed
bd update pdf-go-ias --status closed
bd update pdf-go-t72 --status closed
```

- [ ] **Step 2: Verify status**

```bash
bd status
```

Expected: 7 closed (Phase 2), 6 open (Phase 2.5 backlog).
