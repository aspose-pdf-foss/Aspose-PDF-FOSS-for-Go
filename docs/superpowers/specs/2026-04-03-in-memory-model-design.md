# Phase 1: In-Memory Object Model — Design Spec

**Date:** 2026-04-03  
**Branch:** refactor  
**Status:** Approved

---

## Goal

Replace the current `rawDocument` + patches architecture with a full in-memory PDF object model. All operations read and write the same live data structures — no deferred patches, no save/reload required to observe changes.

---

## Problem with Current Architecture

```
rawDocument (bytes + xref + cache)
    ↓ refs
Document ([]pageRef + patches + configs)
    ↓ write-time only
PDF bytes
```

- Getters read from `rawDocument` (source bytes)
- Setters write to `patches` / `metadataConfig` / `encryptConfig` (deferred)
- Result: `doc.SetMetadata(...)` followed by `doc.Metadata()` returns stale data
- Users cannot work with a document in complex scenarios without saving and re-opening

---

## New Architecture

```
Document
  ├── objects  map[int]*PDFObject   ← all objects in memory
  ├── pages    []*PDFObject         ← ordered page list
  ├── catalog  PDFDict              ← /Catalog root
  ├── info     PDFDict              ← /Info (nil = no metadata)
  ├── encrypt  *encryptConfig       ← nil = no encryption
  └── nextID   int                  ← counter for new objects
```

All operations mutate `Document` directly. Getters and setters operate on the same structures. No copies, no patches.

---

## Data Types

```go
// PDFValue — any value in the PDF object model
type PDFValue interface{}

// Concrete types:
type PDFDict   map[string]PDFValue
type PDFArray  []PDFValue
type PDFStream struct {
    Dict PDFDict
    Data []byte   // decoded bytes (filters applied on parse)
}
type PDFRef struct{ ID int }  // reference to an object by ID

// Primitives: int, float64, string, bool — standard Go types

// PDFObject — node in the object table
type PDFObject struct {
    ID    int
    Value PDFValue
}
```

All internal types (`PDFObject`, `PDFDict`, etc.) are unexported. The public API exposes only `Document`, `Page`, `Metadata`, `PageSize`, `RotationAngle`.

`PDFStream.Data` holds already-decoded bytes. Filters (FlateDecode, ASCIIHex, etc.) are applied during parsing, not at read time.

---

## Page Model

```go
type Page struct {
    doc *Document   // parent document
    obj *PDFObject  // the /Page object
}
```

`Page` is a lightweight view — two pointers, no data copy. All page methods read and write directly into `obj`:

```go
func (p *Page) Number() int
func (p *Page) Size() PageSize
func (p *Page) Rotation() RotationAngle     // reads /Rotate from obj
func (p *Page) SetRotation(a RotationAngle) // writes /Rotate into obj
func (p *Page) CropBox() PageSize
func (p *Page) TrimBox() PageSize
func (p *Page) BleedBox() PageSize
func (p *Page) ArtBox() PageSize
func (p *Page) Label() string
```

Example of consistency after refactor:

```go
doc.Rotate(90, 1)
doc.Page(1).Rotation() // → 90 ✓  (was broken before)

doc.SetMetadata(pdf.Metadata{Author: "sonet"})
meta, _ := doc.Metadata()
meta.Author // → "sonet" ✓  (was broken before)
```

---

## Public API Changes

### Mutable — no return value (previously returned `*Document`):

```go
doc.SetMetadata(meta Metadata)
doc.ClearMetadata()
doc.SetPassword(userPassword, ownerPassword string)
doc.Append(others ...*Document)
```

### Error only — no Document return (previously `(*Document, error)`):

```go
err = doc.Rotate(angle RotationAngle, pageNums ...int) error
err = doc.SetRotation(angle RotationAngle, pageNums ...int) error
err = doc.Reorder(order []int) error
```

### Unchanged — return new Document by nature:

```go
result, err := doc.Extract(ranges ...PageRange) (*Document, error)
parts, err  := doc.Split() ([]*Document, error)
```

### Unchanged — read-only or I/O:

```go
doc.PageCount() int
doc.Pages() []*Page
doc.Page(n int) *Page
doc.Metadata() (Metadata, error)
doc.WriteTo(w io.Writer) (int64, error)
doc.Save(outputPath string) error
```

---

## Parsing Pipeline (Eager)

`Open` and `OpenStream` parse everything upfront:

```
1. readFile → []byte
2. findStartXRef → offset
3. parseXRef → xrefTable + trailer          (reuse existing xref.go)
4. check /Encrypt → error if present
5. parseAllObjects → map[int]*PDFObject     ← NEW: iterate all xref entries
6. resolvePageTree → []*PDFObject           ← NEW: walk /Catalog → /Pages → /Page
7. extractInfo → PDFDict                    ← NEW: read /Info from trailer
8. assemble Document{objects, pages, catalog, info, nextID}
```

**Reused without changes:** `lexer.go`, `parser.go`, `xref.go`  
**Partially reused:** object stream logic from `doc.go`  
**Removed:** `rawDocument`, `pageRef`, `patchKey`, `patches`, `metadataConfig`

---

## Serialization (WriteTo / Save)

```
1. collect all objects from doc.objects
2. build /Pages tree from doc.pages
3. if doc.info != nil → write /Info object
4. if doc.encrypt != nil → write /Encrypt object
5. number objects sequentially (1, 2, 3...)
6. serialize each object:
     PDFDict   → << /Key value >>
     PDFStream → << dict >>\nstream\n...bytes...\nendstream
     PDFArray  → [ v1 v2 v3 ]
     PDFRef    → "N 0 R"
7. write xref table
8. write trailer with /Root, /Info, /Size
```

`writer.go` is refactored: `buildMultiPagePDF` is replaced by a simpler serializer that walks `doc.objects` directly. No dependency collection, no object remapping, no patch application.

---

## Error Handling

| Location | What can fail |
|----------|--------------|
| `Open` / `OpenStream` | all parse errors — one place, predictable |
| `Rotate`, `Reorder` | invalid page numbers — validate inputs |
| `Save` / `WriteTo` | I/O errors only |
| `SetMetadata`, `ClearMetadata`, `SetPassword` | cannot fail — no error return |

---

## Testing

- All existing `*_test.go` files remain valid in structure
- Signature updates required: remove `doc = doc.Method(...)` assignments
- `encrypt_internal_test.go`, `page_labels_internal_test.go` — may need updates as `rawDocument` disappears
- All tests must pass after migration: public API behavior is unchanged, only internals differ

---

## What is NOT in Phase 1

- Content stream parsing (text, graphics operators) — Phase 2
- Text extraction — Phase 2
- Image extraction — Phase 3
- Annotations — Phase 4
- Forms (AcroForm) — Phase 5
- Attachments — Phase 6
- Text modification — Phase 7
