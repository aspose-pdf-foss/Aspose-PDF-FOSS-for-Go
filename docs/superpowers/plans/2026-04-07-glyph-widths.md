# Glyph Width Metrics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement PDF spec 9.4.4 text matrix advancement using glyph widths to eliminate spurious spaces in extracted text.

**Architecture:** Extend `fontInfo` with a `widths [256]float64` field. Populate widths from `/Widths` in the font dictionary, falling back to built-in Standard 14 metrics, then to 600. After each glyph in `showString`/`showTJ`, advance the text matrix by the computed displacement. Refine the space-detection heuristic to use real font metrics.

**Tech Stack:** Pure Go, no dependencies. Width data from Adobe AFM files (public domain).

**Spec:** `docs/superpowers/specs/2026-04-07-glyph-widths-design.md`

**Beads epic:** `pdf-go-kqd`

---

## File Structure

| File | Responsibility |
|------|---------------|
| `font_metrics.go` | **New.** `[256]float64` width tables for all 14 standard PDF fonts + lookup function `standard14Widths(name) ([256]float64, bool)` |
| `font.go` | Add `widths` field to `fontInfo`; add `resolveWidths()` to parse `/Widths`+`/FirstChar`+`/LastChar` with Standard 14 fallback |
| `text.go` | Add `horizScaling` field; advance `tm` after each glyph in `showString`/`showTJ`; handle `Tz` operator; refine space heuristic in `emitRune` |
| `content_parser_test.go` | Tests for width resolution (Standard 14, `/Widths`, fallback) |
| `text_test.go` | Tests for glyph advance (no spurious spaces), `Tz` operator, integration re-verification |

---

### Task 1: Standard 14 font width tables

**Beads:** `pdf-go-kqd.1`

**Files:**
- Create: `font_metrics.go`
- Test: `content_parser_test.go` (append)

- [ ] **Step 1: Write the failing test for Standard 14 width lookup**

In `content_parser_test.go`, add:

```go
func TestStandard14Widths(t *testing.T) {
	// Helvetica: 'A' = 667, 'i' = 278, space = 278
	w, ok := standard14Widths("/Helvetica")
	if !ok {
		t.Fatal("expected Helvetica to be a standard 14 font")
	}
	if w[65] != 667 {
		t.Errorf("Helvetica 'A': got %v, want 667", w[65])
	}
	if w[105] != 278 {
		t.Errorf("Helvetica 'i': got %v, want 278", w[105])
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestStandard14Widths ./...`
Expected: compilation error — `standard14Widths` undefined.

- [ ] **Step 3: Create `font_metrics.go` with all 14 font width tables**

Create `font_metrics.go` with:
- `var helveticaWidths [256]float64` — from Helvetica.afm
- `var helveticaBoldWidths [256]float64` — from Helvetica-Bold.afm
- `var helveticaObliqueWidths [256]float64` — from Helvetica-Oblique.afm (same as Helvetica)
- `var helveticaBoldObliqueWidths [256]float64` — from Helvetica-BoldOblique.afm (same as Helvetica-Bold)
- `var timesRomanWidths [256]float64` — from Times-Roman.afm
- `var timesBoldWidths [256]float64` — from Times-Bold.afm
- `var timesItalicWidths [256]float64` — from Times-Italic.afm
- `var timesBoldItalicWidths [256]float64` — from Times-BoldItalic.afm
- `var courierWidths [256]float64` — all printable positions = 600
- `var symbolWidths [256]float64` — from Symbol.afm
- `var zapfDingbatsWidths [256]float64` — from ZapfDingbats.afm
- `func standard14Widths(name string) ([256]float64, bool)` — switch on `/BaseFont` name, returns table + ok

Width values are integers in 1/1000 em units (stored as `float64` for direct use). Positions 0-31 are 0. All data from Adobe AFM files (public domain, part of PDF spec). Use WinAnsiEncoding character positions for Helvetica/Times/Courier families. Courier Bold/Oblique/BoldOblique all share the same 600 table.

Note: Helvetica-Oblique has identical widths to Helvetica (oblique is just a slant transform). Helvetica-BoldOblique has identical widths to Helvetica-Bold. So only 4 unique Helvetica tables are needed (regular, bold), but for clarity reference them as separate vars (can alias: `var helveticaObliqueWidths = helveticaWidths`).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestStandard14Widths ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add font_metrics.go content_parser_test.go
git commit -m "feat: add Standard 14 font width tables"
```

---

### Task 2: Parse `/Widths` from font dictionary

**Beads:** `pdf-go-kqd.2`

**Files:**
- Modify: `font.go` — add `widths` field to `fontInfo`, add width resolution logic to `resolveFont`
- Test: `content_parser_test.go` (append)

- [ ] **Step 1: Write the failing test for `/Widths` parsing**

In `content_parser_test.go`, add:

```go
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
	// /Widths should override Standard 14 defaults.
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
	// No /Widths — should fall back to Standard 14.
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
	// Unknown font, no /Widths — should fall back to 600.
	if fi.widths[65] != 600 {
		t.Errorf("widths[65]: got %v, want 600 (fallback)", fi.widths[65])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run "TestResolveFontWidths" ./...`
Expected: FAIL — `fi.widths` is zero-valued (field doesn't exist yet or is all zeros).

- [ ] **Step 3: Add `widths` field to `fontInfo` and width resolution to `resolveFont`**

In `font.go`, add `widths [256]float64` to `fontInfo`:

```go
type fontInfo struct {
	name     string
	encoding [256]rune
	widths   [256]float64 // character code → width in 1/1000 text space units
	known    bool
}
```

At the end of `resolveFont`, before returning, add width resolution. The logic (applied regardless of which `return` path is taken — refactor to single exit point):

```go
// Resolve widths. Priority: /Widths dict > Standard 14 > fallback 600.
fi.widths = resolveWidths(objects, fontDict, fi.name)
return fi
```

Add new function `resolveWidths`:

```go
func resolveWidths(objects map[int]*pdfObject, fontDict pdfDict, baseFontName string) [256]float64 {
	var widths [256]float64

	// Try /Widths + /FirstChar + /LastChar from font dict.
	if wVal, ok := fontDict["/Widths"]; ok {
		firstChar := dictGetInt(fontDict, "/FirstChar")
		lastChar := dictGetInt(fontDict, "/LastChar")
		wResolved := resolveRef(objects, wVal)
		if arr, ok := wResolved.(pdfArray); ok {
			for i, v := range arr {
				code := firstChar + i
				if code >= 0 && code < 256 && i <= lastChar-firstChar {
					widths[code] = operandFloat(v)
				}
			}
			return widths
		}
	}

	// Fallback: Standard 14 built-in metrics.
	if std, ok := standard14Widths(baseFontName); ok {
		return std
	}

	// Last resort: monospaced fallback.
	for i := 32; i < 256; i++ {
		widths[i] = 600
	}
	return widths
}
```

Refactor `resolveFont` to have a single exit point so `resolveWidths` is always called. Change structure to:

```go
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
	}

	if !fi.known && !hasEncoding {
		if isStandard14(name) {
			fi.encoding = defaultEncodingForFont(name)
			fi.known = true
		} else {
			for i := range fi.encoding {
				fi.encoding[i] = '\uFFFD'
			}
		}
	}

	fi.widths = resolveWidths(objects, fontDict, name)
	return fi
}
```

- [ ] **Step 4: Run all font-related tests**

Run: `go test -run "TestResolveFont|TestApplyDifferences|TestStandard14" ./...`
Expected: all PASS (existing tests must not break).

- [ ] **Step 5: Commit**

```bash
git add font.go content_parser_test.go
git commit -m "feat: parse /Widths from font dictionary with Standard 14 fallback"
```

---

### Task 3: Advance text matrix after each glyph

**Beads:** `pdf-go-kqd.3`

**Files:**
- Modify: `text.go` — change `showString` and `showTJ` to advance `tm`; add `horizScaling` field; handle `Tz` operator; refine `emitRune` heuristic
- Test: `text_test.go` (append)

- [ ] **Step 1: Write the failing test for glyph advance**

In `text_test.go`, add a test that constructs a PDF where text is split across multiple Tj calls with Td offsets — the exact pattern that causes spurious spaces. The Td offset should be the exact width of the preceding string (no gap = no space).

```go
func TestExtractTextNoSpuriousSpaces(t *testing.T) {
	// Simulate the pattern that causes "shap e":
	// (shap) Tj <advance-by-width-of-shap> Td (e the) Tj
	// With Helvetica at 12pt, "shap" widths: s=556, h=556, a=556, p=556 = 2224
	// In text space: 2224/1000 * 12 = 26.688 points
	content := []byte("BT /F1 12 Tf 100 700 Td (shap) Tj 26.688 0 Td (e the) Tj ET")
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
	// With correct glyph advance, "shap" advances tm by ~26.688,
	// then Td moves by 26.688, so dx ≈ 0 — no space inserted.
	if strings.Contains(text, "shap e") {
		t.Errorf("spurious space detected: %q", text)
	}
	if !strings.Contains(text, "shape the") {
		t.Errorf("text=%q, want it to contain 'shape the'", text)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestExtractTextNoSpuriousSpaces ./...`
Expected: FAIL — text contains `"shap e"` because `tm` is not advanced.

- [ ] **Step 3: Add `horizScaling` field and `Tz` operator handling**

In `text.go`, add `horizScaling` to the `textExtractor` struct:

```go
type textExtractor struct {
	objects map[int]*pdfObject
	fonts   map[string]fontInfo

	// Text state.
	font         fontInfo
	fontSize     float64
	charSpace    float64
	wordSpace    float64
	leading      float64
	horizScaling float64 // Tz / 100; default 1.0
	tm           [6]float64
	lm           [6]float64
	ctm          [6]float64
	ctmStack     [][6]float64

	// Output.
	buf    strings.Builder
	lastX  float64
	lastY  float64
	hasPos bool
}
```

In `newTextExtractor`, set default:

```go
func newTextExtractor(objects map[int]*pdfObject, fonts map[string]fontInfo) *textExtractor {
	return &textExtractor{
		objects:      objects,
		fonts:        fonts,
		ctm:          identityMatrix(),
		horizScaling: 1.0,
	}
}
```

In `process()`, add the `Tz` case after `TL`:

```go
		case "Tz":
			if len(op.Operands) >= 1 {
				e.horizScaling = operandFloat(op.Operands[0]) / 100.0
			}
```

- [ ] **Step 4: Add `advanceGlyph` method and modify `showString`**

Add a new method to `textExtractor` that advances `tm` after a glyph:

```go
func (e *textExtractor) advanceGlyph(code byte) {
	w0 := e.font.widths[code]
	tx := (w0/1000.0*e.fontSize + e.charSpace) * e.horizScaling
	if code == 32 {
		tx += e.wordSpace * e.horizScaling
	}
	e.tm = matMul(translateMatrix(tx, 0), e.tm)
}
```

Modify `showString` to call `advanceGlyph` after each character:

```go
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
		e.advanceGlyph(code)
	}
}
```

- [ ] **Step 5: Modify `showTJ` to call `advanceGlyph`**

```go
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
				e.advanceGlyph(code)
			}
		case int:
			displacement := -float64(v) / 1000.0 * e.fontSize
			e.tm = matMul(translateMatrix(displacement, 0), e.tm)
		case float64:
			displacement := -v / 1000.0 * e.fontSize
			e.tm = matMul(translateMatrix(displacement, 0), e.tm)
		}
	}
}
```

- [ ] **Step 6: Refine `emitRune` space-detection heuristic**

Now that `tm` advances correctly, `dx` represents actual gap between glyphs. Update `emitRune` to use the font's space width from metrics:

```go
func (e *textExtractor) emitRune(r rune) {
	x, y := e.currentPos()

	if e.hasPos {
		dx := x - e.lastX
		dy := e.lastY - y

		// Use the font's space character width for space detection.
		spaceWidth := e.font.widths[32] / 1000.0 * e.fontSize
		if spaceWidth < 1 {
			spaceWidth = e.fontSize * 0.25
		}
		if spaceWidth < 1 {
			spaceWidth = 1
		}

		if math.Abs(dy) > e.fontSize*0.5 {
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
```

- [ ] **Step 7: Run the new test**

Run: `go test -run TestExtractTextNoSpuriousSpaces ./...`
Expected: PASS — `"shape the"` with no spurious space.

- [ ] **Step 8: Run ALL existing tests to verify no regressions**

Run: `go test ./...`
Expected: all PASS. If any existing test fails, investigate and fix (the new glyph advance changes character positioning, which may affect space/newline detection in existing tests).

- [ ] **Step 9: Commit**

```bash
git add text.go text_test.go
git commit -m "feat: advance text matrix after each glyph (PDF spec 9.4.4)"
```

---

### Task 4: Tz operator test

**Beads:** `pdf-go-kqd.4`

**Files:**
- Test: `text_test.go` (append)

- [ ] **Step 1: Write test for Tz operator**

```go
func TestExtractTextHorizScaling(t *testing.T) {
	// Tz 200 doubles horizontal scaling — glyph advance doubles.
	// With doubled advance, "AB" from two separate Tj ops should still join.
	// Helvetica 'A' = 667, at 12pt normal advance = 667/1000*12 = 8.004
	// With Tz 200: advance = 8.004 * 2 = 16.008
	content := []byte("BT /F1 12 Tf 200 Tz 100 700 Td (A) Tj 16.008 0 Td (B) Tj ET")
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
	if strings.Contains(text, "A B") {
		t.Errorf("spurious space with Tz: %q", text)
	}
	if !strings.Contains(text, "AB") {
		t.Errorf("text=%q, want 'AB'", text)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test -run TestExtractTextHorizScaling ./...`
Expected: PASS (Tz was already implemented in Task 3).

- [ ] **Step 3: Commit**

```bash
git add text_test.go
git commit -m "test: add Tz horizontal scaling operator test"
```

---

### Task 5: Integration verification

**Beads:** `pdf-go-kqd.5`, `pdf-go-kqd.6`

**Files:**
- Test: `text_test.go` — verify all integration tests, check marketing.pdf quality

- [ ] **Step 1: Run full test suite**

Run: `go test -v ./...`
Expected: all tests PASS.

- [ ] **Step 2: Run TestExtractTextFiles and inspect marketing.pdf output**

Run: `go test -run TestExtractTextFiles -v ./...`

Then inspect `result_files/TestExtractTextFiles/marketing/full_text.txt`. Verify:
- No spurious mid-word spaces: `"shape"` not `"shap e"`, `"with"` not `"w ith"`, `"http://web.mit.edu/"` not `"http://web.mit.e du/"`
- `"PROFESSIONAL"` not `"P ROFESSIONAL"` (letterspaced text should join)
- Line breaks between paragraphs are preserved
- U+FFFD for bullets is expected (Phase 2.5 issue, not this task)

If marketing.pdf still has issues, investigate the specific content stream and tune thresholds. The fix should be universal — do not hardcode anything specific to this file.

- [ ] **Step 3: Inspect other test PDFs**

Check `result_files/TestExtractTextFiles/` for all 6 PDFs. Verify no regressions (text quality should be same or better).

- [ ] **Step 4: Commit any threshold adjustments**

If threshold tuning was needed:

```bash
git add text.go
git commit -m "fix: tune space-detection threshold for glyph-advance model"
```

---

### Task 6: Update CLAUDE.md and close beads

**Files:**
- Modify: `CLAUDE.md` — update text extraction architecture section

- [ ] **Step 1: Update architecture docs**

In `CLAUDE.md`, update the "Text extraction" section to mention glyph widths:

Change:
```
3. `textExtractor` state machine processes operators (BT/ET/Tf/Td/Tm/Tj/TJ/etc.), tracking text matrix position, font, and spacing
```

To:
```
3. `textExtractor` state machine processes operators (BT/ET/Tf/Td/Tm/Tj/TJ/Tz/etc.), tracking text matrix position, font, spacing, and horizontal scaling; advances text matrix by glyph width after each character (PDF spec 9.4.4)
```

Add to the font.go description:
```
- `font_metrics.go` — built-in width tables for Standard 14 PDF fonts (from Adobe AFM data); `standard14Widths(name)` lookup
```

- [ ] **Step 2: Commit docs update**

```bash
git add CLAUDE.md
git commit -m "docs: update architecture for glyph width metrics"
```

- [ ] **Step 3: Close beads tasks**

```bash
bd update pdf-go-kqd.1 --status closed
bd update pdf-go-kqd.2 --status closed
bd update pdf-go-kqd.3 --status closed
bd update pdf-go-kqd.4 --status closed
bd update pdf-go-kqd.5 --status closed
bd update pdf-go-kqd.6 --status closed
bd update pdf-go-kqd --status closed
```
