---
name: new-feature
description: Implement a new public API function for the pdf-for-go library
---

Implement a new public API function for the pdf-for-go library.

Feature to implement: $ARGUMENTS

Follow these steps:

1. **Design** — describe the public signature and confirm it fits the existing
   API style:
   - All public API is exposed as methods on `*Document`; there are no
     standalone functions that accept file paths
   - Methods return a new `*Document` (or `([]*Document, error)`);
     the receiver is never modified
   - No external dependencies

2. **Implementation** — add code in the appropriate file:
   - Operations that manipulate pages (rotate, reorder, split, extract,
     append, etc.) → `document_pages.go`
   - Core document lifecycle (open, save, metadata) → `document.go`
   - PDF writing logic that extends the write pipeline → `writer.go`
   - All code lives in package `asposepdf` (module root)

3. **Tests** — create `<feature>_test.go` in package `asposepdf_test`:
   - Unit tests use only synthetic PDFs built with `buildMinimalPDF()`
   - If a real PDF from `testdata/split/` is needed, ask the user which
     file to use before hardcoding a name
   - Follow the pattern of existing `*_test.go` files

4. **Update docs** — add the new function to the Public API list in `CLAUDE.md`
   and add a usage example to `README.md`

5. **Verify** — run `go test ./...` and `go build ./...` and confirm everything
   passes, then commit
