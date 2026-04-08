# Phase 3: Image Extraction — Design Spec

**Date:** 2026-04-08
**Branch:** phase3-image-extraction
**Scope:** Extract images from existing PDF documents

## Public API

### Types

```go
type ImageFormat int

const (
    ImageFormatPNG  ImageFormat = iota
    ImageFormatJPEG
)

type ImageColorSpace int

const (
    ColorSpaceDeviceRGB  ImageColorSpace = iota
    ColorSpaceDeviceGray
    ColorSpaceDeviceCMYK
    ColorSpaceIndexed
    ColorSpaceICCBased
)

type Image struct {
    Data       []byte          // encoded image bytes (PNG or JPEG)
    Format     ImageFormat     // output format
    Width      int             // pixel width
    Height     int             // pixel height
    BPC        int             // bits per component (original)
    ColorSpace ImageColorSpace // original PDF color space
    X, Y       float64         // position on page (lower-left, in points)
    PageWidth  float64         // display width on page (in points)
    PageHeight float64         // display height on page (in points)
    Inline     bool            // true if from inline image (BI/ID/EI)
}
```

### Methods

```go
func (img *Image) Save(path string) error
func (img *Image) WriteTo(w io.Writer) (int64, error)

func (*Page) ExtractImages() ([]Image, error)
func (*Document) ExtractImages() ([][]Image, error)  // one slice per page
```

## Output format strategy

- **DCTDecode** → JPEG passthrough (raw stream bytes, no re-encoding)
- **FlateDecode / no filter / other** → decode to raw pixels, encode as PNG
- Original color space and BPC are always reported in the `Image` struct regardless of output format

## Color space handling

| PDF Color Space | Components | Handling |
|---|---|---|
| DeviceRGB | 3 | Direct to PNG RGB |
| DeviceGray | 1 | PNG grayscale |
| DeviceCMYK | 4 | Convert to RGB: `R=(1-C)*(1-K)`, `G=(1-M)*(1-K)`, `B=(1-Y)*(1-K)` |
| Indexed | 1 (palette) | Expand palette to base color space, then process |
| ICCBased | N (1/3/4) | Read `/N` from ICC stream dict; treat as DeviceGray/RGB/CMYK accordingly |

CalGray, CalRGB, Lab — out of scope for Phase 3 (very rare). Emit as-is with best-effort mapping.

## Soft mask (alpha channel)

- `/SMask` on an image XObject points to another image XObject (DeviceGray, same dimensions)
- When present: decode soft mask, use as alpha channel in output PNG (RGBA/GrayA)
- DCTDecode images with `/SMask`: decode JPEG to pixels, apply alpha, re-encode as PNG (JPEG doesn't support alpha)
- `/Mask` (stencil mask, color key): out of scope for Phase 3

## Inline images (BI/ID/EI)

- Small images embedded directly in content stream
- Content stream parser already recognizes `BI` keyword and skips to `EI`
- Phase 3: parse inline image dict (key/value pairs between BI and ID) and image data (between ID and EI)
- Apply same color space / filter pipeline as XObject images
- Mark with `Inline: true` in output

### Inline image abbreviations (PDF spec Table 4.43/4.44)

| Abbrev | Full key |
|---|---|
| BPC | BitsPerComponent |
| CS | ColorSpace |
| D | Decode |
| DP | DecodeParms |
| F | Filter |
| H | Height |
| IM | ImageMask |
| W | Width |

| Abbrev | Full value |
|---|---|
| G | DeviceGray |
| RGB | DeviceRGB |
| CMYK | DeviceCMYK |
| I | Indexed |
| AHx | ASCIIHexDecode |
| A85 | ASCII85Decode |
| LZW | LZWDecode |
| Fl | FlateDecode |
| RL | RunLengthDecode |
| CCF | CCITTFaxDecode |
| DCT | DCTDecode |

## Image position

Position derived from CTM at the `Do` operator:

- PDF images are defined in a 1×1 unit square
- CTM transforms this to page coordinates
- `X, Y` = CTM translation (e[4], e[5]) — lower-left corner
- `PageWidth` = sqrt(a² + b²) where a=CTM[0], b=CTM[1]
- `PageHeight` = sqrt(c² + d²) where c=CTM[2], d=CTM[3]

## Filters supported

| Filter | Strategy |
|---|---|
| DCTDecode | JPEG passthrough |
| FlateDecode | Already implemented in parser.go (zlib + PNG predictor) |
| ASCII85Decode | Already implemented in parser.go |
| ASCIIHexDecode | Already implemented in parser.go |
| No filter | Raw bytes |
| JPXDecode | Out of scope (JPEG 2000, rare, needs external codec) |
| CCITTFaxDecode | Out of scope Phase 3 (fax compression, specialized) |
| JBIG2Decode | Out of scope Phase 3 (specialized) |
| LZWDecode | Out of scope Phase 3 (legacy, rare) |

Unsupported filters: skip image, do not error.

## Architecture — new files

- **`image.go`** — `Image` type, `Save`, `WriteTo`, `ExtractImages` for Page and Document
- **`image_decode.go`** — pixel decoding: color space conversion (CMYK→RGB, Indexed expansion, ICCBased resolution), soft mask application, PNG encoding
- **`image_inline.go`** — inline image parsing (BI/ID/EI dict and data extraction)
- **`image_test.go`** — integration tests with real PDF files
- **`image_decode_test.go`** — unit tests for color space conversion, PNG encoding

## Content stream integration

The existing `process()` loop in `text.go` already handles `Do` (for Form XObjects) and CTM tracking (`cm`, `q`, `Q`). For image extraction, we need a **separate** content stream walker (not reuse text extractor) that:

1. Tracks CTM via `cm`, `q`, `Q` (same matrix math already in `text.go`)
2. On `Do` — checks if XObject is `/Subtype /Image`, extracts it
3. On `BI` — parses inline image instead of skipping

This avoids coupling image extraction with text extraction. The CTM tracking code (matrix ops, graphics state stack) is already factored as standalone functions (`matMul`, `translateMatrix`, `identityMatrix`).

## Error handling

- Unsupported filter → skip image (no error)
- Corrupt image data → skip image (no error)
- Missing /Width, /Height, /BitsPerComponent → skip image
- `ExtractImages` returns error only for page-level failures (can't read content stream)

## Testing

- Integration tests with real PDF files from `testdata/`
- Save extracted images to `result_files/TestExtractImages/<pdf_name>/`
- Unit tests for CMYK→RGB conversion, indexed palette expansion, soft mask merge
- Synthetic content stream tests for inline images
