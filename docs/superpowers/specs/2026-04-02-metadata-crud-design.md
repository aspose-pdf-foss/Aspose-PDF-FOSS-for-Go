# Metadata CRUD Design

**Date:** 2026-04-02  
**Status:** Approved

## Overview

Extend the metadata API to support full CRUD: read (existing), write, and delete. Remove the standalone `GetMetadata` function in favour of the immutable Document API pattern already used everywhere else.

## Changes

### `Metadata` struct — add `Custom` field

```go
type Metadata struct {
    Title        string
    Author       string
    Subject      string
    Keywords     string
    Creator      string
    Producer     string
    CreationDate string
    ModDate      string
    Custom       map[string]string // arbitrary Info dict keys
}
```

Custom field keys are written as-is into the PDF Info dictionary alongside the standard fields.

### New `Document` methods

```go
// SetMetadata returns a new Document configured to write meta as the Info
// dictionary when saved. This is a full replacement: fields absent from meta
// (empty strings, nil Custom) are omitted from the output Info dict.
// To update a single field, read the current metadata, modify the field, and
// call SetMetadata with the updated struct.
func (d *Document) SetMetadata(meta Metadata) *Document

// ClearMetadata returns a new Document configured to omit the Info dictionary
// entirely when saved. Use this to strip all metadata before publishing.
func (d *Document) ClearMetadata() *Document
```

Both methods follow the immutable pattern: the receiver is never modified.

### Remove standalone function

`GetMetadata(inputPath string)` is removed. Replacement:

```go
doc, err := asposepdf.Open("input.pdf")
meta, err := doc.Metadata()
```

### Internal — `metadataConfig`

A new unexported type added to `metadata.go`:

```go
type metadataConfig struct {
    meta  Metadata
    clear bool // if true, suppress Info dict entirely
}
```

`Document` gains a field `metadataConfig *metadataConfig` (nil = preserve source metadata, consistent with current behaviour).

### Writer behaviour

`buildDocumentPDF` checks `d.metadataConfig`:

| Value | Behaviour |
|-------|-----------|
| `nil` | Copy Info dict from primary source document (current behaviour) |
| `clear: true` | Omit Info dict from output |
| `clear: false` | Serialize `meta` into a new Info dict; skip empty-string fields and nil Custom entries |

## Behaviour Details

**Full replacement:** `SetMetadata` replaces the Info dict entirely. Fields set to `""` are not written. Example:

```go
meta, _ := doc.Metadata()   // read current
meta.Title = "New Title"    // modify one field
doc = doc.SetMetadata(meta) // write back — all other fields preserved
```

**Custom fields:** written with their key as-is. Standard keys (`/Title`, `/Author`, etc.) take precedence if the same key appears in both `Custom` and the typed fields.

**`ClearMetadata` + `SetMetadata`:** last call wins (both just set `metadataConfig` on the returned Document).

## Files Affected

- `metadata.go` — add `metadataConfig`, `SetMetadata`, `ClearMetadata`; remove `GetMetadata`
- `document.go` — add `metadataConfig *metadataConfig` field to `Document`; update `withCopiedPatches`
- `writer.go` — serialize `metadataConfig` into the output PDF
- `metadata_test.go` — update `TestGetMetadata` → use `Open`+`Metadata()`; add write/clear tests

## Testing

- `TestSetMetadata` — write metadata, save, reopen, verify all fields round-trip
- `TestSetMetadataCustomFields` — custom fields survive save/reopen
- `TestSetMetadataEmptyFieldRemoved` — empty string fields absent from output
- `TestClearMetadata` — after clear, `Metadata()` returns zero-value struct
- `TestSetMetadataReplaces` — `SetMetadata` on doc with existing metadata fully replaces it
