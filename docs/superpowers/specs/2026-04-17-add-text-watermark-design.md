# AddTextWatermark — Design Spec

## Goal

Add a convenience method on `*Document` that applies a text watermark to all or selected pages, using the existing `AddText` infrastructure.

## Public API

```go
func (d *Document) AddTextWatermark(text string, style TextStyle, pageNums ...int) error
```

- `text` — watermark text (empty string returns nil, no-op)
- `style` — standard `TextStyle` with full user control (Rotation, Behind, Color, etc.)
- `pageNums` — 1-based page numbers; empty means all pages
- Returns error if any `pageNums` value is < 1 or > `PageCount()`

### Usage

```go
// Watermark on all pages
doc.AddTextWatermark("CONFIDENTIAL", pdf.TextStyle{
    Font:     pdf.FontHelveticaBold,
    Size:     60,
    Color:    &pdf.Color{R: 0.8, G: 0.8, B: 0.8, A: 0.3},
    Rotation: 45,
    HAlign:   pdf.HAlignCenter,
    VAlign:   pdf.VAlignMiddle,
    Behind:   true,
})

// Watermark on pages 1 and 3 only
doc.AddTextWatermark("DRAFT", pdf.TextStyle{
    Font:     pdf.FontHelvetica,
    Size:     48,
    Color:    &pdf.Color{R: 1, G: 0, B: 0, A: 0.2},
    Rotation: -45,
    HAlign:   pdf.HAlignCenter,
    VAlign:   pdf.VAlignMiddle,
    Behind:   true,
}, 1, 3)
```

## Internal design

For each target page:
1. Get page dimensions via `page.Size()` (MediaBox)
2. Build full-page rectangle: `Rectangle{LLX: 0, LLY: 0, URX: width, URY: height}`
3. Call `page.AddText(text, style, rect)`

No default overrides — the user's `TextStyle` is passed through as-is. This keeps behavior predictable and avoids hidden magic.

### Page number handling

Follows the `Rotate(angle, pageNums...)` pattern:
- Empty `pageNums` → all pages (1 to PageCount)
- Duplicate page numbers are allowed (watermark applied multiple times — user's responsibility)
- Invalid page number (< 1 or > PageCount) → immediate error before any modifications

Validation happens upfront: all page numbers are checked before applying the watermark to any page. This prevents partial application.

## Error handling

- `text == ""` → return nil (no-op, consistent with `AddText`)
- Invalid page number → `fmt.Errorf("add text watermark: invalid page number %d, document has %d pages", n, pageCount)`
- All other errors (invalid font, negative size, etc.) are delegated to `AddText`

## Files

| File | Change |
|------|--------|
| `text_add.go` | Add `AddTextWatermark` method on `*Document` |
| `text_add_test.go` | Unit tests |
| `text_add_integration_test.go` | Integration test |
| `CLAUDE.md` | Add `AddTextWatermark` to API docs |

## Testing

### Unit tests

- `TestAddTextWatermarkAllPages` — 3-page doc, no pageNums, verify all pages have watermark text in content stream
- `TestAddTextWatermarkSelectedPages` — 3-page doc, pageNums=[1,3], verify only pages 1 and 3 have watermark
- `TestAddTextWatermarkInvalidPage` — verify error for page 0, page > PageCount
- `TestAddTextWatermarkEmpty` — empty string returns nil, no content added

### Integration test

- `TestAddTextWatermarkRoundTrip` — create multi-page doc, add watermark to all pages, save, validate, reopen, extract text from each page

## Scope boundary

This spec covers:
- `AddTextWatermark` convenience method on `*Document`
- Upfront page number validation
- Full-page rectangle calculation from MediaBox

This spec does NOT cover:
- Image watermarks (separate feature)
- Custom rectangle per page (user can call `page.AddText` directly)
- Default style overrides (YAGNI)
