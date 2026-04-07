# Visual Text Sorting + ExtractTextWithLayout

**Date:** 2026-04-07
**Status:** Approved
**Branch:** phase2.5-advanced-text-extraction
**Beads:** pdf-go-vg8.7, pdf-go-vg8.3

## Problem

`ExtractText()` outputs text in content stream order, not visual (reading) order. Example: the "11/06" footer in marketing.pdf page 2 appears at the top of extracted text because the PDF content stream draws it first. Users expect top-to-bottom, left-to-right reading order.

Additionally, there is no API to get structured text with coordinates — users who need font info, positions, or per-line data have no option.

## Solution

### 1. Refactor `textExtractor` to collect fragments

Replace direct `strings.Builder` output with a `[]textFragment` buffer. Each fragment is a contiguous run of text at a single position with consistent font parameters.

```go
// textFragment is an internal type — a contiguous run of text.
type textFragment struct {
    text     strings.Builder
    x, y     float64 // device-space position of first rune
    fontName string
    fontSize float64 // effective font size in device space
}
```

`emitRune` changes:
- Instead of writing to `buf`, append runes to the current fragment.
- Start a new fragment when:
  - A space is detected (dx > threshold) — flush current fragment, start new one at the new x position
  - A newline is detected (dy > threshold) — flush current fragment, start new one
  - Font changes (different fontName or fontSize)
- Space and newline characters are NOT stored in fragments — they are reconstructed during line assembly from gaps between fragments.

**File:** `text.go`

### 2. Group fragments into lines and sort

New file `text_layout.go` with public types and grouping logic.

**Public types:**

```go
// TextFragment represents a contiguous run of text with uniform font.
type TextFragment struct {
    Text     string
    X        float64 // horizontal position in points (from left edge)
    FontName string  // e.g. "Helvetica", "Arial-BoldMT"
    FontSize float64 // effective size in points
}

// TextLine represents a horizontal line of text fragments at a common Y position.
type TextLine struct {
    Text      string         // concatenated text of all fragments (with spaces)
    Y         float64        // vertical position in points (from bottom edge)
    Fragments []TextFragment
}
```

**Grouping algorithm:**

1. Collect all fragments from the text extractor.
2. Sort fragments by Y descending (top of page first), then X ascending (left-to-right).
3. Group into lines: fragments with Y values within `effectiveFontSize * 0.3` of each other belong to the same line.
4. For each line:
   - Set `TextLine.Y` to the Y of the first fragment.
   - Sort fragments by X ascending.
   - Build `TextLine.Text` by concatenating fragment texts, inserting spaces where the X gap between consecutive fragments exceeds `spaceWidth * 0.3`.
   - Strip the `/SUBSET+` prefix from font names for cleaner output (e.g., `/MCEFGG+Garamond-Bold` → `Garamond-Bold`).
5. Sort lines by Y descending (top to bottom).

**File:** `text_layout.go`

### 3. New public method `ExtractTextWithLayout`

```go
// ExtractTextWithLayout returns structured text lines sorted in visual
// (top-to-bottom, left-to-right) reading order. Each line contains
// its concatenated text and individual fragments with positions.
func (p *Page) ExtractTextWithLayout() ([]TextLine, error)
```

Also add `Document`-level method:

```go
func (d *Document) ExtractTextWithLayout() ([][]TextLine, error)
```

Returns `[][]TextLine` — one `[]TextLine` per page.

**File:** `text_layout.go`

### 4. Update `ExtractText` to use visual sorting

`ExtractText()` now uses the same fragment collection + grouping pipeline, then joins lines with `\n`. This changes the output order from content-stream to visual, which is the correct behavior.

```go
func (p *Page) ExtractText() (string, error) {
    lines, err := p.ExtractTextWithLayout()
    if err != nil {
        return "", err
    }
    // Join line texts with newlines.
    var buf strings.Builder
    for i, line := range lines {
        if i > 0 {
            buf.WriteByte('\n')
        }
        buf.WriteString(line.Text)
    }
    return cleanExtractedText(buf.String()), nil
}
```

**File:** `text.go`

### 5. Space detection during line assembly

Space insertion moves from `emitRune` to the line assembly phase. During assembly, for each pair of adjacent fragments on the same line, compute the gap:

```
gap = fragment[i+1].X - (fragment[i].X + fragment[i].estimatedWidth)
```

Since we don't store fragment width, use a simpler heuristic: if `fragment[i+1].X - lastX > effectiveSpaceWidth * 0.3`, insert a space. Here `lastX` is tracked during assembly as the X position after the last fragment's text.

Actually simpler: during `emitRune`, track the position after each glyph advance. When starting a new fragment, record both the fragment's start X and the previous glyph's end X. The gap between `endX` of fragment N and `startX` of fragment N+1 determines spaces.

To make this work, store `endX` in each fragment:

```go
type textFragment struct {
    text     strings.Builder
    x, y     float64 // position of first rune
    endX     float64 // position after last glyph advance
    fontName string
    fontSize float64
}
```

During line assembly: `gap = next.x - prev.endX`. If `gap > effectiveSpaceWidth * 0.3`, insert space.

### 6. Newline detection during line output

Newlines between lines are determined by the Y gap between consecutive sorted lines. If the gap exceeds `fontSize * 1.5`, insert an extra blank line (to preserve paragraph spacing). Otherwise, just `\n`.

## Files Changed

| File | Change |
|------|--------|
| `text.go` | Refactor `emitRune` to collect `[]textFragment`; update `ExtractText` to use visual sorting; remove `buf`, `lastX`, `lastY`, `hasPos` from `textExtractor` |
| `text_layout.go` | **New.** Public types `TextFragment`, `TextLine`; grouping/sorting logic; `ExtractTextWithLayout` methods on `Page` and `Document` |
| `text_test.go` | Update existing tests (output may change order); add tests for visual sorting |
| `text_layout_test.go` | **New.** Tests for `ExtractTextWithLayout`, fragment grouping, line assembly |

## Out of Scope

- Column detection (multi-column layouts) — would need horizontal zone analysis
- Right-to-left text ordering
- Vertical text (CJK vertical writing modes)
- Fragment-level width/height in `TextFragment` (can be added later)
