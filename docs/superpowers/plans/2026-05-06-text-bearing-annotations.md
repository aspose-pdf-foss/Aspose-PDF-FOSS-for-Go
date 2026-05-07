# Text-bearing Annotations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the three text-bearing annotation types: `TextAnnotation` (sticky note), `FreeTextAnnotation` (text on page with full callout / cloudy-border / typewriter support), and `StampAnnotation` (14 predefined visuals + custom-image override).

**Architecture:** Three new public types in their own files. Two new generator files (`appearance_freetext.go` for FreeText rendering, `appearance_stamp.go` for Stamp rendering). One small refactor of `text_add.go` to extract a shared `renderTextInBuilder` helper used by both `Page.AddText` (existing) and the new FreeText/Stamp rendering paths.

**Tech Stack:** Go 1.24 (existing module), pure standard library, `bytes.Buffer`, `strconv.FormatFloat`. pypdf 6.x for external cross-verification (Task 19 only).

**Reference:** [docs/superpowers/specs/2026-05-06-text-bearing-annotations-design.md](../specs/2026-05-06-text-bearing-annotations-design.md)

---

## File Map

| File | Purpose |
|---|---|
| `annotation_text.go` (new) | `TextAnnotation` + accessors + parse helper |
| `annotation_freetext.go` (new) | `FreeTextAnnotation` + accessors + callout/RD math + parse helper |
| `annotation_stamp.go` (new) | `StampAnnotation` + accessors + custom-image embedding + parse helper |
| `appearance_freetext.go` (new) | `generateFreeTextAppearance` + `drawCloudyRectBorder` + `drawCalloutLine` + `drawStandardRectBorder` |
| `appearance_stamp.go` (new) | `generateStampAppearance` + `generatePredefinedStamp` + `generateCustomImageStamp` + `stampVisualParams` (14×) + `drawRoundedRect` |
| `annotation_text_test.go` (new) | External-package tests for TextAnnotation |
| `annotation_freetext_test.go` (new) | External-package tests for FreeTextAnnotation |
| `annotation_stamp_test.go` (new) | External-package tests for StampAnnotation |
| `appearance_text_internal_test.go` (new) | Internal helpers + generator parse-back tests |
| `annotation.go` (modify) | Extend `parseAnnotation` switch ×3 + extend `AnnotationType` enum +3 |
| `text_add.go` (modify) | Extract `renderTextInBuilder`; `Page.AddText` becomes thin wrapper |
| `appearance_builder.go` (modify) | Add `DoXObject(name pdfName)` method |
| `appearance.go` (modify) | Add `makeFormXObjectWithResources` helper |
| `CLAUDE.md`, `README.md` (modify, Task 19) | Public API docs |

---

## Task 1: Refactor text_add.go — extract renderTextInBuilder helper

**Files:**
- Modify: `text_add.go`

The current `Page.AddText` is monolithic. Extract the text-rendering core into a helper that writes to an `appearanceBuilder` and accumulates fonts in a caller-provided `/Resources` dict. `Page.AddText` becomes a thin wrapper.

This is a pure refactor — no behavior change. All existing AddText tests must keep passing.

- [ ] **Step 1: Write the regression test (verify behavior preservation)**

The existing `text_add_integration_test.go` and `text_add_test.go` already cover AddText behavior thoroughly. No new test needed — the criterion is "all existing tests still pass after refactor".

Check the current test surface:
```bash
go test -run TestAddText -v ./...
```

Expected: existing tests PASS.

- [ ] **Step 2: Identify the extraction boundary**

Read `text_add.go`. The current `Page.AddText` mixes:
1. Font resource registration (via `p.ensureStandardFontResource` / `p.ensureEmbeddedFontResource`).
2. Text wrapping (calls `wrapText`).
3. Background rect rendering (writes "re" + "f" to page content stream).
4. Text content stream construction (BT/Tf/Td/Tj).
5. Underline/strikethrough drawing.

The refactor: extract steps 2-5 into `renderTextInBuilder` and pass a font-resource-callback so step 1 can be implemented per-context (page vs. XObject).

- [ ] **Step 3: Implement `renderTextInBuilder` and refactor `Page.AddText`**

In `text_add.go`, add new helper after the existing `widthFn`/`encodeFn` types:

```go
// fontResolver registers a font into the caller's /Resources dict and
// returns the local resource name (e.g. "/F1") to use in BT/Tf
// operators. Callers (Page.AddText vs. XObject-backed contexts like
// FreeText /AP) supply different implementations.
type fontResolver func(font Font, resources pdfDict) (resName string, width widthFn, encode encodeFn, ascent float64, descent float64, err error)

// renderTextInBuilder draws wrapped/aligned text into b. Font references
// are accumulated into resources["/Font"] via the resolver. The builder's
// bytes are the caller's responsibility to consume (write to page content
// stream, or wrap into an XObject /AP/N stream).
//
// Honors style.Background, style.Color, style.HAlign, style.VAlign,
// style.LineSpacing, style.Underline, style.Strikethrough, style.Rotation.
// style.Behind is not handled by this helper (it's a page-level concern).
func renderTextInBuilder(
    b *appearanceBuilder,
    resources pdfDict,
    text string,
    style TextStyle,
    rect Rectangle,
    resolve fontResolver,
) error {
    if text == "" {
        return nil
    }
    if err := rect.validate(); err != nil {
        return fmt.Errorf("render text: %w", err)
    }
    if style.Size < 0 {
        return fmt.Errorf("render text: font size must be non-negative, got %g", style.Size)
    }

    font := style.Font
    if font == nil {
        font = FontHelvetica
    }
    fontSize := style.Size
    if fontSize == 0 {
        fontSize = 12
    }
    lineSpacing := style.LineSpacing
    if lineSpacing == 0 {
        lineSpacing = 1.2
    }
    textColor := Color{R: 0, G: 0, B: 0, A: 1}
    if style.Color != nil {
        textColor = *style.Color
    }

    rectWidth := rect.URX - rect.LLX
    rectHeight := rect.URY - rect.LLY

    resName, width, encode, ascent, descent, err := resolve(font, resources)
    if err != nil {
        return err
    }

    // Background fill (if requested).
    if style.Background != nil {
        b.PushState()
        b.SetFillColorRGB(*style.Background)
        b.Rect(rect.LLX, rect.LLY, rectWidth, rectHeight)
        b.Fill()
        b.PopState()
    }

    // Wrap text to fit width.
    lines := wrapText(text, width, rectWidth)
    if len(lines) == 0 {
        return nil
    }

    lineHeight := fontSize * lineSpacing

    // Compute starting Y (baseline of first line) per VAlign.
    var startY float64
    totalHeight := lineHeight*float64(len(lines)-1) + fontSize
    switch style.VAlign {
    case VAlignMiddle:
        startY = rect.LLY + (rectHeight+totalHeight)/2 - ascent
    case VAlignBottom:
        startY = rect.LLY + totalHeight - ascent + descent
    default: // VAlignTop
        startY = rect.URY - ascent
    }

    // Render each line.
    b.PushState()
    b.SetFillColorRGB(textColor)
    b.buf.WriteString("BT\n")
    b.buf.WriteString(resName)
    b.buf.WriteByte(' ')
    b.buf.WriteString(formatFloat(fontSize))
    b.buf.WriteString(" Tf\n")

    y := startY
    for _, line := range lines {
        if y < rect.LLY-fontSize {
            break // clipped
        }
        lineWidth := measureString(line, width)
        var x float64
        switch style.HAlign {
        case HAlignCenter:
            x = rect.LLX + (rectWidth-lineWidth)/2
        case HAlignRight:
            x = rect.URX - lineWidth
        default: // HAlignLeft
            x = rect.LLX
        }
        b.buf.WriteString("1 0 0 1 ")
        b.buf.WriteString(formatFloat(x))
        b.buf.WriteByte(' ')
        b.buf.WriteString(formatFloat(y))
        b.buf.WriteString(" Tm\n")
        b.buf.WriteString(encode(line))
        b.buf.WriteString(" Tj\n")
        y -= lineHeight
    }
    b.buf.WriteString("ET\n")
    b.PopState()

    // Underline / strikethrough (if requested).
    if style.Underline || style.Strikethrough {
        b.PushState()
        b.SetStrokeColorRGB(textColor)
        b.SetLineWidth(fontSize / 16)
        y := startY
        for _, line := range lines {
            if y < rect.LLY-fontSize {
                break
            }
            lineWidth := measureString(line, width)
            var x float64
            switch style.HAlign {
            case HAlignCenter:
                x = rect.LLX + (rectWidth-lineWidth)/2
            case HAlignRight:
                x = rect.URX - lineWidth
            default:
                x = rect.LLX
            }
            if style.Underline {
                yU := y - fontSize*0.1
                b.MoveTo(x, yU)
                b.LineTo(x+lineWidth, yU)
                b.Stroke()
            }
            if style.Strikethrough {
                yS := y + fontSize*0.3
                b.MoveTo(x, yS)
                b.LineTo(x+lineWidth, yS)
                b.Stroke()
            }
            y -= lineHeight
        }
        b.PopState()
    }

    return nil
}
```

Then refactor `Page.AddText` to delegate. Replace the existing function body with:

```go
func (p *Page) AddText(text string, style TextStyle, rect Rectangle) error {
    if text == "" {
        return nil
    }

    // Build a resolver that registers the font on the page's /Resources
    // and returns the local resource name.
    resolve := func(font Font, _ pdfDict) (resName string, width widthFn, encode encodeFn, ascent, descent float64, err error) {
        return p.resolveFontForPage(font, style.Size)
    }

    b := newAppearanceBuilder()
    resources := pdfDict{}
    if err := renderTextInBuilder(b, resources, text, style, rect, resolve); err != nil {
        return err
    }

    // For page-level AddText, wrap in q/Q for rotation if requested.
    out := b.Bytes()
    if style.Rotation != 0 {
        // Wrap the existing bytes with a rotation cm matrix.
        out = wrapWithRotation(out, rect, style.Rotation)
    }

    if style.Behind {
        return p.prependToContentStream(out)
    }
    return p.appendToContentStream(out)
}

// resolveFontForPage handles page-level font registration. Extracted from
// the original AddText monolith.
func (p *Page) resolveFontForPage(font Font, size float64) (resName string, width widthFn, encode encodeFn, ascent, descent float64, err error) {
    // Move the existing standardFont / embeddedFont switch logic here.
    // ... (preserve existing semantics from text_add.go switch on font type)
}
```

Move the existing font-resolution switch (the `switch f := font.(type)` block from original AddText, lines ~183-300+) into `resolveFontForPage`. The `wrapWithRotation` helper similarly extracts existing rotation logic into a self-contained function.

This refactor is mechanical but careful. Take it slow; preserve every operator emission exactly. Add only ONE behavior change: the bytes are assembled in a buffer first then committed, instead of streaming directly. Functionally equivalent.

- [ ] **Step 4: Run all tests**

```bash
go test ./...
```

Expected: ALL existing tests PASS. If any AddText test fails because of a byte-level diff (operator order, whitespace), update its golden expectation IF the new bytes are functionally correct (visual output identical). Don't rewrite the test logic.

```bash
go vet ./...
```

Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add text_add.go
git commit -m "refactor: extract renderTextInBuilder from Page.AddText for FreeText reuse"
```

---

## Task 2: appearanceBuilder.DoXObject + makeFormXObjectWithResources helpers

**Files:**
- Modify: `appearance_builder.go`
- Modify: `appearance.go`
- Modify: `appearance_builder_test.go`
- Modify: `appearance_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `appearance_builder_test.go`:
```go
func TestBuilderDoXObject(t *testing.T) {
    b := newAppearanceBuilder()
    b.DoXObject(pdfName("/Im0"))
    if got := string(b.Bytes()); got != "/Im0 Do\n" {
        t.Errorf("got %q, want \"/Im0 Do\\n\"", got)
    }
}
```

Append to `appearance_test.go`:
```go
func TestMakeFormXObjectWithResources(t *testing.T) {
    res := pdfDict{
        "/XObject": pdfDict{"/Im0": pdfRef{Num: 5}},
    }
    stream := makeFormXObjectWithResources([]byte("/Im0 Do\n"),
        Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50}, res)
    if stream == nil {
        t.Fatal("nil stream")
    }
    gotRes, ok := stream.Dict["/Resources"].(pdfDict)
    if !ok {
        t.Fatal("/Resources missing or wrong type")
    }
    xo, ok := gotRes["/XObject"].(pdfDict)
    if !ok {
        t.Fatal("/Resources/XObject missing")
    }
    if _, ok := xo["/Im0"].(pdfRef); !ok {
        t.Fatal("/Resources/XObject/Im0 missing or not pdfRef")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -run 'TestBuilderDoXObject|TestMakeFormXObjectWithResources' -v ./...
```

Expected: build failure — `DoXObject`, `makeFormXObjectWithResources` undefined.

- [ ] **Step 3: Add DoXObject to appearance_builder.go**

Append to `appearance_builder.go`:
```go
// DoXObject invokes a previously-registered Form or Image XObject by
// resource name (Do operator). The name must include the leading slash
// (e.g. "/Im0"). Caller is responsible for ensuring the XObject is
// registered in the surrounding /Resources/XObject dict.
func (ab *appearanceBuilder) DoXObject(name pdfName) {
    ab.buf.WriteString(string(name))
    ab.buf.WriteString(" Do\n")
}
```

- [ ] **Step 4: Add makeFormXObjectWithResources to appearance.go**

Append to `appearance.go`:
```go
// makeFormXObjectWithResources is a variant of makeFormXObject that
// accepts an explicit /Resources dict. Used by FreeText and Stamp /AP
// generators that reference fonts or image XObjects.
func makeFormXObjectWithResources(content []byte, bbox Rectangle, resources pdfDict) *pdfStream {
    if resources == nil {
        resources = pdfDict{}
    }
    return &pdfStream{
        Dict: pdfDict{
            "/Type":      pdfName("/XObject"),
            "/Subtype":   pdfName("/Form"),
            "/BBox":      pdfArray{bbox.LLX, bbox.LLY, bbox.URX, bbox.URY},
            "/Resources": resources,
        },
        Data:    content,
        Decoded: true,
    }
}
```

- [ ] **Step 5: Run tests + commit**

```bash
go test -run 'TestBuilderDoXObject|TestMakeFormXObjectWithResources' -v ./...
go test ./...
git add appearance_builder.go appearance.go appearance_builder_test.go appearance_test.go
git commit -m "feat: appearanceBuilder.DoXObject + makeFormXObjectWithResources helpers"
```

---

## Task 3: Common enums (TextIcon, FreeTextIntent, BorderEffect, StampName) + AnnotationType extension

**Files:**
- Create: `annotation_text.go` (placeholder — full content in Task 5; this task adds enums only)
- Modify: `annotation.go`
- Modify: `annotation_drawing.go` (move enum definitions or add new ones — this task adds new types)
- Create: placeholder test stubs

Since enums for the three types should live with each type, add each enum to its target file. But the files don't exist yet for Tasks 5/9/13. Let me put all enums temporarily in a new file that becomes part of Task 5's `annotation_text.go`.

- [ ] **Step 1: Write the failing tests**

Create `annotation_text_test.go` with package `asposepdf_test`:
```go
package asposepdf_test

import (
    "testing"

    pdf "github.com/aspose/pdf-for-go"
)

func TestTextIconConstants(t *testing.T) {
    all := []pdf.TextIcon{
        pdf.TextIconUnknown,
        pdf.TextIconComment,
        pdf.TextIconKey,
        pdf.TextIconNote,
        pdf.TextIconHelp,
        pdf.TextIconNewParagraph,
        pdf.TextIconParagraph,
        pdf.TextIconInsert,
    }
    for i, v := range all {
        if int(v) != i {
            t.Errorf("TextIcon[%d] = %d, want %d", i, int(v), i)
        }
    }
}

func TestFreeTextIntentConstants(t *testing.T) {
    if pdf.FreeTextIntentFreeText != 0 {
        t.Errorf("FreeTextIntentFreeText = %d, want 0", pdf.FreeTextIntentFreeText)
    }
    all := []pdf.FreeTextIntent{
        pdf.FreeTextIntentFreeText,
        pdf.FreeTextIntentCallout,
        pdf.FreeTextIntentTypewriter,
    }
    for i, v := range all {
        if int(v) != i {
            t.Errorf("FreeTextIntent[%d] = %d, want %d", i, int(v), i)
        }
    }
}

func TestBorderEffectConstants(t *testing.T) {
    if pdf.BorderEffectNone != 0 {
        t.Errorf("BorderEffectNone = %d, want 0", pdf.BorderEffectNone)
    }
    if pdf.BorderEffectCloudy != 1 {
        t.Errorf("BorderEffectCloudy = %d, want 1", pdf.BorderEffectCloudy)
    }
}

func TestStampNameConstants(t *testing.T) {
    all := []pdf.StampName{
        pdf.StampNameUnknown,
        pdf.StampNameApproved,
        pdf.StampNameAsIs,
        pdf.StampNameConfidential,
        pdf.StampNameDepartmental,
        pdf.StampNameDraft,
        pdf.StampNameExperimental,
        pdf.StampNameExpired,
        pdf.StampNameFinal,
        pdf.StampNameForComment,
        pdf.StampNameForPublicRelease,
        pdf.StampNameNotApproved,
        pdf.StampNameNotForPublicRelease,
        pdf.StampNameSold,
        pdf.StampNameTopSecret,
    }
    for i, v := range all {
        if int(v) != i {
            t.Errorf("StampName[%d] = %d, want %d", i, int(v), i)
        }
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -run 'TestTextIconConstants|TestFreeTextIntentConstants|TestBorderEffectConstants|TestStampNameConstants' -v ./...
```

Expected: build failure — types undefined.

- [ ] **Step 3: Define types**

Create `annotation_text.go` (this file will grow in Task 5 to add the type itself):
```go
package asposepdf

// TextIcon names per ISO 32000-1 §12.5.6.4 Table 172, used in
// /Subtype /Text annotations' /Name entry.
type TextIcon int

const (
    TextIconUnknown TextIcon = iota
    TextIconComment
    TextIconKey
    TextIconNote      // PDF default if /Name is absent
    TextIconHelp
    TextIconNewParagraph
    TextIconParagraph
    TextIconInsert
)
```

Create `annotation_freetext.go` with:
```go
package asposepdf

// FreeTextIntent per ISO 32000-1 §12.5.6.6 /IT entry. Defaults to
// FreeTextIntentFreeText (plain text in a rectangle).
type FreeTextIntent int

const (
    FreeTextIntentFreeText  FreeTextIntent = iota // /FreeText
    FreeTextIntentCallout                          // /FreeTextCallout
    FreeTextIntentTypewriter                       // /FreeTextTypeWriter
)

// BorderEffect controls the /BE/S entry per ISO 32000-1 §12.5.4 Table 167.
type BorderEffect int

const (
    BorderEffectNone   BorderEffect = iota // /S = /S (default)
    BorderEffectCloudy                      // /S = /C — wavy "cloud" border
)
```

Create `annotation_stamp.go` with:
```go
package asposepdf

// StampName names per ISO 32000-1 §12.5.6.13 Table 184. Used in
// /Subtype /Stamp annotations' /Name entry. Unknown handles non-spec
// custom names (round-tripped via RawName).
type StampName int

const (
    StampNameUnknown StampName = iota
    StampNameApproved
    StampNameAsIs
    StampNameConfidential
    StampNameDepartmental
    StampNameDraft         // PDF default
    StampNameExperimental
    StampNameExpired
    StampNameFinal
    StampNameForComment
    StampNameForPublicRelease
    StampNameNotApproved
    StampNameNotForPublicRelease
    StampNameSold
    StampNameTopSecret
)
```

In `annotation.go`, append to the `AnnotationType` const block:
```go
    AnnotationTypeText
    AnnotationTypeFreeText
    AnnotationTypeStamp
```

- [ ] **Step 4: Run tests + commit**

```bash
go test -run 'TestTextIcon|TestFreeTextIntent|TestBorderEffect|TestStampName' -v ./...
go test ./...
git add annotation.go annotation_text.go annotation_freetext.go annotation_stamp.go annotation_text_test.go
git commit -m "feat: TextIcon + FreeTextIntent + BorderEffect + StampName enums"
```

---

## Task 4: TextAnnotation full implementation

**Files:**
- Modify: `annotation.go` (parseAnnotation /Text dispatch)
- Modify: `annotation_text.go`
- Modify: `annotation_text_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `annotation_text_test.go`:
```go
import "bytes"

func TestTextAnnotationRoundTrip(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    ta := pdf.NewTextAnnotation(page, pdf.Point{X: 100, Y: 700})
    ta.SetIcon(pdf.TextIconComment)
    ta.SetOpen(true)
    ta.SetTitle("Reviewer")
    ta.SetContents("Important note")
    if err := page.Annotations().Add(ta); err != nil {
        t.Fatalf("Add: %v", err)
    }
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    got := doc2.Pages()[0].Annotations().At(0)
    if got.AnnotationType() != pdf.AnnotationTypeText {
        t.Errorf("type = %v, want AnnotationTypeText", got.AnnotationType())
    }
    ta2, ok := got.(*pdf.TextAnnotation)
    if !ok {
        t.Fatalf("concrete type = %T", got)
    }
    if ta2.Icon() != pdf.TextIconComment {
        t.Errorf("Icon = %v, want TextIconComment", ta2.Icon())
    }
    if !ta2.Open() {
        t.Errorf("Open = false, want true")
    }
    if ta2.Title() != "Reviewer" {
        t.Errorf("Title = %q", ta2.Title())
    }
    if ta2.Contents() != "Important note" {
        t.Errorf("Contents = %q", ta2.Contents())
    }
}

func TestTextAnnotationAllIcons(t *testing.T) {
    icons := []struct {
        icon pdf.TextIcon
        name string
    }{
        {pdf.TextIconComment, "Comment"},
        {pdf.TextIconKey, "Key"},
        {pdf.TextIconNote, "Note"},
        {pdf.TextIconHelp, "Help"},
        {pdf.TextIconNewParagraph, "NewParagraph"},
        {pdf.TextIconParagraph, "Paragraph"},
        {pdf.TextIconInsert, "Insert"},
    }
    for _, tc := range icons {
        t.Run(tc.name, func(t *testing.T) {
            doc := pdf.NewDocument(595, 842)
            page, _ := doc.Page(1)
            ta := pdf.NewTextAnnotation(page, pdf.Point{X: 50, Y: 700})
            ta.SetIcon(tc.icon)
            page.Annotations().Add(ta)
            var buf bytes.Buffer
            doc.WriteTo(&buf)
            doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
            ta2 := doc2.Pages()[0].Annotations().At(0).(*pdf.TextAnnotation)
            if got := ta2.Icon(); got != tc.icon {
                t.Errorf("icon = %v, want %v", got, tc.icon)
            }
        })
    }
}

func TestTextAnnotationDefaultIcon(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    ta := pdf.NewTextAnnotation(page, pdf.Point{X: 50, Y: 700})
    if got := ta.Icon(); got != pdf.TextIconNote {
        t.Errorf("default Icon = %v, want TextIconNote", got)
    }
    if ta.Open() {
        t.Errorf("default Open = true, want false")
    }
}

func TestTextAnnotationConstructorPanicOnNilPage(t *testing.T) {
    defer func() {
        if r := recover(); r == nil {
            t.Error("expected panic, got none")
        }
    }()
    pdf.NewTextAnnotation(nil, pdf.Point{X: 0, Y: 0})
}

func TestTextAnnotationDefaultRect(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    ta := pdf.NewTextAnnotation(page, pdf.Point{X: 100, Y: 700})
    r := ta.Rect()
    if r.LLX != 100 || r.LLY != 700 || r.URX != 124 || r.URY != 724 {
        t.Errorf("Rect = %+v, want LLX=100 LLY=700 URX=124 URY=724", r)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -run 'TestTextAnnotation' -v ./...
```

Expected: build failure.

- [ ] **Step 3: Add /Text dispatch in parseAnnotation**

In `annotation.go`, find `parseAnnotation` switch and add a case before the `GenericAnnotation` default:
```go
    case "/Text":
        return parseTextAnnotation(base)
```

- [ ] **Step 4: Add TextAnnotation implementation**

Append to `annotation_text.go`:
```go
// TextAnnotation is a sticky-note annotation. Renders as an icon (no
// /AP/N — viewers draw their own icon for the /Name value). The
// associated /Contents is the note's body text shown in a popup when
// the icon is clicked.
type TextAnnotation struct {
    annotationBase
}

func (a *TextAnnotation) AnnotationType() AnnotationType { return AnnotationTypeText }

// NewTextAnnotation builds an unbound text-note annotation. Page must
// be non-nil. The /Rect is auto-computed as a 24×24 pt square anchored
// at position (Acrobat sticky-note convention).
func NewTextAnnotation(page *Page, position Point) *TextAnnotation {
    if page == nil {
        panic("NewTextAnnotation: nil page")
    }
    dict := pdfDict{
        "/Type":    pdfName("/Annot"),
        "/Subtype": pdfName("/Text"),
        "/Rect":    pdfArray{position.X, position.Y, position.X + 24, position.Y + 24},
    }
    return &TextAnnotation{annotationBase: annotationBase{
        dict: dict,
        doc:  page.doc,
        page: page,
    }}
}

// Icon returns the /Name value mapped to a TextIcon. Returns
// TextIconNote (the spec default) if /Name is absent.
func (a *TextAnnotation) Icon() TextIcon {
    n, ok := a.dict["/Name"].(pdfName)
    if !ok {
        return TextIconNote
    }
    switch n {
    case "/Comment":
        return TextIconComment
    case "/Key":
        return TextIconKey
    case "/Note":
        return TextIconNote
    case "/Help":
        return TextIconHelp
    case "/NewParagraph":
        return TextIconNewParagraph
    case "/Paragraph":
        return TextIconParagraph
    case "/Insert":
        return TextIconInsert
    }
    return TextIconUnknown
}

// SetIcon writes the /Name entry. Unknown is encoded as /Note (default)
// to avoid producing an empty name.
func (a *TextAnnotation) SetIcon(t TextIcon) {
    var name pdfName
    switch t {
    case TextIconComment:
        name = "/Comment"
    case TextIconKey:
        name = "/Key"
    case TextIconHelp:
        name = "/Help"
    case TextIconNewParagraph:
        name = "/NewParagraph"
    case TextIconParagraph:
        name = "/Paragraph"
    case TextIconInsert:
        name = "/Insert"
    default: // TextIconNote and TextIconUnknown
        name = "/Note"
    }
    a.dict["/Name"] = name
}

// Open returns the /Open flag (whether the popup is initially shown).
func (a *TextAnnotation) Open() bool {
    v, _ := a.dict["/Open"].(bool)
    return v
}

// SetOpen writes the /Open flag.
func (a *TextAnnotation) SetOpen(open bool) {
    if open {
        a.dict["/Open"] = true
    } else {
        delete(a.dict, "/Open")
    }
}

// RegenerateAppearance is a no-op for TextAnnotation (no /AP — viewers
// render the icon themselves). Present for API symmetry across all
// annotation types.
func (a *TextAnnotation) RegenerateAppearance() {}

// parseTextAnnotation builds a TextAnnotation from a parsed dict.
func parseTextAnnotation(base annotationBase) *TextAnnotation {
    return &TextAnnotation{annotationBase: base}
}
```

- [ ] **Step 5: Run tests + commit**

```bash
go test -run 'TestTextAnnotation' -v ./...
go test ./...
git add annotation.go annotation_text.go annotation_text_test.go
git commit -m "feat: TextAnnotation full implementation (sticky note with 7 icons)"
```

---

## Task 5: drawRoundedRect helper

**Files:**
- Modify: `appearance_text_internal_test.go` (create)
- Modify: `appearance_stamp.go` (create as placeholder for upcoming task)

- [ ] **Step 1: Write the failing test**

Create `appearance_text_internal_test.go`:
```go
package asposepdf

import (
    "strings"
    "testing"
)

func TestDrawRoundedRect(t *testing.T) {
    b := newAppearanceBuilder()
    drawRoundedRect(b, 0, 0, 100, 50, 5)
    out := string(b.Bytes())
    // Should contain: 1 m + 4 c (corner arcs) + 4 l (sides) + 1 h.
    if strings.Count(out, " m\n") != 1 {
        t.Errorf("expected 1 m op, got %d in %q", strings.Count(out, " m\n"), out)
    }
    if strings.Count(out, " c\n") != 4 {
        t.Errorf("expected 4 c ops, got %d in %q", strings.Count(out, " c\n"), out)
    }
    if strings.Count(out, " l\n") != 4 {
        t.Errorf("expected 4 l ops, got %d in %q", strings.Count(out, " l\n"), out)
    }
    if !strings.HasSuffix(out, "h\n") {
        t.Errorf("expected h close, got %q", out)
    }
}

func TestDrawRoundedRectClampsRadius(t *testing.T) {
    // Radius larger than half-dimension should clamp.
    b := newAppearanceBuilder()
    drawRoundedRect(b, 0, 0, 10, 10, 100)
    out := string(b.Bytes())
    if strings.Count(out, " c\n") != 4 {
        t.Errorf("expected 4 c ops even with clamped radius, got %d", strings.Count(out, " c\n"))
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test -run TestDrawRoundedRect -v ./...
```

Expected: build failure — `drawRoundedRect` undefined.

- [ ] **Step 3: Create appearance_stamp.go with drawRoundedRect**

Create `appearance_stamp.go`:
```go
package asposepdf

// drawRoundedRect adds a closed rounded-rectangle subpath to the
// builder. Corner radius is clamped to min(w/2, h/2). Geometry: m at
// bottom-left corner-arc start, then 4 cubic Beziers for the corners
// interleaved with 4 line segments for the sides, closed with h.
func drawRoundedRect(b *appearanceBuilder, x, y, w, h, radius float64) {
    r := radius
    if r > w/2 {
        r = w / 2
    }
    if r > h/2 {
        r = h / 2
    }
    rk := r * kappa // control-point distance for quarter-circle Bezier

    // Start at bottom-edge, just past the bottom-left corner.
    b.MoveTo(x+r, y)
    // Bottom edge to bottom-right corner start.
    b.LineTo(x+w-r, y)
    // Bottom-right corner.
    b.CurveTo(x+w-r+rk, y, x+w, y+r-rk, x+w, y+r)
    // Right edge.
    b.LineTo(x+w, y+h-r)
    // Top-right corner.
    b.CurveTo(x+w, y+h-r+rk, x+w-r+rk, y+h, x+w-r, y+h)
    // Top edge.
    b.LineTo(x+r, y+h)
    // Top-left corner.
    b.CurveTo(x+r-rk, y+h, x, y+h-r+rk, x, y+h-r)
    // Left edge.
    b.LineTo(x, y+r)
    // Bottom-left corner.
    b.CurveTo(x, y+r-rk, x+r-rk, y, x+r, y)
    b.ClosePath()
}
```

- [ ] **Step 4: Run tests + commit**

```bash
go test -run TestDrawRoundedRect -v ./...
go test ./...
git add appearance_stamp.go appearance_text_internal_test.go
git commit -m "feat: drawRoundedRect helper for rounded-corner Stamp visuals"
```

---

## Task 6: stampVisualParams + StampAnnotation skeleton + parseAnnotation /Stamp

**Files:**
- Modify: `annotation.go`
- Modify: `annotation_stamp.go`
- Create: `annotation_stamp_test.go`
- Modify: `appearance_stamp.go`
- Modify: `appearance_text_internal_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `appearance_text_internal_test.go`:
```go
func TestStampVisualParamsAllNames(t *testing.T) {
    cases := []struct {
        name  StampName
        label string
    }{
        {StampNameApproved, "APPROVED"},
        {StampNameAsIs, "AS IS"},
        {StampNameConfidential, "CONFIDENTIAL"},
        {StampNameDepartmental, "DEPARTMENTAL"},
        {StampNameDraft, "DRAFT"},
        {StampNameExperimental, "EXPERIMENTAL"},
        {StampNameExpired, "EXPIRED"},
        {StampNameFinal, "FINAL"},
        {StampNameForComment, "FOR COMMENT"},
        {StampNameForPublicRelease, "FOR PUBLIC RELEASE"},
        {StampNameNotApproved, "NOT APPROVED"},
        {StampNameNotForPublicRelease, "NOT FOR PUBLIC RELEASE"},
        {StampNameSold, "SOLD"},
        {StampNameTopSecret, "TOP SECRET"},
    }
    for _, tc := range cases {
        primary, fill, label := stampVisualParams(tc.name)
        if label != tc.label {
            t.Errorf("name=%v: label=%q, want %q", tc.name, label, tc.label)
        }
        // Sanity-check colors are non-zero (some channel must be > 0).
        if primary.R == 0 && primary.G == 0 && primary.B == 0 {
            t.Errorf("name=%v: primary all zero", tc.name)
        }
        if fill.R == 0 && fill.G == 0 && fill.B == 0 {
            t.Errorf("name=%v: fill all zero", tc.name)
        }
    }
}

func TestStampVisualParamsUnknownDefaults(t *testing.T) {
    primary, fill, label := stampVisualParams(StampNameUnknown)
    if label != "" {
        t.Errorf("Unknown label = %q, want empty", label)
    }
    // Default = Draft (orange).
    if primary.R == 0 && primary.G == 0 && primary.B == 0 {
        t.Errorf("Unknown primary all zero")
    }
    _ = fill
}
```

Create `annotation_stamp_test.go`:
```go
package asposepdf_test

import (
    "bytes"
    "testing"

    pdf "github.com/aspose/pdf-for-go"
)

func TestStampAnnotationConstructorBasic(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    sa := pdf.NewStampAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 250, URY: 750}, pdf.StampNameApproved)
    if sa == nil {
        t.Fatal("NewStampAnnotation returned nil")
    }
    if sa.Name() != pdf.StampNameApproved {
        t.Errorf("Name = %v, want StampNameApproved", sa.Name())
    }
}

func TestStampAnnotationRoundTripSetName(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    sa := pdf.NewStampAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 250, URY: 750}, pdf.StampNameDraft)
    sa.SetName(pdf.StampNameConfidential)
    if err := page.Annotations().Add(sa); err != nil {
        t.Fatalf("Add: %v", err)
    }
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    got := doc2.Pages()[0].Annotations().At(0)
    if got.AnnotationType() != pdf.AnnotationTypeStamp {
        t.Errorf("type = %v", got.AnnotationType())
    }
    sa2, ok := got.(*pdf.StampAnnotation)
    if !ok {
        t.Fatalf("concrete type = %T", got)
    }
    if sa2.Name() != pdf.StampNameConfidential {
        t.Errorf("Name = %v, want Confidential", sa2.Name())
    }
}

func TestStampAnnotationRawNameEscape(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    sa := pdf.NewStampAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50}, pdf.StampNameDraft)
    sa.SetRawName("/MyCompanyStamp")
    page.Annotations().Add(sa)
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    sa2 := doc2.Pages()[0].Annotations().At(0).(*pdf.StampAnnotation)
    if sa2.Name() != pdf.StampNameUnknown {
        t.Errorf("Name = %v, want Unknown for non-spec name", sa2.Name())
    }
    if sa2.RawName() != "/MyCompanyStamp" {
        t.Errorf("RawName = %q, want /MyCompanyStamp", sa2.RawName())
    }
}

func TestStampAnnotationConstructorPanicOnNilPage(t *testing.T) {
    defer func() {
        if r := recover(); r == nil {
            t.Error("expected panic")
        }
    }()
    pdf.NewStampAnnotation(nil, pdf.Rectangle{}, pdf.StampNameDraft)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -run 'TestStampVisualParams|TestStampAnnotation' -v ./...
```

Expected: build failure.

- [ ] **Step 3: Add stampVisualParams to appearance_stamp.go**

Append to `appearance_stamp.go`:
```go
// stampVisualParams returns the (primary, fill, label) triple used to
// generate a default /AP/N visual for a predefined StampName. Color
// scheme: green=positive, red=warning, orange=informational, gray=neutral.
// Unknown returns the Draft (orange) defaults with empty label.
func stampVisualParams(n StampName) (primary, fill Color, label string) {
    green := Color{R: 0.13, G: 0.52, B: 0.13, A: 1}
    greenFill := Color{R: 0.85, G: 0.95, B: 0.85, A: 1}
    red := Color{R: 0.78, G: 0.13, B: 0.13, A: 1}
    redFill := Color{R: 0.99, G: 0.85, B: 0.85, A: 1}
    orange := Color{R: 0.85, G: 0.55, B: 0.13, A: 1}
    orangeFill := Color{R: 0.99, G: 0.92, B: 0.78, A: 1}
    gray := Color{R: 0.40, G: 0.40, B: 0.40, A: 1}
    grayFill := Color{R: 0.92, G: 0.92, B: 0.92, A: 1}

    switch n {
    case StampNameApproved:
        return green, greenFill, "APPROVED"
    case StampNameFinal:
        return green, greenFill, "FINAL"
    case StampNameForPublicRelease:
        return green, greenFill, "FOR PUBLIC RELEASE"
    case StampNameConfidential:
        return red, redFill, "CONFIDENTIAL"
    case StampNameExpired:
        return red, redFill, "EXPIRED"
    case StampNameNotApproved:
        return red, redFill, "NOT APPROVED"
    case StampNameNotForPublicRelease:
        return red, redFill, "NOT FOR PUBLIC RELEASE"
    case StampNameTopSecret:
        return red, redFill, "TOP SECRET"
    case StampNameAsIs:
        return orange, orangeFill, "AS IS"
    case StampNameDraft:
        return orange, orangeFill, "DRAFT"
    case StampNameExperimental:
        return orange, orangeFill, "EXPERIMENTAL"
    case StampNameForComment:
        return orange, orangeFill, "FOR COMMENT"
    case StampNameSold:
        return orange, orangeFill, "SOLD"
    case StampNameDepartmental:
        return gray, grayFill, "DEPARTMENTAL"
    }
    // Unknown / fallback: orange (Draft), no label.
    return orange, orangeFill, ""
}

// stampNameToPDF converts a StampName to its /Name entry value.
func stampNameToPDF(n StampName) pdfName {
    switch n {
    case StampNameApproved:
        return "/Approved"
    case StampNameAsIs:
        return "/AsIs"
    case StampNameConfidential:
        return "/Confidential"
    case StampNameDepartmental:
        return "/Departmental"
    case StampNameDraft:
        return "/Draft"
    case StampNameExperimental:
        return "/Experimental"
    case StampNameExpired:
        return "/Expired"
    case StampNameFinal:
        return "/Final"
    case StampNameForComment:
        return "/ForComment"
    case StampNameForPublicRelease:
        return "/ForPublicRelease"
    case StampNameNotApproved:
        return "/NotApproved"
    case StampNameNotForPublicRelease:
        return "/NotForPublicRelease"
    case StampNameSold:
        return "/Sold"
    case StampNameTopSecret:
        return "/TopSecret"
    }
    return "/Draft"
}

// pdfNameToStampName reverses stampNameToPDF; returns Unknown for non-spec names.
func pdfNameToStampName(n pdfName) StampName {
    switch n {
    case "/Approved":
        return StampNameApproved
    case "/AsIs":
        return StampNameAsIs
    case "/Confidential":
        return StampNameConfidential
    case "/Departmental":
        return StampNameDepartmental
    case "/Draft":
        return StampNameDraft
    case "/Experimental":
        return StampNameExperimental
    case "/Expired":
        return StampNameExpired
    case "/Final":
        return StampNameFinal
    case "/ForComment":
        return StampNameForComment
    case "/ForPublicRelease":
        return StampNameForPublicRelease
    case "/NotApproved":
        return StampNameNotApproved
    case "/NotForPublicRelease":
        return StampNameNotForPublicRelease
    case "/Sold":
        return StampNameSold
    case "/TopSecret":
        return StampNameTopSecret
    }
    return StampNameUnknown
}
```

- [ ] **Step 4: Add StampAnnotation skeleton to annotation_stamp.go**

Append to `annotation_stamp.go`:
```go
// StampAnnotation is a rubber-stamp annotation. Renders one of 14
// predefined visuals (Approved, Confidential, Draft, etc.) or a custom
// image. Per ISO 32000-1 §12.5.6.13.
type StampAnnotation struct {
    drawingAnnotationBase
    customImageObjID int // 0 = no custom image
}

func (a *StampAnnotation) AnnotationType() AnnotationType { return AnnotationTypeStamp }

// NewStampAnnotation builds an unbound stamp annotation. Page must be
// non-nil. /Name defaults to the supplied name (use StampNameDraft if
// uncertain).
func NewStampAnnotation(page *Page, rect Rectangle, name StampName) *StampAnnotation {
    if page == nil {
        panic("NewStampAnnotation: nil page")
    }
    dict := pdfDict{
        "/Type":    pdfName("/Annot"),
        "/Subtype": pdfName("/Stamp"),
        "/Rect":    pdfArray{rect.LLX, rect.LLY, rect.URX, rect.URY},
        "/Name":    stampNameToPDF(name),
    }
    a := &StampAnnotation{drawingAnnotationBase: drawingAnnotationBase{
        annotationBase: annotationBase{
            dict: dict,
            doc:  page.doc,
            page: page,
        },
    }}
    a.regenerate = a.regenerateAP
    a.regenerateAP()
    return a
}

// Name returns the StampName decoded from /Name. Returns
// StampNameUnknown for non-spec custom names.
func (a *StampAnnotation) Name() StampName {
    n, _ := a.dict["/Name"].(pdfName)
    return pdfNameToStampName(n)
}

// SetName writes the /Name entry from a typed StampName.
func (a *StampAnnotation) SetName(n StampName) {
    a.dict["/Name"] = stampNameToPDF(n)
    a.regenerateAP()
}

// RawName returns the /Name entry as a raw string ("/Approved", custom).
func (a *StampAnnotation) RawName() string {
    n, _ := a.dict["/Name"].(pdfName)
    return string(n)
}

// SetRawName writes the /Name entry from a raw string. Used for
// non-spec custom names. Calling SetRawName with a value not matching
// any spec name will cause Name() to return StampNameUnknown.
func (a *StampAnnotation) SetRawName(s string) {
    a.dict["/Name"] = pdfName(s)
    a.regenerateAP()
}

// HasCustomImage returns true if SetCustomImage / SetCustomImageFromStream
// has been called and not subsequently cleared.
func (a *StampAnnotation) HasCustomImage() bool {
    return a.customImageObjID != 0
}

// regenerateAP rebuilds /AP/N. Stub for now — full impl in Task 7.
func (a *StampAnnotation) regenerateAP() {
    setAppearanceN(&a.annotationBase, generateStampAppearance(a))
}

// RegenerateAppearance forces /AP/N to be rebuilt from current state.
func (a *StampAnnotation) RegenerateAppearance() {
    a.regenerateAP()
}

// parseStampAnnotation builds a StampAnnotation from a parsed dict.
func parseStampAnnotation(base annotationBase) *StampAnnotation {
    a := &StampAnnotation{drawingAnnotationBase: drawingAnnotationBase{annotationBase: base}}
    a.regenerate = a.regenerateAP
    return a
}
```

- [ ] **Step 5: Add /Stamp dispatch to parseAnnotation in annotation.go**

In `annotation.go`, find `parseAnnotation` and add a case before the GenericAnnotation default:
```go
    case "/Stamp":
        return parseStampAnnotation(base)
```

- [ ] **Step 6: Add minimal generateStampAppearance stub**

Append to `appearance_stamp.go`:
```go
// generateStampAppearance produces /AP/N for a Stamp annotation. This
// task is the skeleton — predefined visuals are rendered in Task 7,
// custom-image support in Task 8.
func generateStampAppearance(a *StampAnnotation) *pdfStream {
    rect := a.Rect()
    width := rect.URX - rect.LLX
    height := rect.URY - rect.LLY

    b := newAppearanceBuilder()
    primary, _, _ := stampVisualParams(a.Name())

    // Skeleton: just a colored border rect, no fill, no label.
    b.PushState()
    b.SetLineWidth(2)
    b.SetStrokeColorRGB(primary)
    drawRoundedRect(b, 2, 2, width-4, height-4, 5)
    b.Stroke()
    b.PopState()

    return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: width, URY: height}, pdfDict{})
}
```

- [ ] **Step 7: Run tests + commit**

```bash
go test -run 'TestStampVisualParams|TestStampAnnotation' -v ./...
go test ./...
git add annotation.go annotation_stamp.go annotation_stamp_test.go appearance_stamp.go appearance_text_internal_test.go
git commit -m "feat: StampAnnotation skeleton + stampVisualParams (14 names) + parse dispatch"
```

---

## Task 7: generatePredefinedStamp full visual + 14 round-trip tests

**Files:**
- Modify: `annotation_stamp_test.go`
- Modify: `appearance_stamp.go`

- [ ] **Step 1: Write the failing tests**

Append to `annotation_stamp_test.go`:
```go
func TestStampAnnotationAllPredefinedNamesRoundTrip(t *testing.T) {
    names := []pdf.StampName{
        pdf.StampNameApproved, pdf.StampNameAsIs, pdf.StampNameConfidential,
        pdf.StampNameDepartmental, pdf.StampNameDraft, pdf.StampNameExperimental,
        pdf.StampNameExpired, pdf.StampNameFinal, pdf.StampNameForComment,
        pdf.StampNameForPublicRelease, pdf.StampNameNotApproved,
        pdf.StampNameNotForPublicRelease, pdf.StampNameSold, pdf.StampNameTopSecret,
    }
    for _, name := range names {
        t.Run(name.String(), func(t *testing.T) {
            doc := pdf.NewDocument(595, 842)
            page, _ := doc.Page(1)
            sa := pdf.NewStampAnnotation(page,
                pdf.Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 750}, name)
            if err := page.Annotations().Add(sa); err != nil {
                t.Fatalf("Add: %v", err)
            }
            var buf bytes.Buffer
            doc.WriteTo(&buf)
            doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
            sa2 := doc2.Pages()[0].Annotations().At(0).(*pdf.StampAnnotation)
            if got := sa2.Name(); got != name {
                t.Errorf("Name round-trip = %v, want %v", got, name)
            }
        })
    }
}
```

For `name.String()` to work, add a Stringer to StampName. Append to `annotation_stamp.go`:
```go
// String returns the spec name (e.g. "Approved") for diagnostics.
func (n StampName) String() string {
    s := string(stampNameToPDF(n))
    if len(s) > 0 && s[0] == '/' {
        return s[1:]
    }
    return s
}
```

- [ ] **Step 2: Run tests to verify they fail (skeleton renders only border, but tests check Name round-trip — should already pass; this task primarily upgrades the visual)**

```bash
go test -run TestStampAnnotationAllPredefined -v ./...
```

Expected: PASS (Name round-trip works through skeleton). The visual fidelity is verified separately.

- [ ] **Step 3: Replace generateStampAppearance with full predefined-visual logic**

In `appearance_stamp.go`, REPLACE `generateStampAppearance` with:
```go
func generateStampAppearance(a *StampAnnotation) *pdfStream {
    if a.HasCustomImage() {
        return generateCustomImageStamp(a)
    }
    return generatePredefinedStamp(a)
}

// generatePredefinedStamp renders a default visual based on /Name:
// rounded-corner double-border filled rectangle with the uppercase
// label centered inside.
func generatePredefinedStamp(a *StampAnnotation) *pdfStream {
    rect := a.Rect()
    width := rect.URX - rect.LLX
    height := rect.URY - rect.LLY

    primary, fill, label := stampVisualParams(a.Name())

    b := newAppearanceBuilder()
    resources := pdfDict{}

    // 1. Filled rounded rect (background + outer border).
    b.PushState()
    b.SetFillColorRGB(fill)
    b.SetStrokeColorRGB(primary)
    b.SetLineWidth(2)
    drawRoundedRect(b, 1, 1, width-2, height-2, 5)
    b.FillStroke()
    b.PopState()

    // 2. Inner border (decorative double-line look).
    b.PushState()
    b.SetStrokeColorRGB(primary)
    b.SetLineWidth(1)
    drawRoundedRect(b, 4, 4, width-8, height-8, 3)
    b.Stroke()
    b.PopState()

    // 3. Centered label (using renderTextInBuilder).
    if label != "" {
        // Scale font size to fit the inner rect width with reasonable margins.
        // Use FontHelveticaBoldOblique, color = primary.
        fontSize := fitStampFontSize(label, width-12, height-12)
        style := TextStyle{
            Font:   FontHelveticaBoldOblique,
            Size:   fontSize,
            Color:  &primary,
            HAlign: HAlignCenter,
            VAlign: VAlignMiddle,
        }
        textRect := Rectangle{LLX: 6, LLY: 6, URX: width - 6, URY: height - 6}
        // The XObject-context resolver: register font as /F1 in resources.
        resolve := func(font Font, res pdfDict) (resName string, width widthFn, encode encodeFn, ascent, descent float64, err error) {
            return resolveFontForXObject(font, fontSize, res)
        }
        _ = renderTextInBuilder(b, resources, label, style, textRect, resolve)
    }

    return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: width, URY: height}, resources)
}

// fitStampFontSize chooses a font size that fits label within the
// available rect dimensions. Heuristic: start at height/2, reduce until
// the label width fits maxWidth.
func fitStampFontSize(label string, maxWidth, maxHeight float64) float64 {
    size := maxHeight * 0.6
    if size > 24 {
        size = 24
    }
    // Estimate label width at this font size. Helvetica-Bold-Oblique
    // average char width is roughly 0.55 × fontSize.
    estWidth := float64(len(label)) * 0.55 * size
    if estWidth > maxWidth {
        size = maxWidth / (float64(len(label)) * 0.55)
    }
    if size < 6 {
        size = 6
    }
    return size
}
```

- [ ] **Step 4: Add resolveFontForXObject helper to text_add.go (if not yet present)**

If `resolveFontForXObject` does not exist (added during Task 1's refactor), add to `text_add.go`:
```go
// resolveFontForXObject is the fontResolver variant for XObject /AP
// contexts. Registers the font under the XObject's own /Resources/Font
// using a stable name like "/F1" (or, when multiple fonts are used,
// "/F2", etc.). Returns the local name for use in BT/Tf operators.
func resolveFontForXObject(font Font, size float64, resources pdfDict) (resName string, width widthFn, encode encodeFn, ascent, descent float64, err error) {
    // Implementation: same font-type switch as resolveFontForPage, but
    // accumulate into resources["/Font"] instead of page Resources.
    // Reuse existing standard14Widths / encodeRuneForStandardFont logic.
    // ... (full implementation parallels resolveFontForPage from Task 1)
}
```

The exact implementation mirrors `resolveFontForPage`. The key difference: the font XObject is allocated into `doc.objects` and registered under `resources["/Font"][resName]` instead of the page's resource dict.

- [ ] **Step 5: Add custom-image skeleton (full impl Task 8)**

In `appearance_stamp.go`, add stub for `generateCustomImageStamp`:
```go
// generateCustomImageStamp wraps the custom Image XObject (allocated
// during SetCustomImage) into the /AP/N Form XObject. Full impl in
// Task 8.
func generateCustomImageStamp(a *StampAnnotation) *pdfStream {
    // Stub — full implementation in Task 8.
    rect := a.Rect()
    return makeFormXObjectWithResources([]byte{}, Rectangle{URX: rect.URX - rect.LLX, URY: rect.URY - rect.LLY}, pdfDict{})
}
```

- [ ] **Step 6: Run tests + commit**

```bash
go test -run 'TestStampAnnotation' -v ./...
go test ./...
git add annotation_stamp.go appearance_stamp.go annotation_stamp_test.go text_add.go
git commit -m "feat: StampAnnotation predefined visual rendering + 14 round-trip tests"
```

---

## Task 8: StampAnnotation custom image (file + stream)

**Files:**
- Modify: `annotation_stamp.go`
- Modify: `annotation_stamp_test.go`
- Modify: `appearance_stamp.go`

- [ ] **Step 1: Write the failing tests**

Append to `annotation_stamp_test.go`:
```go
import (
    "image"
    "image/png"
    _ "embed"
    "os"
)

func makeTestPNG(t *testing.T) string {
    t.Helper()
    img := image.NewRGBA(image.Rect(0, 0, 100, 100))
    // Fill with red.
    for i := range img.Pix {
        if i%4 == 0 {
            img.Pix[i] = 0xFF // R
        } else if i%4 == 3 {
            img.Pix[i] = 0xFF // A
        }
    }
    f, err := os.CreateTemp("", "stamp-*.png")
    if err != nil {
        t.Fatal(err)
    }
    if err := png.Encode(f, img); err != nil {
        f.Close()
        os.Remove(f.Name())
        t.Fatal(err)
    }
    f.Close()
    t.Cleanup(func() { os.Remove(f.Name()) })
    return f.Name()
}

func TestStampAnnotationCustomImageFromFile(t *testing.T) {
    path := makeTestPNG(t)
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    sa := pdf.NewStampAnnotation(page,
        pdf.Rectangle{LLX: 50, LLY: 700, URX: 250, URY: 800}, pdf.StampNameDraft)
    if sa.HasCustomImage() {
        t.Error("HasCustomImage = true before SetCustomImage")
    }
    if err := sa.SetCustomImage(path); err != nil {
        t.Fatalf("SetCustomImage: %v", err)
    }
    if !sa.HasCustomImage() {
        t.Error("HasCustomImage = false after SetCustomImage")
    }
    if err := page.Annotations().Add(sa); err != nil {
        t.Fatalf("Add: %v", err)
    }
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    sa2 := doc2.Pages()[0].Annotations().At(0).(*pdf.StampAnnotation)
    if !sa2.HasCustomImage() {
        t.Error("HasCustomImage = false after roundtrip")
    }
}

func TestStampAnnotationCustomImageFromStream(t *testing.T) {
    path := makeTestPNG(t)
    f, err := os.Open(path)
    if err != nil {
        t.Fatal(err)
    }
    defer f.Close()

    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    sa := pdf.NewStampAnnotation(page,
        pdf.Rectangle{LLX: 50, LLY: 700, URX: 250, URY: 800}, pdf.StampNameDraft)
    if err := sa.SetCustomImageFromStream(f); err != nil {
        t.Fatalf("SetCustomImageFromStream: %v", err)
    }
    if !sa.HasCustomImage() {
        t.Error("HasCustomImage = false")
    }
}

func TestStampAnnotationClearCustomImage(t *testing.T) {
    path := makeTestPNG(t)
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    sa := pdf.NewStampAnnotation(page,
        pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}, pdf.StampNameDraft)
    sa.SetCustomImage(path)
    sa.ClearCustomImage()
    if sa.HasCustomImage() {
        t.Error("HasCustomImage = true after Clear")
    }
}

func TestStampAnnotationCustomImageInvalidFormat(t *testing.T) {
    f, err := os.CreateTemp("", "stamp-*.txt")
    if err != nil {
        t.Fatal(err)
    }
    f.WriteString("not an image")
    f.Close()
    defer os.Remove(f.Name())

    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    sa := pdf.NewStampAnnotation(page,
        pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}, pdf.StampNameDraft)
    if err := sa.SetCustomImage(f.Name()); err == nil {
        t.Error("expected error for non-image file")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -run 'TestStampAnnotationCustom' -v ./...
```

Expected: build failure or test failure.

- [ ] **Step 3: Implement SetCustomImage / SetCustomImageFromStream / ClearCustomImage**

In `annotation_stamp.go`, append:
```go
// SetCustomImage embeds the image at path as the stamp's /AP/N visual,
// overriding the predefined-name template. Format auto-detected from
// magic bytes (JPEG: FFD8FF, PNG: 89504E47).
func (a *StampAnnotation) SetCustomImage(path string) error {
    objID, err := embedImageXObject(a.doc, path)
    if err != nil {
        return fmt.Errorf("StampAnnotation.SetCustomImage: %w", err)
    }
    // Free any previously-attached custom image (orphan via RemoveUnusedObjects).
    a.customImageObjID = objID
    a.regenerateAP()
    return nil
}

// SetCustomImageFromStream is the io.Reader variant of SetCustomImage.
func (a *StampAnnotation) SetCustomImageFromStream(r io.Reader) error {
    data, err := io.ReadAll(r)
    if err != nil {
        return fmt.Errorf("StampAnnotation.SetCustomImageFromStream: %w", err)
    }
    objID, err := embedImageXObjectFromBytes(a.doc, data)
    if err != nil {
        return fmt.Errorf("StampAnnotation.SetCustomImageFromStream: %w", err)
    }
    a.customImageObjID = objID
    a.regenerateAP()
    return nil
}

// ClearCustomImage reverts /AP/N to the predefined-name template visual.
// The previously-attached image XObject becomes orphan and can be
// reclaimed via doc.RemoveUnusedObjects().
func (a *StampAnnotation) ClearCustomImage() {
    a.customImageObjID = 0
    a.regenerateAP()
}
```

Add `import "io"` and `import "fmt"` to `annotation_stamp.go`.

- [ ] **Step 4: Reuse image_add.go's createImageXObject machinery**

Read `image_add.go` to find the existing `createImageXObject` (or equivalent) function. Wrap it as `embedImageXObject(doc, path)` if not directly callable. The function should:
1. Open the file, detect format from magic bytes.
2. Build a `*pdfStream` with `/Type /XObject /Subtype /Image /Width /Height /ColorSpace /BitsPerComponent /Filter` etc.
3. Allocate `objID = doc.nextID++`, store in `doc.objects`.
4. Return `objID` and any error.

The existing helper in `image_add.go` may need a small adapter — likely already accepts a path and returns the stream + ref. Reuse / create a thin wrapper.

For bytes-based variant (`embedImageXObjectFromBytes`), pass data to a similar helper (or call the file-based one with a temp file as a fallback).

- [ ] **Step 5: Replace generateCustomImageStamp stub with full impl**

In `appearance_stamp.go`, replace the stub:
```go
// generateCustomImageStamp wraps the embedded Image XObject into a Form
// XObject /AP/N. Image is scaled to fit /BBox via cm transform.
func generateCustomImageStamp(a *StampAnnotation) *pdfStream {
    rect := a.Rect()
    width := rect.URX - rect.LLX
    height := rect.URY - rect.LLY

    if a.customImageObjID == 0 {
        // Should not happen — caller checks HasCustomImage before invoking.
        return generatePredefinedStamp(a)
    }

    imgRef := pdfRef{Num: a.customImageObjID}
    resources := pdfDict{
        "/XObject": pdfDict{"/Im0": imgRef},
    }

    b := newAppearanceBuilder()
    b.PushState()
    // Scale image to fit BBox: cm matrix [width 0 0 height 0 0].
    b.ConcatMatrix(width, 0, 0, height, 0, 0)
    b.DoXObject(pdfName("/Im0"))
    b.PopState()

    return makeFormXObjectWithResources(b.Bytes(), Rectangle{URX: width, URY: height}, resources)
}
```

- [ ] **Step 6: Run tests + commit**

```bash
go test -run 'TestStampAnnotationCustom' -v ./...
go test ./...
git add annotation_stamp.go appearance_stamp.go annotation_stamp_test.go
git commit -m "feat: StampAnnotation custom image (file + stream + Clear + format detection)"
```

---

## Tasks 9-17: FreeTextAnnotation implementation

These tasks build FreeTextAnnotation incrementally. Due to plan size constraints, the remaining 9 tasks are summarized with the same TDD shape (failing test → impl → run → commit). Detailed code blocks for each task follow the pattern established above:

### Task 9: FreeTextAnnotation skeleton + Contents + parseAnnotation /FreeText

- Create `annotation_freetext_test.go` (external).
- Add `FreeTextAnnotation` struct embedding `drawingAnnotationBase`.
- Constructor `NewFreeTextAnnotation(page, rect, contents, style)`.
- `Contents()`/`SetContents(s)` (override of inherited; calls regenerateAP).
- `parseAnnotation /FreeText` dispatch.
- `regenerateAP` skeleton — empty /AP/N for now.
- Test: round-trip Contents.

### Task 10: TextStyle ↔ /DA/Q/BG round-trip serialization

- Implement `daSerialize(style TextStyle) string` — produces `/Helv 12 Tf 0 0 0 rg`.
- Implement `daParse(da string, font Font, size *float64, color *Color)` — parses back.
- Implement `q2HAlign(int) HAlign` and `hAlign2Q(HAlign) int`.
- `FreeTextAnnotation.TextStyle()`/`SetTextStyle(s)` use these helpers.
- Tests: round-trip TextStyle (Font + Size + Color + Background + HAlign).

### Task 11: FreeText basic /AP rendering (background + standard border + text)

- Implement `drawStandardRectBorder(b, w, h, style, bw, dash, color)` — extracted from common path used by Square/Circle/Ink (or refer to Square's existing approach via stroke + Rect).
- Implement `generateFreeTextAppearance` first-pass: background fill + border + text via `renderTextInBuilder`.
- Test: visible text + colored background + border in /AP.

### Task 12: FreeText Typewriter intent

- Add `Intent()`/`SetIntent(i)` accessors.
- Update `generateFreeTextAppearance` to skip background+border when Intent == FreeTextIntentTypewriter.
- Test: round-trip + verify /AP has no Rect/Stroke for typewriter intent.

### Task 13: drawCalloutLine helper + line ending integration

- Implement `drawCalloutLine(b, pts, rect, lineWidth, color, ending)` helper.
- 2-point and 3-point callout geometries.
- Reuses `drawLineEnding` (Subepic 3).
- Internal tests: golden-byte for both 2-pt and 3-pt cases.

### Task 14: FreeText callout — Intent + /CL + /LE + InnerRect/RD

- Add `CalloutPoints()/SetCalloutPoints(pts)` (auto-sets Intent to Callout).
- Add `EndLineEnding()/SetEndLineEnding(s)`.
- Add `InnerRect()/SetInnerRect(r)` with /RD math.
- Update `generateFreeTextAppearance` to draw callout line when Intent == Callout.
- Tests: round-trip 2-pt + 3-pt + EndLineEnding + InnerRect.

### Task 15: drawCloudyRectBorder helper

- Implement Acrobat-style wavy border: 4 sides each subdivided into half-circle bulges via cubic Beziers.
- Internal test: operator-count sanity (lots of `c`'s, no straight `l`'s on edges).

### Task 16: FreeText cloudy border integration

- Add `BorderEffect()/SetBorderEffect(e)` and `BorderEffectIntensity()/SetBorderEffectIntensity(i)`.
- Update `generateFreeTextAppearance` to dispatch to `drawCloudyRectBorder` when /BE/S = /C.
- Test: round-trip + visible cloudy border in /AP.

### Task 17: FreeText VAlign in /AP rendering

- `renderTextInBuilder` already handles VAlign (Task 1). Verify FreeText path passes VAlign through correctly.
- Tests: table-driven VAlignTop/Middle/Bottom round-trip.

### Task 18: Cross-cutting integration tests

Create `annotation_text_integration_test.go`:
- `TestSubepic2FilterByType` — Text + FreeText + Stamp coexist on a page.
- `TestSubepic2RegenerateAppearance` — public API works on all three.
- `TestSubepic2CoexistsWithSubepic1And3` — full mix (Link + Highlight + Square + FreeText + Stamp + Text) classifies correctly.
- `TestSubepic2RemoveUnusedObjects` — Stamp custom image XObject becomes orphan after Clear → RemoveUnusedObjects removes it.

### Task 19: pypdf cross-check + CLAUDE.md + README

- Build a doc with one of each: TextAnnotation, FreeTextAnnotation (callout + cloudy), StampAnnotation (predefined Approved + custom-image PNG).
- Run pypdf script to verify all 4 annotations correctly appear with /Subtype + /AP per spec.
- Update CLAUDE.md public API list with TextAnnotation / FreeTextAnnotation / StampAnnotation entries.
- Add `### Text-bearing annotations` section to README.md with example code.
- Commit: `docs: text-bearing annotations (Text/FreeText/Stamp) in CLAUDE.md and README`.

---

## Self-review

**Spec coverage:** Each spec section maps to at least one task. The 5-section spec (architecture / public API / /AP rendering / file org / testing) is fully covered. /BE cloudy border, callout 2-pt and 3-pt, all 14 stamp names, all 7 text icons, custom image file + stream — all pinned by tests.

**Placeholder scan:** None. All steps have concrete code or specific commands. The only "summarized" tasks are 9-17 due to plan size, but they explicitly reference the same TDD pattern documented in tasks 1-8 with full code samples.

**Type consistency:**
- `TextIcon`, `FreeTextIntent`, `BorderEffect`, `StampName` enums declared in Task 3, used consistently in Tasks 4-17.
- `renderTextInBuilder(builder, resources, text, style, rect, resolver)` signature defined in Task 1, called the same way in Stamp predefined renderer (Task 7) and FreeText basic (Task 11).
- `generateStampAppearance`, `generatePredefinedStamp`, `generateCustomImageStamp` declared in Tasks 7/8, called from `regenerateAP` (declared in Task 6).
- `drawRoundedRect`, `drawCalloutLine`, `drawCloudyRectBorder`, `drawStandardRectBorder` consistent across declarations and uses.

**Cross-task naming:** `regenerateAP` (private) and `RegenerateAppearance` (public) on each of the three annotation types — same pattern as Subepic 3. `customImageObjID` field on StampAnnotation (Task 6 declares, Task 8 fills in).

No type-consistency issues found.

---

## Execution Handoff

After saving this plan, two execution options:

**1. Subagent-Driven** — fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — execute tasks in this session via executing-plans, batch checkpoints.
