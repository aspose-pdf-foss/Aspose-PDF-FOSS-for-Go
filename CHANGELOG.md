# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `(*Page).SearchText(query, opts...)` / `(*Document).SearchText(query, opts...)` — locate occurrences of a query in reading order, returning a `TextMatch` (text + 1-based page + bounding `Rectangle`) for each. Literal and case-sensitive by default; `SearchOptions{CaseInsensitive, Regex}` enables case-folding and RE2 regular expressions. Built on the layout-extraction pipeline; matches are located within a single line. Match rectangles use per-glyph start positions recorded during extraction, so sub-fragment boxes are accurate (not interpolated). Mirrors Aspose.PDF for .NET's `TextFragmentAbsorber`.
- `(*Document).DeletePage(n)` / `(*Document).DeletePages(pageNums...)` — remove pages in place by 1-based number; numbers are de-duplicated and validated before any removal (atomic on error), and removing every page is rejected. Mirrors Aspose.PDF for .NET's `Document.Pages.Delete(int)` / `Delete(int[])`.

### Changed

- Renamed the Info-dictionary API to mirror Aspose.PDF for .NET's `Document.Info`: `(*Document).Metadata()` → `Info()`, `SetMetadata()` → `SetInfo()`, `ClearMetadata()` → `ClearInfo()`, and the `Metadata` struct → `DocumentInfo`. In Aspose.PDF for .NET, `Document.Metadata` is the XMP store (here `(*Document).XMP`), so the previous name collided.

### Documentation

- Clarified that `Validate` is a structural-integrity check, not a PDF/A·PDF/UA conformance check (unlike Aspose.PDF for .NET's `Document.Validate`).
- Corrected `JavaScriptAction` docs: it is constructable via `NewJavaScriptAction` (and encoded back on Save), not parse-only.

## [0.2.0] — 2026-05-27

SVG embedding completes practical coverage (~95% of real-world SVG files) across five sub-phases: shapes & paths, gradients, text, image / defs / use / clipPath, and mask / CSS / filter / marker. All work added internally — no breaking changes to v0.1.0 API.

### Added — public API

- `(*Page).AddSVG(path, rect)` / `AddSVGFromStream(r, rect)` / `AddSVGObject(svg *SVG, rect)` — embed external SVG into a PDF page
- `(*Document).LoadSVG(path) (*SVG, error)` / `LoadSVGFromStream(r io.Reader) (*SVG, error)` — pre-parse for reuse across many pages
- `(*Document).AddSVGWatermark(path, pageNums ...int) error` / `AddSVGWatermarkFromStream` / `AddSVGObjectWatermark` — SVG watermarks on all or selected pages
- `(*SVG).ViewBox() (x, y, w, h float64)` / `(*SVG).Size() (width, height float64)` — inspector accessors on the opaque `*SVG` type
- `SVGFontResolver func(family string, bold, italic bool) Font` — callback type for font-family resolution
- `(*Document).SetSVGFontResolver(fn SVGFontResolver)` — register a custom resolver (e.g. for embedded TTF / Cyrillic); falls back to built-in heuristic

### Added — SVG support matrix

**Phase 2 — SVG-lite embedding** (shapes + paths + transforms + viewBox)
- Basic shapes: `<rect>` (with `rx`/`ry`), `<circle>`, `<ellipse>`, `<line>`, `<polyline>`, `<polygon>`, `<path>`
- Full SVG 1.1 path syntax: M/L/H/V/C/S/Q/T/A/Z + lowercase relatives, with elliptical-arc decomposition into ≤4 cubic Béziers
- Transforms: `translate` / `rotate` / `scale` / `matrix` / `skewX` / `skewY`
- `viewBox` + all 10 `preserveAspectRatio` modes with Y-flip
- Presentation attrs + inline `style="..."` (semicolon-separated)
- Colors: hex (3/6/8-digit), `rgb()`/`rgba()`, 147 CSS named colors, `none`/`transparent`/`currentColor`
- Absolute length units: px / pt / pc / mm / cm / in
- Group inheritance cascade resolved at parse time
- Best-effort error policy: unsupported elements silently skipped; only XML parse failures surface as errors

**Phase 3a — Gradients**
- `<linearGradient>` rendered via PDF Type 2 (axial) shading patterns
- `<radialGradient>` rendered via PDF Type 3 (radial) shading patterns
- Multi-stop gradients use Type 3 stitching combining Type 2 exponential interpolations
- Supports `<stop>` (offset numeric/percent, stop-color, stop-opacity), `gradientUnits` (userSpaceOnUse + objectBoundingBox), `gradientTransform` (full matrix), `spreadMethod=pad`
- `fill="url(#id)"` and `stroke="url(#id)"` resolved at render time; missing refs fall back to no fill

**Phase 3b — Text**
- `<text>` and `<tspan>` with mixed content (CharData + `<tspan>` + CharData) and cursor-based positioning
- `dx`/`dy` offsets, absolute `x`/`y` override on `<tspan>`
- `text-anchor` (start / middle / end) with font-metric-based width measurement
- `font-family` / `font-size` / `font-weight` / `font-style` attributes
- Font matching: built-in heuristic mapping Standard 14 keywords (Arial/Helvetica → FontHelvetica, Times → FontTimesRoman, Courier → FontCourier + bold/italic variants); pluggable `SVGFontResolver` callback for embedded TTF fonts
- Gradient fills (Phase 3a) work on text via the same `/Pattern cs` mechanism

**Phase 3c — Image / defs / use / clipPath**
- `<image>` with `data:image/png;base64,...` and `data:image/jpeg;base64,...` inline (external URLs silently skipped); `preserveAspectRatio` honored
- `<defs>` / `<use>` / `<symbol>` — reusable elements with parse-end deep-clone resolution; forward references supported; cycle detection
- `<clipPath>` with `clipPathUnits` (userSpaceOnUse + objectBoundingBox), multi-child union; maps to PDF `W` / `W*` operators
- `clip-path="url(#id)"` presentation attribute on any shape/text/image

**Phase 3d — Mask / CSS / filter / marker**
- `<mask>` via PDF soft masks (Form XObject `/Group /S /Transparency` + ExtGState `/SMask`); supports `maskUnits` and `maskContentUnits`
- CSS `<style>` blocks with `.class` / `#id` / element selectors; specificity ordering (inline > id > class > type)
- `<filter>` with `feDropShadow` emulated as offset+alpha bbox duplicate (no blur — PDF has no native Gaussian blur and the library stays stdlib-only; other filter primitives silently skipped)
- `<marker>` (marker-start / marker-mid / marker-end) on line/polyline/polygon/path; `orient="auto"` rotation along path tangent; `refX`/`refY` anchor; markerUnits (strokeWidth + userSpaceOnUse)

### Added — infrastructure

- GitHub Actions CI workflow (`go build` + `go test` on Linux/Windows/macOS)
- Go Report Card badge in README
- `gofmt -s` applied across the entire codebase

### Fixed

- Type 3 stitching function `/Bounds` now strictly increasing — SVG allows duplicate `<stop offset>` values for sharp color transitions, but the PDF spec (§7.10.4) requires strictly-monotonic bounds. Duplicate offsets are now bumped by a 1e-6 epsilon, preserving visual intent while satisfying the spec. Acrobat previously refused to open documents with non-monotonic bounds
- SVG group opacity emits `/GSx gs` instead of `//GSx gs` — `ensureExtGState` returns names with a leading slash, so prepending another `/` produced a malformed PDF token that Acrobat rejected with a "document contains errors" warning. Affected SVGs with `<g opacity="..."> ` children (notably the Aspose logo's highlight-overlay group)

### Out of scope (future)

The following SVG features are deliberately not in v0.2.0 (low real-world frequency or require capabilities outside the stdlib-only constraint):

- `<textPath>` (text along a path), vertical text (`writing-mode`), `xml:space="preserve"`
- `em` / `ex` / `%` length units (require font / parent bbox context)
- `spreadMethod="reflect"` / `"repeat"` (requires PostScript function loops)
- CSS descendant / pseudo / attribute selectors, `<style>` `@media` / `@import`
- True Gaussian blur in `<filter>` (no software rasterizer in stdlib)
- SMIL animation (`<animate>`, `<animateTransform>`)
- External `href` in `<image>` (security + IO surface area)
- `data:image/svg+xml` (recursive parsing)

## [0.1.0] — 2026-05-21

Initial public release. Pure Go PDF library — no external dependencies, standard library only. Requires Go 1.24+. API shape mirrors Aspose.PDF for .NET where natural for migrants. Spec references follow ISO 32000-1 (PDF 1.7) and ISO 32000-2 (PDF 2.0).

### Added

- **Document lifecycle**
  - `Open` / `OpenStream` (with `ErrEncrypted` sentinel for password-protected files)
  - `OpenWithPassword` / `OpenStreamWithPassword` (tries password as both user and owner)
  - `Save` / `WriteTo` (implements `io.WriterTo`)
  - `NewDocument` / `NewDocumentFromFormat` for blank documents
  - `AddBlankPage` / `AddBlankPageFromFormat` / `InsertBlankPage*`
  - `(*Document).Pages()` / `Page(n)` / `PageCount()`
  - Predefined page formats: `PageFormatA3` / `PageFormatA4` / `PageFormatLetter` / `PageFormatLegal` with `.Landscape()` variant

- **Pages**
  - `Rotate` / `SetRotation` / `Reorder` / `Append` / `Split` / `Extract`
  - `RemoveUnusedObjects` (orphaned-object cleanup)
  - Page-box accessors (MediaBox, CropBox, TrimBox, BleedBox, ArtBox) with inheritance from page tree
  - Page labels (decimal, roman upper/lower, alphabetic upper/lower, prefix, start)

- **Metadata** — read and write `/Info` dictionary (Title, Author, Subject, Keywords, Creator, Producer, CreationDate, ModDate, plus arbitrary custom string entries)

- **Encryption** — Standard Security Handler
  - **AES-128** (default; V=4 R=4 `/CFM /AESV2` per ISO 32000-1 §7.6.3.2)
  - **AES-256** (V=5 R=6 `/CFM /AESV3` per ISO 32000-2 §7.6.4; PDF 2.0 header)
  - **RC4-128** (legacy V=2 R=3)
  - Permissions: print, copy, modify, annotate, form fill, accessibility, assembly, high-res print
  - Options-pattern API via `EncryptionOptions`; `SetPassword` / `SetPermissions` / `SetEncryption` / `Permissions` / `RemoveEncryption`
  - Encrypted-input parsing: decrypt-on-read for all three algorithms, with Algorithms 2/5/7 (V≤4) and 2.B (V=5) per spec

- **Text rendering**
  - `(*Page).AddText(text, style, rect)` with font selection, alignment (H/V), word wrap, line spacing, color, background fill, underline, strikethrough, rotation, behind-content mode
  - `AddTextWatermark` for all-page or selected-page watermarks
  - `TextStyle` struct with all rendering knobs
  - Standard 14 PDF fonts as package-level vars (`FontHelvetica`, `FontTimesBold`, etc.); `FindFont(name)` lookup
  - Embedded TTF fonts via `(*Document).LoadFont` / `LoadFontFromStream` with glyph subsetting and Identity-H (CID) encoding for full Unicode

- **Text extraction**
  - `(*Page).ExtractText()` — visual reading order text
  - `(*Page).ExtractTextWithLayout()` — structured `TextLine` / `TextFragment` with coordinates, font name and size, bold/italic detection, color, sub/superscript flags, character spacing
  - Standard 14 fonts (WinAnsi, MacRoman, Symbol, ZapfDingbats); ToUnicode CMap; Type0/CIDFont with Identity-H; `/Differences` arrays; Form XObjects recursion; marked content `/ActualText`

- **Image extraction**
  - `(*Page).ExtractImages()` and `(*Document).ExtractImages()` — full pixel data
  - `(*Page).ImageInfos()` / `(*Document).ImageInfos()` — metadata only (no decode)
  - `(*ImageInfo).Extract()` — lazy decode
  - Output: JPEG passthrough or PNG re-encode
  - Color spaces: DeviceRGB, DeviceGray, DeviceCMYK→RGB, Indexed (palette expansion), ICCBased
  - Soft masks (`/SMask`) as PNG alpha; inline images (BI/ID/EI); Form XObjects

- **Image manipulation**
  - `(*Page).AddImage(path, rect)` / `AddImageFromStream(r, rect)` — JPEG or PNG (magic-byte detected)
  - `ImageToDocument(path, opts...)` / `ImageToDocumentFromStream` — DPI-aware single-page conversion
  - `(*ImageInfo).Replace(path)` / `ReplaceFromStream(r)` — in-place data swap, position preserved
  - `(*ImageInfo).Remove()` — full removal (resource + content stream)
  - `(*Document).OptimizeImages(opts)` — DPI downscaling + opaque-PNG → JPEG conversion

- **AcroForm**
  - Field types: `TextBoxField`, `CheckboxField`, `RadioButtonField` (+ option), `ComboBoxField`, `ListBoxField`, `ButtonField`
  - Read existing fields: `Form().Fields()`, `Field(name)`, `HasField`
  - Fill: `SetValue(s)` — UTF-16BE-with-BOM for non-ASCII, Latin-1/PDFDocEncoding otherwise (ISO 32000-1 §7.9.2.2)
  - Programmatic construction: `AddTextField` / `AddCheckbox` / `AddRadioGroup` / `AddComboBox` / `AddListBox` / `AddPushButton`
  - Per-type structural mutators: `SetReadOnly` / `SetRequired` / `SetMaxLen` / `SetMultiline` / `SetPassword` / `SetEditable` / `SetMultiSelect` / `AddOption` / `RemoveOption`
  - `RemoveField` (full cleanup across `/AcroForm/Fields` and per-page `/Annots`)
  - Auto-sets `/AcroForm/NeedAppearances=true` so viewers regenerate cached `/AP`

- **Annotations** — 14 types
  - **Markup** — Link, Highlight, Underline, StrikeOut, Squiggly (with `/QuadPoints` per ISO 32000-1 §12.5.6.10)
  - **Actions** — `GoToURIAction`, `GoToAction`, `NamedAction`, `SubmitFormAction`, `ResetFormAction`, `JavaScriptAction` (constructor + parse)
  - **Drawing** — `SquareAnnotation`, `CircleAnnotation`, `LineAnnotation` (with 10 line-ending styles), `InkAnnotation` (with Catmull-Rom smoothing in /AP)
  - **Text-bearing** — `TextAnnotation` (sticky note, 8 icons), `FreeTextAnnotation` (callout/typewriter/cloudy-border intents, callout points, border effects), `StampAnnotation` (14 predefined names + custom image override)
  - **File attachment** — `FileAttachmentAnnotation` with MIME auto-detection, embedded-file streams via Filespec
  - **Redact** — `RedactAnnotation` mark mode + `(*Document).ApplyRedactions()` destructive content removal (text glyphs with TJ kerning preservation, image XObjects with even-odd clip, drawing paths)
  - Page-scoped collection API (`(*Page).Annotations()`); `/AP` appearance streams auto-generated for all drawing annotation types

- **Outlines (bookmarks)**
  - `(*Document).Outlines()` — root collection; recursive `OutlineItemCollection` tree (1:1 with Aspose.PDF for .NET)
  - All 8 explicit destination types per ISO 32000-1 §12.3.2.2: XYZ / Fit / FitH / FitV / FitR / FitB / FitBH / FitBV
  - `Action` attachment alongside `Destination` (per ISO 32000-1 §12.3.3 — viewers honor `/Dest` first)
  - Style attributes: `Bold` (`/F` bit 2), `Italic` (`/F` bit 1), `Color`, expand/collapse state (sign of `/Count`)
  - Lazy dict-backed reads with copy-on-mutate

- **Named destinations**
  - `(*Document).NamedDestinations()` — collection (Add/Get/Has/Remove/Count/Names/All/Clear) per ISO 32000-1 §12.3.2.3
  - `NamedDestination` — 9th `Destination` type wrapping a name reference, with lazy `Resolve()`
  - Reads both legacy `/Catalog/Dests` dict and modern `/Catalog/Names/Dests` name tree, with collision resolution (`/Names` wins)
  - Writes modern only with automatic legacy migration; sibling `/Names` subentries (JavaScript, EmbeddedFiles) preserved through round-trip
  - Forward references supported (resolve at call time, not bind time)

- **Tables**
  - `pdf.NewTable()` fluent builder; `Table` / `Row` / `Cell` with Aspose.PDF for .NET-parity naming (`BorderInfo`, `MarginInfo`, `ColumnWidths`, `RepeatingRowsCount`, `ColSpan`, `RowSpan`)
  - `(*Page).AddTable(t, rect) (pagesAdded int, err error)` — Rectangle-based rendering with auto-fit or explicit row heights
  - Per-cell borders (bitmask sides), padding, text style, alignment, background fill; inheritance chain: zero → table default → row default → cell override
  - **Multi-page overflow** with `SetOverflowMargins(top, bottom)` — auto-appends continuation pages; outer border per page
  - **Repeating header rows** via `SetRepeatingRowsCount(n)` — drawn on each continuation page
  - **Cell merging** via `Cell.SetColSpan(n)` / `SetRowSpan(n)` — implicit covered cells; rowspan groups are atomic across page breaks
  - **Image cells** via `Cell.SetImage(path)` / `SetImageFromStream(r)` — auto-fit interior width preserving aspect ratio; image wins over text
  - **Row-level styling** via `Row.SetBackground` / `SetTextStyle` / `SetBorder` / `SetMargin` — between table default and cell override in the chain
  - **Batch row construction** via `Table.AddRows([][]string)`
  - **Border edge de-duplication** per page (identical-style adjacent edges emit once)

- **Vector graphics**
  - `(*Page).DrawLine` / `DrawRectangle` / `DrawRoundedRectangle` / `DrawCircle` / `DrawEllipse` / `DrawPolyline` / `DrawPolygon` / `DrawPath`
  - `Path` fluent builder: `NewPath().MoveTo(x,y).LineTo(x,y).CurveTo(c1x,c1y,c2x,c2y,x,y).QuadTo(cx,cy,x,y).Arc(cx,cy,r,startAngle,sweepAngle).Close()` — arc decomposes into ≤4 cubic Beziers per the Goldapp formula
  - `LineStyle` (color, width, dash pattern, line caps, line joins, miter limit) + `ShapeStyle` (stroke + optional fill)
  - Alpha for stroke and fill via ExtGState (`/CA` + `/ca`)
  - PDF user-space coordinates (Y up, origin at page bottom-left)

- **Validate** — `Validate(inputPath)` returns `*ValidationReport` with structural-integrity checks: header, xref/trailer, all-objects-readable, page tree traversal, orphaned `/Pages` nodes, `/Page → /Parent` resolution, streams without `/Filter` not containing compressed data

- **Stream I/O** — every `Open*` accepts `io.Reader`; `(*Document).WriteTo(io.Writer)` implements `io.WriterTo`; encryption applies on `Save` / `WriteTo` regardless of source

### Project conventions

- MIT license (see `LICENSE`)
- SPDX-License-Identifier: MIT header in every `.go` file
- No external runtime dependencies — Go standard library only
- Beads-based issue/task tracking (`bd` CLI)
- All public API documented in `CLAUDE.md` and per-feature sections of `README.md`

[Unreleased]: https://github.com/aspose-pdf-foss/aspose-pdf-foss-for-go/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/aspose-pdf-foss/aspose-pdf-foss-for-go/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/aspose-pdf-foss/aspose-pdf-foss-for-go/releases/tag/v0.1.0
