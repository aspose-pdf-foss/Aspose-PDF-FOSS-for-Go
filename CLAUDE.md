# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Run all tests
go test ./...

# Run a single test
go test -run TestDocumentSplit ./...

# Run tests with verbose output
go test -v ./...

# Build (no binary — library only)
go build ./...
```

## Architecture

Pure Go library. No external dependencies. All code is in the root package `asposepdf`.

### Public API

**`document.go`** — mutable Document API; operations mutate the receiver in place
- `Open(path)` — opens a PDF file and returns a `*Document`; returns `ErrEncrypted` if the file is password-protected
- `OpenStream(r io.Reader)` — opens a PDF from an `io.Reader` and returns a `*Document`; returns `ErrEncrypted` if the file is password-protected
- `OpenWithPassword(path, password)` — opens an encrypted PDF, trying the password as both user and owner password; works on plain PDFs too
- `OpenStreamWithPassword(r io.Reader, password)` — same as `OpenWithPassword` but reads from any `io.Reader`
- `ErrEncrypted` — sentinel error returned by `Open`/`OpenStream` when the file is encrypted; check via `errors.Is(err, asposepdf.ErrEncrypted)`
- `(*Document).PageCount()` — current page count
- `(*Document).Pages()` — returns `[]*Page` live views of all pages
- `(*Document).Page(n)` — returns a `*Page` live view of page n (1-based)
- `(*Document).Rotate(angle, pageNums...) error` — rotates selected pages; rotation accumulates
- `(*Document).SetRotation(angle, pageNums...) error` — sets selected pages to exactly angle, replacing any existing rotation
- `(*Document).Reorder(order) error` — rearranges pages in place; pages may be repeated or omitted
- `(*Document).Append(others...)` — appends all pages from others into this document; nil arguments are skipped
- `(*Document).SetPassword(userPassword, ownerPassword)` — configures encryption; applied on Save/WriteTo
- `(*Document).SetPermissions(p Permissions)` — configures viewer-enforced permissions (print, copy, modify, etc.) for encrypted documents; applied on Save/WriteTo
- `(*Document).SetEncryption(opts EncryptionOptions)` — unified options-pattern API that replaces any prior encryption configuration (passwords + permissions) in one call
- `(*Document).Permissions() (Permissions, bool)` — returns the viewer permissions configured on the document; bool indicates whether the document is encrypted at all
- `(*Document).RemoveEncryption()` — clears any configured passwords and permissions so the next Save produces a plaintext PDF
- `(*Document).WriteTo(w) (int64, error)` — writes the document to an `io.Writer` (implements `io.WriterTo`)
- `(*Document).Save(outputPath) error` — writes the document to a file
- `(*Document).Metadata() (Metadata, error)` — returns Info metadata read from live in-memory state
- `(*Document).ExtractText() ([]string, error)` — returns text for all pages (one entry per page)
- `(*Document).ExtractTextWithLayout() ([][]TextLine, error)` — returns structured text lines for each page

**`document_pages.go`** — split/extract operations
- `(*Document).Split() ([]*Document, error)` — returns each page as a separate `*Document`
- `(*Document).Extract(ranges...) (*Document, error)` — returns a new `*Document` with the selected page ranges

**`page.go`** — `RotationAngle` type and constants (`Rotate0`, `Rotate90`, `Rotate180`, `Rotate270`)

**`page.go`** — Page and PageSize types
- `PageSizes(inputPath)` — returns dimensions of every page in a PDF file
- `(*Page).Number()` — 1-based page number within the document
- `(*Page).Size()` — page dimensions from MediaBox (with inheritance from page tree)
- `(*Page).Rotation()` — effective rotation in degrees (0, 90, 180, 270); reflects Document.Rotate patches
- `(*Page).CropBox()` — visible region; falls back to MediaBox if not set
- `(*Page).TrimBox()` — intended trim dimensions; falls back to CropBox then MediaBox
- `(*Page).BleedBox()` — production bleed region; falls back to CropBox then MediaBox
- `(*Page).ArtBox()` — meaningful content extent; falls back to CropBox then MediaBox
- `(*Page).ExtractText() (string, error)` — returns the text content of a page in visual reading order; unknown font characters become U+FFFD
- `(*Page).ExtractTextWithLayout() ([]TextLine, error)` — returns structured text lines in visual reading order with coordinates and font info
- `PageSize` struct — Width, Height in points (1/72 inch)
- `Color` struct — R, G, B, A float64 (values in [0, 1])
- `TextLine` struct — Text, Y, Fragments []TextFragment
- `TextFragment` struct — Text, X, Y, Width, FontName, FontSize, Height, Bold, Italic, CharSpacing, Color Color, IsSubscript, IsSuperscript
- `(*Page).ExtractImages() ([]Image, error)` — returns all images found on the page
- `(*Document).ExtractImages() ([][]Image, error)` — returns images for all pages (one slice per page)
- `Image` struct — Data, Format, Width, Height, BPC, ColorSpace, X, Y, PageWidth, PageHeight, Inline
- `ImageFormat` — ImageFormatPNG, ImageFormatJPEG
- `ImageColorSpace` — ColorSpaceDeviceRGB, ColorSpaceDeviceGray, ColorSpaceDeviceCMYK, ColorSpaceIndexed, ColorSpaceICCBased
- `(*Image).Save(path) error` — writes the image data to a file
- `(*Image).WriteTo(w) (int64, error)` — writes the image data to a writer
- `ImageInfo` struct — Width, Height, BPC, ColorSpace, Format, X, Y, PageWidth, PageHeight, Inline, Name
- `(*ImageInfo).Extract() (*Image, error)` — decodes the image and returns the full Image with pixel data
- `(*Page).ImageInfos() ([]ImageInfo, error)` — returns metadata for all images without decoding
- `(*Document).ImageInfos() ([][]ImageInfo, error)` — returns image metadata for all pages without decoding
- `(*ImageInfo).Replace(path) error` — replaces image data from a file; format detected by magic bytes (JPEG, PNG); position unchanged
- `(*ImageInfo).ReplaceFromStream(r) error` — replaces image data from an io.Reader
- `(*ImageInfo).Remove() error` — removes image from page (resources + content stream); XObject stays in doc objects
- `Rectangle` struct — LLX, LLY, URX, URY (PDF rectangle in points)
- `(*Page).AddImage(path, rect) error` — adds an image from a file to the page; format detected by magic bytes (JPEG, PNG)
- `(*Page).AddImageFromStream(r, rect) error` — adds an image from an io.Reader to the page
- `ImageToDocument(path, opts...) (*Document, error)` — creates a single-page PDF from an image file; DPI-aware page sizing
- `ImageToDocumentFromStream(r, opts...) (*Document, error)` — creates a single-page PDF from an image reader
- `ImageToDocumentOptions` struct — PageWidth, PageHeight, MarginLeft, MarginRight, MarginTop, MarginBottom
- `(*Document).RemoveUnusedObjects() int` — removes objects not reachable from any page; returns count of removed objects
- `OptimizeImageOptions` struct — MaxDPI, JPEGQuality, ConvertPNGToJPEG
- `(*Document).OptimizeImages(opts) (int, error)` — optimizes images to reduce file size; downscales above MaxDPI, converts opaque PNG to JPEG
- `PageFormat` struct — Width, Height in points; predefined: `PageFormatA3`, `PageFormatA4`, `PageFormatLetter`, `PageFormatLegal`
- `(PageFormat).Landscape()` — returns the format with width and height swapped
- `NewDocument(width, height) *Document` — creates a single-page blank document with given dimensions
- `NewDocumentFromFormat(format) *Document` — creates a single-page blank document from a predefined page format
- `(*Document).AddBlankPage(width, height) error` — appends a blank page with given dimensions
- `(*Document).AddBlankPageFromFormat(format) error` — appends a blank page from a page format
- `(*Document).InsertBlankPage(position, width, height) error` — inserts a blank page at a 1-based position
- `(*Document).InsertBlankPageFromFormat(position, format) error` — inserts a blank page from a page format at a position
- `Font` — interface implemented by standard 14 fonts and embedded TTF fonts; has `BaseFont()` and `IsEmbedded()` methods
- Standard 14 PDF fonts as package-level `Font` vars: `FontHelvetica`, `FontHelveticaBold`, `FontHelveticaOblique`, `FontHelveticaBoldOblique`, `FontTimesRoman`, `FontTimesBold`, `FontTimesItalic`, `FontTimesBoldItalic`, `FontCourier`, `FontCourierBold`, `FontCourierOblique`, `FontCourierBoldOblique`, `FontSymbol`, `FontZapfDingbats`
- `FindFont(name) (Font, error)` — returns a standard 14 `Font` by PostScript name (case-insensitive); error for unknown names
- `(*Document).LoadFont(path) (Font, error)` — reads a TTF file, embeds it into the document, returns a `Font` usable in `TextStyle.Font`
- `(*Document).LoadFontFromStream(r) (Font, error)` — like `LoadFont` but reads from an `io.Reader`
- `HAlign` — `HAlignLeft`, `HAlignCenter`, `HAlignRight`
- `VAlign` — `VAlignTop`, `VAlignMiddle`, `VAlignBottom`
- `TextStyle` struct — Font, Size, Color, Background, HAlign, VAlign, LineSpacing, Underline, Strikethrough, Rotation, Behind
- `(*Page).AddText(text, style, rect) error` — draws text inside a rectangle with word wrap, alignment, clipping, optional underline/strikethrough, rotation, and behind-content mode
- `(*Document).AddTextWatermark(text, style, pageNums...) error` — applies a text watermark to all or selected pages using full-page rectangle from MediaBox

**`page_labels.go`** — page label support
- `(*Page).Label()` — formatted page label from the document's `/PageLabels` number tree; falls back to decimal page number if absent
- Supported styles: `/D` decimal, `/r`/`/R` roman, `/a`/`/A` alphabetic; optional `/P` prefix and `/St` start value

**`page_range.go`**
- `PageRange` struct — From, To (1-based, inclusive)

**`metadata.go`**
- `(*Document).SetMetadata(meta)` — replaces the Info dictionary in memory; full replacement, empty fields omitted
- `(*Document).ClearMetadata()` — removes the Info dictionary; applied on Save/WriteTo
- `Metadata` struct — Title, Author, Subject, Keywords, Creator, Producer, CreationDate, ModDate, Custom map[string]string

**`encrypt.go` / `decrypt.go`**
- `Encrypt(inputPath, outputPath, userPassword, ownerPassword)` — writes a password-protected PDF using RC4-128 (PDF 1.4 Standard Security Handler, revision 3)
- `ErrEncrypted` — sentinel error from `Open`/`OpenStream` on encrypted input
- Decryption pipeline: `OpenWithPassword`/`OpenStreamWithPassword` parse `/Encrypt`, verify password against `/U` (user) or recover via `/O` (owner) using PDF Algorithm 7 reverse, derive document key, then decrypt every parsed object except `/Encrypt` itself in `rawDocument.getObject`. Stream `/Filter` chains are re-applied after RC4 decryption per PDF spec ordering (encrypt-after-filter)
- `Permissions` struct — eight bool flags (AllowPrint, AllowModify, AllowCopy, AllowAnnotations, AllowFormFill, AllowAccessibility, AllowAssembly, AllowPrintHighRes); zero value denies everything. Adobe-convention bit packing per ISO 32000-1 §7.6.3.2 Table 22 with reserved bits 7-8 and 13-32 set high
- `EncryptionOptions` struct — unified encryption configuration: UserPassword, OwnerPassword (empty → defaults to UserPassword), Permissions *Permissions (nil → grant all). Consumed by `(*Document).SetEncryption`

**`validate.go`**
- `Validate(inputPath)` — checks a PDF for structural integrity; returns `*ValidationReport` with a `Valid` flag and a list of `ValidationIssue` (code + message)
- Issue codes: `INVALID_HEADER`, `XREF_ERROR`, `OBJECT_ERROR`, `PAGE_TREE_ERROR`, `STREAM_ERROR`, `ENCRYPTED`
- Checks performed: header, xref/trailer, all objects readable, page tree traversal, orphaned `/Pages` nodes, `/Page` → `/Parent` refs resolve to `/Pages`, streams without `/Filter` don't contain compressed data

### PDF parsing pipeline

1. **`io.go`** — file I/O (`readFile`, `writeFile`)
2. **`xref.go`** — locates and parses the cross-reference table or stream; handles both traditional xref tables (PDF ≤1.4) and cross-reference streams (PDF 1.5+)
3. **`lexer.go`** — byte-level tokenizer; produces tokens (int, float, name, string, keyword, etc.)
4. **`parser.go`** — builds `pdfValue` objects from tokens; handles dicts, arrays, streams with FlateDecode/ASCIIHex/ASCII85 filters and PNG predictor (Predictor 12)
5. **`doc.go`** — document-level logic: object lookup with caching, object streams (ObjStm), page tree traversal, dependency collection
6. **`types.go`** — type definitions: `pdfValue`, `pdfDict`, `pdfArray`, `pdfStream`, `pdfRef`, `pdfObject`, `xrefEntry`

### PDF writing (`writer.go`)

`buildDocumentPDF(d *Document)` is the sole output function:
1. Assign sequential output IDs to all objects in `d.objects`
2. Patch `/Parent` in every page dict to point to the new `/Pages` node (via `pdfDirectRef`)
3. Serialize each object; write `/Pages`, `/Catalog`, `/Info`, `/Encrypt` structural objects last
4. Write xref table + trailer

**`pdfDirectRef`** (defined in `types.go`) — like `pdfRef` but written by `writeValue` without remapping. Used for `/Parent` patches so that the new `/Pages` object number (output space) is never accidentally remapped.

### Dependency collection (`doc.go`)

`collectPageDeps` recursively walks the object graph (dict values, array elements, stream dict, and raw stream bytes via regex `\b(\d+)\s+\d+\s+R\b`) to find all objects needed for a page. Skips `/Pages` and `/Catalog` nodes — these are rebuilt by the writer. Used by `Split` and `Extract` to build new single-document object sets.

`rewriteRefs` deep-copies a `pdfValue` tree translating all `pdfRef` IDs through an id-map. Used by `Append` to merge objects from another document without ID collisions.

### Text extraction (`text.go`, `text_layout.go`, `content_parser.go`, `font.go`, `font_metrics.go`, `encoding.go`, `cmap.go`)

1. `parseContentStream(data)` tokenizes content stream bytes into `contentOp` structs (operator + operands), reusing the existing `lexer`
2. `resolveFont(objects, fontDict)` maps font dictionaries to `fontInfo` — supports WinAnsi, MacRoman, MacExpert, Standard encodings, `/Differences`, standard 14 fonts, Symbol, ZapfDingbats, ToUnicode CMap, Type0/CIDFont with Identity-H encoding; resolves glyph widths from `/Widths`, Standard 14 metrics, CID `/DW`+`/W`, or fallback
3. `parseCMap(data)` (`cmap.go`) parses ToUnicode CMap streams — handles `beginbfchar`/`endbfchar` and `beginbfrange`/`endbfrange` (sequential and array forms); returns `map[uint16]rune`
4. `textExtractor` state machine processes operators (BT/ET/Tf/Td/Tm/Tj/TJ/Tz/etc.), tracking text matrix position, font, spacing, and horizontal scaling; advances text matrix by glyph width after each character (PDF spec 9.4.4); splits into single-byte and multi-byte paths for Type0/CIDFont
5. Fragment collection: `emitRune` collects `textFragment` structs with (x, y, endX, fontName, fontSize); new fragment on font change, Y gap > fontSize×0.5, or X gap > spaceWidth×0.3
6. Visual sorting (`text_layout.go`): `groupFragmentsIntoLines` sorts fragments by Y descending then X ascending, groups by Y proximity into `TextLine` structs; `ExtractTextWithLayout` returns the structured result; `ExtractText` delegates to same pipeline
7. Form XObjects (`Do` operator) are recursively processed with inherited CTM and overridden resources
8. Marked content (`BDC`/`BMC`/`EMC`): when `BDC` carries `/ActualText` in its properties, glyph emission is suppressed and the replacement text is emitted at `EMC`; supports inline dicts, `/Properties` resource lookup, UTF-16BE strings, and nesting

### Image extraction (`image.go`, `image_decode.go`, `image_inline.go`)

1. Content stream walker tracks CTM via `cm`/`q`/`Q` and collects images on `Do` (XObject) and `BI` (inline)
2. DCTDecode images are passed through as JPEG; all others are decoded to pixels and encoded as PNG
3. Color spaces: DeviceRGB, DeviceGray, DeviceCMYK (→RGB), Indexed (palette expansion), ICCBased (treated as underlying RGB/Gray/CMYK)
4. Soft masks (`/SMask`) are applied as PNG alpha channels; JPEG+SMask is re-encoded as PNG
5. Inline images (BI/ID/EI) are parsed with abbreviation expansion (PDF spec Tables 4.43/4.44)
6. Form XObjects are recursed into with inherited CTM and resources

## Output conventions

- All files produced by examples and manual runs are saved to `result_files/` in the project root.
- This folder is not committed to the repository.

## Testing conventions

- Test PDF files are stored flat in `testdata/` (`4pages.pdf`, `Binder1.pdf`, `PdfWithLinks.pdf`, `PdfWithTable.pdf`, `alfa.pdf`, `marketing.pdf`, `Hello world.pdf`, `PdfWithAcroForm.pdf`).
- Which files each test uses is declared in `testdata/testfiles.json` — keyed by test function name; value is `[][]string` (array of groups, each group is an array of file names). One group = one test run; multiple groups = the test is run once per group.
- When writing tests that use real PDF files, use the `testFile(t)`, `testFiles(t)`, or `testGroups(t)` helpers from `helpers_test.go`, and add the corresponding entry to `testdata/testfiles.json`. Ask the user which file to use before adding a new entry.
- Each feature gets its own `*_test.go` file (e.g. `splitter_test.go`, `metadata_test.go`).
- `TestSplitFiles` in `splitter_test.go` iterates files listed in `testdata/testfiles.json` under `"TestSplitFiles"`, splits each into `result_files/TestSplitFiles/<stem>/`, and validates every output page with `Validate`.

## Task tracking (beads)

This project uses [beads](https://github.com/gastownhall/beads) for issue/task tracking via the `bd` CLI.

```bash
# Status overview
bd status

# Create an issue
bd create "title" --body "description"

# List issues
bd list

# Update issue status
bd update <issue-id> --status <open|in-progress|closed>

# View an issue
bd show <issue-id>
```
