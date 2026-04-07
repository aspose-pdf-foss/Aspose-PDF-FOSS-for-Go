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
- `Open(path)` — opens a PDF file and returns a `*Document`
- `OpenStream(r io.Reader)` — opens a PDF from an `io.Reader` and returns a `*Document`
- `(*Document).PageCount()` — current page count
- `(*Document).Pages()` — returns `[]*Page` live views of all pages
- `(*Document).Page(n)` — returns a `*Page` live view of page n (1-based)
- `(*Document).Rotate(angle, pageNums...) error` — rotates selected pages; rotation accumulates
- `(*Document).SetRotation(angle, pageNums...) error` — sets selected pages to exactly angle, replacing any existing rotation
- `(*Document).Reorder(order) error` — rearranges pages in place; pages may be repeated or omitted
- `(*Document).Append(others...)` — appends all pages from others into this document; nil arguments are skipped
- `(*Document).SetPassword(userPassword, ownerPassword)` — configures encryption; applied on Save/WriteTo
- `(*Document).WriteTo(w) (int64, error)` — writes the document to an `io.Writer` (implements `io.WriterTo`)
- `(*Document).Save(outputPath) error` — writes the document to a file
- `(*Document).Metadata() (Metadata, error)` — returns Info metadata read from live in-memory state
- `(*Document).ExtractText() ([]string, error)` — returns text for all pages (one entry per page)

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
- `(*Page).ExtractText() (string, error)` — returns the text content of a page; unknown font characters become U+FFFD
- `PageSize` struct — Width, Height in points (1/72 inch)

**`page_labels.go`** — page label support
- `(*Page).Label()` — formatted page label from the document's `/PageLabels` number tree; falls back to decimal page number if absent
- Supported styles: `/D` decimal, `/r`/`/R` roman, `/a`/`/A` alphabetic; optional `/P` prefix and `/St` start value

**`page_range.go`**
- `PageRange` struct — From, To (1-based, inclusive)

**`metadata.go`**
- `(*Document).SetMetadata(meta)` — replaces the Info dictionary in memory; full replacement, empty fields omitted
- `(*Document).ClearMetadata()` — removes the Info dictionary; applied on Save/WriteTo
- `Metadata` struct — Title, Author, Subject, Keywords, Creator, Producer, CreationDate, ModDate, Custom map[string]string

**`encrypt.go`**
- `Encrypt(inputPath, outputPath, userPassword, ownerPassword)` — writes a password-protected PDF using RC4-128 (PDF 1.4 Standard Security Handler, revision 3)

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

### Text extraction (`text.go`, `content_parser.go`, `font.go`, `font_metrics.go`, `encoding.go`)

1. `parseContentStream(data)` tokenizes content stream bytes into `contentOp` structs (operator + operands), reusing the existing `lexer`
2. `resolveFont(objects, fontDict)` maps font dictionaries to `fontInfo{name, encoding [256]rune, widths [256]float64, known bool}` — supports WinAnsi, MacRoman, Standard encodings, `/Differences`, standard 14 fonts, Symbol, ZapfDingbats; resolves glyph widths from `/Widths` array, Standard 14 built-in metrics (`font_metrics.go`), or 600-unit fallback
3. `textExtractor` state machine processes operators (BT/ET/Tf/Td/Tm/Tj/TJ/Tz/etc.), tracking text matrix position, font, spacing, and horizontal scaling; advances text matrix by glyph width after each character (PDF spec 9.4.4)
4. Space/newline insertion uses font metrics: horizontal gap > spaceWidth×0.3 → space, vertical shift > fontSize×0.5 → newline
5. Form XObjects (`Do` operator) are recursively processed with inherited CTM and overridden resources

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
