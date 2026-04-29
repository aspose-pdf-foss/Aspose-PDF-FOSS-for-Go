# Annotations Framework — Read + Link + Highlight Family — Design Spec

**Epic:** `pdf-go-37n`. Subepic 1 of 4 in the Annotations program. Subepics 2-4 (Standard markup, Drawing primitives, Specialized) remain in the backlog as separate epics.

**Goal:** Programmatically read, create, and modify the two most-used PDF annotation families — **link annotations** (with the `/A` action dictionary covering URI, internal navigation, named viewer commands, and form submit/reset) and **text-markup annotations** (highlight, underline, strike-out, squiggly). Plus a generic read-only framework that exposes any other annotation type already present in a PDF as an opaque `WidgetAnnotation` so the existing AcroForm capability keeps working.

## Why this epic

Links and highlights are the most-requested annotation subtypes — they cover hyperlinks, internal navigation, and review/markup workflows. The user's own question about the `/Submit` button on a form pointed at the gap: form push buttons are inert without an action attached, and actions belong to this epic.

The library already touches `/Annots` for AcroForm widgets (form fields are widget annotations). Adding the broader annotation framework on top extends what's there without rewriting anything. By the end of this subepic, callers can:

- Open any PDF with link annotations and enumerate them programmatically (URLs, internal jumps, JavaScript actions).
- Add new clickable hyperlinks to a page.
- Add highlight/underline/strike-out/squiggly markup to existing text passages.
- Wire form push buttons to `SubmitForm` / `ResetForm` / `URI` / `Named` actions.

## API surface (Aspose.PDF for .NET fidelity)

### Page-side

```go
// Annotations returns the live AnnotationCollection for this page. Always
// non-nil. Annotations are page-scoped; collections from different pages
// do not share state.
func (p *Page) Annotations() *AnnotationCollection

type AnnotationCollection struct { /* internal */ }

func (c *AnnotationCollection) Add(a Annotation) error
func (c *AnnotationCollection) Delete(a Annotation) bool
func (c *AnnotationCollection) DeleteAt(index int) error
func (c *AnnotationCollection) Count() int
func (c *AnnotationCollection) At(index int) Annotation
func (c *AnnotationCollection) All() []Annotation
```

`Add` returns `error` rather than panicking so the re-attach-to-different-page case is recoverable. `nil` annotation triggers panic — programmer error.

### Annotation interface and base properties

```go
type Annotation interface {
    AnnotationType() AnnotationType
    Rect() Rectangle
    SetRect(r Rectangle)
    Color() *Color           // nil if /C not set
    SetColor(c *Color)
    Title() string           // /T (typically the author/reviewer name)
    SetTitle(s string)
    Contents() string        // /Contents
    SetContents(s string)
    PageIndex() int          // 1-based; 0 if not yet attached
}

type AnnotationType int

const (
    AnnotationTypeUnknown AnnotationType = iota
    AnnotationTypeLink
    AnnotationTypeHighlight
    AnnotationTypeUnderline
    AnnotationTypeStrikeOut
    AnnotationTypeSquiggly
    AnnotationTypeWidget    // existing form fields surface here
)
```

### Constructors (Aspose-style: page + rect at construction)

```go
func NewLinkAnnotation(page *Page, rect Rectangle) *LinkAnnotation
func NewHighlightAnnotation(page *Page, rect Rectangle) *HighlightAnnotation
func NewUnderlineAnnotation(page *Page, rect Rectangle) *UnderlineAnnotation
func NewStrikeOutAnnotation(page *Page, rect Rectangle) *StrikeOutAnnotation
func NewSquigglyAnnotation(page *Page, rect Rectangle) *SquigglyAnnotation
```

Constructors take `*Page` for `/P` binding context but don't write the annotation into the document — that happens at `page.Annotations().Add(a)`. `nil` page panics.

### Concrete types

`LinkAnnotation`:
```go
type LinkAnnotation struct { /* embeds annotationBase */ }
func (a *LinkAnnotation) Action() Action
func (a *LinkAnnotation) SetAction(act Action)        // nil clears /A
func (a *LinkAnnotation) Highlight() LinkHighlightMode
func (a *LinkAnnotation) SetHighlight(h LinkHighlightMode)

type LinkHighlightMode int
const (
    LinkHighlightNone LinkHighlightMode = iota
    LinkHighlightInvert
    LinkHighlightOutline
    LinkHighlightPush
)
```

`HighlightAnnotation` / `UnderlineAnnotation` / `StrikeOutAnnotation` / `SquigglyAnnotation`:
```go
// Identical method set across the four markup types — different /Subtype only.
type HighlightAnnotation struct { /* */ }
func (a *HighlightAnnotation) QuadPoints() []QuadPoint
func (a *HighlightAnnotation) SetQuadPoints(qp []QuadPoint)

type QuadPoint struct {
    X1, Y1, X2, Y2, X3, Y3, X4, Y4 float64
}
```

`WidgetAnnotation` (read-only handle for existing form fields):
```go
type WidgetAnnotation struct { /* */ }
// Implements Annotation. AnnotationType() returns AnnotationTypeWidget.
// No setters specific to widget — form fields continue to be mutated via Form API.
```

### Action interface and concrete types

```go
type Action interface {
    ActionType() ActionType
}

type ActionType int
const (
    ActionTypeUnknown ActionType = iota
    ActionTypeGoToURI
    ActionTypeGoTo
    ActionTypeNamed
    ActionTypeSubmitForm
    ActionTypeResetForm
    ActionTypeJavaScript
)

func NewGoToURIAction(uri string) *GoToURIAction
func NewGoToAction(pageNum int, top float64) *GoToAction       // /D = [page /XYZ left top zoom]
func NewNamedAction(name NamedActionType) *NamedAction
func NewSubmitFormAction(uri string, fieldNames []string, flags SubmitFormFlags) *SubmitFormAction
func NewResetFormAction(fieldNames []string) *ResetFormAction
// JavaScript actions: parse-only this subepic, no constructor.

type NamedActionType int
const (
    NamedActionFirstPage NamedActionType = iota + 1
    NamedActionLastPage
    NamedActionNextPage
    NamedActionPrevPage
    NamedActionPrint
)

type SubmitFormFlags int
const (
    SubmitIncludeNoValueFields SubmitFormFlags = 1 << 1
    SubmitExportFormat         SubmitFormFlags = 1 << 2
    SubmitGetMethod            SubmitFormFlags = 1 << 3
    SubmitSubmitCoordinates    SubmitFormFlags = 1 << 4
    SubmitXFDF                 SubmitFormFlags = 1 << 5
    SubmitIncludeAppendSaves   SubmitFormFlags = 1 << 6
    SubmitIncludeAnnotations   SubmitFormFlags = 1 << 7
    SubmitSubmitPDF            SubmitFormFlags = 1 << 8
    // ... full /Flags bit set per ISO 32000-1 §12.7.5.2 Table 237
)
```

Each concrete action type carries its own getters for relevant fields:
```go
func (a *GoToURIAction) URI() string
func (a *GoToURIAction) SetURI(s string)
func (a *GoToAction) PageNum() int
func (a *GoToAction) Top() float64
func (a *NamedAction) Name() NamedActionType
func (a *SubmitFormAction) URL() string
func (a *SubmitFormAction) FieldNames() []string
func (a *SubmitFormAction) Flags() SubmitFormFlags
func (a *ResetFormAction) FieldNames() []string
func (a *JavaScriptAction) Script() string
```

### Usage

```go
// Add a hyperlink:
page, _ := doc.Page(1)
link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
link.SetAction(pdf.NewGoToURIAction("https://example.com"))
link.SetColor(&pdf.Color{R: 0, G: 0, B: 1, A: 1})
if err := page.Annotations().Add(link); err != nil {
    log.Fatal(err)
}

// Highlight a passage:
hl := pdf.NewHighlightAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 615})
hl.SetColor(&pdf.Color{R: 1, G: 1, B: 0, A: 1})
hl.SetTitle("Reviewer")
hl.SetContents("Important paragraph")
page.Annotations().Add(hl)

// Iterate and filter:
for _, a := range page.Annotations().All() {
    if a.AnnotationType() != pdf.AnnotationTypeLink {
        continue
    }
    link := a.(*pdf.LinkAnnotation)
    if u, ok := link.Action().(*pdf.GoToURIAction); ok {
        fmt.Println(u.URI())
    }
}
```

## Internal mechanics

### Read framework

`page.Annotations()` builds the collection lazily:

1. Read `pageDict["/Annots"]`. Empty / missing → empty collection.
2. For each `pdfRef`, resolve to `pdfDict`, dispatch by `/Subtype`:
   - `/Link` → `*LinkAnnotation`
   - `/Highlight` → `*HighlightAnnotation`
   - `/Underline` → `*UnderlineAnnotation`
   - `/StrikeOut` → `*StrikeOutAnnotation`
   - `/Squiggly` → `*SquigglyAnnotation`
   - `/Widget` → `*WidgetAnnotation`
   - any other → `*WidgetAnnotation` typed as `AnnotationTypeUnknown` (still gives access to base props; future subepics specialize)
3. Cache the slice on the `*Page` (private field, rebuilt on `Add`/`Delete`). Cache rebuild same canonical-instance pattern as `Form`.

### Action read framework

`/A` lives inline inside the annotation dict (rarely as an indirect object). `parseAction(dict pdfDict) Action`:

1. `/S` is the action subtype name.
2. Dispatch:
   - `/URI` → `*GoToURIAction` with `/URI`
   - `/GoTo` → `*GoToAction` reading `/D` destination array
   - `/Named` → `*NamedAction` matching `/N` against the enum
   - `/SubmitForm` → `*SubmitFormAction` reading `/F`, `/Fields`, `/Flags`
   - `/ResetForm` → `*ResetFormAction` reading `/Fields`
   - `/JavaScript` → `*JavaScriptAction` reading `/JS` (string or stream)
   - any other (`/Launch`, `/GoToR`, etc.) → returns nil; `link.Action()` returns nil

Unknown action types are silently ignored on read — the annotation still loads, just without an action.

### Constructor + Add coordination

`NewLinkAnnotation(page, rect)` builds an unbound annotation: stores the page reference, builds a partial `pdfDict` with `/Type=/Annot`, `/Subtype=/Link`, `/Rect`, but the dict is not yet in `d.objects`. `Add` does the actual work:

```go
func (c *AnnotationCollection) Add(a Annotation) error {
    base := a.annotationBase()                       // internal accessor
    if base.objID != 0 && base.attachedPage == c.pageObj {
        return nil                                   // idempotent same-page
    }
    if base.objID != 0 && base.attachedPage != c.pageObj {
        return errAnnotationAttachedElsewhere
    }
    // First-time attach.
    base.dict["/P"] = pdfRef{Num: c.pageObj.Num}
    objID := c.doc.nextID
    c.doc.nextID++
    c.doc.objects[objID] = &pdfObject{Num: objID, Value: base.dict}
    base.objID = objID
    base.attachedPage = c.pageObj
    // Append to page's /Annots.
    appendAnnotToPage(c.pageObj, pdfRef{Num: objID})
    c.invalidateCache()
    return nil
}
```

### Delete

```go
func (c *AnnotationCollection) Delete(a Annotation) bool {
    base := a.annotationBase()
    if base.objID == 0 { return false }
    // Splice ref out of /Annots.
    annots, _ := c.pageDict()["/Annots"].(pdfArray)
    newArr := make(pdfArray, 0, len(annots))
    for _, item := range annots {
        if ref, ok := item.(pdfRef); ok && ref.Num == base.objID { continue }
        newArr = append(newArr, item)
    }
    c.pageDict()["/Annots"] = newArr
    delete(c.doc.objects, base.objID)
    base.objID = 0
    base.attachedPage = nil
    c.invalidateCache()
    return true
}
```

For radio-group widget annotations (which have a parent `/AcroForm` field), `Delete` here would orphan the form parent's `/Kids` ref. Documented behavior: don't `Delete` a `*WidgetAnnotation` from the annotations API — use `Form.RemoveField` instead.

### Live-handle invariant

After a successful `Add`, the returned `*LinkAnnotation` (etc.) is a live handle: `link.SetColor(...)` mutates `dict["/C"]`; the next `Save` writes the new color. After `Delete`, the handle becomes dangling — documented contract.

### `/AP` not generated this subepic

- **Link** annotations are invisible click regions; viewers draw a default border from `/Border` if needed.
- **Highlight / Underline / StrikeOut / Squiggly** render natively from `/Subtype` + `/QuadPoints` + `/C` in every modern viewer (Adobe, Chrome, Edge, Preview, Foxit).

Annotations that *do* require `/AP` (FreeText, Square, Circle, Line, Ink, Stamp) are out of scope — separate subepic.

### Coexistence with form widgets

`PdfWithAcroForm.pdf` and `PdfWithLinks.pdf` both have widget annotations (form fields). They surface in `Annotations().All()` as `*WidgetAnnotation`. `WidgetAnnotation` is read-only at the annotation layer — to mutate form fields, callers continue to use `Form.Field(name)` / `Form.RemoveField`. `Add(*WidgetAnnotation)` returns an error: widgets must be created via `Form.AddTextField` etc. `Delete(*WidgetAnnotation)` works but leaves the form's `/AcroForm/Fields` ref dangling — documented as "not recommended".

### Save-time

The existing writer infrastructure handles annotations without changes:

- `pdfRef` in `/Annots` is remapped via `remapFn`.
- `/P` inside annotation dict is remapped via `remapFn`.
- `/A` is an inline dict; `writeValue` recursively descends and remaps any inner refs (e.g., `/Dest` in a `GoToAction` pointing at a page).

No new writer hooks needed.

## Files

| File | Action |
|---|---|
| `annotation.go` (new) | `Annotation` interface, `AnnotationType` enum, `AnnotationCollection`, `annotationBase` (embedded), `WidgetAnnotation`, `walkAnnotations` parsing dispatcher, internal helpers |
| `annotation_action.go` (new) | `Action` interface, `ActionType` enum, six concrete action types, `parseAction` dispatcher, `encodeAction` helper for write side |
| `annotation_link.go` (new) | `LinkAnnotation` + `LinkHighlightMode` |
| `annotation_markup.go` (new) | `HighlightAnnotation`, `UnderlineAnnotation`, `StrikeOutAnnotation`, `SquigglyAnnotation`, `QuadPoint` |
| `annotation_test.go` (new) | Public-API tests |
| `page.go` (modify) | `Annotations()` method + private `annotations *AnnotationCollection` field on `*Page` |
| `testdata/testfiles.json` | Register tests using `PdfWithLinks.pdf` and `PdfWithAcroForm.pdf` |
| `CLAUDE.md` | New entries in `## Public API` |
| `README.md` | New "Annotations" section with link + highlight examples |

## Test strategy

Programmatic-first plus a real-file read test on `testdata/PdfWithLinks.pdf`, which carries six link annotations covering exactly the action subtypes we care about:

| Index | Subtype | Action.S |
|---|---|---|
| 0 | Link | /GoTo |
| 1 | Link | /Launch (out of scope — expect nil action with our parser) |
| 2 | Link | /URI |
| 3 | Link | /JavaScript |
| 4 | Link | /Named |
| 5 | Link | /SubmitForm |

So `PdfWithLinks.pdf` exercises every action type we support **plus** an unknown one (`/Launch`) — perfect for verifying our "unknown action returns nil" contract.

**Test groups (~20 tests):**

1. **Round-trip per annotation type** (5 tests) — Link, Highlight, Underline, StrikeOut, Squiggly. Each: NewXxx + Add + WriteTo + OpenStream + assert Type/Color/Title/Contents/Rect/QuadPoints (markup family).

2. **Round-trip per writeable action** (5 tests) — GoToURI, GoTo, Named, SubmitForm, ResetForm. Each: build link with action, save, reopen, assert action type-asserts and properties match.

3. **JavaScript action read-only** (1 test) — open `PdfWithLinks.pdf`, find the JS-action link (index 3), verify `link.Action().(*JavaScriptAction).Script()` returns non-empty string.

4. **Read all action types from PdfWithLinks.pdf** (1 test) — enumerate, verify the 5 supported actions parse correctly + Launch returns nil.

5. **AnnotationCollection ops** (4 tests) — Add, Delete, At, All / Count semantics. Delete twice → false. Delete on nonexistent → false.

6. **Validation** (3 tests) — re-attach to different page returns error; idempotent same-page Add returns nil; nil Action acceptable on LinkAnnotation.

7. **Coexistence with form widgets** (2 tests) — `PdfWithAcroForm.pdf` has form widgets; verify they surface as `*WidgetAnnotation`. Verify `Form.Field("textField")` still works after enumerating Annotations. Verify Add a new LinkAnnotation on the same page doesn't break the form roundtrip.

8. **Aspose-style filter pattern** (1 test) — for-range over Annotations().All() with type-switch on AnnotationType, mirroring the README example.

**External oracle:** pypdf 6.x reads back the saved file's `/Annots` for a build-from-scratch PDF (one manual cross-check at the end of the plan).

## Non-goals (explicit)

- **Other annotation subtypes** — Text/sticky note, FreeText, Square, Circle, Line, Ink, Stamp, FileAttachment, Redact. Each gets its own subepic in the Annotations program.
- **`/AP` appearance-stream generation** — required for drawing/text annotations, not for the subtypes in this subepic. Separate epic.
- **Border styling beyond `/Border` width** — Aspose has a full `Border` type with style/dash/color. We expose only what's needed for typical link annotations: `/Border [HRadius VRadius Width]` via simple width getter/setter. Full border API is a follow-up.
- **`/AA` (additional actions)** — only `/A` (primary action) here. `/AA` events (mouse up/down/enter/exit) come later if requested.
- **`GoToRemoteAction`** (`/GoToR` — open external PDF) — out, narrow use case.
- **`LaunchAction`** (`/Launch` — run external program) — out, security implications.
- **Submitting JavaScript actions** — read-only this subepic. Creating `*JavaScriptAction` from user-supplied script is deferred to a security-conscious follow-up.
- **`/Subj`** subject field — Aspose has it; we ship Title (`/T`) + Contents (`/Contents`) only. Trivial to add later.
- **Annotation flags `/F`** — Hidden/Print/NoZoom/NoRotate/Locked. Not exposed via setters; preserved on read but not editable. Add when concrete need surfaces.
- **Modification timestamps** (`/M`, `/CreationDate`) — preserved on read, not auto-updated on mutation.
- **Optional content / layers integration** (`/OC`) — out.

## Acceptance

- On a blank `pdf.NewDocument`, add one of each annotation type (Link with GoToURIAction; Highlight/Underline/StrikeOut/Squiggly with quad points), save, reopen, enumerate returns 5 annotations of correct subtypes with correct actions and quad points.
- `PdfWithLinks.pdf` reads back: 6 link annotations, 5 of which expose the correct action type via the public API; 1 (`/Launch`) returns `nil` from `link.Action()`.
- `PdfWithAcroForm.pdf` reads back unchanged: form fields still accessible via `Form.Field`; Annotations() now also exposes them as `*WidgetAnnotation`.
- pypdf round-trips a build-from-scratch annotation file: it sees the same `/Subtype`, `/A/S`, and `/QuadPoints` we wrote.
- Full `go test ./...` green.
