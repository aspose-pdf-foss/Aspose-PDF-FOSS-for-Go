# AddText — Design Spec (Sub-project A: Standard 14 Fonts)

## Goal

Add the ability to draw text on PDF pages inside a `Rectangle` with configurable style, alignment, word wrap, and clipping. This sub-project covers standard 14 PDF fonts only (Latin-1). Unicode support via TTF embedding is a separate future sub-project.

## Public API

### New types

```go
// Color represents an RGBA color with values in [0, 1].
type Color struct {
    R, G, B, A float64
}

// HAlign specifies horizontal text alignment within a rectangle.
type HAlign int

const (
    HAlignLeft   HAlign = iota // default
    HAlignCenter
    HAlignRight
)

// VAlign specifies vertical text alignment within a rectangle.
type VAlign int

const (
    VAlignTop    VAlign = iota // default
    VAlignMiddle
    VAlignBottom
)

// Font identifies one of the standard 14 PDF fonts.
type Font int

const (
    FontHelvetica            Font = iota
    FontHelveticaBold
    FontHelveticaOblique
    FontHelveticaBoldOblique
    FontTimesRoman
    FontTimesBold
    FontTimesItalic
    FontTimesBoldItalic
    FontCourier
    FontCourierBold
    FontCourierOblique
    FontCourierBoldOblique
    FontSymbol
    FontZapfDingbats
)

// TextStyle defines reusable text formatting properties.
type TextStyle struct {
    Font          Font
    Size          float64 // in points; 0 treated as 12
    Color         *Color  // nil → black opaque {0,0,0,1}
    Background    *Color  // nil → no background; A=0 → no background
    HAlign        HAlign  // default: HAlignLeft
    VAlign        VAlign  // default: VAlignTop
    LineSpacing   float64 // multiplier of font size; 0 treated as 1.2
    Underline     bool
    Strikethrough bool
}
```

### New method

```go
// AddText draws text inside the rectangle using the given style.
// Text is wrapped at word boundaries to fit the rectangle width.
// Content exceeding the rectangle height is clipped.
func (p *Page) AddText(text string, style TextStyle, rect Rectangle) error
```

### Renamed type

`TextColor` → `Color` across the entire codebase. The `TextFragment.Color` field type changes from `TextColor` to `Color`.

### Usage

```go
doc, _ := pdf.Open("input.pdf")
page, _ := doc.Page(1)

style := pdf.TextStyle{
    Font:        pdf.FontHelveticaBold,
    Size:        14,
    Color:       &pdf.Color{R: 0, G: 0, B: 0, A: 1},
    Background:  &pdf.Color{R: 1, G: 1, B: 0, A: 0.5},
    HAlign:      pdf.HAlignCenter,
    VAlign:      pdf.VAlignTop,
    LineSpacing: 1.5,
    Underline:   true,
}

page.AddText("Hello, World!", style, pdf.Rectangle{
    LLX: 50, LLY: 600, URX: 300, URY: 700,
})

// Reuse style for another text block
page.AddText("Second paragraph", style, pdf.Rectangle{
    LLX: 50, LLY: 500, URX: 300, URY: 600,
})

doc.Save("output.pdf")
```

## Internal design

### Font mapping

`fontPDFName(f Font) string` maps `Font` constants to PDF base font names: `FontHelvetica` → `"/Helvetica"`, `FontTimesBold` → `"/Times-Bold"`, etc. Glyph widths come from the existing `standard14Widths(name)` in `font_metrics.go`, indexed by WinAnsiEncoding byte codes.

### Word wrap

`wrapText(text string, widths [256]float64, fontSize, maxWidth float64) []string`

- Splits text into lines that fit within `maxWidth` points.
- Breaks at space boundaries; a word longer than `maxWidth` is broken by character.
- Width of each character: `widths[code] / 1000.0 * fontSize`.
- Newlines (`\n`) in input force a line break.

### Position calculation

Line height = `fontSize * lineSpacing`.

Total text height = number of wrapped lines × line height.

**Vertical alignment** determines the Y coordinate of the first line's baseline:
- `VAlignTop`: baseline of first line = `rect.URY - ascent` (ascent from font metrics, scaled by fontSize)
- `VAlignMiddle`: center the text block vertically within the rectangle
- `VAlignBottom`: baseline of last line sits at `rect.LLY + descent_offset`

**Horizontal alignment** determines the X coordinate of each line:
- `HAlignLeft`: `x = rect.LLX`
- `HAlignCenter`: `x = rect.LLX + (rectWidth - lineWidth) / 2`
- `HAlignRight`: `x = rect.LLX + (rectWidth - lineWidth)`

### Content stream generation

Operators are appended to the page content stream (same pattern as `AddImage` via `appendToContentStream`):

```
q                              % save graphics state
<rect.LLX> <rect.LLY> <w> <h> re W n   % clipping path

% Background (if Background != nil and A > 0):
/GS1 gs                       % set opacity (if A < 1)
<R> <G> <B> rg                 % fill color
<rect.LLX> <rect.LLY> <w> <h> re f     % fill rectangle

% Text:
/GS2 gs                       % set text opacity (if A < 1)
BT
  /F1 <size> Tf               % set font
  <R> <G> <B> rg              % text color
  <x> <y> Td                  % position first line
  (<line1>) Tj                % draw line 1
  0 <-lineHeight> Td          % move to next line
  (<line2>) Tj                % draw line 2
  ...
ET

% Underline (if enabled): thin rectangles under each line
<R> <G> <B> rg
<x> <y_underline> <lineWidth> <thickness> re f

% Strikethrough (if enabled): thin rectangles through each line
<x> <y_strike> <lineWidth> <thickness> re f

Q                              % restore graphics state
```

### Resource registration

**Font object**: For each standard 14 font used, create a Type1 font object if not already present:
```
<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>
```
Register in `d.objects` with `d.nextID`, add to page `/Resources /Font` dict as `/F<n>`.

If the page already has a font resource with the same `/BaseFont`, reuse it instead of creating a duplicate.

**ExtGState object** (only when `Color.A < 1` or `Background.A < 1`): Create graphics state dict:
```
<< /ca <alpha> >>
```
Register in `d.objects`, add to page `/Resources /ExtGState` dict as `/GS<n>`.

### String encoding

Text strings in content streams must be encoded in WinAnsiEncoding for standard 14 fonts. Characters outside the WinAnsi range (codes 0–255) are replaced with `?`. The `Tj` operator string uses PDF literal string syntax with proper escaping of `(`, `)`, and `\`.

### Underline and Strikethrough

Both are drawn as thin filled rectangles after the text block (but within the `q`/`Q` save/restore and clipping path):

- **Underline**: Y position = baseline - `fontSize * 0.1`; thickness = `fontSize * 0.05`
- **Strikethrough**: Y position = baseline + `fontSize * 0.3`; thickness = `fontSize * 0.05`

Rectangle width matches the rendered text width of each line. Color matches the text color (including alpha via ExtGState).

## Error handling

- `rect` invalid (URX <= LLX or URY <= LLY) → error
- `Size < 0` → error; `Size == 0` → default 12
- `Font` out of range (not one of the 14 constants) → error
- `text == ""` → no-op, return nil

## Files

| File | Responsibility |
|------|----------------|
| `color.go` | `Color`, `Font` constants, `HAlign`, `VAlign`, `TextStyle`, `fontPDFName` |
| `text_add.go` | `AddText`, `wrapText`, position calculation, content stream generation, resource registration |
| `text_add_test.go` | Unit tests |
| `text_add_integration_test.go` | Integration test |

Rename `TextColor` → `Color` in: `text_layout.go`, `text.go`.

## Testing

### Unit tests (package `asposepdf`)

- `TestWrapText` — verify word wrapping: single line fits, multi-line wrap, word longer than width, newlines in input
- `TestWrapTextNewlines` — explicit `\n` handling
- `TestAddTextDefaultStyle` — zero-value TextStyle applies defaults (size 12, black, left/top, line spacing 1.2)
- `TestAddTextAlignment` — verify HAlign/VAlign combinations produce correct positioning
- `TestAddTextClipping` — text exceeding rect height is clipped (verify clip path operators in content stream)
- `TestAddTextBackground` — background rectangle drawn when Background is set
- `TestAddTextUnderline` — underline rectangles present in content stream
- `TestAddTextStrikethrough` — strikethrough rectangles present in content stream
- `TestAddTextTransparency` — ExtGState created when alpha < 1
- `TestAddTextInvalidRect` — error on invalid rectangle
- `TestAddTextInvalidFont` — error on out-of-range font constant
- `TestAddTextEmptyString` — no-op, no error
- `TestFontPDFName` — all 14 font constants map to correct PDF names
- `TestTextColorRenamed` — `Color` type used in `TextFragment` (compile-time check)

### Integration test (package `asposepdf_test`)

- `TestAddTextRoundTrip` — create blank document, add text with various styles, save, validate, reopen, extract text and verify content matches

## Scope boundary

This spec covers:
- Adding text with standard 14 PDF fonts (Latin-1 / WinAnsiEncoding)
- Word wrap, alignment, clipping
- Color with alpha, background with alpha
- Underline and strikethrough

This spec does NOT cover:
- TTF/OTF font embedding (separate sub-project B)
- Unicode text beyond WinAnsiEncoding range
- Text search or replacement
- Watermarks (rotation, full-page overlay)
- Paragraph spacing, tab stops, justified alignment
