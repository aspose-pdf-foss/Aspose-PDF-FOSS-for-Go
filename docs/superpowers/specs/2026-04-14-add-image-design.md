# AddImage & ImageToDocument — Design Spec

## Goal

Add the ability to place images onto existing PDF pages and convert standalone images into single-page PDF documents. This is Sub-project A of the image manipulation feature set (AddImage, ImageToDocument, ReplaceImage, RemoveImage, OptimizeImages).

## Public API

### New types

```go
// Rectangle represents a PDF rectangle [llx, lly, urx, ury] in points.
type Rectangle struct {
    LLX, LLY float64 // lower-left corner
    URX, URY float64 // upper-right corner
}

// ImageToDocumentOptions controls page sizing for ImageToDocument.
type ImageToDocumentOptions struct {
    PageWidth    float64 // explicit page size (points); 0 = auto from image
    PageHeight   float64
    MarginLeft   float64 // margins (points); default 0
    MarginRight  float64
    MarginTop    float64
    MarginBottom float64
}
```

### New methods and functions

```go
// AddImage adds an image from a file to the page within the given rectangle.
// Format is detected by magic bytes (JPEG, PNG). Image is drawn on top of existing content.
func (p *Page) AddImage(path string, rect Rectangle) error

// AddImageFromStream adds an image from a reader to the page within the given rectangle.
// Format is detected by magic bytes (JPEG, PNG).
func (p *Page) AddImageFromStream(r io.Reader, rect Rectangle) error

// ImageToDocument creates a new Document with a single page containing the image.
// Page size is determined by image dimensions and DPI metadata (default 72 DPI).
func ImageToDocument(path string, opts ...ImageToDocumentOptions) (*Document, error)

// ImageToDocumentFromStream creates a new Document from an image reader.
// Format is detected by magic bytes.
func ImageToDocumentFromStream(r io.Reader, opts ...ImageToDocumentOptions) (*Document, error)
```

## Supported formats

- **JPEG** — detected by magic bytes `FF D8`. Stored as DCTDecode passthrough (no re-encoding).
- **PNG** — detected by magic bytes `89 50 4E 47`. Decoded to pixels, stored as FlateDecode. If the PNG has an alpha channel, a separate SMask XObject is created.

Unsupported formats return an error.

## Internal design

### Format detection

Read the first 8 bytes from the file or stream. Match against known magic bytes:
- `FF D8` → JPEG
- `89 50 4E 47` → PNG

For `io.Reader`, read into a buffer and use `io.MultiReader` to prepend the consumed bytes back before passing to the decoder.

### AddImage mechanics

1. **Read and detect format** — read file/stream, determine format by magic bytes.
2. **Prepare image XObject:**
   - JPEG: data stored as-is, `/Filter /DCTDecode`. Width, height, color space read from JPEG header (SOF marker).
   - PNG: decode via `image/png` to pixels, encode with FlateDecode. If RGBA, create a separate SMask XObject for the alpha channel.
3. **Register in document:**
   - Create new `pdfObject` with `pdfStream` (image XObject dict + data).
   - Assign ID in `doc.objects`.
   - Add `/ImN` reference in page's `/Resources` → `/XObject` (N = next available number).
4. **Write to content stream:**
   - Compute CTM from `Rectangle`: `a = URX - LLX` (width), `d = URY - LLY` (height), `e = LLX`, `f = LLY`.
   - Append to end of content stream: `q <a> 0 0 <d> <e> <f> cm /ImN Do Q`.

Image is always drawn on top of existing content (operators appended at end).

### ImageToDocument mechanics

1. **Read image** — detect format, decode header for pixel dimensions.
2. **Determine DPI:**
   - JPEG: read JFIF APP0 marker (`FF E0`), density fields. If absent or units not inches → 72 DPI.
   - PNG: read pHYs chunk (pixels per unit, unit specifier). If absent → 72 DPI.
3. **Calculate page size:**
   - No options: `pageWidth = pixelWidth / DPI * 72`, `pageHeight = pixelHeight / DPI * 72`.
   - With margins (no explicit PageWidth/PageHeight): `pageWidth = imageWidth + MarginLeft + MarginRight`.
   - With explicit PageWidth/PageHeight: image is fit into `(PageWidth - margins) x (PageHeight - margins)` preserving aspect ratio, centered in the available area.
4. **Build document:**
   - Create new `Document` with one page of computed size.
   - Compute `Rectangle` for image placement on the page.
   - Use AddImage internal logic to add the image.

### DPI parsing

**JPEG (JFIF):** After SOI (`FF D8`), look for APP0 marker (`FF E0`). If found and identifier is "JFIF\0":
- Byte 7: units (0 = no units/aspect ratio, 1 = dots per inch, 2 = dots per cm)
- Bytes 8-9: X density (big-endian uint16)
- Bytes 10-11: Y density (big-endian uint16)
- If units == 1, use as DPI. If units == 2, convert: DPI = density * 2.54. Otherwise fallback to 72.

**PNG (pHYs):** Scan chunks for `pHYs` (4-byte type). If found:
- Bytes 0-3: pixels per unit X (big-endian uint32)
- Bytes 4-7: pixels per unit Y (big-endian uint32)
- Byte 8: unit specifier (0 = unknown, 1 = meter)
- If unit == 1, convert: DPI = ppu / 39.3701. Otherwise fallback to 72.

## Error handling

- **Unsupported format** — magic bytes don't match JPEG or PNG: `"unsupported image format"`
- **Corrupt image** — header/pixel decode fails: `"failed to decode image: <wrapped error>"`
- **Invalid Rectangle** — LLX >= URX or LLY >= URY: `"invalid rectangle: width and height must be positive"`
- **Empty reader** — 0 bytes read: error returned
- **Impossible layout** — margins exceed page size in ImageToDocument: `"margins exceed page dimensions"`

No panics, no silent failures. All errors returned via `error`.

## Files

| File | Responsibility |
|------|----------------|
| `rectangle.go` | `Rectangle` type |
| `image_add.go` | `AddImage`, `AddImageFromStream`, internal logic (XObject creation, content stream writing, format detection) |
| `image_convert.go` | `ImageToDocument`, `ImageToDocumentFromStream`, `ImageToDocumentOptions`, DPI parsing |
| `image_add_test.go` | Unit tests for AddImage (internal) |
| `image_convert_test.go` | Unit tests for ImageToDocument (internal) |
| `image_add_integration_test.go` | Integration tests (external package) |
| `image_convert_integration_test.go` | Integration tests (external package) |

## Testing

### Unit tests (package `asposepdf`)

- `TestDetectImageFormat` — magic bytes: JPEG, PNG, unknown format returns error
- `TestCreateImageXObjectJPEG` — JPEG data produces XObject with DCTDecode filter
- `TestCreateImageXObjectPNG` — PNG pixels produce XObject with FlateDecode filter
- `TestBuildImageCTM` — Rectangle to CTM matrix conversion
- `TestParseJPEGDPI` — reads DPI from JFIF marker, fallback to 72
- `TestParsePNGDPI` — reads DPI from pHYs chunk, fallback to 72
- `TestInvalidRectangle` — error when LLX >= URX or LLY >= URY

### Integration tests (package `asposepdf_test`)

Test data: `testdata/Koala.jpg`, `testdata/Penguins.jpg`, `testdata/Penguins.png`, `testdata/aspose-logo.png`.

- `TestAddImage` — add JPEG (`Penguins.jpg`) and PNG (`aspose-logo.png`) to existing PDF page, save, reopen, verify ExtractImages finds the added images with correct dimensions
- `TestImageToDocument` — convert `Koala.jpg` and `Penguins.png` to PDF, verify page size matches image dimensions (DPI-aware), extract image back and verify dimensions
- `TestImageToDocumentWithOptions` — convert `aspose-logo.png` with A4 page size and margins, verify page dimensions, verify image is present

## Scope boundary

This spec covers **only** AddImage and ImageToDocument. ReplaceImage, RemoveImage, and OptimizeImages are separate sub-projects with their own spec/plan cycles.
