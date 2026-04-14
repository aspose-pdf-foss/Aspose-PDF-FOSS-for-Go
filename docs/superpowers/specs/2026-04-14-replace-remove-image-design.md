# ReplaceImage & RemoveImage — Design Spec

## Goal

Add the ability to replace and remove images on existing PDF pages. This is Sub-project B of the image manipulation feature set (AddImage, ImageToDocument, ReplaceImage, RemoveImage, OptimizeImages).

## Public API

### New methods on `*ImageInfo`

```go
// Replace replaces the image data with a new image from a file.
// Format is detected by magic bytes (JPEG, PNG). Dimensions may change.
// Position and size on the page remain unchanged.
func (info *ImageInfo) Replace(path string) error

// ReplaceFromStream replaces the image data with a new image from a reader.
func (info *ImageInfo) ReplaceFromStream(r io.Reader) error

// Remove removes the image from the page.
// Deletes the XObject reference from page resources and the drawing
// operators from the content stream. The underlying XObject remains
// in the document's object store (safe for shared references).
func (info *ImageInfo) Remove() error
```

### Changes to `ImageInfo`

Add a private field `page *Page` to `ImageInfo`. This gives `Remove()` access to page resources and content stream. The field is populated during `collectImageInfos`.

### Usage

```go
doc, _ := pdf.Open("input.pdf")
page, _ := doc.Page(1)
infos, _ := page.ImageInfos()

// Replace a specific image
for _, info := range infos {
    if info.Width > 500 {
        info.Replace("new_large_image.jpg")
    }
}

// Remove all images from page
for _, info := range infos {
    info.Remove()
}

doc.Save("output.pdf")
```

## Internal design

### Replace mechanics

1. **Read new image** — read file/reader, determine format by magic bytes via `detectImageFormat`.
2. **Create new XObject** — call existing `createImageXObject(data, format)` to get `*pdfStream` and optional SMask.
3. **Update existing stream in place** — overwrite `stream.Dict` and `stream.Data` from the new XObject. Key dict fields to update: `/Width`, `/Height`, `/BitsPerComponent`, `/ColorSpace`, `/Filter`. Remove `/DecodeParms` if present (old stream may have had it). Set `stream.Decoded` to match the new stream. For JPEG→PNG transition: remove `/Filter` (writer will add FlateDecode for Decoded=true streams). For PNG→JPEG transition: set `/Filter` to `/DCTDecode`, `Decoded` to false.
4. **Handle SMask transitions:**
   - Old has SMask, new has SMask: register new SMask object in `doc.objects`, update `/SMask` ref in dict.
   - Old has SMask, new does not: delete `/SMask` key from dict. Old SMask object stays in `doc.objects` (orphaned — safe).
   - Old has no SMask, new has SMask: register new SMask object, add `/SMask` ref.
   - Old has no SMask, new has no SMask: no action.
5. **Content stream is not modified** — CTM (position/size) remains as-is, `Do` operator references the same name.

Key property: replacing the XObject in place means all pages sharing this XObject automatically get the new image. This is correct behavior — if a logo is used on 10 pages, one replacement updates all.

### Remove mechanics

1. **Remove from page resources** — delete the `/ImN` key from the `/XObject` dict in the page's `/Resources`. Use `resolveRef` for both `/Resources` and `/XObject` to handle indirect references.
2. **Remove operators from content stream:**
   - Parse content stream via `parseContentStream` into `[]contentOp`.
   - Track `q`/`Q` nesting depth and positions.
   - Find `Do` operator with operand matching the image name (`/ImN`).
   - Remove the entire `q ... /ImN Do Q` block (from the matching `q` to its closing `Q`).
   - Edge case: if `Do` appears without `q`/`Q` wrapping, remove just the `cm` and `Do` operators.
3. **Rebuild content stream** — serialize remaining operators via `serializeContentOps`, create new content stream object, replace page's `/Contents`.
4. **XObject in `doc.objects` is not removed** — the object may be shared across pages. Orphaned object cleanup is a separate feature (tracked in beads: `pdf-go-luj`).

### Content stream serialization

```go
// serializeContentOps converts parsed operators back to content stream bytes.
func serializeContentOps(ops []contentOp) []byte
```

Serialization rules per operand type:
- Integers: `%d`
- Floats: `formatFloat` (trim trailing zeros)
- Names: as-is (`/Im0`)
- Strings: `(text)` with escaping or `<hex>`
- Arrays (for TJ): `[ ... ]`
- Each operator: operands separated by spaces, then operator, then `\n`

This is the inverse of `parseContentStream`. Round-trip fidelity is tested.

### ImageInfo changes

The `ImageInfo` struct in `image.go` needs one new private field:

```go
type ImageInfo struct {
    // ... existing fields ...
    page    *Page              // page this image belongs to (for Remove)
}
```

`collectImageInfos` must be updated to accept and store the `*Page` reference. This also means `(*Page).ImageInfos()` passes `p` (the Page pointer) into `collectImageInfos`.

For `(*Document).ImageInfos()`, each page's infos get the corresponding `*Page`.

## Error handling

- **Replace with unsupported format** — magic bytes don't match JPEG or PNG: `"unsupported image format"`
- **Replace with corrupt image** — decode fails: `"failed to decode image: <wrapped>"`
- **Replace/Remove on invalid ImageInfo** — nil stream: `"image info: no image data"`
- **Replace with empty data** — 0 bytes: `"replace image: empty data"`
- **Remove fails to parse content stream** — wrapped error from `parseContentStream`

No panics, no silent failures. All errors returned via `error`.

## Files

| File | Responsibility |
|------|----------------|
| `image_replace.go` | `Replace`, `ReplaceFromStream`, internal replacement logic |
| `image_remove.go` | `Remove`, resource cleanup, content stream filtering, `serializeContentOps` |
| `image.go` | Add `page *Page` field to `ImageInfo`, update `collectImageInfos` |
| `image_replace_test.go` | Unit tests for Replace |
| `image_remove_test.go` | Unit tests for Remove, `serializeContentOps` |
| `image_replace_integration_test.go` | Integration tests (external package) |
| `image_remove_integration_test.go` | Integration tests (external package) |

## Testing

### Unit tests (package `asposepdf`)

- `TestReplaceImageJPEG` — replace JPEG with another JPEG, verify stream dict updated (Width, Height, Filter, Data)
- `TestReplaceImagePNGToJPEG` — replace PNG (with alpha/SMask) with JPEG, verify SMask removed from dict
- `TestReplaceImageJPEGToPNGWithAlpha` — replace JPEG with PNG with alpha, verify SMask added
- `TestRemoveImage` — remove image, verify /XObject dict has entry removed and content stream has no Do for that name
- `TestSerializeContentOps` — round-trip: parse → serialize → parse, verify equivalence
- `TestRemoveImageNestedQ` — nested q/Q blocks, verify only the correct block is removed

### Integration tests (package `asposepdf_test`)

Test data: existing `testdata/` files (`4pages.pdf`, `PdfWithImages.pdf`, `Koala.jpg`, `Penguins.jpg`, `Penguins.png`, `aspose-logo.png`).

- `TestReplaceImageRoundTrip` — open PDF with images, replace an image with a different file, save, reopen, ExtractImages, verify the replacement image dimensions match the new file
- `TestRemoveImageRoundTrip` — open PDF with images, count images, remove one, save, reopen, verify image count decreased by one

## Scope boundary

This spec covers **only** ReplaceImage and RemoveImage. OptimizeImages is a separate sub-project (C) with its own spec/plan cycle. Orphaned object cleanup is tracked separately (`pdf-go-luj`).
