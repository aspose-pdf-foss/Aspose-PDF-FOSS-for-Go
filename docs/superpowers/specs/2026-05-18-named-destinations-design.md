# Named Destinations Design Spec (Subepic 2 of `pdf-go-qrx`)

**Date:** 2026-05-18
**Issue:** `pdf-go-qrx` — Outlines / bookmarks + named destinations (navigation)
**Subepic 1 (already shipped):** Outlines/bookmarks with 8 explicit destination types
**Subepic 2 scope:** Named destinations — collection API on Document + `NamedDestination` Destination subtype; read both legacy `/Catalog/Dests` and modern `/Catalog/Names/Dests`, write modern only.
**API philosophy:** Maximally close to Aspose.PDF for .NET — same `NamedDestinations` collection shape, same `NamedDestination` as IAppointment-compatible Destination subtype.

## Goals

- Read `/Catalog/Dests` (legacy PDF 1.1) and `/Catalog/Names/Dests` (modern PDF 1.2+ name tree); merge into a single in-memory collection.
- Write `/Catalog/Names/Dests` only — legacy format auto-migrates on Save.
- `NamedDestination` is the 9th concrete `Destination` type (`DestinationTypeNamed`), so outline entries and any future Link `/Dest` accept it just like the existing 8.
- Forward references work: `NewNamedDestination(doc, name)` before `Add(name, ...)` is legal.
- Outlines and named destinations interop in both directions (outline references named dest via `/Dest <name>`; named dest references explicit destination).
- Round-trips survive AES-128 and AES-256 encryption.

## Non-Goals

- Other `/Catalog/Names` subentries (`JavaScript`, `EmbeddedFiles`, `Pages`, `Templates`, `IDS`, `URLS`). We preserve them at write time if present, but don't surface them in the API.
- Name tree intermediate `/Kids` structure at write — we emit a flat single-root tree. Acrobat does the same for typical documents. Read side handles arbitrary depth.
- Auto-generation of names from outline titles (Aspose .NET doesn't provide this either).

## Architecture

### PDF spec mapping (ISO 32000-1 §12.3.2.3, §7.9.6)

| PDF entry | Go API |
|---|---|
| `Catalog /Names /Dests` name tree | Serialized on Save (modern format) |
| `Catalog /Dests` flat dict | Read-only fallback; absorbed into in-memory collection |
| Name tree leaf `/Names [name1 val1 name2 val2 ...]` | flat map `string → Destination` |
| Name tree intermediate `/Kids` | Walker recurses |
| Outline/Link `/Dest <name>` | Resolved through collection at parse time; emitted as PDF string at write |

### Recursive type interactions

`NamedDestination` implements `Destination` so it works wherever an explicit destination is accepted (`OutlineItemCollection.SetDestination`, future `LinkAnnotation.SetDestination`). `Page()` and `Resolve()` are lazy lookups against `doc.NamedDestinations()`. Forward references are legal — resolution happens at Page() / write time, not at construction.

### File organization

| File | Role |
|---|---|
| `named_destinations.go` (new) | `NamedDestinations` collection + `NamedDestination` Destination subtype + `(*Document).NamedDestinations()` accessor |
| `named_destinations_parse.go` (new) | Read both `/Dests` legacy dict and `/Names/Dests` name tree, merge with /Names winning collisions |
| `named_destinations_write.go` (new) | Build flat single-root `/Names/Dests` tree |
| `outline_destination.go` (modify) | Add `DestinationTypeNamed` enum constant |
| `outline_parse.go` (modify) | Replace Subepic 1's name-string `return nil` stub with `*NamedDestination` wrapper |
| `outline_write.go` (modify) | `encodeOutlineItem` handles `*NamedDestination` by emitting `/Dest <name>` PDF string |
| `writer.go` (modify) | Catalog `/Names/Dests` wiring + preserve existing `/Names` sibling subentries |
| `document.go` (modify) | Add `namedDests *NamedDestinations` field |
| `named_destinations_internal_test.go` (new) | Internal: name tree walker, lex order, merge precedence, encoding shape |
| `named_destinations_test.go` (new) | External: collection API, roundtrip, all dest types |
| `named_destinations_cross_test.go` (new) | Cross-cutting (outlines + AES-128 + AES-256) |
| `named_destinations_aspose_parity_test.go` (new) | Line-by-line .NET translations |
| `named_destinations_pypdf_test.go` (new) | Cross-tool roundtrip with pypdf 6.x |
| `CLAUDE.md`, `README.md` (modify, final task) | Public API docs + parity table |

## Public API

### Document-level

```go
// NamedDestinations returns the document's named-destination collection.
// Always non-nil — empty collection for documents with neither
// /Catalog/Names/Dests nor /Catalog/Dests. Lazy-parsed on first call.
// Mirrors Aspose.PDF for .NET's Document.NamedDestinations property.
func (d *Document) NamedDestinations() *NamedDestinations
```

### NamedDestinations collection

```go
// NamedDestinations is a name-to-destination map per ISO 32000-1 §12.3.2.3.
// Backed at PDF level by /Catalog/Names/Dests (modern name tree); on
// read it also absorbs legacy /Catalog/Dests for backward compatibility.
// On write, only the modern /Names/Dests tree is emitted.
type NamedDestinations struct { /* unexported */ }

// Add registers dest under name. Errors on:
//   - empty name
//   - nil dest
//   - dest is itself a *NamedDestination (would create a name→name loop)
//   - dest's Page (when applicable) does not belong to this Document
// If name was already present, the previous value is replaced silently
// (matches .NET behavior).
func (n *NamedDestinations) Add(name string, dest Destination) error

// Get returns the destination registered under name, or nil if absent.
// The returned Destination is always one of the 8 explicit types —
// never a *NamedDestination (no recursive lookups).
func (n *NamedDestinations) Get(name string) Destination

// Has reports whether name is registered.
func (n *NamedDestinations) Has(name string) bool

// Remove deletes the entry; returns true if it existed.
func (n *NamedDestinations) Remove(name string) bool

// Count returns the number of registered entries.
func (n *NamedDestinations) Count() int

// Names returns a snapshot slice of all registered names in lex order.
func (n *NamedDestinations) Names() []string

// All returns a snapshot map of name → destination.
func (n *NamedDestinations) All() map[string]Destination

// Clear removes every entry.
func (n *NamedDestinations) Clear()

// Document returns the document this collection is bound to.
func (n *NamedDestinations) Document() *Document
```

### NamedDestination — 9th concrete Destination type

```go
type NamedDestination struct {
    doc  *Document
    name string
}

// NewNamedDestination builds a name-reference destination. The name
// need not be registered yet — resolution is lazy.
// Aspose .NET: new NamedDestination(name)
// Go:          pdf.NewNamedDestination(doc, name)
func NewNamedDestination(doc *Document, name string) *NamedDestination

// DestinationType returns DestinationTypeNamed.
func (n *NamedDestination) DestinationType() DestinationType
func (n *NamedDestination) Name() string

// Page resolves through the document's NamedDestinations.
// Returns nil if the name is not registered or the underlying
// destination has no page.
func (n *NamedDestination) Page() *Page

// Resolve returns the underlying explicit destination, or nil if absent.
// Useful when you need the typed concrete type to read its coords.
func (n *NamedDestination) Resolve() Destination
```

### Enum extension

```go
const (
    DestinationTypeXYZ DestinationType = iota
    DestinationTypeFit
    DestinationTypeFitH
    DestinationTypeFitV
    DestinationTypeFitR
    DestinationTypeFitB
    DestinationTypeFitBH
    DestinationTypeFitBV
    DestinationTypeNamed // NEW
)
```

### Aspose .NET parity table

| Aspose .NET (C#) | This library (Go) |
|---|---|
| `doc.NamedDestinations` (property) | `doc.NamedDestinations()` (method) |
| `nd.Add(name, dest)` | `nd.Add(name, dest) error` |
| `nd[name]` indexer | `nd.Get(name) Destination` |
| `nd.Remove(name)` | `nd.Remove(name) bool` |
| `nd.Count` | `nd.Count() int` |
| `nd.ContainsKey(name)` | `nd.Has(name) bool` |
| `nd.Keys` | `nd.Names() []string` |
| `nd.Clear()` | `nd.Clear()` |
| `new NamedDestination(name)` | `pdf.NewNamedDestination(doc, name)` |
| `oic.Destination = new NamedDestination("ch1")` | `oic.SetDestination(pdf.NewNamedDestination(doc, "ch1"))` |

## Read Side

### Entry point and lazy cache

```go
func (d *Document) NamedDestinations() *NamedDestinations {
    if d.namedDests == nil {
        d.namedDests = parseNamedDestinations(d)
    }
    return d.namedDests
}
```

`d.namedDests *NamedDestinations` is a new Document field. Always non-nil after first call.

### `parseNamedDestinations`

1. Empty collection if no `/Catalog`.
2. Read `/Catalog/Dests` flat dict first (legacy).
3. Walk `/Catalog/Names/Dests` name tree second — entries overwrite legacy ones on collision (matches Adobe/pypdf behavior).
4. For each (name, value) pair, parse the value via `parseDestinationAny` and store if successful.

### Name tree walker (`walkNameTree`)

Per ISO 32000-1 §7.9.6:
- Each node has either `/Names` (leaf with alternating name/value pairs) OR `/Kids` (intermediate with child refs).
- `/Limits [firstName lastName]` is advisory — we don't rely on it.
- Cycle protection: `seen[objNum]` set. Depth cap: 100 levels.

### `parseDestinationAny` value forms

- **pdfArray** → existing `parseDestinationArray` (explicit destination).
- **pdfDict** with `/D` entry → unwrap to array.
- **pdfRef** → resolve and recurse.
- **string / pdfHexString** in this position: not allowed by spec (named dest can't reference another name) — silently ignored.

### Outline integration

Subepic 1 left a stub in `parseDestination`:
```go
case string, pdfHexString:
    return nil
```

Subepic 2 replaces with:
```go
case string:
    return resolveNamedDest(doc, v)
case pdfHexString:
    return resolveNamedDest(doc, string(v))
```

```go
// resolveNamedDest always returns a *NamedDestination wrapper — even
// if the name is unregistered. The wrapper preserves the name for
// round-trip; callers detect unregistered names via wrapper.Resolve() == nil.
func resolveNamedDest(doc *Document, name string) Destination {
    if name == "" {
        return nil
    }
    return &NamedDestination{doc: doc, name: name}
}
```

### Defensive parsing

- Cycles in name tree → bounded walk.
- Malformed `/Names` array (odd length, wrong types) → skip bad entry.
- Missing both `/Dests` formats → empty collection.
- Indirect refs at any depth → resolved transparently.
- Encryption-safe: standard `getObject` decryption applies.

## Write Side

### `buildNamedDestTree(d) (treeRootRef, namesDictRef pdfRef, objs []*pdfObject)`

Emits a **flat single-root** name tree (no `/Kids` intermediates). Valid for any size per spec; matches Acrobat's output for typical documents.

1. Collect names in lex order (mandatory per spec).
2. Build `/Names` array: alternating name (PDF string) and destination array.
3. Skip entries whose dest is a `*NamedDestination` (defensive — `Add` rejects them, but guard at write too).
4. Build root node dict: `{ /Names <array>, /Limits [first last] }`.
5. Build parent `/Names` dict: `{ /Dests <ref to root> }`.
6. Return refs + new objects to add to `d.objects`.

### Catalog wiring with sibling preservation

```go
treeRef, namesDictRef, objs := buildNamedDestTree(d)
if treeRef.Num != 0 {
    // Merge with existing /Catalog/Names dict to preserve sibling subentries
    // (JavaScript, EmbeddedFiles, etc.) without clobbering them.
    var namesDict pdfDict
    if existing, ok := catalog["/Names"].(pdfRef); ok {
        if obj, ok := d.objects[existing.Num]; ok {
            if dict, ok := obj.Value.(pdfDict); ok {
                namesDict = pdfDict{}
                for k, v := range dict {
                    if k != "/Dests" { // strip old /Dests
                        namesDict[k] = v
                    }
                }
            }
        }
    }
    if namesDict == nil {
        namesDict = pdfDict{}
    }
    namesDict["/Dests"] = treeRef
    objs[1] = &pdfObject{Num: namesDictRef.Num, Value: namesDict}
    for _, obj := range objs {
        d.objects[obj.Num] = obj
    }
    catalog["/Names"] = namesDictRef
}
```

If source PDF had legacy `/Catalog/Dests`, on Save it is dropped and only `/Names/Dests` is emitted. Migration is automatic.

### Outline `/Dest` serialization update

```go
// In encodeOutlineItem (outline_write.go):
if d := o.destination; d != nil {
    if nd, ok := d.(*NamedDestination); ok {
        dict["/Dest"] = nd.Name()       // PDF string
    } else {
        dict["/Dest"] = encodeDestination(d)
    }
}
```

Same one-line branch goes into any future Link annotation `/Dest` serializer (out of scope for Subepic 2, but ready to drop in).

### Encryption interaction

Name-tree dicts are standard indirect objects → flow through normal `encryptBytes` per-object path. No special handling. Verified by AES-128 and AES-256 cross-cutting tests.

## Testing Strategy

### Internal (`named_destinations_internal_test.go`)

- Name tree walker: flat leaf, /Kids hierarchy, cycle protection.
- Merge precedence: legacy + modern; `/Names/Dests` wins on collision.
- Build tree: shape, /Limits, lex order, empty case.
- Outline encoding: `*NamedDestination` emits `/Dest <name>` string (not array).

### External (`named_destinations_test.go`)

- `NamedDestinations()` always non-nil, stable across calls.
- `Add/Get/Has/Remove/Count/Names/All/Clear` semantics.
- Validation: nil dest, empty name, NamedDestination-as-value, cross-document Page.
- Forward reference: outline references unregistered name, then Add, then Save → output correct.
- Roundtrip: single entry, all 8 dest types, legacy-format migration.
- `/Names` sibling preservation: synthesize PDF with `/Names/JavaScript`; verify it survives Save.

### NamedDestination type (`named_destinations_test.go` continued)

- `DestinationType() == DestinationTypeNamed`.
- `Resolve` returns underlying explicit; nil if unregistered.
- `Page()` mirrors Resolve().Page().
- No panic on unregistered.

### Cross-cutting (`named_destinations_cross_test.go`)

- Outline + named dest roundtrip.
- AES-128 + outline + named dest roundtrip.
- AES-256 + outline + named dest roundtrip.
- Outline parsed from raw PDF with unregistered name → `Destination()` returns `*NamedDestination`, `.Resolve()` returns nil.

### Aspose parity (`named_destinations_aspose_parity_test.go`)

Line-by-line translations of .NET sample code: Add, OutlineWithNamedDest, IndexerLookup.

### pypdf cross-tool (`named_destinations_pypdf_test.go`)

- Our output → pypdf's `reader.named_destinations` finds entries.
- pypdf-built PDF with `add_named_destination` → our `NamedDestinations().Get()` works.

### Regression baseline

- All existing tests pass unchanged.
- Empty `NamedDestinations` produces no `/Names/Dests` (no PDF bloat).
- Documents without named dests round-trip identically.

## Risks

1. **Name tree intermediate `/Kids` on read.** Real-world PDFs may use multi-level trees. Walker handles them; tests cover.
2. **`/Catalog/Names` dict siblings.** We preserve them by merging at write time. Test guards.
3. **Forward references.** Lazy `Resolve()` + name-as-PDF-string serialization. Test covers.
4. **NamedDestination of NamedDestination.** `Add` rejects defensively; walker filters at write. No loop possible.
5. **Encryption.** Name tree dicts decrypt via standard pipeline; tests with both AES variants.
6. **`/Dest` value form.** PDF allows string, name, or hex string for named dest. Read accepts all three; write emits PDF string `(name)` (most common, matches Acrobat).
7. **Legacy `/Catalog/Dests` migration on Save.** Documented in CLAUDE.md — users opening old PDFs and saving get auto-migration. No functional impact.
8. **Name validation.** Empty names and control-char-only names rejected by `Add`. Read-side: silently skip.

## Aspose.PDF for .NET fidelity

Mapping table in the Public API section is authoritative. The parity tests in `named_destinations_aspose_parity_test.go` provide executable proof. A .NET migrant can write Go named-destination code by mechanical translation per the table.

## Open Questions

None — all design decisions agreed during brainstorming.

## References

- ISO 32000-1:2008 §7.9.6 — name trees
- ISO 32000-1:2008 §12.3.2.3 — named destinations
- ISO 32000-1:2008 §12.3.3 — outlines (uses /Dest entries)
- Subepic 1 spec: [2026-05-15-outlines-design.md](2026-05-15-outlines-design.md)
- Aspose.PDF for .NET docs: `NamedDestinations`, `NamedDestination`, `Document.NamedDestinations`
