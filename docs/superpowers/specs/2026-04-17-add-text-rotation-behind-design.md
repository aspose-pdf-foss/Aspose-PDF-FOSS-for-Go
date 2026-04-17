# AddText Rotation & Behind — Design Spec

## Goal

Extend `AddText` with two new capabilities: text rotation (arbitrary angle) and behind-content mode (draw text under existing page content).

## Public API

### Changes to `TextStyle`

Two new fields:

```go
type TextStyle struct {
    Font          Font
    Size          float64
    Color         *Color
    Background    *Color
    HAlign        HAlign
    VAlign        VAlign
    LineSpacing   float64
    Underline     bool
    Strikethrough bool
    Rotation      float64 // degrees counter-clockwise; pivot = lower-left corner of rect (LLX, LLY); default 0
    Behind        bool    // if true, text is drawn under existing page content; default false
}
```

### Usage

```go
// Rotated column header
page.AddText("Revenue", pdf.TextStyle{
    Font:     pdf.FontHelveticaBold,
    Size:     10,
    Rotation: 45,
}, pdf.Rectangle{LLX: 100, LLY: 500, URX: 130, URY: 600})

// Watermark behind content
page.AddText("CONFIDENTIAL", pdf.TextStyle{
    Font:     pdf.FontHelvetica,
    Size:     60,
    Color:    &pdf.Color{R: 0.8, G: 0.8, B: 0.8, A: 0.3},
    Rotation: 45,
    HAlign:   pdf.HAlignCenter,
    VAlign:   pdf.VAlignMiddle,
    Behind:   true,
}, fullPageRect)
```

## Internal design

### Rotation

When `Rotation != 0`, the entire text block (clipping, background, text, decorations) is drawn within a rotated coordinate system. This is achieved by applying a rotation transformation matrix (`cm`) immediately after `q`:

```
q
  1 0 0 1 <LLX> <LLY> cm        % translate origin to pivot point
  cos(θ) sin(θ) -sin(θ) cos(θ) 0 0 cm  % rotate around new origin
  -<LLX> -<LLY> 0 0 cm          % (not needed — clip rect coords adjusted instead)
```

Simpler approach: translate to pivot, rotate, then draw everything with coordinates relative to the pivot (subtract LLX/LLY from all positions). The clipping rectangle, background fill, text positions, and underline/strikethrough rectangles are all expressed relative to (0, 0) instead of (LLX, LLY).

Specifically:
1. `q` — save graphics state
2. `1 0 0 1 LLX LLY cm` — translate origin to lower-left corner of rect
3. `cos sin -sin cos 0 0 cm` — rotate (θ in radians, counter-clockwise)
4. Emit clipping, background, text, decorations with coordinates offset by (-LLX, -LLY)
5. `Q` — restore graphics state

When `Rotation == 0`, the existing code path is used unchanged (no `cm` operators, absolute coordinates).

The `Td` operators for text positioning work the same way — they just use transformed coordinates.

### Behind

When `Behind == true`, the content stream operators are inserted _before_ the existing page content instead of after. This is implemented via a new `prependToContentStream` method on `*Page`, which:

1. Reads existing content stream bytes
2. Prepends the new operators
3. Creates a new stream object and updates `/Contents`

This follows the same pattern as `appendToContentStream` but concatenates in reverse order.

### Interaction between Rotation and Behind

Both features are orthogonal. `Behind` controls _where_ in the content stream the operators go. `Rotation` controls the coordinate transformation _within_ those operators. A rotated watermark behind content uses both.

## Error handling

- `Rotation`: any float64 value is valid. No validation needed — `math.Cos`/`math.Sin` handle all values.
- `Behind`: boolean, no validation.
- All existing error conditions remain unchanged.

## Files

| File | Change |
|------|--------|
| `color.go` | Add `Rotation float64` and `Behind bool` to `TextStyle` |
| `text_add.go` | Add rotation `cm` operators when Rotation != 0; add `prependToContentStream`; use it when Behind is true |
| `text_add_test.go` | Unit tests for rotation and behind |
| `text_add_integration_test.go` | Integration test with rotated + behind text |

## Testing

### Unit tests

- `TestAddTextRotation` — verify `cm` operators appear in content stream when Rotation != 0
- `TestAddTextRotationZero` — verify no `cm` operators when Rotation == 0 (no regression)
- `TestAddTextBehind` — verify operators are prepended (appear before existing content)
- `TestAddTextBehindAndRotation` — both features combined

### Integration test

- `TestAddTextRotationRoundTrip` — create document, add normal text, add rotated text behind, save, validate, reopen

## Scope boundary

This spec covers:
- `Rotation` field in `TextStyle` (arbitrary angle, pivot at LLX/LLY)
- `Behind` field in `TextStyle` (prepend to content stream)
- `prependToContentStream` internal method

This spec does NOT cover:
- `AddTextWatermark` convenience method (separate spec)
- Custom pivot point (always LLX/LLY)
- Per-line rotation (all lines rotate together)
