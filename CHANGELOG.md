# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/aspose-pdf-foss/aspose-pdf-foss-for-go/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/aspose-pdf-foss/aspose-pdf-foss-for-go/releases/tag/v0.1.0
