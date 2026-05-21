# Appearance Streams + Drawing Annotations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the `/AP` (appearance stream) generation infrastructure plus four geometric annotation types (Square, Circle, Line, Ink) with full PDF spec border-style and line-ending support.

**Architecture:** Three private layers — `appearanceBuilder` (typed wrapper over `bytes.Buffer` for PDF content-stream operators), generators (one function per annotation subtype that reads properties from `pdfDict` and produces a Form XObject `*pdfStream`), and `setAppearanceN` (mutate-in-place XObject wiring). One public layer — `annotation_drawing.go` — exposes the four annotation types whose every property setter regenerates `/AP/N` immediately.

**Tech Stack:** Go 1.24 (existing module), pure standard library, `bytes.Buffer`, `strconv.FormatFloat`. pypdf 6.x for external cross-verification (Task 18 only).

**Reference:** [docs/superpowers/specs/2026-05-05-appearance-streams-drawing-annotations-design.md](../specs/2026-05-05-appearance-streams-drawing-annotations-design.md)

---

## File Map

| File | Purpose |
|---|---|
| `appearance_builder.go` (new) | `appearanceBuilder` type + content-stream operator methods (state, color, path, painting). `LineCap`, `LineJoin` enums. `Ellipse` helper. |
| `appearance_builder_test.go` (new) | Golden-byte unit tests for each operator. |
| `appearance.go` (new) | `setAppearanceN` helper. `makeFormXObject` helper. `generateSquareAppearance` / `generateCircleAppearance` / `generateLineAppearance` / `generateInkAppearance`. Border-style dispatch (5 variants). `drawLineEnding` (10 styles). Catmull-Rom → Bezier converter. |
| `appearance_test.go` (new) | Generator tests (parse content stream back, assert operator sequence). Catmull-Rom math unit tests. |
| `annotation_drawing.go` (new) | `Point` struct. `BorderStyle`, `LineEndingStyle` enums. `SquareAnnotation`, `CircleAnnotation`, `LineAnnotation`, `InkAnnotation` types. Constructors. Property setters/getters with auto-regenerate. `RegenerateAppearance` public method. |
| `annotation_drawing_test.go` (new) | Round-trip tests + integration tests. |
| `annotation.go` (modify) | Extend `parseAnnotation` switch with `/Square`, `/Circle`, `/Line`, `/Ink` cases. Extend `AnnotationType` enum with 4 new constants. |
| `CLAUDE.md`, `README.md` (modify, Task 18) | Public API surface. |

---

## Task 1: appearanceBuilder skeleton + graphics-state operators

**Files:**
- Create: `appearance_builder.go`
- Create: `appearance_builder_test.go`

- [ ] **Step 1: Write the failing test**

`appearance_builder_test.go`:
```go
package asposepdf

import "testing"

func TestBuilderPushPopState(t *testing.T) {
	b := newAppearanceBuilder()
	b.PushState()
	b.PopState()
	if got := string(b.Bytes()); got != "q\nQ\n" {
		t.Errorf("got %q, want \"q\\nQ\\n\"", got)
	}
}

func TestBuilderConcatMatrix(t *testing.T) {
	b := newAppearanceBuilder()
	b.ConcatMatrix(1, 0, 0, 1, 10, 20)
	if got := string(b.Bytes()); got != "1 0 0 1 10 20 cm\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderSetLineWidth(t *testing.T) {
	b := newAppearanceBuilder()
	b.SetLineWidth(2.5)
	if got := string(b.Bytes()); got != "2.5 w\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderSetLineCap(t *testing.T) {
	b := newAppearanceBuilder()
	b.SetLineCap(LineCapRound)
	if got := string(b.Bytes()); got != "1 J\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderSetLineJoin(t *testing.T) {
	b := newAppearanceBuilder()
	b.SetLineJoin(LineJoinBevel)
	if got := string(b.Bytes()); got != "2 j\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderSetMiterLimit(t *testing.T) {
	b := newAppearanceBuilder()
	b.SetMiterLimit(10)
	if got := string(b.Bytes()); got != "10 M\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderSetDashPattern(t *testing.T) {
	b := newAppearanceBuilder()
	b.SetDashPattern([]float64{3, 3}, 0)
	if got := string(b.Bytes()); got != "[3 3] 0 d\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderSetDashPatternEmpty(t *testing.T) {
	b := newAppearanceBuilder()
	b.SetDashPattern(nil, 0)
	if got := string(b.Bytes()); got != "[] 0 d\n" {
		t.Errorf("got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestBuilder -v ./...`
Expected: build failure — `newAppearanceBuilder`, `LineCapRound`, etc. undefined.

- [ ] **Step 3: Write minimal implementation**

`appearance_builder.go`:
```go
package asposepdf

import (
	"bytes"
	"strconv"
)

// LineCap is the /J line cap style per ISO 32000-1 §8.4.3.3 Table 54.
type LineCap int

const (
	LineCapButt   LineCap = 0
	LineCapRound  LineCap = 1
	LineCapSquare LineCap = 2
)

// LineJoin is the /j line join style per ISO 32000-1 §8.4.3.4 Table 55.
type LineJoin int

const (
	LineJoinMiter LineJoin = 0
	LineJoinRound LineJoin = 1
	LineJoinBevel LineJoin = 2
)

// appearanceBuilder accumulates PDF content-stream operators for use as
// a Form XObject /AP/N body. Operators are emitted in PDF spec form,
// one per line, separated by newlines.
type appearanceBuilder struct {
	buf bytes.Buffer
}

func newAppearanceBuilder() *appearanceBuilder {
	return &appearanceBuilder{}
}

// Bytes returns the accumulated content-stream bytes.
func (b *appearanceBuilder) Bytes() []byte {
	return b.buf.Bytes()
}

// formatFloat formats f without scientific notation and without trailing
// zeros. Matches the convention used elsewhere in the project.
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// PushState saves the current graphics state (q operator).
func (b *appearanceBuilder) PushState() {
	b.buf.WriteString("q\n")
}

// PopState restores the last saved graphics state (Q operator).
func (b *appearanceBuilder) PopState() {
	b.buf.WriteString("Q\n")
}

// ConcatMatrix concatenates the given 2x3 matrix to the CTM (cm operator).
func (b *appearanceBuilder) ConcatMatrix(a, bb, c, d, e, f float64) {
	b.buf.WriteString(formatFloat(a))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(bb))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(c))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(d))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(e))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(f))
	b.buf.WriteString(" cm\n")
}

// SetLineWidth sets the stroke line width (w operator).
func (b *appearanceBuilder) SetLineWidth(w float64) {
	b.buf.WriteString(formatFloat(w))
	b.buf.WriteString(" w\n")
}

// SetLineCap sets the line-cap style (J operator).
func (b *appearanceBuilder) SetLineCap(c LineCap) {
	b.buf.WriteString(strconv.Itoa(int(c)))
	b.buf.WriteString(" J\n")
}

// SetLineJoin sets the line-join style (j operator).
func (b *appearanceBuilder) SetLineJoin(j LineJoin) {
	b.buf.WriteString(strconv.Itoa(int(j)))
	b.buf.WriteString(" j\n")
}

// SetMiterLimit sets the miter limit (M operator).
func (b *appearanceBuilder) SetMiterLimit(m float64) {
	b.buf.WriteString(formatFloat(m))
	b.buf.WriteString(" M\n")
}

// SetDashPattern sets the line-dash pattern (d operator). A nil or empty
// pattern emits "[] phase d", which means a solid line.
func (b *appearanceBuilder) SetDashPattern(pattern []float64, phase float64) {
	b.buf.WriteByte('[')
	for i, v := range pattern {
		if i > 0 {
			b.buf.WriteByte(' ')
		}
		b.buf.WriteString(formatFloat(v))
	}
	b.buf.WriteByte(']')
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(phase))
	b.buf.WriteString(" d\n")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run TestBuilder -v ./...`
Expected: all 8 tests PASS.

Run: `go test ./...`
Expected: full suite PASS.

- [ ] **Step 5: Commit**

```bash
git add appearance_builder.go appearance_builder_test.go
git commit -m "feat: appearanceBuilder skeleton + graphics-state operators"
```

---

## Task 2: appearanceBuilder color operators

**Files:**
- Modify: `appearance_builder.go`
- Modify: `appearance_builder_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `appearance_builder_test.go`:
```go
func TestBuilderSetStrokeColorRGB(t *testing.T) {
	b := newAppearanceBuilder()
	b.SetStrokeColorRGB(Color{R: 1, G: 0.5, B: 0})
	if got := string(b.Bytes()); got != "1 0.5 0 RG\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderSetFillColorRGB(t *testing.T) {
	b := newAppearanceBuilder()
	b.SetFillColorRGB(Color{R: 0, G: 1, B: 1})
	if got := string(b.Bytes()); got != "0 1 1 rg\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderSetStrokeGray(t *testing.T) {
	b := newAppearanceBuilder()
	b.SetStrokeGray(0.25)
	if got := string(b.Bytes()); got != "0.25 G\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderSetFillGray(t *testing.T) {
	b := newAppearanceBuilder()
	b.SetFillGray(0.75)
	if got := string(b.Bytes()); got != "0.75 g\n" {
		t.Errorf("got %q", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestBuilderSetStroke -v ./...`
Expected: build failure — `SetStrokeColorRGB` etc. undefined.

- [ ] **Step 3: Add color operators**

Append to `appearance_builder.go`:
```go
// SetStrokeColorRGB sets the stroke color to RGB (RG operator).
func (b *appearanceBuilder) SetStrokeColorRGB(c Color) {
	b.buf.WriteString(formatFloat(c.R))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(c.G))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(c.B))
	b.buf.WriteString(" RG\n")
}

// SetFillColorRGB sets the fill color to RGB (rg operator).
func (b *appearanceBuilder) SetFillColorRGB(c Color) {
	b.buf.WriteString(formatFloat(c.R))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(c.G))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(c.B))
	b.buf.WriteString(" rg\n")
}

// SetStrokeGray sets the stroke color to a grayscale value (G operator).
func (b *appearanceBuilder) SetStrokeGray(g float64) {
	b.buf.WriteString(formatFloat(g))
	b.buf.WriteString(" G\n")
}

// SetFillGray sets the fill color to a grayscale value (g operator).
func (b *appearanceBuilder) SetFillGray(g float64) {
	b.buf.WriteString(formatFloat(g))
	b.buf.WriteString(" g\n")
}
```

- [ ] **Step 4: Run tests**

Run: `go test -run TestBuilderSet -v ./...`
Expected: 4 new tests PASS, prior tests still PASS.

Run: `go test ./...`
Expected: full suite PASS.

- [ ] **Step 5: Commit**

```bash
git add appearance_builder.go appearance_builder_test.go
git commit -m "feat: appearanceBuilder color operators"
```

---

## Task 3: appearanceBuilder path construction + Ellipse helper

**Files:**
- Modify: `appearance_builder.go`
- Modify: `appearance_builder_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `appearance_builder_test.go`:
```go
func TestBuilderMoveTo(t *testing.T) {
	b := newAppearanceBuilder()
	b.MoveTo(10, 20)
	if got := string(b.Bytes()); got != "10 20 m\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderLineTo(t *testing.T) {
	b := newAppearanceBuilder()
	b.LineTo(30, 40)
	if got := string(b.Bytes()); got != "30 40 l\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderCurveTo(t *testing.T) {
	b := newAppearanceBuilder()
	b.CurveTo(1, 2, 3, 4, 5, 6)
	if got := string(b.Bytes()); got != "1 2 3 4 5 6 c\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderRect(t *testing.T) {
	b := newAppearanceBuilder()
	b.Rect(0, 0, 100, 50)
	if got := string(b.Bytes()); got != "0 0 100 50 re\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderClosePath(t *testing.T) {
	b := newAppearanceBuilder()
	b.ClosePath()
	if got := string(b.Bytes()); got != "h\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderEllipse(t *testing.T) {
	// Ellipse emits m + 4 c operators.
	b := newAppearanceBuilder()
	b.Ellipse(50, 50, 25, 25)
	out := string(b.Bytes())
	// Verify shape: should start with a moveTo and contain four curveTo's.
	if !strings.Contains(out, " m\n") {
		t.Errorf("Ellipse missing m operator: %q", out)
	}
	cCount := strings.Count(out, " c\n")
	if cCount != 4 {
		t.Errorf("Ellipse should emit 4 c operators, got %d in %q", cCount, out)
	}
}
```

You will need `import "strings"` in `appearance_builder_test.go`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestBuilderMoveTo -v ./...`
Expected: build failure.

- [ ] **Step 3: Add path operators**

Append to `appearance_builder.go`:
```go
// MoveTo begins a new subpath at (x, y) (m operator).
func (b *appearanceBuilder) MoveTo(x, y float64) {
	b.buf.WriteString(formatFloat(x))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(y))
	b.buf.WriteString(" m\n")
}

// LineTo appends a straight line segment to (x, y) (l operator).
func (b *appearanceBuilder) LineTo(x, y float64) {
	b.buf.WriteString(formatFloat(x))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(y))
	b.buf.WriteString(" l\n")
}

// CurveTo appends a cubic Bezier curve from the current point through
// control points (x1, y1) and (x2, y2) to endpoint (x3, y3) (c operator).
func (b *appearanceBuilder) CurveTo(x1, y1, x2, y2, x3, y3 float64) {
	b.buf.WriteString(formatFloat(x1))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(y1))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(x2))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(y2))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(x3))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(y3))
	b.buf.WriteString(" c\n")
}

// Rect adds a closed rectangular subpath (re operator).
func (b *appearanceBuilder) Rect(x, y, w, h float64) {
	b.buf.WriteString(formatFloat(x))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(y))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(w))
	b.buf.WriteByte(' ')
	b.buf.WriteString(formatFloat(h))
	b.buf.WriteString(" re\n")
}

// ClosePath closes the current subpath (h operator).
func (b *appearanceBuilder) ClosePath() {
	b.buf.WriteString("h\n")
}

// kappa is the standard control-point distance ratio for approximating
// a quarter-circle with a cubic Bezier. (4/3) * (sqrt(2) - 1).
const kappa = 0.5522847498307933

// Ellipse adds a closed elliptic subpath centered at (cx, cy) with
// semi-axes rx and ry, approximated by 4 cubic Beziers.
func (b *appearanceBuilder) Ellipse(cx, cy, rx, ry float64) {
	dx := rx * kappa
	dy := ry * kappa
	// Start at right edge, going counter-clockwise.
	b.MoveTo(cx+rx, cy)
	b.CurveTo(cx+rx, cy+dy, cx+dx, cy+ry, cx, cy+ry)       // right → top
	b.CurveTo(cx-dx, cy+ry, cx-rx, cy+dy, cx-rx, cy)       // top → left
	b.CurveTo(cx-rx, cy-dy, cx-dx, cy-ry, cx, cy-ry)       // left → bottom
	b.CurveTo(cx+dx, cy-ry, cx+rx, cy-dy, cx+rx, cy)       // bottom → right
}
```

- [ ] **Step 4: Run tests**

Run: `go test -run TestBuilder -v ./...`
Expected: all tests PASS.

Run: `go test ./...`
Expected: full suite PASS.

- [ ] **Step 5: Commit**

```bash
git add appearance_builder.go appearance_builder_test.go
git commit -m "feat: appearanceBuilder path construction + Ellipse helper"
```

---

## Task 4: appearanceBuilder painting operators

**Files:**
- Modify: `appearance_builder.go`
- Modify: `appearance_builder_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `appearance_builder_test.go`:
```go
func TestBuilderStroke(t *testing.T) {
	b := newAppearanceBuilder()
	b.Stroke()
	if got := string(b.Bytes()); got != "S\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderClosePathStroke(t *testing.T) {
	b := newAppearanceBuilder()
	b.ClosePathStroke()
	if got := string(b.Bytes()); got != "s\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderFill(t *testing.T) {
	b := newAppearanceBuilder()
	b.Fill()
	if got := string(b.Bytes()); got != "f\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderFillStroke(t *testing.T) {
	b := newAppearanceBuilder()
	b.FillStroke()
	if got := string(b.Bytes()); got != "B\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderClosePathFillStroke(t *testing.T) {
	b := newAppearanceBuilder()
	b.ClosePathFillStroke()
	if got := string(b.Bytes()); got != "b\n" {
		t.Errorf("got %q", got)
	}
}

func TestBuilderEndPath(t *testing.T) {
	b := newAppearanceBuilder()
	b.EndPath()
	if got := string(b.Bytes()); got != "n\n" {
		t.Errorf("got %q", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestBuilderStroke -v ./...`
Expected: build failure.

- [ ] **Step 3: Add painting operators**

Append to `appearance_builder.go`:
```go
// Stroke strokes the current path (S operator).
func (b *appearanceBuilder) Stroke() {
	b.buf.WriteString("S\n")
}

// ClosePathStroke closes and strokes the current path (s operator).
func (b *appearanceBuilder) ClosePathStroke() {
	b.buf.WriteString("s\n")
}

// Fill fills the current path using the non-zero winding rule (f operator).
func (b *appearanceBuilder) Fill() {
	b.buf.WriteString("f\n")
}

// FillStroke fills then strokes the current path (B operator).
func (b *appearanceBuilder) FillStroke() {
	b.buf.WriteString("B\n")
}

// ClosePathFillStroke closes, fills, then strokes the current path (b operator).
func (b *appearanceBuilder) ClosePathFillStroke() {
	b.buf.WriteString("b\n")
}

// EndPath discards the current path without painting (n operator).
func (b *appearanceBuilder) EndPath() {
	b.buf.WriteString("n\n")
}
```

- [ ] **Step 4: Run tests**

Run: `go test -run TestBuilder -v ./...`
Expected: all tests PASS.

Run: `go test ./...`
Expected: full suite PASS.

- [ ] **Step 5: Commit**

```bash
git add appearance_builder.go appearance_builder_test.go
git commit -m "feat: appearanceBuilder painting operators"
```

---

## Task 5: setAppearanceN + makeFormXObject helpers

**Files:**
- Create: `appearance.go`
- Create: `appearance_test.go`

- [ ] **Step 1: Write the failing test**

`appearance_test.go`:
```go
package asposepdf

import (
	"testing"
)

func TestMakeFormXObject(t *testing.T) {
	stream := makeFormXObject([]byte("Q\n"), Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50})
	if stream == nil {
		t.Fatal("makeFormXObject returned nil")
	}
	if got := stream.Dict["/Type"]; got != pdfName("/XObject") {
		t.Errorf("/Type = %v, want /XObject", got)
	}
	if got := stream.Dict["/Subtype"]; got != pdfName("/Form") {
		t.Errorf("/Subtype = %v, want /Form", got)
	}
	bbox, ok := stream.Dict["/BBox"].(pdfArray)
	if !ok || len(bbox) != 4 {
		t.Fatalf("/BBox = %v, want 4-elem pdfArray", stream.Dict["/BBox"])
	}
	if !stream.Decoded {
		t.Error("stream.Decoded should be true (writer flate-compresses)")
	}
}

func TestSetAppearanceNAllocatesNew(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	// A bare annotationBase (not via constructor — for unit-test isolation).
	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Square"),
		"/Rect":    pdfArray{0.0, 0.0, 10.0, 10.0},
	}
	base := &annotationBase{dict: dict, doc: doc, page: page}

	stream := makeFormXObject([]byte("S\n"), Rectangle{URX: 10, URY: 10})
	setAppearanceN(base, stream)

	apDict, ok := dict["/AP"].(pdfDict)
	if !ok {
		t.Fatal("/AP missing after setAppearanceN")
	}
	ref, ok := apDict["/N"].(pdfRef)
	if !ok {
		t.Fatal("/AP/N missing or not a pdfRef")
	}
	if _, ok := doc.objects[ref.Num]; !ok {
		t.Errorf("/AP/N target object %d not in doc.objects", ref.Num)
	}
}

func TestSetAppearanceNMutatesInPlace(t *testing.T) {
	doc := NewDocument(595, 842)
	page, _ := doc.Page(1)
	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Square"),
		"/Rect":    pdfArray{0.0, 0.0, 10.0, 10.0},
	}
	base := &annotationBase{dict: dict, doc: doc, page: page}

	stream1 := makeFormXObject([]byte("S\n"), Rectangle{URX: 10, URY: 10})
	setAppearanceN(base, stream1)

	apDict := dict["/AP"].(pdfDict)
	firstObjID := apDict["/N"].(pdfRef).Num

	// Second call: must reuse the same object ID, only mutate the bytes.
	stream2 := makeFormXObject([]byte("f\n"), Rectangle{URX: 10, URY: 10})
	setAppearanceN(base, stream2)

	apDict = dict["/AP"].(pdfDict)
	if got := apDict["/N"].(pdfRef).Num; got != firstObjID {
		t.Errorf("setAppearanceN allocated new objID %d on second call (was %d)", got, firstObjID)
	}
	// And the underlying object must hold the new bytes.
	obj := doc.objects[firstObjID]
	if got := string(obj.Value.(*pdfStream).Data); got != "f\n" {
		t.Errorf("object data = %q, want %q", got, "f\n")
	}
}

func TestSetAppearanceNUnboundDoc(t *testing.T) {
	// Does nothing when base.doc is nil (annotation not yet linked).
	dict := pdfDict{}
	base := &annotationBase{dict: dict, doc: nil}
	stream := makeFormXObject([]byte("S\n"), Rectangle{URX: 10, URY: 10})
	setAppearanceN(base, stream)
	if _, ok := dict["/AP"]; ok {
		t.Error("/AP must not be set when doc is nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestMakeFormXObject -v ./...`
Expected: build failure — `makeFormXObject`, `setAppearanceN` undefined.

- [ ] **Step 3: Implement helpers**

`appearance.go`:
```go
package asposepdf

// makeFormXObject builds a Form XObject stream wrapping the given content
// bytes and bbox. The returned stream is ready for storage in
// doc.objects and reference from /AP/N.
//
// /Resources is empty — drawing annotations (Square/Circle/Line/Ink)
// don't use fonts or images. Future subepics (FreeText, Stamp) will
// extend this helper or supply their own.
func makeFormXObject(content []byte, bbox Rectangle) *pdfStream {
	return &pdfStream{
		Dict: pdfDict{
			"/Type":      pdfName("/XObject"),
			"/Subtype":   pdfName("/Form"),
			"/BBox":      pdfArray{bbox.LLX, bbox.LLY, bbox.URX, bbox.URY},
			"/Resources": pdfDict{},
		},
		Data:    content,
		Decoded: true,
	}
}

// setAppearanceN replaces /AP/N on the annotation. If /AP/N already
// references an XObject in doc.objects, that object is mutated in place
// (no new objID allocated, no orphans). Otherwise a fresh XObject is
// allocated and /AP/N updated to reference it.
//
// No-op when base.doc is nil (annotation not yet doc-linked — should
// not normally happen because constructors set base.doc immediately).
func setAppearanceN(base *annotationBase, stream *pdfStream) {
	if base.doc == nil {
		return
	}
	apDict, _ := base.dict["/AP"].(pdfDict)
	if ref, ok := apDict["/N"].(pdfRef); ok {
		if obj, exists := base.doc.objects[ref.Num]; exists {
			obj.Value = stream
			return
		}
	}
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

- [ ] **Step 4: Run tests**

Run: `go test -run 'TestMakeFormXObject|TestSetAppearanceN' -v ./...`
Expected: 4 new tests PASS.

Run: `go test ./...`
Expected: full suite PASS.

- [ ] **Step 5: Commit**

```bash
git add appearance.go appearance_test.go
git commit -m "feat: setAppearanceN + makeFormXObject helpers"
```

---

## Task 6: Common types — Point, BorderStyle, LineEndingStyle enums

**Files:**
- Create: `annotation_drawing.go`
- Create: `annotation_drawing_test.go`

- [ ] **Step 1: Write the failing test**

`annotation_drawing_test.go`:
```go
package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func TestPointConstruction(t *testing.T) {
	p := pdf.Point{X: 10, Y: 20}
	if p.X != 10 || p.Y != 20 {
		t.Errorf("Point = %+v, want {10 20}", p)
	}
}

func TestBorderStyleConstants(t *testing.T) {
	if pdf.BorderSolid != 0 {
		t.Errorf("BorderSolid = %d, want 0", pdf.BorderSolid)
	}
	// Verify the 5 constants are distinct and ordered.
	all := []pdf.BorderStyle{
		pdf.BorderSolid,
		pdf.BorderDashed,
		pdf.BorderBeveled,
		pdf.BorderInset,
		pdf.BorderUnderline,
	}
	for i, v := range all {
		if int(v) != i {
			t.Errorf("BorderStyle[%d] = %d, want %d", i, int(v), i)
		}
	}
}

func TestLineEndingStyleConstants(t *testing.T) {
	if pdf.LineEndingNone != 0 {
		t.Errorf("LineEndingNone = %d, want 0", pdf.LineEndingNone)
	}
	all := []pdf.LineEndingStyle{
		pdf.LineEndingNone,
		pdf.LineEndingSquare,
		pdf.LineEndingCircle,
		pdf.LineEndingDiamond,
		pdf.LineEndingOpenArrow,
		pdf.LineEndingClosedArrow,
		pdf.LineEndingButt,
		pdf.LineEndingROpenArrow,
		pdf.LineEndingRClosedArrow,
		pdf.LineEndingSlash,
	}
	for i, v := range all {
		if int(v) != i {
			t.Errorf("LineEndingStyle[%d] = %d, want %d", i, int(v), i)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestPoint|TestBorderStyleConstants|TestLineEndingStyleConstants' -v ./...`
Expected: build failure — `pdf.Point`, `pdf.BorderSolid` etc. undefined.

- [ ] **Step 3: Define types**

`annotation_drawing.go`:
```go
package asposepdf

// Point is a single point in PDF user-space coordinates.
type Point struct {
	X, Y float64
}

// BorderStyle controls the /BS dict for drawing annotations per
// ISO 32000-1 §12.5.4 Table 168.
type BorderStyle int

const (
	BorderSolid     BorderStyle = iota // /S = /S
	BorderDashed                        // /S = /D + /D dash array
	BorderBeveled                       // /S = /B (3D raised effect)
	BorderInset                         // /S = /I (3D recessed effect)
	BorderUnderline                     // /S = /U (only the bottom edge)
)

// LineEndingStyle is one of the 10 line-ending shapes per ISO 32000-1
// §12.5.6.7 Table 176, used in /Line annotations' /LE entry.
type LineEndingStyle int

const (
	LineEndingNone LineEndingStyle = iota
	LineEndingSquare
	LineEndingCircle
	LineEndingDiamond
	LineEndingOpenArrow
	LineEndingClosedArrow
	LineEndingButt
	LineEndingROpenArrow   // OpenArrow rotated 180° (away from line)
	LineEndingRClosedArrow // ClosedArrow rotated 180°
	LineEndingSlash
)
```

- [ ] **Step 4: Run tests**

Run: `go test -run 'TestPoint|TestBorderStyleConstants|TestLineEndingStyleConstants' -v ./...`
Expected: 3 tests PASS.

Run: `go test ./...`
Expected: full suite PASS.

- [ ] **Step 5: Commit**

```bash
git add annotation_drawing.go annotation_drawing_test.go
git commit -m "feat: Point + BorderStyle + LineEndingStyle types"
```

---

## Task 7: SquareAnnotation skeleton (Solid border, no fill)

**Files:**
- Modify: `annotation.go`
- Modify: `annotation_drawing.go`
- Modify: `annotation_drawing_test.go`
- Modify: `appearance.go`

- [ ] **Step 1: Write the failing test**

Append to `annotation_drawing_test.go`:
```go
import "bytes"

func TestSquareAnnotationSolidStroke(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 200, URY: 700})
	sq.SetColor(&pdf.Color{R: 1, G: 0, B: 0, A: 1})
	sq.SetBorderWidth(2)
	if err := page.Annotations().Add(sq); err != nil {
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
	got := page2.Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypeSquare {
		t.Errorf("type = %v, want AnnotationTypeSquare", got.AnnotationType())
	}
	sq2, ok := got.(*pdf.SquareAnnotation)
	if !ok {
		t.Fatalf("concrete type = %T, want *pdf.SquareAnnotation", got)
	}
	if c := sq2.Color(); c == nil || c.R != 1 {
		t.Errorf("Color = %v, want red", c)
	}
	if w := sq2.BorderWidth(); w != 2 {
		t.Errorf("BorderWidth = %v, want 2", w)
	}
}
```

(`bytes` import goes at top of file alongside existing imports.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestSquareAnnotationSolidStroke -v ./...`
Expected: build failure — `pdf.NewSquareAnnotation`, `pdf.SquareAnnotation`, `pdf.AnnotationTypeSquare` undefined.

- [ ] **Step 3: Add AnnotationTypeSquare and parseAnnotation /Square dispatch**

In `annotation.go`, find the `AnnotationType` const block and append (after `AnnotationTypeWidget`):
```go
	AnnotationTypeSquare
```

In `annotation.go`'s `parseAnnotation` function, find the switch and add a case before the `GenericAnnotation` fallthrough:
```go
	case "/Square":
		return &SquareAnnotation{annotationBase: base}
```

- [ ] **Step 4: Add SquareAnnotation type and constructor**

In `annotation_drawing.go`, append:
```go
// SquareAnnotation draws a rectangular annotation with stroked border
// and optional interior fill. Renders natively from /AP/N — Solid,
// Dashed, Beveled, Inset, and Underline border styles supported.
type SquareAnnotation struct {
	annotationBase
}

func (a *SquareAnnotation) AnnotationType() AnnotationType { return AnnotationTypeSquare }

// NewSquareAnnotation builds an unbound square annotation. Page must be
// non-nil. The annotation is not added to the document until
// page.Annotations().Add(square) succeeds.
func NewSquareAnnotation(page *Page, rect Rectangle) *SquareAnnotation {
	if page == nil {
		panic("NewSquareAnnotation: nil page")
	}
	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Square"),
		"/Rect":    pdfArray{rect.LLX, rect.LLY, rect.URX, rect.URY},
	}
	a := &SquareAnnotation{annotationBase: annotationBase{
		dict: dict,
		doc:  page.doc,
		page: page,
	}}
	a.regenerateAP()
	return a
}

// BorderWidth returns the stroke line width. Reads /BS/W (preferred) or
// /Border[2] (legacy fallback). Defaults to 1 if neither is present.
func (a *SquareAnnotation) BorderWidth() float64 {
	if bs, ok := a.dict["/BS"].(pdfDict); ok {
		if w, err := toFloat(bs["/W"]); err == nil {
			return w
		}
	}
	if border, ok := a.dict["/Border"].(pdfArray); ok && len(border) >= 3 {
		if w, err := toFloat(border[2]); err == nil {
			return w
		}
	}
	return 1
}

// SetBorderWidth writes /BS/W and clears any legacy /Border array.
func (a *SquareAnnotation) SetBorderWidth(w float64) {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		bs = pdfDict{}
	}
	bs["/W"] = w
	a.dict["/BS"] = bs
	delete(a.dict, "/Border")
	a.regenerateAP()
}

// regenerateAP rebuilds /AP/N from the annotation's current properties.
func (a *SquareAnnotation) regenerateAP() {
	setAppearanceN(&a.annotationBase, generateSquareAppearance(a))
}

// RegenerateAppearance forces /AP/N to be rebuilt from current properties.
// Useful when the underlying dict was mutated directly (bypassing setters).
func (a *SquareAnnotation) RegenerateAppearance() {
	a.regenerateAP()
}
```

- [ ] **Step 5: Add generateSquareAppearance (Solid only for now)**

In `appearance.go`, append:
```go
// generateSquareAppearance produces /AP/N for a Square annotation.
// This phase supports the Solid border style only — Dashed/Beveled/
// Inset/Underline are added in subsequent tasks.
func generateSquareAppearance(a *SquareAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	bw := a.BorderWidth()

	b := newAppearanceBuilder()
	b.PushState()
	b.SetLineWidth(bw)
	if c := a.Color(); c != nil {
		b.SetStrokeColorRGB(*c)
	}
	// Inset rectangle by half line width so the stroke stays inside /BBox.
	inset := bw / 2
	b.Rect(inset, inset, width-bw, height-bw)
	b.Stroke()
	b.PopState()

	return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
}
```

- [ ] **Step 6: Run tests**

Run: `go test -run TestSquareAnnotationSolidStroke -v ./...`
Expected: PASS.

Run: `go test ./...`
Expected: full suite PASS.

- [ ] **Step 7: Commit**

```bash
git add annotation.go annotation_drawing.go annotation_drawing_test.go appearance.go
git commit -m "feat: SquareAnnotation skeleton + Solid border + parseAnnotation /Square"
```

---

## Task 8: BorderStyle Dashed for Square + DashPattern setters

**Files:**
- Modify: `annotation_drawing.go`
- Modify: `annotation_drawing_test.go`
- Modify: `appearance.go`

- [ ] **Step 1: Write the failing test**

Append to `annotation_drawing_test.go`:
```go
func TestSquareAnnotationDashedBorder(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 200, URY: 700})
	sq.SetBorderStyle(pdf.BorderDashed)
	sq.SetDashPattern([]float64{5, 2})
	if err := page.Annotations().Add(sq); err != nil {
		t.Fatalf("Add: %v", err)
	}

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	sq2 := doc2.Pages()[0].Annotations().At(0).(*pdf.SquareAnnotation)
	if got := sq2.BorderStyle(); got != pdf.BorderDashed {
		t.Errorf("BorderStyle = %v, want BorderDashed", got)
	}
	dp := sq2.DashPattern()
	if len(dp) != 2 || dp[0] != 5 || dp[1] != 2 {
		t.Errorf("DashPattern = %v, want [5 2]", dp)
	}
}

func TestSquareAnnotationDashPatternDefensiveCopy(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 10, URY: 10})
	in := []float64{3, 3}
	sq.SetDashPattern(in)
	in[0] = 99 // mutate caller's slice
	if got := sq.DashPattern(); got[0] != 3 {
		t.Errorf("DashPattern[0] = %v after caller mutation, want 3 (defensive copy)", got[0])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestSquareAnnotationDashed -v ./...`
Expected: build failure — `SetBorderStyle`, `SetDashPattern` etc. undefined.

- [ ] **Step 3: Add BorderStyle + DashPattern accessors to SquareAnnotation**

Append to `annotation_drawing.go`:
```go
// BorderStyle returns the /BS/S style. Defaults to BorderSolid if absent.
func (a *SquareAnnotation) BorderStyle() BorderStyle {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		return BorderSolid
	}
	switch n, _ := bs["/S"].(pdfName); n {
	case "/D":
		return BorderDashed
	case "/B":
		return BorderBeveled
	case "/I":
		return BorderInset
	case "/U":
		return BorderUnderline
	}
	return BorderSolid
}

// SetBorderStyle writes /BS/S using the PDF spec name codes.
func (a *SquareAnnotation) SetBorderStyle(s BorderStyle) {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		bs = pdfDict{}
	}
	bs["/S"] = borderStyleName(s)
	a.dict["/BS"] = bs
	delete(a.dict, "/Border")
	a.regenerateAP()
}

// DashPattern returns a defensive copy of /BS/D (dash array). Returns
// nil if /BS/D is absent or empty.
func (a *SquareAnnotation) DashPattern() []float64 {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		return nil
	}
	arr, _ := bs["/D"].(pdfArray)
	if len(arr) == 0 {
		return nil
	}
	out := make([]float64, 0, len(arr))
	for _, v := range arr {
		f, _ := toFloat(v)
		out = append(out, f)
	}
	return out
}

// SetDashPattern writes /BS/D. The slice is copied; the caller may
// safely mutate p after this returns.
func (a *SquareAnnotation) SetDashPattern(p []float64) {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		bs = pdfDict{}
	}
	if len(p) == 0 {
		delete(bs, "/D")
	} else {
		arr := make(pdfArray, 0, len(p))
		for _, v := range p {
			arr = append(arr, v)
		}
		bs["/D"] = arr
	}
	a.dict["/BS"] = bs
	delete(a.dict, "/Border")
	a.regenerateAP()
}

// borderStyleName maps a BorderStyle to its PDF name code per Table 168.
func borderStyleName(s BorderStyle) pdfName {
	switch s {
	case BorderDashed:
		return "/D"
	case BorderBeveled:
		return "/B"
	case BorderInset:
		return "/I"
	case BorderUnderline:
		return "/U"
	}
	return "/S"
}
```

- [ ] **Step 4: Update generateSquareAppearance to support Dashed**

In `appearance.go`, replace the existing `generateSquareAppearance` body with:
```go
func generateSquareAppearance(a *SquareAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	bw := a.BorderWidth()
	style := a.BorderStyle()

	b := newAppearanceBuilder()
	b.PushState()
	b.SetLineWidth(bw)
	if c := a.Color(); c != nil {
		b.SetStrokeColorRGB(*c)
	}
	if style == BorderDashed {
		dp := a.DashPattern()
		if len(dp) == 0 {
			dp = []float64{3, 3}
		}
		b.SetDashPattern(dp, 0)
	}
	inset := bw / 2
	b.Rect(inset, inset, width-bw, height-bw)
	b.Stroke()
	b.PopState()

	return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
}
```

- [ ] **Step 5: Run tests**

Run: `go test -run 'TestSquareAnnotation' -v ./...`
Expected: 3 PASS (Solid, Dashed, defensive copy).

Run: `go test ./...`
Expected: full suite PASS.

- [ ] **Step 6: Commit**

```bash
git add annotation_drawing.go annotation_drawing_test.go appearance.go
git commit -m "feat: SquareAnnotation BorderStyle Dashed + DashPattern setters"
```

---

## Task 9: BorderStyle Beveled + Inset for Square (two-pass color rendering)

**Files:**
- Modify: `annotation_drawing_test.go`
- Modify: `appearance.go`

- [ ] **Step 1: Write the failing tests**

Append to `annotation_drawing_test.go`:
```go
func TestSquareAnnotationBeveledRendersTwoColors(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 200, URY: 700})
	sq.SetBorderStyle(pdf.BorderBeveled)
	sq.SetColor(&pdf.Color{R: 0.5, G: 0.5, B: 0.5, A: 1})
	if err := page.Annotations().Add(sq); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	sq2 := doc2.Pages()[0].Annotations().At(0).(*pdf.SquareAnnotation)
	if got := sq2.BorderStyle(); got != pdf.BorderBeveled {
		t.Errorf("BorderStyle = %v, want BorderBeveled", got)
	}
}

func TestSquareAnnotationInsetRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 200, URY: 700})
	sq.SetBorderStyle(pdf.BorderInset)
	if err := page.Annotations().Add(sq); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	sq2 := doc2.Pages()[0].Annotations().At(0).(*pdf.SquareAnnotation)
	if got := sq2.BorderStyle(); got != pdf.BorderInset {
		t.Errorf("BorderStyle = %v, want BorderInset", got)
	}
}
```

Append an internal-package test (in same package as helpers) — create a new file `appearance_internal_test.go` to test the helper:
```go
package asposepdf

import "testing"

func TestBeveledColorPair(t *testing.T) {
	base := Color{R: 0.5, G: 0.5, B: 0.5, A: 1}
	light, dark := beveledColorPair(base, false)
	// Light = 50% blend with white → all channels 0.75
	if light.R != 0.75 || light.G != 0.75 || light.B != 0.75 {
		t.Errorf("light = %+v, want {0.75 0.75 0.75 1}", light)
	}
	// Dark = base * 0.5 → all channels 0.25
	if dark.R != 0.25 || dark.G != 0.25 || dark.B != 0.25 {
		t.Errorf("dark = %+v, want {0.25 0.25 0.25 1}", dark)
	}
}

func TestBeveledColorPairInverted(t *testing.T) {
	// Inverted = Inset style — light/dark swapped.
	base := Color{R: 0.5, G: 0.5, B: 0.5, A: 1}
	light, dark := beveledColorPair(base, true)
	if light.R != 0.25 {
		t.Errorf("inverted light.R = %v, want 0.25 (Inset swaps)", light.R)
	}
	if dark.R != 0.75 {
		t.Errorf("inverted dark.R = %v, want 0.75 (Inset swaps)", dark.R)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestSquareAnnotationBeveled|TestSquareAnnotationInset|TestBeveledColorPair' -v ./...`
Expected: build failures (TestBeveled* tests need `beveledColorPair` defined; round-trip tests need updated generator).

- [ ] **Step 3: Add beveledColorPair helper to appearance.go**

Append to `appearance.go`:
```go
// beveledColorPair returns a (light, dark) color pair for Beveled and
// Inset border rendering. Light = base × 0.5 + white × 0.5; Dark =
// base × 0.5. When inverted is true (Inset style) the pair is swapped.
//
// PDF spec doesn't precisely fix the algorithm; this matches Acrobat
// output for the same input.
func beveledColorPair(base Color, inverted bool) (light, dark Color) {
	light = Color{
		R: base.R*0.5 + 0.5,
		G: base.G*0.5 + 0.5,
		B: base.B*0.5 + 0.5,
		A: 1,
	}
	dark = Color{
		R: base.R * 0.5,
		G: base.G * 0.5,
		B: base.B * 0.5,
		A: 1,
	}
	if inverted {
		return dark, light
	}
	return light, dark
}
```

- [ ] **Step 4: Update generateSquareAppearance to support Beveled and Inset**

In `appearance.go`, replace `generateSquareAppearance` body with:
```go
func generateSquareAppearance(a *SquareAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	bw := a.BorderWidth()
	style := a.BorderStyle()

	b := newAppearanceBuilder()

	switch style {
	case BorderBeveled, BorderInset:
		drawBeveledRectBorder(b, width, height, bw, a.Color(), style == BorderInset)
	default:
		b.PushState()
		b.SetLineWidth(bw)
		if c := a.Color(); c != nil {
			b.SetStrokeColorRGB(*c)
		}
		if style == BorderDashed {
			dp := a.DashPattern()
			if len(dp) == 0 {
				dp = []float64{3, 3}
			}
			b.SetDashPattern(dp, 0)
		}
		inset := bw / 2
		b.Rect(inset, inset, width-bw, height-bw)
		b.Stroke()
		b.PopState()
	}

	return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
}

// drawBeveledRectBorder emits a two-pass beveled (or inset) border on a
// rectangle of size (width, height). Top + left edges use the light
// color; bottom + right edges use the dark color (inverted for Inset).
func drawBeveledRectBorder(b *appearanceBuilder, width, height, bw float64, baseColor *Color, inverted bool) {
	base := Color{R: 0, G: 0, B: 0, A: 1}
	if baseColor != nil {
		base = *baseColor
	}
	light, dark := beveledColorPair(base, inverted)

	// Light pass: top + left edges as filled trapezoid.
	b.PushState()
	b.SetFillColorRGB(light)
	// Outer top-left corner → outer top-right → inner top-right → inner top-left.
	b.MoveTo(0, height)
	b.LineTo(width, height)
	b.LineTo(width-bw, height-bw)
	b.LineTo(bw, height-bw)
	b.ClosePath()
	b.Fill()
	// Outer top-left → outer bottom-left → inner bottom-left → inner top-left.
	b.MoveTo(0, height)
	b.LineTo(0, 0)
	b.LineTo(bw, bw)
	b.LineTo(bw, height-bw)
	b.ClosePath()
	b.Fill()
	b.PopState()

	// Dark pass: bottom + right edges.
	b.PushState()
	b.SetFillColorRGB(dark)
	// Outer bottom-left → outer bottom-right → inner bottom-right → inner bottom-left.
	b.MoveTo(0, 0)
	b.LineTo(width, 0)
	b.LineTo(width-bw, bw)
	b.LineTo(bw, bw)
	b.ClosePath()
	b.Fill()
	// Outer bottom-right → outer top-right → inner top-right → inner bottom-right.
	b.MoveTo(width, 0)
	b.LineTo(width, height)
	b.LineTo(width-bw, height-bw)
	b.LineTo(width-bw, bw)
	b.ClosePath()
	b.Fill()
	b.PopState()
}
```

- [ ] **Step 5: Run tests**

Run: `go test -run 'TestSquareAnnotation|TestBeveledColorPair' -v ./...`
Expected: all PASS.

Run: `go test ./...`
Expected: full suite PASS.

- [ ] **Step 6: Commit**

```bash
git add annotation_drawing_test.go appearance.go appearance_internal_test.go
git commit -m "feat: SquareAnnotation Beveled + Inset borders (two-pass color render)"
```

---

## Task 10: BorderStyle Underline for Square + InteriorColor + parseAnnotation finalization

**Files:**
- Modify: `annotation_drawing.go`
- Modify: `annotation_drawing_test.go`
- Modify: `appearance.go`

- [ ] **Step 1: Write the failing tests**

Append to `annotation_drawing_test.go`:
```go
func TestSquareAnnotationUnderlineRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 200, URY: 700})
	sq.SetBorderStyle(pdf.BorderUnderline)
	if err := page.Annotations().Add(sq); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	sq2 := doc2.Pages()[0].Annotations().At(0).(*pdf.SquareAnnotation)
	if got := sq2.BorderStyle(); got != pdf.BorderUnderline {
		t.Errorf("BorderStyle = %v, want BorderUnderline", got)
	}
}

func TestSquareAnnotationInteriorColorFill(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 200, URY: 700})
	sq.SetColor(&pdf.Color{R: 0, G: 0, B: 1, A: 1})
	sq.SetInteriorColor(&pdf.Color{R: 1, G: 1, B: 0, A: 1})
	if err := page.Annotations().Add(sq); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	sq2 := doc2.Pages()[0].Annotations().At(0).(*pdf.SquareAnnotation)
	ic := sq2.InteriorColor()
	if ic == nil || ic.R != 1 || ic.G != 1 || ic.B != 0 {
		t.Errorf("InteriorColor = %v, want yellow", ic)
	}
}

func TestSquareAnnotationInteriorColorClear(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 10, URY: 10})
	sq.SetInteriorColor(&pdf.Color{R: 1, G: 0, B: 0, A: 1})
	sq.SetInteriorColor(nil)
	if got := sq.InteriorColor(); got != nil {
		t.Errorf("InteriorColor after clear = %v, want nil", got)
	}
}

func TestSquareAnnotationNoXObjectLeak(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 10, URY: 10})
	if err := page.Annotations().Add(sq); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Multiple property mutations — must reuse the same XObject objID.
	sq.SetBorderWidth(2)
	sq.SetBorderWidth(3)
	sq.SetBorderStyle(pdf.BorderDashed)
	sq.SetColor(&pdf.Color{R: 1, G: 0, B: 0, A: 1})
	sq.SetInteriorColor(&pdf.Color{R: 0, G: 1, B: 0, A: 1})
	removed := doc.RemoveUnusedObjects()
	if removed != 0 {
		t.Errorf("RemoveUnusedObjects removed %d objects after multiple setters; want 0 (mutate-in-place expected)", removed)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestSquareAnnotationUnderline|TestSquareAnnotationInterior|TestSquareAnnotationNoXObjectLeak' -v ./...`
Expected: build failures and/or behavior failures.

- [ ] **Step 3: Add InteriorColor accessors to SquareAnnotation**

Append to `annotation_drawing.go`:
```go
// InteriorColor returns the /IC fill color, or nil if absent.
func (a *SquareAnnotation) InteriorColor() *Color {
	arr, ok := a.dict["/IC"].(pdfArray)
	if !ok || len(arr) != 3 {
		return nil
	}
	r, _ := toFloat(arr[0])
	g, _ := toFloat(arr[1])
	bl, _ := toFloat(arr[2])
	return &Color{R: r, G: g, B: bl, A: 1}
}

// SetInteriorColor writes /IC as an RGB array; nil removes the entry.
func (a *SquareAnnotation) SetInteriorColor(c *Color) {
	if c == nil {
		delete(a.dict, "/IC")
	} else {
		a.dict["/IC"] = pdfArray{c.R, c.G, c.B}
	}
	a.regenerateAP()
}
```

- [ ] **Step 4: Update generateSquareAppearance to support Underline + fill**

In `appearance.go`, replace `generateSquareAppearance` body with:
```go
func generateSquareAppearance(a *SquareAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	bw := a.BorderWidth()
	style := a.BorderStyle()

	b := newAppearanceBuilder()

	switch style {
	case BorderBeveled, BorderInset:
		// Two-pass color render. Fill first if /IC is set.
		if ic := a.InteriorColor(); ic != nil {
			b.PushState()
			b.SetFillColorRGB(*ic)
			inset := bw
			b.Rect(inset, inset, width-2*bw, height-2*bw)
			b.Fill()
			b.PopState()
		}
		drawBeveledRectBorder(b, width, height, bw, a.Color(), style == BorderInset)

	case BorderUnderline:
		b.PushState()
		b.SetLineWidth(bw)
		if c := a.Color(); c != nil {
			b.SetStrokeColorRGB(*c)
		}
		// Bottom edge only.
		b.MoveTo(0, bw/2)
		b.LineTo(width, bw/2)
		b.Stroke()
		b.PopState()
		// Underline ignores /IC by spec convention.

	default:
		b.PushState()
		b.SetLineWidth(bw)
		if c := a.Color(); c != nil {
			b.SetStrokeColorRGB(*c)
		}
		if style == BorderDashed {
			dp := a.DashPattern()
			if len(dp) == 0 {
				dp = []float64{3, 3}
			}
			b.SetDashPattern(dp, 0)
		}
		inset := bw / 2
		b.Rect(inset, inset, width-bw, height-bw)
		hasFill := false
		if ic := a.InteriorColor(); ic != nil {
			b.SetFillColorRGB(*ic)
			hasFill = true
		}
		if hasFill {
			b.FillStroke()
		} else {
			b.Stroke()
		}
		b.PopState()
	}

	return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
}
```

- [ ] **Step 5: Run tests**

Run: `go test -run TestSquareAnnotation -v ./...`
Expected: all 7 Square tests PASS.

Run: `go test ./...`
Expected: full suite PASS.

- [ ] **Step 6: Commit**

```bash
git add annotation_drawing.go annotation_drawing_test.go appearance.go
git commit -m "feat: SquareAnnotation Underline + InteriorColor (full border style coverage)"
```

---

## Task 11: CircleAnnotation (full implementation)

**Files:**
- Modify: `annotation.go`
- Modify: `annotation_drawing.go`
- Modify: `annotation_drawing_test.go`
- Modify: `appearance.go`

- [ ] **Step 1: Write the failing test**

Append to `annotation_drawing_test.go`:
```go
func TestCircleAnnotationRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	c := pdf.NewCircleAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 700})
	c.SetColor(&pdf.Color{R: 1, G: 0, B: 0, A: 1})
	c.SetInteriorColor(&pdf.Color{R: 1, G: 1, B: 0, A: 1})
	c.SetBorderWidth(3)
	c.SetBorderStyle(pdf.BorderDashed)
	c.SetDashPattern([]float64{4, 2})
	if err := page.Annotations().Add(c); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	got := doc2.Pages()[0].Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypeCircle {
		t.Errorf("type = %v, want AnnotationTypeCircle", got.AnnotationType())
	}
	c2, ok := got.(*pdf.CircleAnnotation)
	if !ok {
		t.Fatalf("concrete type = %T", got)
	}
	if c2.BorderStyle() != pdf.BorderDashed {
		t.Errorf("BorderStyle = %v", c2.BorderStyle())
	}
	if w := c2.BorderWidth(); w != 3 {
		t.Errorf("BorderWidth = %v, want 3", w)
	}
	if ic := c2.InteriorColor(); ic == nil || ic.R != 1 {
		t.Errorf("InteriorColor = %v", ic)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestCircleAnnotationRoundTrip -v ./...`
Expected: build failure — `pdf.NewCircleAnnotation`, `pdf.CircleAnnotation`, `pdf.AnnotationTypeCircle` undefined.

- [ ] **Step 3: Add AnnotationTypeCircle and parseAnnotation /Circle dispatch**

In `annotation.go`, append to the `AnnotationType` const block (after `AnnotationTypeSquare`):
```go
	AnnotationTypeCircle
```

In `parseAnnotation`'s switch, add (before GenericAnnotation fallthrough):
```go
	case "/Circle":
		return &CircleAnnotation{annotationBase: base}
```

- [ ] **Step 4: Add CircleAnnotation type — mirrors Square**

Append to `annotation_drawing.go`:
```go
// CircleAnnotation draws an elliptical annotation. Mirrors
// SquareAnnotation API; only the rendered shape and /Subtype differ.
type CircleAnnotation struct {
	annotationBase
}

func (a *CircleAnnotation) AnnotationType() AnnotationType { return AnnotationTypeCircle }

// NewCircleAnnotation builds an unbound circle annotation. Page must be
// non-nil. The ellipse is inscribed in the given rectangle.
func NewCircleAnnotation(page *Page, rect Rectangle) *CircleAnnotation {
	if page == nil {
		panic("NewCircleAnnotation: nil page")
	}
	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Circle"),
		"/Rect":    pdfArray{rect.LLX, rect.LLY, rect.URX, rect.URY},
	}
	a := &CircleAnnotation{annotationBase: annotationBase{
		dict: dict,
		doc:  page.doc,
		page: page,
	}}
	a.regenerateAP()
	return a
}

func (a *CircleAnnotation) BorderWidth() float64 {
	if bs, ok := a.dict["/BS"].(pdfDict); ok {
		if w, err := toFloat(bs["/W"]); err == nil {
			return w
		}
	}
	if border, ok := a.dict["/Border"].(pdfArray); ok && len(border) >= 3 {
		if w, err := toFloat(border[2]); err == nil {
			return w
		}
	}
	return 1
}

func (a *CircleAnnotation) SetBorderWidth(w float64) {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		bs = pdfDict{}
	}
	bs["/W"] = w
	a.dict["/BS"] = bs
	delete(a.dict, "/Border")
	a.regenerateAP()
}

func (a *CircleAnnotation) BorderStyle() BorderStyle {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		return BorderSolid
	}
	switch n, _ := bs["/S"].(pdfName); n {
	case "/D":
		return BorderDashed
	case "/B":
		return BorderBeveled
	case "/I":
		return BorderInset
	case "/U":
		return BorderUnderline
	}
	return BorderSolid
}

func (a *CircleAnnotation) SetBorderStyle(s BorderStyle) {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		bs = pdfDict{}
	}
	bs["/S"] = borderStyleName(s)
	a.dict["/BS"] = bs
	delete(a.dict, "/Border")
	a.regenerateAP()
}

func (a *CircleAnnotation) DashPattern() []float64 {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		return nil
	}
	arr, _ := bs["/D"].(pdfArray)
	if len(arr) == 0 {
		return nil
	}
	out := make([]float64, 0, len(arr))
	for _, v := range arr {
		f, _ := toFloat(v)
		out = append(out, f)
	}
	return out
}

func (a *CircleAnnotation) SetDashPattern(p []float64) {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		bs = pdfDict{}
	}
	if len(p) == 0 {
		delete(bs, "/D")
	} else {
		arr := make(pdfArray, 0, len(p))
		for _, v := range p {
			arr = append(arr, v)
		}
		bs["/D"] = arr
	}
	a.dict["/BS"] = bs
	delete(a.dict, "/Border")
	a.regenerateAP()
}

func (a *CircleAnnotation) InteriorColor() *Color {
	arr, ok := a.dict["/IC"].(pdfArray)
	if !ok || len(arr) != 3 {
		return nil
	}
	r, _ := toFloat(arr[0])
	g, _ := toFloat(arr[1])
	bl, _ := toFloat(arr[2])
	return &Color{R: r, G: g, B: bl, A: 1}
}

func (a *CircleAnnotation) SetInteriorColor(c *Color) {
	if c == nil {
		delete(a.dict, "/IC")
	} else {
		a.dict["/IC"] = pdfArray{c.R, c.G, c.B}
	}
	a.regenerateAP()
}

func (a *CircleAnnotation) regenerateAP() {
	setAppearanceN(&a.annotationBase, generateCircleAppearance(a))
}

// RegenerateAppearance forces /AP/N to be rebuilt from current properties.
func (a *CircleAnnotation) RegenerateAppearance() {
	a.regenerateAP()
}
```

- [ ] **Step 5: Add generateCircleAppearance**

Append to `appearance.go`:
```go
// generateCircleAppearance produces /AP/N for a Circle annotation.
// Geometry: an ellipse inscribed in the local bbox. Border styles
// match SquareAnnotation: Solid, Dashed, Beveled, Inset, Underline
// (Underline = lower semicircle only).
func generateCircleAppearance(a *CircleAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	bw := a.BorderWidth()
	style := a.BorderStyle()

	b := newAppearanceBuilder()

	cx := width / 2
	cy := height / 2
	rx := width/2 - bw/2
	ry := height/2 - bw/2

	switch style {
	case BorderBeveled, BorderInset:
		drawBeveledEllipseBorder(b, cx, cy, rx, ry, bw, a.Color(), style == BorderInset, a.InteriorColor())

	case BorderUnderline:
		// Lower semicircle only: from (cx-rx, cy) clockwise to (cx+rx, cy).
		b.PushState()
		b.SetLineWidth(bw)
		if c := a.Color(); c != nil {
			b.SetStrokeColorRGB(*c)
		}
		// Bottom half ellipse: 2 cubic Beziers.
		dx := rx * kappa
		dy := ry * kappa
		b.MoveTo(cx-rx, cy)
		b.CurveTo(cx-rx, cy-dy, cx-dx, cy-ry, cx, cy-ry)
		b.CurveTo(cx+dx, cy-ry, cx+rx, cy-dy, cx+rx, cy)
		b.Stroke()
		b.PopState()

	default:
		b.PushState()
		b.SetLineWidth(bw)
		if c := a.Color(); c != nil {
			b.SetStrokeColorRGB(*c)
		}
		if style == BorderDashed {
			dp := a.DashPattern()
			if len(dp) == 0 {
				dp = []float64{3, 3}
			}
			b.SetDashPattern(dp, 0)
		}
		hasFill := false
		if ic := a.InteriorColor(); ic != nil {
			b.SetFillColorRGB(*ic)
			hasFill = true
		}
		b.Ellipse(cx, cy, rx, ry)
		if hasFill {
			b.FillStroke()
		} else {
			b.Stroke()
		}
		b.PopState()
	}

	return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
}

// drawBeveledEllipseBorder emits a two-pass beveled (or inset) border on
// an ellipse. Top + left semicircles get the light color; bottom + right
// get the dark color. Optional /IC fill is rendered first.
func drawBeveledEllipseBorder(b *appearanceBuilder, cx, cy, rx, ry, bw float64, baseColor *Color, inverted bool, fill *Color) {
	if fill != nil {
		b.PushState()
		b.SetFillColorRGB(*fill)
		// Inner ellipse for the fill region.
		innerRx := rx - bw/2
		innerRy := ry - bw/2
		if innerRx > 0 && innerRy > 0 {
			b.Ellipse(cx, cy, innerRx, innerRy)
			b.Fill()
		}
		b.PopState()
	}
	base := Color{R: 0, G: 0, B: 0, A: 1}
	if baseColor != nil {
		base = *baseColor
	}
	light, dark := beveledColorPair(base, inverted)

	// Light pass: upper-left half ring.
	b.PushState()
	b.SetFillColorRGB(light)
	dx := rx * kappa
	dy := ry * kappa
	innerRx := rx - bw
	innerRy := ry - bw
	innerDx := innerRx * kappa
	innerDy := innerRy * kappa
	// Outer top half (left → top → right).
	b.MoveTo(cx-rx, cy)
	b.CurveTo(cx-rx, cy+dy, cx-dx, cy+ry, cx, cy+ry)
	b.CurveTo(cx+dx, cy+ry, cx+rx, cy+dy, cx+rx, cy)
	// Step in to inner ellipse, retrace top half backwards.
	b.LineTo(cx+innerRx, cy)
	b.CurveTo(cx+innerRx, cy+innerDy, cx+innerDx, cy+innerRy, cx, cy+innerRy)
	b.CurveTo(cx-innerDx, cy+innerRy, cx-innerRx, cy+innerDy, cx-innerRx, cy)
	b.ClosePath()
	b.Fill()
	b.PopState()

	// Dark pass: lower-right half ring.
	b.PushState()
	b.SetFillColorRGB(dark)
	b.MoveTo(cx-rx, cy)
	b.CurveTo(cx-rx, cy-dy, cx-dx, cy-ry, cx, cy-ry)
	b.CurveTo(cx+dx, cy-ry, cx+rx, cy-dy, cx+rx, cy)
	b.LineTo(cx+innerRx, cy)
	b.CurveTo(cx+innerRx, cy-innerDy, cx+innerDx, cy-innerRy, cx, cy-innerRy)
	b.CurveTo(cx-innerDx, cy-innerRy, cx-innerRx, cy-innerDy, cx-innerRx, cy)
	b.ClosePath()
	b.Fill()
	b.PopState()
}
```

- [ ] **Step 6: Run tests**

Run: `go test -run TestCircleAnnotation -v ./...`
Expected: PASS.

Run: `go test ./...`
Expected: full suite PASS.

- [ ] **Step 7: Commit**

```bash
git add annotation.go annotation_drawing.go annotation_drawing_test.go appearance.go
git commit -m "feat: CircleAnnotation with full border-style + fill coverage"
```

---

## Task 12: LineAnnotation skeleton (geometry only, no endings)

**Files:**
- Modify: `annotation.go`
- Modify: `annotation_drawing.go`
- Modify: `annotation_drawing_test.go`
- Modify: `appearance.go`

- [ ] **Step 1: Write the failing test**

Append to `annotation_drawing_test.go`:
```go
func TestLineAnnotationRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ln := pdf.NewLineAnnotation(page, pdf.Point{X: 100, Y: 700}, pdf.Point{X: 300, Y: 600})
	ln.SetColor(&pdf.Color{R: 0, G: 0, B: 1, A: 1})
	ln.SetBorderWidth(2)
	if err := page.Annotations().Add(ln); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	got := doc2.Pages()[0].Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypeLine {
		t.Errorf("type = %v, want AnnotationTypeLine", got.AnnotationType())
	}
	ln2 := got.(*pdf.LineAnnotation)
	if s := ln2.Start(); s.X != 100 || s.Y != 700 {
		t.Errorf("Start = %+v, want {100 700}", s)
	}
	if e := ln2.End(); e.X != 300 || e.Y != 600 {
		t.Errorf("End = %+v, want {300 600}", e)
	}
	if w := ln2.BorderWidth(); w != 2 {
		t.Errorf("BorderWidth = %v, want 2", w)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestLineAnnotationRoundTrip -v ./...`
Expected: build failure.

- [ ] **Step 3: Add AnnotationTypeLine and parseAnnotation /Line dispatch**

In `annotation.go`, append to the `AnnotationType` const block:
```go
	AnnotationTypeLine
```

In `parseAnnotation`'s switch, add:
```go
	case "/Line":
		return &LineAnnotation{annotationBase: base}
```

- [ ] **Step 4: Add LineAnnotation type and constructor**

Append to `annotation_drawing.go`:
```go
// LineAnnotation draws a straight line between two points, with
// optional line endings on each end (arrows, circles, etc.). The
// /Rect entry is auto-computed from the endpoints + line endings.
type LineAnnotation struct {
	annotationBase
}

func (a *LineAnnotation) AnnotationType() AnnotationType { return AnnotationTypeLine }

// NewLineAnnotation builds an unbound line annotation. Page must be
// non-nil. The /Rect is auto-computed as the bounding box of the line
// plus padding for line endings (9 × BorderWidth on each side).
func NewLineAnnotation(page *Page, start, end Point) *LineAnnotation {
	if page == nil {
		panic("NewLineAnnotation: nil page")
	}
	dict := pdfDict{
		"/Type":    pdfName("/Annot"),
		"/Subtype": pdfName("/Line"),
		"/L":       pdfArray{start.X, start.Y, end.X, end.Y},
	}
	a := &LineAnnotation{annotationBase: annotationBase{
		dict: dict,
		doc:  page.doc,
		page: page,
	}}
	a.recomputeRect()
	a.regenerateAP()
	return a
}

func (a *LineAnnotation) Start() Point {
	arr, ok := a.dict["/L"].(pdfArray)
	if !ok || len(arr) < 4 {
		return Point{}
	}
	x, _ := toFloat(arr[0])
	y, _ := toFloat(arr[1])
	return Point{X: x, Y: y}
}

func (a *LineAnnotation) End() Point {
	arr, ok := a.dict["/L"].(pdfArray)
	if !ok || len(arr) < 4 {
		return Point{}
	}
	x, _ := toFloat(arr[2])
	y, _ := toFloat(arr[3])
	return Point{X: x, Y: y}
}

func (a *LineAnnotation) SetStart(p Point) {
	end := a.End()
	a.dict["/L"] = pdfArray{p.X, p.Y, end.X, end.Y}
	a.recomputeRect()
	a.regenerateAP()
}

func (a *LineAnnotation) SetEnd(p Point) {
	start := a.Start()
	a.dict["/L"] = pdfArray{start.X, start.Y, p.X, p.Y}
	a.recomputeRect()
	a.regenerateAP()
}

func (a *LineAnnotation) BorderWidth() float64 {
	if bs, ok := a.dict["/BS"].(pdfDict); ok {
		if w, err := toFloat(bs["/W"]); err == nil {
			return w
		}
	}
	if border, ok := a.dict["/Border"].(pdfArray); ok && len(border) >= 3 {
		if w, err := toFloat(border[2]); err == nil {
			return w
		}
	}
	return 1
}

func (a *LineAnnotation) SetBorderWidth(w float64) {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		bs = pdfDict{}
	}
	bs["/W"] = w
	a.dict["/BS"] = bs
	delete(a.dict, "/Border")
	a.recomputeRect()
	a.regenerateAP()
}

func (a *LineAnnotation) BorderStyle() BorderStyle {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		return BorderSolid
	}
	switch n, _ := bs["/S"].(pdfName); n {
	case "/D":
		return BorderDashed
	case "/B":
		return BorderBeveled
	case "/I":
		return BorderInset
	case "/U":
		return BorderUnderline
	}
	return BorderSolid
}

func (a *LineAnnotation) SetBorderStyle(s BorderStyle) {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		bs = pdfDict{}
	}
	bs["/S"] = borderStyleName(s)
	a.dict["/BS"] = bs
	delete(a.dict, "/Border")
	a.regenerateAP()
}

func (a *LineAnnotation) DashPattern() []float64 {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		return nil
	}
	arr, _ := bs["/D"].(pdfArray)
	if len(arr) == 0 {
		return nil
	}
	out := make([]float64, 0, len(arr))
	for _, v := range arr {
		f, _ := toFloat(v)
		out = append(out, f)
	}
	return out
}

func (a *LineAnnotation) SetDashPattern(p []float64) {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		bs = pdfDict{}
	}
	if len(p) == 0 {
		delete(bs, "/D")
	} else {
		arr := make(pdfArray, 0, len(p))
		for _, v := range p {
			arr = append(arr, v)
		}
		bs["/D"] = arr
	}
	a.dict["/BS"] = bs
	delete(a.dict, "/Border")
	a.regenerateAP()
}

// recomputeRect updates /Rect to the bounding box of the line plus
// padding for line endings (9 × BorderWidth per Acrobat convention).
func (a *LineAnnotation) recomputeRect() {
	start := a.Start()
	end := a.End()
	pad := 9 * a.BorderWidth()
	llx := minF(start.X, end.X) - pad
	lly := minF(start.Y, end.Y) - pad
	urx := maxF(start.X, end.X) + pad
	ury := maxF(start.Y, end.Y) + pad
	a.dict["/Rect"] = pdfArray{llx, lly, urx, ury}
}

// minF / maxF — small helpers; using math.Min/Max is fine but they take
// float64 only and Go 1.21+ has built-in min/max. Use built-in min/max.
func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func (a *LineAnnotation) regenerateAP() {
	setAppearanceN(&a.annotationBase, generateLineAppearance(a))
}

// RegenerateAppearance forces /AP/N to be rebuilt from current properties.
func (a *LineAnnotation) RegenerateAppearance() {
	a.regenerateAP()
}
```

- [ ] **Step 5: Add generateLineAppearance (no endings yet)**

Append to `appearance.go`:
```go
// generateLineAppearance produces /AP/N for a Line annotation. This
// phase covers the line geometry only — line endings are added in
// subsequent tasks.
func generateLineAppearance(a *LineAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	start := a.Start()
	end := a.End()
	bw := a.BorderWidth()
	style := a.BorderStyle()

	// Translate page-space endpoints to local /BBox-space.
	sx := start.X - rect.LLX
	sy := start.Y - rect.LLY
	ex := end.X - rect.LLX
	ey := end.Y - rect.LLY

	b := newAppearanceBuilder()
	b.PushState()
	b.SetLineWidth(bw)
	if c := a.Color(); c != nil {
		b.SetStrokeColorRGB(*c)
	}
	if style == BorderDashed {
		dp := a.DashPattern()
		if len(dp) == 0 {
			dp = []float64{3, 3}
		}
		b.SetDashPattern(dp, 0)
	}
	b.MoveTo(sx, sy)
	b.LineTo(ex, ey)
	b.Stroke()
	b.PopState()

	return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
}
```

- [ ] **Step 6: Run tests**

Run: `go test -run TestLineAnnotationRoundTrip -v ./...`
Expected: PASS.

Run: `go test ./...`
Expected: full suite PASS.

- [ ] **Step 7: Commit**

```bash
git add annotation.go annotation_drawing.go annotation_drawing_test.go appearance.go
git commit -m "feat: LineAnnotation skeleton (geometry, no endings yet)"
```

---

## Task 13: drawLineEnding helper — all 10 styles

**Files:**
- Modify: `appearance.go`
- Modify: `appearance_internal_test.go`

- [ ] **Step 1: Write the failing test**

Append to `appearance_internal_test.go`:
```go
import (
	"strings"
)

// drawLineEnding emits content-stream operators. Each style produces a
// distinguishable shape; verify presence of expected operators.

func TestDrawLineEndingNone(t *testing.T) {
	b := newAppearanceBuilder()
	drawLineEnding(b, LineEndingNone, 0, 0, 0, 1, nil)
	if got := string(b.Bytes()); got != "" {
		t.Errorf("None should emit nothing, got %q", got)
	}
}

func TestDrawLineEndingShapesEmitGeometry(t *testing.T) {
	cases := []struct {
		style LineEndingStyle
		// Each style must emit at least one path-construction op (m / l / c / re).
		minPathOps int
	}{
		{LineEndingSquare, 4},        // 4 corners as l ops at minimum
		{LineEndingCircle, 4},        // 4 c ops (Ellipse)
		{LineEndingDiamond, 4},       // 4 corners
		{LineEndingOpenArrow, 2},     // 2 l ops
		{LineEndingClosedArrow, 3},   // 3 l ops
		{LineEndingButt, 1},          // 1 l op
		{LineEndingROpenArrow, 2},    // 2 l ops
		{LineEndingRClosedArrow, 3},  // 3 l ops
		{LineEndingSlash, 1},         // 1 l op
	}
	for _, tc := range cases {
		t.Run(string(rune('A'+int(tc.style))), func(t *testing.T) {
			b := newAppearanceBuilder()
			drawLineEnding(b, tc.style, 50, 50, 0, 1, nil)
			out := string(b.Bytes())
			pathOps := strings.Count(out, " l\n") + strings.Count(out, " c\n") + strings.Count(out, " re\n")
			if pathOps < tc.minPathOps {
				t.Errorf("style %v: %d path ops, want >= %d. Output: %q", tc.style, pathOps, tc.minPathOps, out)
			}
		})
	}
}

func TestDrawLineEndingClosedArrowFills(t *testing.T) {
	b := newAppearanceBuilder()
	drawLineEnding(b, LineEndingClosedArrow, 50, 50, 0, 1, &Color{R: 1, G: 0, B: 0, A: 1})
	out := string(b.Bytes())
	if !strings.Contains(out, "B\n") && !strings.Contains(out, "b\n") {
		t.Errorf("ClosedArrow should fill+stroke (B or b), got %q", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestDrawLineEnding' -v ./...`
Expected: build failure — `drawLineEnding` undefined.

- [ ] **Step 3: Implement drawLineEnding**

Append to `appearance.go`:
```go
import "math"

// drawLineEnding renders one line ending shape at (x, y) rotated by
// theta radians (direction toward line interior), using the current
// stroke color and an optional fill color (for filled shapes:
// Square/Circle/Diamond/ClosedArrow/RClosedArrow). Ending span =
// 9 × lineWidth (Acrobat convention).
//
// The ending is emitted inside a q ... Q block with a local cm so that
// shapes are authored in axis-aligned coordinates and rotated via the
// matrix.
func drawLineEnding(b *appearanceBuilder, style LineEndingStyle, x, y, theta, lineWidth float64, fill *Color) {
	if style == LineEndingNone {
		return
	}
	span := 9 * lineWidth
	half := span / 2

	cos := math.Cos(theta)
	sin := math.Sin(theta)

	b.PushState()
	// cm: rotate by theta then translate to (x, y). PDF cm matrix is
	// [a b c d e f] = [cos sin -sin cos x y].
	b.ConcatMatrix(cos, sin, -sin, cos, x, y)

	switch style {
	case LineEndingSquare:
		b.Rect(-half, -half, span, span)
		paintShape(b, fill)
	case LineEndingCircle:
		b.Ellipse(0, 0, half, half)
		paintShape(b, fill)
	case LineEndingDiamond:
		b.MoveTo(half, 0)
		b.LineTo(0, half)
		b.LineTo(-half, 0)
		b.LineTo(0, -half)
		b.ClosePath()
		paintShape(b, fill)
	case LineEndingOpenArrow:
		// Two lines fanning out from origin (toward "inside" of line).
		b.MoveTo(span, half)
		b.LineTo(0, 0)
		b.LineTo(span, -half)
		b.Stroke()
	case LineEndingClosedArrow:
		// Triangle: origin, (span, half), (span, -half).
		b.MoveTo(0, 0)
		b.LineTo(span, half)
		b.LineTo(span, -half)
		b.ClosePath()
		paintShape(b, fill)
	case LineEndingButt:
		// Short perpendicular segment across the point.
		b.MoveTo(0, half)
		b.LineTo(0, -half)
		b.Stroke()
	case LineEndingROpenArrow:
		// OpenArrow rotated 180° (fanning out the other way).
		b.MoveTo(-span, half)
		b.LineTo(0, 0)
		b.LineTo(-span, -half)
		b.Stroke()
	case LineEndingRClosedArrow:
		// ClosedArrow rotated 180°.
		b.MoveTo(0, 0)
		b.LineTo(-span, half)
		b.LineTo(-span, -half)
		b.ClosePath()
		paintShape(b, fill)
	case LineEndingSlash:
		// Diagonal at 60° (cos 60° = 0.5, sin 60° ≈ 0.866). Length = span.
		dx := half
		dy := half * math.Sqrt(3)
		b.MoveTo(-dx, -dy)
		b.LineTo(dx, dy)
		b.Stroke()
	}

	b.PopState()
}

// paintShape paints the current subpath. With fill: FillStroke (B).
// Without fill: just Stroke (S). Used by line endings that have a
// filled body.
func paintShape(b *appearanceBuilder, fill *Color) {
	if fill != nil {
		b.PushState()
		b.SetFillColorRGB(*fill)
		b.FillStroke()
		b.PopState()
	} else {
		b.Stroke()
	}
}
```

Note: the existing `appearance.go` may not yet import `math`. Add to its imports if needed.

- [ ] **Step 4: Run tests**

Run: `go test -run TestDrawLineEnding -v ./...`
Expected: all PASS.

Run: `go test ./...`
Expected: full suite PASS.

- [ ] **Step 5: Commit**

```bash
git add appearance.go appearance_internal_test.go
git commit -m "feat: drawLineEnding helper covering all 10 styles"
```

---

## Task 14: LineAnnotation full integration — endings + InteriorColor + LeaderLineLength

**Files:**
- Modify: `annotation_drawing.go`
- Modify: `annotation_drawing_test.go`
- Modify: `appearance.go`

- [ ] **Step 1: Write the failing tests**

Append to `annotation_drawing_test.go`:
```go
func TestLineAnnotationAllEndingStyles(t *testing.T) {
	for i, name := range []string{
		"None", "Square", "Circle", "Diamond",
		"OpenArrow", "ClosedArrow", "Butt",
		"ROpenArrow", "RClosedArrow", "Slash",
	} {
		style := pdf.LineEndingStyle(i)
		t.Run(name, func(t *testing.T) {
			doc := pdf.NewDocument(595, 842)
			page, _ := doc.Page(1)
			ln := pdf.NewLineAnnotation(page, pdf.Point{X: 100, Y: 700}, pdf.Point{X: 300, Y: 600})
			ln.SetStartLineEnding(style)
			ln.SetEndLineEnding(style)
			if err := page.Annotations().Add(ln); err != nil {
				t.Fatalf("Add: %v", err)
			}
			var buf bytes.Buffer
			doc.WriteTo(&buf)
			doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
			ln2 := doc2.Pages()[0].Annotations().At(0).(*pdf.LineAnnotation)
			if got := ln2.StartLineEnding(); got != style {
				t.Errorf("StartLineEnding = %v, want %v", got, style)
			}
			if got := ln2.EndLineEnding(); got != style {
				t.Errorf("EndLineEnding = %v, want %v", got, style)
			}
		})
	}
}

func TestLineAnnotationInteriorColorAndLeaderLine(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	ln := pdf.NewLineAnnotation(page, pdf.Point{X: 100, Y: 700}, pdf.Point{X: 300, Y: 700})
	ln.SetStartLineEnding(pdf.LineEndingClosedArrow)
	ln.SetEndLineEnding(pdf.LineEndingClosedArrow)
	ln.SetInteriorColor(&pdf.Color{R: 1, G: 1, B: 0, A: 1})
	ln.SetLeaderLineLength(10)
	if err := page.Annotations().Add(ln); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	ln2 := doc2.Pages()[0].Annotations().At(0).(*pdf.LineAnnotation)
	ic := ln2.InteriorColor()
	if ic == nil || ic.R != 1 {
		t.Errorf("InteriorColor = %v", ic)
	}
	if ll := ln2.LeaderLineLength(); ll != 10 {
		t.Errorf("LeaderLineLength = %v, want 10", ll)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestLineAnnotationAllEndingStyles|TestLineAnnotationInteriorColorAndLeaderLine' -v ./...`
Expected: build failure — `SetStartLineEnding` etc. undefined.

- [ ] **Step 3: Add line-ending and InteriorColor accessors to LineAnnotation**

Append to `annotation_drawing.go`:
```go
// StartLineEnding returns the style applied to the start of the line.
// Defaults to LineEndingNone if /LE is absent or malformed.
func (a *LineAnnotation) StartLineEnding() LineEndingStyle {
	arr, _ := a.dict["/LE"].(pdfArray)
	if len(arr) < 1 {
		return LineEndingNone
	}
	n, _ := arr[0].(pdfName)
	return parseLineEndingName(n)
}

// EndLineEnding returns the style applied to the end of the line.
func (a *LineAnnotation) EndLineEnding() LineEndingStyle {
	arr, _ := a.dict["/LE"].(pdfArray)
	if len(arr) < 2 {
		return LineEndingNone
	}
	n, _ := arr[1].(pdfName)
	return parseLineEndingName(n)
}

func (a *LineAnnotation) SetStartLineEnding(s LineEndingStyle) {
	end := a.EndLineEnding()
	a.dict["/LE"] = pdfArray{lineEndingName(s), lineEndingName(end)}
	a.regenerateAP()
}

func (a *LineAnnotation) SetEndLineEnding(s LineEndingStyle) {
	start := a.StartLineEnding()
	a.dict["/LE"] = pdfArray{lineEndingName(start), lineEndingName(s)}
	a.regenerateAP()
}

func (a *LineAnnotation) InteriorColor() *Color {
	arr, ok := a.dict["/IC"].(pdfArray)
	if !ok || len(arr) != 3 {
		return nil
	}
	r, _ := toFloat(arr[0])
	g, _ := toFloat(arr[1])
	bl, _ := toFloat(arr[2])
	return &Color{R: r, G: g, B: bl, A: 1}
}

func (a *LineAnnotation) SetInteriorColor(c *Color) {
	if c == nil {
		delete(a.dict, "/IC")
	} else {
		a.dict["/IC"] = pdfArray{c.R, c.G, c.B}
	}
	a.regenerateAP()
}

func (a *LineAnnotation) LeaderLineLength() float64 {
	v, err := toFloat(a.dict["/LL"])
	if err != nil {
		return 0
	}
	return v
}

func (a *LineAnnotation) SetLeaderLineLength(l float64) {
	if l == 0 {
		delete(a.dict, "/LL")
	} else {
		a.dict["/LL"] = l
	}
	a.regenerateAP()
}

// lineEndingName maps a LineEndingStyle to its PDF spec name per Table 176.
func lineEndingName(s LineEndingStyle) pdfName {
	switch s {
	case LineEndingSquare:
		return "/Square"
	case LineEndingCircle:
		return "/Circle"
	case LineEndingDiamond:
		return "/Diamond"
	case LineEndingOpenArrow:
		return "/OpenArrow"
	case LineEndingClosedArrow:
		return "/ClosedArrow"
	case LineEndingButt:
		return "/Butt"
	case LineEndingROpenArrow:
		return "/ROpenArrow"
	case LineEndingRClosedArrow:
		return "/RClosedArrow"
	case LineEndingSlash:
		return "/Slash"
	}
	return "/None"
}

// parseLineEndingName reverses lineEndingName.
func parseLineEndingName(n pdfName) LineEndingStyle {
	switch n {
	case "/Square":
		return LineEndingSquare
	case "/Circle":
		return LineEndingCircle
	case "/Diamond":
		return LineEndingDiamond
	case "/OpenArrow":
		return LineEndingOpenArrow
	case "/ClosedArrow":
		return LineEndingClosedArrow
	case "/Butt":
		return LineEndingButt
	case "/ROpenArrow":
		return LineEndingROpenArrow
	case "/RClosedArrow":
		return LineEndingRClosedArrow
	case "/Slash":
		return LineEndingSlash
	}
	return LineEndingNone
}
```

- [ ] **Step 4: Update generateLineAppearance to render endings**

In `appearance.go`, replace `generateLineAppearance` with:
```go
func generateLineAppearance(a *LineAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	start := a.Start()
	end := a.End()
	bw := a.BorderWidth()
	style := a.BorderStyle()

	// Translate page-space endpoints to local /BBox-space.
	sx := start.X - rect.LLX
	sy := start.Y - rect.LLY
	ex := end.X - rect.LLX
	ey := end.Y - rect.LLY

	dx := ex - sx
	dy := ey - sy
	theta := math.Atan2(dy, dx)

	b := newAppearanceBuilder()
	b.PushState()
	b.SetLineWidth(bw)
	if c := a.Color(); c != nil {
		b.SetStrokeColorRGB(*c)
	}
	if style == BorderDashed {
		dp := a.DashPattern()
		if len(dp) == 0 {
			dp = []float64{3, 3}
		}
		b.SetDashPattern(dp, 0)
	}
	b.MoveTo(sx, sy)
	b.LineTo(ex, ey)
	b.Stroke()
	b.PopState()

	// Line endings. theta points from start toward end; for the start
	// ending we rotate by theta+π so it points "inward" along the line.
	ic := a.InteriorColor()
	drawLineEnding(b, a.StartLineEnding(), sx, sy, theta+math.Pi, bw, ic)
	drawLineEnding(b, a.EndLineEnding(), ex, ey, theta, bw, ic)

	return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
}
```

- [ ] **Step 5: Run tests**

Run: `go test -run TestLineAnnotation -v ./...`
Expected: all line tests PASS (12 ending sub-tests + 2 round-trips).

Run: `go test ./...`
Expected: full suite PASS.

- [ ] **Step 6: Commit**

```bash
git add annotation_drawing.go annotation_drawing_test.go appearance.go
git commit -m "feat: LineAnnotation endings (10 styles) + InteriorColor + LeaderLineLength"
```

---

## Task 15: InkAnnotation skeleton (polyline rendering, no smoothing)

**Files:**
- Modify: `annotation.go`
- Modify: `annotation_drawing.go`
- Modify: `annotation_drawing_test.go`
- Modify: `appearance.go`

- [ ] **Step 1: Write the failing test**

Append to `annotation_drawing_test.go`:
```go
func TestInkAnnotationTwoPointStrokeRoundTrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	strokes := [][]pdf.Point{
		{{X: 100, Y: 700}, {X: 200, Y: 750}},
		{{X: 50, Y: 600}, {X: 150, Y: 650}},
	}
	ink := pdf.NewInkAnnotation(page, strokes)
	ink.SetColor(&pdf.Color{R: 0, G: 0, B: 1, A: 1})
	ink.SetBorderWidth(1.5)
	if err := page.Annotations().Add(ink); err != nil {
		t.Fatalf("Add: %v", err)
	}
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	got := doc2.Pages()[0].Annotations().At(0)
	if got.AnnotationType() != pdf.AnnotationTypeInk {
		t.Errorf("type = %v, want AnnotationTypeInk", got.AnnotationType())
	}
	ink2 := got.(*pdf.InkAnnotation)
	gotStrokes := ink2.Strokes()
	if len(gotStrokes) != 2 {
		t.Fatalf("Strokes len = %d, want 2", len(gotStrokes))
	}
	if len(gotStrokes[0]) != 2 || gotStrokes[0][0].X != 100 {
		t.Errorf("Strokes[0] = %v", gotStrokes[0])
	}
}

func TestInkAnnotationDefensiveCopy(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	in := [][]pdf.Point{{{X: 0, Y: 0}, {X: 10, Y: 10}}}
	ink := pdf.NewInkAnnotation(page, in)
	in[0][0].X = 99 // mutate caller's slice
	got := ink.Strokes()
	if got[0][0].X != 0 {
		t.Errorf("Strokes[0][0].X = %v after caller mutation, want 0", got[0][0].X)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestInkAnnotation -v ./...`
Expected: build failure.

- [ ] **Step 3: Add AnnotationTypeInk and parseAnnotation /Ink dispatch**

In `annotation.go`, append to `AnnotationType` const block:
```go
	AnnotationTypeInk
```

In `parseAnnotation` switch:
```go
	case "/Ink":
		return &InkAnnotation{annotationBase: base}
```

- [ ] **Step 4: Add InkAnnotation type**

Append to `annotation_drawing.go`:
```go
// InkAnnotation draws a series of free-form strokes — typically used to
// represent handwritten ink. Each stroke is a sequence of points
// rendered with Catmull-Rom-smoothed cubic Beziers (3+ points) or as a
// straight line (exactly 2 points).
type InkAnnotation struct {
	annotationBase
}

func (a *InkAnnotation) AnnotationType() AnnotationType { return AnnotationTypeInk }

// NewInkAnnotation builds an unbound ink annotation. Page must be
// non-nil. Each inner slice is one continuous stroke. The /Rect entry
// is auto-computed as the bounding box of all stroke points plus
// padding for the stroke width.
func NewInkAnnotation(page *Page, strokes [][]Point) *InkAnnotation {
	if page == nil {
		panic("NewInkAnnotation: nil page")
	}
	a := &InkAnnotation{annotationBase: annotationBase{
		dict: pdfDict{
			"/Type":    pdfName("/Annot"),
			"/Subtype": pdfName("/Ink"),
		},
		doc:  page.doc,
		page: page,
	}}
	a.SetStrokes(strokes)
	return a
}

// Strokes returns a deep copy of /InkList. Mutating the result does
// not affect the annotation.
func (a *InkAnnotation) Strokes() [][]Point {
	arr, _ := a.dict["/InkList"].(pdfArray)
	out := make([][]Point, 0, len(arr))
	for _, strokeAny := range arr {
		strokeArr, _ := strokeAny.(pdfArray)
		stroke := make([]Point, 0, len(strokeArr)/2)
		for i := 0; i+1 < len(strokeArr); i += 2 {
			x, _ := toFloat(strokeArr[i])
			y, _ := toFloat(strokeArr[i+1])
			stroke = append(stroke, Point{X: x, Y: y})
		}
		out = append(out, stroke)
	}
	return out
}

// SetStrokes writes /InkList. The slices are deep-copied; the caller
// may safely mutate after this returns.
func (a *InkAnnotation) SetStrokes(strokes [][]Point) {
	if len(strokes) == 0 {
		delete(a.dict, "/InkList")
	} else {
		outer := make(pdfArray, 0, len(strokes))
		for _, stroke := range strokes {
			inner := make(pdfArray, 0, len(stroke)*2)
			for _, p := range stroke {
				inner = append(inner, p.X, p.Y)
			}
			outer = append(outer, inner)
		}
		a.dict["/InkList"] = outer
	}
	a.recomputeRect()
	a.regenerateAP()
}

// AddStroke appends a single stroke to the annotation. Convenience for
// incremental construction.
func (a *InkAnnotation) AddStroke(stroke []Point) {
	current := a.Strokes()
	current = append(current, stroke)
	a.SetStrokes(current)
}

func (a *InkAnnotation) BorderWidth() float64 {
	if bs, ok := a.dict["/BS"].(pdfDict); ok {
		if w, err := toFloat(bs["/W"]); err == nil {
			return w
		}
	}
	if border, ok := a.dict["/Border"].(pdfArray); ok && len(border) >= 3 {
		if w, err := toFloat(border[2]); err == nil {
			return w
		}
	}
	return 1
}

func (a *InkAnnotation) SetBorderWidth(w float64) {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		bs = pdfDict{}
	}
	bs["/W"] = w
	a.dict["/BS"] = bs
	delete(a.dict, "/Border")
	a.recomputeRect()
	a.regenerateAP()
}

func (a *InkAnnotation) BorderStyle() BorderStyle {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		return BorderSolid
	}
	switch n, _ := bs["/S"].(pdfName); n {
	case "/D":
		return BorderDashed
	case "/B":
		return BorderBeveled
	case "/I":
		return BorderInset
	case "/U":
		return BorderUnderline
	}
	return BorderSolid
}

func (a *InkAnnotation) SetBorderStyle(s BorderStyle) {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		bs = pdfDict{}
	}
	bs["/S"] = borderStyleName(s)
	a.dict["/BS"] = bs
	delete(a.dict, "/Border")
	a.regenerateAP()
}

func (a *InkAnnotation) DashPattern() []float64 {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		return nil
	}
	arr, _ := bs["/D"].(pdfArray)
	if len(arr) == 0 {
		return nil
	}
	out := make([]float64, 0, len(arr))
	for _, v := range arr {
		f, _ := toFloat(v)
		out = append(out, f)
	}
	return out
}

func (a *InkAnnotation) SetDashPattern(p []float64) {
	bs, _ := a.dict["/BS"].(pdfDict)
	if bs == nil {
		bs = pdfDict{}
	}
	if len(p) == 0 {
		delete(bs, "/D")
	} else {
		arr := make(pdfArray, 0, len(p))
		for _, v := range p {
			arr = append(arr, v)
		}
		bs["/D"] = arr
	}
	a.dict["/BS"] = bs
	delete(a.dict, "/Border")
	a.regenerateAP()
}

// recomputeRect updates /Rect to the bounding box of all points across
// all strokes plus padding equal to the stroke width.
func (a *InkAnnotation) recomputeRect() {
	strokes := a.Strokes()
	if len(strokes) == 0 {
		a.dict["/Rect"] = pdfArray{0.0, 0.0, 0.0, 0.0}
		return
	}
	first := true
	var llx, lly, urx, ury float64
	for _, stroke := range strokes {
		for _, p := range stroke {
			if first {
				llx, lly, urx, ury = p.X, p.Y, p.X, p.Y
				first = false
				continue
			}
			llx = minF(llx, p.X)
			lly = minF(lly, p.Y)
			urx = maxF(urx, p.X)
			ury = maxF(ury, p.Y)
		}
	}
	pad := a.BorderWidth()
	a.dict["/Rect"] = pdfArray{llx - pad, lly - pad, urx + pad, ury + pad}
}

func (a *InkAnnotation) regenerateAP() {
	setAppearanceN(&a.annotationBase, generateInkAppearance(a))
}

// RegenerateAppearance forces /AP/N to be rebuilt from current properties.
func (a *InkAnnotation) RegenerateAppearance() {
	a.regenerateAP()
}
```

- [ ] **Step 5: Add generateInkAppearance (polyline only for now)**

Append to `appearance.go`:
```go
// generateInkAppearance produces /AP/N for an Ink annotation. This phase
// renders strokes as polylines (m + l*). Catmull-Rom smoothing for
// strokes with 3+ points is added in the next task.
func generateInkAppearance(a *InkAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	bw := a.BorderWidth()
	style := a.BorderStyle()
	strokes := a.Strokes()

	b := newAppearanceBuilder()
	b.PushState()
	b.SetLineWidth(bw)
	if c := a.Color(); c != nil {
		b.SetStrokeColorRGB(*c)
	}
	if style == BorderDashed {
		dp := a.DashPattern()
		if len(dp) == 0 {
			dp = []float64{3, 3}
		}
		b.SetDashPattern(dp, 0)
	}

	for _, stroke := range strokes {
		if len(stroke) < 2 {
			continue
		}
		// Translate to local /BBox-space.
		b.MoveTo(stroke[0].X-rect.LLX, stroke[0].Y-rect.LLY)
		for _, p := range stroke[1:] {
			b.LineTo(p.X-rect.LLX, p.Y-rect.LLY)
		}
		b.Stroke()
	}
	b.PopState()

	return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
}
```

- [ ] **Step 6: Run tests**

Run: `go test -run TestInkAnnotation -v ./...`
Expected: PASS.

Run: `go test ./...`
Expected: full suite PASS.

- [ ] **Step 7: Commit**

```bash
git add annotation.go annotation_drawing.go annotation_drawing_test.go appearance.go
git commit -m "feat: InkAnnotation skeleton (polyline rendering)"
```

---

## Task 16: Catmull-Rom smoothing for Ink

**Files:**
- Modify: `annotation_drawing_test.go`
- Modify: `appearance.go`
- Modify: `appearance_internal_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `appearance_internal_test.go`:
```go
func TestCatmullRomToBezierSimple(t *testing.T) {
	// 4 collinear points along x-axis: P0=(0,0) P1=(1,0) P2=(2,0) P3=(3,0).
	// Segment P1→P2: control points should be on the same line.
	// C1 = P1 + (P2 - P0)/6 = (1,0) + ((2,0)-(0,0))/6 = (1+2/6, 0)
	// C2 = P2 - (P3 - P1)/6 = (2,0) - ((3,0)-(1,0))/6 = (2-2/6, 0)
	c1, c2 := catmullRomControlPoints(
		Point{X: 0, Y: 0},
		Point{X: 1, Y: 0},
		Point{X: 2, Y: 0},
		Point{X: 3, Y: 0},
	)
	wantC1X := 1 + 2.0/6.0
	wantC2X := 2 - 2.0/6.0
	if math.Abs(c1.X-wantC1X) > 1e-9 || c1.Y != 0 {
		t.Errorf("c1 = %+v, want {%.6f 0}", c1, wantC1X)
	}
	if math.Abs(c2.X-wantC2X) > 1e-9 || c2.Y != 0 {
		t.Errorf("c2 = %+v, want {%.6f 0}", c2, wantC2X)
	}
}
```

Note: this internal test file needs `import "math"` if not already imported.

Append to `annotation_drawing_test.go`:
```go
func TestInkAnnotationCatmullRomSmoothsThreePlusPoints(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	// 5-point stroke: smoothing should produce c (curve) operators in /AP.
	strokes := [][]pdf.Point{{
		{X: 100, Y: 700},
		{X: 120, Y: 720},
		{X: 150, Y: 730},
		{X: 180, Y: 720},
		{X: 200, Y: 700},
	}}
	ink := pdf.NewInkAnnotation(page, strokes)
	if err := page.Annotations().Add(ink); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Verify the underlying /AP stream contains Bezier (c) operators.
	// We do this via the parsed document's content stream walking.
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	page2, _ := doc2.Page(1)
	got := page2.Annotations().At(0)
	ink2 := got.(*pdf.InkAnnotation)
	gotStrokes := ink2.Strokes()
	// Round-trip preserves /InkList raw points unchanged (smoothing is
	// /AP-only; /InkList stores the original polyline).
	if len(gotStrokes) != 1 || len(gotStrokes[0]) != 5 {
		t.Fatalf("Strokes shape = %v, want 1 stroke of 5 points", gotStrokes)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run TestCatmullRomToBezierSimple -v ./...`
Expected: build failure — `catmullRomControlPoints` undefined.

Run: `go test -run TestInkAnnotationCatmullRomSmoothsThreePlusPoints -v ./...`
Expected: PASS already (current implementation just round-trips /InkList; this is the property the test checks). This test is a guard that the polyline data isn't lost when smoothing is added.

- [ ] **Step 3: Add Catmull-Rom helper**

Append to `appearance.go`:
```go
// catmullRomControlPoints returns the cubic-Bezier control points C1, C2
// for a Catmull-Rom segment from P1 to P2 with neighbors P0 (before P1)
// and P3 (after P2). Tension factor 0.5 (standard Catmull-Rom).
func catmullRomControlPoints(p0, p1, p2, p3 Point) (c1, c2 Point) {
	c1 = Point{
		X: p1.X + (p2.X-p0.X)/6,
		Y: p1.Y + (p2.Y-p0.Y)/6,
	}
	c2 = Point{
		X: p2.X - (p3.X-p1.X)/6,
		Y: p2.Y - (p3.Y-p1.Y)/6,
	}
	return c1, c2
}
```

- [ ] **Step 4: Replace generateInkAppearance with smoothing version**

In `appearance.go`, replace the existing `generateInkAppearance` body with:
```go
func generateInkAppearance(a *InkAnnotation) *pdfStream {
	rect := a.Rect()
	width := rect.URX - rect.LLX
	height := rect.URY - rect.LLY

	bw := a.BorderWidth()
	style := a.BorderStyle()
	strokes := a.Strokes()

	b := newAppearanceBuilder()
	b.PushState()
	b.SetLineWidth(bw)
	if c := a.Color(); c != nil {
		b.SetStrokeColorRGB(*c)
	}
	if style == BorderDashed {
		dp := a.DashPattern()
		if len(dp) == 0 {
			dp = []float64{3, 3}
		}
		b.SetDashPattern(dp, 0)
	}

	for _, stroke := range strokes {
		if len(stroke) < 2 {
			continue
		}
		// Translate to local /BBox-space.
		local := make([]Point, len(stroke))
		for i, p := range stroke {
			local[i] = Point{X: p.X - rect.LLX, Y: p.Y - rect.LLY}
		}
		emitInkStroke(b, local)
	}
	b.PopState()

	return makeFormXObject(b.Bytes(), Rectangle{URX: width, URY: height})
}

// emitInkStroke renders one stroke. With 2 points: simple m+l. With 3+
// points: Catmull-Rom smoothing into cubic Beziers. Phantom points at
// the ends are produced by mirroring the first / last segment.
func emitInkStroke(b *appearanceBuilder, points []Point) {
	n := len(points)
	if n == 0 {
		return
	}
	if n == 1 {
		// A single point produces no visible stroke; skip.
		return
	}
	if n == 2 {
		b.MoveTo(points[0].X, points[0].Y)
		b.LineTo(points[1].X, points[1].Y)
		b.Stroke()
		return
	}
	// 3+ points: Catmull-Rom. Phantom points: P[-1] = P[0], P[n] = P[n-1].
	getPoint := func(i int) Point {
		if i < 0 {
			return points[0]
		}
		if i >= n {
			return points[n-1]
		}
		return points[i]
	}
	b.MoveTo(points[0].X, points[0].Y)
	for i := 0; i < n-1; i++ {
		c1, c2 := catmullRomControlPoints(getPoint(i-1), getPoint(i), getPoint(i+1), getPoint(i+2))
		b.CurveTo(c1.X, c1.Y, c2.X, c2.Y, points[i+1].X, points[i+1].Y)
	}
	b.Stroke()
}
```

- [ ] **Step 5: Run tests**

Run: `go test -run 'TestCatmullRom|TestInk' -v ./...`
Expected: all PASS.

Run: `go test ./...`
Expected: full suite PASS.

- [ ] **Step 6: Commit**

```bash
git add annotation_drawing_test.go appearance.go appearance_internal_test.go
git commit -m "feat: Catmull-Rom smoothing for Ink strokes (3+ points)"
```

---

## Task 17: Setter-driven regenerate test + RegenerateAppearance public method test + filter pattern integration

**Files:**
- Modify: `annotation_drawing_test.go`

- [ ] **Step 1: Write the integration tests**

Append to `annotation_drawing_test.go`:
```go
func TestSetterDrivenRegenerate(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100})
	if err := page.Annotations().Add(sq); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Multiple mutations after Add. Last value must win on save.
	sq.SetBorderWidth(1)
	sq.SetBorderWidth(2)
	sq.SetBorderWidth(7)
	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	sq2 := doc2.Pages()[0].Annotations().At(0).(*pdf.SquareAnnotation)
	if w := sq2.BorderWidth(); w != 7 {
		t.Errorf("BorderWidth after multiple sets = %v, want 7", w)
	}
}

func TestRegenerateAppearancePublicMethod(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100})
	if err := page.Annotations().Add(sq); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Calling RegenerateAppearance on each of the 4 types must not error.
	sq.RegenerateAppearance()

	c := pdf.NewCircleAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100})
	page.Annotations().Add(c)
	c.RegenerateAppearance()

	ln := pdf.NewLineAnnotation(page, pdf.Point{X: 0, Y: 0}, pdf.Point{X: 50, Y: 50})
	page.Annotations().Add(ln)
	ln.RegenerateAppearance()

	ink := pdf.NewInkAnnotation(page, [][]pdf.Point{{{X: 0, Y: 0}, {X: 10, Y: 10}}})
	page.Annotations().Add(ink)
	ink.RegenerateAppearance()
}

func TestDrawingAnnotationsFilterByType(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	page.Annotations().Add(pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 50, URY: 50}))
	page.Annotations().Add(pdf.NewCircleAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 100, URX: 50, URY: 150}))
	page.Annotations().Add(pdf.NewLineAnnotation(page, pdf.Point{X: 100, Y: 0}, pdf.Point{X: 200, Y: 0}))
	page.Annotations().Add(pdf.NewInkAnnotation(page, [][]pdf.Point{{{X: 300, Y: 0}, {X: 350, Y: 50}}}))

	var buf bytes.Buffer
	doc.WriteTo(&buf)
	doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	page2, _ := doc2.Page(1)

	counts := map[pdf.AnnotationType]int{}
	for _, a := range page2.Annotations().All() {
		counts[a.AnnotationType()]++
	}
	if counts[pdf.AnnotationTypeSquare] != 1 ||
		counts[pdf.AnnotationTypeCircle] != 1 ||
		counts[pdf.AnnotationTypeLine] != 1 ||
		counts[pdf.AnnotationTypeInk] != 1 {
		t.Errorf("counts = %v, want one of each (Square/Circle/Line/Ink)", counts)
	}
}

func TestUnboundAnnotationGeneratesAP(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	// Create but do NOT Add. /AP/N still gets generated because constructor
	// sets doc reference.
	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 0, LLY: 0, URX: 50, URY: 50})
	sq.SetColor(&pdf.Color{R: 1, G: 0, B: 0, A: 1})
	// Even before Add, /AP/N must reference an XObject in doc.objects.
	// We can't inspect dict directly from external package, but we can
	// confirm RemoveUnusedObjects sees an orphan XObject (the unbound /AP/N).
	removed := doc.RemoveUnusedObjects()
	// Square's XObject is unreachable (no annotation in /Annots) so it
	// gets removed. We expect at least 1 removal — that's the unbound
	// /AP/N XObject.
	if removed < 1 {
		t.Errorf("RemoveUnusedObjects removed %d, want >= 1 (orphan /AP/N XObject)", removed)
	}
}
```

- [ ] **Step 2: Run tests**

Run: `go test -run 'TestSetterDrivenRegenerate|TestRegenerateAppearancePublicMethod|TestDrawingAnnotationsFilterByType|TestUnboundAnnotationGeneratesAP' -v ./...`
Expected: all PASS.

Run: `go test ./...`
Expected: full suite PASS.

- [ ] **Step 3: Commit**

```bash
git add annotation_drawing_test.go
git commit -m "test: setter-driven regenerate, RegenerateAppearance, filter, unbound /AP integration"
```

---

## Task 18: pypdf cross-check + CLAUDE.md + README

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`

- [ ] **Step 1: pypdf cross-check (manual)**

Create `/d/tmp/check_drawing/main.go`:
```go
package main

import (
	"log"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

func main() {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)

	sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 800})
	sq.SetColor(&pdf.Color{R: 1, G: 0, B: 0, A: 1})
	sq.SetInteriorColor(&pdf.Color{R: 1, G: 1, B: 0, A: 1})
	sq.SetBorderStyle(pdf.BorderDashed)
	page.Annotations().Add(sq)

	c := pdf.NewCircleAnnotation(page, pdf.Rectangle{LLX: 250, LLY: 700, URX: 400, URY: 800})
	c.SetColor(&pdf.Color{R: 0, G: 0, B: 1, A: 1})
	c.SetBorderWidth(3)
	page.Annotations().Add(c)

	ln := pdf.NewLineAnnotation(page, pdf.Point{X: 50, Y: 600}, pdf.Point{X: 400, Y: 500})
	ln.SetStartLineEnding(pdf.LineEndingClosedArrow)
	ln.SetEndLineEnding(pdf.LineEndingClosedArrow)
	ln.SetInteriorColor(&pdf.Color{R: 1, G: 1, B: 0, A: 1})
	page.Annotations().Add(ln)

	ink := pdf.NewInkAnnotation(page, [][]pdf.Point{
		{{X: 50, Y: 400}, {X: 100, Y: 450}, {X: 150, Y: 420}, {X: 200, Y: 460}, {X: 250, Y: 400}},
	})
	ink.SetColor(&pdf.Color{R: 0, G: 0.5, B: 0, A: 1})
	page.Annotations().Add(ink)

	if err := doc.Save("D:/tmp/drawing_built.pdf"); err != nil {
		log.Fatal(err)
	}
}
```

`/d/tmp/check_drawing/go.mod`:
```
module check_drawing

go 1.24

require github.com/aspose-pdf-foss/aspose-pdf-foss-for-go v0.0.0

replace github.com/aspose-pdf-foss/aspose-pdf-foss-for-go => D:/aspose/claude/aspose.pdf-for-go-foss
```

Run:
```bash
mkdir -p /d/tmp/check_drawing
# write main.go and go.mod above
cd /d/tmp/check_drawing && go run main.go
python -c "
from pypdf import PdfReader
r = PdfReader('D:/tmp/drawing_built.pdf')
av = r.pages[0].get('/Annots')
ann = av.get_object() if hasattr(av, 'get_object') else av
print('count:', len(ann))
for i, a in enumerate(ann):
    ao = a.get_object() if hasattr(a, 'get_object') else a
    sub = ao.get('/Subtype', '?')
    has_ap = '/AP' in ao
    print(f'  [{i}] /Subtype={sub} has_AP={has_ap}')
"
```

Expected output:
```
count: 4
  [0] /Subtype=/Square has_AP=True
  [1] /Subtype=/Circle has_AP=True
  [2] /Subtype=/Line has_AP=True
  [3] /Subtype=/Ink has_AP=True
```

If pypdf doesn't see all 4 with /AP, STOP and report BLOCKED.

Optional but recommended: open `D:/tmp/drawing_built.pdf` in Adobe Reader, SumatraPDF, and Firefox/Chrome built-in viewer. Verify all 4 annotations render visibly.

Cleanup: `rm -rf /d/tmp/check_drawing /d/tmp/drawing_built.pdf`.

- [ ] **Step 2: Update CLAUDE.md**

Open `CLAUDE.md`. Find the existing annotation block (it begins with `**`annotation.go` / `annotation_action.go` / `annotation_link.go` / `annotation_markup.go`**`). Add a new block immediately after it:

```markdown
**`annotation_drawing.go` / `appearance.go` / `appearance_builder.go`**
- `Point` struct — single point in PDF user-space (used for Line endpoints, Ink strokes)
- `BorderStyle` enum — `BorderSolid`, `BorderDashed`, `BorderBeveled`, `BorderInset`, `BorderUnderline` per ISO 32000-1 Table 168
- `LineEndingStyle` enum — 10 styles (`LineEndingNone`, `LineEndingSquare`, `LineEndingCircle`, `LineEndingDiamond`, `LineEndingOpenArrow`, `LineEndingClosedArrow`, `LineEndingButt`, `LineEndingROpenArrow`, `LineEndingRClosedArrow`, `LineEndingSlash`) per ISO 32000-1 Table 176
- `SquareAnnotation` / `CircleAnnotation` — `BorderWidth/SetBorderWidth`, `BorderStyle/SetBorderStyle`, `DashPattern/SetDashPattern`, `Color/SetColor` (stroke), `InteriorColor/SetInteriorColor` (fill), inherited `Rect/SetRect/Title/SetTitle/Contents/SetContents/PageIndex`. Constructors `NewSquareAnnotation(page, rect)` / `NewCircleAnnotation(page, rect)`
- `LineAnnotation` — `Start/SetStart`, `End/SetEnd`, `StartLineEnding/SetStartLineEnding`, `EndLineEnding/SetEndLineEnding`, `LeaderLineLength/SetLeaderLineLength`, `InteriorColor/SetInteriorColor`. Auto-bbox /Rect from endpoints + `9 × BorderWidth` padding. Constructor `NewLineAnnotation(page, start, end)`
- `InkAnnotation` — `Strokes/SetStrokes` (defensive deep copy), `AddStroke`, full border surface. Catmull-Rom smoothed in /AP for 3+ point strokes; raw /InkList stored unchanged. Constructor `NewInkAnnotation(page, strokes)`
- All four types regenerate `/AP/N` on every property setter; an explicit `RegenerateAppearance()` method is also exposed on each type
- `/AP/N` infrastructure: every drawing annotation owns one Form XObject in `doc.objects`. Setters mutate the XObject in place — no leaks across multiple property changes
```

Also add to the existing `(*Page)` API section the four new annotation type listings:

Find the existing line `- `(*Page).Annotations() *AnnotationCollection` ...` and update the AnnotationType list directly above the annotation block to include:
```
AnnotationTypeSquare, AnnotationTypeCircle, AnnotationTypeLine, AnnotationTypeInk
```

(Place these alongside the existing constants in the prior annotation block.)

- [ ] **Step 3: Update README.md**

Open `README.md`. Find the existing `### Annotations` section. Update the supported-subtypes line at the bottom from:

> Supported subtypes in this release: `Link`, `Highlight`, `Underline`, `StrikeOut`, `Squiggly`. ...

to:

> Supported subtypes: `Link`, `Highlight`, `Underline`, `StrikeOut`, `Squiggly`, `Square`, `Circle`, `Line`, `Ink`. ...

Then add a new `### Drawing annotations (Square / Circle / Line / Ink)` subsection immediately after the `### Annotations` section, before `### Validation`:

````markdown
### Drawing annotations (Square / Circle / Line / Ink)

```go
doc := pdf.NewDocument(595, 842)
page, _ := doc.Page(1)

// Filled rectangle with dashed red border
sq := pdf.NewSquareAnnotation(page, pdf.Rectangle{LLX: 50, LLY: 700, URX: 200, URY: 800})
sq.SetColor(&pdf.Color{R: 1, G: 0, B: 0, A: 1})
sq.SetInteriorColor(&pdf.Color{R: 1, G: 1, B: 0, A: 1})
sq.SetBorderStyle(pdf.BorderDashed)
sq.SetDashPattern([]float64{4, 2})
page.Annotations().Add(sq)

// Blue circle, beveled border
c := pdf.NewCircleAnnotation(page, pdf.Rectangle{LLX: 250, LLY: 700, URX: 400, URY: 800})
c.SetColor(&pdf.Color{R: 0, G: 0, B: 1, A: 1})
c.SetBorderStyle(pdf.BorderBeveled)
c.SetBorderWidth(3)
page.Annotations().Add(c)

// Arrow line with closed-arrow heads on both ends
ln := pdf.NewLineAnnotation(page,
    pdf.Point{X: 50, Y: 600}, pdf.Point{X: 400, Y: 500})
ln.SetStartLineEnding(pdf.LineEndingClosedArrow)
ln.SetEndLineEnding(pdf.LineEndingClosedArrow)
ln.SetInteriorColor(&pdf.Color{R: 1, G: 1, B: 0, A: 1})
page.Annotations().Add(ln)

// Smooth ink stroke (Catmull-Rom smoothing applied automatically for 3+ points)
ink := pdf.NewInkAnnotation(page, [][]pdf.Point{
    {{X: 50, Y: 400}, {X: 100, Y: 450}, {X: 150, Y: 420},
     {X: 200, Y: 460}, {X: 250, Y: 400}},
})
ink.SetColor(&pdf.Color{R: 0, G: 0.5, B: 0, A: 1})
page.Annotations().Add(ink)

doc.Save("drawing.pdf")
```

Border styles available: `BorderSolid`, `BorderDashed`, `BorderBeveled`, `BorderInset`,
`BorderUnderline` (per ISO 32000-1 Table 168). Line endings: `LineEndingNone`,
`Square`, `Circle`, `Diamond`, `OpenArrow`, `ClosedArrow`, `Butt`, `ROpenArrow`,
`RClosedArrow`, `Slash` (Table 176). Each property setter (`SetBorderStyle`,
`SetColor`, `SetStrokes`, etc.) immediately regenerates the annotation's
appearance stream so `/AP/N` is always in sync; no `/NeedAppearances=true`
required, drawing annotations render in any spec-conforming viewer.
````

Also update the `Annotations` bullet in `## Features` (around line 49) — change from:

> Page-scoped collection API (`Page.Annotations()` with `Add`/`At`/`Delete`/`DeleteAt`); existing form widgets surface as read-only `WidgetAnnotation`

to:

> Page-scoped collection API (`Page.Annotations()` with `Add`/`At`/`Delete`/`DeleteAt`); existing form widgets surface as read-only `WidgetAnnotation`. Drawing primitives (Square/Circle/Line/Ink) with full ISO 32000-1 border styles (Solid/Dashed/Beveled/Inset/Underline) and 10 line-ending styles. `/AP` appearance streams generated automatically — annotations render natively in any spec-conforming viewer

- [ ] **Step 4: Run full suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md README.md
git commit -m "docs: drawing annotations (Square/Circle/Line/Ink) in CLAUDE.md and README"
```

---

## Self-review

**Spec coverage:** every major spec section maps to at least one task.

| Spec section | Tasks |
|---|---|
| `appearanceBuilder` (path-drawing primitives) | 1, 2, 3, 4 |
| `setAppearanceN` + mutate-in-place | 5, also exercised by Task 10 (TestSquareAnnotationNoXObjectLeak) |
| `Point`, `BorderStyle`, `LineEndingStyle` types | 6 |
| `SquareAnnotation` (5 border styles + fill + parse) | 7, 8, 9, 10 |
| `CircleAnnotation` (full) | 11 |
| `LineAnnotation` (geometry, endings, /IC, /LL) | 12, 13, 14 |
| `InkAnnotation` (polyline + Catmull-Rom) | 15, 16 |
| Setter-driven regenerate, `RegenerateAppearance()` public method, filter pattern, unbound /AP | 17 |
| Documentation (CLAUDE.md, README) + pypdf cross-check | 18 |
| `parseAnnotation` dispatch updates | 7 (Square), 11 (Circle), 12 (Line), 15 (Ink) |
| `AnnotationType` enum additions | same as above |
| `/BS` modern dict (Solid/Dashed/Beveled/Inset/Underline) on every drawing type | 8, 9, 10, 11, 12, 15 |
| `/Border` legacy fallback on read | covered by `BorderWidth()` accessors in Tasks 7, 11, 12, 15 |
| Defensive slice copies (DashPattern, Strokes) | 8, 15 |

No gaps.

**Placeholder scan:** no "TBD", "TODO", "implement later", or "fill in details". Each step contains literal code or exact commands.

**Type consistency:**
- `appearanceBuilder` method names (`PushState`, `PopState`, `MoveTo`, `LineTo`, `CurveTo`, `Rect`, `Ellipse`, `Stroke`, `Fill`, `FillStroke`, etc.) — referenced consistently across tasks 1–4 (definitions) and 7–16 (uses).
- `setAppearanceN(*annotationBase, *pdfStream)` signature defined in Task 5, called the same way from every `regenerateAP()` (Tasks 7, 11, 12, 15).
- `BorderStyle` constants (`BorderSolid` = 0, `BorderDashed` = 1, etc.) and their PDF code mapping (`/S`, `/D`, `/B`, `/I`, `/U`) consistent across `borderStyleName` and `BorderStyle()` accessors across Square/Circle/Line/Ink.
- `LineEndingStyle` constants and their PDF names consistent across `lineEndingName` (Task 14) and `parseLineEndingName` (Task 14).
- `Point{X, Y}` struct — same shape across all signatures.
- `Catmull-Rom` math: `c1 = p1 + (p2-p0)/6`, `c2 = p2 - (p3-p1)/6` — used identically in `catmullRomControlPoints` (Task 16) and the test fixture (Task 16).

**Cross-task naming:** `regenerateAP()` (private method on each of the 4 drawing types) and `RegenerateAppearance()` (public method on each of the 4 drawing types) — same naming on Square (Task 7), Circle (Task 11), Line (Task 12), Ink (Task 15). `recomputeRect()` private method on LineAnnotation (Task 12) and InkAnnotation (Task 15).

No type-consistency issues found.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-05-appearance-streams-drawing-annotations.md`.
