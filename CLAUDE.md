# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## License

This codebase is MIT-licensed (see `LICENSE` at the repo root). Every `.go` file carries an `// SPDX-License-Identifier: MIT` header above its `package` declaration. When adding new `.go` files, preserve this convention — the header on its own line followed by a blank line, then the `package` line.

## API design

When building new functionality, always shape the public API to mirror **Aspose.PDF for .NET**. Its class/method names, types, and call patterns are the reference model: prefer the same concepts (e.g. `Document`, `Page`, `TextFragment`, `Table`/`Row`/`Cell`, `OutlineItemCollection`, `BorderInfo`/`MarginInfo`), the same method names, and the same overall workflow, adapting only where Go idioms require it (error returns instead of exceptions, `io.Reader`/`io.Writer` stream variants, exported funcs over constructors). A Go developer who knows Aspose.PDF for .NET should recognize this API immediately. When a new feature has a counterpart in Aspose.PDF for .NET, check that API first and align with it before inventing a new shape; the existing surface (noted as "Mirrors Aspose.PDF for .NET's …" throughout the Public API section below) shows the pattern to follow.

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
- `(*Document).Info() (DocumentInfo, error)` — returns Info-dictionary metadata read from live in-memory state; mirrors Aspose.PDF for .NET's `Document.Info`
- `(*Document).ExtractText() ([]string, error)` — returns text for all pages (one entry per page)
- `(*Document).ExtractTextWithLayout() ([][]TextLine, error)` — returns structured text lines for each page
- `(*Document).SearchText(query, opts...) ([]TextMatch, error)` — finds occurrences of query across all pages (each match carries its page); mirrors Aspose.PDF for .NET's `TextFragmentAbsorber`

**`document_pages.go`** — page delete/split/extract operations
- `(*Document).DeletePage(n) error` — removes page n (1-based) in place; mirrors Aspose.PDF for .NET's `Document.Pages.Delete(int)`
- `(*Document).DeletePages(pageNums...) error` — removes the given 1-based pages in place; numbers are de-duplicated and validated before any removal (atomic on error); errors on no numbers or on removing every page; mirrors `Document.Pages.Delete(int[])`
- `(*Document).Split() ([]*Document, error)` — returns each page as a separate `*Document`
- `(*Document).Extract(ranges...) (*Document, error)` — returns a new `*Document` with the selected page ranges

**`page.go`** — `RotationAngle` type and constants (`Rotate0`, `Rotate90`, `Rotate180`, `Rotate270`)

**`page.go`** — Page and PageSize types
- `PageSizes(inputPath)` — returns dimensions of every page in a PDF file
- `(*Page).Number()` — 1-based page number within the document
- `(*Page).Size() (PageSize, error)` — page dimensions (width/height) from MediaBox (with inheritance from page tree)
- `(*Page).Rotation()` — effective rotation in degrees (0, 90, 180, 270); reflects Document.Rotate patches
- `(*Page).MediaBox() (Rectangle, error)` — full page rectangle (with inheritance); mirrors Aspose.PDF for .NET's `Page.MediaBox`
- `(*Page).CropBox() (Rectangle, error)` — visible region as a `Rectangle`; falls back to MediaBox if not set
- `(*Page).TrimBox() (Rectangle, error)` — intended trim dimensions; falls back to CropBox then MediaBox
- `(*Page).BleedBox() (Rectangle, error)` — production bleed region; falls back to CropBox then MediaBox
- `(*Page).ArtBox() (Rectangle, error)` — meaningful content extent; falls back to CropBox then MediaBox
- `(*Page).SetMediaBox/SetCropBox/SetTrimBox/SetBleedBox/SetArtBox(Rectangle) error` — set the respective box directly on the page (validated, overrides inherited/referenced values); mirror Aspose.PDF for .NET's `Page.*Box` setters
- `(*Page).SetPageSize(width, height float64) error` — resize the page by setting its MediaBox to `[0 0 width height]` (content is not scaled); mirrors `Page.SetPageSize`
- `(*Page).ExtractText() (string, error)` — returns the text content of a page in visual reading order; unknown font characters become U+FFFD
- `(*Page).ExtractTextWithLayout() ([]TextLine, error)` — returns structured text lines in visual reading order with coordinates and font info
- `(*Page).SearchText(query string, opts ...SearchOptions) ([]TextMatch, error)` — finds occurrences of query on the page in reading order, returning a bounding `Rectangle` per match (`text_search.go`); built on the layout pipeline, matches are located within a single line; literal by default, with optional case-insensitive and RE2-regex modes. Match rectangles use the per-glyph start positions recorded during extraction (`textFragment.runeX` → unexported `TextFragment.runeX`), so sub-fragment boxes are accurate rather than interpolated (right edge approximate only when a match ends a fragment's last glyph)
- `SearchOptions` struct — CaseInsensitive bool, Regex bool (zero value = case-sensitive literal, matching Aspose.PDF for .NET's default)
- `TextMatch` struct — Text string, PageNumber int (1-based), Rect Rectangle (PDF user space)
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
- `(*Document).SubsetFonts() (int, error)` — rebuilds every embedded TTF (`LoadFont`) to keep only the glyphs actually drawn, returning the number of fonts subsetted. Call once, after all text is added and before Save/WriteTo (later text could reference dropped glyphs). Typical reduction ~300-700 KB/font → a few KB. Implementation (`font_subset.go`): tracks used glyph IDs as the text encoders emit them, takes the transitive closure over composite-glyph components, renumbers the kept glyphs into a compact range, rebuilds `glyf`/`loca`/`hmtx`/`maxp`/`head`/`hhea` + a synthetic `post` 3.0 (drops `cmap`/`OS/2`/`name`/hinting), and reassembles the sfnt with correct checksums. Original glyph IDs are preserved as CIDs via a generated `/CIDToGIDMap` stream (was `/Identity`), so no content stream is rewritten; `/W` and `/ToUnicode` are trimmed to the used glyphs. Mirrors the post-processing shape of `OptimizeImages`
- `HAlign` — `HAlignLeft`, `HAlignCenter`, `HAlignRight`
- `VAlign` — `VAlignTop`, `VAlignMiddle`, `VAlignBottom`
- `TextStyle` struct — Font, Size, Color, Background, HAlign, VAlign, LineSpacing, Underline, Strikethrough, Rotation, Behind
- `(*Page).AddText(text, style, rect) error` — draws text inside a rectangle with word wrap, alignment, clipping, optional underline/strikethrough, rotation, and behind-content mode
- `(*Document).AddTextWatermark(text, style, pageNums...) error` — applies a text watermark to all or selected pages using full-page rectangle from MediaBox
- `(*Document).Form() *Form` — returns the document's AcroForm (always non-nil; empty form for documents without /AcroForm)
- `(*Page).Annotations() *AnnotationCollection` — returns the page's annotation collection (always non-nil; empty for pages with no /Annots)
- `(*Document).ApplyRedactions() error` — destructively removes content (text glyphs, image XObjects, paths) inside every `/Redact` annotation's regions, draws overlay text/fill, then deletes the redact annotations
- `(*Document).ValidateRedactions() error` — pre-flight parseability check on redact-bearing pages; recommended before `ApplyRedactions`

**`vector.go` / `vector_draw.go`**
- `LineCap` enum — `LineCapButt` (default), `LineCapRound`, `LineCapSquare`. PDF operator J. (Shared with `appearance_builder.go` annotation drawing.)
- `LineJoin` enum — `LineJoinMiter` (default), `LineJoinRound`, `LineJoinBevel`. PDF operator j.
- `LineStyle` struct — `Color *Color`, `Width float64`, `DashPattern []float64`, `DashPhase float64`, `Cap`, `Join`, `MiterLimit float64`. Width ≤ 0 → no stroke. Mirrors Aspose.PDF for .NET's GraphInfo stroke fields.
- `ShapeStyle` struct — embeds `LineStyle` + adds `FillColor *Color`, `FillPattern string` (internal SVG pattern-name hook), and `FillGradient Gradient` (public linear/radial gradient fill). Precedence on fill: `FillGradient` → `FillPattern` → `FillColor`. Either stroke or fill (or both) may be configured; if neither, draw call is a no-op.
- `Path` — opaque fluent builder. `NewPath().MoveTo(x, y).LineTo(x, y).CurveTo(c1x, c1y, c2x, c2y, x, y).QuadTo(cx, cy, x, y).Arc(cx, cy, r, startAngle, sweepAngle).Close()`. Arc decomposes into ≤4 cubic Beziers per the Goldapp formula (k = (4/3)·tan(θ/4)).
- `(*Page).DrawLine(from, to Point, style LineStyle) error` — single line segment.
- `(*Page).DrawRectangle(rect Rectangle, style ShapeStyle) error` — axis-aligned rect, stroke and/or fill.
- `(*Page).DrawRoundedRectangle(rect Rectangle, radius float64, style ShapeStyle) error` — radius auto-clamped to half-shorter-side.
- `(*Page).DrawCircle(center Point, radius float64, style ShapeStyle) error` — 4-Bezier approximation (kappa = 0.5522847498).
- `(*Page).DrawEllipse(center Point, rx, ry float64, style ShapeStyle) error` — axis-aligned ellipse.
- `(*Page).DrawPolyline(points []Point, style LineStyle) error` — open path, stroke-only. Errors if len(points) < 2.
- `(*Page).DrawPolygon(points []Point, style ShapeStyle) error` — closed path, stroke and/or fill. Errors if len(points) < 3.
- `(*Page).DrawPath(path *Path, style ShapeStyle) error` — arbitrary path. Errors on nil path.
- `Gradient` interface (`LinearGradient` / `RadialGradient`) + `GradientStop{Offset, Color}` (`vector_gradient.go`) — public gradient fills usable as `ShapeStyle.FillGradient` on every fill-capable Draw call. Constructors `NewLinearGradient(x1,y1,x2,y2, stops...)` and `NewRadialGradient(cx,cy,r, stops...)` (focal = centre); set `RadialGradient.FX/FY` for an off-centre highlight. Coordinates are PDF user space. Reuses the SVG shading machinery: `(*Page).resolveShapeGradient` adapts the public type to an internal `svgGradient` and calls `ensurePatternResource` (Type 2 axial / Type 3 stitched / Type 3 radial shading pattern) with an identity matrix; the resulting `/Pattern` resource name is stored in `FillPattern`, so the existing `/Pattern cs … scn` emission paints it. Limitations (mirror SVG Phase 3a): spread = pad, per-stop alpha not rendered (DeviceRGB shadings).
- Alpha (`Color.A < 1`) for stroke and fill is rendered via the existing `ensureExtGState` (now sets both `/CA` stroke alpha and `/ca` fill alpha). Distinct stroke vs. fill alpha values in the same shape: takes the more-restrictive value (single ExtGState per draw call). For per-property precision, use separate draw calls.
- Coordinates are PDF user space (Y up, origin at page bottom-left). Drawing outside the page is allowed; PDF viewers clip to MediaBox.

**`svg.go` / `svg_parse.go` / `svg_render.go` / `svg_path.go` / `svg_transform.go` / `svg_viewbox.go` / `svg_attrs.go` / `svg_types.go` / `svg_named_colors.go` / `vector_emit.go`**
- `(*Page).AddSVG(path, rect)` — reads an SVG file and renders it into the given rectangle on the page; unsupported elements are skipped silently
- `(*Page).AddSVGFromStream(r io.Reader, rect)` — io.Reader variant
- `(*Page).AddSVGObject(svg *SVG, rect)` — renders a pre-parsed `*SVG`
- `(*Document).LoadSVG(path) (*SVG, error)` — parse once, reuse on many pages
- `(*Document).LoadSVGFromStream(r io.Reader) (*SVG, error)` — io.Reader variant
- `(*Document).AddSVGWatermark(path string, pageNums ...int) error` — watermark on all (when pageNums empty) or selected pages; uses each page's full MediaBox honoring SVG `preserveAspectRatio`
- `(*Document).AddSVGWatermarkFromStream(r io.Reader, pageNums ...int) error` — io.Reader variant
- `(*Document).AddSVGObjectWatermark(svg *SVG, pageNums ...int) error` — pre-parsed watermark
- `SVG` — opaque pre-parsed type returned by `LoadSVG` / `LoadSVGFromStream`
- `(*SVG).ViewBox() (x, y, w, h float64)` — viewBox attribute or `(0, 0, intrinsicW, intrinsicH)` fallback
- `(*SVG).Size() (width, height float64)` — intrinsic dimensions from `<svg width=... height=...>` attrs
- **Supported in Phase 2**: basic shapes (`<rect>`/`<circle>`/`<ellipse>`/`<line>`/`<polyline>`/`<polygon>`/`<path>`); full SVG 1.1 path syntax (M/L/H/V/C/S/Q/T/A/Z + lowercase relatives) with elliptical-arc decomposition into cubic Béziers; transforms (`translate`/`rotate`/`scale`/`matrix`/`skewX`/`skewY`); `viewBox` + all 10 `preserveAspectRatio` modes with Y-flip; presentation attrs + inline `style="..."`; hex (3/6/8-digit), `rgb()`/`rgba()`, 147 CSS named colors, `none`/`transparent`/`currentColor`; absolute length units (px/pt/pc/mm/cm/in); group inheritance cascade (resolved at parse time)
- **Added in Phase 3a**: `<linearGradient>` and `<radialGradient>` rendering via PDF Type 2 (axial) / Type 3 (radial) shading patterns. Supports `<stop>` (offset numeric/percent, stop-color, stop-opacity), `gradientUnits` (both `userSpaceOnUse` and `objectBoundingBox`), `gradientTransform` (full matrix), `spreadMethod=pad`. Multi-stop gradients use Type 3 stitching combining Type 2 exponential interpolations. `fill="url(#id)"` and `stroke="url(#id)"` resolved at render time; missing refs fall back to no fill (best-effort).
- **Added in Phase 3b**: `<text>` and `<tspan>` rendering with mixed content (CharData + `<tspan>` + CharData), cursor-based positioning, `dx`/`dy` offsets, absolute `x`/`y` override on `<tspan>`, `text-anchor` (start/middle/end), `font-family`/`font-size`/`font-weight`/`font-style` attributes. Font matching: built-in heuristic mapping Standard 14 keywords (Arial/Helvetica → FontHelvetica, Times → FontTimesRoman, Courier → FontCourier, plus bold/italic variants); pluggable `SVGFontResolver` callback via `(*Document).SetSVGFontResolver` for embedded TTF fonts (Cyrillic etc.). Gradient fills (Phase 3a) work on text via the same `/Pattern cs` mechanism.
- `SVGFontResolver` — `func(family string, bold, italic bool) Font` — callback signature for font resolution
- `(*Document).SetSVGFontResolver(fn SVGFontResolver)` — register a custom resolver; the renderer queries it before falling back to the heuristic; pass `nil` to revert
- **Added in Phase 3c**: `<image>` (data:image/png and data:image/jpeg base64 inline — external URLs silently skipped); `<defs>`/`<use>`/`<symbol>` (reusable elements with parse-end deep-clone resolution; forward refs supported; cycle detection); `<clipPath>` (children = shape elements; `clipPathUnits` userSpaceOnUse + objectBoundingBox; multi-child union; maps to PDF `W`/`W*` operators); `clip-path="url(#id)"` presentation attribute on any shape/text/image.
- **Added in Phase 3d**: practical SVG completion — `<mask>` via PDF soft masks (Form XObject `/Group /S /Transparency` + ExtGState `/SMask`, supporting `maskUnits` and `maskContentUnits`); CSS `<style>` blocks with `.class`/`#id`/element selectors (specificity: inline > id > class > type); `<filter>` with `feDropShadow` emulated as offset+alpha bbox duplicate (no blur — PDF has no native Gaussian blur, other filter primitives silently skipped); `<marker>` (`marker-start`/`marker-mid`/`marker-end`) with `orient=auto` rotation along path tangent, `refX`/`refY` anchor, `markerUnits=strokeWidth`+`userSpaceOnUse`.
- **Out of scope (Phase 3e)**: `<textPath>`, vertical writing modes, `xml:space="preserve"`, em/ex/% length units, `spreadMethod=reflect/repeat`, CSS descendant/pseudo/attribute selectors, real Gaussian blur (requires rasterizer), external `href` in `<image>`, `data:image/svg+xml`
- **Best-effort error policy**: unsupported elements skipped silently; only XML parse failures and invalid numeric attrs surface as errors
- `vector_emit.go` — internal `emit*ToBuf` helpers extracted from Phase 1 `(*Page).Draw*` methods so SVG renderer can reuse the exact byte-emission code (PDF output is byte-identical to hand-written Phase 1 calls)

**`page_labels.go`** — page label support
- `(*Page).Label()` — formatted page label from the document's `/PageLabels` number tree; falls back to decimal page number if absent
- Supported styles: `/D` decimal, `/r`/`/R` roman, `/a`/`/A` alphabetic; optional `/P` prefix and `/St` start value

**`page_range.go`**
- `PageRange` struct — From, To (1-based, inclusive)

**`metadata.go`** — Info-dictionary metadata. Naming note: this is the PDF `/Info` dictionary and mirrors Aspose.PDF for .NET's `Document.Info` (`DocumentInfo`). In Aspose.PDF for .NET, `Document.Metadata` is the *XMP* store — which here is `(*Document).XMP` (see `xmp.go`), not this type. We deliberately name the Info surface `Info`/`DocumentInfo` (not `Metadata`) to avoid that collision.
- `(*Document).Info() (DocumentInfo, error)` — returns the Info-dictionary metadata read from live in-memory state
- `(*Document).SetInfo(info)` — replaces the Info dictionary in memory; full replacement, empty fields omitted
- `(*Document).ClearInfo()` — removes the Info dictionary; applied on Save/WriteTo
- `DocumentInfo` struct — Title, Author, Subject, Keywords, Creator, Producer, CreationDate, ModDate, Custom map[string]string

**`xmp.go`** — XMP metadata (the `/Catalog/Metadata` RDF/XML packet, ISO 32000-1 §14.3.2)
- `(*Document).XMP() (XMPMetadata, error)` — parse the XMP packet; zero value (`IsEmpty()`) when absent
- `(*Document).SetXMP(meta XMPMetadata) error` — serialise + store a standard packet (uncompressed `/Type /Metadata /Subtype /XML`); reuses the existing object on update
- `(*Document).ClearXMP()` — remove `/Catalog/Metadata` (orphans the stream; `RemoveUnusedObjects` reclaims it)
- `(*Document).XMPRaw() ([]byte, error)` / `(*Document).SetXMPRaw([]byte) error` — raw packet bytes escape hatch
- `(*Document).SyncInfoToXMP() error` — build an XMP packet from the current `/Info` dict so both metadata stores agree (PDF date → ISO 8601)
- `XMPMetadata` struct — Title (dc:title), Authors []string (dc:creator Seq), Description (dc:description), Keywords []string (dc:subject Bag), CreatorTool (xmp:CreatorTool), Producer (pdf:Producer), CreateDate/ModifyDate/MetadataDate (xmp:*, ISO 8601 strings), Custom []XMPProperty
- `XMPProperty` struct — Namespace, Prefix, Name, Value for arbitrary simple namespaced properties. Parser handles both attribute and element forms and rdf:Alt/Seq/Bag containers; `encoding/xml` does not surface the original prefix so a custom property's Prefix is synthesised on read (Namespace/Name/Value round-trip)

**`encrypt.go` / `decrypt.go` / `encrypt_aes.go` / `decrypt_aes.go` / `encrypt_aes256.go` / `decrypt_aes256.go`**
- `Encrypt(inputPath, outputPath, userPassword, ownerPassword)` — top-level helper writes RC4-128-protected PDF (PDF 1.4 Standard Security Handler V=2 R=3). For AES, use `(*Document).SetEncryption(EncryptionOptions{...})`
- `ErrEncrypted` — sentinel error from `Open`/`OpenStream` on encrypted input
- Decryption pipeline: `OpenWithPassword`/`OpenStreamWithPassword` parse `/Encrypt`, dispatch by `/V` (V=2 R=3 → RC4 path; V=4 R=4 → AES-128 path via `/CFM /AESV2`; V=5 R=6 → AES-256 path via `/CFM /AESV3` per ISO 32000-2). All paths share PKCS#7 helpers. For V≤4 password handling reuses Algorithms 2/5/7 (MD5-based); for V=5 R=6 password handling uses Algorithm 2.B (iterated SHA-256/384/512 hash chain). Per-object decryption uses Algorithm 1 (RC4) or 1.A (AES-128, with `"sAlT"` literal suffix in MD5 input); AES-256 uses the FEK directly (no per-object derivation). Stream `/Filter` chains are re-applied after decryption per PDF spec ordering (encrypt-after-filter)
- `Permissions` struct — eight bool flags (AllowPrint, AllowModify, AllowCopy, AllowAnnotations, AllowFormFill, AllowAccessibility, AllowAssembly, AllowPrintHighRes); zero value denies everything. Adobe-convention bit packing per ISO 32000-1 §7.6.3.2 Table 22 with reserved bits 7-8 and 13-32 set high
- `EncryptionOptions` struct — unified encryption configuration: UserPassword, OwnerPassword (empty → defaults to UserPassword), Permissions *Permissions (nil → grant all), Algorithm EncryptionAlgorithm (zero value → AES-128). Consumed by `(*Document).SetEncryption`
- `EncryptionAlgorithm` enum — `EncryptionAlgAES128` (default, AES-128 V=4 R=4 `/CFM /AESV2` per ISO 32000-1 §7.6.3.2), `EncryptionAlgRC4_128` (legacy V=2 R=3), `EncryptionAlgAES256` (AES-256 V=5 R=6 `/CFM /AESV3` per ISO 32000-2 §7.6.4; bumps PDF header to `%PDF-2.0` and includes /U /O /UE /OE /Perms entries with tamper-detection)
- AES-128 specifics: per-object key via `MD5(docKey || objNum_LE_3 || gen_LE_2 || "sAlT")[:16]` (Algorithm 1.A); AES-128-CBC with PKCS#7 padding and random 16-byte IV prepended to each encrypted string/stream. Single document-wide StdCF crypt filter; `/StmF` and `/StrF` both point to it
- AES-256 specifics: random 256-bit File Encryption Key (FEK) is encrypted into /UE under user-derived key and /OE under owner-derived key; passwords are validated against /U / /O hashes computed by Algorithm 2.B; /Perms is an AES-256-ECB encrypted permissions block under FEK providing tamper-detection of /P. Per-object encryption uses FEK directly with AES-256-CBC + PKCS#7 + random 16-byte IV. PDF header bumped to `%PDF-2.0` per ISO 32000-2 requirement

**`outline.go` / `outline_parse.go` / `outline_write.go` / `outline_destination.go`**
- `(*Document).Outlines() *OutlineItemCollection` — root outline collection. Always non-nil; empty for documents without `/Outlines`. Lazy-parsed on first call. Mirrors Aspose.PDF for .NET's `Document.Outlines`
- `OutlineItemCollection` — recursive tree node (entry + children collection). Mirrors Aspose .NET's `OutlineItemCollection : IList<OutlineItemCollection>`
- Constructor: `NewOutlineItemCollection(doc *Document) *OutlineItemCollection` — unattached entry, must be added via `Add`/`Insert` to take effect. .NET equivalent: `new OutlineItemCollection(doc.Outlines)`
- Style accessors: `Title()/SetTitle`, `Bold()/SetBold` (`/F` bit 2), `Italic()/SetItalic` (`/F` bit 1), `Color()/SetColor` (`*pdf.Color`), `IsExpanded()/SetIsExpanded` (sign of `/Count`)
- Target accessors: `Action()/SetAction` (reuses `Action` from annotations), `Destination()/SetDestination`. Both may be set; per ISO 32000-1 §12.3.3 viewers honor `/Dest` first
- Tree: `Add(child) error`, `Insert(index, child) error`, `Remove(child) bool`, `RemoveAt(index) error`, `At(index) *OutlineItemCollection`, `Count() int`, `All() []*OutlineItemCollection`, `Parent() *OutlineItemCollection`. Errors on nil child, cross-document, cycle, or already-attached
- `Destination` interface + 8 concrete types per ISO 32000-1 §12.3.2.2: `DestinationXYZ`, `DestinationFit`, `DestinationFitH`, `DestinationFitV`, `DestinationFitR`, `DestinationFitB`, `DestinationFitBH`, `DestinationFitBV`. Each has `NewDestinationXxx(page *Page, ...)` constructor; XYZ/FitH/FitV/FitBH/FitBV also have `NewDestinationXxxUnchanged` variants that encode `/null` for "leave as-is"
- Pages in destinations referenced by `*Page` (resolved to underlying object number at write time). Lazy dict-backed reads with copy-on-mutate: parsed items read from their PDF dict directly; first `SetXxx` call materializes all values into struct fields
- Encryption-safe: outlines roundtrip cleanly under AES-128, AES-256, and RC4-128

**`named_destinations.go` / `named_destinations_parse.go` / `named_destinations_write.go`**
- `(*Document).NamedDestinations() *NamedDestinations` — name-to-destination collection. Always non-nil; empty for documents without `/Catalog/Names/Dests` or `/Catalog/Dests`. Lazy-parsed on first call. Mirrors Aspose.PDF for .NET's `Document.NamedDestinations`
- `NamedDestinations` — collection with `Add(name, dest) error`, `Get(name) Destination`, `Has(name) bool`, `Remove(name) bool`, `Count() int`, `Names() []string` (lex-sorted snapshot), `All() map[string]Destination` (snapshot), `Clear()`, `Document()`. Per ISO 32000-1 §12.3.2.3
- `NamedDestination` — 9th concrete `Destination` type wrapping a name reference; `DestinationType()` returns `DestinationTypeNamed`. Constructor `NewNamedDestination(doc, name)`. Lazy `Resolve() Destination` and `Page() *Page` look up in the collection at call time (forward references allowed). Mirrors Aspose .NET's `NamedDestination` subtype
- Read path: `/Catalog/Dests` legacy dict + `/Catalog/Names/Dests` modern name tree merged into one collection; on collision `/Names/Dests` wins. Name tree walker handles arbitrary `/Kids` depth with cycle protection
- Write path: emit `/Catalog/Names/Dests` as a flat single-root tree (valid for any size per ISO 32000-1 §7.9.6). Legacy `/Catalog/Dests` is dropped on save — automatic migration. Sibling `/Catalog/Names` subentries (JavaScript, EmbeddedFiles, etc.) are preserved through round-trip
- Outline integration: `OutlineItemCollection.SetDestination(NewNamedDestination(doc, name))` serializes as `/Dest <name>` PDF string; on parse, `Destination()` returns `*NamedDestination` wrapper. Unregistered names still wrap (preserves the reference) — `Resolve()` returns nil to signal missing

**`form.go` / `form_fields.go`**
- `Form` — AcroForm view; `Fields() []Field`, `Field(name string) Field`, `HasField(name string) bool`, `NeedAppearances() bool`, `SetNeedAppearances(v bool)`
- `Field` interface — `PartialName() string`, `FullName() string`, `Value() string`, `SetValue(s string) error`, `IsReadOnly() bool`, `IsRequired() bool`, `PageIndex() int`, `Rect() Rectangle`
- Concrete types: `TextBoxField`, `CheckboxField`, `RadioButtonField` + `RadioButtonOptionField`, `ComboBoxField`, `ListBoxField`, `ButtonField` (push button)
- `ChoiceOption` — option data for ComboBox / ListBox: `Value`, `Export`
- `FormFieldType` enum + `FieldType(f Field) FormFieldType` convenience helper
- Field values are encoded UTF-16BE-with-BOM when non-ASCII, Latin-1 / PDFDocEncoding otherwise (per ISO 32000-1 §7.9.2.2)
- Widget `/AP/N` is pre-generated on every AddXxx call and re-generated on every value-mutating setter (SetValue / SetChecked / SetSelected / AddOption / RemoveOption), so the form renders identically across Acrobat / Foxit / browser PDF viewers / MuPDF / Poppler without depending on `/AcroForm/NeedAppearances`. The flag defaults to false; `SetNeedAppearances(true)` is opt-in for callers who want viewer-side regeneration (one side-effect: Acrobat marks the document as modified on open). Appearance generators in `appearance_widget.go` cover all six widget types — text/combo/list/push render the current value via the page text engine; checkbox/radio paint vector chrome (rect + tick, ring + dot)
- `(*Form).AddTextField/AddCheckbox/AddRadioGroup/AddComboBox/AddListBox/AddPushButton` — programmatic field creation; auto-creates /AcroForm and /AcroForm/DR/Font/Helv on first call; combined field+widget dict for single-widget fields, parent + kids for radio groups
- `(*Form).RemoveField(name) bool` — removes field plus all its widgets from /AcroForm/Fields and per-page /Annots
- `(*Document).Flatten() error` / `(*Form).Flatten() error` — bake every form field's current appearance into its page content stream and remove all fields, widgets, and `/AcroForm` (the result renders identically but is no longer fillable). `Document.Flatten` is a convenience for `Form().Flatten`. Mirrors Aspose.PDF for .NET's `Document.Flatten` / `Form.Flatten` (`flatten.go`)
- `(Field).Flatten() error` — flattens a single field (bakes its widgets, removes just that field), leaving other fields and `/AcroForm` intact. Mirrors `Field.Flatten`
- Per-type structural mutators: `SetReadOnly`, `SetRequired` on every type; `TextBoxField.{SetMaxLen,SetMultiline,SetPassword}`; `ComboBoxField.{SetEditable,AddOption,RemoveOption}`; `ListBoxField.{SetMultiSelect,AddOption,RemoveOption}`
- `RadioItem` struct — `PageNum`, `Rect`, `Export` for cross-page radio groups
- `(Field).SetStyle(FieldStyle) error` / `(Field).Style() FieldStyle` — visual styling on every field type. `FieldStyle` struct: `BorderColor`, `BackgroundColor`, `TextColor *Color`, `BorderWidth float64`, `BorderStyle` (Solid/Dashed/Beveled/Inset/Underline), `DashPattern []float64`, `TextFont Font`, `TextSize float64`, `TextAlign HAlign`. Persisted per ISO 32000-1 as `/MK` (/BC, /BG), `/BS` (/W, /S, /D), `/DA` (font + size + colour), `/Q` (quadding) and re-rendered into the widget `/AP`, so the chosen look is identical across Acrobat / MuPDF / browser viewers. Round-trips through Save+Open. Choice/text alignment honoured (left/centre/right); checkbox uses BorderColor for the box and TextColor for the tick; radio uses BorderColor for the ring and TextColor for the dot. Font styling covers the Standard 14 fully; embedded TTF fonts render with the chosen font in-session and fall back to Helvetica metrics after a Save+Open round-trip (the `/DA` still references the embedded font for viewer-side regeneration). `(*Form).ensureFont(Font)` registers any font in `/AcroForm/DR/Font` and is reused for the default Helvetica
- `(*ButtonField).SetAppearance(ButtonAppearance) error` (`form_button.go`) — rich push-button appearance. `ButtonAppearance` struct: `Caption` (/MK/CA), `RolloverText` (/MK/RC), `DownText` (/MK/AC), `IconPath` (PNG/JPEG, baked into /AP), `IconPosition ButtonIconPosition` (/MK/TP: `ButtonCaptionOnly`/`ButtonIconOnly`/`ButtonIconAboveCaption`/`ButtonCaptionOverIcon`), `TextColor`/`FaceColor`/`BorderColor *Color`. Writes the /MK characteristics and bakes three appearance streams — /AP/N (normal), /AP/R (rollover), /AP/D (down, with a depressed-look offset + darkened face) — so the button reacts to hover/press in any viewer. Rollover/down captions fall back to Caption when empty. The icon is embedded as an Image XObject and drawn into each state's Form XObject. Out of scope (follow-up): caption rotation (/MK/R) and exposing the icon as a /MK/I Form XObject for viewer-side regeneration

**`annotation.go` / `annotation_action.go` / `annotation_link.go` / `annotation_markup.go`**
- `Annotation` interface — `AnnotationType()`, `Rect()/SetRect()`, `Color()/SetColor()`, `Title()/SetTitle()`, `Contents()/SetContents()`, `PageIndex()`
- `AnnotationType` enum — `AnnotationTypeUnknown`, `AnnotationTypeLink`, `AnnotationTypeHighlight`, `AnnotationTypeUnderline`, `AnnotationTypeStrikeOut`, `AnnotationTypeSquiggly`, `AnnotationTypeWidget`, `AnnotationTypeSquare`, `AnnotationTypeCircle`, `AnnotationTypeLine`, `AnnotationTypeInk`, `AnnotationTypeText`, `AnnotationTypeFreeText`, `AnnotationTypeStamp`, `AnnotationTypeFileAttachment`, `AnnotationTypeRedact`
- Concrete types: `LinkAnnotation`, `HighlightAnnotation`, `UnderlineAnnotation`, `StrikeOutAnnotation`, `SquigglyAnnotation`, `WidgetAnnotation` (existing form fields, read-only via this surface), `GenericAnnotation` (catch-all for unsupported subtypes)
- `AnnotationCollection` — `Add(a) error`, `At(i) Annotation`, `Delete(a) bool`, `DeleteAt(i) error`, `Count() int`, `All() []Annotation`. Add panics on nil; idempotent same-page; errors on cross-page re-attach
- `(Annotation).Flatten() error` — bakes the annotation's normal appearance (`/AP/N`, honoring `/AS`) into the page content at its `/Rect`, then removes it (no-appearance annotations are removed without drawing). `(*AnnotationCollection).Flatten() error` — flattens every non-widget annotation on the page (widgets are left for `Form.Flatten`). Shared baking machinery in `flatten.go`; mirrors Aspose.PDF for .NET's `Annotation.Flatten`
- Constructors: `NewLinkAnnotation(page, rect)`, `NewHighlightAnnotation(page, rect)`, `NewUnderlineAnnotation(page, rect)`, `NewStrikeOutAnnotation(page, rect)`, `NewSquigglyAnnotation(page, rect)`
- `LinkAnnotation.Action() Action`, `LinkAnnotation.SetAction(act Action)` — nil clears /A
- `LinkAnnotation.Highlight() LinkHighlightMode`, `LinkAnnotation.SetHighlight(h LinkHighlightMode)` — controls /H click-feedback (None / Invert / Outline / Push)
- `LinkHighlightMode` enum — `LinkHighlightInvert` (default), `LinkHighlightNone`, `LinkHighlightOutline`, `LinkHighlightPush`
- `Action` interface — `ActionType()`; concrete types: `GoToURIAction`, `GoToAction`, `NamedAction`, `SubmitFormAction`, `ResetFormAction`, `JavaScriptAction` (parsed from existing PDFs and constructable; read via `Script() string`)
- Action constructors: `NewGoToURIAction(uri)`, `NewGoToAction(pageNum, top)`, `NewNamedAction(name)`, `NewSubmitFormAction(url, fields, flags)`, `NewResetFormAction(fields)`, `NewJavaScriptAction(script)`
- `ActionType` enum — `ActionTypeUnknown`, `ActionTypeGoToURI`, `ActionTypeGoTo`, `ActionTypeNamed`, `ActionTypeSubmitForm`, `ActionTypeResetForm`, `ActionTypeJavaScript`
- `NamedActionType` enum — `NamedActionFirstPage`, `NamedActionLastPage`, `NamedActionNextPage`, `NamedActionPrevPage`, `NamedActionPrint`
- `SubmitFormFlags` bitfield per ISO 32000-1 Table 237 (`SubmitIncludeNoValueFields`, `SubmitExportFormat`, `SubmitGetMethod`, ...)
- `QuadPoint` struct — `X1 Y1 X2 Y2 X3 Y3 X4 Y4` floats per ISO 32000-1 §12.5.6.10 (UL/UR/LL/LR corners). Used by `SetQuadPoints`/`QuadPoints` on the four markup types

**`annotation_drawing.go` / `appearance.go` / `appearance_builder.go`**
- `Point` struct — single point in PDF user-space (used for Line endpoints, Ink strokes)
- `BorderStyle` enum — `BorderSolid`, `BorderDashed`, `BorderBeveled`, `BorderInset`, `BorderUnderline` per ISO 32000-1 Table 168
- `LineEndingStyle` enum — 10 styles (`LineEndingNone`, `LineEndingSquare`, `LineEndingCircle`, `LineEndingDiamond`, `LineEndingOpenArrow`, `LineEndingClosedArrow`, `LineEndingButt`, `LineEndingROpenArrow`, `LineEndingRClosedArrow`, `LineEndingSlash`) per ISO 32000-1 Table 176
- `SquareAnnotation` / `CircleAnnotation` — `BorderWidth/SetBorderWidth`, `BorderStyle/SetBorderStyle`, `DashPattern/SetDashPattern`, `Color/SetColor` (stroke), `InteriorColor/SetInteriorColor` (fill), inherited `Rect/SetRect/Title/SetTitle/Contents/SetContents/PageIndex`. Constructors `NewSquareAnnotation(page, rect)` / `NewCircleAnnotation(page, rect)`
- `LineAnnotation` — `Start/SetStart`, `End/SetEnd`, `StartLineEnding/SetStartLineEnding`, `EndLineEnding/SetEndLineEnding`, `LeaderLineLength/SetLeaderLineLength`, `InteriorColor/SetInteriorColor`. Auto-bbox /Rect from endpoints + `9 × BorderWidth` padding. Constructor `NewLineAnnotation(page, start, end)`
- `InkAnnotation` — `Strokes/SetStrokes` (defensive deep copy), `AddStroke`, full border surface. Catmull-Rom smoothed in /AP for 3+ point strokes; raw /InkList stored unchanged. Constructor `NewInkAnnotation(page, strokes)`
- All four types regenerate `/AP/N` on every property setter; an explicit `RegenerateAppearance()` method is also exposed on each type
- `/AP/N` infrastructure: every drawing annotation owns one Form XObject in `doc.objects`. Setters mutate the XObject in place — no leaks across multiple property changes

**`annotation_text.go` / `annotation_freetext.go` / `annotation_stamp.go` / `appearance_freetext.go` / `appearance_stamp.go`**
- `TextAnnotation` — sticky-note annotation. `Icon()/SetIcon(t)`, `Open()/SetOpen(b)`, inherited `SetRect/SetColor/SetTitle/SetContents`. Constructor `NewTextAnnotation(page, position Point)` — auto-bbox 24×24pt at anchor. No /AP — viewers render the icon themselves
- `TextIcon` enum — `TextIconNote` (default), `TextIconComment`, `TextIconKey`, `TextIconHelp`, `TextIconNewParagraph`, `TextIconParagraph`, `TextIconInsert`, `TextIconUnknown`
- `FreeTextAnnotation` — text drawn directly on the page. `Contents()/SetContents`, `TextStyle()/SetTextStyle` (round-trips through /DA + /Q + /BG). Border via `drawingAnnotationBase` (BorderWidth/BorderStyle/DashPattern). `Intent()/SetIntent` for Plain/Callout/Typewriter modes; `CalloutPoints/EndLineEnding/InnerRect` for callouts; `BorderEffect/BorderEffectIntensity` for cloudy borders. Honors `style.VAlign` (Top/Middle/Bottom) in /AP/N rendering. Constructor `NewFreeTextAnnotation(page, rect, contents, style)`
- `FreeTextIntent` enum — `FreeTextIntentFreeText` (default), `FreeTextIntentCallout`, `FreeTextIntentTypewriter`
- `BorderEffect` enum — `BorderEffectNone` (default), `BorderEffectCloudy` (wavy "cloud" border via /BE/S=/C)
- `StampAnnotation` — rubber-stamp annotation. `Name()/SetName(StampName)`, `RawName()/SetRawName(string)` (escape hatch for non-spec names). Custom image override via `SetCustomImage(path)/SetCustomImageFromStream(r)/ClearCustomImage()`. Border via `drawingAnnotationBase`. Constructor `NewStampAnnotation(page, rect, name)`. Library-default visuals for all 14 predefined names (color-coded: green=positive, red=warning, orange=informational, gray=neutral)
- `StampName` enum — 14 names per ISO 32000-1 §12.5.6.13 Table 184: `StampNameApproved`, `StampNameAsIs`, `StampNameConfidential`, `StampNameDepartmental`, `StampNameDraft`, `StampNameExperimental`, `StampNameExpired`, `StampNameFinal`, `StampNameForComment`, `StampNameForPublicRelease`, `StampNameNotApproved`, `StampNameNotForPublicRelease`, `StampNameSold`, `StampNameTopSecret`, plus `StampNameUnknown` for non-spec names
- All three types regenerate `/AP/N` on every property setter (TextAnnotation has no /AP — `RegenerateAppearance()` is no-op for API symmetry); explicit `RegenerateAppearance()` method exposed on each type

**`annotation_fileattachment.go` / `annotation_redact.go` / `appearance_redact.go` / `redact_apply*.go`**
- `FileAttachmentAnnotation` — embedded file annotation. `Icon()/SetIcon(i)`, `SetFile(path)/SetFileFromStream(r, name)`, `HasFile()`, read-only metadata `FileName/FileMIMEType/FileSize/FileBytes/FileDescription/SetFileDescription`. Constructor `NewFileAttachmentAnnotation(page, position Point)` — auto-bbox 24×24pt. No /AP — viewers render the icon themselves
- `FileAttachmentIcon` enum — `FileAttachmentIconPaperclip` (default), `FileAttachmentIconGraph`, `FileAttachmentIconPushPin`, `FileAttachmentIconTag`, `FileAttachmentIconUnknown`. MIME type auto-detected from file extension via `mime.TypeByExtension`; embedded file stored as a `/EmbeddedFile` stream referenced by a `/Filespec` dict
- `RedactAnnotation` — mark + apply redaction. `QuadPoints()/SetQuadPoints`, `InteriorColor()/SetInteriorColor`, `OverlayText()/SetOverlayText`, `RepeatOverlayText()/SetRepeatOverlayText`, `OverlayTextStyle()/SetOverlayTextStyle`. Border via `drawingAnnotationBase`. Mark-mode `/AP/N` renders quad fills + optional overlay preview; destructive content removal via `(*Document).ApplyRedactions()`. Constructor `NewRedactAnnotation(page, rect)`
- `(*Document).ApplyRedactions() error` — destructively removes content inside every `/Redact` annotation's `/QuadPoints` (or `/Rect`) regions: text glyphs (per-glyph filter with TJ kerning gaps to preserve surviving positions), `Do` XObject invocations (drop or clip), and drawing paths (drop or clip). After rewrite, fills each quad with `/IC` color and renders `/OverlayText` (centered or tiled if `/Repeat`); then removes the redact annotations from `/Annots`. Best-effort semantics — partial state on failure
- `(*Document).ValidateRedactions() error` — pre-flight dry-run parseability check on every redact-bearing page; recommended before `ApplyRedactions`
- `NewJavaScriptAction(script string) *JavaScriptAction` — public constructor for `/JavaScript` actions; the action is encoded back on Save (JS actions are also parsed from existing PDFs). Includes documented security warning — embedded JavaScript executes in the recipient's viewer
- Apply pipeline files: `redact_apply.go` orchestrates; `redact_apply_text.go` rewrites Tj/TJ/'/" with per-glyph filtering; `redact_apply_image.go` clips/drops `Do` invocations using even-odd clip paths; `redact_apply_path.go` clips/drops path-construction sequences buffered until a paint terminator

**`table.go` / `table_render.go`**
- `pdf.NewTable() *Table` — builder for a tabular layout drawn onto a Page. Mirrors Aspose.PDF for .NET's `Table` class. After `(*Page).AddTable` renders the table, the `*Table` is not held by the document
- `Table` — `SetColumnWidths([]float64) *Table` (in points), `SetBorder(BorderInfo) *Table` (outer), `SetDefaultCellBorder(BorderInfo) *Table`, `SetDefaultCellMargin(MarginInfo) *Table` (per-cell padding default), `SetDefaultCellStyle(TextStyle) *Table`, `AddRow() *Row`, `Rows() []*Row`, `RowCount() int`. Getters for each setter
- `Row` — `AddCell(text) *Cell`, `AddCells(texts ...string) []*Cell`, `Cells() []*Cell`, `CellCount() int`, `SetHeight(float64) *Row` (0 = auto-fit), `Height() float64`, `Table() *Table`
- `Cell` — `SetText`, `SetTextStyle`, `SetBackground(*Color)`, `SetBorder(BorderInfo)`, `SetMargin(MarginInfo)`, `SetHAlign(HAlign)`, `SetVAlign(VAlign)` — all chainable, all paired with getters. Per-cell setters override the table default. `Background()/Border()/Margin()` return nil when the cell inherits the table default
- `BorderSide` enum — `BorderSideNone`, `BorderSideTop`, `BorderSideRight`, `BorderSideBottom`, `BorderSideLeft`, `BorderSideAll` (bitwise OR of all four)
- `BorderInfo` struct — `Sides BorderSide`, `Width float64`, `Color *Color` (nil = black). Zero value = no border. Width 0 also = no border regardless of Sides. Mirrors Aspose.PDF for .NET's `BorderInfo`
- `MarginInfo` struct — `Top`/`Right`/`Bottom`/`Left` in points. Inside a Cell represents the padding between border and content. Mirrors Aspose.PDF for .NET's `MarginInfo`
- `(*Page).AddTable(t *Table, rect Rectangle) (int, error)` — renders the table inside the rectangle. Cell content is drawn via the existing `AddText` machinery (inherits its word-wrap, alignment, font embedding, Unicode handling, clipping). When rows don't fit, continuation pages are auto-appended to the document and the return value reports how many were added (0 if the table fits in `rect`). Errors on nil table, bad rect, mismatched cell count, or non-positive column width
- Cell text style resolution: zero `TextStyle` → table.DefaultCellStyle overlay → cell.TextStyle overlay → explicit HAlign/VAlign overrides
- Border layering: cell backgrounds first, then cell text, then cell borders (so borders appear on top of clipped text edges), then table outer border last (so outer border appears on top of cell-edge overlaps)
- `Cell.SetColSpan(n) / ColSpan() int` — cell occupies n consecutive columns; default 1. When set, the caller does not add cells for the columns covered by the span — the row simply has fewer cells. Mirrors Aspose.PDF for .NET's `Cell.ColSpan`
- `Cell.SetRowSpan(n) / RowSpan() int` — cell occupies n consecutive rows; default 1. Covered positions in subsequent rows are skipped by the caller. Mirrors Aspose.PDF for .NET's `Cell.RowSpan`
- `Table.SetRepeatingRowsCount(n) / RepeatingRowsCount() int` — marks the first n rows as headers that repeat at the top of every continuation page (default 0). Mirrors Aspose.PDF for .NET's `Table.RepeatingRowsCount`
- `Table.SetOverflowMargins(top, bottom) / OverflowMargins()` — top/bottom margins (points) for the continuation rect on auto-appended pages; defaults 50pt each. Same LLX/URX as the original rect; Y range = [bottom, pageHeight - top]
- `(*Page).AddTable(t, rect) (pagesAdded int, err error)` — now returns the number of continuation pages auto-appended (0 if the table fits in rect). Validation also rejects: ColSpan/RowSpan out of bounds, merge overlaps, rowspan crossing the header/body boundary, header height exceeding rect height, or any spanning group too tall for a continuation page
- Spanning groups: rows linked by rowspan are atomic — a group never breaks across pages. Each group is the smallest contiguous range [s, e] such that no rowspan in [s, e] extends past e. Page-break decisions operate on groups, not individual rows
- `Cell.SetImage(path) / SetImageFromStream(r) / Image() (path, hasImage)` — cell renders an image instead of text (image wins over text if both set). Auto-fits cell interior width preserving aspect ratio; HAlign/VAlign positions it. PNG and JPEG supported. Mirrors Aspose.PDF for .NET's `Cell.Image`
- `Row.SetBackground(*Color) / Background() *Color` — row-level background; cells inherit unless they call SetBackground themselves
- `Row.SetTextStyle(TextStyle) / TextStyle() *TextStyle` — row-level text style overlay between table.DefaultCellStyle and cell.TextStyle in the inheritance chain
- `Row.SetBorder(BorderInfo) / Border() *BorderInfo` — row-level border default; cells inherit unless overridden
- `Row.SetMargin(MarginInfo) / Margin() *MarginInfo` — row-level cell padding default
- `Table.AddRows([][]string) []*Row` — batch row constructor; one row per inner slice, one cell per string. Returns the rows for further per-row styling. Spans not supported in batch flow
- Border edge de-duplication: identical-style adjacent border lines (cell-cell shared edges, outer border overlapping cell perimeter edges) emit only once per page. Different styles still render both for caller intent. Per-page edge tracking
- Inheritance chain (4 deep): zero TextStyle/MarginInfo/BorderInfo ← `table.Default*` ← `row.*` ← `cell.*` ← cell.HAlign/VAlign override. Background chain: nil ← `row.Background` ← `cell.Background`
- Out of Phase 3 scope (Phase 4 candidates): auto-fit column widths (content-driven), dash patterns on borders, per-side border width/color, rowspan splitting across page breaks, image cells with explicit pixel sizing

**`validate.go`**
- `Validate(inputPath)` — checks a PDF for **structural integrity** (parseable + internally consistent); returns `*ValidationReport` with a `Valid` flag and a list of `ValidationIssue` (code + message). NOT a standards check: it does not validate PDF/A or PDF/UA. This differs from Aspose.PDF for .NET's `Document.Validate`, which checks PDF/A·PDF/UA conformance — the capability here is intentionally narrower
- Issue codes: `INVALID_HEADER`, `XREF_ERROR`, `OBJECT_ERROR`, `PAGE_TREE_ERROR`, `STREAM_ERROR`, `ENCRYPTED`
- Checks performed: header, xref/trailer, all objects readable, page tree traversal, orphaned `/Pages` nodes, `/Page` → `/Parent` refs resolve to `/Pages`, streams without `/Filter` don't contain compressed data

**`raster.go` / `raster_path.go` / `raster_stroke.go` / `render.go` / `render_device.go` / `render_image.go` / `render_glyph.go` / `render_text.go` / `render_clip.go` / `render_gstate.go` / `render_shading.go` / `render_annotations.go` / `render_document.go` / `render_tiling.go` / `render_type3.go` / `render_dingbats.go` / `cff.go` / `bmp.go` / `tiff.go`** — pure-Go page rasterizer (no external dependencies; stdlib `image`/`image/png`/`image/jpeg`/`image/gif`/`compress/zlib` only). Phased epic (umbrella `pdf-go-61r`); spec `docs/superpowers/specs/2026-06-04-render-to-image-design.md`. Status: **render epic complete (P1–P6 + annotations + multi-page TIFF)** (vector + images + text + clip/alpha/shadings + annotation appearances + stroke quality → PNG/JPEG/GIF/BMP/TIFF). P2 adds Image XObjects (decoded via the existing image pipeline + `/SMask` alpha, inverse-mapped onto the page) and Form XObject recursion (`render_image.go`) plus an in-house BMP encoder (`bmp.go`). P3 adds text: a TrueType glyph-outline decoder (`render_glyph.go`, simple + composite glyphs from the `glyf` table) and a text-object state machine (`render_text.go`) that fills glyphs for **embedded** TrueType fonts (Type0/CIDFontType2 + `/FontFile2`). P4 (`render_std14.go`) renders **non-embedded** fonts (Standard 14) by substituting glyph shapes from bundled **metric-compatible** open families — Arimo (Helvetica/Arial), Tinos (Times), Cousine (Courier), Carlito (Calibri — a narrower face than Arial, so without it Calibri fell back to the wider Arimo and its glyphs overran the document's Calibri advances), SIL OFL, subset to Latin and `//go:embed`ed. Because their advance widths equal the originals, word-wrapped layout is preserved and glyphs aren't distorted; serif/mono and bold/italic render distinctly. `render_fontrepo.go` adds a `FontRepository` (mirrors Aspose.PDF for .NET): `AddFontFolder`/`AddFontFile`/`AddSystemFonts` register sources used before the bundled substitute. Resolution for a non-embedded simple font: registered/system font → bundled substitute. Internals (lowercase): `flattener` adaptively flattens Bézier/arc paths to device-space polylines; `rasterizer` produces anti-aliased coverage via analytic-X + 4× supersampled-Y scanlines (nonzero + even-odd); `compositeCoverage` does src-over with an optional clip mask; `strokeToFill` (`raster_stroke.go`) builds the stroke outline with real line caps (butt/round/square), line joins (miter — with miter-limit→bevel fallback — / round / bevel) and dash patterns (`applyDash`); `renderer` interprets the content stream (q/Q/cm/w, path ops, f/S/B painting, Gray/RGB/CMYK colour, text with `Tr` rendering modes — fill/stroke, invisible mode 3 paints nothing, `Do` images, `BI` inline images, `gs`, `W`/`W*` clip, `sh`, `J`/`j`/`M`/`d` stroke params, `cs`/`CS` colour spaces, `BDC`/`EMC` Optional-Content visibility) and **skips the few operators not yet supported** so any page renders. Stencil image masks (`/ImageMask true`, inline `/IM true`, `render_imagemask.go`) paint the current fill colour through their 1-bit "on" samples (per `/Decode`), inverse-mapped through the CTM. A `/JBIG2Decode` image mask is JBIG2-decoded first (its globals come from `/DecodeParms`, which `decodeStream` can't reach), since otherwise the raw compressed bytes would be painted as the mask. Blend modes (`render_blend.go`) are honored: `gs` `/BM` selects a `blendMode` on the graphics state — separable (Multiply/Screen/Overlay/Darken/Lighten/ColorDodge/ColorBurn/HardLight/SoftLight/Difference/Exclusion, per channel) or non-separable (Hue/Saturation/Color/Luminosity, on the whole colour via `Lum`/`Sat`/`SetLum`/`SetSat`/`ClipColor`). The compositors funnel through `blendApply` as `C = (1−a)·Cb + a·B(Cb,Cs)` for an opaque backdrop; Normal/unsupported keep the fast src-over path. Type3 fonts (`render_type3.go`) render each glyph by executing its `/CharProcs` content stream with `FontMatrix · text-rendering matrix · text matrix · CTM`, reusing the interpreter; the code→glyph-name table comes from `/Encoding/Differences`. Optional Content (`render_oc.go`): content tagged with an OCG/OCMD that is OFF in `/Catalog/OCProperties/D` — via a `/OC` marked-content section (`BDC`…`EMC`) or an `/OC` entry on an XObject — is suppressed (OCMD policies AnyOn/AllOn/AnyOff/AllOff honored). After the content stream it paints annotation appearance streams (`render_annotations.go`): every `/Annots` entry's normal appearance (`/AP/N`, selected by `/AS` for state subdictionaries) is drawn as a Form XObject mapped into `/Rect` per ISO 32000-1 §12.5.5 — so AcroForm field widgets, stamps, highlights, free text, etc. render the way a viewer shows them; `/Popup` and Hidden/NoView (`/F` bits 2/6) annotations are skipped. P5 (`render_clip.go`/`render_gstate.go`/`render_shading.go`): `W`/`W*` clipping intersected into a per-pixel coverage mask (`gs.clip`, applied after the next paint per ISO 32000-1 §8.5.4, saved/restored by q/Q); constant alpha via `gs` → `/ca`/`/CA`/`/LW` (translucent content blends); soft masks via `gs` `/SMask` (`render_softmask.go`): the group `/G` is rendered off-screen and reduced to a per-pixel luminosity/alpha mask that `effectiveClip` folds into every paint (saved/restored by q/Q, cleared by `/None`); axial (type 2) and radial (type 3) shadings via `sh` and shading patterns (PatternType 2), with a m-in/n-out function evaluator (`render_shading.go` + `render_function_ps.go`): types 0 sampled, 2 exponential, 3 stitching, function arrays, and 4 PostScript calculator (a real stack interpreter — arithmetic/stack/comparison/`if`/`ifelse`) and colour model inferred from output-component count. Separation and DeviceN colour spaces (`render_colorspace.go`): `cs`/`CS` resolve the space; on `sc`/`scn` the tint operands run through the tint-transform function (commonly a Type 4) to RGB. Tiling patterns (PatternType 1, `render_tiling.go`) are rendered by executing the cell content stream once per tile across the fill path's bbox (stepped by `/XStep`/`/YStep` through `/Matrix`), clipped to the path ∩ current clip; uncolored (PaintType 2) cells inherit the fill colour set with the pattern; the tile count is capped at `maxTiles`. P6 adds stroke quality (caps/joins/dash above) and the performance pass: `coverageBBox`/`compositeCoverageBBox` allocate and scan only a path's pixel bounding box instead of the whole frame (glyphs/fills are tiny), cutting the 13-page showcase from ~35 s to ~1.1 s at 150 DPI (~30×). `cff.go` adds CFF/Type2 charstring outlines (`pdf-go-l7b`): `parseCFF` reads the CFF container (INDEXes, Top/Private DICTs, charset, FDArray/FDSelect for CID-keyed) and a Type2 interpreter flattens charstrings into the same on-curve `glyphContour` the glyf decoder emits. Embedded OpenType-CFF (`/FontFile3`: Type1C / CIDFontType0C / OpenType) is resolved in `buildRenderFont` via `parseCFFProgram` (bare CFF or `OTTO` wrapper); `.otf` registered in the `FontRepository` is parsed as an sfnt whose `'CFF '` table attaches to `ttfFont.cff`, so `glyphContours` draws from it. `renderFont` carries either `prog` (TrueType) or `cff`; `gid()` maps CID→GID through the CFF charset for CIDFontType0C. **Simple** (non-Type0) CFF fonts (Type1C `/FontFile3`, e.g. MinionPro subsets) build a code→GID map (`cffFont.simpleGID`, `buildSimpleEncoding`): the CFF charset gives GID→SID and the Encoding gives code→glyph — for the predefined Standard encoding a code's SID is `code−31` across 32–126, reversed through the charset; a custom Encoding table (formats 0/1) is read directly. A **predefined** charset (Top DICT charset offset 0 = ISOAdobe) is materialised as the identity GID→SID map so subset fonts that keep the standard glyph order (e.g. NewsGothicBT) still resolve. Without this, simple CFF fonts mapped every code to GID 0 and rendered no text. (`.otf` embedding via `LoadFont` is still rejected — it has no glyf for `/FontFile2`.) Non-embedded ZapfDingbats (`render_dingbats.go`) synthesizes vector outlines for the common marks — Acrobat's checkbox/radio "check styles": check `4`/`5`, cross `6`/`7`/`8`, circle `l`, square `m`/`n`, diamond `u`, star `H` — built in 1000-em space and painted via the shared `paintContours`; advance widths come from the ZapfDingbats AFM, so checkbox/radio widget appearances render. Out of render scope: mesh shadings (ShadingType 4–7), isolated/knockout transparency groups (`pdf-go-rom`), non-embedded Symbol glyph shapes and the rarer decorative ZapfDingbats marks (`pdf-go-ylj`: code→Unicode encoding is wired so embedded/registered fonts render, but no Latin substitute is forced — non-embedded Symbol with no covering font draws nothing rather than .notdef boxes).
- `RenderOptions` struct — `DPI float64` (0 → `DefaultDPI` = 150), `Background *Color` (nil → white)
- `(*Page).RenderImage(opts) (image.Image, error)` — rasterize the page (CropBox region, Y-flip/rotation, chosen DPI) to an `*image.RGBA`; never errors on unsupported content (skips it)
- `(*Page).RenderPNG(w, opts) error` / `RenderJPEG(w, opts, quality) error` / `RenderGIF(w, opts) error` / `RenderBMP(w, opts) error` / `RenderTIFF(w, opts) error`
- `(*Document).RenderImage(pageNum, opts) (image.Image, error)`
- `(*Document).RenderTIFF(w, opts, pageNums...) error` — renders the document (or the listed 1-based pages, in order) into a **single multi-page TIFF**; pages are rendered one at a time so peak memory is one page image (`render_document.go`, `tiff.go`). The TIFF encoder is in-house (stdlib has none; `golang.org/x/image/tiff` would be an external dep): little-endian, RGB8, one strip/page, Deflate via `compress/zlib` (Compression tag 8), chained IFDs for multi-page, DPI in X/YResolution. Validated against Pillow (opens 13-page output, detects `tiff_adobe_deflate`). Mirrors Aspose.PDF for .NET's TiffDevice
- Aspose-style device wrappers (mirror Aspose.PDF for .NET): `Resolution` (`NewResolution(dpi)`), `PngDevice`/`JpegDevice`/`GifDevice`/`BmpDevice` (`NewPngDevice(res)` etc.) with `Process(page, w) error`; `TiffDevice` (`NewTiffDevice(res)`) with `Process(page, w) error` (single page) and `ProcessDocument(doc, w) error` (whole-doc multi-page)
- `AddFontFolder(path)` / `AddFontFile(path)` / `AddSystemFonts()` / `ClearFontSources()` — register fonts the renderer uses for non-embedded text before the bundled metric-compatible substitutes; mirrors Aspose.PDF for .NET's `FontRepository`. System fonts are opt-in. Indexes `.ttf` and `.ttc` (TrueType Collection — each sub-font indexed separately via `parseFontCollection`/`parseSFNTAt` in `ttf.go`; each sub-font's table directory is captured on `ttfFont.tables` so shared-table glyph reads resolve at the right offset). Also indexes `.otf` (OpenType-CFF): parsed as an sfnt whose `'CFF '` table attaches to `ttfFont.cff`, so its glyphs render through the CFF interpreter (`cff.go`). Matching is by the font's real name-table **family** (name ID 1) + style, with Standard-14 aliases (Helvetica→Arial, Times→Times New Roman, Courier→Courier New) — not a serif/mono/sans keyword guess — so a request never resolves to an unrelated face; no family match → bundled substitute

### PDF parsing pipeline

1. **`io.go`** — file I/O (`readFile`, `writeFile`)
2. **`xref.go`** — locates and parses the cross-reference table or stream; handles both traditional xref tables (PDF ≤1.4) and cross-reference streams (PDF 1.5+)
3. **`lexer.go`** — byte-level tokenizer; produces tokens (int, float, name, string, keyword, etc.)
4. **`parser.go`** — builds `pdfValue` objects from tokens; handles dicts, arrays, streams with FlateDecode/ASCIIHex/ASCII85 filters and PNG predictor (Predictor 12). CCITTFaxDecode (`ccitt.go` / `ccitt_tables.go`) decodes Group 4 (`/K<0`) and Group 3 1-D (`/K=0`) fax data into packed 1-bpp rows (used by 1-bit scanned images and image masks); reads `/Columns`/`/Rows`/`/BlackIs1`/`/EncodedByteAlign` from `/DecodeParms` (mixed Group 3 2-D, `/K>0`, is not yet supported)
5. **`doc.go`** — document-level logic: object lookup with caching, object streams (ObjStm), page tree traversal, dependency collection
6. **`types.go`** — type definitions: `pdfValue`, `pdfDict`, `pdfArray`, `pdfStream`, `pdfRef`, `pdfObject`, `xrefEntry`

**Malformed-input tolerance** (`openStreamCore` → `buildFromXRef`, with fallbacks):
- Stream `/Length` may be an indirect reference (`/Length N 0 R`, ISO 32000-1 §7.3.8.2), missing, or wrong — `readStreamData` (`parser.go`) takes a usable direct integer verbatim, otherwise scans for the `endstream` keyword via `streamEndIndex`, which prefers the `endstream` confirmed by a following `endobj` so a spurious `endstream` byte sequence inside binary stream data (common in CCITT/JPEG streams) doesn't truncate the stream mid-data.
- `skipToStreamData` (`lexer.go`) skips spaces/tabs before the CR/LF after the `stream` keyword (non-conformant `stream \r\n`), so the filter sees a correctly aligned data start instead of mis-decoding (which previously caused a multi-GB allocation when the resulting garbage parsed as content ops).
- `flateDecode` (`parser.go`) keeps the inflated bytes when a valid-header zlib stream ends in a bad trailing Adler-32 checksum or is truncated by a wrong `/Length` (common in the wild — otherwise the whole page blanks). A bad zlib *header* is still rejected, so an undecrypted/random stream is not inflated into garbage.
- xref entry rows are read up to any line terminator (LF, CR, or CRLF), so classic-Mac CR-only xref tables parse.
- `xref_reconstruct.go` — when the xref is missing/corrupt or the catalog/page tree won't resolve (e.g. an off-by-one subsection start), `reconstructXRef` rebuilds the table by scanning the file for `N G obj` headers (latest revision wins) and recovers the trailer from the last `trailer` dict or the first `/Type /Catalog` object, then `openStreamCore` retries. Limitation: objects only inside compressed object streams (ObjStm) aren't recovered by the scan.

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
2. DCTDecode images are passed through as JPEG; **CMYK** DCTDecode (and any with an `/SMask`) are decoded to RGB and re-encoded as PNG, because Adobe/Photoshop CMYK & YCCK JPEGs store their channels inverted (APP14 "Adobe" marker) and Go's `image/jpeg` returns them raw — `jpegHasAdobeMarker` detects the marker and `decodeJPEGToPixels` re-inverts so white doesn't render as black; all non-DCT images are decoded to pixels and encoded as PNG
3. Color spaces: DeviceRGB, DeviceGray, DeviceCMYK (→RGB via `adobeCMYKToRGB`, a 5×5×5×5 LUT baked from the MuPDF/Adobe default CMYK profile with quadrilinear interpolation — `cmyk_lut.go` — so process colours match Acrobat instead of the bluish naive `(1-C)(1-K)`; used by image decoding and the renderer's `k`/`K`/`scn` fill paths), Indexed (palette expansion), ICCBased (treated as underlying RGB/Gray/CMYK)
4. Soft masks (`/SMask`) are applied as PNG alpha channels; JPEG+SMask is re-encoded as PNG
5. Inline images (BI/ID/EI) are parsed with abbreviation expansion (PDF spec Tables 4.43/4.44)
6. Form XObjects are recursed into with inherited CTM and resources
7. **JBIG2** (`/JBIG2Decode`, 1-bpp scanned bilevel): decoded in-house (`jbig2*.go`) since `decodeStream` can't reach the `/JBIG2Globals` stream — `extractXObjectImageData` detects the filter, resolves globals from `/DecodeParms` (`jbig2GlobalsData`), and calls `jbig2Decode` to a packed 1-bpp DeviceGray bitmap (0 = black; JBIG2 foreground is inverted), so extraction and the renderer (which shares `Extract`) both work. See the JBIG2 section below for scope.

### JBIG2 decoding (`jbig2.go`, `jbig2_mq.go`, `jbig2_generic.go`, `jbig2_refine.go`, `jbig2_symbol.go`, `jbig2_text.go`, `jbig2_tables.go`)

Pure-Go JBIG2 decoder (ITU-T T.88) for the PDF `/JBIG2Decode` filter. Epic `pdf-go-hqr`. **Phase 1 (complete)** covers the embedded-organization segment stream on the arithmetic (MQ) coding path — the combination scanned-document PDFs almost always use:
- `jbig2_tables.go` — the MQ probability-estimation state table (Table E.1).
- `jbig2_mq.go` — the MQ arithmetic decoder (software conventions: C split into chigh/clow), the arithmetic integer decoding procedures (`decodeInt` for IADH/IADW/IAEX/IAAI/IADT/IAFS/IADS/IAIT/IARI/IARDW/IARDH/IARDX/IARDY) and `decodeIAID` for symbol IDs. Faithful port of the reference decoder; validated byte-identical to jbig2dec / PyMuPDF.
- `jbig2_generic.go` — `jbig2Bitmap` (height×width, 1 byte/pixel) and generic-region decoding (templates 0–3 with adaptive pixels and TPGDON typical prediction).
- `jbig2_refine.go` — generic refinement-region decoding (templates 0/1), used by refinement/aggregate symbol dictionaries and refining text regions.
- `jbig2_symbol.go` — `jbig2Ctx` (all IAx contexts + GB/GR context arrays + IAID), symbol-dictionary decoding by height class; the SDREFAGG path refines a single referenced symbol (REFAGGNINST==1) or aggregates several via a refining text region (>1); IAEX export run-lengths.
- `jbig2_text.go` — text-region decoding: strips of symbol instances placed by IADT/IAIT (T), IAFS/IADS (S) and IAID, honoring REFCORNER/TRANSPOSED/SBDSOFFSET and per-instance refinement; the strip loop reads the terminating OOB IADS so the bit stream stays in sync.
- `jbig2.go` — segment-header parser (embedded organization), page composition across segments (page info / generic region / symbol dict / text region), and the public `jbig2Decode(stream, globals, w, h)` returning packed 1-bpp.

**Phase 2 (added: `jbig2_huffman.go`, `jbig2_huffsym.go`, `jbig2_hufftext.go`, `jbig2_mmr.go`, `jbig2_halftone.go`):**
- `jbig2_huffman.go` — Huffman bit reader, canonical-prefix table builder/decoder, and the 15 standard tables (Annex B); custom table segments (type 53) parse in `jbig2.go`.
- `jbig2_huffsym.go` — Huffman symbol dictionaries: height classes with a collective bitmap (MMR-coded, via `jbig2_mmr.go`, or uncompressed) split into per-symbol bitmaps. `jbig2_hufftext.go` — Huffman text regions with the run-code symbol-ID table (§7.4.3.1.7), including transposed regions with SBSTRIPS>1. Validated byte-identical (0 differing pixels) on a real two-page globals + symbol-dict + text-region scan (one page transposed, one not).
- `jbig2_mmr.go` — MMR (Group 4) regions/bitplanes reuse the `ccitt.go` G4 decoder (`pdf-go-d6n`).
- `jbig2_halftone.go` — pattern dictionaries (type 16) and halftone regions (types 20/22/23): Gray-coded bitplanes select indexed patterns stamped on a grid (`pdf-go-nkf`). Standalone refinement regions (types 40/42/43, `pdf-go-2mk`) decode in `jbig2.go` via the Phase 1 refinement decoder.

Halftone/standalone-refinement are implemented per spec but not yet corpus-validated. Unsupported constructs are skipped, so a page still renders rather than erroring.

## Output conventions

- All files produced by examples and manual runs are saved to `result_files/` in the project root.
- This folder is not committed to the repository.
- Exception: `_examples/feature_showcase/main.go` writes `docs/feature_showcase.pdf` (committed and linked from README). Regenerate only on meaningful changes to keep git history lean.

## Examples

- Standalone runnable example programs live in `_examples/<name>/main.go`. The leading `_` makes the Go toolchain skip these when matching `./...`, so `go build ./...` and `go test ./...` do not touch them. Run individually with e.g. `go run ./_examples/feature_showcase`.
- Short, focused API examples live as `ExampleXxx` functions in `examples_test.go` (package `asposepdf_test`). These appear under "Examples" on pkg.go.dev next to each documented function and are validated by `go test` via their `// Output:` comments.

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
