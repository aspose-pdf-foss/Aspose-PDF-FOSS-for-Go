# NewDocument — Design Spec

## Goal

Add the ability to create a single-page blank PDF document with given dimensions or a predefined page format. Supports portrait and landscape orientation via a `Landscape()` method on `PageFormat`.

## Public API

### New types and functions

```go
// PageFormat describes a page size in points (1/72 inch).
type PageFormat struct {
    Width  float64
    Height float64
}

// Predefined page formats (portrait orientation).
var (
    PageFormatA3     = PageFormat{842, 1191}
    PageFormatA4     = PageFormat{595, 842}
    PageFormatLetter = PageFormat{612, 792}
    PageFormatLegal  = PageFormat{612, 1008}
)

// Landscape returns the format with width and height swapped.
func (f PageFormat) Landscape() PageFormat

// NewDocument creates a single-page blank document with the given dimensions in points.
func NewDocument(width, height float64) *Document

// NewDocumentFromFormat creates a single-page blank document using a predefined page format.
func NewDocumentFromFormat(format PageFormat) *Document
```

### Usage

```go
// From explicit dimensions
doc := pdf.NewDocument(595, 842)
doc.Save("blank.pdf")

// From predefined format
doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
doc.Save("a4.pdf")

// Landscape orientation
doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4.Landscape())
doc.Save("a4_landscape.pdf")
```

## Internal design

### Document construction

`NewDocument(width, height)` builds a minimal `*Document` in memory:

1. Create an empty content stream — `*pdfStream` with empty `Data`, `Decoded=true`, empty `Dict`.
2. Create a page dict — `pdfDict` with `/Type /Page`, `/MediaBox [0 0 width height]`, empty `/Resources` dict, `/Contents` pointing to the content stream object.
3. Register both as `*pdfObject` in `doc.objects`.
4. Add the page object to `doc.pages`.
5. Return the `*Document`.

This follows the same pattern as `ImageToDocument` but without image data in the content stream.

### Landscape

`Landscape()` is a value method that returns a new `PageFormat` with `Width` and `Height` swapped. No enum, no mode flag — the caller simply uses the swapped dimensions.

### PageFormat as exported var

The predefined formats are `var` (not `const`) because Go does not support const structs. They are package-level variables. Users can also create custom `PageFormat` values directly.

## Error handling

No errors. Both `NewDocument` and `NewDocumentFromFormat` always succeed — they construct in-memory objects with no I/O. Zero or negative dimensions are the caller's responsibility (the resulting PDF would have an empty MediaBox, but that is valid PDF).

## Files

| File | Responsibility |
|------|----------------|
| `document_new.go` | `PageFormat`, predefined formats, `Landscape`, `NewDocument`, `NewDocumentFromFormat` |
| `document_new_test.go` | Unit tests (package `asposepdf`) |
| `document_new_integration_test.go` | Integration test (package `asposepdf_test`) |

## Testing

### Unit tests (package `asposepdf`)

- `TestNewDocument` — create with explicit dimensions (595, 842), verify `PageCount()=1`, page `Size()` matches.
- `TestNewDocumentFromFormat` — create from `PageFormatA4`, verify dimensions are 595×842.
- `TestNewDocumentLandscape` — create from `PageFormatA4.Landscape()`, verify dimensions are 842×595.
- `TestPageFormatLandscapeSwaps` — verify `PageFormat{595, 842}.Landscape()` returns `{842, 595}`.

### Integration test (package `asposepdf_test`)

- `TestNewDocumentRoundTrip` — create from A4, save, reopen, Validate, verify `PageCount()=1` and dimensions match.

## Scope boundary

This spec covers only creating a blank single-page document. It does NOT cover:
- Multi-page blank documents (can be built by creating one and using `Append`)
- Adding content to the blank page (existing `AddImage`, text APIs handle that)
- Custom page boxes (CropBox, TrimBox, etc.) — only MediaBox is set
