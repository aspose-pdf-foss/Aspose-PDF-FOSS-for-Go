# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Run all tests
go test ./...

# Run a single test
go test -run TestExtract ./...

# Run tests with verbose output
go test -v ./...

# Build (no binary — library only)
go build ./...
```

## Architecture

Pure Go library. No external dependencies. All code is in the root package `pdfsplitter`.

### Public API

**`splitter.go`**
- `Split(inputPath, outputDir)` — splits all pages, delegates to `SplitRange(1, 0)`
- `SplitRange(inputPath, outputDir, from, to)` — splits a page range (1-based, `to=0` means last page)
- `Extract(inputPath, outputPath, ranges...)` — builds a new PDF from selected page ranges

**`merger.go`**
- `Merge(outputPath, inputPaths...)` — combines pages from multiple PDFs into one

**`rotate.go`**
- `Rotate(inputPath, outputPath, angle, pageNums...)` — rotates pages by 90/180/270°; angle adds to existing `/Rotate`; empty `pageNums` rotates all pages

**`document.go`** — mutable Document API
- `Open(path)` — opens a PDF and returns a mutable `*Document`
- `(*Document).PageCount()` — current page count
- `(*Document).Pages()` — returns live `[]*Page` views of all pages
- `(*Document).Page(n)` — returns a live `*Page` view of page n (1-based)
- `(*Document).Rotate(angle, pageNums...)` — rotates pages in-place
- `(*Document).ExtractPages(ranges...)` — keeps only specified page ranges
- `(*Document).Reorder(order)` — rearranges pages; pages may be repeated or omitted
- `(*Document).AppendFrom(other)` — appends all pages from another Document
- `(*Document).WriteTo(w)` — writes current state to an `io.Writer` (implements `io.WriterTo`)
- `(*Document).Save(outputPath)` — writes current state to file
- `(*Document).Metadata()` — returns Info metadata from the primary source document

**`page.go`** — Page and PageSize types
- `PageSizes(inputPath)` — returns dimensions of every page in a PDF file
- `(*Page).Number()` — 1-based page number within the document
- `(*Page).Size()` — page dimensions from MediaBox (with inheritance from page tree)
- `(*Page).Rotation()` — effective rotation in degrees (0, 90, 180, 270); reflects Document.Rotate patches
- `PageSize` struct — Width, Height in points (1/72 inch)

**`metadata.go`**
- `GetMetadata(inputPath)` — reads Info metadata from a PDF file
- `Metadata` struct — Title, Author, Subject, Keywords, Creator, Producer, CreationDate, ModDate

**`encrypt.go`**
- `Encrypt(inputPath, outputPath, userPassword, ownerPassword)` — writes a password-protected PDF using RC4-128 (PDF 1.4 Standard Security Handler, revision 3)
- `(*Document).SetPassword(userPassword, ownerPassword)` — configures encryption for the next `Save`/`WriteTo` call; if ownerPassword is empty it defaults to userPassword

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

`buildMultiPagePDF` is the core output function for the functional API:
1. Union the `deps` sets of all selected pages
2. Remap original object numbers to 1-based sequential numbers
3. Serialize each object; patch `/Parent` refs on page objects to point to the new `/Pages` node
4. Write a minimal PDF: header + objects + xref table + trailer

`buildPagePDF` is a thin wrapper around `buildMultiPagePDF` for single-page use.

`buildDocumentPDF` is used by `(*Document).WriteTo`: handles pages from multiple source documents in arbitrary order, with per-page patches (e.g. `/Rotate`).

**`pdfDirectRef`** (defined in `types.go`) — like `pdfRef` but written by `writeValue` without remapping. Used for `/Parent` patches so that the new `/Pages` object number (which lives in the output space, not the source space) is never accidentally translated by the remap function.

### Dependency collection

`collectDeps` recursively walks the object graph (dict values, array elements, stream dict, and raw stream bytes via regex `\b(\d+)\s+\d+\s+R\b`) to find all referenced objects. It skips `/Pages`, `/Catalog`, and `/Page` nodes — these belong to the page tree and are rebuilt by the writer. `collectInheritedDeps` additionally walks up the page tree to capture inherited `/Resources`.

`walkPageTree` adds each `/Page` object to deps directly (bypassing `collectDeps`) and then calls `collectValueDeps` on its dict, so foreign page objects reached transitively (e.g. via link annotations) are never copied into the output.

## Output conventions

- All files produced by examples and manual runs are saved to `result_files/` in the project root.
- This folder is not committed to the repository.

## Testing conventions

- Test PDF files are stored in `testdata/split/` (`4pages.pdf`, `Binder1.pdf`, `PdfWithLinks.pdf`, `PdfWithTable.pdf`, `alfa.pdf`, `marketing.pdf`).
- When writing tests that use real PDF files, always take them from `testdata/split/` and ask the user which file to use before hardcoding a name.
- Each feature gets its own `*_test.go` file (e.g. `merger_test.go`, `splitter_test.go`).
- `TestSplitFiles` in `splitter_test.go` iterates all files in `testdata/split/`, splits each into `result_files/<stem>/`, and validates every output page with `Validate`.
