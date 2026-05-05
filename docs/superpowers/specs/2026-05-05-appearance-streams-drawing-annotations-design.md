# Appearance Streams + Drawing Annotations — Design Spec

**Date:** 2026-05-05
**Epic:** `pdf-go-37n` (Annotations umbrella). This is the second tranche of work
under that epic, combining the `/AP` foundation with Subepic 3 (drawing primitives)
in one plan.
**Previous tranche:** Subepic 1 (Link + Highlight family + Actions) shipped
2026-05-05.

## Goal

Add the foundational `/AP` (appearance stream) generation infrastructure required for
spec-conforming rendering of drawing annotations, and ship the four geometric annotation
types: `Square`, `Circle`, `Line`, `Ink`. Together these complete Subepic 3 of the
annotations epic and unblock subsequent subepics (FreeText, Stamp, etc.) that also need
`/AP` infrastructure.

Users get drawable annotations that render correctly in any spec-conforming viewer
(Acrobat, Foxit, SumatraPDF, browser PDF) without relying on `/NeedAppearances=true`.

## Non-goals

- Subepic 2 types (Text/sticky-note, FreeText, Stamp).
- Subepic 4 types (FileAttachment, Redact, JavaScript-construct).
- Constructing JavaScript actions (security policy from Subepic 1 stays).
- `/AP/R` (rollover) and `/AP/D` (down) variants — only `/AP/N` (normal).
- Transparency (`/CA`, `/ca` in `/ExtGState`).
- Patterns, gradients (`/Pattern`, `/Shading`).
- Custom blend modes.
- Polygon, PolyLine subtypes (separate future work).

## Architecture

Three-layer split with clean boundaries.

```
┌──────────────────────────────────────────────────────────┐
│ annotation_drawing.go (public)                           │
│   SquareAnnotation / CircleAnnotation / LineAnnotation / │
│   InkAnnotation. Constructors + property setters.        │
│   Each setter mutates pdfDict + calls regenerateAP().    │
└─────────────────────┬────────────────────────────────────┘
                      │
┌─────────────────────▼────────────────────────────────────┐
│ appearance.go (private)                                  │
│   generateSquareAppearance / Circle / Line / Ink.        │
│   setAppearanceN helper (allocate-or-mutate XObject).    │
│   Border style dispatch (5 variants).                    │
│   drawLineEnding (10 styles).                            │
│   Catmull-Rom → Bezier converter.                        │
└─────────────────────┬────────────────────────────────────┘
                      │
┌─────────────────────▼────────────────────────────────────┐
│ appearance_builder.go (private)                          │
│   appearanceBuilder type — typed wrapper over            │
│   bytes.Buffer. PDF content-stream operators (q/Q/cm/w/  │
│   J/j/d/M, RG/rg/G/g, m/l/c/re/h, S/s/f/B/b/n).         │
│   Ellipse helper (4-Bezier circle approximation).        │
└──────────────────────────────────────────────────────────┘
```

## Public API

### Common new types

```go
// Point — one point in PDF user-space.
type Point struct {
    X, Y float64
}

// BorderStyle controls the /BS dict for drawing annotations.
// Per ISO 32000-1 §12.5.4 Table 168.
type BorderStyle int

const (
    BorderSolid     BorderStyle = iota // /S = /S
    BorderDashed                        // /S = /D + /D dash array
    BorderBeveled                       // /S = /B (3D raised)
    BorderInset                         // /S = /I (3D recessed)
    BorderUnderline                     // /S = /U (bottom edge only)
)

// LineEndingStyle is one of the 10 line-ending shapes per ISO 32000-1
// §12.5.6.7 Table 176 (the /LE entry on /Line annotations).
type LineEndingStyle int

const (
    LineEndingNone LineEndingStyle = iota
    LineEndingSquare
    LineEndingCircle
    LineEndingDiamond
    LineEndingOpenArrow
    LineEndingClosedArrow
    LineEndingButt
    LineEndingROpenArrow   // reverse direction
    LineEndingRClosedArrow
    LineEndingSlash
)
```

### Common border API on every drawing annotation

`SquareAnnotation`, `CircleAnnotation`, `LineAnnotation`, `InkAnnotation` all expose:

```go
.BorderWidth() float64
.SetBorderWidth(w float64)
.BorderStyle() BorderStyle
.SetBorderStyle(s BorderStyle)
.DashPattern() []float64           // returns a defensive copy
.SetDashPattern(p []float64)       // accepts a defensive copy
```

Defaults: `BorderWidth = 1`, `BorderStyle = BorderSolid`, `DashPattern = nil`.
When `BorderStyle = BorderDashed` and `DashPattern == nil`, the renderer uses `[3, 3]`.

### `SquareAnnotation` and `CircleAnnotation`

Identical API surface; only the `/Subtype` differs.

```go
NewSquareAnnotation(page *Page, rect Rectangle) *SquareAnnotation
NewCircleAnnotation(page *Page, rect Rectangle) *CircleAnnotation

// Plus all common border accessors.
// Plus inherited from annotationBase: Rect/SetRect, Color/SetColor (stroke),
// Title/SetTitle, Contents/SetContents, PageIndex.

// Type-specific:
.InteriorColor() *Color    // /IC — fill color. nil = no fill, stroke-only.
.SetInteriorColor(c *Color)
```

### `LineAnnotation`

Constructor takes two endpoints (not a rectangle); `/Rect` is auto-computed as the
bounding box plus padding for line endings.

```go
NewLineAnnotation(page *Page, start Point, end Point) *LineAnnotation

.Start() Point
.End() Point
.SetStart(p Point)
.SetEnd(p Point)

.StartLineEnding() LineEndingStyle
.EndLineEnding() LineEndingStyle
.SetStartLineEnding(s LineEndingStyle)
.SetEndLineEnding(s LineEndingStyle)

.InteriorColor() *Color           // fill color for arrow heads etc.
.SetInteriorColor(c *Color)

.LeaderLineLength() float64        // /LL — for dimension lines
.SetLeaderLineLength(l float64)

// Inherited: Color (stroke), Title, Contents, BorderWidth, BorderStyle, DashPattern.
// Note: SetRect on LineAnnotation regenerates from Start/End — direct rect mutation
// is preserved for spec-compliance but the auto-computed bbox wins on subsequent
// SetStart/SetEnd calls.
```

### `InkAnnotation`

```go
NewInkAnnotation(page *Page, strokes [][]Point) *InkAnnotation

.Strokes() [][]Point         // defensive deep copy
.SetStrokes(strokes [][]Point)
.AddStroke(stroke []Point)   // convenience for incremental construction

// Inherited: Color (stroke), Title, Contents, BorderWidth, BorderStyle, DashPattern.
// No InteriorColor: ink strokes don't fill.
```

The `/InkList` PDF entry stores raw points; the `/AP` renderer applies Catmull-Rom
smoothing for visual quality (see "Ink rendering" below).

### `AnnotationType` enum additions

```go
AnnotationTypeSquare
AnnotationTypeCircle
AnnotationTypeLine
AnnotationTypeInk
```

### Explicit appearance regeneration

Every setter on the four drawing types regenerates `/AP/N` immediately. For exotic
cases where a caller mutates the underlying dict directly (e.g. tests, debug tooling),
a public method is provided:

```go
.RegenerateAppearance()    // on every drawing annotation
```

## Internal infrastructure

### `appearanceBuilder`

Single file `appearance_builder.go`. Receiver-based methods on `*appearanceBuilder`,
each emitting one PDF content-stream operator into an internal `bytes.Buffer`. All
numbers formatted with `strconv.FormatFloat(v, 'f', -1, 64)` (no scientific notation,
no trailing zeros — matches existing project convention from `text_add.go`).

```go
type LineCap int
const (
    LineCapButt LineCap = iota   // 0
    LineCapRound                 // 1
    LineCapSquare                // 2
)

type LineJoin int
const (
    LineJoinMiter LineJoin = iota // 0
    LineJoinRound                 // 1
    LineJoinBevel                 // 2
)

type appearanceBuilder struct {
    buf bytes.Buffer
}

func newAppearanceBuilder() *appearanceBuilder

// Graphics state
func (b *appearanceBuilder) PushState()                            // q
func (b *appearanceBuilder) PopState()                             // Q
func (b *appearanceBuilder) ConcatMatrix(a, b, c, d, e, f float64) // cm

// Line attributes
func (b *appearanceBuilder) SetLineWidth(w float64)                       // w
func (b *appearanceBuilder) SetLineCap(c LineCap)                         // J
func (b *appearanceBuilder) SetLineJoin(j LineJoin)                       // j
func (b *appearanceBuilder) SetMiterLimit(m float64)                      // M
func (b *appearanceBuilder) SetDashPattern(pattern []float64, phase float64) // d

// Color
func (b *appearanceBuilder) SetStrokeColorRGB(c Color)   // R G B RG
func (b *appearanceBuilder) SetFillColorRGB(c Color)     // r g b rg
func (b *appearanceBuilder) SetStrokeGray(g float64)     // G
func (b *appearanceBuilder) SetFillGray(g float64)       // g

// Path construction
func (b *appearanceBuilder) MoveTo(x, y float64)                              // m
func (b *appearanceBuilder) LineTo(x, y float64)                              // l
func (b *appearanceBuilder) CurveTo(x1, y1, x2, y2, x3, y3 float64)           // c
func (b *appearanceBuilder) Rect(x, y, w, h float64)                          // re
func (b *appearanceBuilder) Ellipse(cx, cy, rx, ry float64)                   // helper:
                                                                              //   m + 4×c forming
                                                                              //   ellipse via
                                                                              //   kappa = 0.5522847498
func (b *appearanceBuilder) ClosePath()                                       // h

// Painting
func (b *appearanceBuilder) Stroke()                  // S
func (b *appearanceBuilder) ClosePathStroke()         // s
func (b *appearanceBuilder) Fill()                    // f (non-zero rule)
func (b *appearanceBuilder) FillStroke()              // B
func (b *appearanceBuilder) ClosePathFillStroke()     // b
func (b *appearanceBuilder) EndPath()                 // n (discard path)

// Result
func (b *appearanceBuilder) Bytes() []byte
```

The builder does not validate operator order (e.g. it doesn't enforce that `Stroke`
follows path construction). Generators are expected to produce balanced sequences.

### Generators

In `appearance.go`. Each function reads the annotation's properties from its dict,
builds a content stream via `appearanceBuilder`, returns `*pdfStream` ready for
`setAppearanceN`.

```go
func generateSquareAppearance(a *SquareAnnotation) *pdfStream
func generateCircleAppearance(a *CircleAnnotation) *pdfStream
func generateLineAppearance(a *LineAnnotation) *pdfStream
func generateInkAppearance(a *InkAnnotation) *pdfStream
```

#### Coordinate system

`/AP/N` is a Form XObject. Its `/BBox` is `[0, 0, width, height]` where
`width = Rect.URX - Rect.LLX`. The renderer draws in local 0-based coordinates;
the viewer scales/positions the XObject inside `/Rect` on the page. This is the
standard PDF idiom (matches Acrobat output).

For `LineAnnotation`, the bbox padding accommodates line endings: each endpoint is
extended by exactly `9 × BorderWidth()` in every direction relative to the line's
geometry. This matches Acrobat's convention and is also the size used by
`drawLineEnding` itself, so endings never exceed the bbox.

#### Border style implementations

All five variants share scaffolding (PushState → set color/width/dash → path → paint
→ PopState). Variants differ in path construction and paint operator:

- **Solid**: single path (`Rect` for Square, `Ellipse` for Circle), single Stroke or
  FillStroke.
- **Dashed**: same as Solid plus `SetDashPattern(p, 0)` before Stroke. `p` defaults to
  `[3, 3]` if user did not set one.
- **Beveled**: two-pass render. Light color (= color × 0.5 + white × 0.5) for top/left
  edges; dark color (= color × 0.5) for bottom/right edges. On Square: two L-shaped
  paths. On Circle: two semicircles. On Line/Ink: light/dark offset perpendicular.
- **Inset**: same as Beveled with light/dark swapped.
- **Underline**: Square draws only the bottom edge (`m LLX,LLY l URX,LLY S`); Circle
  draws the lower semicircle; Line and Ink fall back to Solid (spec doesn't define
  Underline for these — graceful degradation).

#### Line endings

Helper `drawLineEnding(b, style, x, y, theta, lineWidth, fill)` renders one ending at
position `(x, y)` rotated to angle `theta` (radians, direction toward line interior).
Acrobat-equivalent sizing: ending span = exactly `9 × lineWidth`. Implementation per
style:

| Style | Geometry | Fill? |
|---|---|---|
| `None` | empty | n/a |
| `Square` | square centered on point, side = ending span | `InteriorColor` if set |
| `Circle` | circle centered on point, diameter = ending span | `InteriorColor` if set |
| `Diamond` | rhombus centered on point | `InteriorColor` if set |
| `OpenArrow` | two lines from point at ±30° (no close) | no |
| `ClosedArrow` | triangle (3 points + close) | `InteriorColor` if set, else stroke color |
| `Butt` | short perpendicular segment across the line | no |
| `ROpenArrow` | OpenArrow rotated 180° (away from line) | no |
| `RClosedArrow` | ClosedArrow rotated 180° | `InteriorColor` if set |
| `Slash` | diagonal line at 60° | no |

Each ending is wrapped in its own `q ... Q` block with a local rotation transform
(`cm`) so the geometry can be authored in axis-aligned coordinates and rotated to fit.

#### Ink rendering (Catmull-Rom)

For each stroke (array of points):

- 0 or 1 points: emit nothing (zero-area stroke).
- 2 points: simple `m x1 y1 l x2 y2 S` (no smoothing possible).
- 3+ points: Catmull-Rom smoothing. For each segment between consecutive points
  `P[i]` and `P[i+1]`, generate a cubic Bezier with control points:

  ```
  C1 = P[i]   + (P[i+1] - P[i-1]) / 6
  C2 = P[i+1] - (P[i+2] - P[i]  ) / 6
  ```

  Phantom points: `P[-1] = P[0]` (mirror the first segment) and
  `P[n] = P[n-1]` (mirror the last segment). Tension factor 0.5 (standard
  Catmull-Rom). Emit as `m P[0] / c (C1, C2, P[1]) / c (C1, C2, P[2]) / ... / S`.

The raw `/InkList` array stores the original points unchanged; smoothing is purely
visual in `/AP/N`.

### `setAppearanceN`

In `appearance.go`. Mutate-in-place semantics so repeated setter calls don't leak
XObjects:

```go
func setAppearanceN(base *annotationBase, stream *pdfStream) {
    if base.doc == nil {
        return // unbound; deferred until Add
    }
    apDict, _ := base.dict["/AP"].(pdfDict)
    if ref, ok := apDict["/N"].(pdfRef); ok {
        if obj, exists := base.doc.objects[ref.Num]; exists {
            obj.Value = stream  // in-place — same objID, new bytes
            return
        }
    }
    // No existing /AP/N — allocate a new XObject.
    objID := base.doc.nextID
    base.doc.nextID++
    base.doc.objects[objID] = &pdfObject{Num: objID, Value: stream}
    if apDict == nil {
        apDict = pdfDict{}
    }
    apDict["/N"] = pdfRef{Num: objID}
    base.dict["/AP"] = apDict
}
```

`generateXxxAppearance` returns a `*pdfStream` with:
- `/Type` = `/XObject`
- `/Subtype` = `/Form`
- `/BBox` = `[0, 0, width, height]`
- `/Resources` = `<<>>` (empty for Subepic 3 — no fonts, no images)
- `/Length` set by writer
- `Data` = builder bytes
- `Decoded` = true (writer applies FlateDecode)

### Pre-Add behavior

The constructors set `doc: page.doc` and `page: page` immediately, so `regenerateAP`
runs successfully on the first setter call — even before `Add()`. The XObject lives
in `doc.objects` from then on; if the user never calls `Add()`, the XObject becomes
an orphan, cleaned up by `doc.RemoveUnusedObjects()`.

## Property → dict mapping

| Property | PDF dict location | Type |
|---|---|---|
| `Color` | `/C` (stroke color, common) | 3-elem array |
| `InteriorColor` | `/IC` | 3-elem array |
| `BorderWidth` | `/BS/W` (preferred) or `/Border[2]` (legacy fallback on read) | number |
| `BorderStyle` | `/BS/S` | name (`/S`, `/D`, `/B`, `/I`, `/U`) |
| `DashPattern` | `/BS/D` | array of numbers |
| Square/Circle bbox | `/Rect` | 4-elem array |
| Line endpoints | `/L` | 4-elem array `[x1 y1 x2 y2]` |
| Line endings | `/LE` | 2-elem name array `[start-style end-style]` |
| Line `LeaderLineLength` | `/LL` | number |
| Ink strokes | `/InkList` | array of arrays of numbers `[[x1 y1 x2 y2 ...] ...]` |

Read paths handle both the old `/Border` array form and the modern `/BS` dict; write
paths always emit `/BS` (modern) and clear any legacy `/Border` array to avoid
ambiguity.

## Setter regeneration

Every public setter on `Square/Circle/Line/InkAnnotation` follows this pattern:

```go
func (a *SquareAnnotation) SetInteriorColor(c *Color) {
    if c == nil {
        delete(a.dict, "/IC")
    } else {
        a.dict["/IC"] = pdfArray{c.R, c.G, c.B}
    }
    a.regenerateAP()
}

func (a *SquareAnnotation) regenerateAP() {
    setAppearanceN(&a.annotationBase, generateSquareAppearance(a))
}
```

The `RegenerateAppearance()` public method is just `a.regenerateAP()` made callable
externally.

## Error handling

The PDF content stream builder is infallible by design — it formats numbers, never
performs I/O. Generators are likewise infallible (no missing-data branches: defaults
fill in for absent properties).

The four constructors panic on `nil page` (matching existing pattern from
`NewLinkAnnotation`, `NewHighlightAnnotation`, etc.). All other inputs are tolerated
(zero-size rectangle produces an empty appearance; empty Ink strokes array produces
an empty appearance).

`Add` returns the existing errors from `AnnotationCollection.Add` (cross-page
re-attach error). No new error paths.

## Testing strategy

Four levels.

### Level 1: Builder unit tests (`appearance_builder_test.go`)

Golden-byte assertions for each operator. ~25 tests, ~200 lines.

```go
func TestBuilderRect(t *testing.T) {
    b := newAppearanceBuilder()
    b.Rect(10, 20, 100, 50)
    if got := string(b.Bytes()); got != "10 20 100 50 re\n" {
        t.Errorf("got %q", got)
    }
}
```

Coverage: every operator (state, color, path, paint), `Ellipse` helper kappa-correct
Bezier, number formatting (no `5.000000`, no `1e+06`), `SetDashPattern` empty array,
edge cases for very small/large coordinates.

### Level 2: Generator-logic tests (`appearance_test.go`)

Parse the generated content stream back via existing `parseContentStream` (already in
the project from text-extraction), assert operator sequence and operands. ~30 tests,
~250 lines.

Coverage:
- Square: each of 5 border styles × {with-fill, without-fill} = 10 tests.
- Circle: same 10 tests.
- Line: each of 10 ending styles, both as start and as end = ~20 fixture combinations
  collapsed to ~12 tests.
- Ink: 0/1/2/3+ point strokes, multiple strokes, Catmull-Rom math unit tests.
- Catmull-Rom unit tests: fixed input points → expected control points (5 cases).

### Level 3: End-to-end round-trip (`annotation_drawing_test.go`)

Full create → save → reopen → assert. ~10 tests, ~350 lines.

- One round-trip per type (Square/Circle/Line/Ink).
- `TestSetterDrivenRegenerate`: multiple property mutations after `Add` → final `/AP/N`
  reflects the last setter.
- `TestRegenerateAppearanceExplicit`: public method works, replaces `/AP/N`.
- `TestNoXObjectLeak`: 5 sequential setters → `RemoveUnusedObjects()` removes nothing
  (we mutate-in-place).
- `TestUnboundAnnotationGeneratesAP`: setters run before `Add`; XObject lives in
  `doc.objects`; after `Add` it's reachable.
- `TestBeveledRendersTwoColors`: parsed content stream contains two distinct color
  operations (light + dark).
- `TestLineEndingClosedArrowFills`: closed-arrow path ends with `B` (fill+stroke), not
  bare `S`.

### Level 4: External viewer cross-check (manual, in final task)

Same pattern as Subepic 1's pypdf check — generate a doc with all 4 types, verify
`/Subtype` and `/AP/N` via pypdf, optionally inspect visually in
Adobe/SumatraPDF/Foxit/browser. Not a CI gate.

### Test fixtures

No external PDF fixtures needed — all tests build documents from `NewDocument`.
The existing `testdata/PdfWithLinks.pdf` doesn't carry Square/Circle/Line/Ink and
isn't useful here.

## Dependencies / impact on existing code

- `annotation.go`: extend `parseAnnotation` switch with 4 new subtypes; extend
  `AnnotationType` enum with 4 new constants. Roughly +12 lines.
- `CLAUDE.md`, `README.md`: new public API documented (final task in plan).
- No changes to the writer, parser, or any non-annotation subsystem.
- No changes to the existing 7 annotation types (Link/Highlight/Underline/StrikeOut/
  Squiggly/Widget/Generic).

## Risks

1. **Catmull-Rom edge cases.** Phantom-point math at the start/end of an Ink stroke
   has multiple textbook variants (mirror vs. extrapolate vs. duplicate). Tests pin
   the chosen variant (duplicate); other variants would silently change visual
   output. Mitigation: documented in code, tested with golden control-point math.

2. **Beveled/Inset color algorithm.** ISO 32000-1 doesn't precisely specify the light
   and dark colors — only the visual intent. Acrobat uses a 50% blend. We adopt the
   same. Different viewers may render differently for these styles, which is
   acceptable per spec but worth documenting.

3. **Line ending sizing.** ISO 32000-1 doesn't specify the exact ending size in terms
   of line width. Acrobat uses approximately 9× line width, which is the convention
   we adopt. Not a correctness issue but a style choice.

4. **`/AP/N` mutate-in-place vs. copy semantics.** Multiple setters re-use the same
   XObject objID. If a third party retains the `pdfStream` pointer they read from
   `/AP/N`, they'll see updates after subsequent setters. This matches the live-handle
   philosophy used elsewhere in the project but should be documented in the public
   API doc comment for `RegenerateAppearance`.

5. **Coordinate-system off-by-one.** Drawing in `[0, 0, width, height]` local space
   vs. `Rect` page space: the math is straightforward but visual bugs (off-by-one,
   inverted-Y) can slip in. End-to-end tests with manual viewer cross-check (Level 4)
   are the safety net.

## Out of scope (deferred)

- Polygon (`/Polygon`) and PolyLine (`/PolyLine`) subtypes — share `/Vertices`
  geometry with Line/Ink but their own use cases. Future subepic.
- Text-bearing annotations (Text/sticky-note, FreeText) — Subepic 2 of annotations.
- Stamp — Subepic 2.
- File attachments, redaction — Subepic 4.
- `/AP/R` and `/AP/D` — niche, no Aspose-fidelity demand for them yet.
- Transparency/blend modes — same.
- Highlight color customization beyond stroke/interior — patterns/gradients deferred.

## Aspose.PDF for .NET fidelity

API names and shapes mirror Aspose.PDF for .NET:

- `SquareAnnotation`, `CircleAnnotation`, `LineAnnotation`, `InkAnnotation` match the
  .NET class names.
- `BorderStyle` enum values match the .NET `BorderStyle` enumeration members
  (Solid/Dashed/Beveled/Inset/Underline).
- `LineEnding` enum values match the .NET `LineEnding` enumeration.
- `InteriorColor`/`SetInteriorColor` matches the .NET `InteriorColor` property.
- `Strokes`/`SetStrokes` for Ink matches .NET `InkList` semantics.
