# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- PDF → HTML export (phase 4: native no-background mode) — `HTMLSaveOptions.Mode = HTMLModeNative` drops the raster page background entirely: each page's graphics are exported as **one inline SVG layer** emitted by a vector backend riding the built-in content-stream renderer. Path fills and strokes become `<path>` elements with **true Bézier curves** and native stroke attributes (width, caps, joins, dash); raster pictures become SVG `<image>` elements carrying the PDF's **original JPEG/PNG bytes** placed by the CTM; W/W* and text-mode clips become chained `<clipPath>` definitions; constant alpha and all 15 PDF blend modes map to `fill-/stroke-opacity` and CSS `mix-blend-mode`. Content SVG cannot express — shadings, tiling patterns, soft masks, stencil masks, transparency-group compositing — **degrades locally**: just that operation is rendered through the ordinary raster pipeline and embedded as a small positioned PNG at the correct z-order, so one exotic object never rasterizes the page. Text stays the visible WOFF-font span layer; annotation appearances (form widgets, stamps, highlights) render into the SVG. Verified pixel-faithful in headless Chrome across the feature showcase. This completes the PDF→HTML epic (`pdf-go-rfom.3`)
- PDF → HTML export (phase 3: WOFF font embedding) — in `HTMLModeText` the document's embedded font programs are re-wrapped as **WOFF1** and emitted as `@font-face` data: URLs, so the visible text layer renders in the document's real faces instead of metric substitutes (`HTMLSaveOptions.NoFontEmbedding` opts out). Pure Go: a lenient sfnt splitter, per-table zlib WOFF encoder, and synthesis of the tables browsers require but PDF subsets drop — a format-4 cmap built from `/ToUnicode` ∘ `/CIDToGIDMap` (composite fonts carry no usable cmap), plus minimal `name`/`OS/2`/`post`. Covers `/FontFile2` TrueType (simple + CIDFontType2) and `/FontFile3 /Subtype /OpenType` CFF-sfnt (OTTO flavor preserved); bare CFF and Type1 keep the substitute + width-fitting fallback. Embedded-face spans skip width fitting (advances match by construction; PDF character spacing maps to CSS `letter-spacing`). Validated with fontTools strict parsing and full cmap coverage of the exported text. (`pdf-go-rfom.2`)
- PDF → HTML export (phase 2: visible-text mode) — `HTMLSaveOptions.Mode = HTMLModeText` renders each page's background **without glyphs** (graphics, images and text-clip effects only) and draws the text as **visible HTML spans**: real colour, size, bold/italic, a metric-matched family stack (Arial / Times New Roman / Courier New), width-fitted to the PDF layout via `letter-spacing` for small metric gaps and `transform: scaleX` for larger ones — text stays crisp at any zoom, is accessible, and the file is roughly half the faithful-mode size. Both modes also gain: link annotations exported as clickable `<a>` overlays (`/URI` to the outside, `/GoTo`/page-ref `/Dest` as `#pageN` jumps), a `Pages` option for exporting a subset (anchors keep source numbers), and `loading="lazy"` on page backgrounds so large documents open fast. Next phases: WOFF font embedding, then a fully HTML-native no-background mode (inline SVG vectors + `<img>`). (`pdf-go-rfom.1`)
- PDF → HTML export (phase 1) — `(*Document).SaveHTML(path, opts)` / `WriteHTML(w, opts)` convert the document to **one self-contained HTML file**: each page is rendered by the built-in rasterizer (pixel-identical to `RenderPNG`) and embedded as a base64 PNG, with a transparent, precisely positioned text layer on top — so the HTML looks exactly like the PDF while its text is selectable, copyable and searchable (Ctrl+F). No external assets, no JavaScript. `HTMLSaveOptions{DPI, Title}`. A visible-text ("true HTML") mode, font embedding and an SVG background are planned as later phases; the renderer already gained the `suppressText` primitive for the text-less background render. Mirrors the intent of Aspose.PDF for .NET's `Document.Save(SaveFormat.Html)`. (`pdf-go-rfom`)
- Transparency-group compositing (render) — Form XObjects with `/Group /S /Transparency` drawn under a group-level alpha, blend mode or soft mask are now rendered off-screen and composited as a single flattened object (ISO 32000-1 §11.4.7), fixing the classic double-darkening of overlaps inside a semi-transparent group; knockout groups (`/K true`) make later vector elements replace earlier ones within their coverage. Opaque Normal-mode groups keep the fast inline path. Completes the renderer's transparency model (constant alpha + 16 blend modes + soft masks + groups). (`pdf-go-rom`)

### Fixed

- `Document.Save` no longer ignores the output file's `Close` error — a failed `Close` on a freshly written file can mean silently lost data (OS write buffers not flushed); the `WriteTo` error still takes precedence when both fail. Found by adopting `golangci-lint` in CI (which also drove a dead-code sweep and explicit `_ =` markers wherever ignoring an error is deliberate).

## [0.4.0] — 2026-07-02

Building on the v0.3.0 renderer, this release adds an enterprise and compliance layer, plus a document-generation and international-text stack — all still pure Go, standard library only, MIT-licensed. Headlines: **digital signatures** (sign/verify, PAdES, DocMDP certification, RFC 3161 timestamps, multiple signatures, and signing encrypted documents); the **PDF/A** + **PDF/UA** compliance stack (validate + convert across PDF/A-1/2/3 a/b with automatic font embedding and a pure-Go sRGB OutputIntent; a PDF/UA validator; and a **Tagged-PDF authoring** toolkit for accessible output); a **flow / document-generator** layout model (paragraphs, headings, tables, lists, floating boxes, multi-column, text flow-around, auto-pagination, optional auto-tagging); **right-to-left text** (a pure-Go Unicode BiDi engine with Arabic contextual shaping); a complete **AcroForm data-interchange** story (typed JSON, FDF, XFDF) with the extra field types (Number/Date/Password/RichText/FileSelect); **structural text extraction** (`ParagraphAbsorber`); and a batch of production helpers — reusable Form XObjects, tiling patterns, inline images, optional-content layers, document-level embedded files, linearized fast-web-view output, whole-document grayscale, and a PDF-page stamp. No external dependencies were added.

### Added

- Structural (paragraph) extraction — `(*Page).Paragraphs()` / `(*Document).Paragraphs()` group a page's text into columns (`MarkupSection`) and paragraphs (`MarkupParagraph{Text, Rectangle, Lines}`), rather than a flat string. Fragments are clustered into columns by a horizontal occupancy histogram (a wide vertical gutter splits columns), then each column's lines are grouped into paragraphs by baseline gaps and font-size changes. Built on the existing layout pipeline; heuristic but recovers prose paragraph/column structure well. Mirrors Aspose.PDF for .NET's `ParagraphAbsorber`. (`pdf-go-14la`)
- PDF-page stamp — `NewPdfPageStamp(srcDoc, srcPageNum)` creates a `PdfPageStamp` that overlays (or underlays) a page from another PDF document, drawn via `(*Page/*Document).AddStamp` like the other stamps. The source page is imported once as a Form XObject (reusing the imposition import machinery) and drawn into the stamp's `Rect` scaled to fit while preserving aspect, positioned by `HAlign`/`VAlign`, with `Opacity`, centre-pivot `RotateAngle`, and `Background`. Mirrors Aspose.PDF for .NET's `PdfPageStamp`. (`pdf-go-o7r2`)
- Text-extraction mode & search-in-annotations — `ExtractText` gains an optional `TextExtractOptions{Mode}` selecting `TextExtractReading` (visual order, default) or `TextExtractRaw` (content-stream emission order), mirroring Aspose's `TextFormattingMode`; and `SearchOptions.SearchInAnnotations` extends `SearchText` to also match annotation `/Contents` text (sticky notes, free text, markup), each match carrying the annotation's rectangle. (`pdf-go-h9b2`)
- Extra AcroForm field types — `(*Form).AddPasswordField`, `AddFileSelectField`, `AddRichTextField`, `AddNumberField(…, NumberFormatOptions)` and `AddDateField(…, format)` create the typed variants `PasswordBoxField` / `FileSelectBoxField` / `RichTextBoxField` / `NumberField` / `DateField`. Each is a text field distinguished by a `/Ff` flag (Password/FileSelect/RichText) or a JavaScript format action (Number → `AFNumber_Format`, Date → `AFDate_FormatEx`), so the type round-trips through Save+Open; each embeds `TextBoxField` and inherits its full API, plus `RichTextBoxField.SetRichValue`/`RichValue` and `DateField.Format`. The typed JSON/FDF/XFDF form-data export/import handles them as text. Mirrors Aspose.PDF for .NET's `NumberField`/`DateField`/`PasswordBoxField`/`RichTextBoxField`/`FileSelectBoxField` (BarcodeField, which needs a barcode renderer, is out of scope). (`pdf-go-pnj3`)
- Arabic contextual shaping (RTL phase 2) — Arabic text now renders with connected letterforms. Before BiDi reordering, `AddText` maps each Arabic letter to its Unicode Presentation Forms-B glyph (isolated / initial / medial / final, chosen by joining context) and forms the mandatory lam-alef ligatures; combining vowel marks are transparent to joining. Pure-Go, in the layout layer, so any font that covers Presentation Forms-B (e.g. the bundled DejaVu Sans) renders proper Arabic — no encoder or renderer change. Full OpenType GSUB/GPOS shaping and mark positioning remain phase 3. (`pdf-go-emak`)
- RTL / bidirectional text (phase 1) — `AddText` now lays out right-to-left text. When a string is `TextStyle.RTL` or contains any strong Hebrew/Arabic character, each wrapped line is reordered into visual order by a pure-Go Unicode Bidi Algorithm (UAX #9 — weak/neutral/implicit resolution, L2 reordering, paired-punctuation mirroring), and an RTL base paragraph right-aligns by default. Numbers and embedded Latin stay left-to-right in their correct positions. It runs in the layout layer, so the encoder and renderer are unchanged. Hebrew is complete; Arabic is correctly ordered but not yet contextually shaped (phase 2). (`pdf-go-emak`)
- Form data round-trip (FDF & XFDF) — `(*Form).ExportFDF`/`WriteFDF`/`ImportFDF`/`ReadFDF` and `ExportXFDF`/`WriteXFDF`/`ImportXFDF`/`ReadXFDF` add the Acrobat-interoperable form-data formats alongside JSON, over the same name→value model. FDF (ISO 32000-1 §12.7.7) is a PDF-syntax `%FDF-1.2 … /FDF /Fields` file (import scans indirect objects and descends `/Kids`); XFDF (ISO 19444-1) is XML (`<xfdf><fields><field name><value>`, nested fields flattened to dotted names). Import returns the count applied and skips unknown names. Mirrors Aspose.PDF for .NET's `Facades.Form.Export/Import{Fdf,Xfdf}`. (`pdf-go-cx6r`)
- Form data round-trip (typed JSON) — `(*Form).ExportJSON`/`WriteJSON` serialise every field value to a typed JSON document keyed by full field name (`{"name": {"type": "...", "value": ...}}`; text/radio/combobox → string, checkbox → bool, listbox → array of strings; push buttons skipped), and `(*Form).ImportJSON`/`ReadJSON` apply such a document back, returning the count applied. Import dispatches on the target field's concrete type and reuses the existing value setters (so widget appearances regenerate); unknown names and non-applying values are skipped. `JSONExportOptions{Indent, OmitEmpty}`. The data/document separation enables template-fill and form-data interchange workflows, and is the foundation for FDF/XFDF. Mirrors Aspose.PDF for .NET's `Document.Form.ExportToJson`/`ImportFromJson`. (`pdf-go-lw0s`)
- Column break & floating-box refinements — `(*Flow).AddColumnBreak()` forces the following content into the next column; `FloatingBox.SetSpacing(gap)` controls the inter-element gap (0 to centre a single paragraph against symmetric padding); and a `FloatingBox` border now honours `BorderInfo.Sides`, so `BorderSideLeft` renders a block-quote rule instead of a full outline. (`pdf-go-m9kt`)
- Text flow-around — `(*Flow).AddFloatBox(box, FloatLeft|FloatRight, width)` floats a box against a column edge and wraps the following paragraphs around it line by line, returning to full width once the text passes the box. Completes Tier 3 of the flow model; mirrors Aspose.PDF for .NET's floated `FloatingBox`. (`pdf-go-m9kt`)
- Multi-column flow — `FlowOptions.Columns` (and `ColumnGap`) lay a flow out in two or more columns: content fills each column top-to-bottom, then moves to the next column, then the next page (newsletters, reports). Works with auto-tagging and floating boxes. Tier 3 of the flow model (text flow-around and keep-with-next remain). (`pdf-go-m9kt`)
- Floating boxes — `FloatingBox` is a positioned content container (border/background/padding + its own paragraphs/headings/images/lists). `(*Page).AddFloatingBox(box, rect)` places it absolutely; `(*Flow).AddFloatingBox(box)` inserts it into a flow at its measured height. When the document is tagged the box becomes a `/Div` and its frame an artifact, so a flow with boxes still validates as PDF/UA. Tier 2 of the flow model; mirrors Aspose.PDF for .NET's `FloatingBox` (text flow-around and columns are Tier 3). (`pdf-go-m9kt`)
- Flow layout / document generator — `(*Document).NewFlow(FlowOptions)` returns a `*Flow` that lays content out top-to-bottom and paginates automatically: chainable `AddParagraph`/`AddHeading`/`AddImage`/`AddTable`/`AddList`/`AddSpacer`, then `Render()` flows it into the document, appending pages as needed (paragraphs split across pages line-by-line; tables fall back to their own pagination). The flow counterpart to the Rectangle-based API — an additive layer that coexists with it. With `FlowOptions.Tagged` set, every element is auto-tagged so the output passes `ValidatePDFUA` — a one-call accessible report generator. Mirrors the intent of Aspose.PDF for .NET's generator / `Page.Paragraphs` flow model. (`pdf-go-m9kt`)
- Tagged lists & artifacts — `(*Page).AddTaggedList(tc, parent, items, style, rect, ordered)` draws a bulleted or numbered list and builds its `/L → /LI → /Lbl`+`/LBody` structure; `(*Page).TagArtifact(draw)` brackets decoration (headers/footers, page numbers, backgrounds) as an `/Artifact` so it is excluded from the structure tree. Together with `TagContent`/`AddTaggedTable`, the tagged-content authoring toolkit now covers paragraphs, headings, figures, tables, lists and artifacts — enough to author a fully PDF/UA-conformant document. (`pdf-go-y5z`)
- Tagged tables — `(*Page).AddTaggedTable(tc, parent, table, rect)` renders a table and simultaneously builds its accessible structure: a `/Table` element with a `/TR` per row and a `/TH`/`/TD` per cell (the repeating header rows become `/TH`), each cell's content tagged as marked content and the backgrounds/borders bracketed as `/Artifact`. Paginates, validates as PDF/UA and round-trips — the first auto-tagging helper on top of the tagged-content authoring API. (`pdf-go-y5z`)
- Grayscale conversion — `(*Document).ConvertToGrayscale()` converts a document to grayscale in place, mapping every colour to its luminance grey. Covers content-stream device colours (RGB/Gray/CMYK and ICCBased via `rg`/`RG`/`k`/`K`/`cs`/`sc`/`scn`), raster images (re-encoded as DeviceGray, soft masks preserved), axial/radial shadings and shading/tiling patterns, and annotation colours + appearance streams — recursing through Form XObjects. Geometry, text and layout are untouched; verified by the rendered colorfulness dropping to zero. Separation/DeviceN, Indexed and type 0/4 shading functions are best-effort. Mirrors Aspose.PDF for .NET's `RgbToDeviceGrayConversionStrategy`. (`pdf-go-6gcy`)
- PDF/A accessible ("a") levels — `ValidatePDFA`/`ConvertToPDFA` now accept `PDFA1A`/`PDFA2A`/`PDFA3A` in addition to the basic `*B` levels. The "a" levels add the Tagged-PDF requirement: the validator checks the structure tree, `/MarkInfo /Marked` and `/Lang` (and matches `pdfaid:conformance` against the requested letter). A document authored with `TaggedContent` (above) and then converted with `PDFA1A` is fully conformant; converting an untagged document to an "a" level reports the missing tagging. This completes the PDF/A conformance matrix (1b/2b/3b + 1a/2a/3a). (`pdf-go-s2n`)
- Tagged PDF authoring (phase 2) — `(*Document).TaggedContent()` builds a logical structure tree as the drawing API emits content, producing accessible PDF/UA-conformant output. `SetTitle`/`SetLanguage` set the required catalog marks; `(*Page).TagContent(parent, structType, draw)` brackets a drawing block in a `/<type> <</MCID n>> BDC … EMC` marked-content sequence and adds a `/StructElem` (wiring up `/ParentTree` + `/StructParents`); `StructElement.AddChild` builds grouping elements (Table→TR→TD, L→LI, …) and `SetAlt`/`SetActualText`/`SetLanguage` add accessibility metadata. A document authored this way passes `ValidatePDFUA` and round-trips. This is the write-side counterpart to the PDF/UA validator and the foundation for the PDF/A "a" (accessible) levels. Mirrors the intent of Aspose.PDF for .NET's `Document.TaggedContent`/`ITaggedContent`. (`pdf-go-y5z`)
- PDF/UA validator (phase 1) — `(*Document).ValidatePDFUA()` returns a `*PDFUAValidationReport` checking the PDF/UA-1 (ISO 14289-1, accessibility) structural prerequisites: the document is marked as Tagged PDF and carries a structure tree (`/StructTreeRoot` + `/ParentTree`), a default language (`/Lang`), a title that is displayed rather than the file name (`/ViewerPreferences/DisplayDocTitle`), alternate text on every `/Figure`/`/Formula`, and accessibility not blocked by encryption. Read-only — the tagged-content authoring side is the next phase (it also unlocks the PDF/A "a" levels). Recognises real tagged PDFs. Mirrors the intent of Aspose.PDF for .NET's `Document.Validate(PdfFormat.PDF_UA_1)`. (`pdf-go-y5z`)
- PDF/A validator (phase 1) — `(*Document).ValidatePDFA(format)` returns a `*PDFAValidationReport` listing archival-conformance violations for the basic levels `PDFA1B` / `PDFA2B` / `PDFA3B` (ISO 19005-1/2/3): missing/mismatched XMP `pdfaid`, non-embedded fonts, encryption, JavaScript/Launch actions, device colour without an ICC OutputIntent, transparency (PDF/A-1), annotation flags/appearances, a compressed `/Metadata` stream, LZW (PDF/A-1) and embedded files (PDF/A-1). The "a" (tagged) levels are out of scope (need the structure tree). Mirrors the intent of Aspose.PDF for .NET's `Document.Validate(PdfFormat)`. (`pdf-go-s2n`)
- PDF/A conformance writer (phase 2) — `(*Document).ConvertToPDFA(format)` adjusts the document toward a PDF/A "b" level in place and returns the remaining-violations report: removes encryption, JavaScript/Launch actions and (for PDF/A-1) file attachments; sets annotation print flags; **auto-embeds non-embedded simple fonts** (Standard-14 and other single-byte Type1/TrueType) using the bundled metric-compatible substitutes — the original encoding and metrics are preserved, so text still extracts and renders; adds an sRGB ICC **OutputIntent** (the profile is generated in pure Go — no bundled `.icc`); and writes a `pdfaid` XMP packet. A document built with this library's `AddText` (Standard-14 fonts) now converts to fully conformant in one call. `Symbol`/`ZapfDingbats`, composite fonts and PDF/A-1 transparency remain the caller's responsibility (surfaced in the report). Mirrors the intent of Aspose.PDF for .NET's `Document.Convert(PdfFormat)`. (`pdf-go-s2n`)
- Linearization / fast web view — `(*Document).SaveLinearized(path)` / `WriteToLinearized(w)` write a linearized PDF (ISO 32000-1 Annex F): the first page's objects and a primary hint table sit at the front of the file so a viewer can render page 1 before the whole file downloads. The result is an ordinary PDF that any reader opens normally; object assembly was refactored into a shared `assemble()` step used by both the normal and linearized writers. Validated against qpdf 12.3.2 `check_linearization` for documents without cross-page shared objects; documents that share fonts/resources across pages still produce valid, readable, `is_linearized`-true output (Acrobat "Fast Web View: Yes") but their shared-resource hint accuracy is still being refined. Encryption and signing cannot be combined with linearization. (`pdf-go-h7r`)
- Inline images (write) — `(*Page).AddInlineImage(path, rect)` / `AddInlineImageFromStream(r, rect)` draw a small PNG/JPEG directly into the page content stream as an inline image (`BI … ID … EI`, ISO 32000-1 §8.9.7) rather than an Image XObject — for tiny one-off pictures (icons, rules). Both formats are decoded to 8-bpc samples, Flate-compressed and ASCIIHex-wrapped (`/F [/AHx /Fl]`) so the `EI` boundary is unambiguous; PNG transparency is flattened over white (inline images cannot carry a soft mask). `AddImage` (an XObject) remains the recommended default for anything larger. (`pdf-go-fts`)
- Validate — flag image XObjects missing the `/ColorSpace` or `/BitsPerComponent` required by ISO 32000-1 §8.9.5 (Acrobat draws such images blank; we infer both and render them). Image masks and JPXDecode images are exempt. Diagnostics only — rendering is unchanged. (`pdf-go-82e`)
- Tiling patterns — `(*Document).CreateTilingPattern(width, height)` returns a `TilingPattern` whose `Canvas()` is a drawing surface for the cell (the full `*Page` API); fill any shape with the repeating cell by setting `ShapeStyle.FillTiling`. `(*TilingPattern).SetStep(xstep, ystep)` adjusts the tile spacing (gaps/overlap). The PatternType 1 (colored) pattern is built once and registered on the page `/Resources/Pattern` automatically. Complements the existing gradient (shading-pattern) fills; mirrors Aspose.PDF for .NET's colored tiling patterns. Also fixes a draw no-op where a fill supplied only via `FillTiling` was not recognized. (`pdf-go-ctv`)
- Reusable Form XObjects — `(*Document).CreateForm(width, height)` returns an `XForm` whose `Canvas()` is a drawing surface (the full `*Page` API: `AddText`, `Draw*`, `AddImage`, `AddTable`, …); `(*Page).AddForm(form, rect)` places it so its box maps onto `rect`, building the `/Form` XObject once and reusing it for every placement — one shared content stream behind many `Do` invocations across pages. `(*Page).Forms()` lists a page's existing Form XObjects (re-placeable on any page), and `(*Document).ImportForm` copies a form with its whole resource graph into another document. Useful for page templates, shared headers/footers and watermarks. Mirrors Aspose.PDF for .NET's `XForm`. (`pdf-go-2tj`)
- Optional content / layers authoring — `(*Document).AddLayer(name)` creates an OCG layer (registered in `/Catalog/OCProperties`), `(*Page).BeginLayer(layer)` / `EndLayer()` bracket the page content that belongs to it, and `Layer.SetVisible(bool)` sets its default visibility; `(*Document).Layers()` lists them and `Layer.Name`/`SetName`/`IsVisible` inspect/edit. The built-in renderer already honored OCG visibility (hidden layers are not drawn), and layers round-trip with names + visibility. Mirrors Aspose.PDF for .NET's `Document.Layers`. (`pdf-go-83h1`)
- Document-level embedded files — `(*Document).EmbeddedFiles()` attaches arbitrary companion files to a PDF via the `/Names/EmbeddedFiles` name tree: `Add(path)` / `AddFromStream(name, r)` to embed (MIME detected from the extension), `Get` / `Names` / `Count` / `Has` / `All` / `Remove` / `Clear` to manage, and per-file `Data` / `Save` / `WriteTo` / `MIMEType` / `Description` / `Size` to read back. Coexists with named destinations and JavaScript under `/Catalog/Names`, and shares the `/EmbeddedFile` + `/Filespec` machinery with `FileAttachmentAnnotation` (page-pinned attachments). Mirrors Aspose.PDF for .NET's `Document.EmbeddedFiles`. (`pdf-go-p1d`)
- Change password — `(*Document).ChangePassword(newUserPassword, newOwnerPassword)` re-encrypts an open (decrypted) document with new passwords on the next Save, keeping the current encryption algorithm (RC4-128 / AES-128 / AES-256) and permissions; an empty owner password defaults to the user password. Errors on a plaintext document. Mirrors Aspose.PDF for .NET's `Document.ChangePasswords`. (`pdf-go-klsr`)
- Font loading by family name — `(*Document).LoadFontByName(family, bold, italic)` resolves a font through the `FontRepository` (registered folders/files via `AddFontFolder`/`AddFontFile`, then the OS font directories) and embeds the matched face, returning a `Font` for `TextStyle.Font` — the by-name counterpart to `LoadFont(path)`. The literal family is tried before the Standard-14 alias expansion (so "DejaVu Sans" embeds DejaVu, not Arial), and a `.ttc` collection sub-font is re-wrapped as a standalone sfnt before embedding. Mirrors Aspose.PDF for .NET's `FontRepository.FindFont`. (`pdf-go-ha1`)
- Digital signatures (sign + verify) — `(*Document).Sign(SignOptions)` adds a PKCS#7-detached signature (`adbe.pkcs7.detached`, SHA-256, RSA or ECDSA) covering the whole file, applied on Save/WriteTo; `(*Document).VerifySignatures()` verifies every signature and reports `Valid`/`IntegrityOK`/`CoversWholeDocument`/signer `Certificate`. Signatures are invisible by default, or **visible** via `SignOptions.Visible` + `Rect` + `Page`, which draws a "Digitally signed by …" appearance block (customizable through `SignatureAppearance`, mirroring Aspose's `SignatureCustomAppearance`). `SignOptions.PAdES` produces an ETSI.CAdES.detached (PAdES baseline) signature with the ESS signing-certificate-v2 attribute. `SignOptions.Certify` (`CertifyNoChanges`/`CertifyFillForms`/`CertifyAnnotations`) produces a certification (DocMDP) signature recording which later changes are permitted. `SignOptions.TimestampURL` fetches an RFC 3161 trusted timestamp from a TSA and embeds it as the signature-time-stamp unsigned attribute, anchoring the signing time to a trusted clock (validated against DigiCert's TSA; pyHanko reports `TIMESTAMP_TOKEN<INTACT:TRUSTED>`). **Multiple signatures** are supported: signing an already-signed document writes an incremental update (original bytes preserved verbatim + an appended revision with a `/Prev` xref) so earlier signatures stay valid — auto-enabled when a signature is present, or forced via `SignOptions.Incremental` (validated with pyHanko: first signature `ENTIRE_REVISION` + `ACCEPTABLE_MODIFICATIONS`, second `ENTIRE_FILE`/`UNTOUCHED`). **Encrypted documents** can be signed too (RC4-128 / AES-128 / AES-256): the signature is applied as an incremental update whose appended objects are encrypted with the document's own scheme, while the signature `/Contents`/`/ByteRange` stay plaintext per ISO 32000-1 §7.6.2 (validated with pyHanko: `INTACT:TRUSTED,UNTOUCHED`, `ENTIRE_FILE` for all three algorithms). The PKCS#7/CMS container is hand-rolled on `encoding/asn1` (no external dependency); signed output validates in OpenSSL and pyHanko (`INTACT:TRUSTED,UNTOUCHED`, `ENTIRE_FILE` coverage, `docmdp_ok`). Keys are supplied as `crypto.Signer` + `*x509.Certificate`, so no `.p12`/private-key file is needed (and none is committed). Mirrors the intent of Aspose.PDF for .NET's `PdfFileSignature`. Out of scope for now: LTV (DSS/VRI). (`pdf-go-bm9`)
- OpenType-CFF (`.otf`) font embedding — `Document.LoadFont`/`LoadFontFromStream` now accept CFF-based OpenType fonts (previously rejected), embedding them as a `CIDFontType0` descendant with an OpenType `/FontFile3` (the TrueType path is unchanged: `CIDFontType2` + `/FontFile2`). Identity-H glyph IDs map directly to the non-CID-keyed CFF's glyphs (ISO 32000-1 §9.7.4.2); `/ToUnicode` round-trips for text extraction. Verified rendering both with the built-in renderer and MuPDF. CID-keyed CFF and CFF subsetting remain out of scope. (`pdf-go-4sg`)
- Page imposition — `(*Document).NUp(NUpOptions)` lays multiple source pages onto larger sheets in a Rows×Cols grid (fit-inside + centered, optional borders/margins/gutter), and `(*Document).Booklet(BookletOptions)` imposes pages two-up reordered for saddle-stitch binding (padded to a multiple of 4). Both return a new `*Document` (receiver untouched) built from Form XObjects, so the output is an ordinary PDF. A production helper (not an ISO 32000 feature) mirroring the intent of Aspose.PDF for .NET's `PdfFileEditor.MakeNUp`/`MakeBooklet`, adapted to this library's `Document`-method API.
- Table of contents generation — `(*Document).GenerateTOC(opts...)` builds a TOC from the outline (bookmark) tree and inserts it as new page(s) at the front (nesting → indent level; page numbers and clickable GoTo links reflect the post-insertion order), and `(*Page).AddTOC(entries, rect, opts...)` renders a supplied entry list into a region with overflow auto-pagination. `TOCEntry` (with an optional `Label` override for logical page labels) / `TOCOptions` control the heading, per-level indent, dotted leaders, page numbers, and links. The feature-showcase Contents page is now rendered through `Page.AddTOC`. Loosely mirrors Aspose.PDF for .NET's `TocInfo`.
- Document-level JavaScript + open action — `(*Document).JavaScript()` exposes the `/Catalog/Names/JavaScript` named-script collection (`Add`/`Get`/`Has`/`Remove`/`Names`/`Count`/`Clear`), and `(*Document).SetOpenAction(Action)` / `OpenAction()` / `RemoveOpenAction()` set the action run when the document opens (GoTo, JavaScript, Named, …). Both round-trip through Save and coexist with named destinations under `/Catalog/Names`. Mirrors Aspose.PDF for .NET's `Document.JavaScript` and `Document.OpenAction`. (Document-level JavaScript executes in the recipient's viewer on open — embed only audited scripts.)
- Polygon / Polyline / Caret annotations — `NewPolygonAnnotation(page, vertices)` (closed, fillable), `NewPolylineAnnotation(page, vertices)` (open, with start/end line endings), and `NewCaretAnnotation(page, rect)` (insertion marker, `SetSymbol(CaretSymbol)`). Polygon/Polyline share `Vertices()/SetVertices`, `InteriorColor()/SetInteriorColor`, and the full border surface; all three synthesize `/AP/N` on every setter so they render in any viewer and round-trip through Save+Open. Mirrors Aspose.PDF for .NET's `PolygonAnnotation` / `PolylineAnnotation` / `CaretAnnotation`.
- Page-label authoring — `(*Document).SetPageLabels([]PageLabelRange)` installs the `/PageLabels` number tree (numbering style, prefix, start value per range) and `ClearPageLabels()` removes it; round-trips through Save and is read back by `(*Page).Label()`. Mirrors Aspose.PDF for .NET's `Document.PageLabels`.
- Search in a region — `SearchOptions.Rectangle` (a `*Rectangle`) limits `SearchText` results to matches whose bounding box intersects the region (PDF user space, per page). Mirrors Aspose.PDF for .NET's `TextSearchOptions.Rectangle`.
- Text replace — `(*Document).ReplaceText(old, replacement, opts...)` / `(*Page).ReplaceText(...)` find-and-replace, returning the number of replacements. Matching mirrors `SearchText` (literal, `ReplaceOptions{CaseInsensitive, Regex}`). The matched glyphs are removed and the replacement is redrawn at the same baseline/size/colour in a metric-compatible Standard-14 face chosen from the original's family/style, so any replacement text renders even over an embedded subset font (no line re-flow). Mirrors the find-and-replace idiom of Aspose.PDF for .NET's `TextFragmentAbsorber` + `TextFragment.Text`. Text extraction now also starts a new fragment on a large backward X jump, keeping reading order correct for out-of-order content.
- Stamps — `TextStamp`, `ImageStamp`, and `PageNumberStamp` overlay (or underlay) content on pages, applied with `(*Page).AddStamp` / `(*Document).AddStamp`. Mirrors Aspose.PDF for .NET's `Aspose.Pdf.Stamp` family: shared `Rect` (zero = whole page), `HAlign`/`VAlign`, `Opacity`, `RotateAngle` (rotates about the rect centre), and `Background` (draw behind page content). `PageNumberStamp` formats `{0}` (current) / `{1}` (total) with a `StartingNumber`, rendering the correct number per page — convenient for headers/footers and watermarks.

### Changed

- A text field carrying a Password / FileSelect / RichText flag, or a Number / Date format action, now reads back as its specific typed field (`PasswordBoxField` / `FileSelectBoxField` / `RichTextBoxField` / `NumberField` / `DateField`) instead of `TextBoxField`. Each embeds `TextBoxField`, so its methods still apply; code that type-asserts `.(*TextBoxField)` on such a field should use the specific type (or `FieldType`).

## [0.3.0] — 2026-06-16

The headline of this release is a complete, dependency-free **page renderer**: pages rasterize to PNG/JPEG/GIF/BMP and single- or multi-page TIFF, covering vector graphics, images, text, shadings, patterns, transparency, and annotation appearances. Months of visual testing against a real-world PDF corpus drove a large batch of parsing/rendering correctness and robustness fixes (see **Fixed**). No breaking changes since v0.2.0 beyond the two API renames noted in **Changed**.

### Added

- Page rendering to raster images (phased pure-Go rasterizer, umbrella `pdf-go-61r`). `(*Page).RenderImage(RenderOptions) (image.Image, error)` rasterizes a page at a chosen DPI (default 150) over its CropBox; `RenderPNG`/`RenderJPEG`/`RenderGIF`/`RenderBMP` encode it; `(*Document).RenderImage(pageNum, …)` renders by number. Aspose-style `Resolution` + `PngDevice`/`JpegDevice`/`GifDevice`/`BmpDevice`/`TiffDevice` with `Process(page, w)`. The renderer is dependency-free (own anti-aliased rasterizer + stroker + image/TIFF encoders). **P1** covers vector graphics (paths, fills, strokes, Gray/RGB/CMYK colour, CTM); **P2** adds Image XObjects (with `/SMask` alpha) and Form XObject recursion; **P3** adds text for embedded TrueType fonts (own `glyf` outline decoder + text-object state machine); **P4** renders non-embedded Standard-14 fonts from bundled metric-compatible families (Arimo/Tinos/Cousine/Carlito, SIL OFL, Latin-subset) so layout is preserved and serif/mono/bold/italic are distinct, with a `FontRepository` (`AddFontFolder`/`AddFontFile`/`AddSystemFonts`) to use exact installed fonts instead (including `.ttc` collections and `.otf`/CFF); **P5** adds clipping (`W`/`W*`), constant alpha, axial/radial shadings + shading patterns, tiling patterns, soft masks, blend modes (separable + non-separable), Separation/DeviceN colour with Type 4 PostScript tint functions, and Optional-Content visibility; **P6** adds stroke quality (caps/joins/dash) and a bounding-box performance pass. The renderer also paints annotation appearance streams (AcroForm widgets, stamps, highlights, free text, …) and honours text rendering modes (including the glyph-clip modes 4–7). Unsupported operators are skipped, so any page still produces an image.
- Embedded font formats for rendering — classic **Type1** (`/FontFile`, in-house eexec + charstring interpreter), **CFF / Type2** charstrings (`/FontFile3`: Type1C simple and CID-keyed, plus OpenType-CFF), and **Type3** fonts (glyphs as `/CharProcs` content streams). Together with the TrueType decoder this covers every embedded font format real PDFs use.
- Image codecs for rendering and extraction, all pure-Go: **CCITTFaxDecode** (Group 4 and Group 3 1-D fax), **JBIG2** bilevel scans (`/JBIG2Decode` — symbol-dictionary/text-region + generic/refinement regions on the arithmetic path, plus Huffman/MMR/halftone in phase 2), and **JPEG2000** (`/JPXDecode` colour scans, including MRC high-resolution stencil-masked foreground layers). Plus the **LZWDecode** stream filter.
- Non-embedded **CJK** text — Type0 fonts with predefined Adobe CMaps (GB1 / CNS1 / Japan1 / Korea1, e.g. GBK-EUC / Shift-JIS / Big5 / EUC-KR and the `Uni*` families) rendered from installed system CJK fonts via a CID→Unicode mapping; simple CJK faces resolve to the same installed font for consistency.
- Adobe-profile **DeviceCMYK → RGB** conversion (baked LUT) so process colours in images and `k`/`K`/`scn` fills match Acrobat rather than the naïve formula.
- Page geometry setters — `(*Page).SetMediaBox/SetCropBox/SetTrimBox/SetBleedBox/SetArtBox(Rectangle)` write a box directly on the page (validated, overriding inherited/referenced values), and `(*Page).SetPageSize(width, height)` resizes the page via its MediaBox (content is not scaled). New `(*Page).MediaBox() (Rectangle, error)` getter. Mirrors Aspose.PDF for .NET's `Page.*Box` setters and `Page.SetPageSize`.
- Flattening — bake interactive content into static page content: `(*Document).Flatten()` / `(*Form).Flatten()` (all form fields + drop `/AcroForm`), `(Field).Flatten()` (a single field, leaving the rest of the form intact), `(Annotation).Flatten()` (one annotation), and `(*AnnotationCollection).Flatten()` (all non-widget annotations on a page). Appearances (`/AP/N`, honoring `/AS`) are drawn into the page content at the annotation `/Rect` per ISO 32000-1 §12.5.5, then the interactive objects are removed. Mirrors Aspose.PDF for .NET's `Document.Flatten` / `Form.Flatten` / `Field.Flatten` / `Annotation.Flatten`.
- `(*Page).SearchText(query, opts...)` / `(*Document).SearchText(query, opts...)` — locate occurrences of a query in reading order, returning a `TextMatch` (text + 1-based page + bounding `Rectangle`) for each. Literal and case-sensitive by default; `SearchOptions{CaseInsensitive, Regex}` enables case-folding and RE2 regular expressions. Built on the layout-extraction pipeline; matches are located within a single line. Match rectangles use per-glyph start positions recorded during extraction, so sub-fragment boxes are accurate (not interpolated). Mirrors Aspose.PDF for .NET's `TextFragmentAbsorber`.
- `(*Document).DeletePage(n)` / `(*Document).DeletePages(pageNums...)` — remove pages in place by 1-based number; numbers are de-duplicated and validated before any removal (atomic on error), and removing every page is rejected. Mirrors Aspose.PDF for .NET's `Document.Pages.Delete(int)` / `Delete(int[])`.

### Changed

- `(*Page).CropBox/TrimBox/BleedBox/ArtBox` now return a `Rectangle` (full box coordinates) instead of a `PageSize` (width/height), mirroring Aspose.PDF for .NET's box properties. Use `Size()` for width/height, or compute from the rectangle. `(*Page).Size()` is unchanged.
- Renamed the Info-dictionary API to mirror Aspose.PDF for .NET's `Document.Info`: `(*Document).Metadata()` → `Info()`, `SetMetadata()` → `SetInfo()`, `ClearMetadata()` → `ClearInfo()`, and the `Metadata` struct → `DocumentInfo`. In Aspose.PDF for .NET, `Document.Metadata` is the XMP store (here `(*Document).XMP`), so the previous name collided.

### Documentation

- Clarified that `Validate` is a structural-integrity check, not a PDF/A·PDF/UA conformance check (unlike Aspose.PDF for .NET's `Document.Validate`).
- Corrected `JavaScriptAction` docs: it is constructable via `NewJavaScriptAction` (and encoded back on Save), not parse-only.

### Fixed

Driven by visual testing against a real-world PDF corpus (78 fixes). Highlights:

- **Parser robustness** — never hang or crash on malformed input. A direct `/Length` is trusted only when `endstream` actually follows it (a bogus `/Length 1` no longer truncates a page); the lexer always advances past stray delimiter/binary bytes, so a page whose content fails to inflate can no longer spin forever; tolerant xref recovery (far `startxref`, object-header boundaries), tolerant object loading and `/Pages` traversal keep partially-damaged files openable; out-of-range `startxref`/xref offsets no longer panic; literal-string octal escapes and 40-bit RC4 (V=1 R=2) are decoded.
- **Inline images** — accept the ASCIIHex/ASCII85 `>` EOD marker directly before `EI`, and consume *unfiltered* inline images by their exact computed byte length instead of scanning for `EI` in binary data (both previously dropped runs of glyph-mask "text"); apply the `/DecodeParms` PNG predictor to filtered inline images (dotted/dashed leaders rendered solid without it).
- **Rendering correctness** — text clipping (`Tr` modes 4–7) so "draw glyphs as a clip, then paint an image through them" renders as text; Separation/DeviceN shadings run the tint transform instead of reading one component as gray (no more near-black backgrounds); box-average minified images (smooth downscale + correct `/Matte` borders); honour ExtGState constant alpha on images; correct stencil-mask, JBIG2 `/Decode` inversion, indexed-palette, and CMYK-JPEG handling.
- **Fonts & text** — render embedded classic Type1; resolve non-embedded CJK to installed faces even when the family is installed under a longer name (e.g. `Microsoft JhengHei UI`); prefer installed Standard-14 metric-equivalents; fill WinAnsi positions 0x80–0x9F in Standard-14 widths. Text **extraction** now restores the font and text state on `Q` (graphics-state stack), fixing labels that decoded through the wrong font after a `q … Tf … Q` block.
- **Annotations** — honour gray and CMYK `/IC` interior colours and skip a border when no colour is set (Square/Circle); correct Line-annotation arrowhead direction and dashing; synthesize appearances for Square/Circle/Line/Ink that lack `/AP`; AcroForm widget `/AP` matches Acrobat's text layout (no ghosting) and choice fields write the `/I` selected-indices array.
- **Showcase** — `docs/feature_showcase.pdf` now ships unencrypted so it previews inline on GitHub and opens in any viewer.

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

[Unreleased]: https://github.com/aspose-pdf-foss/aspose-pdf-foss-for-go/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/aspose-pdf-foss/aspose-pdf-foss-for-go/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/aspose-pdf-foss/aspose-pdf-foss-for-go/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/aspose-pdf-foss/aspose-pdf-foss-for-go/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/aspose-pdf-foss/aspose-pdf-foss-for-go/releases/tag/v0.1.0
