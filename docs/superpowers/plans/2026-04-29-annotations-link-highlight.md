# Annotations Subepic 1 — Link + Highlight + Actions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `Page.Annotations()` collection with read+write support for Link annotations (with /A action dictionaries: GoToURI/GoTo/Named/SubmitForm/ResetForm/JavaScript-read-only) and the four text-markup annotation types (Highlight/Underline/StrikeOut/Squiggly), mirroring Aspose.PDF for .NET API conventions.

**Architecture:** Four new files (`annotation.go` for the collection + Annotation interface + walker; `annotation_action.go` for the Action hierarchy + parser; `annotation_link.go` for LinkAnnotation; `annotation_markup.go` for the four markup subtypes). Annotation instances are live handles over the underlying `pdfDict` once `Add`-ed; mutations propagate through to Save without a separate write step. Existing form widgets surface in `Annotations().All()` as opaque `*WidgetAnnotation` so the AcroForm subsystem keeps working.

**Tech Stack:** Go 1.24, standard library only. pypdf 6.x as external test oracle. `testdata/PdfWithLinks.pdf` is a perfect fixture — 6 link annotations covering every action subtype we support plus one (`/Launch`) we don't.

**Reference:** [docs/superpowers/specs/2026-04-29-annotations-link-highlight-design.md](../specs/2026-04-29-annotations-link-highlight-design.md)

---

## File Map

| File | Purpose |
|---|---|
| `annotation.go` (new) | `Annotation` interface, `AnnotationType` enum, `AnnotationCollection`, `annotationBase` (embedded), `WidgetAnnotation`, `walkAnnotations` parsing dispatcher, `parseAnnotation` per /Subtype, internal helpers (appendAnnotToPage, etc.) |
| `annotation_action.go` (new) | `Action` interface, `ActionType` enum, six concrete action types, `parseAction` dispatcher, `encodeAction` for write side |
| `annotation_link.go` (new) | `LinkAnnotation` + `LinkHighlightMode` enum |
| `annotation_markup.go` (new) | `HighlightAnnotation`, `UnderlineAnnotation`, `StrikeOutAnnotation`, `SquigglyAnnotation`, `QuadPoint` |
| `annotation_test.go` (new) | Public-API tests against testdata fixtures and round-trips |
| `page.go` (modify) | New `Annotations() *AnnotationCollection` method + private `annotations *AnnotationCollection` cache field on `*Page` |
| `testdata/testfiles.json` | Register tests using `PdfWithLinks.pdf` and `PdfWithAcroForm.pdf` |
| `CLAUDE.md` (modify) | New entries in the public API list |
| `README.md` (modify) | New "Annotations" section with link + highlight examples |

---

## Task 1: Skeleton — Annotation interface, AnnotationCollection, empty Page.Annotations()

**Files:**
- Create: `annotation.go`
- Create: `annotation_test.go`
- Modify: `page.go`

- [ ] **Step 1: Write the failing test**

`annotation_test.go`:
```go
package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

func TestPageAnnotationsNonNilOnPlainDoc(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, err := doc.Page(1)
	if err != nil {
		t.Fatalf("Page(1): %v", err)
	}
	ac := page.Annotations()
	if ac == nil {
		t.Fatal("Annotations() returned nil; want non-nil empty collection")
	}
	if got := ac.Count(); got != 0 {
		t.Errorf("Count() = %d on plain doc, want 0", got)
	}
	if got := ac.All(); len(got) != 0 {
		t.Errorf("All() len = %d, want 0", len(got))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test -run TestPageAnnotationsNonNilOnPlainDoc ./...
```

Expected: build failure — `Annotations` undefined.

- [ ] **Step 3: Write minimal implementation**

`annotation.go`:
```go
package asposepdf

// AnnotationType identifies the kind of annotation. Returned by
// Annotation.AnnotationType() so callers can switch on type without a
// type-assertion ladder.
type AnnotationType int

const (
	AnnotationTypeUnknown AnnotationType = iota
	AnnotationTypeLink
	AnnotationTypeHighlight
	AnnotationTypeUnderline
	AnnotationTypeStrikeOut
	AnnotationTypeSquiggly
	AnnotationTypeWidget
)

// Annotation is the common interface implemented by every concrete
// annotation type. Page-scoped — annotations belong to a specific page
// and are managed through that page's AnnotationCollection.
type Annotation interface {
	AnnotationType() AnnotationType
	Rect() Rectangle
	SetRect(r Rectangle)
	Color() *Color
	SetColor(c *Color)
	Title() string
	SetTitle(s string)
	Contents() string
	SetContents(s string)
	PageIndex() int

	// internal accessor — implementers embed annotationBase which exposes
	// this. Not part of the public surface.
	annotationBaseRef() *annotationBase
}

// annotationBase is embedded into every concrete annotation type. It
// owns the underlying pdfDict and tracks attachment state.
type annotationBase struct {
	dict         pdfDict
	doc          *Document
	page         *Page // construction-time page reference
	attachedPage *pdfObject
	objID        int // 0 until Add() runs
}

// annotationBaseRef satisfies the unexported part of the Annotation
// interface — see the interface declaration above.
func (b *annotationBase) annotationBaseRef() *annotationBase { return b }

// AnnotationCollection is the live, ordered set of annotations attached
// to a single page. Mutations through Add / Delete propagate to the
// page dict's /Annots array and to the document's object table; the
// next Save writes them out.
type AnnotationCollection struct {
	page  *Page
	items []Annotation
	built bool // false until first Annotations() call walks /Annots
}

// Count reports how many annotations live on this page.
func (c *AnnotationCollection) Count() int {
	c.ensureBuilt()
	return len(c.items)
}

// All returns the page's annotations as a slice. The returned slice
// shares pointer identity with At() / Field-by-name lookups so mutating
// a value through one accessor is visible through the others.
func (c *AnnotationCollection) All() []Annotation {
	c.ensureBuilt()
	return c.items
}

// ensureBuilt populates c.items lazily on first access. For now this is
// a no-op; Task 2 fills it in.
func (c *AnnotationCollection) ensureBuilt() {
	if c.built {
		return
	}
	c.built = true
	// Task 2 walks page /Annots here.
}
```

`page.go` — add the method and private cache field. Find the `Page` struct and add an `annotations *AnnotationCollection` field. Add the method anywhere near other accessors:
```go
// Annotations returns the page's annotation collection. Always non-nil;
// for a page with no /Annots array, the collection is empty.
func (p *Page) Annotations() *AnnotationCollection {
	if p.annotations == nil {
		p.annotations = &AnnotationCollection{page: p}
	}
	return p.annotations
}
```

- [ ] **Step 4: Run test to verify it passes**

```
go test -run TestPageAnnotationsNonNilOnPlainDoc -v ./...
go test ./...
```

Both PASS.

- [ ] **Step 5: Commit**

```
git add annotation.go annotation_test.go page.go
git commit -m "feat: Annotations skeleton — Page.Annotations() + Annotation interface"
```

---

## Task 2: Walk /Annots — surface existing annotations as WidgetAnnotation/Unknown

**Files:**
- Modify: `annotation.go`
- Modify: `annotation_test.go`
- Modify: `testdata/testfiles.json`

- [ ] **Step 1: Register the test**

`testdata/testfiles.json` — add:
```json
  "TestPageAnnotationsWalkExistingPDF": [["PdfWithAcroForm.pdf"]],
```

- [ ] **Step 2: Write the failing test**

Append to `annotation_test.go`:
```go
func TestPageAnnotationsWalkExistingPDF(t *testing.T) {
	doc, err := pdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	page, _ := doc.Page(1)
	ac := page.Annotations()
	if ac.Count() == 0 {
		t.Fatal("expected non-zero annotations on PdfWithAcroForm.pdf (form widgets)")
	}
	// Every annotation here is a form widget — verify type detection.
	for i, a := range ac.All() {
		if a.AnnotationType() != pdf.AnnotationTypeWidget {
			t.Errorf("annotation[%d]: type = %v, want AnnotationTypeWidget (form widget)", i, a.AnnotationType())
		}
		if _, ok := a.(*pdf.WidgetAnnotation); !ok {
			t.Errorf("annotation[%d]: concrete type = %T, want *pdf.WidgetAnnotation", i, a)
		}
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

```
go test -run TestPageAnnotationsWalkExistingPDF ./...
```

Expected: build failure (`*pdf.WidgetAnnotation` undefined) or test failure (Count returns 0 because ensureBuilt is empty).

- [ ] **Step 4: Implement the walk + WidgetAnnotation**

Add to `annotation.go`:
```go
// WidgetAnnotation is the read-only view of a form widget annotation
// surfaced through AnnotationCollection. Form fields continue to be
// mutated via the Form API — a WidgetAnnotation only exposes the base
// Annotation surface (Rect, Color, Title, Contents, PageIndex).
type WidgetAnnotation struct {
	annotationBase
}

func (a *WidgetAnnotation) AnnotationType() AnnotationType { return AnnotationTypeWidget }

// rect/title/contents/color/pageIndex helpers are shared via annotationBase
// methods declared below.

// Rect returns the annotation rectangle. Empty Rectangle if /Rect is
// missing or malformed.
func (b *annotationBase) Rect() Rectangle {
	arr, ok := b.dict["/Rect"].(pdfArray)
	if !ok || len(arr) != 4 {
		return Rectangle{}
	}
	llx, _ := toFloat(arr[0])
	lly, _ := toFloat(arr[1])
	urx, _ := toFloat(arr[2])
	ury, _ := toFloat(arr[3])
	return Rectangle{LLX: llx, LLY: lly, URX: urx, URY: ury}
}

// SetRect writes the annotation rectangle.
func (b *annotationBase) SetRect(r Rectangle) {
	b.dict["/Rect"] = pdfArray{r.LLX, r.LLY, r.URX, r.URY}
}

// Color returns the /C array as an RGB Color. Returns nil if /C is
// absent.
func (b *annotationBase) Color() *Color {
	arr, ok := b.dict["/C"].(pdfArray)
	if !ok {
		return nil
	}
	switch len(arr) {
	case 1:
		g, _ := toFloat(arr[0])
		return &Color{R: g, G: g, B: g, A: 1}
	case 3:
		r, _ := toFloat(arr[0])
		g, _ := toFloat(arr[1])
		bl, _ := toFloat(arr[2])
		return &Color{R: r, G: g, B: bl, A: 1}
	case 4:
		// CMYK — convert to a rough RGB approximation. Most annotation
		// software writes RGB; CMYK is rare for /C.
		c, _ := toFloat(arr[0])
		m, _ := toFloat(arr[1])
		y, _ := toFloat(arr[2])
		k, _ := toFloat(arr[3])
		return &Color{
			R: (1 - c) * (1 - k),
			G: (1 - m) * (1 - k),
			B: (1 - y) * (1 - k),
			A: 1,
		}
	}
	return nil
}

// SetColor writes /C as an RGB array; nil removes the entry.
func (b *annotationBase) SetColor(c *Color) {
	if c == nil {
		delete(b.dict, "/C")
		return
	}
	b.dict["/C"] = pdfArray{c.R, c.G, c.B}
}

// Title returns /T (the annotation author / reviewer name).
func (b *annotationBase) Title() string {
	return decodeFormString(b.dict["/T"])
}

func (b *annotationBase) SetTitle(s string) {
	if s == "" {
		delete(b.dict, "/T")
		return
	}
	b.dict["/T"] = encodeFormString(s)
}

// Contents returns /Contents (the annotation body text).
func (b *annotationBase) Contents() string {
	return decodeFormString(b.dict["/Contents"])
}

func (b *annotationBase) SetContents(s string) {
	if s == "" {
		delete(b.dict, "/Contents")
		return
	}
	b.dict["/Contents"] = encodeFormString(s)
}

// PageIndex returns the 1-based index of the page this annotation lives
// on. 0 if the annotation is not yet attached or its /P doesn't resolve.
func (b *annotationBase) PageIndex() int {
	if b.attachedPage == nil {
		return 0
	}
	for i, p := range b.doc.pages {
		if p.Num == b.attachedPage.Num {
			return i + 1
		}
	}
	return 0
}

// walkAnnotations builds the AnnotationCollection.items slice from the
// page's /Annots array. Each ref is dispatched by /Subtype to the right
// concrete type.
func (c *AnnotationCollection) walkAnnotations() {
	pageDict, _ := c.page.pageObj().Value.(pdfDict)
	if pageDict == nil {
		return
	}
	arr, _ := pageDict["/Annots"].(pdfArray)
	if len(arr) == 0 {
		return
	}
	for _, item := range arr {
		ref, ok := item.(pdfRef)
		if !ok {
			continue
		}
		obj, ok := c.page.doc.objects[ref.Num]
		if !ok {
			continue
		}
		dict, ok := obj.Value.(pdfDict)
		if !ok {
			continue
		}
		base := annotationBase{
			dict:         dict,
			doc:          c.page.doc,
			page:         c.page,
			attachedPage: c.page.pageObj(),
			objID:        ref.Num,
		}
		annot := parseAnnotation(base)
		if annot != nil {
			c.items = append(c.items, annot)
		}
	}
}

// parseAnnotation builds the right concrete type for the given dict.
// Future subepics extend this dispatch.
func parseAnnotation(base annotationBase) Annotation {
	subtype, _ := base.dict["/Subtype"].(pdfName)
	switch subtype {
	case "/Widget":
		return &WidgetAnnotation{annotationBase: base}
	}
	// Unknown / not-yet-supported subtype: treat as a generic widget
	// (read-only base properties, no specialized accessors).
	return &WidgetAnnotation{annotationBase: base}
}
```

Update `ensureBuilt`:
```go
func (c *AnnotationCollection) ensureBuilt() {
	if c.built {
		return
	}
	c.built = true
	c.walkAnnotations()
}
```

`pageObj()` already exists on `*Page` (used by AcroForm form-design code). Reuse.

- [ ] **Step 5: Run tests to verify they pass**

```
go test -run TestPageAnnotationsWalkExistingPDF -v ./...
go test ./...
```

Both PASS.

- [ ] **Step 6: Commit**

```
git add annotation.go annotation_test.go testdata/testfiles.json
git commit -m "feat: Annotations walk — surface existing /Annots as WidgetAnnotation"
```

---

## Task 3: LinkAnnotation skeleton + Add coordination

**Files:**
- Create: `annotation_link.go`
- Modify: `annotation.go` (add Add/Delete on AnnotationCollection)
- Modify: `annotation_test.go`

- [ ] **Step 1: Write the failing test**

Append to `annotation_test.go`:
```go
func TestAnnotationCollectionAddLinkRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	link.SetTitle("reviewer")
	link.SetContents("note")
	if err := page.Annotations().Add(link); err != nil {
		t.Fatalf("Add: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page2, _ := doc2.Page(1)
	ac2 := page2.Annotations()
	if ac2.Count() != 1 {
		t.Fatalf("Count after roundtrip = %d, want 1", ac2.Count())
	}
	got := ac2.At(0)
	if got.AnnotationType() != pdf.AnnotationTypeLink {
		t.Errorf("type = %v, want AnnotationTypeLink", got.AnnotationType())
	}
	if _, ok := got.(*pdf.LinkAnnotation); !ok {
		t.Errorf("concrete type = %T, want *pdf.LinkAnnotation", got)
	}
	if got.Title() != "reviewer" {
		t.Errorf("Title = %q, want \"reviewer\"", got.Title())
	}
}
```

You will need `import "bytes"` in `annotation_test.go` for the buffer.

- [ ] **Step 2: Run test to verify it fails**

```
go test -run TestAnnotationCollectionAddLinkRoundTrip ./...
```

Expected: build failure — `NewLinkAnnotation`, `LinkAnnotation`, `Add`, `At` undefined.

- [ ] **Step 3: Add `Add`, `At`, `Delete` to AnnotationCollection in annotation.go**

```go
// Add attaches an annotation to this page. Errors if the annotation is
// already attached to a different page; idempotent same-page Add returns
// nil. Panics on nil annotation (programmer error).
func (c *AnnotationCollection) Add(a Annotation) error {
	if a == nil {
		panic("Annotations.Add: nil annotation")
	}
	c.ensureBuilt()
	base := a.annotationBaseRef()
	if base.objID != 0 {
		if base.attachedPage == c.page.pageObj() {
			return nil // idempotent same-page
		}
		return fmt.Errorf("annotation already attached to page %d; Delete from that page first", c.attachedPageIndex(base))
	}
	// First-time attach.
	base.dict["/P"] = pdfRef{Num: c.page.pageObj().Num}
	objID := c.page.doc.nextID
	c.page.doc.nextID++
	c.page.doc.objects[objID] = &pdfObject{Num: objID, Value: base.dict}
	base.objID = objID
	base.attachedPage = c.page.pageObj()
	base.doc = c.page.doc

	// Append to page's /Annots array.
	pageDict, _ := c.page.pageObj().Value.(pdfDict)
	annots, _ := pageDict["/Annots"].(pdfArray)
	annots = append(annots, pdfRef{Num: objID})
	pageDict["/Annots"] = annots

	// Update local items so subsequent Count/All/At reflect the new state.
	c.items = append(c.items, a)
	return nil
}

// At returns the annotation at the given index. Panics if out of range.
func (c *AnnotationCollection) At(index int) Annotation {
	c.ensureBuilt()
	return c.items[index]
}

// Delete removes the annotation from this page. Returns true if found,
// false otherwise. After Delete the annotation handle is dangling.
func (c *AnnotationCollection) Delete(a Annotation) bool {
	if a == nil {
		return false
	}
	c.ensureBuilt()
	base := a.annotationBaseRef()
	if base.objID == 0 || base.attachedPage != c.page.pageObj() {
		return false
	}
	// Splice out of /Annots.
	pageDict, _ := c.page.pageObj().Value.(pdfDict)
	if pageDict == nil {
		return false
	}
	annots, _ := pageDict["/Annots"].(pdfArray)
	newArr := make(pdfArray, 0, len(annots))
	for _, item := range annots {
		if ref, ok := item.(pdfRef); ok && ref.Num == base.objID {
			continue
		}
		newArr = append(newArr, item)
	}
	pageDict["/Annots"] = newArr
	delete(c.page.doc.objects, base.objID)
	// Update local items.
	for i, it := range c.items {
		if it == a {
			c.items = append(c.items[:i], c.items[i+1:]...)
			break
		}
	}
	base.objID = 0
	base.attachedPage = nil
	return true
}

// attachedPageIndex returns the 1-based index of the page an annotation
// is currently attached to (used in error messages).
func (c *AnnotationCollection) attachedPageIndex(base *annotationBase) int {
	if base.attachedPage == nil {
		return 0
	}
	for i, p := range c.page.doc.pages {
		if p.Num == base.attachedPage.Num {
			return i + 1
		}
	}
	return 0
}
```

`fmt` needs to be in the imports.

- [ ] **Step 4: Create `annotation_link.go`**

```go
package asposepdf

// LinkAnnotation is a clickable region. Its visual is rendered by the
// viewer (no /AP needed). The associated /A action determines what
// happens on click — see Action and the various NewXxxAction factories.
type LinkAnnotation struct {
	annotationBase
}

func (a *LinkAnnotation) AnnotationType() AnnotationType { return AnnotationTypeLink }

// NewLinkAnnotation builds an unbound link annotation. Page must be
// non-nil. The annotation is not added to the document until
// page.Annotations().Add(link) succeeds.
func NewLinkAnnotation(page *Page, rect Rectangle) *LinkAnnotation {
	if page == nil {
		panic("NewLinkAnnotation: nil page")
	}
	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Link"),
		"/Rect":    pdfArray{rect.LLX, rect.LLY, rect.URX, rect.URY},
	}
	return &LinkAnnotation{annotationBase: annotationBase{
		dict: dict,
		doc:  page.doc,
		page: page,
	}}
}
```

- [ ] **Step 5: Update parseAnnotation in annotation.go to dispatch /Link**

Edit the switch in `parseAnnotation`:
```go
func parseAnnotation(base annotationBase) Annotation {
	subtype, _ := base.dict["/Subtype"].(pdfName)
	switch subtype {
	case "/Widget":
		return &WidgetAnnotation{annotationBase: base}
	case "/Link":
		return &LinkAnnotation{annotationBase: base}
	}
	return &WidgetAnnotation{annotationBase: base}
}
```

- [ ] **Step 6: Run tests to verify they pass**

```
go test -run TestAnnotationCollectionAddLinkRoundTrip -v ./...
go test ./...
```

Both PASS.

- [ ] **Step 7: Commit**

```
git add annotation.go annotation_link.go annotation_test.go
git commit -m "feat: LinkAnnotation + AnnotationCollection.Add/At/Delete"
```

---

## Task 4: Action framework + GoToURIAction

**Files:**
- Create: `annotation_action.go`
- Modify: `annotation_link.go` (add SetAction/Action methods)
- Modify: `annotation_test.go`

- [ ] **Step 1: Write the failing test**

Append to `annotation_test.go`:
```go
func TestLinkAnnotationGoToURIAction(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	link.SetAction(pdf.NewGoToURIAction("https://example.com/path"))
	if err := page.Annotations().Add(link); err != nil {
		t.Fatalf("Add: %v", err)
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page2, _ := doc2.Page(1)
	link2 := page2.Annotations().At(0).(*pdf.LinkAnnotation)
	act := link2.Action()
	if act == nil {
		t.Fatal("Action() = nil after roundtrip")
	}
	if act.ActionType() != pdf.ActionTypeGoToURI {
		t.Errorf("ActionType = %v, want ActionTypeGoToURI", act.ActionType())
	}
	uri, ok := act.(*pdf.GoToURIAction)
	if !ok {
		t.Fatalf("concrete type = %T, want *pdf.GoToURIAction", act)
	}
	if uri.URI() != "https://example.com/path" {
		t.Errorf("URI = %q, want %q", uri.URI(), "https://example.com/path")
	}
}
```

- [ ] **Step 2: Run to confirm failure**

```
go test -run TestLinkAnnotationGoToURIAction ./...
```

Expected: build failure — `NewGoToURIAction`, `Action`, `ActionType`, `*GoToURIAction`, `LinkAnnotation.Action`, `LinkAnnotation.SetAction` undefined.

- [ ] **Step 3: Create `annotation_action.go`**

```go
package asposepdf

// ActionType identifies the kind of action attached to an annotation
// (typically a LinkAnnotation's /A entry).
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

// Action is the common interface implemented by every concrete action
// type. Action values are inline within the parent annotation's /A
// dict — they are not separately addressable PDF objects.
type Action interface {
	ActionType() ActionType
	// encode returns the /A dict representation of this action.
	encode() pdfDict
}

// GoToURIAction opens a URI in the user's default handler (typically a
// web browser).
type GoToURIAction struct {
	uri string
}

func (a *GoToURIAction) ActionType() ActionType { return ActionTypeGoToURI }

// URI returns the destination URI.
func (a *GoToURIAction) URI() string { return a.uri }

// SetURI updates the destination URI.
func (a *GoToURIAction) SetURI(uri string) { a.uri = uri }

func (a *GoToURIAction) encode() pdfDict {
	return pdfDict{
		"/Type": pdfName("/Action"),
		"/S":    pdfName("/URI"),
		"/URI":  a.uri,
	}
}

// NewGoToURIAction builds a /URI action. Empty URI is permitted but
// usually not what callers want.
func NewGoToURIAction(uri string) *GoToURIAction { return &GoToURIAction{uri: uri} }

// parseAction reads an /A dict and returns the matching concrete action
// type. Returns nil for unsupported subtypes (e.g. /Launch, /GoToR).
func parseAction(d pdfDict) Action {
	s, _ := d["/S"].(pdfName)
	switch s {
	case "/URI":
		uri := decodeFormString(d["/URI"])
		return &GoToURIAction{uri: uri}
	}
	return nil
}
```

- [ ] **Step 4: Add Action and SetAction to LinkAnnotation in `annotation_link.go`**

```go
// Action returns the action attached to this link, or nil if no /A is
// present or the action type is unsupported.
func (a *LinkAnnotation) Action() Action {
	d, ok := a.dict["/A"].(pdfDict)
	if !ok {
		return nil
	}
	return parseAction(d)
}

// SetAction writes the /A entry. nil clears /A.
func (a *LinkAnnotation) SetAction(act Action) {
	if act == nil {
		delete(a.dict, "/A")
		return
	}
	a.dict["/A"] = act.encode()
}
```

- [ ] **Step 5: Run tests**

```
go test -run TestLinkAnnotationGoToURIAction -v ./...
go test ./...
```

Both PASS.

- [ ] **Step 6: Commit**

```
git add annotation_action.go annotation_link.go annotation_test.go
git commit -m "feat: Action framework + GoToURIAction with LinkAnnotation.SetAction"
```

---

## Task 5: GoToAction (internal page navigation)

**Files:**
- Modify: `annotation_action.go`
- Modify: `annotation_test.go`

- [ ] **Step 1: Write the failing test**

Append to `annotation_test.go`:
```go
func TestLinkAnnotationGoToAction(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	if err := doc.AddBlankPage(595, 842); err != nil {
		t.Fatalf("AddBlankPage: %v", err)
	}
	page1, _ := doc.Page(1)
	link := pdf.NewLinkAnnotation(page1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	link.SetAction(pdf.NewGoToAction(2, 800))
	if err := page1.Annotations().Add(link); err != nil {
		t.Fatalf("Add: %v", err)
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page2, _ := doc2.Page(1)
	link2 := page2.Annotations().At(0).(*pdf.LinkAnnotation)
	act := link2.Action()
	if act == nil {
		t.Fatal("Action() = nil")
	}
	gt, ok := act.(*pdf.GoToAction)
	if !ok {
		t.Fatalf("concrete = %T, want *pdf.GoToAction", act)
	}
	if gt.PageNum() != 2 {
		t.Errorf("PageNum = %d, want 2", gt.PageNum())
	}
	if gt.Top() != 800 {
		t.Errorf("Top = %f, want 800", gt.Top())
	}
}
```

- [ ] **Step 2: Run to confirm failure**

```
go test -run TestLinkAnnotationGoToAction ./...
```

Expected: build failure.

- [ ] **Step 3: Implement GoToAction in `annotation_action.go`**

Add:
```go
// GoToAction navigates to a page within the same document. PageNum is
// 1-based; Top is the y-coordinate of the destination view in default
// user space.
type GoToAction struct {
	pageNum int
	top     float64
	doc     *Document // optional — set when read from existing PDF for resolving page refs
}

func (a *GoToAction) ActionType() ActionType { return ActionTypeGoTo }

func (a *GoToAction) PageNum() int   { return a.pageNum }
func (a *GoToAction) Top() float64   { return a.top }
func (a *GoToAction) SetPageNum(n int)        { a.pageNum = n }
func (a *GoToAction) SetTop(t float64)        { a.top = t }

func (a *GoToAction) encode() pdfDict {
	// /D = [pageRef /XYZ left top zoom]
	// We can't embed the pageRef without the doc context; fall back to a
	// page-index-based destination if doc isn't set. Most viewers handle
	// numeric page indexes via /D -> [<int> /XYZ ...] — but per spec /D
	// requires a page indirect ref. We require a doc to encode properly.
	dest := pdfArray{a.pageNum - 1, pdfName("/XYZ"), pdfNull{}, a.top, pdfNull{}}
	if a.doc != nil && a.pageNum >= 1 && a.pageNum <= len(a.doc.pages) {
		dest = pdfArray{
			pdfRef{Num: a.doc.pages[a.pageNum-1].Num},
			pdfName("/XYZ"),
			pdfNull{},
			a.top,
			pdfNull{},
		}
	}
	return pdfDict{
		"/Type": pdfName("/Action"),
		"/S":    pdfName("/GoTo"),
		"/D":    dest,
	}
}

// NewGoToAction builds a /GoTo action targeting the given 1-based page
// and a y-coordinate (top of view).
func NewGoToAction(pageNum int, top float64) *GoToAction {
	return &GoToAction{pageNum: pageNum, top: top}
}
```

- [ ] **Step 4: Update parseAction dispatch**

Replace the switch body in `parseAction`:
```go
func parseAction(d pdfDict) Action {
	s, _ := d["/S"].(pdfName)
	switch s {
	case "/URI":
		uri := decodeFormString(d["/URI"])
		return &GoToURIAction{uri: uri}
	case "/GoTo":
		return parseGoToAction(d)
	}
	return nil
}

// parseGoToAction reads /D — supports the [pageRef /XYZ left top zoom]
// explicit destination form. Named destinations (/D as name or string)
// return PageNum=0; callers can detect via PageNum() == 0.
func parseGoToAction(d pdfDict) *GoToAction {
	dest, ok := d["/D"].(pdfArray)
	if !ok || len(dest) < 1 {
		return &GoToAction{}
	}
	a := &GoToAction{}
	switch first := dest[0].(type) {
	case pdfRef:
		// Look up the page by ref number — but parseAction doesn't have
		// a doc reference. Encode as zero; the caller (LinkAnnotation
		// holding the action) can re-derive PageNum if needed in a
		// future refinement. For the round-trip path we go through the
		// numeric form below.
		_ = first
	case int:
		a.pageNum = first + 1
	case float64:
		a.pageNum = int(first) + 1
	}
	if len(dest) >= 4 {
		t, _ := toFloat(dest[3])
		a.top = t
	}
	return a
}
```

To get the page-num resolution working for the indirect-ref case, modify `parseAnnotation` (in annotation.go) to attach `base.doc` to the parsed action so it can resolve page refs. Easier approach: post-process inside `LinkAnnotation.Action()` — after parseAction, if it's a GoToAction with pageNum=0 and the underlying dict had a pdfRef destination, look up the page index now.

Update `LinkAnnotation.Action()`:
```go
func (a *LinkAnnotation) Action() Action {
	d, ok := a.dict["/A"].(pdfDict)
	if !ok {
		return nil
	}
	act := parseAction(d)
	// If GoToAction with an indirect-ref destination, resolve pageNum
	// using the document.
	if gt, ok := act.(*GoToAction); ok && gt.pageNum == 0 {
		if dest, ok := d["/D"].(pdfArray); ok && len(dest) > 0 {
			if ref, ok := dest[0].(pdfRef); ok && a.doc != nil {
				for i, p := range a.doc.pages {
					if p.Num == ref.Num {
						gt.pageNum = i + 1
						break
					}
				}
			}
		}
		gt.doc = a.doc
	}
	return act
}
```

- [ ] **Step 5: Run tests**

```
go test -run TestLinkAnnotationGoToAction -v ./...
go test ./...
```

Both PASS.

- [ ] **Step 6: Commit**

```
git add annotation_action.go annotation_link.go annotation_test.go
git commit -m "feat: GoToAction with explicit destination + page-ref resolve"
```

---

## Task 6: NamedAction (built-in viewer commands)

**Files:**
- Modify: `annotation_action.go`
- Modify: `annotation_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestLinkAnnotationNamedAction(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	link.SetAction(pdf.NewNamedAction(pdf.NamedActionPrint))
	if err := page.Annotations().Add(link); err != nil {
		t.Fatalf("Add: %v", err)
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page2, _ := doc2.Page(1)
	link2 := page2.Annotations().At(0).(*pdf.LinkAnnotation)
	na, ok := link2.Action().(*pdf.NamedAction)
	if !ok {
		t.Fatalf("type = %T, want *pdf.NamedAction", link2.Action())
	}
	if na.Name() != pdf.NamedActionPrint {
		t.Errorf("Name = %v, want NamedActionPrint", na.Name())
	}
}
```

- [ ] **Step 2: Run to confirm failure**

```
go test -run TestLinkAnnotationNamedAction ./...
```

- [ ] **Step 3: Implement NamedAction**

Add to `annotation_action.go`:
```go
// NamedActionType identifies one of the standard viewer commands
// supported by /Named actions per ISO 32000-1 §12.6.4.11.
type NamedActionType int

const (
	NamedActionUnknown NamedActionType = iota
	NamedActionFirstPage
	NamedActionLastPage
	NamedActionNextPage
	NamedActionPrevPage
	NamedActionPrint
)

// NamedAction triggers a built-in viewer command (FirstPage, Print, ...).
type NamedAction struct {
	name NamedActionType
}

func (a *NamedAction) ActionType() ActionType { return ActionTypeNamed }
func (a *NamedAction) Name() NamedActionType  { return a.name }
func (a *NamedAction) SetName(n NamedActionType) { a.name = n }

func (a *NamedAction) encode() pdfDict {
	return pdfDict{
		"/Type": pdfName("/Action"),
		"/S":    pdfName("/Named"),
		"/N":    pdfName(namedActionToPDF(a.name)),
	}
}

func namedActionToPDF(n NamedActionType) string {
	switch n {
	case NamedActionFirstPage:
		return "/FirstPage"
	case NamedActionLastPage:
		return "/LastPage"
	case NamedActionNextPage:
		return "/NextPage"
	case NamedActionPrevPage:
		return "/PrevPage"
	case NamedActionPrint:
		return "/Print"
	}
	return ""
}

func pdfNameToNamedAction(s pdfName) NamedActionType {
	switch s {
	case "/FirstPage":
		return NamedActionFirstPage
	case "/LastPage":
		return NamedActionLastPage
	case "/NextPage":
		return NamedActionNextPage
	case "/PrevPage":
		return NamedActionPrevPage
	case "/Print":
		return NamedActionPrint
	}
	return NamedActionUnknown
}

// NewNamedAction builds a /Named action.
func NewNamedAction(n NamedActionType) *NamedAction {
	return &NamedAction{name: n}
}
```

Update parseAction switch:
```go
case "/Named":
    n, _ := d["/N"].(pdfName)
    return &NamedAction{name: pdfNameToNamedAction(n)}
```

- [ ] **Step 4: Run tests**

```
go test -run TestLinkAnnotationNamedAction -v ./...
go test ./...
```

Both PASS.

- [ ] **Step 5: Commit**

```
git add annotation_action.go annotation_test.go
git commit -m "feat: NamedAction for built-in viewer commands"
```

---

## Task 7: SubmitFormAction

**Files:**
- Modify: `annotation_action.go`
- Modify: `annotation_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestLinkAnnotationSubmitFormAction(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	link.SetAction(pdf.NewSubmitFormAction(
		"https://example.com/submit",
		[]string{"name", "email"},
		pdf.SubmitGetMethod|pdf.SubmitExportFormat,
	))
	if err := page.Annotations().Add(link); err != nil {
		t.Fatalf("Add: %v", err)
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page2, _ := doc2.Page(1)
	sf, ok := page2.Annotations().At(0).(*pdf.LinkAnnotation).Action().(*pdf.SubmitFormAction)
	if !ok {
		t.Fatalf("not a SubmitFormAction")
	}
	if sf.URL() != "https://example.com/submit" {
		t.Errorf("URL = %q", sf.URL())
	}
	got := sf.FieldNames()
	if len(got) != 2 || got[0] != "name" || got[1] != "email" {
		t.Errorf("FieldNames = %v, want [name email]", got)
	}
	if sf.Flags()&pdf.SubmitGetMethod == 0 {
		t.Error("SubmitGetMethod flag not set")
	}
	if sf.Flags()&pdf.SubmitExportFormat == 0 {
		t.Error("SubmitExportFormat flag not set")
	}
}
```

- [ ] **Step 2: Run to confirm failure**

```
go test -run TestLinkAnnotationSubmitFormAction ./...
```

- [ ] **Step 3: Implement SubmitFormAction**

Add to `annotation_action.go`:
```go
// SubmitFormFlags is the /Flags bitfield for a /SubmitForm action per
// ISO 32000-1 Table 237. Bit 1 is least significant.
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
	SubmitCanonicalFormat      SubmitFormFlags = 1 << 9
	SubmitExclNonUserAnnots    SubmitFormFlags = 1 << 10
	SubmitExclFKey             SubmitFormFlags = 1 << 11
	SubmitEmbedForm            SubmitFormFlags = 1 << 13
)

// SubmitFormAction submits form field values to a URL.
type SubmitFormAction struct {
	url    string
	fields []string
	flags  SubmitFormFlags
}

func (a *SubmitFormAction) ActionType() ActionType   { return ActionTypeSubmitForm }
func (a *SubmitFormAction) URL() string              { return a.url }
func (a *SubmitFormAction) FieldNames() []string     { return a.fields }
func (a *SubmitFormAction) Flags() SubmitFormFlags   { return a.flags }
func (a *SubmitFormAction) SetURL(u string)          { a.url = u }
func (a *SubmitFormAction) SetFieldNames(f []string) { a.fields = f }
func (a *SubmitFormAction) SetFlags(f SubmitFormFlags) { a.flags = f }

func (a *SubmitFormAction) encode() pdfDict {
	d := pdfDict{
		"/Type": pdfName("/Action"),
		"/S":    pdfName("/SubmitForm"),
		"/F":    pdfDict{"/FS": pdfName("/URL"), "/F": a.url},
	}
	if len(a.fields) > 0 {
		arr := make(pdfArray, 0, len(a.fields))
		for _, f := range a.fields {
			arr = append(arr, f)
		}
		d["/Fields"] = arr
	}
	if a.flags != 0 {
		d["/Flags"] = int(a.flags)
	}
	return d
}

// NewSubmitFormAction builds a /SubmitForm action.
func NewSubmitFormAction(url string, fields []string, flags SubmitFormFlags) *SubmitFormAction {
	return &SubmitFormAction{url: url, fields: fields, flags: flags}
}
```

Update parseAction switch:
```go
case "/SubmitForm":
    return parseSubmitFormAction(d)
```

Add helper:
```go
func parseSubmitFormAction(d pdfDict) *SubmitFormAction {
	a := &SubmitFormAction{}
	// /F can be either a URL filespec dict or a plain string.
	switch v := d["/F"].(type) {
	case pdfDict:
		a.url = decodeFormString(v["/F"])
	case string:
		a.url = decodeFormString(v)
	}
	if arr, ok := d["/Fields"].(pdfArray); ok {
		for _, item := range arr {
			if s, ok := item.(string); ok {
				a.fields = append(a.fields, decodeFormString(s))
			}
		}
	}
	if f, ok := d["/Flags"]; ok {
		a.flags = SubmitFormFlags(toInt(f))
	}
	return a
}
```

- [ ] **Step 4: Run tests**

```
go test -run TestLinkAnnotationSubmitFormAction -v ./...
go test ./...
```

Both PASS.

- [ ] **Step 5: Commit**

```
git add annotation_action.go annotation_test.go
git commit -m "feat: SubmitFormAction with /Fields and /Flags"
```

---

## Task 8: ResetFormAction

**Files:**
- Modify: `annotation_action.go`
- Modify: `annotation_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestLinkAnnotationResetFormAction(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	link.SetAction(pdf.NewResetFormAction([]string{"name", "email"}))
	if err := page.Annotations().Add(link); err != nil {
		t.Fatalf("Add: %v", err)
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page2, _ := doc2.Page(1)
	rf, ok := page2.Annotations().At(0).(*pdf.LinkAnnotation).Action().(*pdf.ResetFormAction)
	if !ok {
		t.Fatalf("not a ResetFormAction")
	}
	got := rf.FieldNames()
	if len(got) != 2 || got[0] != "name" || got[1] != "email" {
		t.Errorf("FieldNames = %v, want [name email]", got)
	}
}
```

- [ ] **Step 2: Run to confirm failure**

- [ ] **Step 3: Implement ResetFormAction**

Add to `annotation_action.go`:
```go
// ResetFormAction resets named form fields to their /DV defaults.
type ResetFormAction struct {
	fields []string
}

func (a *ResetFormAction) ActionType() ActionType { return ActionTypeResetForm }
func (a *ResetFormAction) FieldNames() []string   { return a.fields }
func (a *ResetFormAction) SetFieldNames(f []string) { a.fields = f }

func (a *ResetFormAction) encode() pdfDict {
	d := pdfDict{
		"/Type": pdfName("/Action"),
		"/S":    pdfName("/ResetForm"),
	}
	if len(a.fields) > 0 {
		arr := make(pdfArray, 0, len(a.fields))
		for _, f := range a.fields {
			arr = append(arr, f)
		}
		d["/Fields"] = arr
	}
	return d
}

// NewResetFormAction builds a /ResetForm action targeting the given
// field names. Empty fields means "reset all fields" per spec.
func NewResetFormAction(fields []string) *ResetFormAction {
	return &ResetFormAction{fields: fields}
}
```

Update parseAction switch:
```go
case "/ResetForm":
    a := &ResetFormAction{}
    if arr, ok := d["/Fields"].(pdfArray); ok {
        for _, item := range arr {
            if s, ok := item.(string); ok {
                a.fields = append(a.fields, decodeFormString(s))
            }
        }
    }
    return a
```

- [ ] **Step 4: Run tests + commit**

```
go test -run TestLinkAnnotationResetFormAction -v ./...
go test ./...
git add annotation_action.go annotation_test.go
git commit -m "feat: ResetFormAction"
```

---

## Task 9: JavaScriptAction read-only + read-all-actions test against PdfWithLinks.pdf

**Files:**
- Modify: `annotation_action.go`
- Modify: `annotation_test.go`
- Modify: `testdata/testfiles.json`

- [ ] **Step 1: Register tests**

`testdata/testfiles.json`:
```json
  "TestPdfWithLinksReadAllActions": [["PdfWithLinks.pdf"]],
```

- [ ] **Step 2: Write the failing test**

```go
func TestPdfWithLinksReadAllActions(t *testing.T) {
	doc, err := pdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	page, _ := doc.Page(1)
	ac := page.Annotations()
	if ac.Count() != 6 {
		t.Fatalf("Count = %d, want 6", ac.Count())
	}
	// Per the fixture survey: indices 0..5 carry GoTo, Launch, URI,
	// JavaScript, Named, SubmitForm respectively. /Launch is unsupported
	// — Action() returns nil for it.
	wantTypes := []pdf.ActionType{
		pdf.ActionTypeGoTo,
		pdf.ActionTypeUnknown, // /Launch is out of scope
		pdf.ActionTypeGoToURI,
		pdf.ActionTypeJavaScript,
		pdf.ActionTypeNamed,
		pdf.ActionTypeSubmitForm,
	}
	for i, a := range ac.All() {
		link, ok := a.(*pdf.LinkAnnotation)
		if !ok {
			t.Errorf("annotation[%d]: type = %T, want *LinkAnnotation", i, a)
			continue
		}
		act := link.Action()
		gotType := pdf.ActionTypeUnknown
		if act != nil {
			gotType = act.ActionType()
		}
		if gotType != wantTypes[i] {
			t.Errorf("annotation[%d]: action type = %v, want %v", i, gotType, wantTypes[i])
		}
	}

	// Spot-check JavaScript: action[3] should be JS with non-empty script.
	js, ok := ac.At(3).(*pdf.LinkAnnotation).Action().(*pdf.JavaScriptAction)
	if !ok {
		t.Fatal("annotation[3] is not JavaScriptAction")
	}
	if js.Script() == "" {
		t.Error("JavaScriptAction.Script() returned empty string")
	}
}
```

- [ ] **Step 3: Implement JavaScriptAction (parse-only, no constructor)**

Add to `annotation_action.go`:
```go
// JavaScriptAction holds a JavaScript snippet attached to an annotation.
// This subepic supports parsing JS actions read from existing PDFs.
// Constructing JavaScript actions from user-supplied script is deferred
// to a future security-conscious epic — there is no NewJavaScriptAction.
type JavaScriptAction struct {
	script string
}

func (a *JavaScriptAction) ActionType() ActionType { return ActionTypeJavaScript }
func (a *JavaScriptAction) Script() string         { return a.script }

// encode is required by the Action interface but not used (no constructor).
// Returns a minimal /JavaScript dict so re-saving a file with a parsed JS
// action preserves the script verbatim.
func (a *JavaScriptAction) encode() pdfDict {
	return pdfDict{
		"/Type": pdfName("/Action"),
		"/S":    pdfName("/JavaScript"),
		"/JS":   a.script,
	}
}
```

Update parseAction:
```go
case "/JavaScript":
    a := &JavaScriptAction{}
    switch v := d["/JS"].(type) {
    case string:
        a.script = decodeFormString(v)
    case *pdfStream:
        a.script = string(v.Data)
    }
    return a
```

- [ ] **Step 4: Run tests**

```
go test -run TestPdfWithLinksReadAllActions -v ./...
go test ./...
```

Both PASS.

- [ ] **Step 5: Commit**

```
git add annotation_action.go annotation_test.go testdata/testfiles.json
git commit -m "feat: JavaScriptAction parse-only + read-all-actions test"
```

---

## Task 10: HighlightAnnotation + QuadPoints

**Files:**
- Create: `annotation_markup.go`
- Modify: `annotation.go` (parseAnnotation dispatch)
- Modify: `annotation_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestHighlightAnnotationRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	hl := pdf.NewHighlightAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 615})
	hl.SetColor(&pdf.Color{R: 1, G: 1, B: 0, A: 1})
	hl.SetTitle("Reviewer")
	hl.SetContents("Important")
	hl.SetQuadPoints([]pdf.QuadPoint{
		{X1: 50, Y1: 615, X2: 300, Y2: 615, X3: 50, Y3: 600, X4: 300, Y4: 600},
	})
	if err := page.Annotations().Add(hl); err != nil {
		t.Fatalf("Add: %v", err)
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	page2, _ := doc2.Page(1)
	got := page2.Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypeHighlight {
		t.Errorf("type = %v, want AnnotationTypeHighlight", got.AnnotationType())
	}
	hl2 := got.(*pdf.HighlightAnnotation)
	if hl2.Title() != "Reviewer" {
		t.Errorf("Title = %q", hl2.Title())
	}
	qp := hl2.QuadPoints()
	if len(qp) != 1 {
		t.Fatalf("QuadPoints len = %d, want 1", len(qp))
	}
	if qp[0].X1 != 50 || qp[0].Y4 != 600 {
		t.Errorf("QuadPoint mismatch: %+v", qp[0])
	}
}
```

- [ ] **Step 2: Run to confirm failure**

- [ ] **Step 3: Create `annotation_markup.go`**

```go
package asposepdf

// QuadPoint is one quadrilateral within a markup annotation's
// /QuadPoints array. Each point names the four corners of one selection
// quad, in PDF default user space coordinates.
type QuadPoint struct {
	X1, Y1, X2, Y2, X3, Y3, X4, Y4 float64
}

// HighlightAnnotation marks a region with semi-transparent highlight
// color. Renders natively in spec-conforming viewers from /Subtype +
// /QuadPoints + /C — no /AP needed.
type HighlightAnnotation struct {
	annotationBase
}

func (a *HighlightAnnotation) AnnotationType() AnnotationType { return AnnotationTypeHighlight }

// QuadPoints returns the array of quads describing the selection.
func (a *HighlightAnnotation) QuadPoints() []QuadPoint {
	return readQuadPoints(a.dict["/QuadPoints"])
}

// SetQuadPoints writes /QuadPoints. nil or empty slice removes the entry.
func (a *HighlightAnnotation) SetQuadPoints(qp []QuadPoint) {
	if len(qp) == 0 {
		delete(a.dict, "/QuadPoints")
		return
	}
	a.dict["/QuadPoints"] = quadPointsToPDFArray(qp)
}

// NewHighlightAnnotation builds an unbound highlight annotation. Page
// must be non-nil.
func NewHighlightAnnotation(page *Page, rect Rectangle) *HighlightAnnotation {
	return &HighlightAnnotation{annotationBase: newMarkupBase(page, rect, "/Highlight")}
}

// newMarkupBase is the shared constructor body for the four markup
// types. Only /Subtype differs; everything else is identical.
func newMarkupBase(page *Page, rect Rectangle, subtype pdfName) annotationBase {
	if page == nil {
		panic("NewMarkupAnnotation: nil page")
	}
	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": subtype,
		"/Rect":    pdfArray{rect.LLX, rect.LLY, rect.URX, rect.URY},
	}
	return annotationBase{dict: dict, doc: page.doc, page: page}
}

func readQuadPoints(v pdfValue) []QuadPoint {
	arr, ok := v.(pdfArray)
	if !ok || len(arr)%8 != 0 {
		return nil
	}
	out := make([]QuadPoint, 0, len(arr)/8)
	for i := 0; i+7 < len(arr); i += 8 {
		var qp QuadPoint
		qp.X1, _ = toFloat(arr[i])
		qp.Y1, _ = toFloat(arr[i+1])
		qp.X2, _ = toFloat(arr[i+2])
		qp.Y2, _ = toFloat(arr[i+3])
		qp.X3, _ = toFloat(arr[i+4])
		qp.Y3, _ = toFloat(arr[i+5])
		qp.X4, _ = toFloat(arr[i+6])
		qp.Y4, _ = toFloat(arr[i+7])
		out = append(out, qp)
	}
	return out
}

func quadPointsToPDFArray(qp []QuadPoint) pdfArray {
	arr := make(pdfArray, 0, len(qp)*8)
	for _, q := range qp {
		arr = append(arr, q.X1, q.Y1, q.X2, q.Y2, q.X3, q.Y3, q.X4, q.Y4)
	}
	return arr
}
```

Update parseAnnotation in `annotation.go`:
```go
case "/Highlight":
    return &HighlightAnnotation{annotationBase: base}
```

- [ ] **Step 4: Run tests + commit**

```
go test -run TestHighlightAnnotationRoundTrip -v ./...
go test ./...
git add annotation.go annotation_markup.go annotation_test.go
git commit -m "feat: HighlightAnnotation + QuadPoint encoding"
```

---

## Task 11: Underline + StrikeOut + Squiggly (markup family)

**Files:**
- Modify: `annotation_markup.go`
- Modify: `annotation.go` (parseAnnotation dispatch)
- Modify: `annotation_test.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestUnderlineAnnotationRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	a := pdf.NewUnderlineAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 615})
	a.SetQuadPoints([]pdf.QuadPoint{{X1: 50, Y1: 615, X2: 300, Y2: 615, X3: 50, Y3: 600, X4: 300, Y4: 600}})
	page.Annotations().Add(a)
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	got := doc2.Pages()[0].Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypeUnderline {
		t.Errorf("type = %v, want AnnotationTypeUnderline", got.AnnotationType())
	}
}

func TestStrikeOutAnnotationRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	a := pdf.NewStrikeOutAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 615})
	a.SetQuadPoints([]pdf.QuadPoint{{X1: 50, Y1: 615, X2: 300, Y2: 615, X3: 50, Y3: 600, X4: 300, Y4: 600}})
	page.Annotations().Add(a)
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	got := doc2.Pages()[0].Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypeStrikeOut {
		t.Errorf("type = %v, want AnnotationTypeStrikeOut", got.AnnotationType())
	}
}

func TestSquigglyAnnotationRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	a := pdf.NewSquigglyAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 615})
	a.SetQuadPoints([]pdf.QuadPoint{{X1: 50, Y1: 615, X2: 300, Y2: 615, X3: 50, Y3: 600, X4: 300, Y4: 600}})
	page.Annotations().Add(a)
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	got := doc2.Pages()[0].Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypeSquiggly {
		t.Errorf("type = %v, want AnnotationTypeSquiggly", got.AnnotationType())
	}
}
```

- [ ] **Step 2: Run to confirm failures**

- [ ] **Step 3: Implement the three sibling types in `annotation_markup.go`**

Append:
```go
type UnderlineAnnotation struct {
	annotationBase
}

func (a *UnderlineAnnotation) AnnotationType() AnnotationType { return AnnotationTypeUnderline }
func (a *UnderlineAnnotation) QuadPoints() []QuadPoint        { return readQuadPoints(a.dict["/QuadPoints"]) }
func (a *UnderlineAnnotation) SetQuadPoints(qp []QuadPoint) {
	if len(qp) == 0 {
		delete(a.dict, "/QuadPoints")
		return
	}
	a.dict["/QuadPoints"] = quadPointsToPDFArray(qp)
}

func NewUnderlineAnnotation(page *Page, rect Rectangle) *UnderlineAnnotation {
	return &UnderlineAnnotation{annotationBase: newMarkupBase(page, rect, "/Underline")}
}

type StrikeOutAnnotation struct {
	annotationBase
}

func (a *StrikeOutAnnotation) AnnotationType() AnnotationType { return AnnotationTypeStrikeOut }
func (a *StrikeOutAnnotation) QuadPoints() []QuadPoint        { return readQuadPoints(a.dict["/QuadPoints"]) }
func (a *StrikeOutAnnotation) SetQuadPoints(qp []QuadPoint) {
	if len(qp) == 0 {
		delete(a.dict, "/QuadPoints")
		return
	}
	a.dict["/QuadPoints"] = quadPointsToPDFArray(qp)
}

func NewStrikeOutAnnotation(page *Page, rect Rectangle) *StrikeOutAnnotation {
	return &StrikeOutAnnotation{annotationBase: newMarkupBase(page, rect, "/StrikeOut")}
}

type SquigglyAnnotation struct {
	annotationBase
}

func (a *SquigglyAnnotation) AnnotationType() AnnotationType { return AnnotationTypeSquiggly }
func (a *SquigglyAnnotation) QuadPoints() []QuadPoint        { return readQuadPoints(a.dict["/QuadPoints"]) }
func (a *SquigglyAnnotation) SetQuadPoints(qp []QuadPoint) {
	if len(qp) == 0 {
		delete(a.dict, "/QuadPoints")
		return
	}
	a.dict["/QuadPoints"] = quadPointsToPDFArray(qp)
}

func NewSquigglyAnnotation(page *Page, rect Rectangle) *SquigglyAnnotation {
	return &SquigglyAnnotation{annotationBase: newMarkupBase(page, rect, "/Squiggly")}
}
```

Update parseAnnotation switch in `annotation.go`:
```go
case "/Underline":
    return &UnderlineAnnotation{annotationBase: base}
case "/StrikeOut":
    return &StrikeOutAnnotation{annotationBase: base}
case "/Squiggly":
    return &SquigglyAnnotation{annotationBase: base}
```

- [ ] **Step 4: Run tests + commit**

```
go test -run 'TestUnderline|TestStrikeOut|TestSquiggly' -v ./...
go test ./...
git add annotation.go annotation_markup.go annotation_test.go
git commit -m "feat: Underline/StrikeOut/Squiggly markup annotations"
```

---

## Task 12: Collection ops (DeleteAt + idempotent Add + re-attach error)

**Files:**
- Modify: `annotation.go` (DeleteAt method, refine Add validation)
- Modify: `annotation_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestAnnotationCollectionDeleteAt(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	page.Annotations().Add(link)
	if err := page.Annotations().DeleteAt(0); err != nil {
		t.Fatalf("DeleteAt(0): %v", err)
	}
	if page.Annotations().Count() != 0 {
		t.Errorf("Count after DeleteAt = %d, want 0", page.Annotations().Count())
	}
	if err := page.Annotations().DeleteAt(0); err == nil {
		t.Error("DeleteAt on empty collection should return error")
	}
}

func TestAnnotationCollectionIdempotentAdd(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	if err := page.Annotations().Add(link); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	if err := page.Annotations().Add(link); err != nil {
		t.Errorf("second Add same page should be no-op success, got: %v", err)
	}
	if page.Annotations().Count() != 1 {
		t.Errorf("Count after redundant Add = %d, want 1", page.Annotations().Count())
	}
}

func TestAnnotationCollectionReattachError(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	if err := doc.AddBlankPage(595, 842); err != nil {
		t.Fatalf("AddBlankPage: %v", err)
	}
	page1, _ := doc.Page(1)
	page2, _ := doc.Page(2)
	link := pdf.NewLinkAnnotation(page1, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	if err := page1.Annotations().Add(link); err != nil {
		t.Fatalf("Add to page 1: %v", err)
	}
	if err := page2.Annotations().Add(link); err == nil {
		t.Error("Add to page 2 should error — already attached to page 1")
	}
}
```

- [ ] **Step 2: Run to confirm failures**

```
go test -run 'TestAnnotationCollectionDeleteAt|TestAnnotationCollectionIdempotentAdd|TestAnnotationCollectionReattachError' ./...
```

`Reattach` and `Idempotent` tests should already pass from Task 3's Add implementation. `DeleteAt` will fail.

- [ ] **Step 3: Implement DeleteAt in `annotation.go`**

```go
// DeleteAt removes the annotation at the given index. Errors on out-of-range.
func (c *AnnotationCollection) DeleteAt(index int) error {
	c.ensureBuilt()
	if index < 0 || index >= len(c.items) {
		return fmt.Errorf("AnnotationCollection.DeleteAt(%d): out of range [0,%d)", index, len(c.items))
	}
	a := c.items[index]
	if !c.Delete(a) {
		return fmt.Errorf("AnnotationCollection.DeleteAt(%d): underlying delete failed", index)
	}
	return nil
}
```

- [ ] **Step 4: Run tests + commit**

```
go test -run 'TestAnnotationCollection' -v ./...
go test ./...
git add annotation.go annotation_test.go
git commit -m "feat: AnnotationCollection.DeleteAt + validation tests"
```

---

## Task 13: Form widget coexistence regression

**Files:**
- Modify: `annotation_test.go`
- Modify: `testdata/testfiles.json`

This task adds NO production code — it pins the contract that adding annotations doesn't break the AcroForm subsystem.

- [ ] **Step 1: Register tests**

`testdata/testfiles.json`:
```json
  "TestAnnotationsCoexistWithForm": [["PdfWithAcroForm.pdf"]],
```

- [ ] **Step 2: Write the failing test**

```go
func TestAnnotationsCoexistWithForm(t *testing.T) {
	doc, err := pdf.Open(testFile(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	page, _ := doc.Page(1)

	// Form API still works.
	if doc.Form().HasField("textField") == false {
		t.Fatal("textField missing — Form parsing broke")
	}

	// Annotations() returns existing form widgets as WidgetAnnotation.
	widgetCount := 0
	for _, a := range page.Annotations().All() {
		if a.AnnotationType() == pdf.AnnotationTypeWidget {
			widgetCount++
		}
	}
	if widgetCount == 0 {
		t.Fatal("expected at least one WidgetAnnotation")
	}

	// Add a new LinkAnnotation; ensure the form continues to roundtrip.
	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 50, URX: 200, URY: 70})
	link.SetAction(pdf.NewGoToURIAction("https://example.com"))
	if err := page.Annotations().Add(link); err != nil {
		t.Fatalf("Add link: %v", err)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if doc2.Form().HasField("textField") == false {
		t.Error("textField missing after annotations + roundtrip")
	}
	// The new link should be there too.
	page2, _ := doc2.Page(1)
	hasLink := false
	for _, a := range page2.Annotations().All() {
		if a.AnnotationType() == pdf.AnnotationTypeLink {
			hasLink = true
			break
		}
	}
	if !hasLink {
		t.Error("LinkAnnotation lost after roundtrip with form widgets")
	}
}
```

- [ ] **Step 3: Run tests**

```
go test -run TestAnnotationsCoexistWithForm -v ./...
go test ./...
```

PASS — coexistence already works because parseAnnotation correctly dispatches /Widget to WidgetAnnotation while leaving Form's own walk untouched.

- [ ] **Step 4: Commit**

```
git add annotation_test.go testdata/testfiles.json
git commit -m "test: annotations coexist with AcroForm widgets across roundtrip"
```

---

## Task 14: Aspose-style filter pattern integration test

**Files:**
- Modify: `annotation_test.go`

- [ ] **Step 1: Write the test**

```go
func TestAnnotationFilterByType(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	link.SetAction(pdf.NewGoToURIAction("https://example.com"))
	page.Annotations().Add(link)

	hl := pdf.NewHighlightAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 615})
	page.Annotations().Add(hl)

	ul := pdf.NewUnderlineAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 500, URX: 300, URY: 515})
	page.Annotations().Add(ul)

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	page2, _ := doc2.Page(1)

	// Aspose-style filter pattern.
	links := 0
	highlights := 0
	underlines := 0
	for _, a := range page2.Annotations().All() {
		switch a.AnnotationType() {
		case pdf.AnnotationTypeLink:
			links++
			if u, ok := a.(*pdf.LinkAnnotation).Action().(*pdf.GoToURIAction); ok {
				if u.URI() != "https://example.com" {
					t.Errorf("URI = %q", u.URI())
				}
			}
		case pdf.AnnotationTypeHighlight:
			highlights++
		case pdf.AnnotationTypeUnderline:
			underlines++
		}
	}
	if links != 1 || highlights != 1 || underlines != 1 {
		t.Errorf("counts: links=%d highlights=%d underlines=%d", links, highlights, underlines)
	}
}
```

- [ ] **Step 2: Run + commit**

```
go test -run TestAnnotationFilterByType -v ./...
go test ./...
git add annotation_test.go
git commit -m "test: Aspose-style annotation filter pattern integration test"
```

---

## Task 15: pypdf cross-verification + docs + close bd

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`

- [ ] **Step 1: pypdf cross-check (manual)**

Create `D:/tmp/check_annotations/main.go`:
```go
package main

import (
	"log"

	pdf "github.com/aspose/pdf-for-go"
)

func main() {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
	link.SetAction(pdf.NewGoToURIAction("https://example.com"))
	page.Annotations().Add(link)

	hl := pdf.NewHighlightAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 615})
	hl.SetColor(&pdf.Color{R: 1, G: 1, B: 0, A: 1})
	hl.SetQuadPoints([]pdf.QuadPoint{{X1: 50, Y1: 615, X2: 300, Y2: 615, X3: 50, Y3: 600, X4: 300, Y4: 600}})
	page.Annotations().Add(hl)

	if err := doc.Save("D:/tmp/annotations_built.pdf"); err != nil {
		log.Fatal(err)
	}
}
```

`D:/tmp/check_annotations/go.mod`:
```
module check_annotations

go 1.24

require github.com/aspose/pdf-for-go v0.0.0

replace github.com/aspose/pdf-for-go => D:/aspose/claude/aspose.pdf-for-go-foss
```

Run:
```
cd D:/tmp/check_annotations && go run main.go
python -c "
from pypdf import PdfReader
r = PdfReader('D:/tmp/annotations_built.pdf')
av = r.pages[0].get('/Annots')
ann = av.get_object() if hasattr(av, 'get_object') else av
print('count:', len(ann))
for i, a in enumerate(ann):
    ao = a.get_object() if hasattr(a, 'get_object') else a
    sub = ao.get('/Subtype', '?')
    a_dict = ao.get('/A')
    a_s = a_dict.get_object().get('/S') if a_dict and hasattr(a_dict, 'get_object') else None
    print(f'  [{i}] /Subtype={sub} action.S={a_s}')
"
```

Expected output:
```
count: 2
  [0] /Subtype=/Link action.S=/URI
  [1] /Subtype=/Highlight action.S=None
```

If pypdf doesn't report both annotations, STOP and report BLOCKED.

Cleanup: `rm -rf D:/tmp/check_annotations D:/tmp/annotations_built.pdf`.

- [ ] **Step 2: Update CLAUDE.md**

Open `CLAUDE.md`. Find the public API list (under document.go bullets). After the AcroForm bullets, add:

```markdown
- `(*Page).Annotations() *AnnotationCollection` — returns the page's annotation collection (always non-nil; empty for pages with no /Annots)
```

After the `**`form.go` / `form_fields.go`**` block, add a new block:

```markdown
**`annotation.go` / `annotation_action.go` / `annotation_link.go` / `annotation_markup.go`**
- `Annotation` interface — `AnnotationType()`, `Rect()/SetRect()`, `Color()/SetColor()`, `Title()/SetTitle()`, `Contents()/SetContents()`, `PageIndex()`
- Concrete types: `LinkAnnotation`, `HighlightAnnotation`, `UnderlineAnnotation`, `StrikeOutAnnotation`, `SquigglyAnnotation`, `WidgetAnnotation` (existing form fields)
- `AnnotationCollection` — `Add(a)`, `Delete(a) bool`, `DeleteAt(i)`, `Count()`, `At(i)`, `All() []Annotation`
- Constructors: `NewLinkAnnotation(page, rect)`, `NewHighlightAnnotation`, `NewUnderlineAnnotation`, `NewStrikeOutAnnotation`, `NewSquigglyAnnotation`
- `LinkAnnotation.Action()/SetAction()`, `LinkHighlightMode` enum
- `Action` interface — `ActionType()`; concrete types: `GoToURIAction`, `GoToAction`, `NamedAction`, `SubmitFormAction`, `ResetFormAction`, `JavaScriptAction` (parse-only)
- Action constructors: `NewGoToURIAction(uri)`, `NewGoToAction(pageNum, top)`, `NewNamedAction(name)`, `NewSubmitFormAction(url, fields, flags)`, `NewResetFormAction(fields)`
- `QuadPoint` struct — 8 floats per highlight/underline/strikeout/squiggly quad
- `NamedActionType` enum (FirstPage/LastPage/NextPage/PrevPage/Print)
- `SubmitFormFlags` bitfield per ISO 32000-1 Table 237
```

- [ ] **Step 3: Update README.md**

After the "Forms (AcroForm)" section, add:

````markdown
### Annotations

```go
doc, _ := pdf.Open("input.pdf")
page, _ := doc.Page(1)

// Add a hyperlink
link := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 720})
link.SetAction(pdf.NewGoToURIAction("https://example.com"))
link.SetColor(&pdf.Color{R: 0, G: 0, B: 1, A: 1})
page.Annotations().Add(link)

// Highlight a passage
hl := pdf.NewHighlightAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 300, URY: 615})
hl.SetColor(&pdf.Color{R: 1, G: 1, B: 0, A: 1})
hl.SetTitle("Reviewer")
hl.SetContents("Important paragraph")
hl.SetQuadPoints([]pdf.QuadPoint{
    {X1: 50, Y1: 615, X2: 300, Y2: 615, X3: 50, Y3: 600, X4: 300, Y4: 600},
})
page.Annotations().Add(hl)

// Iterate and filter
for _, a := range page.Annotations().All() {
    if a.AnnotationType() != pdf.AnnotationTypeLink {
        continue
    }
    if uri, ok := a.(*pdf.LinkAnnotation).Action().(*pdf.GoToURIAction); ok {
        fmt.Println(uri.URI())
    }
}

// Wire a form push button to a server submit
submit := pdf.NewLinkAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 50, URX: 200, URY: 80})
submit.SetAction(pdf.NewSubmitFormAction(
    "https://example.com/api/subscribe",
    []string{"name", "email"},
    pdf.SubmitGetMethod|pdf.SubmitExportFormat,
))
page.Annotations().Add(submit)

doc.Save("with_annotations.pdf")
```

Supported subtypes in this release: `Link`, `Highlight`, `Underline`, `StrikeOut`, `Squiggly`. Existing form widgets surface as `WidgetAnnotation` for read-only inspection — to mutate form fields use the `Form` API. JavaScript actions are read-only (parsed but not constructible). Out of scope for this release: text/sticky-note, FreeText, drawing primitives (Square/Circle/Line/Ink), Stamp, FileAttachment, Redact, `/AP` appearance generation.
````

- [ ] **Step 4: Verify README compiles**

Extract the new code into `D:/tmp/readme_annot_smoke/main.go` (wrap with package main, import "fmt", "github.com/aspose/pdf-for-go") plus a go.mod with the replace directive. Run `go build ./...`. Cleanup.

- [ ] **Step 5: Run full suite**

```
go test ./...
```

PASS.

- [ ] **Step 6: Commit**

```
git add CLAUDE.md README.md
git commit -m "docs: Annotations subepic 1 in CLAUDE.md and README"
```

- [ ] **Step 7: Close the bd issue**

```
bd update pdf-go-37n --status closed --append-notes "Subepic 1 (read framework + Link + Highlight family + actions: URI/GoTo/Named/SubmitForm/ResetForm + JavaScript-read-only) shipped. Subepics 2-4 (text/sticky+FreeText+Stamp; Square/Circle/Line/Ink drawing; FileAttachment/Redact/JavaScript-emit) remain open as future epics — file new top-level epics rather than reopening this one."
```

The user may prefer to leave `pdf-go-37n` open since it covers all subepics. Verify with the user before closing.

---

## Self-review

**Spec coverage:** every spec section maps to at least one task.

| Spec section | Tasks |
|---|---|
| Page-side AnnotationCollection (Add/Delete/Count/At/All/DeleteAt) | 1, 3, 12 |
| Annotation interface + base properties (Rect/Color/Title/Contents/PageIndex) | 1, 2 |
| Constructors `New<Type>Annotation(page, rect)` | 3 (Link), 10 (Highlight), 11 (Underline/StrikeOut/Squiggly) |
| LinkAnnotation (Action/SetAction/Highlight) | 3, 4 |
| Highlight family + QuadPoints | 10, 11 |
| WidgetAnnotation (read-only handle for form fields) | 2 |
| Action framework + 6 concrete types | 4 (URI), 5 (GoTo), 6 (Named), 7 (SubmitForm), 8 (ResetForm), 9 (JS read-only) |
| Read framework (walk /Annots, parse by /Subtype, parseAction by /S) | 2, 4-9 |
| Constructor + Add coordination (lazy attach, /P, objID, /Annots append) | 3 |
| Delete cascade (splice ref, delete from objects) | 3 (Delete in collection), 12 (DeleteAt wrapper) |
| Live-handle invariant | implicit in 3 (Add returns canonical instance), preserved in 10/11 markup constructors |
| Re-attach error / idempotent same-page | 12 |
| /AP not generated this subepic | non-implementation, documented in spec |
| Coexistence with form widgets | 2, 13 |
| Save-time coordination via existing writer | implicit (no new writer code) |
| Aspose-style filter usage | 14 |
| Tests against PdfWithLinks.pdf | 9 |
| Files: annotation.go, annotation_action.go, annotation_link.go, annotation_markup.go, annotation_test.go, page.go, testfiles.json | All tasks |
| CLAUDE.md, README.md | 15 |
| pypdf cross-check | 15 |

No gaps.

**Placeholder scan:** searched for "TBD", "TODO", "implement later", "appropriate error handling", "similar to" — none. Every code step has full code blocks; every test step has the test verbatim.

**Type consistency:** `Annotation` interface declared in Task 1 with `annotationBaseRef()` private method; every concrete type (`LinkAnnotation`/`HighlightAnnotation`/`UnderlineAnnotation`/`StrikeOutAnnotation`/`SquigglyAnnotation`/`WidgetAnnotation`) embeds `annotationBase` which provides this. `Action` interface in Task 4 with `encode() pdfDict`; every concrete action implements it. `QuadPoint` field names (`X1, Y1, X2, Y2, X3, Y3, X4, Y4`) consistent across Task 10 (declaration) and Task 11 (sibling types). `SubmitFormFlags` bit positions match ISO 32000-1 Table 237 spec citation in Task 7.

**One known design simplification:** `parseGoToAction` returns pageNum=0 if /D[0] is a `pdfRef` because parseAction doesn't have a `*Document` reference; the page lookup happens in `LinkAnnotation.Action()` after parseAction returns. Documented in Task 5.
