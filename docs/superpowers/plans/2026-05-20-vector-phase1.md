# Vector Graphics Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add first-class vector drawing primitives to `(*Page)`: lines, rectangles, circles, ellipses, polylines, polygons, arbitrary paths. Pure additive API — no breaking changes.

**Architecture:** Each draw method builds a single `q ... Q` content stream block with explicit graphics state (color, width, dash, caps/joins, alpha), path-construction operators, and a paint op (S / f / B). Helpers `formatLineStyle` / `formatShapeStyle` / `pathOpsToOperators` shared across methods. `Path` is an opaque builder with internal `pathOp` slice.

**Tech Stack:** Go 1.24, standard library only.

**Reference:** [docs/superpowers/specs/2026-05-20-vector-phase1-design.md](../specs/2026-05-20-vector-phase1-design.md)

**Beads:** [pdf-go-5pq](bd show pdf-go-5pq) (Phase 1) under umbrella [pdf-go-ybu](bd show pdf-go-ybu).

---

## File Map

| File | Purpose |
|---|---|
| `vector.go` (new) | `LineCap` / `LineJoin` enums, `LineStyle` / `ShapeStyle` structs, `Path` builder type + methods, private `formatLineStyle` / `formatShapeStyle` / `pathOpsToOperators` helpers. |
| `vector_draw.go` (new) | All `(*Page).DrawXxx` methods. |
| `vector_test.go` (new) | External tests (`package asposepdf_test`). |
| `vector_internal_test.go` (new) | Internal tests: `Path` builder, arc decomposition. |
| `CLAUDE.md` (modify, Task 15) | New "Vector graphics" section. |
| `README.md` (modify, Task 15) | Features bullet + usage snippet. |

---

## Task 1: `LineCap` / `LineJoin` enums + `LineStyle` + `ShapeStyle` types

**Files:**
- Create: `vector.go`
- Create: `vector_test.go`

- [ ] **Step 1: Append failing tests**

Create `vector_test.go`:

```go
package asposepdf_test

import (
	"testing"

	pdf "github.com/aspose/pdf-for-go"
)

func TestVector_LineCapConstants(t *testing.T) {
	if pdf.LineCapButt != 0 {
		t.Errorf("LineCapButt = %d, want 0", pdf.LineCapButt)
	}
	if pdf.LineCapRound != 1 {
		t.Errorf("LineCapRound = %d, want 1", pdf.LineCapRound)
	}
	if pdf.LineCapSquare != 2 {
		t.Errorf("LineCapSquare = %d, want 2", pdf.LineCapSquare)
	}
}

func TestVector_LineJoinConstants(t *testing.T) {
	if pdf.LineJoinMiter != 0 || pdf.LineJoinRound != 1 || pdf.LineJoinBevel != 2 {
		t.Errorf("LineJoin enum mismatch: Miter=%d Round=%d Bevel=%d",
			pdf.LineJoinMiter, pdf.LineJoinRound, pdf.LineJoinBevel)
	}
}

func TestVector_LineStyleZeroValue(t *testing.T) {
	var s pdf.LineStyle
	if s.Color != nil || s.Width != 0 || s.DashPattern != nil ||
		s.Cap != pdf.LineCapButt || s.Join != pdf.LineJoinMiter {
		t.Errorf("LineStyle zero value mismatch: %+v", s)
	}
}

func TestVector_ShapeStyleEmbedsLineStyle(t *testing.T) {
	s := pdf.ShapeStyle{
		LineStyle: pdf.LineStyle{Width: 2},
		FillColor: &pdf.Color{R: 1, G: 0, B: 0, A: 1},
	}
	if s.Width != 2 {
		t.Errorf("embedded LineStyle.Width = %g, want 2", s.Width)
	}
	if s.FillColor == nil || s.FillColor.R != 1 {
		t.Error("FillColor not preserved")
	}
}
```

- [ ] **Step 2: Run + observe build failure**

```powershell
go test -run 'TestVector_(LineCap|LineJoin|LineStyle|ShapeStyle)' -v ./...
```

- [ ] **Step 3: Create `vector.go`**

```go
package asposepdf

// LineCap controls the shape at the endpoints of an open stroked path.
// Per ISO 32000-1 §8.4.3.3 (PDF operator J).
type LineCap int

const (
	LineCapButt   LineCap = 0 // default — flat end at the endpoint
	LineCapRound  LineCap = 1 // semicircle centered on endpoint
	LineCapSquare LineCap = 2 // square extending half-width beyond endpoint
)

// LineJoin controls the shape at the corners of stroked paths.
// Per ISO 32000-1 §8.4.3.4 (PDF operator j).
type LineJoin int

const (
	LineJoinMiter LineJoin = 0 // sharp corner (clipped at miter limit)
	LineJoinRound LineJoin = 1
	LineJoinBevel LineJoin = 2
)

// LineStyle describes how a stroked path is drawn.
//
// Zero value: black, 0pt wide (no stroke), solid, butt cap, miter join.
// Mirrors Aspose.PDF for .NET's GraphInfo stroke fields.
type LineStyle struct {
	Color       *Color    // nil → black {0,0,0,1}
	Width       float64   // ≤ 0 → no stroke (the draw call becomes a no-op for stroke)
	DashPattern []float64 // [on, off, on, off, ...]; nil or empty → solid
	DashPhase   float64   // offset into the dash pattern, default 0
	Cap         LineCap   // default LineCapButt
	Join        LineJoin  // default LineJoinMiter
	MiterLimit  float64   // ≤ 0 → PDF default (10)
}

// ShapeStyle combines a stroke (LineStyle) with an optional fill color.
//
// FillColor nil → no fill (stroke-only). Width ≤ 0 in the embedded LineStyle
// → no stroke (fill-only). If both are unset, the draw call is a no-op.
//
// Mirrors Aspose.PDF for .NET's GraphInfo (stroke + fill).
type ShapeStyle struct {
	LineStyle
	FillColor *Color // nil = no fill
}
```

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestVector_(LineCap|LineJoin|LineStyle|ShapeStyle)' -v ./...
go test ./...
git add vector.go vector_test.go
git commit -m "feat: vector — LineCap/LineJoin enums + LineStyle/ShapeStyle types"
```

---

## Task 2: `Path` type + `MoveTo` / `LineTo` / `Close` builder methods

**Files:**
- Modify: `vector.go`
- Create: `vector_internal_test.go`

- [ ] **Step 1: Append failing internal tests**

Create `vector_internal_test.go`:

```go
package asposepdf

import "testing"

func TestPath_NewIsEmpty(t *testing.T) {
	p := NewPath()
	if p == nil {
		t.Fatal("NewPath returned nil")
	}
	if len(p.ops) != 0 {
		t.Errorf("ops = %d, want 0", len(p.ops))
	}
}

func TestPath_MoveToLineToClose(t *testing.T) {
	p := NewPath().MoveTo(10, 20).LineTo(30, 40).Close()
	if len(p.ops) != 3 {
		t.Fatalf("ops = %d, want 3", len(p.ops))
	}
	if p.ops[0].kind != pathOpMoveTo || p.ops[0].x != 10 || p.ops[0].y != 20 {
		t.Errorf("op[0] = %+v", p.ops[0])
	}
	if p.ops[1].kind != pathOpLineTo || p.ops[1].x != 30 || p.ops[1].y != 40 {
		t.Errorf("op[1] = %+v", p.ops[1])
	}
	if p.ops[2].kind != pathOpClose {
		t.Errorf("op[2].kind = %v, want pathOpClose", p.ops[2].kind)
	}
}

func TestPath_Chaining(t *testing.T) {
	// Each mutator returns *Path for chaining.
	p := NewPath().MoveTo(0, 0).LineTo(1, 1).LineTo(2, 0).LineTo(1, -1).Close()
	if len(p.ops) != 5 {
		t.Errorf("len = %d, want 5", len(p.ops))
	}
}
```

- [ ] **Step 2: Run + observe build failure**

```powershell
go test -run 'TestPath_' -v ./...
```

- [ ] **Step 3: Add to `vector.go`**

```go
// pathOpKind enumerates the kinds of operations a Path can contain.
type pathOpKind int

const (
	pathOpMoveTo pathOpKind = iota
	pathOpLineTo
	pathOpCurveTo // cubic Bezier — uses x/y as endpoint plus c1x/c1y/c2x/c2y
	pathOpClose
)

// pathOp is a single operation in a Path. Unused fields for the kind are zero.
type pathOp struct {
	kind               pathOpKind
	x, y               float64 // endpoint (MoveTo, LineTo, CurveTo)
	c1x, c1y, c2x, c2y float64 // bezier control points (CurveTo only)
}

// Path is a sequence of MoveTo/LineTo/CurveTo/Close operations defining an
// arbitrary 2D path in PDF user space (origin at page bottom-left, Y up).
//
// Construct via NewPath() and chain mutator methods, then pass to
// (*Page).DrawPath.
type Path struct {
	ops []pathOp
}

// NewPath returns an empty path.
func NewPath() *Path { return &Path{} }

// MoveTo begins a new subpath at (x, y). Returns p for chaining.
func (p *Path) MoveTo(x, y float64) *Path {
	p.ops = append(p.ops, pathOp{kind: pathOpMoveTo, x: x, y: y})
	return p
}

// LineTo adds a straight line segment from the current point to (x, y).
// If there is no current point, equivalent to MoveTo.
func (p *Path) LineTo(x, y float64) *Path {
	p.ops = append(p.ops, pathOp{kind: pathOpLineTo, x: x, y: y})
	return p
}

// Close closes the current subpath with a line back to the most recent MoveTo.
// PDF operator h.
func (p *Path) Close() *Path {
	p.ops = append(p.ops, pathOp{kind: pathOpClose})
	return p
}
```

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestPath_' -v ./...
go test ./...
git add vector.go vector_internal_test.go
git commit -m "feat: vector — Path builder with MoveTo/LineTo/Close"
```

---

## Task 3: `Path.CurveTo` + `Path.QuadTo`

**Files:**
- Modify: `vector.go`
- Modify: `vector_internal_test.go`

- [ ] **Step 1: Append failing internal tests**

```go
func TestPath_CurveTo(t *testing.T) {
	p := NewPath().MoveTo(0, 0).CurveTo(10, 0, 20, 10, 30, 30)
	if len(p.ops) != 2 {
		t.Fatalf("ops = %d, want 2", len(p.ops))
	}
	op := p.ops[1]
	if op.kind != pathOpCurveTo {
		t.Errorf("kind = %v, want CurveTo", op.kind)
	}
	if op.c1x != 10 || op.c1y != 0 || op.c2x != 20 || op.c2y != 10 || op.x != 30 || op.y != 30 {
		t.Errorf("control points = %+v", op)
	}
}

func TestPath_QuadToConvertsToCubic(t *testing.T) {
	// Quadratic with current point (P0=0,0), control (Q=10,10), endpoint (P3=20,0).
	// Equivalent cubic control points (per standard conversion):
	//   C1 = P0 + (2/3)(Q - P0) = (20/3, 20/3) ≈ (6.667, 6.667)
	//   C2 = P3 + (2/3)(Q - P3) = (20 + 2/3*(10-20), 0 + 2/3*10) = (40/3, 20/3) ≈ (13.333, 6.667)
	p := NewPath().MoveTo(0, 0).QuadTo(10, 10, 20, 0)
	if len(p.ops) != 2 {
		t.Fatalf("ops = %d, want 2", len(p.ops))
	}
	op := p.ops[1]
	if op.kind != pathOpCurveTo {
		t.Fatalf("kind = %v, want CurveTo (auto-converted)", op.kind)
	}
	const eps = 1e-9
	if abs(op.c1x-20.0/3) > eps || abs(op.c1y-20.0/3) > eps {
		t.Errorf("c1 = (%g, %g), want (20/3, 20/3)", op.c1x, op.c1y)
	}
	if abs(op.c2x-40.0/3) > eps || abs(op.c2y-20.0/3) > eps {
		t.Errorf("c2 = (%g, %g), want (40/3, 20/3)", op.c2x, op.c2y)
	}
	if op.x != 20 || op.y != 0 {
		t.Errorf("endpoint = (%g, %g), want (20, 0)", op.x, op.y)
	}
}

// abs is a tiny float64 absolute-value helper for tests.
func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func TestPath_QuadToNoCurrentPoint_AssumesOrigin(t *testing.T) {
	// PDF spec says paths with no MoveTo start at (0,0). QuadTo should treat
	// the missing current point as (0,0) for control-point conversion.
	p := NewPath().QuadTo(10, 10, 20, 0)
	if len(p.ops) != 1 {
		t.Fatalf("ops = %d, want 1", len(p.ops))
	}
	op := p.ops[0]
	if op.kind != pathOpCurveTo {
		t.Errorf("kind = %v", op.kind)
	}
}
```

- [ ] **Step 2: Run + observe failure**

- [ ] **Step 3: Add to `vector.go`**

```go
// CurveTo adds a cubic Bezier curve from the current point to (x, y) with
// control points (c1x, c1y) and (c2x, c2y). PDF operator c.
func (p *Path) CurveTo(c1x, c1y, c2x, c2y, x, y float64) *Path {
	p.ops = append(p.ops, pathOp{
		kind: pathOpCurveTo,
		x:    x, y: y,
		c1x: c1x, c1y: c1y, c2x: c2x, c2y: c2y,
	})
	return p
}

// QuadTo adds a quadratic Bezier curve (one control point) from the current
// point to (x, y), automatically converted to the equivalent cubic per the
// standard quadratic-to-cubic formula:
//
//	C1 = P0 + (2/3) * (Q - P0)
//	C2 = P3 + (2/3) * (Q - P3)
//
// If there is no current point, treats (0, 0) as the start (matching PDF
// path semantics).
func (p *Path) QuadTo(cx, cy, x, y float64) *Path {
	// Find current point — last MoveTo/LineTo/CurveTo endpoint, or (0,0).
	var p0x, p0y float64
	for i := len(p.ops) - 1; i >= 0; i-- {
		op := p.ops[i]
		if op.kind == pathOpMoveTo || op.kind == pathOpLineTo || op.kind == pathOpCurveTo {
			p0x, p0y = op.x, op.y
			break
		}
	}
	c1x := p0x + (2.0/3.0)*(cx-p0x)
	c1y := p0y + (2.0/3.0)*(cy-p0y)
	c2x := x + (2.0/3.0)*(cx-x)
	c2y := y + (2.0/3.0)*(cy-y)
	return p.CurveTo(c1x, c1y, c2x, c2y, x, y)
}
```

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestPath_(CurveTo|QuadTo)' -v ./...
go test ./...
git add vector.go vector_internal_test.go
git commit -m "feat: vector — Path.CurveTo (cubic) + Path.QuadTo (auto-converts to cubic)"
```

---

## Task 4: `Path.Arc` via cubic Bezier approximation

**Files:**
- Modify: `vector.go`
- Modify: `vector_internal_test.go`

The classic 4-bezier circle approximation uses control-point magnitude `k = 4/3 × tan(θ/4)` for an arc of angle `θ`. For 90° arcs, `k ≈ 0.5522847498`. For arbitrary sweeps, subdivide into ≤90° chunks and use the right `k` per chunk.

- [ ] **Step 1: Append failing internal tests**

```go
func TestPathArc_QuarterCircle(t *testing.T) {
	// Quarter-circle from (1, 0) to (0, 1) — sweep 90° starting at angle 0.
	// Should produce exactly 1 CurveTo (plus a MoveTo for the start).
	p := NewPath().Arc(0, 0, 1, 0, math.Pi/2)
	curveCount := 0
	for _, op := range p.ops {
		if op.kind == pathOpCurveTo {
			curveCount++
		}
	}
	if curveCount != 1 {
		t.Errorf("quarter arc curve count = %d, want 1", curveCount)
	}
	// Endpoint should be near (0, 1).
	last := p.ops[len(p.ops)-1]
	if abs(last.x-0) > 1e-9 || abs(last.y-1) > 1e-9 {
		t.Errorf("endpoint = (%g, %g), want (0, 1)", last.x, last.y)
	}
}

func TestPathArc_FullCircle(t *testing.T) {
	// Full circle — 4 cubic Bezier arcs.
	p := NewPath().Arc(0, 0, 1, 0, 2*math.Pi)
	curveCount := 0
	for _, op := range p.ops {
		if op.kind == pathOpCurveTo {
			curveCount++
		}
	}
	if curveCount != 4 {
		t.Errorf("full-circle arc curve count = %d, want 4", curveCount)
	}
}

func TestPathArc_270Degrees(t *testing.T) {
	// 270° → 3 Bezier curves.
	p := NewPath().Arc(0, 0, 1, 0, 1.5*math.Pi)
	curveCount := 0
	for _, op := range p.ops {
		if op.kind == pathOpCurveTo {
			curveCount++
		}
	}
	if curveCount != 3 {
		t.Errorf("270° arc curve count = %d, want 3", curveCount)
	}
}

func TestPathArc_NegativeSweep(t *testing.T) {
	// Clockwise (negative sweep) 90°: should still work, endpoint moves CW.
	p := NewPath().Arc(0, 0, 1, math.Pi/2, -math.Pi/2)
	curveCount := 0
	for _, op := range p.ops {
		if op.kind == pathOpCurveTo {
			curveCount++
		}
	}
	if curveCount != 1 {
		t.Errorf("CW quarter curve count = %d, want 1", curveCount)
	}
	last := p.ops[len(p.ops)-1]
	if abs(last.x-1) > 1e-9 || abs(last.y-0) > 1e-9 {
		t.Errorf("CW endpoint = (%g, %g), want (1, 0)", last.x, last.y)
	}
}
```

Add `import "math"` to `vector_internal_test.go`.

- [ ] **Step 2: Run + observe failure**

- [ ] **Step 3: Implement in `vector.go`**

Add `import "math"` to `vector.go`.

```go
// Arc adds an arc to the path, approximated by cubic Bezier curves.
//
// (cx, cy) is the center; r is the radius. startAngle and sweepAngle are in
// radians; sweepAngle may be negative (clockwise). The arc is subdivided into
// segments of ≤90°, each approximated by one cubic Bezier using
// k = (4/3) * tan(segmentAngle / 4) for the control-point magnitude.
//
// If the path has no current point, MoveTo to the arc's start is implicit.
// After the call, the path's current point is the arc's endpoint.
func (p *Path) Arc(cx, cy, r, startAngle, sweepAngle float64) *Path {
	if r <= 0 || sweepAngle == 0 {
		return p
	}

	// Add implicit MoveTo if path has no current point.
	hasCurrent := false
	for i := len(p.ops) - 1; i >= 0; i-- {
		k := p.ops[i].kind
		if k == pathOpMoveTo || k == pathOpLineTo || k == pathOpCurveTo {
			hasCurrent = true
			break
		}
	}
	if !hasCurrent {
		p.MoveTo(cx+r*math.Cos(startAngle), cy+r*math.Sin(startAngle))
	}

	// Subdivide sweep into ≤90° segments.
	const maxSegAngle = math.Pi / 2
	abs := func(v float64) float64 {
		if v < 0 {
			return -v
		}
		return v
	}
	totalAbs := abs(sweepAngle)
	nSegs := int(math.Ceil(totalAbs / maxSegAngle))
	if nSegs < 1 {
		nSegs = 1
	}
	segAngle := sweepAngle / float64(nSegs)
	k := (4.0 / 3.0) * math.Tan(segAngle/4.0)

	a0 := startAngle
	for i := 0; i < nSegs; i++ {
		a1 := a0 + segAngle
		cos0, sin0 := math.Cos(a0), math.Sin(a0)
		cos1, sin1 := math.Cos(a1), math.Sin(a1)
		// Tangent at a0 is (-sin0, cos0); at a1 is (-sin1, cos1).
		c1x := cx + r*(cos0-k*sin0)
		c1y := cy + r*(sin0+k*cos0)
		c2x := cx + r*(cos1+k*sin1)
		c2y := cy + r*(sin1-k*cos1)
		ex := cx + r*cos1
		ey := cy + r*sin1
		p.CurveTo(c1x, c1y, c2x, c2y, ex, ey)
		a0 = a1
	}
	return p
}
```

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestPathArc_' -v ./...
go test ./...
git add vector.go vector_internal_test.go
git commit -m "feat: vector — Path.Arc via cubic Bezier approximation (Goldapp/Stanislav formula)"
```

---

## Task 5: `(*Page).DrawLine` + `formatLineStyle` helper

This task introduces the first rendering path. The shared `formatLineStyle` helper used by all subsequent draw methods is defined here.

**Files:**
- Create: `vector_draw.go`
- Modify: `vector_test.go`

- [ ] **Step 1: Append failing tests**

Append to `vector_test.go`:

```go
import (
	"bytes"
	"strings"
)

// renderedVectorContent decodes flate-compressed content streams (same helper
// pattern as table tests) so test assertions can inspect raw PDF operators.
// If a similar helper already exists in another _test.go file (renderedContent
// from table_test.go), reuse that — they live in the same test package.
//
// (Note: if the existing renderedContent helper from table_test.go is
// already in scope here as a package-test helper, just call it directly.
// Otherwise, this task creates a local copy or moves it to a shared helper.)

func TestDrawLine_BasicSolidStroke(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawLine(
		pdf.Point{X: 100, Y: 100},
		pdf.Point{X: 200, Y: 150},
		pdf.LineStyle{Color: &pdf.Color{R: 1, G: 0, B: 0, A: 1}, Width: 2},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	for _, want := range []string{"100 100 m", "200 150 l", "S", "1 0 0 RG", "2 w"} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestDrawLine_DashPattern(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawLine(
		pdf.Point{X: 0, Y: 0}, pdf.Point{X: 100, Y: 0},
		pdf.LineStyle{Width: 1, DashPattern: []float64{4, 2}},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	if !strings.Contains(s, "[4 2] 0 d") {
		t.Errorf("output missing dash pattern: %s", s)
	}
}

func TestDrawLine_WidthZero_NoStroke(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawLine(pdf.Point{}, pdf.Point{X: 100}, pdf.LineStyle{Width: 0})
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	if strings.Contains(s, " S\n") {
		t.Error("width=0 should not emit stroke op")
	}
}

func TestDrawLine_LineCapRound(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	_ = page.DrawLine(pdf.Point{}, pdf.Point{X: 50}, pdf.LineStyle{
		Width: 4, Cap: pdf.LineCapRound,
	})
	s := renderedContent(t, doc)
	if !strings.Contains(s, "1 J") {
		t.Error("LineCapRound should emit `1 J`")
	}
}
```

If `renderedContent` doesn't exist in scope (it's defined in `table_test.go` which is the same package `asposepdf_test`), just call it — Go's test compilation links all files in the package. If it's not directly accessible (unlikely), copy it. Check first.

- [ ] **Step 2: Run + observe build failure**

```powershell
go test -run 'TestDrawLine_' -v ./...
```

`DrawLine` undefined.

- [ ] **Step 3: Create `vector_draw.go`**

```go
package asposepdf

import (
	"fmt"
	"strings"
)

// formatLineStyle emits the PDF graphics state operators for stroking with
// the given style: w (width), J (cap), j (join), M (miter limit), d (dash),
// RG (stroke color). Always emits all six for predictable behavior — defaults
// from the surrounding gstate would otherwise leak through `q`.
//
// Returns "" if style.Width <= 0 (caller should not emit a stroke).
func formatLineStyle(s LineStyle) string {
	if s.Width <= 0 {
		return ""
	}
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("%s w\n", formatFloat(s.Width)))
	buf.WriteString(fmt.Sprintf("%d J\n", int(s.Cap)))
	buf.WriteString(fmt.Sprintf("%d j\n", int(s.Join)))
	if s.MiterLimit > 0 {
		buf.WriteString(fmt.Sprintf("%s M\n", formatFloat(s.MiterLimit)))
	} else {
		buf.WriteString("10 M\n") // PDF default
	}
	if len(s.DashPattern) > 0 {
		parts := make([]string, len(s.DashPattern))
		for i, d := range s.DashPattern {
			parts[i] = formatFloat(d)
		}
		buf.WriteString(fmt.Sprintf("[%s] %s d\n",
			strings.Join(parts, " "), formatFloat(s.DashPhase)))
	} else {
		buf.WriteString("[] 0 d\n")
	}
	c := Color{R: 0, G: 0, B: 0, A: 1}
	if s.Color != nil {
		c = *s.Color
	}
	buf.WriteString(fmt.Sprintf("%s %s %s RG\n",
		formatFloat(c.R), formatFloat(c.G), formatFloat(c.B)))
	return buf.String()
}

// DrawLine strokes a single line segment from→to with the given style.
// No-op if style.Width <= 0.
//
// Mirrors Aspose.PDF for .NET's Drawing.Line.
func (p *Page) DrawLine(from, to Point, style LineStyle) error {
	if style.Width <= 0 {
		return nil
	}
	var buf strings.Builder
	buf.WriteString("q\n")
	buf.WriteString(formatLineStyle(style))
	buf.WriteString(fmt.Sprintf("%s %s m\n", formatFloat(from.X), formatFloat(from.Y)))
	buf.WriteString(fmt.Sprintf("%s %s l\n", formatFloat(to.X), formatFloat(to.Y)))
	buf.WriteString("S\n")
	buf.WriteString("Q\n")
	return p.appendToContentStream([]byte(buf.String()))
}
```

`Point` is the existing type from `annotation_drawing.go`. `formatFloat` is the existing helper. `appendToContentStream` is the existing `(*Page)` method.

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestDrawLine_' -v ./...
go test ./...
git add vector_draw.go vector_test.go
git commit -m "feat: vector — (*Page).DrawLine + formatLineStyle helper"
```

---

## Task 6: `(*Page).DrawRectangle` + `formatShapeStyle` + `paintOp` helpers

This task adds the helpers for fill + stroke combination.

**Files:**
- Modify: `vector_draw.go`
- Modify: `vector_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestDrawRectangle_StrokeOnly(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawRectangle(
		pdf.Rectangle{LLX: 50, LLY: 50, URX: 150, URY: 100},
		pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1, Color: &pdf.Color{R: 0, G: 0, B: 1, A: 1}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	if !strings.Contains(s, "50 50 100 50 re") {
		t.Errorf("missing rect op: %s", s)
	}
	if !strings.Contains(s, " S\n") {
		t.Error("stroke-only should emit S")
	}
	if strings.Contains(s, " f\n") || strings.Contains(s, " B\n") {
		t.Error("stroke-only should not emit f or B")
	}
}

func TestDrawRectangle_FillOnly(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawRectangle(
		pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100},
		pdf.ShapeStyle{FillColor: &pdf.Color{R: 1, G: 1, B: 0, A: 1}},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	if !strings.Contains(s, "1 1 0 rg") {
		t.Errorf("missing fill color: %s", s)
	}
	if !strings.Contains(s, " f\n") {
		t.Error("fill-only should emit f")
	}
}

func TestDrawRectangle_StrokeAndFill(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawRectangle(
		pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100},
		pdf.ShapeStyle{
			LineStyle: pdf.LineStyle{Width: 1, Color: &pdf.Color{R: 1, G: 0, B: 0, A: 1}},
			FillColor: &pdf.Color{R: 0, G: 1, B: 0, A: 1},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	if !strings.Contains(s, " B\n") {
		t.Errorf("stroke+fill should emit B: %s", s)
	}
	if !strings.Contains(s, "1 0 0 RG") || !strings.Contains(s, "0 1 0 rg") {
		t.Error("both stroke and fill colors should be present")
	}
}

func TestDrawRectangle_NoStyleNoOp(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawRectangle(pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}, pdf.ShapeStyle{})
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	if strings.Contains(s, " re\n") {
		t.Error("empty ShapeStyle should produce no rectangle output")
	}
}
```

- [ ] **Step 2: Run + observe failure**

- [ ] **Step 3: Add helpers + DrawRectangle to `vector_draw.go`**

```go
// paintOp returns the PDF painting operator for the given style:
//   "S"  — stroke only
//   "f"  — fill only
//   "B"  — stroke + fill
//   ""   — neither (caller should skip emission entirely)
func paintOp(s ShapeStyle) string {
	stroke := s.LineStyle.Width > 0
	fill := s.FillColor != nil
	switch {
	case stroke && fill:
		return "B"
	case stroke:
		return "S"
	case fill:
		return "f"
	default:
		return ""
	}
}

// formatFillColor emits a fill-color (rg) op, or "" if color is nil.
func formatFillColor(c *Color) string {
	if c == nil {
		return ""
	}
	return fmt.Sprintf("%s %s %s rg\n",
		formatFloat(c.R), formatFloat(c.G), formatFloat(c.B))
}

// formatShapeStyle emits stroke + fill graphics state ops.
// Returns "" if neither stroke nor fill is configured.
func formatShapeStyle(s ShapeStyle) string {
	op := paintOp(s)
	if op == "" {
		return ""
	}
	var buf strings.Builder
	if s.LineStyle.Width > 0 {
		buf.WriteString(formatLineStyle(s.LineStyle))
	}
	buf.WriteString(formatFillColor(s.FillColor))
	return buf.String()
}

// DrawRectangle strokes and/or fills an axis-aligned rectangle.
// No-op if neither stroke (Width > 0) nor fill (FillColor != nil) is set.
//
// Mirrors Aspose.PDF for .NET's Drawing.Rectangle.
func (p *Page) DrawRectangle(rect Rectangle, style ShapeStyle) error {
	op := paintOp(style)
	if op == "" {
		return nil
	}
	w := rect.URX - rect.LLX
	h := rect.URY - rect.LLY
	var buf strings.Builder
	buf.WriteString("q\n")
	buf.WriteString(formatShapeStyle(style))
	buf.WriteString(fmt.Sprintf("%s %s %s %s re\n",
		formatFloat(rect.LLX), formatFloat(rect.LLY),
		formatFloat(w), formatFloat(h)))
	buf.WriteString(op + "\n")
	buf.WriteString("Q\n")
	return p.appendToContentStream([]byte(buf.String()))
}
```

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestDrawRectangle_' -v ./...
go test ./...
git add vector_draw.go vector_test.go
git commit -m "feat: vector — (*Page).DrawRectangle + formatShapeStyle/paintOp helpers"
```

---

## Task 7: `(*Page).DrawCircle` + `(*Page).DrawEllipse`

Both use the same 4-bezier approximation. Shared helper `ellipseBezier(cx, cy, rx, ry)` returns the path-construction operator string.

**Files:**
- Modify: `vector_draw.go`
- Modify: `vector_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestDrawCircle_StrokeOnlyEmitsFourBeziers(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawCircle(
		pdf.Point{X: 100, Y: 100}, 50,
		pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	curveCount := strings.Count(s, " c\n")
	if curveCount != 4 {
		t.Errorf("curve op count = %d, want 4", curveCount)
	}
	if !strings.Contains(s, " h\n") {
		t.Error("path should be closed (h)")
	}
}

func TestDrawCircle_NegativeRadiusErrors(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawCircle(pdf.Point{}, -1, pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}})
	if err == nil {
		t.Error("negative radius should error")
	}
}

func TestDrawEllipse_Basic(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawEllipse(
		pdf.Point{X: 100, Y: 100}, 80, 40,
		pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	curveCount := strings.Count(s, " c\n")
	if curveCount != 4 {
		t.Errorf("curve op count = %d, want 4", curveCount)
	}
}

func TestDrawEllipse_NegativeAxisErrors(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawEllipse(pdf.Point{}, -1, 1, pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}})
	if err == nil {
		t.Error("negative rx should error")
	}
	err = page.DrawEllipse(pdf.Point{}, 1, -1, pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}})
	if err == nil {
		t.Error("negative ry should error")
	}
}
```

- [ ] **Step 2: Run + observe failure**

- [ ] **Step 3: Implement in `vector_draw.go`**

```go
// ellipseApproxKappa is the magic constant for cubic Bezier approximation of
// a quarter-circle: k = (4/3)*tan(π/8) = 4*(√2 - 1)/3 ≈ 0.5522847498.
const ellipseApproxKappa = 0.5522847498307933

// ellipsePathOps emits the path-construction operators for an axis-aligned
// ellipse centered at (cx, cy) with horizontal radius rx and vertical radius
// ry. Composed of four cubic Beziers + close (h).
func ellipsePathOps(cx, cy, rx, ry float64) string {
	kx := rx * ellipseApproxKappa
	ky := ry * ellipseApproxKappa
	var buf strings.Builder
	// Start at right-most point.
	buf.WriteString(fmt.Sprintf("%s %s m\n",
		formatFloat(cx+rx), formatFloat(cy)))
	// Upper-right quadrant.
	buf.WriteString(fmt.Sprintf("%s %s %s %s %s %s c\n",
		formatFloat(cx+rx), formatFloat(cy+ky),
		formatFloat(cx+kx), formatFloat(cy+ry),
		formatFloat(cx), formatFloat(cy+ry)))
	// Upper-left.
	buf.WriteString(fmt.Sprintf("%s %s %s %s %s %s c\n",
		formatFloat(cx-kx), formatFloat(cy+ry),
		formatFloat(cx-rx), formatFloat(cy+ky),
		formatFloat(cx-rx), formatFloat(cy)))
	// Lower-left.
	buf.WriteString(fmt.Sprintf("%s %s %s %s %s %s c\n",
		formatFloat(cx-rx), formatFloat(cy-ky),
		formatFloat(cx-kx), formatFloat(cy-ry),
		formatFloat(cx), formatFloat(cy-ry)))
	// Lower-right.
	buf.WriteString(fmt.Sprintf("%s %s %s %s %s %s c\n",
		formatFloat(cx+kx), formatFloat(cy-ry),
		formatFloat(cx+rx), formatFloat(cy-ky),
		formatFloat(cx+rx), formatFloat(cy)))
	buf.WriteString("h\n")
	return buf.String()
}

// DrawCircle strokes and/or fills a circle. Returns error for negative radius.
func (p *Page) DrawCircle(center Point, radius float64, style ShapeStyle) error {
	if radius < 0 {
		return fmt.Errorf("draw circle: negative radius %g", radius)
	}
	return p.DrawEllipse(center, radius, radius, style)
}

// DrawEllipse strokes and/or fills an axis-aligned ellipse.
func (p *Page) DrawEllipse(center Point, rx, ry float64, style ShapeStyle) error {
	if rx < 0 || ry < 0 {
		return fmt.Errorf("draw ellipse: negative semi-axis (rx=%g, ry=%g)", rx, ry)
	}
	op := paintOp(style)
	if op == "" || rx == 0 || ry == 0 {
		return nil
	}
	var buf strings.Builder
	buf.WriteString("q\n")
	buf.WriteString(formatShapeStyle(style))
	buf.WriteString(ellipsePathOps(center.X, center.Y, rx, ry))
	buf.WriteString(op + "\n")
	buf.WriteString("Q\n")
	return p.appendToContentStream([]byte(buf.String()))
}
```

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestDraw(Circle|Ellipse)_' -v ./...
go test ./...
git add vector_draw.go vector_test.go
git commit -m "feat: vector — DrawCircle + DrawEllipse (4-bezier approximation, kappa=0.5522…)"
```

---

## Task 8: `(*Page).DrawPolyline` + `(*Page).DrawPolygon`

**Files:**
- Modify: `vector_draw.go`
- Modify: `vector_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestDrawPolyline_TwoPoints(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawPolyline(
		[]pdf.Point{{X: 0, Y: 0}, {X: 100, Y: 100}},
		pdf.LineStyle{Width: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	if !strings.Contains(s, "0 0 m") || !strings.Contains(s, "100 100 l") {
		t.Errorf("missing polyline path ops: %s", s)
	}
	if !strings.Contains(s, " S\n") {
		t.Error("polyline should stroke (open path)")
	}
	if strings.Contains(s, " h\n") || strings.Contains(s, " B\n") || strings.Contains(s, " f\n") {
		t.Error("polyline should not close or fill")
	}
}

func TestDrawPolyline_OnePointErrors(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawPolyline([]pdf.Point{{X: 0, Y: 0}}, pdf.LineStyle{Width: 1})
	if err == nil {
		t.Error("polyline with one point should error")
	}
}

func TestDrawPolygon_Triangle(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawPolygon(
		[]pdf.Point{{X: 0, Y: 0}, {X: 100, Y: 0}, {X: 50, Y: 87}},
		pdf.ShapeStyle{
			LineStyle: pdf.LineStyle{Width: 1},
			FillColor: &pdf.Color{R: 0, G: 1, B: 0, A: 1},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	lineCount := strings.Count(s, " l\n")
	if lineCount < 2 {
		t.Errorf("triangle should have ≥ 2 line ops, got %d", lineCount)
	}
	if !strings.Contains(s, " h\n") {
		t.Error("polygon should close (h)")
	}
	if !strings.Contains(s, " B\n") {
		t.Error("polygon with stroke+fill should emit B")
	}
}

func TestDrawPolygon_TwoPointsErrors(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawPolygon(
		[]pdf.Point{{X: 0, Y: 0}, {X: 100, Y: 100}},
		pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}},
	)
	if err == nil {
		t.Error("polygon with two points should error")
	}
}
```

- [ ] **Step 2: Run + observe failure**

- [ ] **Step 3: Implement**

```go
// DrawPolyline strokes an open polyline (first and last points are NOT
// connected). No fill — even if you wanted one, an open path has ambiguous
// fill semantics. Errors if len(points) < 2 or style.Width <= 0.
func (p *Page) DrawPolyline(points []Point, style LineStyle) error {
	if len(points) < 2 {
		return fmt.Errorf("draw polyline: need >= 2 points, got %d", len(points))
	}
	if style.Width <= 0 {
		return nil
	}
	var buf strings.Builder
	buf.WriteString("q\n")
	buf.WriteString(formatLineStyle(style))
	buf.WriteString(fmt.Sprintf("%s %s m\n", formatFloat(points[0].X), formatFloat(points[0].Y)))
	for _, pt := range points[1:] {
		buf.WriteString(fmt.Sprintf("%s %s l\n", formatFloat(pt.X), formatFloat(pt.Y)))
	}
	buf.WriteString("S\n")
	buf.WriteString("Q\n")
	return p.appendToContentStream([]byte(buf.String()))
}

// DrawPolygon strokes and/or fills a closed polygon (last point connects back
// to the first via `h`). Errors if len(points) < 3.
func (p *Page) DrawPolygon(points []Point, style ShapeStyle) error {
	if len(points) < 3 {
		return fmt.Errorf("draw polygon: need >= 3 points, got %d", len(points))
	}
	op := paintOp(style)
	if op == "" {
		return nil
	}
	var buf strings.Builder
	buf.WriteString("q\n")
	buf.WriteString(formatShapeStyle(style))
	buf.WriteString(fmt.Sprintf("%s %s m\n", formatFloat(points[0].X), formatFloat(points[0].Y)))
	for _, pt := range points[1:] {
		buf.WriteString(fmt.Sprintf("%s %s l\n", formatFloat(pt.X), formatFloat(pt.Y)))
	}
	buf.WriteString("h\n")
	buf.WriteString(op + "\n")
	buf.WriteString("Q\n")
	return p.appendToContentStream([]byte(buf.String()))
}
```

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestDraw(Polyline|Polygon)_' -v ./...
go test ./...
git add vector_draw.go vector_test.go
git commit -m "feat: vector — DrawPolyline (open, stroke) + DrawPolygon (closed, fill+stroke)"
```

---

## Task 9: `(*Page).DrawPath` + `pathOpsToOperators` helper

**Files:**
- Modify: `vector_draw.go`
- Modify: `vector_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestDrawPath_NilErrors(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawPath(nil, pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}})
	if err == nil {
		t.Error("nil path should error")
	}
}

func TestDrawPath_EmptyPathNoOp(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawPath(pdf.NewPath(), pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}})
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	if strings.Contains(s, " m\n") {
		t.Error("empty path should emit nothing")
	}
}

func TestDrawPath_BuilderChain(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	path := pdf.NewPath().MoveTo(50, 50).LineTo(150, 50).CurveTo(180, 80, 180, 120, 150, 150).Close()
	err := page.DrawPath(path, pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 2}})
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	for _, want := range []string{"50 50 m", " l\n", " c\n", " h\n", " S\n"} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Run + observe failure**

- [ ] **Step 3: Implement**

```go
// pathOpsToOperators converts Path's internal ops into a PDF content stream
// fragment of path-construction operators (m, l, c, h). Does NOT emit a
// paint operator — the caller appends "S", "f", or "B" as appropriate.
func pathOpsToOperators(ops []pathOp) string {
	var buf strings.Builder
	for _, op := range ops {
		switch op.kind {
		case pathOpMoveTo:
			buf.WriteString(fmt.Sprintf("%s %s m\n", formatFloat(op.x), formatFloat(op.y)))
		case pathOpLineTo:
			buf.WriteString(fmt.Sprintf("%s %s l\n", formatFloat(op.x), formatFloat(op.y)))
		case pathOpCurveTo:
			buf.WriteString(fmt.Sprintf("%s %s %s %s %s %s c\n",
				formatFloat(op.c1x), formatFloat(op.c1y),
				formatFloat(op.c2x), formatFloat(op.c2y),
				formatFloat(op.x), formatFloat(op.y)))
		case pathOpClose:
			buf.WriteString("h\n")
		}
	}
	return buf.String()
}

// DrawPath strokes and/or fills the previously-built path. Errors if path is
// nil. No-op if path has no operations or style is empty.
func (p *Page) DrawPath(path *Path, style ShapeStyle) error {
	if path == nil {
		return fmt.Errorf("draw path: nil path")
	}
	if len(path.ops) == 0 {
		return nil
	}
	op := paintOp(style)
	if op == "" {
		return nil
	}
	var buf strings.Builder
	buf.WriteString("q\n")
	buf.WriteString(formatShapeStyle(style))
	buf.WriteString(pathOpsToOperators(path.ops))
	buf.WriteString(op + "\n")
	buf.WriteString("Q\n")
	return p.appendToContentStream([]byte(buf.String()))
}
```

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestDrawPath_' -v ./...
go test ./...
git add vector_draw.go vector_test.go
git commit -m "feat: vector — (*Page).DrawPath + pathOpsToOperators helper"
```

---

## Task 10: `(*Page).DrawRoundedRectangle`

Uses `Path` internally — straight edges + Arc on each corner.

**Files:**
- Modify: `vector_draw.go`
- Modify: `vector_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestDrawRoundedRectangle_Basic(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawRoundedRectangle(
		pdf.Rectangle{LLX: 50, LLY: 50, URX: 200, URY: 150}, 10,
		pdf.ShapeStyle{
			LineStyle: pdf.LineStyle{Width: 1},
			FillColor: &pdf.Color{R: 0.9, G: 0.9, B: 0.9, A: 1},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	// 4 straight edges (l) + 4 corner arcs (c) + close (h).
	if strings.Count(s, " l\n") < 3 {
		t.Errorf("expected >=3 line ops for edges, got %d", strings.Count(s, " l\n"))
	}
	if strings.Count(s, " c\n") < 4 {
		t.Errorf("expected >=4 curve ops for corners, got %d", strings.Count(s, " c\n"))
	}
}

func TestDrawRoundedRectangle_NegativeRadiusErrors(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawRoundedRectangle(
		pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}, -5,
		pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}},
	)
	if err == nil {
		t.Error("negative radius should error")
	}
}

func TestDrawRoundedRectangle_LargeRadiusClampedToHalfShorterSide(t *testing.T) {
	// Rect 100×40, radius 50 → clamped to 20 (half of shorter side).
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawRoundedRectangle(
		pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 40}, 50,
		pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}},
	)
	if err != nil {
		t.Fatal(err)
	}
	// No assertion on exact output — just verify it doesn't error.
}
```

- [ ] **Step 2: Run + observe failure**

- [ ] **Step 3: Implement**

```go
// DrawRoundedRectangle strokes and/or fills an axis-aligned rectangle with
// rounded corners of the given radius. The radius is clamped to half the
// shorter side. Returns error for negative radius.
//
// Implemented as a Path: 4 straight edges + 4 quarter-arc corners.
func (p *Page) DrawRoundedRectangle(rect Rectangle, radius float64, style ShapeStyle) error {
	if radius < 0 {
		return fmt.Errorf("draw rounded rectangle: negative radius %g", radius)
	}
	op := paintOp(style)
	if op == "" {
		return nil
	}
	w := rect.URX - rect.LLX
	h := rect.URY - rect.LLY
	if w <= 0 || h <= 0 {
		return nil
	}
	r := radius
	if maxR := w / 2; r > maxR {
		r = maxR
	}
	if maxR := h / 2; r > maxR {
		r = maxR
	}

	// Build the path:
	//   Start at (LLX+r, LLY)
	//   Line to (URX-r, LLY)
	//   Arc bottom-right (center (URX-r, LLY+r), 270° → 360°)
	//   Line to (URX, URY-r)
	//   Arc top-right (center (URX-r, URY-r), 0° → 90°)
	//   Line to (LLX+r, URY)
	//   Arc top-left (center (LLX+r, URY-r), 90° → 180°)
	//   Line to (LLX, LLY+r)
	//   Arc bottom-left (center (LLX+r, LLY+r), 180° → 270°)
	//   Close
	const (
		halfPi    = 1.5707963267948966
		threeHalf = 4.71238898038469
		twoPi     = 6.283185307179586
	)
	_ = twoPi

	path := NewPath().
		MoveTo(rect.LLX+r, rect.LLY).
		LineTo(rect.URX-r, rect.LLY).
		Arc(rect.URX-r, rect.LLY+r, r, threeHalf, halfPi). // 270→360 (+90°)
		LineTo(rect.URX, rect.URY-r).
		Arc(rect.URX-r, rect.URY-r, r, 0, halfPi). // 0→90
		LineTo(rect.LLX+r, rect.URY).
		Arc(rect.LLX+r, rect.URY-r, r, halfPi, halfPi). // 90→180
		LineTo(rect.LLX, rect.LLY+r).
		Arc(rect.LLX+r, rect.LLY+r, r, 2*halfPi, halfPi). // 180→270
		Close()

	return p.DrawPath(path, style)
}
```

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestDrawRoundedRectangle_' -v ./...
go test ./...
git add vector_draw.go vector_test.go
git commit -m "feat: vector — DrawRoundedRectangle (Path-based, clamps radius to half-shorter-side)"
```

---

## Task 11: Alpha (transparency) via `ExtGState`

Wire `Color.A < 1` into `formatLineStyle` and `formatFillColor` so semi-transparent strokes/fills work.

**Files:**
- Modify: `vector_draw.go`
- Modify: `vector_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestDrawLine_AlphaUsesExtGState(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawLine(
		pdf.Point{}, pdf.Point{X: 100},
		pdf.LineStyle{
			Width: 2,
			Color: &pdf.Color{R: 1, G: 0, B: 0, A: 0.5},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	// Should reference an /ExtGState alias and emit `gs` op.
	if !strings.Contains(s, " gs\n") {
		t.Errorf("alpha < 1 should emit gs op: %s", s)
	}
}

func TestDrawRectangle_FillAlphaUsesExtGState(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawRectangle(
		pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100},
		pdf.ShapeStyle{FillColor: &pdf.Color{R: 0, G: 0, B: 1, A: 0.3}},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := renderedContent(t, doc)
	if !strings.Contains(s, " gs\n") {
		t.Errorf("fill alpha < 1 should emit gs op: %s", s)
	}
}

func TestDrawLine_FullOpacityNoExtGState(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	_ = page.DrawLine(
		pdf.Point{}, pdf.Point{X: 100},
		pdf.LineStyle{Width: 1, Color: &pdf.Color{R: 0, G: 0, B: 0, A: 1}},
	)
	s := renderedContent(t, doc)
	if strings.Contains(s, " gs\n") {
		t.Error("alpha = 1 should not emit gs (no transparency needed)")
	}
}
```

- [ ] **Step 2: Run + observe failure**

`Color.A < 1` currently has no effect.

- [ ] **Step 3: Wire ExtGState into draw methods**

Each draw method needs to:
1. Before emitting `q`, register any required ExtGState aliases (one per unique alpha value).
2. Emit the `gs` op after color setup, inside the q-block.

The existing helper `(*Page).ensureExtGState(alpha float64) (string, error)` (from `text_add.go`) registers an ExtGState resource for the given alpha value and returns the resource name (e.g., `/GS1`).

But — and this is important — `ensureExtGState` uses `/CA` for the stroke-alpha resource ONLY. For fill alpha (`/ca`), there's no helper yet OR it's the same one. Read `text_add.go` to confirm. If only stroke alpha is supported, we need to add a fill counterpart (or extend the existing helper).

Read `text_add.go` around `ensureExtGState` to see the current signature and what alpha it handles. The plan implementer may need to either:
- Use the same `ensureExtGState` for both /CA and /ca (if it returns a resource with both).
- Add a `ensureExtGStateStrokeAndFill(alpha)` or similar pair.

**Implementer choice:** read the existing helper, pick the minimal extension. If `ensureExtGState` already sets `/CA = α` AND `/ca = α` in the same resource (likely), one call covers both stroke and fill.

For the draw methods:

```go
// In DrawLine:
if style.Color != nil && style.Color.A < 1 {
    gsName, err := p.ensureExtGState(style.Color.A)
    if err != nil {
        return err
    }
    buf.WriteString(fmt.Sprintf("%s gs\n", gsName))
}
```

And similar in DrawRectangle / DrawCircle / DrawEllipse / etc. — for stroke OR fill, register the alpha of the relevant color.

Refactor: extract a small helper inside `vector_draw.go`:

```go
// applyAlpha registers an ExtGState if either the stroke color (in style.Color)
// or fill color has alpha < 1, and returns the gs op string. Returns "" if no
// transparency is needed.
//
// Note: if both stroke and fill have different alphas, this returns two gs
// ops (each in its own line). The PDF gstate stack will apply both — the
// stroke gs reference applies its /CA and the fill gs reference applies /ca.
// We rely on ensureExtGState producing per-alpha resources with BOTH /CA and
// /ca set to the same value (simpler — both stroke and fill at the same alpha
// inside one shape).
//
// Limitation: distinct stroke and fill alpha values are NOT supported in
// Phase 1. If both colors have alpha < 1 but with different values, the fill
// alpha takes precedence. Document this; Phase 2/3 can split if needed.
func (p *Page) applyAlpha(strokeColor, fillColor *Color) (string, error) {
    // Pick the most-restrictive alpha actually in use.
    a := 1.0
    if strokeColor != nil && strokeColor.A < a {
        a = strokeColor.A
    }
    if fillColor != nil && fillColor.A < a {
        a = fillColor.A
    }
    if a >= 1 {
        return "", nil
    }
    name, err := p.ensureExtGState(a)
    if err != nil {
        return "", err
    }
    return fmt.Sprintf("%s gs\n", name), nil
}
```

Then in each draw method, call `applyAlpha` after the `q\n` and before the style emission:

```go
buf.WriteString("q\n")
gsOp, err := p.applyAlpha(style.Color, nil)  // for DrawLine: only stroke
if err != nil {
    return err
}
buf.WriteString(gsOp)
buf.WriteString(formatLineStyle(style))
// ...
```

For DrawRectangle / etc., pass both stroke and fill.

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestDraw(Line|Rectangle)_(Alpha|FullOpacity)' -v ./...
go test ./...
git add vector_draw.go vector_test.go
git commit -m "feat: vector — alpha via ExtGState (semi-transparent stroke + fill)"
```

---

## Task 12: Aspose .NET parity tests

**Files:**
- Modify: `vector_test.go`

- [ ] **Step 1: Append tests**

```go
// Aspose .NET sample:
//   Graph graph = new Graph(width, height);
//   Line line = new Line(new float[] {x1, y1, x2, y2});
//   line.GraphInfo.Color = Color.Red;
//   line.GraphInfo.LineWidth = 2;
//   line.GraphInfo.DashArray = new int[] {4, 2};
//   graph.Shapes.Add(line);
//   page.Paragraphs.Add(graph);
//
// In this library — methods directly on Page (no Graph container needed):
func TestAsposeParity_DrawLineWithDash(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawLine(
		pdf.Point{X: 50, Y: 50}, pdf.Point{X: 200, Y: 200},
		pdf.LineStyle{
			Color:       &pdf.Color{R: 1, G: 0, B: 0, A: 1},
			Width:       2,
			DashPattern: []float64{4, 2},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
}

// Aspose .NET sample:
//   Circle circle = new Circle(cx, cy, radius);
//   circle.GraphInfo.Color = Color.Blue;
//   circle.GraphInfo.FillColor = Color.LightBlue;
//   circle.GraphInfo.LineWidth = 1;
//   graph.Shapes.Add(circle);
func TestAsposeParity_DrawCircleWithFill(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	err := page.DrawCircle(
		pdf.Point{X: 200, Y: 200}, 50,
		pdf.ShapeStyle{
			LineStyle: pdf.LineStyle{
				Color: &pdf.Color{R: 0, G: 0, B: 1, A: 1},
				Width: 1,
			},
			FillColor: &pdf.Color{R: 0.7, G: 0.9, B: 1, A: 1},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
}

// Aspose .NET sample (arbitrary path via line segments):
//   Curve curve = new Curve(new float[] {p0x, p0y, c1x, c1y, c2x, c2y, p3x, p3y});
//   graph.Shapes.Add(curve);
func TestAsposeParity_DrawPathArbitrary(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	path := pdf.NewPath().
		MoveTo(50, 50).
		CurveTo(100, 0, 200, 100, 250, 50).
		LineTo(300, 100).
		Close()
	err := page.DrawPath(path, pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}})
	if err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run + commit**

```powershell
go test -run 'TestAsposeParity_Draw' -v ./...
go test ./...
git add vector_test.go
git commit -m "test: Aspose .NET parity for vector — DrawLine/DrawCircle/DrawPath"
```

---

## Task 13: Cross-cutting — AES-128 + multi-page

**Files:**
- Modify: `vector_test.go`

- [ ] **Step 1: Append tests**

```go
func TestVector_AES128Roundtrip(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	page, _ := doc.Page(1)
	_ = page.DrawCircle(pdf.Point{X: 100, Y: 100}, 50, pdf.ShapeStyle{
		LineStyle: pdf.LineStyle{Width: 2, Color: &pdf.Color{R: 1, G: 0, B: 0, A: 1}},
	})
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword: "x",
		Algorithm:    pdf.EncryptionAlgAES128,
	})

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	doc2, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "x")
	if err != nil {
		t.Fatal(err)
	}
	page2, _ := doc2.Page(1)
	if _, err := page2.ExtractText(); err != nil {
		t.Fatal(err) // Page must at least be parseable.
	}
}

func TestVector_MultiplePages(t *testing.T) {
	doc := pdf.NewDocument(595, 842)
	if err := doc.AddBlankPage(595, 842); err != nil {
		t.Fatal(err)
	}
	if err := doc.AddBlankPage(595, 842); err != nil {
		t.Fatal(err)
	}

	for i := 1; i <= doc.PageCount(); i++ {
		page, _ := doc.Page(i)
		_ = page.DrawCircle(
			pdf.Point{X: 100, Y: float64(100 + i*50)}, 30,
			pdf.ShapeStyle{LineStyle: pdf.LineStyle{Width: 1}},
		)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if doc2.PageCount() != 3 {
		t.Errorf("PageCount after roundtrip = %d, want 3", doc2.PageCount())
	}
}
```

- [ ] **Step 2: Run + commit**

```powershell
go test -run 'TestVector_(AES128|MultiplePages)' -v ./...
go test ./...
git add vector_test.go
git commit -m "test: vector — AES-128 roundtrip + multi-page rendering"
```

---

## Task 14: Demo update — add a vector chart to `full_scenario`

Add a small bar chart on a new page or as part of the existing sales report page. The chart visually demonstrates `DrawRectangle` (bars) + `DrawLine` (axes) + `DrawCircle` (data point markers) + `DrawText` (labels — uses existing `AddText`).

**Files:**
- Modify: `my_examples/full_scenario/main.go`

- [ ] **Step 1: Pick a location**

The sales report on page 6 has a table that overflows. After the table, on the LAST page used by the table (a continuation page from overflow), there's likely empty space. Alternative: add a separate page 7 (or whatever the next-after-sales-report number is) just for a chart.

**Recommended:** Add a separate function `addVectorShowcase(page *pdf.Page)` and call it on a NEW page that the example explicitly adds. Cleaner than weaving into the existing overflow result.

Update the page count from 5 to 6+1=7 initial pages (sales overflow may add more on top).

Or simpler — render the chart on the SAME page as the sales-report intro (page 6), above the table. Just position carefully.

For Phase 1's demo: **add a small "monthly trend" mini-chart at the top of page 6**, above the table title. This is the cleanest insertion point and shows vector + table on the same page.

Actually even simpler — **add a new "Vector Graphics Showcase" page after the sales report**. The example becomes 7 explicit pages (one of which is the multi-page sales report). Let me write it as a new page.

Concrete steps:
1. Update the page-creation loop to add 6 additional pages (total 7 explicit pages).
2. Add `page7, _ := doc.Page(7)`.
3. Add `addVectorShowcase(doc, page7)` call between `addSalesReport` and the watermark loop.
4. Update `addBookmarks` signature to take 7 pages, add a "Vector Showcase" bookmark.
5. Implement `addVectorShowcase(doc, page)` rendering some demo content.

For the showcase, demonstrate:
- Title text (use existing AddText)
- Bar chart: a few rectangles of varying heights with colored fill
- Axis lines via DrawLine (with dash pattern showing dashed style)
- A circle indicating an "important data point"
- A rounded-rectangle "callout box" with text label
- A polygon (e.g., a triangular alert marker)
- An arc (e.g., a pie-slice marker)

Use `Path` for the pie slice:
```go
slice := pdf.NewPath().MoveTo(cx, cy).LineTo(cx+r, cy).Arc(cx, cy, r, 0, math.Pi/4).Close()
```

- [ ] **Step 2: Implement `addVectorShowcase`**

Sketch:

```go
func addVectorShowcase(doc *pdf.Document, page *pdf.Page) {
	size, _ := page.Size()

	// Title.
	mustText(page.AddText("Vector Graphics Showcase",
		pdf.TextStyle{
			Font: pdf.FontHelveticaBold, Size: 22,
			Color:  &pdf.Color{R: 0.1, G: 0.15, B: 0.4, A: 1},
			HAlign: pdf.HAlignCenter,
		},
		pdf.Rectangle{LLX: 50, LLY: size.Height - 88, URX: size.Width - 50, URY: size.Height - 55}))

	mustText(page.AddText("DrawLine • DrawRectangle • DrawCircle • DrawPolygon • DrawPath • Arc",
		pdf.TextStyle{
			Font: pdf.FontHelveticaOblique, Size: 11,
			Color:  &pdf.Color{R: 0.4, G: 0.4, B: 0.4, A: 1},
			HAlign: pdf.HAlignCenter,
		},
		pdf.Rectangle{LLX: 50, LLY: size.Height - 113, URX: size.Width - 50, URY: size.Height - 98}))

	// === Bar chart ===
	// 7 monthly bars with varying heights.
	bars := []struct {
		label  string
		height float64
		color  *pdf.Color
	}{
		{"Jan", 70, ...},
		// ...
	}
	// ... draw axis (DrawLine with dash), then bars (DrawRectangle), labels (AddText) ...

	// === Highlight: filled circle on the best bar ===
	if err := page.DrawCircle(pdf.Point{X: bestX, Y: bestY}, 6,
		pdf.ShapeStyle{
			LineStyle: pdf.LineStyle{Width: 0},
			FillColor: &pdf.Color{R: 1, G: 0.5, B: 0, A: 1},
		},
	); err != nil { log.Fatal(err) }

	// === Callout box: rounded rect with text ===
	page.DrawRoundedRectangle(pdf.Rectangle{...}, 8, pdf.ShapeStyle{
		LineStyle: pdf.LineStyle{Width: 1.5, Color: navy},
		FillColor: &pdf.Color{R: 0.95, G: 0.97, B: 1, A: 1},
	})
	page.AddText("Peak month: Mar", ...)

	// === Triangle alert + arc pie slice + polygon ===
	page.DrawPolygon([]pdf.Point{{X: 100, Y: 100}, {X: 130, Y: 100}, {X: 115, Y: 130}}, ...)

	piePath := pdf.NewPath().MoveTo(cx, cy).LineTo(cx+r, cy).Arc(cx, cy, r, 0, math.Pi/3).Close()
	page.DrawPath(piePath, pdf.ShapeStyle{
		LineStyle: pdf.LineStyle{Width: 1, Color: navy},
		FillColor: &pdf.Color{R: 0.9, G: 0.7, B: 0.3, A: 1},
	})

	// === Footer ===
	mustText(page.AddText("Page 7 — Vector Graphics", ...))
}
```

Use realistic data (don't over-engineer). The point is to visually exercise each draw method at least once.

Also update:
- Page-creation loop: from 5 → 6 additional pages.
- `page7, _ := doc.Page(7)`.
- Call `addVectorShowcase(doc, page7)` between `addSalesReport(doc, page6)` and the watermark loop.
- `addBookmarks` signature + add a "Vector Showcase" bookmark.
- File header docstring: mention the vector showcase page.

- [ ] **Step 3: Verify**

```powershell
go run ./my_examples/full_scenario
```

`my_examples/` is gitignored — no commit.

---

## Task 15: Docs + close beads issue

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`

- [ ] **Step 1: Update CLAUDE.md**

Add a new section. Find the existing `**\`text_add.go\`**` block (or similar) — vector goes BEFORE it (alphabetical) or right after it (conceptually adjacent: text drawing + vector drawing on page).

Pick "right after text_add" placement. Insert:

```markdown
**`vector.go` / `vector_draw.go`**
- `LineCap` enum — `LineCapButt` (default), `LineCapRound`, `LineCapSquare`. PDF operator J.
- `LineJoin` enum — `LineJoinMiter` (default), `LineJoinRound`, `LineJoinBevel`. PDF operator j.
- `LineStyle` struct — `Color *Color`, `Width float64`, `DashPattern []float64`, `DashPhase float64`, `Cap`, `Join`, `MiterLimit float64`. Width ≤ 0 → no stroke. Mirrors Aspose.PDF for .NET's GraphInfo stroke fields.
- `ShapeStyle` struct — embeds `LineStyle` + adds `FillColor *Color`. Either or both may be configured; if neither, draw call is a no-op.
- `Path` — opaque fluent builder. `NewPath().MoveTo(x, y).LineTo(x, y).CurveTo(c1x, c1y, c2x, c2y, x, y).QuadTo(cx, cy, x, y).Arc(cx, cy, r, startAngle, sweepAngle).Close()`. Arc decomposes into ≤4 cubic Beziers per the Goldapp formula.
- `(*Page).DrawLine(from, to Point, style LineStyle) error` — single line segment.
- `(*Page).DrawRectangle(rect Rectangle, style ShapeStyle) error` — axis-aligned rect, stroke and/or fill.
- `(*Page).DrawRoundedRectangle(rect Rectangle, radius float64, style ShapeStyle) error` — radius auto-clamped to half-shorter-side.
- `(*Page).DrawCircle(center Point, radius float64, style ShapeStyle) error` — 4-Bezier approximation.
- `(*Page).DrawEllipse(center Point, rx, ry float64, style ShapeStyle) error` — axis-aligned ellipse.
- `(*Page).DrawPolyline(points []Point, style LineStyle) error` — open path, stroke-only. Errors if len(points) < 2.
- `(*Page).DrawPolygon(points []Point, style ShapeStyle) error` — closed path, stroke and/or fill. Errors if len(points) < 3.
- `(*Page).DrawPath(path *Path, style ShapeStyle) error` — arbitrary path. Errors on nil path.
- Alpha (`Color.A < 1`) for stroke and fill is rendered via the existing ExtGState infrastructure (`ensureExtGState`). Distinct stroke vs. fill alphas in the same shape: takes the more-restrictive value (single ExtGState per draw call). For per-property alpha precision, use separate draw calls.
- Coordinates are PDF user space (Y up, origin at page bottom-left). Drawing outside the page is allowed; PDF viewers clip to MediaBox.
- Phase 2 will add SVG embedding (`(*Page).AddSVG`); Phase 3 will add gradients, embedded raster in SVG, text matching, etc.
```

- [ ] **Step 2: Update README.md**

Append to the Features bullet list (find existing bullets like "Tables", "Outlines"):

```markdown
- **Vector graphics** — `(*Page).DrawLine / DrawRectangle / DrawRoundedRectangle / DrawCircle / DrawEllipse / DrawPolyline / DrawPolygon / DrawPath` for first-class vector content on PDF pages. `Path` fluent builder with `MoveTo/LineTo/CurveTo/QuadTo/Arc/Close`. `LineStyle` + `ShapeStyle` (color, width, dash pattern, line caps, line joins, alpha). Mirrors Aspose.PDF for .NET's `Graph`/`Shape` model but exposed directly on Page (no container) and Go-idiomatic
```

Add a code snippet — find the place where Tables/Annotations sections are, add a new `### Vector graphics` section:

```markdown
### Vector graphics

```go
doc := pdf.NewDocument(595, 842)
page, _ := doc.Page(1)

// Stroke a dashed red line.
page.DrawLine(
    pdf.Point{X: 50, Y: 700}, pdf.Point{X: 545, Y: 700},
    pdf.LineStyle{
        Color:       &pdf.Color{R: 1, G: 0, B: 0, A: 1},
        Width:       2,
        DashPattern: []float64{6, 3},
    },
)

// Fill a rounded box with semi-transparent blue.
page.DrawRoundedRectangle(
    pdf.Rectangle{LLX: 100, LLY: 500, URX: 400, URY: 600}, 10,
    pdf.ShapeStyle{
        LineStyle: pdf.LineStyle{Width: 1, Color: &pdf.Color{R: 0, G: 0, B: 0.5, A: 1}},
        FillColor: &pdf.Color{R: 0.6, G: 0.8, B: 1, A: 0.5},
    },
)

// Custom path: triangle with bezier corner.
path := pdf.NewPath().
    MoveTo(200, 300).
    LineTo(400, 300).
    CurveTo(420, 320, 420, 360, 400, 380).
    LineTo(200, 380).
    Close()
page.DrawPath(path, pdf.ShapeStyle{
    LineStyle: pdf.LineStyle{Width: 1.5},
    FillColor: &pdf.Color{R: 1, G: 0.9, B: 0.3, A: 1},
})

doc.Save("shapes.pdf")
```
```

(Use real triple backticks in README — these are escaped only inside this prompt.)

- [ ] **Step 3: Run + vet + commit**

```powershell
go test ./...
go vet ./...
git add CLAUDE.md README.md
git commit -m "docs: vector graphics Phase 1 (drawing primitives) in CLAUDE.md and README"
```

- [ ] **Step 4: Close beads issue**

```powershell
bd update pdf-go-5pq --status closed --append-notes "Vector graphics Phase 1 shipped 2026-05-20. Public API: LineCap/LineJoin enums; LineStyle/ShapeStyle; Path builder (MoveTo/LineTo/CurveTo/QuadTo/Arc/Close). Page methods: DrawLine/DrawRectangle/DrawRoundedRectangle/DrawCircle/DrawEllipse/DrawPolyline/DrawPolygon/DrawPath. Alpha via ExtGState. Aspose .NET parity (Graph/Shape model, but exposed directly on Page). AES-128 + multi-page verified. Foundation for Phase 2 (SVG-lite embedding). Out of Phase 1 scope: transforms, gradients, patterns, clipping path API, SVG parsing."
```

Report the exact bd output.

## Self-review

**Spec coverage:**

| Spec section | Task(s) |
|---|---|
| LineCap/LineJoin enums + LineStyle/ShapeStyle | 1 |
| Path with MoveTo/LineTo/Close | 2 |
| Path with CurveTo/QuadTo | 3 |
| Path with Arc | 4 |
| DrawLine + formatLineStyle | 5 |
| DrawRectangle + formatShapeStyle + paintOp | 6 |
| DrawCircle + DrawEllipse | 7 |
| DrawPolyline + DrawPolygon | 8 |
| DrawPath + pathOpsToOperators | 9 |
| DrawRoundedRectangle | 10 |
| Alpha via ExtGState | 11 |
| Aspose parity | 12 |
| Cross-cutting (AES + multi-page) | 13 |
| Demo update | 14 |
| Docs + close bd | 15 |

**Placeholder scan:** every task has concrete code and exact commit messages. The demo update (Task 14) is sketched — implementer needs to fill in coordinate calculations for the bar chart, but the structure is explicit.

**Type consistency:** `LineStyle`/`ShapeStyle` used identically across all draw methods. `Path` ops are internal and immutable after build. `Point`/`Rectangle`/`Color` reused from existing files. Helper signatures (`formatLineStyle`, `formatShapeStyle`, `paintOp`, `pathOpsToOperators`, `applyAlpha`) introduced incrementally and consumed in subsequent tasks.

**Estimated total:** ~15 tasks × 25–40 min each = 6–9 hours of focused work. Production code ~500 LOC. Tests ~600 LOC. Docs ~60 LOC.
