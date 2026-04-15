# AddBlankPage / InsertBlankPage — Design Spec

## Goal

Add the ability to append or insert blank pages into an existing document with given dimensions or a predefined page format.

## Public API

### New methods on `*Document`

```go
// AddBlankPage appends a blank page to the end of the document.
func (d *Document) AddBlankPage(width, height float64) error

// AddBlankPageFromFormat appends a blank page using a predefined page format.
func (d *Document) AddBlankPageFromFormat(format PageFormat) error

// InsertBlankPage inserts a blank page at the given 1-based position.
// Existing pages at and after that position shift by one.
func (d *Document) InsertBlankPage(position int, width, height float64) error

// InsertBlankPageFromFormat inserts a blank page at the given position using a predefined page format.
func (d *Document) InsertBlankPageFromFormat(position int, format PageFormat) error
```

### Usage

```go
doc, _ := pdf.Open("input.pdf")

// Append to end
doc.AddBlankPage(595, 842)

// Append using format
doc.AddBlankPageFromFormat(pdf.PageFormatA4)

// Insert at position 2 (becomes the new page 2, existing pages shift)
doc.InsertBlankPage(2, 595, 842)

// Insert landscape page at position 1
doc.InsertBlankPageFromFormat(1, pdf.PageFormatLetter.Landscape())

doc.Save("output.pdf")
```

## Internal design

### Page construction

Both `AddBlankPage` and `InsertBlankPage` create a minimal page object set — the same pattern used by `NewDocument`:

1. Create an empty content stream — `*pdfStream` with empty `Data`, `Decoded=true`, empty `Dict`.
2. Create a page dict — `pdfDict` with `/Type /Page`, `/MediaBox [0 0 width height]`, empty `/Resources` dict, `/Contents` pointing to the content stream object.
3. Register both as `*pdfObject` in `d.objects` using `d.nextID`.

### Page placement

- `AddBlankPage`: appends the page object to `d.pages`.
- `InsertBlankPage`: inserts the page object into `d.pages` at index `position-1` using Go slice insert idiom.

### Delegation

- `AddBlankPageFromFormat` delegates to `AddBlankPage` with `format.Width`, `format.Height`.
- `InsertBlankPageFromFormat` delegates to `InsertBlankPage` with `format.Width`, `format.Height`.

## Error handling

- `AddBlankPage` / `AddBlankPageFromFormat`: always succeed (no error possible in practice, but signature includes `error` for consistency with other Document methods).
- `InsertBlankPage` / `InsertBlankPageFromFormat`: return error if `position < 1` or `position > PageCount()+1`. Position `PageCount()+1` is valid — equivalent to appending.

## Files

| File | Responsibility |
|------|----------------|
| `document_new.go` | Add 4 methods (file already contains `NewDocument`, `PageFormat`) |
| `document_new_test.go` | Unit tests (extend existing file) |
| `document_new_integration_test.go` | Integration test (extend existing file) |

## Testing

### Unit tests (package `asposepdf`)

- `TestAddBlankPage` — open a document, add a blank page, verify `PageCount` increased by 1 and last page has correct dimensions.
- `TestInsertBlankPage` — insert at position 1, verify it became the first page and original pages shifted.
- `TestInsertBlankPageEnd` — insert at position `PageCount()+1`, verify it's equivalent to appending.
- `TestInsertBlankPageInvalidPosition` — position 0 and `PageCount()+2` return errors.

### Integration test (package `asposepdf_test`)

- `TestAddBlankPageRoundTrip` — open real PDF, add a blank page, save, reopen, Validate, verify page count and new page dimensions.

## Scope boundary

This spec covers only adding blank pages. It does NOT cover:
- Adding pages with content (use existing `AddImage` etc. after adding blank page)
- Removing pages
- Moving/reordering pages (already exists: `Reorder`)
