# ImageInfo — Lazy Image Metadata and Selective Extraction

## Goal

Add a lightweight `ImageInfo` type that collects image metadata (dimensions, color space, position, format) without decoding pixel data. Each `ImageInfo` holds internal references enabling on-demand extraction via `Extract() (*Image, error)`.

## Public API

### New type

```go
type ImageInfo struct {
    Width, Height int             // pixel dimensions
    BPC           int             // bits per component
    ColorSpace    ImageColorSpace // original PDF color space
    Format        ImageFormat     // output format (PNG or JPEG)
    X, Y          float64         // position on page (lower-left, points)
    PageWidth     float64         // display width on page (points)
    PageHeight    float64         // display height on page (points)
    Inline        bool            // true if from inline image (BI/ID/EI)
    Name          string          // XObject name (e.g. "/Im0"); empty for inline
}

func (info *ImageInfo) Extract() (*Image, error)
```

### New methods

```go
func (p *Page) ImageInfos() ([]ImageInfo, error)
func (d *Document) ImageInfos() ([][]ImageInfo, error)
```

### Existing methods — preserved, refactored internally

`ExtractImages()` on both `Page` and `Document` is rewritten as `ImageInfos()` + `Extract()` on each item. No change to public signature or behavior.

## Internal Design

### Private fields on ImageInfo

```go
// XObject images
objects   map[int]*pdfObject  // document object store
stream    *pdfStream          // image stream reference
formVal   pdfValue            // for getRawStreamData (JPEG passthrough)

// Inline images
dict      pdfDict             // normalized inline dict
rawData   []byte              // raw image data bytes

ctm       [6]float64          // CTM at point of discovery
```

### collectImageInfos

New internal function with the same walker logic as `extractImagesFromOps` (cm/q/Q/Do/BI), but instead of decoding pixels it:

1. Resolves the XObject stream / inline dict
2. Reads metadata from the stream dict (Width, Height, BPC, ColorSpace, Filter)
3. Determines output Format from filter (DCTDecode → JPEG, everything else → PNG; DCTDecode+SMask → PNG)
4. Stores internal references (stream, objects, formVal, dict, rawData, ctm)
5. Returns `[]ImageInfo`

Form XObject recursion follows the same pattern as existing `extractFormXObjectImages`.

### Extract()

Performs the actual decode using the stored references. For XObject images, reuses the logic currently in `extractXObjectImage`. For inline images, reuses the logic from `extractInlineImage`. Returns `*Image` (pointer, since it allocates Data).

### ExtractImages refactored

```go
func (p *Page) ExtractImages() ([]Image, error) {
    infos, err := p.ImageInfos()
    if err != nil {
        return nil, err
    }
    var images []Image
    for i := range infos {
        img, err := infos[i].Extract()
        if err != nil {
            continue // skip undecodable images, same as current behavior
        }
        images = append(images, *img)
    }
    return images, nil
}
```

## Files changed

- `image.go` — add `ImageInfo` type, `collectImageInfos`, `Extract()`, `ImageInfos()` methods; refactor `ExtractImages` to delegate; remove `extractImagesFromOps`

## Testing

- New unit test: `TestImageInfoMetadata` — verify ImageInfo fields match for a synthetic XObject (same pattern as existing `TestExtractXObjectImageJPEGPassthrough`)
- New unit test: `TestImageInfoExtract` — verify `Extract()` returns valid `*Image` with Data
- New integration test: `TestImageInfos` — run on test PDFs, verify counts match `ExtractImages`, verify metadata fields are populated
- Existing `TestExtractImages` must continue to pass unchanged
