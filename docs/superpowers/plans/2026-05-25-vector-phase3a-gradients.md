# Vector Graphics Phase 3a Implementation Plan — SVG Gradients

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `<linearGradient>` / `<radialGradient>` rendering to the SVG pipeline. All work internal — no public API changes. Existing `(*Page).AddSVG` automatically produces gradient-filled PDFs from gradient-containing SVGs (Aspose logo's colored arcs in particular).

**Architecture:** Parser collects gradient definitions into `SVG.gradients` map (id → gradient). At render time, `fill="url(#id)"` resolved to a gradient → built into PDF Type 2 (axial) or Type 3 (radial) shading pattern → registered as `/Pattern Px` resource on page → fill emits `/Pattern cs` + `/Px scn` instead of plain RGB. Multi-stop gradients use PDF Type 3 stitching function combining Type 2 exponential interpolations.

**Tech Stack:** Go 1.24, standard library only.

**Reference:** [docs/superpowers/specs/2026-05-25-vector-phase3a-gradients-design.md](../specs/2026-05-25-vector-phase3a-gradients-design.md)

**Beads:** [pdf-go-mi3](bd show pdf-go-mi3) (Phase 3a) under umbrella [pdf-go-ybu](bd show pdf-go-ybu).

---

## File Map

| File | Purpose |
|---|---|
| `svg_gradient.go` (new) | IR types: `svgGradient` interface, `svgLinearGradient`, `svgRadialGradient`, `svgGradientStop`, `svgGradientUnits`, `svgSpreadMethod`, `svgPaint` (tagged union of color/grad-ref) |
| `svg_parse_gradient.go` (new) | XML parsers for `<linearGradient>` / `<radialGradient>` / `<stop>` / `<defs>` walker |
| `svg_render_gradient.go` (new) | PDF shading pattern emission: `buildShadingFunction`, `gradientToShadingObject`, `(*Page).ensurePatternResource` |
| `svg_types.go` (modify) | Add `gradients map[string]svgGradient` to `SVG`; replace `svgStyle.fill/stroke` from `*Color` to `*svgPaint` |
| `svg_attrs.go` (modify) | Extend `parseSVGColor` to recognize `url(#id)` — but it can't return `*svgPaint`, so introduce `parseSVGPaint` returning `*svgPaint`; keep `parseSVGColor` for non-fill/stroke usages |
| `svg_parse.go` (modify) | Walker recognizes `<defs>`, `<linearGradient>`, `<radialGradient>`; `applySingleSVGStyleProp` for `fill`/`stroke` uses `parseSVGPaint` |
| `svg_render.go` (modify) | `svgStyleToShapeStyle` / `svgStyleToLineStyle` resolve gradient refs; new helper `paintToPDFOps` emits either color setter or pattern setter |
| `svg_test.go` / `svg_parse_gradient_test.go` / `svg_render_gradient_test.go` (tests) | Coverage for parsing, function building, integration |
| `CLAUDE.md` / `README.md` | Phase 3a updates |

---

## Task 1: Gradient IR types + svgPaint refactor

**Files:**
- Create: `svg_gradient.go`
- Modify: `svg_types.go`

### Step 1: Create `svg_gradient.go`

```go
// SPDX-License-Identifier: MIT

package asposepdf

// svgGradient is the interface for both linear and radial gradients.
type svgGradient interface {
	gradientKind() string
}

// svgGradientStop is one stop in a gradient (offset 0..1, resolved color, opacity 0..1).
type svgGradientStop struct {
	offset  float64
	color   *Color  // resolved at parse time; never nil
	opacity float64 // multiplied into final alpha; default 1.0
}

type svgGradientUnits int

const (
	svgGradientUserSpace  svgGradientUnits = 0 // userSpaceOnUse (default)
	svgGradientObjectBBox svgGradientUnits = 1 // objectBoundingBox
)

type svgSpreadMethod int

const (
	svgSpreadPad svgSpreadMethod = 0 // default; only one supported in Phase 3a
)

type svgLinearGradient struct {
	x1, y1, x2, y2 float64
	stops          []svgGradientStop
	units          svgGradientUnits
	spread         svgSpreadMethod
	transform      *svgMatrix
}

func (*svgLinearGradient) gradientKind() string { return "linearGradient" }

type svgRadialGradient struct {
	cx, cy, r, fx, fy float64
	stops             []svgGradientStop
	units             svgGradientUnits
	spread            svgSpreadMethod
	transform         *svgMatrix
}

func (*svgRadialGradient) gradientKind() string { return "radialGradient" }

// svgPaint is a tagged union for fill/stroke values: either a solid color or a gradient ref.
// At least one of color or gradRef should be set (or both nil for "none"/"transparent").
type svgPaint struct {
	color   *Color // non-nil → plain color
	gradRef string // non-empty → url(#id) reference; resolved at render time
}
```

### Step 2: Modify `svg_types.go`

Find the `SVG` struct and add `gradients` field:

```go
type SVG struct {
	viewBox   *svgViewBox
	width     float64
	height    float64
	par       svgPreserveAspect
	root      *svgGroup
	gradients map[string]svgGradient // id → gradient definition (collected from <defs>)
}
```

Find `svgStyle` and change `fill` / `stroke` from `*Color` to `*svgPaint`:

```go
type svgStyle struct {
	fill          *svgPaint  // was *Color
	stroke        *svgPaint  // was *Color
	strokeWidth   float64
	dashArray     []float64
	dashOffset    float64
	lineCap       LineCap
	lineJoin      LineJoin
	miterLimit    float64
	opacity       float64
	fillOpacity   float64
	strokeOpacity float64
	fillRule      string
	display       bool
}
```

Update `defaultSVGStyle()` accordingly:

```go
func defaultSVGStyle() svgStyle {
	return svgStyle{
		fill:          &svgPaint{color: &Color{R: 0, G: 0, B: 0, A: 1}},
		stroke:        nil,
		// ... rest unchanged ...
	}
}
```

### Step 3: Build

```
go build ./...
```

**This will FAIL** in `svg_parse.go` (`applySingleSVGStyleProp` assigns `*Color` to `s.fill`) and `svg_render.go` (`svgStyleToShapeStyle` reads `*Color` from `s.fill`). The subsequent tasks fix these call sites. For now, intentionally leave them broken — but if you need a passing build before commit, add temporary adapter code that builds (e.g., extract `s.fill.color` when assigning from `*Color`).

**Simplest path:** Update the two call sites inline as part of this task:

- In `svg_parse.go` `applySingleSVGStyleProp` for `case "fill":` change `if c, ok := parseSVGColor(val); ok { s.fill = c }` to `if c, ok := parseSVGColor(val); ok { s.fill = &svgPaint{color: c} }`. Same for stroke.
- In `svg_render.go` `svgStyleToShapeStyle` change `if s.fill != nil` to `if s.fill != nil && s.fill.color != nil`, then use `c := *s.fill.color`. Same for stroke.

### Step 4: Run full test suite

```
go test ./...
```

All Phase 2 tests must still pass — the refactor is structural but semantically identical for non-gradient fills.

### Step 5: Commit

```
refactor: svg — replace svgStyle.fill/stroke *Color with *svgPaint (tagged color/gradient union); add gradient IR types
```

---

## Task 2: parseSVGPaint (url(#id) recognition)

**Files:**
- Modify: `svg_attrs.go`
- Modify: `svg_attrs_test.go`
- Modify: `svg_parse.go` (call site)

### Step 1: Failing tests

Append to `svg_attrs_test.go`:

```go
func TestParseSVGPaint_PlainColor(t *testing.T) {
	p, ok := parseSVGPaint("red")
	if !ok || p == nil || p.color == nil || p.gradRef != "" {
		t.Errorf("red → %+v ok=%v", p, ok)
	}
	if p.color.R != 1 {
		t.Errorf("red.R = %g", p.color.R)
	}
}

func TestParseSVGPaint_URLReference(t *testing.T) {
	p, ok := parseSVGPaint("url(#myGrad)")
	if !ok || p == nil || p.color != nil || p.gradRef != "myGrad" {
		t.Errorf("url(#myGrad) → %+v ok=%v", p, ok)
	}
}

func TestParseSVGPaint_URLWithWhitespace(t *testing.T) {
	p, _ := parseSVGPaint("url( #abc )")
	if p == nil || p.gradRef != "abc" {
		t.Errorf("url( #abc ) → %+v", p)
	}
}

func TestParseSVGPaint_None(t *testing.T) {
	p, ok := parseSVGPaint("none")
	if !ok || p != nil {
		t.Errorf("none → %+v ok=%v, want nil/true", p, ok)
	}
}

func TestParseSVGPaint_Garbage(t *testing.T) {
	_, ok := parseSVGPaint("not-a-thing")
	if ok { t.Error("garbage should fail") }
}
```

### Step 2: Run failing tests

```
go test -run TestParseSVGPaint -v ./...
```

### Step 3: Add `parseSVGPaint` to `svg_attrs.go`

```go
// parseSVGPaint parses fill/stroke values: solid colors OR gradient refs.
// For "none"/"transparent" returns (nil, true). For unrecognized input returns (nil, false).
// For "url(#id)" returns &svgPaint{gradRef: "id"}.
// For plain colors returns &svgPaint{color: c}.
func parseSVGPaint(s string) (*svgPaint, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	// url(#id) reference
	if strings.HasPrefix(s, "url(") {
		close := strings.IndexByte(s, ')')
		if close < 0 {
			return nil, false
		}
		body := strings.TrimSpace(s[4:close])
		if !strings.HasPrefix(body, "#") {
			return nil, false
		}
		id := strings.TrimSpace(body[1:])
		if id == "" {
			return nil, false
		}
		return &svgPaint{gradRef: id}, true
	}
	// Fall back to color parsing
	c, ok := parseSVGColor(s)
	if !ok {
		return nil, false
	}
	if c == nil {
		return nil, true // none/transparent → nil paint with ok=true
	}
	return &svgPaint{color: c}, true
}
```

### Step 4: Update call site in `svg_parse.go`

In `applySingleSVGStyleProp`, replace:

```go
case "fill":
    if c, ok := parseSVGColor(val); ok {
        s.fill = &svgPaint{color: c}
    }
case "stroke":
    if c, ok := parseSVGColor(val); ok {
        s.stroke = &svgPaint{color: c}
    }
```

with:

```go
case "fill":
    if p, ok := parseSVGPaint(val); ok {
        s.fill = p
    }
case "stroke":
    if p, ok := parseSVGPaint(val); ok {
        s.stroke = p
    }
```

### Step 5: Run

```
go test -run TestParseSVGPaint -v ./... && go test ./...
```

All pass, no regressions.

### Step 6: Commit

```
feat: svg — parseSVGPaint recognizes url(#id) gradient references
```

---

## Task 3: `<stop>` element parser

**Files:**
- Create: `svg_parse_gradient.go`
- Create: `svg_parse_gradient_test.go`

### Step 1: Failing tests in `svg_parse_gradient_test.go`

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/xml"
	"math"
	"strings"
	"testing"
)

func parseStopElement(t *testing.T, xmlStr string) svgGradientStop {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader(xmlStr))
	for {
		tok, err := d.Token()
		if err != nil { t.Fatal(err) }
		if start, ok := tok.(xml.StartElement); ok {
			return parseSVGGradientStop(d, start)
		}
	}
}

func TestParseSVGStop_BasicOffsetAndColor(t *testing.T) {
	s := parseStopElement(t, `<stop offset="0.5" stop-color="red"/>`)
	if math.Abs(s.offset-0.5) > 1e-9 || s.color == nil || s.color.R != 1 || s.opacity != 1 {
		t.Errorf("got %+v color=%+v", s, s.color)
	}
}

func TestParseSVGStop_OffsetPercent(t *testing.T) {
	s := parseStopElement(t, `<stop offset="75%" stop-color="blue"/>`)
	if math.Abs(s.offset-0.75) > 1e-9 {
		t.Errorf("offset = %g", s.offset)
	}
}

func TestParseSVGStop_OpacityFromAttribute(t *testing.T) {
	s := parseStopElement(t, `<stop offset="0" stop-color="green" stop-opacity="0.5"/>`)
	if math.Abs(s.opacity-0.5) > 1e-9 {
		t.Errorf("opacity = %g", s.opacity)
	}
}

func TestParseSVGStop_StyleAttribute(t *testing.T) {
	s := parseStopElement(t, `<stop offset="0" style="stop-color:red;stop-opacity:0.3"/>`)
	if s.color == nil || s.color.R != 1 {
		t.Errorf("color = %+v", s.color)
	}
	if math.Abs(s.opacity-0.3) > 1e-9 {
		t.Errorf("opacity = %g", s.opacity)
	}
}

func TestParseSVGStop_DefaultsWhenAbsent(t *testing.T) {
	s := parseStopElement(t, `<stop offset="0"/>`)
	// Default color: black; default opacity: 1
	if s.color == nil || s.color.R != 0 || s.color.G != 0 || s.color.B != 0 {
		t.Errorf("default color = %+v", s.color)
	}
	if s.opacity != 1 {
		t.Errorf("default opacity = %g", s.opacity)
	}
}
```

### Step 2: Run failing tests

```
go test -run TestParseSVGStop -v ./...
```

### Step 3: Create `svg_parse_gradient.go`

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"encoding/xml"
	"strings"
)

// parseSVGGradientStop reads a <stop> element. Caller has already received the StartElement.
// On exit, the </stop> end element has been consumed.
func parseSVGGradientStop(d *xml.Decoder, start xml.StartElement) svgGradientStop {
	stop := svgGradientStop{
		color:   &Color{R: 0, G: 0, B: 0, A: 1},
		opacity: 1,
	}
	for _, a := range start.Attr {
		applyStopAttr(&stop, a.Name.Local, a.Value)
	}
	for _, a := range start.Attr {
		if a.Name.Local == "style" {
			for _, decl := range strings.Split(a.Value, ";") {
				kv := strings.SplitN(decl, ":", 2)
				if len(kv) != 2 {
					continue
				}
				applyStopAttr(&stop, strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1]))
			}
		}
	}
	_ = d.Skip()
	return stop
}

func applyStopAttr(s *svgGradientStop, name, val string) {
	switch name {
	case "offset":
		val = strings.TrimSpace(val)
		if strings.HasSuffix(val, "%") {
			n, ok := parseSVGNumber(strings.TrimSuffix(val, "%"))
			if ok {
				s.offset = n / 100
			}
		} else if n, ok := parseSVGNumber(val); ok {
			s.offset = n
		}
		s.offset = clamp01(s.offset)
	case "stop-color":
		if c, ok := parseSVGColor(val); ok && c != nil {
			s.color = c
		}
	case "stop-opacity":
		if n, ok := parseSVGNumber(val); ok {
			s.opacity = clamp01(n)
		}
	}
}
```

### Step 4: Run, ensure all pass

```
go test -run TestParseSVGStop -v ./...
```

### Step 5: Commit

```
feat: svg — <stop> element parser (offset numeric/percent, stop-color, stop-opacity, style)
```

---

## Task 4: `<linearGradient>` / `<radialGradient>` / `<defs>` parsers + SVG.gradients collection

**Files:**
- Modify: `svg_parse_gradient.go`
- Modify: `svg_parse.go`
- Modify: `svg_parse_gradient_test.go`

### Step 1: Failing tests

Add fixtures `testdata/svg/linear_gradient.svg`:
```xml
<?xml version="1.0"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
  <defs>
    <linearGradient id="grad1" x1="0" y1="0" x2="100" y2="0">
      <stop offset="0" stop-color="red"/>
      <stop offset="1" stop-color="blue"/>
    </linearGradient>
  </defs>
  <rect x="0" y="0" width="100" height="100" fill="url(#grad1)"/>
</svg>
```

`testdata/svg/radial_gradient.svg`:
```xml
<?xml version="1.0"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
  <defs>
    <radialGradient id="grad2" cx="50" cy="50" r="50" fx="50" fy="50" gradientUnits="userSpaceOnUse" gradientTransform="matrix(1 0 0 1 0 0)">
      <stop offset="0" stop-color="white" stop-opacity="1"/>
      <stop offset="0.5" stop-color="orange"/>
      <stop offset="1" stop-color="red" stop-opacity="0.5"/>
    </radialGradient>
  </defs>
  <circle cx="50" cy="50" r="50" fill="url(#grad2)"/>
</svg>
```

Append to `svg_parse_gradient_test.go`:

```go
import "os"

func TestParseSVG_LinearGradientCollected(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/linear_gradient.svg")
	svg, err := parseSVGBytes(data)
	if err != nil { t.Fatal(err) }
	if len(svg.gradients) != 1 { t.Fatalf("gradients count = %d", len(svg.gradients)) }
	g, ok := svg.gradients["grad1"].(*svgLinearGradient)
	if !ok { t.Fatalf("type = %T", svg.gradients["grad1"]) }
	if g.x1 != 0 || g.x2 != 100 || g.y1 != 0 || g.y2 != 0 {
		t.Errorf("coords = (%g,%g)-(%g,%g)", g.x1, g.y1, g.x2, g.y2)
	}
	if len(g.stops) != 2 {
		t.Errorf("stops count = %d", len(g.stops))
	}
	if g.stops[0].color.R != 1 || g.stops[1].color.B != 1 {
		t.Errorf("stop colors wrong: %+v", g.stops)
	}
}

func TestParseSVG_RadialGradientCollected(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/radial_gradient.svg")
	svg, _ := parseSVGBytes(data)
	g, ok := svg.gradients["grad2"].(*svgRadialGradient)
	if !ok { t.Fatalf("type = %T", svg.gradients["grad2"]) }
	if g.cx != 50 || g.r != 50 {
		t.Errorf("radial coords wrong: %+v", g)
	}
	if len(g.stops) != 3 {
		t.Errorf("stops = %d", len(g.stops))
	}
	if g.units != svgGradientUserSpace {
		t.Errorf("units = %v", g.units)
	}
	if g.transform == nil {
		t.Error("transform should be present")
	}
}

func TestParseSVG_RectWithGradientFillRef(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/linear_gradient.svg")
	svg, _ := parseSVGBytes(data)
	r, _ := svg.root.children[0].(*svgRect)
	if r == nil || r.style.fill == nil || r.style.fill.gradRef != "grad1" {
		t.Errorf("rect fill = %+v", r.style.fill)
	}
}
```

### Step 2: Run, observe failures

```
go test -run "TestParseSVG_(Linear|Radial)Gradient|TestParseSVG_RectWithGradient" -v ./...
```

### Step 3: Implement parsers in `svg_parse_gradient.go`

Add at the end of `svg_parse_gradient.go`:

```go
// parseSVGLinearGradient reads a <linearGradient> element.
func parseSVGLinearGradient(d *xml.Decoder, start xml.StartElement) *svgLinearGradient {
	g := &svgLinearGradient{x2: 1} // SVG default: x1=0 y1=0 x2=1 y2=0 (in objectBoundingBox units)
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "x1":
			g.x1, _ = parseSVGLength(a.Value)
		case "y1":
			g.y1, _ = parseSVGLength(a.Value)
		case "x2":
			g.x2, _ = parseSVGLength(a.Value)
		case "y2":
			g.y2, _ = parseSVGLength(a.Value)
		case "gradientUnits":
			if strings.TrimSpace(a.Value) == "userSpaceOnUse" {
				g.units = svgGradientUserSpace
			} else {
				g.units = svgGradientObjectBBox
			}
		case "gradientTransform":
			if m, ok := parseSVGTransform(a.Value); ok && m != matrixIdentity() {
				g.transform = &m
			}
		case "spreadMethod":
			// Phase 3a: only pad supported; reflect/repeat fall back silently
			g.spread = svgSpreadPad
		}
	}
	g.stops = collectGradientStops(d)
	return g
}

// parseSVGRadialGradient reads a <radialGradient> element.
func parseSVGRadialGradient(d *xml.Decoder, start xml.StartElement) *svgRadialGradient {
	g := &svgRadialGradient{
		cx: 0.5, cy: 0.5, r: 0.5, // SVG defaults (in objectBoundingBox units)
	}
	hasFx, hasFy := false, false
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "cx":
			g.cx, _ = parseSVGLength(a.Value)
		case "cy":
			g.cy, _ = parseSVGLength(a.Value)
		case "r":
			g.r, _ = parseSVGLength(a.Value)
		case "fx":
			g.fx, _ = parseSVGLength(a.Value)
			hasFx = true
		case "fy":
			g.fy, _ = parseSVGLength(a.Value)
			hasFy = true
		case "gradientUnits":
			if strings.TrimSpace(a.Value) == "userSpaceOnUse" {
				g.units = svgGradientUserSpace
			} else {
				g.units = svgGradientObjectBBox
			}
		case "gradientTransform":
			if m, ok := parseSVGTransform(a.Value); ok && m != matrixIdentity() {
				g.transform = &m
			}
		}
	}
	// SVG default: fx=cx, fy=cy when absent
	if !hasFx {
		g.fx = g.cx
	}
	if !hasFy {
		g.fy = g.cy
	}
	g.stops = collectGradientStops(d)
	return g
}

// collectGradientStops walks child elements consuming </xxxGradient> at the end.
func collectGradientStops(d *xml.Decoder) []svgGradientStop {
	var stops []svgGradientStop
	for {
		tok, err := d.Token()
		if err != nil { return stops }
		switch t := tok.(type) {
		case xml.EndElement:
			return stops
		case xml.StartElement:
			if t.Name.Local == "stop" {
				stops = append(stops, parseSVGGradientStop(d, t))
			} else {
				_ = d.Skip()
			}
		}
	}
}
```

### Step 4: Wire into XML walker in `svg_parse.go`

In `parseSVGRoot`, after initializing `svg`, ensure the gradients map is initialized:

```go
svg := &SVG{
    root:      &svgGroup{style: defaultSVGStyle()},
    gradients: make(map[string]svgGradient),
}
```

In `parseSVGElement` switch, add cases:

```go
case "defs":
    return nil, parseSVGDefs(d, parent)
case "linearGradient":
    return nil, registerLinearGradient(d, parent, start)
case "radialGradient":
    return nil, registerRadialGradient(d, parent, start)
```

Add helpers at bottom of `svg_parse.go` (or in `svg_parse_gradient.go`):

```go
// parseSVGDefs walks <defs> children, collecting gradient definitions.
// Returns once </defs> is consumed.
func parseSVGDefs(d *xml.Decoder, parent *svgGroup) error {
    // Find the SVG root via walking parent chain — but we don't have it.
    // SIMPLEST FIX: pass *SVG through parseSVGElement signature.
    // ... see Step 5
    return nil
}
```

**The cleanest fix:** add a `*SVG` parameter through `parseSVGElement` / `parseSVGChildren`. Refactor those signatures. The parser already needs access to gradient registry to register at parse time.

Refactor `parseSVGChildren` to accept `*SVG`:

```go
func parseSVGChildren(d *xml.Decoder, svg *SVG, parent *svgGroup) error {
    for {
        tok, err := d.Token()
        // ...
        case xml.StartElement:
            child, err := parseSVGElement(d, svg, parent, t)
            // ...
    }
}

func parseSVGElement(d *xml.Decoder, svg *SVG, parent *svgGroup, start xml.StartElement) (svgNode, error) {
    switch start.Name.Local {
    case "g":
        g := &svgGroup{style: parent.style}
        // ...
        if err := parseSVGChildren(d, svg, g); err != nil {
            return nil, err
        }
        return g, nil
    // ...
    case "defs":
        return nil, parseSVGDefs(d, svg, parent)
    case "linearGradient":
        if id := findAttr(start.Attr, "id"); id != "" {
            svg.gradients[id] = parseSVGLinearGradient(d, start)
        } else {
            _ = d.Skip()
        }
        return nil, nil
    case "radialGradient":
        if id := findAttr(start.Attr, "id"); id != "" {
            svg.gradients[id] = parseSVGRadialGradient(d, start)
        } else {
            _ = d.Skip()
        }
        return nil, nil
    // ...
    }
}

func findAttr(attrs []xml.Attr, name string) string {
    for _, a := range attrs {
        if a.Name.Local == name {
            return a.Value
        }
    }
    return ""
}

func parseSVGDefs(d *xml.Decoder, svg *SVG, parent *svgGroup) error {
    for {
        tok, err := d.Token()
        if err != nil { return err }
        switch t := tok.(type) {
        case xml.EndElement:
            return nil
        case xml.StartElement:
            // defs can contain gradients (collected) or other defs (recurse)
            switch t.Name.Local {
            case "linearGradient":
                if id := findAttr(t.Attr, "id"); id != "" {
                    svg.gradients[id] = parseSVGLinearGradient(d, t)
                } else {
                    _ = d.Skip()
                }
            case "radialGradient":
                if id := findAttr(t.Attr, "id"); id != "" {
                    svg.gradients[id] = parseSVGRadialGradient(d, t)
                } else {
                    _ = d.Skip()
                }
            default:
                _ = d.Skip()
            }
        }
    }
}
```

Update the call from `parseSVGRoot`:

```go
if err := parseSVGChildren(d, svg, svg.root); err != nil {
    return nil, err
}
```

All other callsites of `parseSVGChildren` / `parseSVGElement` (group recursion) need updating to pass `svg` through.

### Step 5: Run

```
go test -run "TestParseSVG" -v ./...
```

All new gradient tests PASS, all Phase 2 tests still PASS.

### Step 6: Commit

```
feat: svg — parse <linearGradient>/<radialGradient>/<defs>; collect into SVG.gradients map
```

---

## Task 5: Phase 2 cascade adapt + Aspose logo doesn't error

**Files:**
- Modify: `svg_render.go`
- Modify: `svg_test.go` (or `svg_render_test.go`)

### Step 1: Update render cascade to handle `*svgPaint`

The `svgStyleToShapeStyle` and `svgStyleToLineStyle` already need to handle `s.fill = nil` (no fill). Now they also need to handle `s.fill.gradRef != ""` (gradient ref pending).

Phase 3a Task 5 approach: **gradient refs render as TRANSPARENT (no fill)** temporarily, until Tasks 6-8 implement actual shading. Aspose logo at this point: gradient-filled arcs will be invisible (currently fall back to black in Phase 2 — change to transparent).

In `svg_render.go`:

```go
func svgStyleToShapeStyle(s svgStyle) ShapeStyle {
	ss := ShapeStyle{LineStyle: svgStyleToLineStyle(s)}
	if s.fill != nil && s.fill.color != nil {
		c := *s.fill.color
		c.A *= s.fillOpacity
		ss.FillColor = &c
	}
	// If s.fill.gradRef != "" → handled in Task 6+ (pattern emission); for now no fill.
	return ss
}

func svgStyleToLineStyle(s svgStyle) LineStyle {
	ls := LineStyle{
		Width:       s.strokeWidth,
		DashPattern: s.dashArray,
		DashPhase:   s.dashOffset,
		Cap:         s.lineCap,
		Join:        s.lineJoin,
		MiterLimit:  s.miterLimit,
	}
	if s.stroke != nil && s.stroke.color != nil {
		c := *s.stroke.color
		c.A *= s.strokeOpacity
		ls.Color = &c
	} else {
		ls.Width = 0
	}
	return ls
}
```

### Step 2: Verify Aspose logo still renders without error (gradient shapes invisible)

```
go test -run TestAddSVG_AsposeLogoGradientShapesSkippedSilently -v ./...
```

Test rewritten name from Phase 2 — it still checks "no error". Pass.

### Step 3: Commit

```
refactor: svg — render cascade handles *svgPaint (plain color path; gradient stub for Task 6+)
```

---

## Task 6: PDF shading function builder

**Files:**
- Create: `svg_render_gradient.go`
- Create: `svg_render_gradient_test.go`

This task builds the **PDF function objects** that drive shading patterns. No rendering integration yet — pure data structure construction.

### Step 1: Inspect existing PDF object machinery

Read `types.go` to find `pdfObject`, `pdfDict`, `pdfArray`, `pdfRef` types. Read `document.go` to find how indirect objects are added (look for the field on `Document` that holds the object map — likely `d.objects` of type `map[int]*pdfObject`).

### Step 2: Failing tests in `svg_render_gradient_test.go`

```go
// SPDX-License-Identifier: MIT

package asposepdf

import (
	"testing"
)

func TestBuildShadingFunction_OneStop(t *testing.T) {
	stops := []svgGradientStop{
		{offset: 0, color: &Color{R: 1, G: 0, B: 0, A: 1}, opacity: 1},
	}
	fn := buildShadingFunction(stops)
	if fn == nil {
		t.Fatal("nil function")
	}
	dict, ok := fn.value.(pdfDict)
	if !ok {
		t.Fatalf("function value not a dict: %T", fn.value)
	}
	if v, _ := dict["/FunctionType"]; v == nil {
		t.Fatal("missing /FunctionType")
	}
}

func TestBuildShadingFunction_TwoStops_ExponentialType2(t *testing.T) {
	stops := []svgGradientStop{
		{offset: 0, color: &Color{R: 1, G: 0, B: 0, A: 1}, opacity: 1},
		{offset: 1, color: &Color{R: 0, G: 0, B: 1, A: 1}, opacity: 1},
	}
	fn := buildShadingFunction(stops)
	dict := fn.value.(pdfDict)
	if dict["/FunctionType"].(pdfValue).value != 2 {
		t.Errorf("FunctionType = %v, want 2", dict["/FunctionType"])
	}
	// C0 = red, C1 = blue
	c0 := dict["/C0"].(pdfArray)
	if len(c0) != 3 || c0[0].(pdfValue).value != 1.0 {
		t.Errorf("C0 = %v", c0)
	}
}

func TestBuildShadingFunction_ThreeStops_StitchingType3(t *testing.T) {
	stops := []svgGradientStop{
		{offset: 0, color: &Color{R: 1, G: 0, B: 0, A: 1}, opacity: 1},
		{offset: 0.5, color: &Color{R: 0, G: 1, B: 0, A: 1}, opacity: 1},
		{offset: 1, color: &Color{R: 0, G: 0, B: 1, A: 1}, opacity: 1},
	}
	fn := buildShadingFunction(stops)
	dict := fn.value.(pdfDict)
	if dict["/FunctionType"].(pdfValue).value != 3 {
		t.Errorf("FunctionType = %v, want 3", dict["/FunctionType"])
	}
	functions := dict["/Functions"].(pdfArray)
	if len(functions) != 2 {
		t.Errorf("Functions count = %d, want 2", len(functions))
	}
	bounds := dict["/Bounds"].(pdfArray)
	if len(bounds) != 1 || bounds[0].(pdfValue).value != 0.5 {
		t.Errorf("Bounds = %v", bounds)
	}
	encode := dict["/Encode"].(pdfArray)
	if len(encode) != 4 {
		t.Errorf("Encode = %v", encode)
	}
}
```

(Note: the exact `pdfValue` / `pdfDict` / `pdfArray` API may differ — read `types.go` and adapt the test assertions to the actual representation.)

### Step 3: Run, observe failures

```
go test -run TestBuildShadingFunction -v ./...
```

### Step 4: Implement in `svg_render_gradient.go`

```go
// SPDX-License-Identifier: MIT

package asposepdf

// buildShadingFunction returns a pdfObject containing a function that maps t in [0,1]
// to RGB color, suitable for use as the /Function of a PDF shading dictionary.
//
// For 1 stop: returns a constant Type 2 exponential with N=1, C0=C1=stop color.
// For 2 stops: returns a single Type 2 exponential interpolating between them.
// For 3+ stops: returns a Type 3 stitching function combining (N-1) Type 2 exponentials,
// one for each adjacent stop pair, with /Bounds at internal stop offsets.
func buildShadingFunction(stops []svgGradientStop) *pdfObject {
	if len(stops) == 0 {
		// Degenerate: black constant
		stops = []svgGradientStop{{offset: 0, color: &Color{R: 0, G: 0, B: 0, A: 1}, opacity: 1}}
	}
	if len(stops) == 1 {
		return exponentialFunction(stops[0].color, stops[0].color)
	}
	if len(stops) == 2 {
		return exponentialFunction(stops[0].color, stops[1].color)
	}
	// Stitching
	funcs := make(pdfArray, 0, len(stops)-1)
	bounds := make(pdfArray, 0, len(stops)-2)
	encode := make(pdfArray, 0, 2*(len(stops)-1))
	for i := 0; i < len(stops)-1; i++ {
		funcs = append(funcs, pdfValueRef(exponentialFunction(stops[i].color, stops[i+1].color)))
		encode = append(encode, pdfFloat(0), pdfFloat(1))
	}
	for i := 1; i < len(stops)-1; i++ {
		bounds = append(bounds, pdfFloat(stops[i].offset))
	}
	dict := pdfDict{
		"/FunctionType": pdfInt(3),
		"/Domain":       pdfArray{pdfFloat(0), pdfFloat(1)},
		"/Functions":    funcs,
		"/Bounds":       bounds,
		"/Encode":       encode,
	}
	return &pdfObject{value: dict}
}

// exponentialFunction builds a /FunctionType 2 dict with N=1 (linear interpolation).
func exponentialFunction(c0, c1 *Color) *pdfObject {
	dict := pdfDict{
		"/FunctionType": pdfInt(2),
		"/Domain":       pdfArray{pdfFloat(0), pdfFloat(1)},
		"/C0":           pdfArray{pdfFloat(c0.R), pdfFloat(c0.G), pdfFloat(c0.B)},
		"/C1":           pdfArray{pdfFloat(c1.R), pdfFloat(c1.G), pdfFloat(c1.B)},
		"/N":            pdfInt(1),
	}
	return &pdfObject{value: dict}
}
```

(Adapt `pdfInt`, `pdfFloat`, `pdfValueRef` to the actual library API — they may be named differently. Read `types.go` first.)

### Step 5: Run, ensure all pass

```
go test -run TestBuildShadingFunction -v ./...
```

### Step 6: Commit

```
feat: svg — buildShadingFunction (PDF Type 2/3 function for gradient interpolation)
```

---

## Task 7: gradientToShadingObject + Page.ensurePatternResource

**Files:**
- Modify: `svg_render_gradient.go`
- Modify: `page.go` or `vector_draw.go` (wherever `ensureExtGState` lives — add `ensurePatternResource` similarly)

### Step 1: Add helpers in `svg_render_gradient.go`

```go
// gradientToShadingObject creates a /Shading dictionary indirect object for the gradient,
// given its computed coords in user space (after bbox + transform composition).
func gradientToShadingObject(grad svgGradient) *pdfObject {
	shading := pdfDict{
		"/ColorSpace": pdfName("/DeviceRGB"),
		"/Extend":     pdfArray{pdfBool(true), pdfBool(true)},
	}
	switch g := grad.(type) {
	case *svgLinearGradient:
		shading["/ShadingType"] = pdfInt(2)
		shading["/Coords"] = pdfArray{
			pdfFloat(g.x1), pdfFloat(g.y1),
			pdfFloat(g.x2), pdfFloat(g.y2),
		}
		shading["/Function"] = pdfValueRef(buildShadingFunction(g.stops))
	case *svgRadialGradient:
		shading["/ShadingType"] = pdfInt(3)
		shading["/Coords"] = pdfArray{
			pdfFloat(g.fx), pdfFloat(g.fy), pdfFloat(0), // focal point with radius 0
			pdfFloat(g.cx), pdfFloat(g.cy), pdfFloat(g.r),
		}
		shading["/Function"] = pdfValueRef(buildShadingFunction(g.stops))
	default:
		return nil
	}
	return &pdfObject{value: shading}
}
```

### Step 2: Add `ensurePatternResource` to `Page` (mirror `ensureExtGState` pattern)

Read existing `ensureExtGState` in `vector_draw.go` or `page.go` to copy the convention. Add (in same file):

```go
// ensurePatternResource creates a shading pattern object combining the shading
// dictionary and the supplied matrix, registers it in the document, inserts a
// /Pattern Px entry in this page's /Resources/Pattern, and returns "Px".
func (p *Page) ensurePatternResource(shading *pdfObject, matrix svgMatrix) string {
	// 1. Register the shading object in document indirect objects
	shadingID := p.doc.addObject(shading)

	// 2. Build the pattern dict
	patternDict := pdfDict{
		"/Type":        pdfName("/Pattern"),
		"/PatternType": pdfInt(2), // shading pattern
		"/Shading":     pdfRef(shadingID),
		"/Matrix": pdfArray{
			pdfFloat(matrix[0]), pdfFloat(matrix[1]),
			pdfFloat(matrix[2]), pdfFloat(matrix[3]),
			pdfFloat(matrix[4]), pdfFloat(matrix[5]),
		},
	}
	patternObj := &pdfObject{value: patternDict}
	patternID := p.doc.addObject(patternObj)

	// 3. Insert into /Resources/Pattern/Px
	resources := p.ensureResourcesDict()
	patterns, ok := resources["/Pattern"].(pdfDict)
	if !ok {
		patterns = pdfDict{}
		resources["/Pattern"] = patterns
	}
	name := fmt.Sprintf("P%d", len(patterns))
	patterns["/"+name] = pdfRef(patternID)
	return name
}
```

(Adapt to actual `Document` / `Page` internal API — `addObject` may be different, `ensureResourcesDict` may need to be added or already exists. Read `vector_draw.go` / `page.go` first.)

### Step 3: No unit tests in this task — covered by Task 8's integration tests

### Step 4: Build

```
go build ./...
```

Must compile cleanly.

### Step 5: Commit

```
feat: svg — gradientToShadingObject + Page.ensurePatternResource (PDF shading pattern infrastructure)
```

---

## Task 8: Renderer integration — emit gradient fills

**Files:**
- Modify: `svg_render.go`
- Modify: `svg_render_gradient.go`

This is the integration task that finally renders gradient fills in PDF.

### Step 1: Compute shape bounding box helper in `svg_render_gradient.go`

```go
// svgShapeBBox returns the axis-aligned bbox of a shape in its local coordinate space.
// Used for objectBoundingBox gradient unit mapping.
func svgShapeBBox(n svgNode) (x0, y0, x1, y1 float64) {
	switch s := n.(type) {
	case *svgRect:
		return s.x, s.y, s.x + s.w, s.y + s.h
	case *svgCircle:
		return s.cx - s.r, s.cy - s.r, s.cx + s.r, s.cy + s.r
	case *svgEllipse:
		return s.cx - s.rx, s.cy - s.ry, s.cx + s.rx, s.cy + s.ry
	case *svgLine:
		x0, x1 = minMax(s.x1, s.x2)
		y0, y1 = minMax(s.y1, s.y2)
		return
	case *svgPolyline:
		return pointsBBox(s.points)
	case *svgPolygon:
		return pointsBBox(s.points)
	case *svgPath:
		return pathBBox(s.commands)
	}
	return 0, 0, 0, 0
}

func minMax(a, b float64) (float64, float64) {
	if a < b { return a, b }
	return b, a
}

func pointsBBox(pts []Point) (x0, y0, x1, y1 float64) {
	if len(pts) == 0 { return }
	x0, y0 = pts[0].X, pts[0].Y
	x1, y1 = x0, y0
	for _, p := range pts[1:] {
		if p.X < x0 { x0 = p.X }
		if p.X > x1 { x1 = p.X }
		if p.Y < y0 { y0 = p.Y }
		if p.Y > y1 { y1 = p.Y }
	}
	return
}

func pathBBox(ops []svgPathOp) (x0, y0, x1, y1 float64) {
	first := true
	track := func(x, y float64) {
		if first {
			x0, y0, x1, y1 = x, y, x, y
			first = false
			return
		}
		if x < x0 { x0 = x }
		if x > x1 { x1 = x }
		if y < y0 { y0 = y }
		if y > y1 { y1 = y }
	}
	for _, op := range ops {
		switch op.kind {
		case 'M', 'L':
			track(op.args[0], op.args[1])
		case 'C':
			track(op.args[4], op.args[5])
		case 'Q':
			track(op.args[2], op.args[3])
		}
	}
	return
}
```

### Step 2: Implement gradient paint emission

In `svg_render_gradient.go`, add:

```go
// emitGradientFill writes the operators to set the current fill to a shading pattern,
// or returns false if the gradient ref can't be resolved.
func emitGradientFill(buf *bytes.Buffer, p *Page, svg *SVG, paint *svgPaint, shape svgNode) bool {
	if paint == nil || paint.gradRef == "" {
		return false
	}
	grad, ok := svg.gradients[paint.gradRef]
	if !ok {
		return false
	}
	// Compute the pattern matrix (bbox + gradientTransform composition)
	matrix := matrixIdentity()
	switch g := grad.(type) {
	case *svgLinearGradient:
		if g.units == svgGradientObjectBBox {
			x0, y0, x1, y1 := svgShapeBBox(shape)
			matrix = matrixMul(matrix, svgMatrix{x1 - x0, 0, 0, y1 - y0, x0, y0})
		}
		if g.transform != nil {
			matrix = matrixMul(matrix, *g.transform)
		}
	case *svgRadialGradient:
		if g.units == svgGradientObjectBBox {
			x0, y0, x1, y1 := svgShapeBBox(shape)
			matrix = matrixMul(matrix, svgMatrix{x1 - x0, 0, 0, y1 - y0, x0, y0})
		}
		if g.transform != nil {
			matrix = matrixMul(matrix, *g.transform)
		}
	}
	shading := gradientToShadingObject(grad)
	if shading == nil {
		return false
	}
	name := p.ensurePatternResource(shading, matrix)
	fmt.Fprintf(buf, "/Pattern cs\n/%s scn\n", name)
	return true
}
```

### Step 3: Wire into render path

The current architecture calls `emitRectangleToBuf(buf, p, rect, style)` etc., where `style` already includes fill color baked in. To inject pattern emission, refactor: have `renderSVGRect` etc. set the fill state BEFORE calling the emit helper, and configure `style` to skip its own fill setup if pattern was set.

**Simplest approach for Phase 3a:** SVG renderer's per-shape function knows about the SVG (it has access via the walk). Change the renderSVG signature to pass `svg *SVG` through, then in `renderSVGRect` etc., before calling `emitRectangleToBuf`:

```go
func renderSVGRect(buf *bytes.Buffer, p *Page, svg *SVG, r *svgRect) {
	if !r.style.display || r.w <= 0 || r.h <= 0 { return }
	buf.WriteString("q\n")
	if r.transform != nil { writeCMOperator(buf, *r.transform) }
	style := svgStyleToShapeStyle(r.style)

	// Check for gradient fill — emit pattern setter BEFORE calling emit helper.
	// Then nullify style.FillColor so emit helper doesn't override with plain color.
	if emitGradientFill(buf, p, svg, r.style.fill, r) {
		style.FillColor = &Color{R: 1, G: 1, B: 1, A: 1} // placeholder; pattern is now active
		// We still need the emit helper to produce "f" operator. The trick: pass a sentinel
		// that tells it "skip fill color setup but DO emit fill paint op".
	}

	rect := Rectangle{LLX: r.x, LLY: r.y, URX: r.x + r.w, URY: r.y + r.h}
	if r.rx > 0 || r.ry > 0 {
		rr := r.rx
		if rr == 0 { rr = r.ry }
		emitRoundedRectangleToBuf(buf, p, rect, rr, style)
	} else {
		emitRectangleToBuf(buf, p, rect, style)
	}
	buf.WriteString("Q\n")
}
```

The "skip fill color setup" question is awkward. **Pragmatic Phase 3a approach:** after `emitGradientFill` emits `/Pattern cs\n/Px scn\n`, the next path-construction + paint operator pair (`re\nf`) will use the pattern fill color. The trick is that `emit*ToBuf` helpers currently re-set the fill color before the paint op. Need to:

Option A: Make `emit*ToBuf` accept a flag like `skipFillColor bool`.
Option B: Have `emit*ToBuf` skip fill color setup when `style.FillColor` is nil. Then we'd set `style.FillColor = nil` after emitting pattern. But then the paint op wouldn't include fill (`B` becomes `S`).

**Cleanest:** Add a sentinel — define `var sentinelPatternColor = &Color{R: -1, G: -1, B: -1, A: -1}` and have `emit*ToBuf` recognize it as "fill is already set externally, just emit fill paint op". This is ugly but localized.

**Even cleaner:** Refactor emit helpers to split into 3 phases: (1) set graphics state, (2) emit path, (3) emit paint op. Then SVG renderer can:
1. Set graphics state (alpha, stroke params)
2. Emit pattern cs/scn for fill IF gradient
3. Emit path
4. Emit paint op

Given the complexity, **Task 8 implementation note**: pick whichever refactor strategy makes Phase 1 tests still pass AND lets gradient fills render. Test both approaches if needed.

### Step 4: Test integration

Add in `svg_render_gradient_test.go`:

```go
import (
	"bytes"
	"os"
)

func TestRenderSVG_LinearGradientEmitsPattern(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/linear_gradient.svg")
	svg, _ := parseSVGBytes(data)
	doc := NewDocumentFromFormat(PageFormatA4)
	page, _ := doc.Page(1)
	if err := renderSVG(page, svg, Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 200}); err != nil {
		t.Fatal(err)
	}
	stream, _ := pageContentStreamBytes(page)
	if !bytes.Contains(stream, []byte("/Pattern cs")) {
		t.Errorf("expected /Pattern cs in stream:\n%s", stream)
	}
	if !bytes.Contains(stream, []byte(" scn")) {
		t.Errorf("expected pattern setter (scn op)")
	}
}

func TestRenderSVG_RadialGradientEmitsPattern(t *testing.T) {
	data, _ := os.ReadFile("testdata/svg/radial_gradient.svg")
	svg, _ := parseSVGBytes(data)
	doc := NewDocumentFromFormat(PageFormatA4)
	page, _ := doc.Page(1)
	renderSVG(page, svg, Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 200})
	stream, _ := pageContentStreamBytes(page)
	if !bytes.Contains(stream, []byte("/Pattern cs")) {
		t.Error("missing /Pattern cs for radial")
	}
}
```

### Step 5: Run

```
go test -run "TestRenderSVG_(Linear|Radial)GradientEmitsPattern" -v ./... && go test ./...
```

### Step 6: Commit

```
feat: svg — render gradient fills via /Pattern cs + shading patterns (linear + radial)
```

---

## Task 9: Integration tests + Aspose logo verification + AES round-trip + docs

**Files:**
- Modify: `svg_test.go` (extension)
- Modify: `CLAUDE.md`
- Modify: `README.md`

### Step 1: Add integration tests

Append to `svg_test.go`:

```go
func TestAddSVG_AsposeLogoFullColorAfterPhase3a(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	if err := page.AddSVG("testdata/aspose-logo.svg", pdf.Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 800}); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll("result_files", 0755)
	out := "result_files/TestAddSVG_AsposeLogoFullColorAfterPhase3a.pdf"
	if err := doc.Save(out); err != nil { t.Fatal(err) }
	report, _ := pdf.Validate(out)
	if !report.Valid {
		t.Errorf("validation failed: %+v", report.Issues)
	}
	// Re-open: confirm round-trip survives
	reopened, err := pdf.Open(out)
	if err != nil { t.Fatal(err) }
	if reopened.PageCount() != 1 {
		t.Errorf("page count = %d", reopened.PageCount())
	}
}

func TestAddSVG_GradientAES128Roundtrip(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	page.AddSVG("testdata/aspose-logo.svg", pdf.Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 800})
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword: "u", OwnerPassword: "o", Algorithm: pdf.EncryptionAlgAES128,
	})
	os.MkdirAll("result_files", 0755)
	out := "result_files/TestAddSVG_GradientAES128Roundtrip.pdf"
	if err := doc.Save(out); err != nil { t.Fatal(err) }
	if _, err := pdf.OpenWithPassword(out, "u"); err != nil { t.Fatal(err) }
}

func TestAddSVG_GradientAES256Roundtrip(t *testing.T) {
	doc := pdf.NewDocumentFromFormat(pdf.PageFormatA4)
	page, _ := doc.Page(1)
	page.AddSVG("testdata/aspose-logo.svg", pdf.Rectangle{LLX: 50, LLY: 700, URX: 300, URY: 800})
	doc.SetEncryption(pdf.EncryptionOptions{
		UserPassword: "u", OwnerPassword: "o", Algorithm: pdf.EncryptionAlgAES256,
	})
	os.MkdirAll("result_files", 0755)
	out := "result_files/TestAddSVG_GradientAES256Roundtrip.pdf"
	if err := doc.Save(out); err != nil { t.Fatal(err) }
	if _, err := pdf.OpenWithPassword(out, "u"); err != nil { t.Fatal(err) }
}
```

### Step 2: Update `CLAUDE.md` — find the existing SVG block, update the "Supported" line to mention gradients

Find:
```
- **Supported in Phase 2**: basic shapes ... [list]
- **Out of scope (Phase 3)**: `<text>`, `<image>` (raster via data-uri), gradients ...
```

Replace the gradients mention in "Out of scope" with: `gradients via PDF Type 2/3 shading patterns shipped in Phase 3a (linear + radial with stops, gradientUnits, gradientTransform, spreadMethod=pad)`. Move gradient details to "Supported" line. Keep `<text>`, `<image>`, etc. in "Out of scope" but remove "gradients" from there.

### Step 3: Update `README.md` — same edit in the SVG embedding section

### Step 4: Run full suite

```
go test ./...
```

### Step 5: Commit

```
test: svg — Phase 3a gradient integration tests (Aspose logo + AES round-trip) + docs
```

### Step 6: Close beads ticket

```
bd update pdf-go-mi3 --status closed
```

### Step 7: Final cleanup

```
gofmt -s -w .
git add -u
git commit -m "style: apply gofmt -s after Phase 3a implementation"
```

---

## Self-Review

Coverage of design:
- Linear gradient ✅ (Tasks 3, 4, 7, 8)
- Radial gradient ✅ (Tasks 3, 4, 7, 8)
- `<stop>` parser ✅ (Task 3)
- `<defs>` walker ✅ (Task 4)
- gradientUnits (both) ✅ (Task 8 — objectBoundingBox handled in `emitGradientFill`)
- gradientTransform ✅ (Task 8)
- spreadMethod=pad ✅ (default in Task 4)
- `url(#id)` reference ✅ (Task 2)
- svgPaint cascade ✅ (Tasks 1, 5)
- PDF stitching function ✅ (Task 6)
- Pattern resource ✅ (Task 7)
- Aspose logo correctness ✅ (Task 9)
- Encryption round-trip ✅ (Task 9)
- Docs ✅ (Task 9)

Type consistency:
- `svgPaint` used everywhere fill/stroke is — Tasks 1, 2, 5, 8
- `*pdfObject` API in Tasks 6-7 may need adjustment based on actual `types.go` — implementer note in each task

Implementer freedom:
- Task 7 leaves `ensurePatternResource` implementation flexible based on existing `ensureExtGState` pattern
- Task 8 leaves the "skip-fill-color in emit helper" approach open (3 options described); implementer picks whichever is cleanest
