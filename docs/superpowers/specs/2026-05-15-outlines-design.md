# Outlines (Bookmarks) Design Spec (Subepic 1 of `pdf-go-qrx`)

**Date:** 2026-05-15
**Issue:** `pdf-go-qrx` — Outlines / bookmarks + named destinations (navigation)
**Subepic 1 scope:** Outlines (read + create + edit), with all 8 destination types and Action reuse
**Subepic 2 (future):** Named destinations (`/Names /Dests` tree + legacy `/Dests` dict)
**API philosophy:** Maximally close to Aspose.PDF for .NET — same names, same recursive `OutlineItemCollection` model, equivalent IList semantics.

## Goals

- Read existing `/Outlines` tree from any PDF and surface it as `Document.Outlines() *OutlineItemCollection`.
- Create new outline entries with `pdf.NewOutlineItemCollection(doc)`.
- Add/Insert/Remove/RemoveAt children at any tree level via collection methods on each entry.
- Support all eight PDF destination types (`/XYZ`, `/Fit`, `/FitH`, `/FitV`, `/FitR`, `/FitB`, `/FitBH`, `/FitBV`) with optional "unchanged" fields.
- Support `/A` action attribute using the existing `Action` interface from the annotations epic (`GoToURIAction`, `GoToAction`, `NamedAction`, `SubmitFormAction`, `ResetFormAction`, `JavaScriptAction`).
- Support style attributes per ISO 32000-1: `Bold`, `Italic` (`/F` bit flags), `Color` (`/C` RGB array), `IsExpanded` (`/Count` sign).
- Cross-epic compatibility: AES-128/AES-256 encryption + AcroForm + annotations + outlines all coexist through full roundtrips.

## Non-Goals

- Named destinations — Subepic 2 of `pdf-go-qrx`.
- `/SE` structure tree element references.
- Aspose.PDF for .NET's `Outlines.GoToTarget` convenience (can be added later if requested).
- WidthFactor (rare Acrobat extension).
- Auto-generated table-of-contents from document headings.

## Architecture

### PDF spec mapping (ISO 32000-1 §12.3.3)

| PDF entry | Go API |
|---|---|
| Catalog `/Outlines` (ref to outline root dict) | `(*Document).Outlines() *OutlineItemCollection` |
| Outline root dict (`/Type /Outlines /First /Last /Count`) | The collection returned by `Outlines()` |
| Item dict `/Title` | `OutlineItemCollection.Title() / SetTitle()` |
| Item dict `/F` bit 1 (italic) | `Italic() / SetItalic()` |
| Item dict `/F` bit 2 (bold) | `Bold() / SetBold()` |
| Item dict `/C [r g b]` | `Color() / SetColor()` |
| Item dict `/Count` sign | `IsExpanded() / SetIsExpanded()` |
| Item dict `/Dest` | `Destination() / SetDestination()` |
| Item dict `/A` | `Action() / SetAction()` |
| Item dict `/Parent /Prev /Next /First /Last` | Managed internally; not part of public API |

### Recursive tree model

`OutlineItemCollection` is both an outline entry AND a collection of its children. The Document's root `Outlines()` returns the root collection whose own `Title()` is empty — only its children are visible bookmarks. This 1:1 mirrors Aspose.PDF for .NET, where `OutlineItemCollection` implements `IList<OutlineItemCollection>`.

### File organization

| File | Role |
|---|---|
| `outline.go` (new) | `OutlineItemCollection` type, accessors, tree manipulation, `(*Document).Outlines()` |
| `outline_destination.go` (new) | `Destination` interface, 8 concrete types, encode/decode helpers |
| `outline_parse.go` (new) | Read-side: walks `/Outlines` tree, builds in-memory model lazily |
| `outline_write.go` (new) | Write-side: serializes outline tree into PDF objects with Parent/Prev/Next/First/Last wiring |
| `writer.go` (modify) | Emit `/Outlines` reference in `/Catalog` if root has any children |
| `outline_test.go` (new) | External: roundtrip, all destinations, style props, tree manipulation, cross-epic |
| `outline_internal_test.go` (new) | Internal: destination array encode/decode, /F flag math, count math |
| `outline_aspose_parity_test.go` (new) | Mirrors Aspose .NET samples line-by-line, proving API parity |
| `outline_pypdf_test.go` (new) | Cross-tool roundtrip with pypdf 6.x |
| `CLAUDE.md`, `README.md` (modify, final task) | Public API docs + migration table from .NET |

## Public API

### Document-level

```go
// Outlines returns the document's root outline collection. Always
// non-nil — an empty collection is returned for documents without an
// /Outlines entry. Items added to the returned collection become
// top-level outline entries.
//
// Mirrors Aspose.PDF for .NET's Document.Outlines property.
func (d *Document) Outlines() *OutlineItemCollection
```

### OutlineItemCollection

```go
// OutlineItemCollection represents an outline entry and the collection
// of its children. The recursive structure mirrors Aspose.PDF for .NET:
// each entry is both a tree node (with Title, Color, Action,
// Destination, etc.) and a collection (Add/At/Remove/Count for
// children). The root collection — returned by Document.Outlines() —
// has no parent and an empty Title; only its children are visible as
// top-level bookmarks.
//
// Per ISO 32000-1 §12.3.3.
type OutlineItemCollection struct { ... }

// NewOutlineItemCollection builds an unattached outline entry bound to
// the given document. Add it to a parent via Document.Outlines().Add(...)
// or via another entry's Add(...) — until added it has no effect on
// the saved PDF.
//
// Aspose .NET: new OutlineItemCollection(doc.Outlines)
// Go:          pdf.NewOutlineItemCollection(doc)
func NewOutlineItemCollection(doc *Document) *OutlineItemCollection

// Document returns the document this collection is bound to.
func (o *OutlineItemCollection) Document() *Document

// Parent returns the parent entry, or nil for the root collection.
func (o *OutlineItemCollection) Parent() *OutlineItemCollection
```

#### Style accessors

```go
func (o *OutlineItemCollection) Title() string
func (o *OutlineItemCollection) SetTitle(s string)

// Bold corresponds to /F bit 2. Default false.
func (o *OutlineItemCollection) Bold() bool
func (o *OutlineItemCollection) SetBold(b bool)

// Italic corresponds to /F bit 1. Default false.
func (o *OutlineItemCollection) Italic() bool
func (o *OutlineItemCollection) SetItalic(b bool)

// Color returns the RGB label color, or nil if /C is absent (default
// black). SetColor(nil) clears /C.
func (o *OutlineItemCollection) Color() *Color
func (o *OutlineItemCollection) SetColor(c *Color)

// IsExpanded — viewer's initial expand/collapse state. Encoded via the
// sign of /Count. Default true.
func (o *OutlineItemCollection) IsExpanded() bool
func (o *OutlineItemCollection) SetIsExpanded(b bool)
```

#### Target accessors

Both `Action` and `Destination` may be set on the same entry. Per ISO 32000-1 §12.3.3, if both are present, viewers honor `/Dest`. This mirrors Aspose.PDF for .NET.

```go
// Action returns the action attached via /A. Reuses the Action interface
// defined for annotations (GoToURIAction, GoToAction, NamedAction,
// SubmitFormAction, ResetFormAction, JavaScriptAction).
func (o *OutlineItemCollection) Action() Action
func (o *OutlineItemCollection) SetAction(a Action)

// Destination — explicit view destination via /Dest. If both
// Destination and Action are set, /Dest takes priority per PDF spec.
func (o *OutlineItemCollection) Destination() Destination
func (o *OutlineItemCollection) SetDestination(d Destination)
```

#### Tree manipulation (mirrors .NET `IList<OutlineItemCollection>`)

```go
// Add appends child as the last child of this entry. Errors:
// - child is nil
// - child is already attached elsewhere
// - child or any descendant equals this entry (cycle)
// - child belongs to a different document
func (o *OutlineItemCollection) Add(child *OutlineItemCollection) error

func (o *OutlineItemCollection) Insert(index int, child *OutlineItemCollection) error

// Remove detaches child if it's a direct child. Returns true on hit.
// The detached child becomes unattached and can be re-added.
func (o *OutlineItemCollection) Remove(child *OutlineItemCollection) bool

func (o *OutlineItemCollection) RemoveAt(index int) error

// At returns the child at the given 0-based index, or nil if out-of-range.
func (o *OutlineItemCollection) At(index int) *OutlineItemCollection

// Count returns the number of direct children (not total descendants).
func (o *OutlineItemCollection) Count() int

// All returns a snapshot slice of direct children. Modifying the
// returned slice does not affect the underlying tree.
func (o *OutlineItemCollection) All() []*OutlineItemCollection
```

### Destination types

```go
type DestinationType int

const (
    DestinationTypeXYZ DestinationType = iota
    DestinationTypeFit
    DestinationTypeFitH
    DestinationTypeFitV
    DestinationTypeFitR
    DestinationTypeFitB
    DestinationTypeFitBH
    DestinationTypeFitBV
)

// Destination is the common interface for all explicit destinations.
type Destination interface {
    DestinationType() DestinationType
    Page() *Page
}
```

Concrete types (all 8) live in `outline_destination.go`. Each has:
- A field set including `page *Page` and view parameters per its variant.
- `NewDestinationXxx(page *Page, …)` constructor with all-explicit arguments.
- For XYZ/FitH/FitV/FitBH/FitBV: an additional `NewDestinationXxxUnchanged` constructor letting callers leave specific fields as "unchanged" (encoded as `/null` in PDF).
- Accessor methods exposing the stored fields.

Examples:

```go
type DestinationXYZ struct {
    page    *Page
    left    float64
    top     float64
    zoom    float64
    useLeft bool
    useTop  bool
    useZoom bool
}

func NewDestinationXYZ(page *Page, left, top, zoom float64) *DestinationXYZ
func NewDestinationXYZUnchanged(page *Page, left float64, useLeft bool, top float64, useTop bool, zoom float64, useZoom bool) *DestinationXYZ
func (d *DestinationXYZ) DestinationType() DestinationType { return DestinationTypeXYZ }
func (d *DestinationXYZ) Page() *Page                       { return d.page }
func (d *DestinationXYZ) Left() float64                     { return d.left }
func (d *DestinationXYZ) Top() float64                      { return d.top }
func (d *DestinationXYZ) Zoom() float64                     { return d.zoom }
func (d *DestinationXYZ) HasLeft() bool                     { return d.useLeft }
func (d *DestinationXYZ) HasTop() bool                      { return d.useTop }
func (d *DestinationXYZ) HasZoom() bool                     { return d.useZoom }
```

Other types follow the same pattern.

### Aspose .NET parity table

| Aspose .NET (C#) | This library (Go) |
|---|---|
| `doc.Outlines` (property) | `doc.Outlines()` (method) |
| `new OutlineItemCollection(doc.Outlines)` | `pdf.NewOutlineItemCollection(doc)` |
| `oic.Title = "..."` | `oic.SetTitle("...")` |
| `oic.Bold = true` | `oic.SetBold(true)` |
| `oic.Italic = true` | `oic.SetItalic(true)` |
| `oic.Color = Color.Red` | `oic.SetColor(&pdf.Color{R: 1, G: 0, B: 0, A: 1})` |
| `oic.IsExpanded = true` | `oic.SetIsExpanded(true)` |
| `oic.Action = new GoToAction(...)` | `oic.SetAction(pdf.NewGoToAction(...))` |
| `oic.Destination = new XYZExplicitDestination(page, 0, 800, 1)` | `oic.SetDestination(pdf.NewDestinationXYZ(page, 0, 800, 1))` |
| `parent.Add(child)` | `parent.Add(child) error` |
| `parent.Insert(0, child)` | `parent.Insert(0, child) error` |
| `parent.Count` | `parent.Count() int` |
| `parent[0]` (indexer) | `parent.At(0) *OutlineItemCollection` |
| `parent.Remove(child)` | `parent.Remove(child) bool` |
| `parent.RemoveAt(0)` | `parent.RemoveAt(0) error` |

## Read Side

### Lazy parse trigger

```go
func (d *Document) Outlines() *OutlineItemCollection {
    if d.outlinesRoot == nil {
        d.outlinesRoot = parseOutlines(d)
    }
    return d.outlinesRoot
}
```

`d.outlinesRoot *OutlineItemCollection` is a new Document field. Always non-nil after first call.

### `parseOutlines(d) *OutlineItemCollection`

1. Read `Catalog /Outlines` ref. Return empty root if missing.
2. Resolve to outline root dict. Return empty root if unparseable.
3. Walk `/First` chain via `walkSiblings` with cycle protection.

### `walkSiblings(d, parent, ref)`

- Linked-list walk via `/Next`.
- `seen[objNum]` cycle protection.
- Recurses into each child's `/First` for grandchildren.
- Depth cap (100 levels) defends against pathological inputs.

### `parseOutlineItem(d, dict, objNum, parent) *OutlineItemCollection`

Creates an `OutlineItemCollection` referencing the original `dict` and `objNum`. **Does not** eagerly parse Title/Color/Dest — accessors read from `item.dict` on demand. Trade-off: cheap to Open, slightly costlier to repeatedly read same property. Acceptable for typical usage.

### Destination parsing

`parseDestination(doc, raw pdfValue) Destination` handles:

- `pdfArray` — direct destination array.
- `string` / `pdfHexString` — named destination. In Subepic 1, resolved via the legacy `Catalog/Dests` dict if present. Full named-destination support arrives in Subepic 2.
- `pdfRef` — indirect ref to destination array; resolve then recurse.

`parseDestinationArray` dispatches on the fit-name element (`/XYZ`, `/Fit`, etc.) and constructs the appropriate concrete `Destination` type. `null` entries in XYZ-family arrays become `useXxx = false` on the constructed object.

### Page resolution

`resolvePageFromDestRef(doc, raw pdfValue) *Page` walks `doc.pages[]` looking for a match on underlying object number. Returns nil if the destination references a page outside the in-memory document (rare; safely ignored).

### Cycle and depth protection

- Sibling chain: `seen` set keyed by object number.
- Children recursion: same `seen` set propagated through the call stack.
- Hard depth cap of 100 levels.

### Best-effort error handling

Malformed outline entries are silently skipped. We never fail Open on outline parse problems. PDF readers in the wild (Acrobat, pypdf) take the same lenient stance.

## Write Side

### Trigger point

Writer's catalog construction: if `d.outlinesRoot != nil && d.outlinesRoot.Count() > 0`, build outline objects, append to `d.objects`, set catalog's `/Outlines` to the ref.

### `buildOutlineObjects(d) (rootRef pdfRef, objs []*pdfObject)`

1. Allocate `d.nextID++` for the root /Outlines dict.
2. DFS pre-order walk; allocate one object ID per item.
3. Compute sibling chains (Prev/Next) and parent/first-child/last-child links via a map keyed by parent object number.
4. Compute /Count recursively (positive = expanded, negative = collapsed) for items with children.
5. Build each item dict with /Title, /Parent, /Prev, /Next, /First, /Last, /Count, /F (flags), /C (color), /Dest, /A.
6. Build root /Outlines dict with /Type=/Outlines, /First, /Last, /Count.
7. Return root ref and slice of new pdfObjects.

### Counter math (`visibleDescendantCount`)

```
count(node) = #directChildren
            + Σ over each expanded child: |count(child)|
```

Sign on emit: positive if node is expanded, negative if collapsed. Items with no children skip `/Count` entry entirely.

### `encodeOutlineItem(entry, rootObjNum) pdfDict`

Produces the item dict. Notable details:

- `/Parent` uses `pdfRef{Num: rootObjNum}` for top-level entries, otherwise the parent entry's allocated number.
- `/F` is omitted when no flags are set (avoid writing `/F 0`).
- `/C` is omitted when Color is nil.
- `/Dest` is emitted before `/A` for clarity; both can coexist.

### `encodeDestination(d Destination) pdfArray`

Switch on concrete type; produces:
- XYZ: `[pageRef /XYZ left top zoom]` (each coord can be `pdfNull{}` if `useXxx` is false).
- Fit: `[pageRef /Fit]`.
- FitH: `[pageRef /FitH top]`.
- FitV: `[pageRef /FitV left]`.
- FitR: `[pageRef /FitR left bottom right top]`.
- FitB: `[pageRef /FitB]`.
- FitBH: `[pageRef /FitBH top]`.
- FitBV: `[pageRef /FitBV left]`.

The page reference is `pdfDirectRef{Num: page.objNum}` (same trick used for `/Parent` in the page tree — bypasses the writer's ID remap so the destination points at the actual emitted page object).

### Catalog wiring

```go
outlinesRef, outlineObjs := buildOutlineObjects(d)
if outlinesRef.Num != 0 {
    catalog["/Outlines"] = outlinesRef
    for _, obj := range outlineObjs {
        d.objects[obj.Num] = obj
    }
}
```

### Encryption interaction

Outline dicts (including /Title strings) flow through the standard `encryptBytes` per-object path. No special handling needed. Encrypted outlines + AES-256 + AcroForm + annotations roundtrips are covered by `TestOutlines_CrossEpicAcroForm` and a paired AES-256 test.

### Round-trip preservation

- Untouched outline tree (user never called `Outlines()`): `outlinesRoot` stays nil; catalog's `/Outlines` ref points at the original objects which remain in `d.objects` from Open. Output preserves the structure exactly.
- Touched (Outlines() called): new tree rebuild allocates fresh object IDs; old outline objects in `d.objects` become orphans, which the writer's reachability sweep removes.

## Testing Strategy

### Internal tests (`outline_internal_test.go`)

- `TestOutlineFlags_Encoding` — bit math for /F.
- `TestVisibleDescendantCount_Flat` — flat tree.
- `TestVisibleDescendantCount_Nested` — nested with expanded children.
- `TestVisibleDescendantCount_Collapsed` — sign convention.
- `TestEncodeDestinationXYZ_AllExplicit`
- `TestEncodeDestinationXYZ_UnchangedFields` — `pdfNull` slots.
- `TestEncodeDestinationFit` — 2-element array.
- `TestEncodeDestinationFitR` — 6-element array.
- `TestParseDestinationArray_AllVariants` — round-trip all 8 types.
- `TestParseOutlines_EmptyDoc`
- `TestParseOutlines_MalformedSkip`

### External tests (`outline_test.go`)

- `TestOutlines_EmptyDocReturnsRoot`
- `TestOutlines_AddSingleEntry`
- `TestOutlines_NestedHierarchy`
- `TestOutlines_StylePropertiesRoundTrip`
- `TestOutlines_IsExpandedRoundTrip`
- `TestOutlines_AllDestinationTypes`
- `TestOutlines_ActionRoundTrip`
- `TestOutlines_DestinationAndActionCoexist`
- `TestOutlines_TreeManipulation_Insert`
- `TestOutlines_TreeManipulation_RemoveAt`
- `TestOutlines_TreeManipulation_Remove`
- `TestOutlines_AddCrossDocumentError`
- `TestOutlines_AddNilError`
- `TestOutlines_AddSelfError` (cycle)
- `TestOutlines_ParentNavigation`
- `TestOutlines_PreservesOnReSaveWithoutCall`
- `TestOutlines_CrossEpicAcroForm` (AES-128 + AcroForm + outlines)
- `TestOutlines_CrossEpicAnnotation` (link annotations + outlines)

### Aspose parity tests (`outline_aspose_parity_test.go`)

Line-by-line translations of Aspose .NET sample code. Each test proves the API is genuinely Aspose-shaped. Sample tests:

- `TestAsposeParity_AddBookmark`
- `TestAsposeParity_DestinationXYZ`
- `TestAsposeParity_NestedChildren`
- `TestAsposeParity_BoldItalicColor`
- `TestAsposeParity_GoToAction`

These tests don't add behavioral coverage — they're documentation-as-code keeping the parity claim honest.

### Cross-tool tests (`outline_pypdf_test.go`)

- `TestOutlines_ReadableByPypdf` — our outline output → pypdf's `reader.outline` API → verify titles and page refs.
- `TestOutlines_ReadsPypdfOutlines` — pypdf-built PDF with bookmarks → our `Outlines()` returns equivalent tree.

Auto-skip if pypdf unavailable.

### Regression baseline

- All existing tests (annotations, forms, encryption, text) pass unchanged.
- Documents without outlines have no `/Outlines` entry in the catalog.
- Output of a no-outline doc is functionally identical to pre-Subepic-1 output.

## Risks

1. **Cyclic refs in malformed input.** `seen` set + depth cap = bounded parse time.
2. **Stale page refs after `Document.Append(other)`.** Documented limitation; users set destinations after all pages are added.
3. **Encryption + outlines.** Cross-epic tests cover AES-128 + outlines and AES-256 + outlines.
4. **Orphan outline objects after mutate-and-save.** Relies on writer's reachability sweep. If absent, document as minor PDF-size leak (functionally harmless).
5. **/Count sign edge cases.** Items without children must omit /Count entirely (we never write `/Count 0`).
6. **`OutlineItemCollection` is verbose for Go.** Intentional Aspose-parity choice; README uses short local var names (`oic`) to mitigate.
7. **Constructor signature differs from .NET.** `.NET: new OutlineItemCollection(doc.Outlines)` vs `Go: pdf.NewOutlineItemCollection(doc)` — documented migration note. Behaviorally identical (both produce an unattached entry).

## Aspose.PDF for .NET fidelity

The mapping table in the Public API section is the authoritative reference. Tests in `outline_aspose_parity_test.go` provide executable proof. A .NET developer can write Go outline code by mechanical translation per the table.

## Open Questions

None — all design decisions agreed during brainstorming.

## References

- ISO 32000-1:2008 §12.3.2.2 — explicit destinations
- ISO 32000-1:2008 §12.3.2.3 — named destinations (Subepic 2 scope)
- ISO 32000-1:2008 §12.3.3 — outlines
- Adobe `pdf_reference_1-7.pdf` Chapter 8.2
- Aspose.PDF for .NET docs: `OutlineItemCollection`, `Document.Outlines`, `*ExplicitDestination` family
