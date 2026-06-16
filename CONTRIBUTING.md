# Contributing

Thanks for your interest in **aspose.pdf-for-go-foss** — a pure-Go,
dependency-free PDF library.

Right now the most valuable contribution is a good **bug report** — especially a
real-world PDF that the library renders, parses, or extracts incorrectly. See
[Reporting bugs](#reporting-bugs) below.

The rest of this document records the project's conventions and ground rules.
They apply to any code change and are a useful reference if you are reading the
source or proposing a fix.

## Ground rules

These are hard requirements for any change to the library:

- **Pure Go, standard library only.** The library must build with **zero
  external dependencies**. Do not add `require` entries to `go.mod`. If you need
  functionality the stdlib doesn't provide (a codec, a parser), implement it
  in-house.
- **SPDX license header.** Every `.go` file starts with the line
  `// SPDX-License-Identifier: MIT` on its own line, a blank line, then the
  `package` declaration.
- **Mirror Aspose.PDF for .NET.** When a feature has a counterpart in
  Aspose.PDF for .NET, shape the public API after it — same concepts, type
  names, and call patterns (`Document`, `Page`, `TextFragment`, `Table`/`Row`/
  `Cell`, `BorderInfo`/`MarginInfo`, …), adapting only where Go idioms require
  it (error returns over exceptions, `io.Reader`/`io.Writer` variants, exported
  funcs over constructors). A Go developer who knows Aspose.PDF for .NET should
  recognize the API immediately.
- **Keep docs in sync.** If you change the public API, update `README.md` (and
  `CHANGELOG.md`). Code examples in the README and `examples_test.go` must
  compile.

## Development

```bash
go build ./...      # library builds (no binary — it is a library)
go test ./...       # full test suite must pass
go vet ./...        # must be clean
go run ./_examples/<name>   # run a standalone example
```

The library lives in the root package `asposepdf`. Standalone example programs
live under `_examples/<name>/main.go` (the leading `_` makes the Go toolchain
skip them in `./...`).

## Tests

- Every feature gets its own `*_test.go` file.
- Short, focused API examples go in `examples_test.go` as `ExampleXxx` functions
  with `// Output:` comments — they appear on pkg.go.dev and are validated by
  `go test`.
- Tests that need a real PDF use a file from `testdata/`, wired through the
  `testFile(t)` / `testFiles(t)` / `testGroups(t)` helpers and declared in
  `testdata/testfiles.json` (keyed by test function name). If you need a new
  test PDF, open an issue first so we can agree on the fixture.
- A bug fix should come with a regression test that fails before the fix and
  passes after.

## Reporting bugs

Open a GitHub issue with the PDF that reproduces the problem (or a minimal
synthetic one), the API call you made, what you expected, and what happened.
For rendering issues, attach the rendered output and, if you can, a reference
rendering from another viewer.

## Spec

When in doubt about PDF behavior, the normative reference is **ISO 32000-1**
(PDF 1.7) / ISO 32000-2 (PDF 2.0). Cite the relevant section in code comments
where it clarifies a non-obvious decision.
