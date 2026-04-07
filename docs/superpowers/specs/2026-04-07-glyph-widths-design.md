# Glyph Width Metrics for Text Extraction

**Date:** 2026-04-07
**Status:** Approved
**Branch:** phase2-text-extraction

## Problem

Text extraction produces broken words (`"shap e"`, `"htt p"`, `"P ROFESSIONAL"`) and broken URLs (`"http://web.mit.e du/"`). The root cause: after `Tj`/`TJ` operators, the text matrix is not advanced by the rendered glyph widths. Per PDF spec 9.4.4, after each glyph the text matrix must shift by:

```
tx = ((w0/1000 * Tfs) + Tc + Tw) * Th
```

Without `w0` (glyph width), the text matrix stays at the start of each string. Subsequent `Td` offsets are then measured from the wrong origin, causing the space-detection heuristic to fire on normal character gaps.

Secondary issue: bullet characters (U+2022) render as U+FFFD because the bullet font's encoding is unresolved — this is a separate encoding problem already tracked in Phase 2.5.

## Solution: Approach B — Standard 14 Metrics + `/Widths` from Font Dict

### 1. Extend `fontInfo` with width data

Add a `widths [256]float64` field to `fontInfo`. Values are in 1/1000 text space units (standard PDF convention).

**File:** `font.go`

### 2. Built-in Standard 14 font metrics

New file `font_metrics.go` containing `[256]float64` width tables for all 14 standard PDF fonts:

- **Courier family** (4 variants): all widths = 600 (monospaced)
- **Helvetica family** (4 variants): variable widths from AFM data
- **Times-Roman family** (4 variants): variable widths from AFM data
- **Symbol**: widths from AFM data
- **ZapfDingbats**: widths from AFM data

Source: Adobe Font Metrics (AFM) files, which are public domain and part of the PDF specification. Only the character widths are needed (not kerning pairs or other metrics).

### 3. Width resolution in `resolveFont`

Priority order:
1. Read `/Widths` array + `/FirstChar` + `/LastChar` from font dictionary — fill `widths[]` for the specified range
2. If `/Widths` is absent/incomplete and font is Standard 14 — use built-in metrics table
3. If neither available — fallback to 600 (average monospaced width)

For `/Widths` parsing: the array contains `LastChar - FirstChar + 1` values. Each value is the width of the character at position `FirstChar + i`.

### 4. Text matrix advancement in `showString` / `showTJ`

After emitting each glyph, advance the text matrix:

```go
w0 := e.font.widths[code]
tx := w0/1000.0*e.fontSize + e.charSpace
if code == 32 {
    tx += e.wordSpace
}
e.tm = matMul(translateMatrix(tx, 0), e.tm)
```

Also add `horizScaling` field to `textExtractor` (default 1.0), set by `Tz` operator. Apply as `tx *= e.horizScaling`.

**File:** `text.go`

### 5. Improved space-detection heuristic

With correct text matrix advancement, `dx` in `emitRune` will represent the actual gap between glyphs (not the distance from string start). The space-detection threshold can be refined:

- Use the actual space character width from font metrics (`font.widths[32]`) instead of `fontSize * 0.25`
- Keep threshold at ~0.3× space width or raise slightly if needed

### 6. `Tz` operator support

Add handling for the `Tz` (horizontal scaling) operator in `textExtractor.process()`. This affects glyph displacement calculation. Default value is 100 (meaning 1.0 scaling factor). Store as `horizScaling = operandFloat / 100.0`.

## Files Changed

| File | Change |
|------|--------|
| `font.go` | Add `widths` field to `fontInfo`; parse `/Widths`+`/FirstChar`+`/LastChar`; fallback to Standard 14 |
| `font_metrics.go` | **New.** Built-in width tables for 14 standard fonts |
| `text.go` | Advance `tm` after each glyph in `showString`/`showTJ`; add `Tz` operator; refine space heuristic |
| `content_parser_test.go` | Tests for `/Widths` parsing |
| `text_test.go` | Test for glyph advance (no spurious spaces); re-verify all existing tests |

## Out of Scope

- ToUnicode CMap (Phase 2.5, tracked separately)
- CIDFont `/DW` and `/W` width arrays (Phase 2.5, CIDFont task)
- Kerning pairs from AFM data (not needed for text extraction positioning)
- `/MissingWidth` from FontDescriptor (minor optimization, can add later)
