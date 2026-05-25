# Vector Graphics Phase 3b — SVG `<text>` Rendering

**Beads:** [pdf-go-jkn](bd show pdf-go-jkn) (Phase 3b) under umbrella [pdf-go-ybu](bd show pdf-go-ybu) (Vector support)
**Date:** 2026-05-25
**Status:** Design proposed

---

## Roadmap context

| Phase | Scope | Status |
|---|---|---|
| 1 | Native drawing primitives on `(*Page)` | ✅ Shipped (v0.1.0) |
| 2 | SVG-lite embedding (shapes, paths, transforms, viewBox) | ✅ Shipped |
| 3a | SVG gradients (linear + radial via PDF Type 2/3 shading patterns) | ✅ Shipped |
| **3b (this spec)** | SVG `<text>` rendering with font matching | Designing |
| 3c | SVG `<image>` (data-uri raster) + `<defs>`/`<use>` + masks/clipPath | Future |
| 3d | CSS subset, filters, markers, exotic units, advanced spreadMethod, textPath | Future |

Phase 3b adds the second-most-impactful SVG feature after gradients — text rendering. Icons, infographics, technical diagrams, and most real-world SVG content contain text elements.

---

## Phase 3b goals

Render `<text>` and `<tspan>` elements from SVG content into PDF. The text uses the same `pdf.Font` abstraction as the existing `(*Page).AddText` API — Standard 14 fonts mapped via heuristic from SVG `font-family` keywords, plus an optional callback for embedded TTF fonts (Cyrillic, custom fonts).

### Non-goals (Phase 3d candidates)

- `<textPath>` — text laid out along an arbitrary `<path>`. Complex glyph-by-glyph placement on parametric curves
- Vertical text (`writing-mode="tb"`) — east-Asian convention; rare in Western SVG
- `text-decoration` (underline/overline/line-through) — straightforward but not in scope here
- `word-spacing` / `letter-spacing` — adjustable inter-glyph metrics
- `font-stretch` (condensed/expanded)
- CSS system font keywords (`caption`, `icon`, `menu`...)
- CSS `font` shorthand (parsing the multi-part shorthand)
- `xml:space="preserve"` — preserves significant whitespace verbatim
- `lengthAdjust` / `textLength` — fit text to specific width

---

## Scope summary

| Element / attribute | Phase 3b support |
|---|---|
| `<text>` | Container element; emits one or more text runs |
| `<tspan>` | Nested text run with own styling; supports nested tspan |
| `x` / `y` attrs | Set absolute cursor position (on both `<text>` and `<tspan>`) |
| `dx` / `dy` attrs | Relative offset from current cursor (on `<tspan>`) |
| `text-anchor` | `start` (default), `middle`, `end` |
| `font-family` | Heuristic mapping + optional callback |
| `font-size` | In user units (default 16, per CSS) |
| `font-weight` | `normal`, `bold`, numeric (≥600 → bold) |
| `font-style` | `normal`, `italic`, `oblique` |
| `fill` / `stroke` / `opacity` / `fill-opacity` / `stroke-opacity` | Same cascade as shapes (via existing `svgPaint`); gradient fills (Phase 3a) supported |
| `transform` | Applied via PDF `cm` operator (same as shapes) |
| Mixed content | CharData + `<tspan>` + CharData interleaved within one `<text>` |
| Whitespace | Default normalization: collapse whitespace sequences to single space, trim leading/trailing |

---

## Public API

One new function type + one new method on `*Document`:

```go
// SVGFontResolver maps an SVG font-family + style to a pdf.Font.
// Return nil to fall back to the library's built-in heuristic
// (Standard 14 mapping based on family keyword).
type SVGFontResolver func(family string, bold, italic bool) Font

// SetSVGFontResolver installs a custom resolver. The renderer queries it
// first for each SVG font-family encountered; on nil return, falls back
// to the heuristic. Use this to plug in embedded TTF fonts (Cyrillic, etc.)
// loaded via Document.LoadFont.
//
// Pass nil to clear a previously-set resolver (revert to heuristic-only).
func (d *Document) SetSVGFontResolver(fn SVGFontResolver)
```

### Example: Cyrillic SVG

```go
doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
deja, _ := doc.LoadFont("testdata/DejaVuSans.ttf")
doc.SetSVGFontResolver(func(family string, bold, italic bool) pdf.Font {
    if strings.EqualFold(family, "DejaVu Sans") {
        return deja
    }
    return nil // fall back to heuristic
})
page, _ := doc.Page(1)
page.AddSVG("diagram-with-russian-labels.svg", pdf.Rectangle{...})
```

### Font resolution priority

1. `doc.svgFontResolver(family, bold, italic)` — if non-nil and returns non-nil
2. `heuristicFont(family, bold, italic)` — built-in Standard 14 mapping
3. Implicit fallback inside heuristic: `FontHelvetica` (or its bold/italic variants)

---

## Internal architecture

### Files

| File | Responsibility |
|---|---|
| `svg_text.go` (new) | IR types (`svgText`, `svgTextRun`, `svgTextAnchor` enum, `svgTextStyle`) + `heuristicFont` mapping |
| `svg_parse_text.go` (new) | XML parser for `<text>` / `<tspan>`; cursor-driven run emission with mixed-content handling, whitespace normalization, dx/dy/abs x/y |
| `svg_render_text.go` (new) | Renderer: `renderSVGText`, `resolveSVGFont` (callback + heuristic), `measureSVGText` (for text-anchor positioning), `emitSVGTextRun` (PDF BT/ET/Tf/Tm/Tj) |
| `svg_types.go` (modify) | Add `text` style fields to `svgStyle` (fontFamily, fontSize, bold, italic, anchor) |
| `svg_parse.go` (modify) | Walker recognizes `<text>`; dispatches `<text>` → `parseSVGText`; `applySingleSVGStyleProp` handles text-related properties |
| `svg_render.go` (modify) | `renderSVGNode` dispatches `*svgText` → `renderSVGText` |
| `document.go` (modify) | Add `svgFontResolver SVGFontResolver` field + `SetSVGFontResolver` method |
| `svg_text_test.go` / `svg_parse_text_test.go` / `svg_render_text_test.go` | Unit + integration coverage |
| `testdata/svg/text_*.svg` | Fixtures: basic, anchors, tspan, cyrillic, mixed-content |

### Internal types

```go
type svgTextAnchor int

const (
    svgTextAnchorStart  svgTextAnchor = 0 // default (left of x)
    svgTextAnchorMiddle svgTextAnchor = 1
    svgTextAnchorEnd    svgTextAnchor = 2
)

// Extended svgStyle: text-specific attributes inherited through the cascade.
type svgStyle struct {
    // ... existing fields (fill, stroke, etc.) ...

    // Text-specific (new in Phase 3b)
    fontFamily string  // "Arial", "Times", etc. — empty inherits
    fontSize   float64 // user units; default 16
    bold       bool
    italic     bool
    anchor     svgTextAnchor
}

// svgTextRun is a single contiguous glyph run with uniform styling at (x, y).
type svgTextRun struct {
    text  string
    x, y  float64 // absolute position (after dx/dy resolution at parse time)
    style svgStyle
}

// svgText is the <text> element IR node.
type svgText struct {
    runs      []svgTextRun
    style     svgStyle
    transform *svgMatrix
}

func (*svgText) svgNodeKind() string { return "text" }
```

### Parsing algorithm

`<text>` and `<tspan>` are mixed-content XML elements:

```xml
<text x="10" y="20" font-family="Arial" font-size="14">
  Hello <tspan font-weight="bold">world</tspan>!
</text>
```

Produces three runs:
1. `"Hello "` at `(10, 20)`, regular
2. `"world"` at `(10 + width("Hello "), 20)`, bold
3. `"!"` at `(10 + width("Hello ") + width("world"), 20)`, regular

The parser maintains a cursor `(cx, cy)` and walks XML tokens:

- `xml.StartElement <text x=... y=...>` → `cx, cy = x, y`; recurse into children
- `xml.CharData "..."` → normalize whitespace; emit run at cursor with current style; advance cursor by run width
- `xml.StartElement <tspan>`:
  - If `x` / `y` attrs present: set cursor to absolute
  - Else if `dx` / `dy` attrs present: shift cursor
  - Push style stack (inherited + tspan attrs)
  - Recurse into children
  - Pop style stack
- `xml.EndElement` → return from recursion

### Rendering algorithm

```go
func renderSVGText(buf *bytes.Buffer, p *Page, svg *SVG, t *svgText) {
    if !t.style.display || len(t.runs) == 0 {
        return
    }
    buf.WriteString("q\n")
    if t.transform != nil {
        writeCMOperator(buf, *t.transform)
    }
    for _, run := range t.runs {
        font := resolveSVGFont(p.doc, run.style)
        if font == nil {
            continue // best-effort: skip
        }
        emitSVGTextRun(buf, p, run, font)
    }
    buf.WriteString("Q\n")
}
```

`emitSVGTextRun` emits the PDF text block:

```
q
[/PatternX scn] (only if fill is a gradient — Phase 3a integration)
[r g b rg] (only if plain color fill)
BT
/Fxx <size> Tf
[anchor-adjusted x] [y] Tm   ← actually a full text matrix [1 0 0 -1 x y]
(text-bytes) Tj
ET
Q
```

The text matrix `[1 0 0 -1 x y]` includes a local Y-flip: the outer CTM from `computeViewBoxMatrix` flipped Y for the whole SVG, but PDF text glyphs are drawn upward from baseline. Combining the two gives correct orientation.

### Text width measurement

Reuse the font-metrics infrastructure from text_add.go:

- Standard 14: built-in width tables (already present)
- Embedded TTF: `hmtx` table lookup (already present)

Returns the total advance width of the run's glyphs at the run's font + size.

Used for:
- Cursor advancement after each run (parse time)
- Text-anchor positioning (render time): `middle` → `x - width/2`; `end` → `x - width`

### Heuristic font matcher

```go
func heuristicFont(family string, bold, italic bool) Font {
    f := normalizeFontFamily(family) // lowercase, trim, strip quotes, take first comma-separated entry
    switch {
    case isMonospace(f):
        return chooseCourier(bold, italic)
    case isSerif(f):
        return chooseTimes(bold, italic)
    default:
        return chooseHelvetica(bold, italic)
    }
}

func isMonospace(f string) bool {
    return strings.Contains(f, "courier") || strings.Contains(f, "monospace") ||
        strings.Contains(f, "mono")
}

func isSerif(f string) bool {
    return strings.Contains(f, "serif") || strings.Contains(f, "times") ||
        strings.Contains(f, "georgia") || strings.Contains(f, "garamond")
}
```

### Cascade integration

Text-specific attributes (`font-family`, `font-size`, `font-weight`, `font-style`, `text-anchor`) join the existing presentation-attr cascade in `applySingleSVGStyleProp`. Inheritance through `<g>` groups works automatically — a `<g font-family="Times">` propagates to all text descendants.

---

## Key behaviors

### Y-flip for baseline

The outer CTM applied by `computeViewBoxMatrix` does `scale(scaleX, -scaleY)` — Y is flipped for the whole SVG content. This works for shapes (path operators `m`/`l`/`c`) but breaks text: glyphs would render upside-down.

**Fix**: each `BT` block uses text matrix `[1 0 0 -1 x y]`, which locally re-flips Y inside the text block. Composed with the outer CTM, this gives upright text positioned at SVG-coordinates `(x, y)`.

### Text-anchor implementation

| `text-anchor` | Glyphs relative to (x, y) | PDF x-offset in text matrix |
|---|---|---|
| `start` (default) | x is left of baseline | `x` |
| `middle` | x is center of baseline | `x - width/2` |
| `end` | x is right of baseline | `x - width` |

Width is measured at the time of rendering using the resolved font's metrics.

### Whitespace normalization (xml:space="default")

```go
func normalizeSVGTextWhitespace(s string) string {
    return strings.Join(strings.Fields(s), " ")
}
```

`strings.Fields` splits on any whitespace; `Join(" ")` collapses to single spaces. Leading/trailing whitespace is removed.

For Phase 3b, this is applied unconditionally. `xml:space="preserve"` (rarely used) is deferred to Phase 3d.

### Mixed content semantics

```xml
<text>A <tspan>B </tspan>C</text>
```

Parse order: CharData "A ", StartElement tspan, CharData "B ", EndElement tspan, CharData "C". Each piece becomes a run, cursor advances continuously. The space between "A" and "B" is preserved (single space after normalization).

### tspan with absolute x/y

```xml
<text x="10" y="20">A<tspan x="100" y="50">B</tspan>C</text>
```

- Run 1: "A" at (10, 20)
- Run 2: "B" at (100, 50) — absolute, cursor jumps
- Run 3: "C" at (100 + width("B"), 50) — cursor continues from B's end

### tspan with dx/dy

```xml
<text x="10" y="20">A<tspan dx="5" dy="-3">B</tspan>C</text>
```

- Run 1: "A" at (10, 20)
- Run 2: "B" at (10 + width("A") + 5, 20 - 3) — shifted from cursor
- Run 3: "C" at (10 + width("A") + 5 + width("B"), 20 - 3) — continues from B's end

### Font resolution edge cases

- Empty `font-family` (inherited from parent or absent) → uses parent's resolved font; if root, defaults to Helvetica
- Comma-separated list `font-family="DejaVu Sans, Arial, sans-serif"` → resolver/heuristic uses ONLY the first name (`"DejaVu Sans"`); SVG fallback chain is not implemented
- `font-weight: bolder`/`lighter` → treated as `bold`/`normal` (no relative computation)
- `font-style: oblique` → maps to italic (PDF doesn't distinguish)

### Gradient fill on text

Phase 3a's `svgPaint` cascade and `/Pattern cs` emission already handle gradient fills on shapes. For text, the same logic applies inside the `BT/ET` block. The renderer just emits the pattern setter before `Tj`, same as shapes do before `f`.

### Empty / degenerate cases

- `<text></text>` (no content) → no runs → no-op
- `<text>` with only whitespace → normalized to single space → one run with " "
- `<tspan>` with no content → no-op
- Cursor at `(0, 0)` if `<text>` has no x/y

### Aspose .NET parity

Aspose.PDF for .NET has `TextFragment`/`TextSegment` for programmatic text construction. Our `<text>` rendering does NOT map to these — it lives entirely in the SVG pipeline. Users wanting `TextFragment`-style API use existing `(*Page).AddText`.

---

## Testing strategy

### Unit tests

- `svg_text_test.go` — heuristic font matcher (mapping table coverage, bold/italic combos, edge cases); whitespace normalization
- `svg_parse_text_test.go` — parsing single-line `<text>`; nested `<tspan>`; mixed content (CharData + tspan); dx/dy; absolute x/y on tspan; inherited styling from `<g>`
- `svg_render_text_test.go` — emit `BT/ET` block with right Tf and Tm; text-anchor middle/end produce correct x offset; gradient fill on text emits /Pattern cs

### Integration tests

- `svg_test.go` — end-to-end: SVG with text renders, output contains `BT...ET` block per run
- Custom resolver: load DejaVuSans, register resolver, render SVG with Cyrillic `<text>` — verify font is the TTF
- Aspose .NET parity (where applicable): comparing rendered output of equivalent SVG to .NET's PDF output
- AES-128 / AES-256 encryption round-trip with SVG containing text

### Test fixtures

- `testdata/svg/text_basic.svg` — `<text x="10" y="20" font-family="Arial">Hello world</text>`
- `testdata/svg/text_anchors.svg` — three labels with text-anchor=start/middle/end at same x
- `testdata/svg/text_tspan.svg` — `<text>Hello <tspan font-weight="bold">world</tspan>!</text>`
- `testdata/svg/text_tspan_nested.svg` — nested `<tspan>` with own x/y and dx/dy
- `testdata/svg/text_cyrillic.svg` — `<text font-family="DejaVu Sans">Привет мир</text>`
- `testdata/svg/text_mixed_styles.svg` — different families/sizes/weights in one `<text>`
