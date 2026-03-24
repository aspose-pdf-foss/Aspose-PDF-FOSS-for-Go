---
name: fix-bug
description: Investigate and fix a bug in the pdf-for-go library
---

Investigate and fix the following bug in the pdf-for-go library:

$ARGUMENTS

Follow these steps:

1. **Reproduce** — read the relevant source files and, if needed, inspect test PDF files in `test_data/split/` to understand when the bug occurs.

2. **Root cause** — identify the exact location in the code where the bug originates. Common areas:
   - `doc.go` — dependency collection (`collectDeps`, `collectInheritedDeps`, `walkPageTree`)
   - `writer.go` — PDF serialization (`buildDocumentPDF`, `writeValue`)
   - `parser.go` — stream decoding (`decodeStream`, `parseDictOrStream`)
   - `types.go` — core data structures

3. **Fix** — make a minimal targeted change. Do not refactor surrounding code.

4. **Add validation** — if the bug can be detected structurally, add a check to `validate.go` (assign a new step number, use an appropriate issue code: `INVALID_HEADER`, `XREF_ERROR`, `OBJECT_ERROR`, `PAGE_TREE_ERROR`, `STREAM_ERROR`, or `ENCRYPTED`).

5. **Tests** — add a regression test:
   - In `validate_test.go` if the fix is covered by validation.
   - In the relevant `*_test.go` otherwise.
   - Use files from `test_data/split/` for real-PDF tests; ask the user which file to use if unsure.

6. **Verify** — run `go test ./...` and confirm all tests pass.
