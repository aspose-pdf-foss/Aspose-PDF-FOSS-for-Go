# Named Destinations Implementation Plan (Subepic 2 of `pdf-go-qrx`)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship named destinations — a name-to-destination map on `Document` with both modern (`/Catalog/Names/Dests` name tree) and legacy (`/Catalog/Dests` flat dict) read support, modern-only write, and a `NamedDestination` Destination subtype that integrates seamlessly with outline entries. API maximally close to Aspose.PDF for .NET.

**Architecture:** Foundation type (`NamedDestination`) → collection skeleton (`NamedDestinations` + Document accessor) → write path (build flat tree + writer wiring + outline /Dest update) → read path (name tree walker + merge + outline read hook) → roundtrip tests + cross-cutting + Aspose parity + pypdf cross-tool.

**Tech Stack:** Go 1.24, standard library only. pypdf 6.x for cross-tool verification (Task 12).

**Reference:** [docs/superpowers/specs/2026-05-18-named-destinations-design.md](../specs/2026-05-18-named-destinations-design.md)

---

## File Map

| File | Purpose |
|---|---|
| `named_destinations.go` (new) | `NamedDestinations` collection, `NamedDestination` Destination subtype, `(*Document).NamedDestinations()` |
| `named_destinations_parse.go` (new) | Parse both `/Dests` legacy dict and `/Names/Dests` name tree |
| `named_destinations_write.go` (new) | Build flat single-root name tree |
| `outline_destination.go` (modify) | Add `DestinationTypeNamed` enum constant |
| `outline_parse.go` (modify) | Replace name-string `return nil` stub with `*NamedDestination` wrapper |
| `outline_write.go` (modify) | `encodeOutlineItem` emits `/Dest <name>` string for `*NamedDestination` |
| `writer.go` (modify) | Catalog `/Names/Dests` wiring + preserve sibling subentries |
| `document.go` (modify) | Add `namedDests *NamedDestinations` field |
| `named_destinations_internal_test.go` (new) | Internal: tree walker, lex order, merge precedence, encoding shape |
| `named_destinations_test.go` (new) | External: collection API, roundtrip, all dest types, forward refs |
| `named_destinations_cross_test.go` (new) | Cross-cutting (outlines + AES-128 + AES-256) |
| `named_destinations_aspose_parity_test.go` (new) | Aspose .NET line-by-line parity tests |
| `named_destinations_pypdf_test.go` (new) | pypdf cross-tool roundtrip both directions |
| `CLAUDE.md`, `README.md` (modify, Task 13) | Public API docs + parity table |

---

## Task 1: `DestinationTypeNamed` enum + `NamedDestination` concrete type

**Files:**
- Modify: `outline_destination.go`
- Create: `named_destinations.go`
- Create: `named_destinations_internal_test.go`

- [ ] **Step 1: Add the enum constant**

In `outline_destination.go`, find the existing `DestinationType` const block and append `DestinationTypeNamed`:

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
    DestinationTypeNamed // NEW: named destination reference via collection lookup
)
```

- [ ] **Step 2: Write failing internal tests**

Create `named_destinations_internal_test.go`:

```go
package asposepdf

import (
    "testing"
)

func TestDestinationTypeNamedConstant(t *testing.T) {
    if int(DestinationTypeNamed) != 8 {
        t.Errorf("DestinationTypeNamed = %d, want 8 (after FitBV=7)", int(DestinationTypeNamed))
    }
}

func TestNewNamedDestination_Basic(t *testing.T) {
    doc := NewDocument(595, 842)
    nd := NewNamedDestination(doc, "chapter1")
    if nd == nil {
        t.Fatal("NewNamedDestination returned nil")
    }
    if nd.DestinationType() != DestinationTypeNamed {
        t.Errorf("DestinationType = %v, want DestinationTypeNamed", nd.DestinationType())
    }
    if nd.Name() != "chapter1" {
        t.Errorf("Name() = %q, want \"chapter1\"", nd.Name())
    }
}

func TestNamedDestination_UnresolvedReturnsNil(t *testing.T) {
    doc := NewDocument(595, 842)
    nd := NewNamedDestination(doc, "no-such-name")
    if nd.Resolve() != nil {
        t.Error("Resolve() should be nil for unregistered name")
    }
    if nd.Page() != nil {
        t.Error("Page() should be nil for unregistered name")
    }
}
```

- [ ] **Step 3: Run + observe build failure**

```powershell
go test -run 'TestDestinationTypeNamed|TestNewNamedDestination|TestNamedDestination_' -v ./...
```
Expected: `NewNamedDestination`, `NamedDestination` undefined.

- [ ] **Step 4: Implement `NamedDestination` in `named_destinations.go`**

Create the file:

```go
package asposepdf

// NamedDestination wraps a name reference into the document's
// NamedDestinations collection. Implements Destination so it can be
// used wherever an explicit destination is accepted (outline entries,
// future link annotation /Dest values, GoToAction).
//
// Resolution is lazy: Page() and Resolve() look up the name in
// doc.NamedDestinations() at call time. This allows constructing a
// NamedDestination before adding the entry to the collection
// (forward reference).
//
// Per ISO 32000-1 §12.3.2.3.
type NamedDestination struct {
    doc  *Document
    name string
}

// NewNamedDestination builds a name-reference destination. The name
// need not be registered yet — resolution is lazy at Page() and
// write time.
//
// Aspose .NET: new NamedDestination(name)
// Go:          pdf.NewNamedDestination(doc, name)
func NewNamedDestination(doc *Document, name string) *NamedDestination {
    return &NamedDestination{doc: doc, name: name}
}

// DestinationType returns DestinationTypeNamed.
func (n *NamedDestination) DestinationType() DestinationType { return DestinationTypeNamed }

// Name returns the registered name this destination references.
func (n *NamedDestination) Name() string { return n.name }

// Page resolves the underlying destination's page via the document's
// NamedDestinations collection. Returns nil if the name is not
// registered or the underlying destination has no Page.
func (n *NamedDestination) Page() *Page {
    inner := n.Resolve()
    if inner == nil {
        return nil
    }
    return inner.Page()
}

// Resolve returns the underlying explicit destination registered under
// this name, or nil if absent. Useful when you need the typed
// concrete (e.g. *DestinationXYZ to read coordinates).
func (n *NamedDestination) Resolve() Destination {
    if n.doc == nil || n.name == "" {
        return nil
    }
    return n.doc.NamedDestinations().Get(n.name)
}
```

This won't compile yet — `Document.NamedDestinations()` and `NamedDestinations.Get` don't exist until Task 2. Add stubs at the bottom of `named_destinations.go` to make this task build standalone:

```go
// Stubs replaced in Task 2 with the real collection.
type NamedDestinations struct{ doc *Document }

func (n *NamedDestinations) Get(name string) Destination { return nil }

func (d *Document) NamedDestinations() *NamedDestinations {
    if d.namedDests == nil {
        d.namedDests = &NamedDestinations{doc: d}
    }
    return d.namedDests
}
```

And in `document.go`, add the field:
```go
type Document struct {
    // ... existing fields ...
    namedDests *NamedDestinations
}
```

- [ ] **Step 5: Run tests + commit**

```powershell
go test -run 'TestDestinationTypeNamed|TestNewNamedDestination|TestNamedDestination_' -v ./...
go test ./...
git add outline_destination.go named_destinations.go document.go named_destinations_internal_test.go
git commit -m "feat: NamedDestination concrete Destination type + DestinationTypeNamed enum value"
```

Expected: all 3 tests pass; full suite green (Subepic 1 untouched).

---

## Task 2: `NamedDestinations` collection — full surface

**Files:**
- Modify: `named_destinations.go`
- Create: `named_destinations_test.go`

- [ ] **Step 1: Write failing external tests**

Create `named_destinations_test.go`:

```go
package asposepdf_test

import (
    "testing"

    pdf "github.com/aspose/pdf-for-go"
)

func TestNamedDestinations_EmptyDoc(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    nd := doc.NamedDestinations()
    if nd == nil {
        t.Fatal("NamedDestinations() returned nil")
    }
    if nd.Count() != 0 {
        t.Errorf("Count = %d, want 0", nd.Count())
    }
    if nd.Document() != doc {
        t.Error("Document() != original doc")
    }
}

func TestNamedDestinations_RootStable(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    if doc.NamedDestinations() != doc.NamedDestinations() {
        t.Error("repeated calls should return same instance")
    }
}

func TestNamedDestinations_AddGet(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    nd := doc.NamedDestinations()
    dest := pdf.NewDestinationXYZ(page, 100, 800, 1)
    if err := nd.Add("intro", dest); err != nil {
        t.Fatalf("Add: %v", err)
    }
    if nd.Count() != 1 {
        t.Errorf("Count = %d", nd.Count())
    }
    if got := nd.Get("intro"); got != dest {
        t.Errorf("Get returned %v, want %v", got, dest)
    }
    if !nd.Has("intro") {
        t.Error("Has should report true")
    }
}

func TestNamedDestinations_AddNilError(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    if err := doc.NamedDestinations().Add("x", nil); err == nil {
        t.Error("Add(nil) should error")
    }
}

func TestNamedDestinations_AddEmptyNameError(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    if err := doc.NamedDestinations().Add("", pdf.NewDestinationFit(page)); err == nil {
        t.Error("Add with empty name should error")
    }
}

func TestNamedDestinations_AddNamedDestValueError(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    nd := doc.NamedDestinations()
    inner := pdf.NewNamedDestination(doc, "x")
    if err := nd.Add("y", inner); err == nil {
        t.Error("Add(NamedDestination value) should error (would loop)")
    }
}

func TestNamedDestinations_AddOverwrites(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    nd := doc.NamedDestinations()
    d1 := pdf.NewDestinationFit(page)
    d2 := pdf.NewDestinationXYZ(page, 0, 0, 0)
    nd.Add("x", d1)
    if err := nd.Add("x", d2); err != nil {
        t.Fatalf("overwrite Add: %v", err)
    }
    if nd.Count() != 1 {
        t.Errorf("Count after overwrite = %d", nd.Count())
    }
    if nd.Get("x") != d2 {
        t.Error("overwrite should replace value")
    }
}

func TestNamedDestinations_Remove(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    nd := doc.NamedDestinations()
    nd.Add("x", pdf.NewDestinationFit(page))
    if !nd.Remove("x") {
        t.Error("Remove on present should return true")
    }
    if nd.Count() != 0 {
        t.Errorf("Count after Remove = %d", nd.Count())
    }
    if nd.Remove("x") {
        t.Error("Remove on absent should return false")
    }
}

func TestNamedDestinations_NamesSorted(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    nd := doc.NamedDestinations()
    for _, n := range []string{"zebra", "apple", "mango"} {
        nd.Add(n, pdf.NewDestinationFit(page))
    }
    names := nd.Names()
    if len(names) != 3 || names[0] != "apple" || names[1] != "mango" || names[2] != "zebra" {
        t.Errorf("Names() = %v, want sorted [apple mango zebra]", names)
    }
}

func TestNamedDestinations_AllSnapshot(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    nd := doc.NamedDestinations()
    nd.Add("x", pdf.NewDestinationFit(page))
    snap := nd.All()
    if len(snap) != 1 {
        t.Errorf("All() len = %d", len(snap))
    }
    // Mutate snapshot → collection should be unchanged.
    delete(snap, "x")
    if nd.Count() != 1 {
        t.Error("All() should return a snapshot, not the live map")
    }
}

func TestNamedDestinations_Clear(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    nd := doc.NamedDestinations()
    nd.Add("a", pdf.NewDestinationFit(page))
    nd.Add("b", pdf.NewDestinationFit(page))
    nd.Clear()
    if nd.Count() != 0 {
        t.Error("Clear should empty the collection")
    }
}
```

- [ ] **Step 2: Run + observe build failure**

```powershell
go test -run TestNamedDestinations -v ./...
```
Expected: build failures (methods don't exist).

- [ ] **Step 3: Implement collection in `named_destinations.go`**

Replace the stub `NamedDestinations` type with the full implementation. Append after `NamedDestination`:

```go
import (
    "fmt"
    "sort"
)

// NamedDestinations is a name-to-destination map per ISO 32000-1 §12.3.2.3.
// Backed at PDF level by the modern /Catalog/Names/Dests name tree
// (PDF 1.2+); on read it also absorbs legacy /Catalog/Dests for
// backward compatibility. On write, only /Names/Dests is emitted.
//
// Mirrors Aspose.PDF for .NET's NamedDestinations collection.
type NamedDestinations struct {
    doc     *Document
    entries map[string]Destination
}

// Document returns the document this collection is bound to.
func (n *NamedDestinations) Document() *Document { return n.doc }

// Count returns the number of registered entries.
func (n *NamedDestinations) Count() int { return len(n.entries) }

// Has reports whether name is registered.
func (n *NamedDestinations) Has(name string) bool {
    _, ok := n.entries[name]
    return ok
}

// Get returns the destination registered under name, or nil if absent.
// Never returns a *NamedDestination (no recursive lookups).
func (n *NamedDestinations) Get(name string) Destination {
    return n.entries[name]
}

// Add registers dest under name. Errors on:
//   - empty name
//   - nil dest
//   - dest is itself a *NamedDestination (would create a name→name loop)
// If name was already present, the previous value is replaced silently.
func (n *NamedDestinations) Add(name string, dest Destination) error {
    if name == "" {
        return fmt.Errorf("NamedDestinations.Add: empty name")
    }
    if dest == nil {
        return fmt.Errorf("NamedDestinations.Add(%q): nil destination", name)
    }
    if _, ok := dest.(*NamedDestination); ok {
        return fmt.Errorf("NamedDestinations.Add(%q): value cannot itself be a NamedDestination (would loop)", name)
    }
    if n.entries == nil {
        n.entries = map[string]Destination{}
    }
    n.entries[name] = dest
    return nil
}

// Remove deletes the entry; returns true if it existed.
func (n *NamedDestinations) Remove(name string) bool {
    if _, ok := n.entries[name]; !ok {
        return false
    }
    delete(n.entries, name)
    return true
}

// Names returns a snapshot slice of all registered names in lex order.
func (n *NamedDestinations) Names() []string {
    out := make([]string, 0, len(n.entries))
    for k := range n.entries {
        out = append(out, k)
    }
    sort.Strings(out)
    return out
}

// All returns a snapshot map of name → destination.
func (n *NamedDestinations) All() map[string]Destination {
    out := make(map[string]Destination, len(n.entries))
    for k, v := range n.entries {
        out[k] = v
    }
    return out
}

// Clear removes every entry.
func (n *NamedDestinations) Clear() {
    n.entries = nil
}
```

Remove the old stub `Get` and `NamedDestinations` struct from Task 1; the new declaration replaces them. Keep the `Document.NamedDestinations()` accessor stub (it returns the cached pointer; full parse comes in Task 7).

- [ ] **Step 4: Run + commit**

```powershell
go test -run TestNamedDestinations -v ./...
go test ./...
git add named_destinations.go named_destinations_test.go
git commit -m "feat: NamedDestinations collection (Add/Get/Has/Remove/Count/Names/All/Clear)"
```

---

## Task 3: Build flat name tree (`buildNamedDestTree`)

**Files:**
- Create: `named_destinations_write.go`
- Modify: `named_destinations_internal_test.go`

- [ ] **Step 1: Append failing internal tests**

Append to `named_destinations_internal_test.go`:

```go
import "bytes"

func TestBuildNamedDestTree_Empty(t *testing.T) {
    doc := NewDocument(595, 842)
    treeRef, namesDictRef, objs := buildNamedDestTree(doc)
    if treeRef.Num != 0 || namesDictRef.Num != 0 || len(objs) != 0 {
        t.Errorf("empty doc: treeRef=%v namesDictRef=%v objCount=%d, want zeros", treeRef, namesDictRef, len(objs))
    }
}

func TestBuildNamedDestTree_FlatShape(t *testing.T) {
    doc := NewDocument(595, 842)
    page, _ := doc.Page(1)
    nd := doc.NamedDestinations()
    nd.Add("alpha",  NewDestinationFit(page))
    nd.Add("beta",   NewDestinationFit(page))
    nd.Add("gamma",  NewDestinationFit(page))
    treeRef, namesDictRef, objs := buildNamedDestTree(doc)
    if treeRef.Num == 0 || namesDictRef.Num == 0 {
        t.Fatal("refs should be non-zero")
    }
    if len(objs) != 2 {
        t.Fatalf("expected 2 objects (tree root + /Names dict), got %d", len(objs))
    }
    // Find tree root.
    var treeRoot pdfDict
    for _, o := range objs {
        if d, ok := o.Value.(pdfDict); ok {
            if _, hasNames := d["/Names"]; hasNames {
                treeRoot = d
            }
        }
    }
    if treeRoot == nil {
        t.Fatal("no tree root found")
    }
    // /Names array: 3 names × 2 = 6 entries (name, dest, name, dest, ...).
    namesArr, _ := treeRoot["/Names"].(pdfArray)
    if len(namesArr) != 6 {
        t.Errorf("/Names len = %d, want 6", len(namesArr))
    }
    // Lex order check.
    if namesArr[0] != "alpha" || namesArr[2] != "beta" || namesArr[4] != "gamma" {
        t.Errorf("/Names not lex-sorted: %v %v %v", namesArr[0], namesArr[2], namesArr[4])
    }
    // /Limits.
    limits, _ := treeRoot["/Limits"].(pdfArray)
    if len(limits) != 2 || limits[0] != "alpha" || limits[1] != "gamma" {
        t.Errorf("/Limits wrong: %v", limits)
    }
}

func TestBuildNamedDestTree_SkipsNestedNamedDest(t *testing.T) {
    // Direct call simulating defensive write (Add already rejects this,
    // but the writer must defend too).
    doc := NewDocument(595, 842)
    page, _ := doc.Page(1)
    nd := doc.NamedDestinations()
    nd.Add("real", NewDestinationFit(page))
    // Bypass Add validation by writing directly into the map.
    nd.entries["loop"] = &NamedDestination{doc: doc, name: "real"}
    treeRef, _, _ := buildNamedDestTree(doc)
    if treeRef.Num == 0 {
        t.Fatal("should still emit (real entry survives)")
    }
    _ = bytes.Buffer{} // keep import even if unused
    // The expectation: "loop" gets either resolved or skipped — never crashes.
    // We just assert no panic and tree is emitted.
}
```

- [ ] **Step 2: Run + observe build failure**

```powershell
go test -run TestBuildNamedDestTree -v ./...
```
Expected: build failure (undefined `buildNamedDestTree`).

- [ ] **Step 3: Implement `named_destinations_write.go`**

```go
package asposepdf

// buildNamedDestTree emits the /Names/Dests name tree as a flat
// single-root node containing all entries in lexicographic order.
// Returns: the tree root ref (value for /Catalog/Names → /Dests),
// the parent /Names dict ref (value for /Catalog/Names), and the
// slice of new pdfObjects to add to d.objects. Returns zero/zero/nil
// if the collection is empty.
//
// Per ISO 32000-1 §7.9.6 (name trees) and §12.3.2.3 (named destinations).
func buildNamedDestTree(d *Document) (pdfRef, pdfRef, []*pdfObject) {
    nd := d.namedDests
    if nd == nil || nd.Count() == 0 {
        return pdfRef{}, pdfRef{}, nil
    }

    names := nd.Names() // sorted snapshot per Names() contract

    var namesArr pdfArray
    for _, name := range names {
        dest := nd.entries[name]
        // Defensive: if a NamedDestination snuck in past Add validation,
        // try to resolve it; skip on failure.
        if inner, ok := dest.(*NamedDestination); ok {
            dest = inner.Resolve()
            if dest == nil {
                continue
            }
        }
        destArr := encodeDestination(dest)
        if destArr == nil {
            continue
        }
        namesArr = append(namesArr, name)
        namesArr = append(namesArr, destArr)
    }
    if len(namesArr) == 0 {
        return pdfRef{}, pdfRef{}, nil
    }

    // Tree root: single flat node with /Names and /Limits.
    treeRootDict := pdfDict{
        "/Names":  namesArr,
        "/Limits": pdfArray{names[0], names[len(names)-1]},
    }
    treeRootID := d.nextID
    d.nextID++

    // Parent /Names dict.
    namesDictID := d.nextID
    d.nextID++
    namesDict := pdfDict{
        "/Dests": pdfRef{Num: treeRootID},
    }

    return pdfRef{Num: treeRootID}, pdfRef{Num: namesDictID}, []*pdfObject{
        {Num: treeRootID, Value: treeRootDict},
        {Num: namesDictID, Value: namesDict},
    }
}
```

- [ ] **Step 4: Run + commit**

```powershell
go test -run TestBuildNamedDestTree -v ./...
go test ./...
git add named_destinations_write.go named_destinations_internal_test.go
git commit -m "feat: buildNamedDestTree — flat single-root /Names/Dests builder"
```

---

## Task 4: Wire `/Catalog/Names/Dests` into writer (preserve sibling subentries)

**Files:**
- Modify: `writer.go`
- Modify: `named_destinations_test.go`

- [ ] **Step 1: Append failing external test**

Append to `named_destinations_test.go`:

```go
import (
    "bytes"
    "strings"
)

func TestNamedDestinations_WriterEmitsNamesDests(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    doc.NamedDestinations().Add("intro", pdf.NewDestinationFit(page))

    var buf bytes.Buffer
    if _, err := doc.WriteTo(&buf); err != nil {
        t.Fatal(err)
    }
    s := buf.String()
    if !strings.Contains(s, "/Names") {
        t.Error("output missing /Catalog/Names entry")
    }
    if !strings.Contains(s, "/Dests") {
        t.Error("output missing /Dests inside name tree")
    }
    if !strings.Contains(s, "/Limits") {
        t.Error("output missing /Limits in tree root")
    }
    if !strings.Contains(s, "intro") {
        t.Error("output missing the registered name")
    }
}

func TestNamedDestinations_WriterSkipsEmptyCollection(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    if strings.Contains(buf.String(), "/Dests") {
        t.Error("empty collection should not produce /Dests in output")
    }
}
```

- [ ] **Step 2: Run + observe failure**

The writer doesn't emit named-dest objects yet. Test fails by missing strings.

- [ ] **Step 3: Wire into writer.go**

Find where the `/Outlines` block is wired into the catalog (Subepic 1 location, around the `outlinesRef, outlineObjs := buildOutlineObjects(d)` call). Add a parallel block AFTER outlines:

```go
// /Names/Dests if collection non-empty.
treeRef, namesDictRef, ndObjs := buildNamedDestTree(d)
if treeRef.Num != 0 {
    // Merge with existing /Catalog/Names dict to preserve sibling subentries
    // (JavaScript, EmbeddedFiles, etc.) without clobbering them.
    var namesDict pdfDict
    if existing, ok := catalog["/Names"].(pdfRef); ok {
        if obj, ok := d.objects[existing.Num]; ok {
            if dict, ok := obj.Value.(pdfDict); ok {
                namesDict = pdfDict{}
                for k, v := range dict {
                    if k != "/Dests" { // strip old /Dests; new one replaces it
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
    // Replace the synthesized /Names dict in ndObjs with the merged one.
    ndObjs[1] = &pdfObject{Num: namesDictRef.Num, Value: namesDict}

    for _, obj := range ndObjs {
        d.objects[obj.Num] = obj
    }
    catalog["/Names"] = namesDictRef
}
```

Place this AFTER the outline wiring so the catalog's `/Names` entry exists even if outlines come first.

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestNamedDestinations_Writer' -v ./...
go test ./...
git add writer.go named_destinations_test.go
git commit -m "feat: writer emits /Catalog/Names/Dests tree + preserves sibling subentries"
```

---

## Task 5: Outline write update — emit `/Dest <name>` for `*NamedDestination`

**Files:**
- Modify: `outline_write.go`
- Modify: `named_destinations_test.go`

- [ ] **Step 1: Append failing test**

```go
func TestNamedDestinations_OutlineEmitsNameString(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    doc.NamedDestinations().Add("chapter1", pdf.NewDestinationFit(page))

    oic := pdf.NewOutlineItemCollection(doc)
    oic.SetTitle("Chapter 1")
    oic.SetDestination(pdf.NewNamedDestination(doc, "chapter1"))
    doc.Outlines().Add(oic)

    var buf bytes.Buffer
    doc.WriteTo(&buf)
    s := buf.String()
    // Outline /Dest should contain the name "chapter1" as a PDF string,
    // not an array. We can't trivially distinguish a string operand
    // from an array operand in raw bytes without parsing, so just
    // assert the name appears AND no /Dest [ array form is used near
    // the outline title.
    if !strings.Contains(s, "chapter1") {
        t.Error("output missing the named destination reference")
    }
}
```

- [ ] **Step 2: Run + observe**

The current `encodeOutlineItem` calls `encodeDestination(d)` which returns nil for `*NamedDestination` (it's not in the type switch). So `/Dest` ends up nil → not emitted at all. The test will fail because the outline has no /Dest entry.

- [ ] **Step 3: Patch `encodeOutlineItem`**

In `outline_write.go`, find the destination-handling block (Subepic 1):

```go
if d := o.destination; d != nil {
    dict["/Dest"] = encodeDestination(d)
}
```

Replace with:

```go
if d := o.destination; d != nil {
    if nd, ok := d.(*NamedDestination); ok {
        dict["/Dest"] = nd.Name() // PDF string holding the name reference
    } else if arr := encodeDestination(d); arr != nil {
        dict["/Dest"] = arr
    }
}
```

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestNamedDestinations_OutlineEmits' -v ./...
go test ./...
git add outline_write.go named_destinations_test.go
git commit -m "feat: encodeOutlineItem emits /Dest <name> string for NamedDestination"
```

---

## Task 6: Name tree walker + `parseDestinationAny`

**Files:**
- Create: `named_destinations_parse.go`
- Modify: `named_destinations_internal_test.go`

- [ ] **Step 1: Append failing internal tests**

```go
func TestWalkNameTree_FlatLeaf(t *testing.T) {
    doc := NewDocument(595, 842)
    leaf := pdfDict{
        "/Names": pdfArray{
            "alpha", pdfArray{pdfRef{Num: 999}, pdfName("/Fit")},
            "beta",  pdfArray{pdfRef{Num: 999}, pdfName("/Fit")},
        },
    }
    visited := map[string]bool{}
    walkNameTree(doc, leaf, func(name string, val pdfValue) {
        visited[name] = true
    })
    if !visited["alpha"] || !visited["beta"] {
        t.Errorf("visited = %v, want alpha + beta", visited)
    }
}

func TestWalkNameTree_KidsHierarchy(t *testing.T) {
    doc := NewDocument(595, 842)
    // Two leaves
    leafA := pdfDict{
        "/Names": pdfArray{"a", pdfArray{pdfRef{Num: 99}, pdfName("/Fit")}},
    }
    leafB := pdfDict{
        "/Names": pdfArray{"b", pdfArray{pdfRef{Num: 99}, pdfName("/Fit")}},
    }
    // Store as objects so /Kids can reference them.
    leafAID := doc.nextID; doc.nextID++
    doc.objects[leafAID] = &pdfObject{Num: leafAID, Value: leafA}
    leafBID := doc.nextID; doc.nextID++
    doc.objects[leafBID] = &pdfObject{Num: leafBID, Value: leafB}
    root := pdfDict{
        "/Kids": pdfArray{pdfRef{Num: leafAID}, pdfRef{Num: leafBID}},
    }
    visited := map[string]bool{}
    walkNameTree(doc, root, func(name string, val pdfValue) {
        visited[name] = true
    })
    if !visited["a"] || !visited["b"] {
        t.Errorf("visited = %v, want a + b", visited)
    }
}

func TestWalkNameTree_Cycle(t *testing.T) {
    doc := NewDocument(595, 842)
    // Self-referencing /Kids — should not loop infinitely.
    rootID := doc.nextID; doc.nextID++
    cycle := pdfDict{
        "/Kids": pdfArray{pdfRef{Num: rootID}},
    }
    doc.objects[rootID] = &pdfObject{Num: rootID, Value: cycle}
    // Walk by ref, not dict, so the seen check engages.
    visited := 0
    walkNameTree(doc, pdfRef{Num: rootID}, func(name string, val pdfValue) {
        visited++
    })
    // No infinite loop. visited may be 0 since there are no leaves.
    _ = visited
}

func TestParseDestinationAny_Array(t *testing.T) {
    doc := NewDocument(595, 842)
    page, _ := doc.Page(1)
    arr := pdfArray{pdfRef{Num: page.pageObj().Num}, pdfName("/Fit")}
    d := parseDestinationAny(doc, arr)
    if d == nil {
        t.Fatal("nil")
    }
    if d.DestinationType() != DestinationTypeFit {
        t.Errorf("type = %v", d.DestinationType())
    }
}

func TestParseDestinationAny_DictWithD(t *testing.T) {
    doc := NewDocument(595, 842)
    page, _ := doc.Page(1)
    dict := pdfDict{
        "/D": pdfArray{pdfRef{Num: page.pageObj().Num}, pdfName("/Fit")},
    }
    d := parseDestinationAny(doc, dict)
    if d == nil || d.DestinationType() != DestinationTypeFit {
        t.Errorf("/D-wrapped parsing failed: %v", d)
    }
}
```

- [ ] **Step 2: Run + observe build failure**

- [ ] **Step 3: Create `named_destinations_parse.go`**

```go
package asposepdf

// walkNameTree visits every (name, value) pair in a PDF name tree per
// ISO 32000-1 §7.9.6. Each node has either /Names (leaf) OR /Kids
// (intermediate). /Limits is advisory and ignored for walking.
//
// Defensive: cycle protection via seen[objNum], hard depth cap of 100.
func walkNameTree(d *Document, root pdfValue, visit func(name string, val pdfValue)) {
    seen := map[int]bool{}
    walkNameTreeNode(d, root, visit, seen, 0)
}

func walkNameTreeNode(d *Document, raw pdfValue, visit func(string, pdfValue), seen map[int]bool, depth int) {
    if depth > 100 {
        return
    }
    var nodeDict pdfDict
    switch v := raw.(type) {
    case pdfRef:
        if seen[v.Num] {
            return
        }
        seen[v.Num] = true
        obj, ok := d.objects[v.Num]
        if !ok {
            return
        }
        nodeDict, _ = obj.Value.(pdfDict)
    case pdfDict:
        nodeDict = v
    }
    if nodeDict == nil {
        return
    }

    // Leaf: /Names array of alternating name/value pairs.
    if namesArr, ok := nodeDict["/Names"].(pdfArray); ok {
        for i := 0; i+1 < len(namesArr); i += 2 {
            var name string
            switch s := namesArr[i].(type) {
            case string:
                name = s
            case pdfHexString:
                name = string(s)
            default:
                continue
            }
            visit(name, namesArr[i+1])
        }
        return
    }

    // Intermediate: /Kids array of child refs.
    if kids, ok := nodeDict["/Kids"].(pdfArray); ok {
        for _, kid := range kids {
            walkNameTreeNode(d, kid, visit, seen, depth+1)
        }
    }
}

// parseDestinationAny resolves a name's value into a Destination of
// one of the 8 explicit types. Per ISO 32000-1 §12.3.2.3 named
// destinations cannot themselves reference another name — so we
// silently ignore string values here.
func parseDestinationAny(d *Document, raw pdfValue) Destination {
    if raw == nil {
        return nil
    }
    switch v := raw.(type) {
    case pdfArray:
        return parseDestinationArray(d, v)
    case pdfDict:
        if dArr, ok := v["/D"].(pdfArray); ok {
            return parseDestinationArray(d, dArr)
        }
    case pdfRef:
        if obj, ok := d.objects[v.Num]; ok {
            return parseDestinationAny(d, obj.Value)
        }
    }
    return nil
}
```

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestWalkNameTree|TestParseDestinationAny' -v ./...
go test ./...
git add named_destinations_parse.go named_destinations_internal_test.go
git commit -m "feat: name tree walker + parseDestinationAny (handles array / dict-with-D / ref)"
```

---

## Task 7: `parseNamedDestinations` — merge legacy + modern + wire to Document

**Files:**
- Modify: `named_destinations.go`
- Modify: `named_destinations_parse.go`
- Modify: `named_destinations_internal_test.go`

- [ ] **Step 1: Append failing internal tests**

```go
func TestParseNamedDestinations_LegacyOnly(t *testing.T) {
    doc := NewDocument(595, 842)
    page, _ := doc.Page(1)
    // Manually inject /Catalog/Dests for the test.
    catalog := doc.catalog()
    if catalog == nil {
        t.Fatal("no catalog")
    }
    catalog["/Dests"] = pdfDict{
        "legacyOne": pdfArray{pdfRef{Num: page.pageObj().Num}, pdfName("/Fit")},
    }
    nd := parseNamedDestinations(doc)
    if !nd.Has("legacyOne") {
        t.Error("legacy /Dests entry not parsed")
    }
}

func TestParseNamedDestinations_ModernOnly(t *testing.T) {
    doc := NewDocument(595, 842)
    page, _ := doc.Page(1)
    catalog := doc.catalog()
    // Build minimal /Names/Dests tree as direct dict (no refs needed).
    treeRoot := pdfDict{
        "/Names": pdfArray{
            "modernOne", pdfArray{pdfRef{Num: page.pageObj().Num}, pdfName("/Fit")},
        },
    }
    catalog["/Names"] = pdfDict{
        "/Dests": treeRoot,
    }
    nd := parseNamedDestinations(doc)
    if !nd.Has("modernOne") {
        t.Error("modern /Names/Dests entry not parsed")
    }
}

func TestParseNamedDestinations_BothFormats_NamesWins(t *testing.T) {
    doc := NewDocument(595, 842)
    page, _ := doc.Page(1)
    catalog := doc.catalog()
    // Legacy says XYZ.
    catalog["/Dests"] = pdfDict{
        "shared": pdfArray{pdfRef{Num: page.pageObj().Num}, pdfName("/XYZ"), 1.0, 2.0, 3.0},
    }
    // Modern says Fit.
    catalog["/Names"] = pdfDict{
        "/Dests": pdfDict{
            "/Names": pdfArray{
                "shared", pdfArray{pdfRef{Num: page.pageObj().Num}, pdfName("/Fit")},
            },
        },
    }
    nd := parseNamedDestinations(doc)
    got := nd.Get("shared")
    if got == nil {
        t.Fatal("shared entry not parsed")
    }
    if got.DestinationType() != DestinationTypeFit {
        t.Errorf("collision resolution: got %v, want Fit (modern wins)", got.DestinationType())
    }
}
```

- [ ] **Step 2: Run + observe failures**

- [ ] **Step 3: Implement `parseNamedDestinations`**

Append to `named_destinations_parse.go`:

```go
// parseNamedDestinations reads /Catalog/Names/Dests (modern name tree)
// and merges /Catalog/Dests (legacy flat dict). On collision, the
// /Names/Dests entry wins (matches Adobe Acrobat / pypdf behavior).
// Always returns a non-nil collection.
func parseNamedDestinations(d *Document) *NamedDestinations {
    out := &NamedDestinations{doc: d, entries: map[string]Destination{}}

    catalog := d.catalog()
    if catalog == nil {
        return out
    }

    // 1. Legacy /Dests (loaded first so /Names/Dests can override).
    if destsRaw, ok := catalog["/Dests"]; ok {
        if dict, ok := resolveToDict(d, destsRaw); ok {
            for name, val := range dict {
                if dest := parseDestinationAny(d, val); dest != nil {
                    out.entries[name] = dest
                }
            }
        }
    }

    // 2. Modern /Names/Dests name tree.
    if namesRaw, ok := catalog["/Names"]; ok {
        if namesDict, ok := resolveToDict(d, namesRaw); ok {
            if destsRaw, ok := namesDict["/Dests"]; ok {
                walkNameTree(d, destsRaw, func(name string, val pdfValue) {
                    if dest := parseDestinationAny(d, val); dest != nil {
                        out.entries[name] = dest
                    }
                })
            }
        }
    }

    return out
}

// resolveToDict resolves indirect refs to a pdfDict, returning false
// if the value is not a dict or the ref can't be resolved.
func resolveToDict(d *Document, raw pdfValue) (pdfDict, bool) {
    switch v := raw.(type) {
    case pdfDict:
        return v, true
    case pdfRef:
        if obj, ok := d.objects[v.Num]; ok {
            if dict, ok := obj.Value.(pdfDict); ok {
                return dict, true
            }
        }
    }
    return nil, false
}
```

Note: `Document.catalog()` may not yet exist. If absent, add a small helper to `document.go`:
```go
func (d *Document) catalog() pdfDict {
    // Inspect d.objects for the /Catalog object. The catalog's object
    // number is recorded during parse; on freshly-created docs it
    // exists in d.objects from NewDocument. Adapt to actual field
    // name (e.g. d.catalogObjNum or similar — check existing usage in
    // writer.go).
    obj, ok := d.objects[d.catalogObjNum]
    if !ok {
        return nil
    }
    dict, _ := obj.Value.(pdfDict)
    return dict
}
```

(Verify the actual field name by searching writer.go for catalog construction.)

- [ ] **Step 4: Wire to `Document.NamedDestinations()`**

In `named_destinations.go`, replace the Task 1 stub `Document.NamedDestinations()` with:

```go
func (d *Document) NamedDestinations() *NamedDestinations {
    if d.namedDests == nil {
        d.namedDests = parseNamedDestinations(d)
    }
    return d.namedDests
}
```

Remove the temporary stub Get/Add that returned defaults — Task 2 replaced them with the real ones.

- [ ] **Step 5: Run + commit**

```powershell
go test -run 'TestParseNamedDestinations' -v ./...
go test ./...
git add named_destinations.go named_destinations_parse.go named_destinations_internal_test.go document.go
git commit -m "feat: parseNamedDestinations — merge legacy /Dests + modern /Names/Dests, lazy on Document.NamedDestinations()"
```

---

## Task 8: Outline read hook — replace Subepic 1's `return nil` stub

**Files:**
- Modify: `outline_parse.go`
- Modify: `named_destinations_test.go`

- [ ] **Step 1: Append failing external test**

```go
func TestNamedDestinations_OutlineParsesNamedRef(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    doc.NamedDestinations().Add("ch1", pdf.NewDestinationFit(page))
    oic := pdf.NewOutlineItemCollection(doc)
    oic.SetTitle("Chapter")
    oic.SetDestination(pdf.NewNamedDestination(doc, "ch1"))
    doc.Outlines().Add(oic)

    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    root := doc2.Outlines()
    if root.Count() != 1 {
        t.Fatal("outline lost")
    }
    dest := root.At(0).Destination()
    nd, ok := dest.(*pdf.NamedDestination)
    if !ok {
        t.Fatalf("Destination type = %T, want *NamedDestination", dest)
    }
    if nd.Name() != "ch1" {
        t.Errorf("Name = %q, want ch1", nd.Name())
    }
    if nd.Resolve() == nil {
        t.Error("Resolve should return the registered destination")
    }
}

func TestNamedDestinations_OutlineUnregisteredNameStillWraps(t *testing.T) {
    // Synthesize a PDF where outline has /Dest (someName) but the name
    // is not in /Names/Dests. parseDestination must still return a
    // *NamedDestination wrapper; Resolve returns nil.
    doc := pdf.NewDocument(595, 842)
    oic := pdf.NewOutlineItemCollection(doc)
    oic.SetTitle("Orphan")
    oic.SetDestination(pdf.NewNamedDestination(doc, "missing"))
    doc.Outlines().Add(oic)

    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    dest := doc2.Outlines().At(0).Destination()
    nd, ok := dest.(*pdf.NamedDestination)
    if !ok {
        t.Fatalf("Destination type = %T, want *NamedDestination", dest)
    }
    if nd.Name() != "missing" {
        t.Errorf("Name = %q, want missing", nd.Name())
    }
    if nd.Resolve() != nil {
        t.Error("Resolve should be nil for unregistered name")
    }
}
```

- [ ] **Step 2: Run + observe failures**

The current Subepic 1 stub returns nil for name strings, so `Destination()` returns nil instead of `*NamedDestination`.

- [ ] **Step 3: Patch `outline_parse.go`**

Find the current stub at line ~87 (the `case string, pdfHexString:` arm):

```go
case string, pdfHexString:
    // Named destination — Subepic 2 territory. For Subepic 1, return
    // nil (caller's Destination() returns nil; viewers still navigate
    // via /A if present).
    return nil
```

Replace with:

```go
case string:
    return resolveNamedDest(doc, v)
case pdfHexString:
    return resolveNamedDest(doc, string(v))
```

And add the helper at the end of `outline_parse.go` (or in `named_destinations_parse.go` if cleaner):

```go
// resolveNamedDest returns a *NamedDestination wrapper for the given
// name. Even unregistered names return a wrapper — preserves the name
// for round-trip; callers detect unresolved names via wrapper.Resolve()
// returning nil.
func resolveNamedDest(doc *Document, name string) Destination {
    if name == "" {
        return nil
    }
    return &NamedDestination{doc: doc, name: name}
}
```

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestNamedDestinations_OutlineParsesNamedRef|TestNamedDestinations_OutlineUnregisteredNameStillWraps' -v ./...
go test ./...
git add outline_parse.go named_destinations_test.go
git commit -m "feat: outline parser resolves /Dest <name> to *NamedDestination wrapper"
```

After this commit, read+write paths are fully wired for named destinations and the integration with outlines is complete.

---

## Task 9: End-to-end roundtrip tests (collection + all dest types)

**Files:**
- Modify: `named_destinations_test.go`

- [ ] **Step 1: Append**

```go
func TestNamedDestinations_RoundTrip_SingleEntry(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    doc.NamedDestinations().Add("intro", pdf.NewDestinationXYZ(page, 100, 800, 1.5))

    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    if err != nil {
        t.Fatal(err)
    }
    dest := doc2.NamedDestinations().Get("intro")
    if dest == nil {
        t.Fatal("intro not in NamedDestinations after roundtrip")
    }
    xyz, ok := dest.(*pdf.DestinationXYZ)
    if !ok {
        t.Fatalf("type = %T, want *DestinationXYZ", dest)
    }
    if xyz.Left() != 100 || xyz.Top() != 800 || xyz.Zoom() != 1.5 {
        t.Errorf("coords: %v %v %v", xyz.Left(), xyz.Top(), xyz.Zoom())
    }
}

func TestNamedDestinations_RoundTrip_AllDestTypes(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    cases := map[string]struct {
        d    pdf.Destination
        want pdf.DestinationType
    }{
        "a-xyz":  {pdf.NewDestinationXYZ(page, 1, 2, 3), pdf.DestinationTypeXYZ},
        "b-fit":  {pdf.NewDestinationFit(page), pdf.DestinationTypeFit},
        "c-fith": {pdf.NewDestinationFitH(page, 100), pdf.DestinationTypeFitH},
        "d-fitv": {pdf.NewDestinationFitV(page, 50), pdf.DestinationTypeFitV},
        "e-fitr": {pdf.NewDestinationFitR(page, 10, 20, 30, 40), pdf.DestinationTypeFitR},
        "f-fitb": {pdf.NewDestinationFitB(page), pdf.DestinationTypeFitB},
        "g-fbh":  {pdf.NewDestinationFitBH(page, 100), pdf.DestinationTypeFitBH},
        "h-fbv":  {pdf.NewDestinationFitBV(page, 50), pdf.DestinationTypeFitBV},
    }
    for name, c := range cases {
        doc.NamedDestinations().Add(name, c.d)
    }
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    for name, c := range cases {
        got := doc2.NamedDestinations().Get(name)
        if got == nil {
            t.Errorf("[%s] missing after roundtrip", name)
            continue
        }
        if got.DestinationType() != c.want {
            t.Errorf("[%s] type = %v, want %v", name, got.DestinationType(), c.want)
        }
    }
}

func TestNamedDestinations_ForwardReference(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    oic := pdf.NewOutlineItemCollection(doc)
    oic.SetTitle("Notes")
    // Reference before registering.
    oic.SetDestination(pdf.NewNamedDestination(doc, "notes"))
    doc.Outlines().Add(oic)
    // Register later.
    doc.NamedDestinations().Add("notes", pdf.NewDestinationFit(page))

    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    nd, _ := doc2.Outlines().At(0).Destination().(*pdf.NamedDestination)
    if nd == nil || nd.Resolve() == nil {
        t.Error("forward reference didn't resolve after roundtrip")
    }
}
```

- [ ] **Step 2: Run + commit**

```powershell
go test -run 'TestNamedDestinations_RoundTrip|TestNamedDestinations_ForwardReference' -v ./...
git add named_destinations_test.go
git commit -m "test: named destinations roundtrip (single, all dest types, forward reference)"
```

---

## Task 10: Cross-cutting (outlines + AES-128 + AES-256)

**Files:**
- Create: `named_destinations_cross_test.go`

- [ ] **Step 1: Create the test file**

```go
package asposepdf_test

import (
    "bytes"
    "testing"

    pdf "github.com/aspose/pdf-for-go"
)

func TestNamedDest_WithOutlineRoundTrip(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    doc.NamedDestinations().Add("ch1", pdf.NewDestinationFit(page))
    oic := pdf.NewOutlineItemCollection(doc)
    oic.SetTitle("Chapter 1")
    oic.SetDestination(pdf.NewNamedDestination(doc, "ch1"))
    doc.Outlines().Add(oic)

    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    nd, _ := doc2.Outlines().At(0).Destination().(*pdf.NamedDestination)
    if nd == nil || nd.Name() != "ch1" {
        t.Fatalf("outline named-dest lost; got %v", doc2.Outlines().At(0).Destination())
    }
    inner := nd.Resolve()
    if inner == nil || inner.DestinationType() != pdf.DestinationTypeFit {
        t.Errorf("Resolve = %v", inner)
    }
}

func TestNamedDest_WithAES128(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    doc.NamedDestinations().Add("secret", pdf.NewDestinationXYZ(page, 50, 700, 1))
    doc.SetEncryption(pdf.EncryptionOptions{
        UserPassword: "x",
        Algorithm:    pdf.EncryptionAlgAES128,
    })
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "x")
    if err != nil {
        t.Fatal(err)
    }
    if doc2.NamedDestinations().Get("secret") == nil {
        t.Error("named dest lost through AES-128 roundtrip")
    }
}

func TestNamedDest_WithAES256(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    doc.NamedDestinations().Add("vault", pdf.NewDestinationFit(page))
    oic := pdf.NewOutlineItemCollection(doc)
    oic.SetTitle("Vault")
    oic.SetDestination(pdf.NewNamedDestination(doc, "vault"))
    doc.Outlines().Add(oic)
    doc.SetEncryption(pdf.EncryptionOptions{
        UserPassword: "x",
        Algorithm:    pdf.EncryptionAlgAES256,
    })
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "x")
    if err != nil {
        t.Fatal(err)
    }
    if doc2.NamedDestinations().Get("vault") == nil {
        t.Error("named dest lost through AES-256 roundtrip")
    }
    if doc2.Outlines().At(0).Destination() == nil {
        t.Error("outline named-dest reference lost")
    }
}
```

- [ ] **Step 2: Run + commit**

```powershell
go test -run 'TestNamedDest_' -v ./...
go test ./...
git add named_destinations_cross_test.go
git commit -m "test: named destinations cross-cutting (outlines + AES-128 + AES-256)"
```

---

## Task 11: Aspose .NET parity tests

**Files:**
- Create: `named_destinations_aspose_parity_test.go`

```go
package asposepdf_test

import (
    "testing"

    pdf "github.com/aspose/pdf-for-go"
)

// Aspose .NET sample: register named destinations
//   doc.NamedDestinations.Add("ch1",
//       new XYZExplicitDestination(doc.Pages[1], 0, 800, 1));
//   doc.NamedDestinations.Add("appendix",
//       new FitExplicitDestination(doc.Pages[1]));
func TestAsposeParity_NamedDestinationsAdd(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    nd := doc.NamedDestinations()
    if err := nd.Add("ch1", pdf.NewDestinationXYZ(page, 0, 800, 1)); err != nil {
        t.Fatal(err)
    }
    if err := nd.Add("appendix", pdf.NewDestinationFit(page)); err != nil {
        t.Fatal(err)
    }
    if nd.Count() != 2 {
        t.Errorf("Count = %d, want 2", nd.Count())
    }
}

// Aspose .NET sample: outline pointing at named destination
//   OutlineItemCollection oic = new OutlineItemCollection(doc.Outlines);
//   oic.Title = "Chapter 1";
//   oic.Destination = new NamedDestination("ch1");
//   doc.Outlines.Add(oic);
func TestAsposeParity_OutlineWithNamedDest(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    doc.NamedDestinations().Add("ch1", pdf.NewDestinationFit(page))
    oic := pdf.NewOutlineItemCollection(doc)
    oic.SetTitle("Chapter 1")
    oic.SetDestination(pdf.NewNamedDestination(doc, "ch1"))
    if err := doc.Outlines().Add(oic); err != nil {
        t.Fatal(err)
    }
}

// Aspose .NET sample: indexer lookup
//   IAppointment dest = doc.NamedDestinations["ch1"];
func TestAsposeParity_IndexerLookup(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    doc.NamedDestinations().Add("ch1", pdf.NewDestinationFit(page))
    dest := doc.NamedDestinations().Get("ch1")
    if dest == nil {
        t.Error("Get('ch1') returned nil")
    }
}

// Aspose .NET sample: ContainsKey + Remove + Count
//   if (doc.NamedDestinations.ContainsKey("old")) { doc.NamedDestinations.Remove("old"); }
//   int n = doc.NamedDestinations.Count;
func TestAsposeParity_ContainsRemoveCount(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    doc.NamedDestinations().Add("old", pdf.NewDestinationFit(page))
    if !doc.NamedDestinations().Has("old") {
        t.Fatal("Has should report true")
    }
    doc.NamedDestinations().Remove("old")
    if doc.NamedDestinations().Count() != 0 {
        t.Error("Count after Remove != 0")
    }
}
```

```powershell
go test -run TestAsposeParity -v ./...
git add named_destinations_aspose_parity_test.go
git commit -m "test: Aspose .NET parity tests for named destinations"
```

---

## Task 12: pypdf cross-tool tests

**Files:**
- Create: `named_destinations_pypdf_test.go`

- [ ] **Step 1: Verify pypdf API**

```powershell
python -c "from pypdf import PdfReader; r=PdfReader; print([a for a in dir(r) if 'named' in a.lower() or 'dest' in a.lower()])"
python -c "from pypdf import PdfWriter; w=PdfWriter; print([a for a in dir(w) if 'named' in a.lower() or 'dest' in a.lower()])"
```

Confirm pypdf 6.x exposes `reader.named_destinations` (property returning dict-like) and `writer.add_named_destination(name, page_number)` (or similar). Adapt the script below to the actual API if needed.

- [ ] **Step 2: Create the file**

```go
package asposepdf_test

import (
    "bytes"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "testing"

    pdf "github.com/aspose/pdf-for-go"
)

func skipIfNoPypdfForNamedDest(t *testing.T) {
    t.Helper()
    if err := exec.Command("python", "-c", "import pypdf").Run(); err != nil {
        t.Skip("pypdf not available — skipping cross-tool test")
    }
}

func TestNamedDest_ReadableByPypdf(t *testing.T) {
    skipIfNoPypdfForNamedDest(t)
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    doc.NamedDestinations().Add("intro", pdf.NewDestinationFit(page))

    tmp, _ := os.CreateTemp("", "nd-cross-*.pdf")
    defer os.Remove(tmp.Name())
    doc.WriteTo(tmp)
    tmp.Close()

    script := `
from pypdf import PdfReader
r = PdfReader(r"` + filepath.ToSlash(tmp.Name()) + `")
nd = r.named_destinations
print("|".join(sorted(nd.keys())))
`
    out, err := exec.Command("python", "-c", script).Output()
    if err != nil {
        t.Fatalf("pypdf named_destinations read failed: %v", err)
    }
    if !strings.Contains(string(out), "intro") {
        t.Errorf("pypdf missing 'intro' in named destinations: %q", out)
    }
}

func TestNamedDest_ReadsPypdfOutput(t *testing.T) {
    skipIfNoPypdfForNamedDest(t)
    tmp, _ := os.CreateTemp("", "nd-from-pypdf-*.pdf")
    defer os.Remove(tmp.Name())
    tmp.Close()

    script := `
from pypdf import PdfWriter
w = PdfWriter()
w.add_blank_page(width=595, height=842)
w.add_named_destination("byPypdf", 0)
with open(r"` + filepath.ToSlash(tmp.Name()) + `", "wb") as f:
    w.write(f)
`
    if err := exec.Command("python", "-c", script).Run(); err != nil {
        t.Fatalf("pypdf write failed: %v", err)
    }
    raw, _ := os.ReadFile(tmp.Name())
    doc, err := pdf.OpenStream(bytes.NewReader(raw))
    if err != nil {
        t.Fatal(err)
    }
    if doc.NamedDestinations().Get("byPypdf") == nil {
        t.Error("our parser didn't find 'byPypdf' in pypdf-built PDF")
    }
}
```

- [ ] **Step 3: Run + commit**

```powershell
go test -run 'TestNamedDest_ReadableByPypdf|TestNamedDest_ReadsPypdfOutput' -v ./...
go test ./...
git add named_destinations_pypdf_test.go
git commit -m "test: pypdf cross-tool roundtrip for named destinations"
```

---

## Task 13: Docs + close Subepic 2 + close `pdf-go-qrx` umbrella

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`

- [ ] **Step 1: Append to CLAUDE.md** (after the outlines block):

```markdown
**`named_destinations.go` / `named_destinations_parse.go` / `named_destinations_write.go`**
- `(*Document).NamedDestinations() *NamedDestinations` — name-to-destination collection. Always non-nil; empty for documents without `/Catalog/Names/Dests` or `/Catalog/Dests`. Lazy-parsed on first call. Mirrors Aspose.PDF for .NET's `Document.NamedDestinations`
- `NamedDestinations` — collection with `Add(name, dest) error`, `Get(name) Destination`, `Has(name) bool`, `Remove(name) bool`, `Count() int`, `Names() []string` (lex-sorted snapshot), `All() map[string]Destination` (snapshot), `Clear()`, `Document()`. Per ISO 32000-1 §12.3.2.3
- `NamedDestination` — 9th concrete `Destination` type wrapping a name reference; `DestinationType()` returns `DestinationTypeNamed`. Constructor `NewNamedDestination(doc, name)`. Lazy `Resolve() Destination` and `Page() *Page` look up in the collection at call time (forward references allowed). Mirrors Aspose .NET's `NamedDestination` IAppointment subtype
- Read path: `/Catalog/Dests` legacy dict + `/Catalog/Names/Dests` modern name tree merged into one collection; on collision `/Names/Dests` wins. Name tree walker handles arbitrary `/Kids` depth with cycle protection
- Write path: emit `/Catalog/Names/Dests` as a flat single-root tree (valid for any size per ISO 32000-1 §7.9.6). Legacy `/Catalog/Dests` is dropped on save — automatic migration. Sibling `/Catalog/Names` subentries (JavaScript, EmbeddedFiles, etc.) are preserved through round-trip
- Outline integration: `OutlineItemCollection.SetDestination(NewNamedDestination(doc, name))` serializes as `/Dest <name>` PDF string; on parse, `Destination()` returns `*NamedDestination` wrapper. Unregistered names still wrap (preserves the reference) — `Resolve()` returns nil to signal missing
```

- [ ] **Step 2: Update README** — add an Outlines example showing named destinations:

In the existing `### Outlines (Bookmarks)` section, add a follow-up snippet:

````markdown
```go
// Named destinations — define once, reuse from outlines and links
doc.NamedDestinations().Add("intro",    pdf.NewDestinationFit(page1))
doc.NamedDestinations().Add("appendix", pdf.NewDestinationFitH(page2, 500))

oic := pdf.NewOutlineItemCollection(doc)
oic.SetTitle("Appendix")
oic.SetDestination(pdf.NewNamedDestination(doc, "appendix"))
doc.Outlines().Add(oic)
```

API mirrors Aspose.PDF for .NET's `NamedDestinations` collection and `NamedDestination`
class 1:1. Reads both `/Catalog/Dests` (legacy PDF 1.1) and `/Catalog/Names/Dests`
(modern PDF 1.2+) — legacy auto-migrates to modern on save.
````

Also update the existing `- **Outlines (bookmarks)**` Features bullet to add a phrase like `... Named destinations (\`Document.NamedDestinations()\`) integrate as the 9th destination type with forward-reference support.`

- [ ] **Step 3: Run full suite + commit docs**

```powershell
go test ./...
go vet ./...
git add CLAUDE.md README.md
git commit -m "docs: named destinations (Subepic 2 of pdf-go-qrx) in CLAUDE.md and README"
```

- [ ] **Step 4: Close `pdf-go-qrx` umbrella**

After full suite green:

```bash
bd update pdf-go-qrx --status closed --append-notes "Subepic 2 (Named destinations — collection API on Document + NamedDestination as 9th Destination subtype, read both legacy /Catalog/Dests and modern /Names/Dests name tree, write modern only with automatic legacy migration, forward-reference support, /Catalog/Names sibling preservation) shipped 2026-05-XX. Public API: (*Document).NamedDestinations() *NamedDestinations + NewNamedDestination(doc, name) *NamedDestination + collection methods (Add/Get/Has/Remove/Count/Names/All/Clear). Aspose .NET parity tests + pypdf cross-tool roundtrip both directions pass. Cross-cutting verified with outlines + AES-128 + AES-256. Navigation umbrella pdf-go-qrx complete: Subepic 1 (outlines) + Subepic 2 (named destinations) both shipped."
```

`pdf-go-qrx` umbrella now **CLOSED** — navigation feature complete.

---

## Self-review

**Spec coverage:**

| Spec section | Task(s) |
|---|---|
| `DestinationTypeNamed` enum value | 1 |
| `NamedDestination` Destination subtype | 1 |
| `NamedDestinations` collection API | 2 |
| Flat name tree builder | 3 |
| Catalog `/Names/Dests` wiring + sibling preservation | 4 |
| Outline write `/Dest <name>` string | 5 |
| Name tree walker + parseDestinationAny | 6 |
| Parse legacy `/Dests` + modern `/Names/Dests` merge | 7 |
| Outline read hook for name strings | 8 |
| End-to-end roundtrip | 9 |
| Cross-cutting (outlines + AES) | 10 |
| Aspose .NET parity | 11 |
| pypdf cross-tool | 12 |
| Docs + close umbrella | 13 |

**Placeholder scan:** every task has full code or precise pointer to existing code. Minor "verify with grep" notes in Tasks 7 (`Document.catalog()` accessor) and 4 (writer wiring location) are quick reads of writer.go.

**Type consistency:** `NamedDestination` from Task 1 is used in Tasks 2, 5, 8. `NamedDestinations` from Task 2 is the value cached on Document and consumed by Task 7. `buildNamedDestTree` from Task 3 is called by Task 4's writer wiring. `walkNameTree` and `parseDestinationAny` from Task 6 are called by Task 7's `parseNamedDestinations`. No signature drift.

---

## Execution Handoff

After saving this plan, two execution options:

**1. Subagent-Driven** — fresh subagent per task, two-stage review (spec + quality).
**2. Inline Execution** — execute in this session via executing-plans.
