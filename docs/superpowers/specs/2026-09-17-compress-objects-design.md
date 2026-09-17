# Object Streams (CompressObjects) — Design

Date: 2026-09-17 · Issue: pdf-go-o0wy (phase 1 of 2) · Status: approved

## Goal

Pack non-stream objects into object streams (`/Type /ObjStm`, ISO 32000-1
§7.5.7) and write a cross-reference stream (§7.5.8) instead of a classic xref
table. Lossless; typically 10–30% smaller for documents with many small
objects (forms, annotations, outlines, structure trees). Mirrors Aspose.PDF for
.NET's `OptimizationOptions.CompressObjects`.

Phase 2 of the issue, `UnembedFonts`, is lossy, unrelated to file structure,
and is tracked as a separate follow-up.

## Decisions taken before design

- **Two phases.** Object streams now; `UnembedFonts` afterwards.
- **On by default.** `DefaultOptimizationOptions()` includes
  `CompressObjects`. The only cost is a minimum header of PDF 1.5, which every
  reader of the last two decades understands.
- **Branch inside the sequential writer.** `assemble()` stays shared; only the
  layout step differs, as the linearized writer already does. Rejected:
  post-processing a classic file (double work, moves signature placeholders)
  and hybrid-reference files with `/XRefStm` (complexity for PDF 1.4 readers
  nobody targets).

## Public API

```go
type OptimizationOptions struct {
    // ...existing fields...
    // CompressObjects packs non-stream objects into object streams and writes
    // a cross-reference stream on Save/WriteTo (lossless; PDF 1.5+).
    CompressObjects bool
}

type OptimizationResult struct {
    // ...existing fields...
    // CompressedObjects is the number of objects eligible for packing at the
    // time Optimize ran; the packing itself happens on Save/WriteTo.
    CompressedObjects int
}
```

`Optimize` sets an unexported `Document.compressObjects` flag, which the writer
honours on every later `Save`/`WriteTo`. `SaveLinearized`/`WriteToLinearized`
ignore it (the linearized writer emits classic tables only).

## Layout

Written in this order after the header:

1. Every object that is **not eligible**, as an ordinary indirect object.
2. Object streams, each holding up to 100 eligible objects in ascending output
   number: `/Type /ObjStm /N <count> /First <offset>`, a header of
   `objnum offset` pairs, then the object bodies; Flate-compressed.
3. The cross-reference stream: `/Type /XRef /Size /W [1 w2 w3] /Index`, plus the
   trailer keys `/Root /Info /Encrypt /ID`; Flate-compressed.
   Entries: type 0 (free: next free 0, gen 65535 for object 0), type 1
   (offset, gen 0), type 2 (object-stream number, index within it). `w2` is
   the byte width of the largest offset or object-stream number, `w3` the
   width of the largest index (minimum 1).
4. `startxref` pointing at the cross-reference stream, `%%EOF`.

The header becomes at least `%PDF-1.5`; a higher version (`%PDF-2.0` for
AES-256) is kept.

### Eligibility

An object may be packed when it is not a stream and has generation 0, and it
is none of: the encryption dictionary, a signature dictionary (its `/Contents`
and `/ByteRange` are patched in place by byte offset), an object stream, the
cross-reference stream.

## Interactions

- **Encryption.** Objects inside an object stream are not encrypted
  individually; the object stream is encrypted as a whole under its own object
  number, like any stream. The cross-reference stream is never encrypted
  (§7.5.8.2), which the reader already honours.
- **Full-rewrite signing.** The signature dictionary stays an indirect object,
  so `applySignature` is unchanged.
- **Incremental revisions** (`appendRevision`: incremental signing,
  `AddValidationInfo`). The appended section uses the same kind as the file's
  last one: a cross-reference stream after a cross-reference stream, a classic
  table after a classic table. Readers generally accept mixing, strict
  validators do not. Objects in an appended revision are written unpacked.
- **Linearization.** Unaffected; the flag is ignored.

## Internals

| File | Responsibility |
|---|---|
| `objstm_write.go` | Eligibility, batching, object-stream serialization, cross-reference stream encoding |
| `writer.go` | Branch to the object-stream layout when `d.compressObjects` |
| `sign_incremental.go` | `appendRevision` writes a cross-reference stream when the previous section is one |
| `optimize.go` | `CompressObjects` option, result count, default preset |

## Validation

- Round trip: save compressed, reopen, same page count and extracted text.
- Encryption: RC4-128, AES-128, AES-256 — reopen with the password, text
  identical.
- Signatures: full-rewrite signing of a compressed document and incremental
  signing on top of a compressed file both verify; the incremental section is
  a cross-reference stream.
- Structure unit tests: `/W` widths, entry types, `/N` and `/First` parse back.
- Independent check: pikepdf (qpdf) opens the output and `check()` reports no
  errors — across the ~1,000-document corpus: resave compressed, compare page
  count, record size before/after.

## Out of scope

- `UnembedFonts` (phase 2).
- Packing objects inside incremental revisions.
- Hybrid-reference files (`/XRefStm`).
- Object-stream output from the linearized writer.
