# Vector Graphics Phase 3d — SVG `<mask>`, CSS, `<filter>`, `<marker>`

**Beads:** [pdf-go-j6s](bd show pdf-go-j6s) (Phase 3d) under umbrella [pdf-go-ybu](bd show pdf-go-ybu)
**Date:** 2026-05-26
**Status:** Design proposed

---

## Roadmap context

| Phase | Scope | Status |
|---|---|---|
| 1 | Native drawing primitives | ✅ Shipped (v0.1.0) |
| 2 | SVG-lite (shapes, paths, transforms, viewBox) | ✅ Shipped |
| 3a | SVG gradients (linear + radial) | ✅ Shipped |
| 3b | SVG `<text>` rendering | ✅ Shipped |
| 3c | SVG `<image>`, `<defs>`/`<use>`/`<symbol>`, `<clipPath>` | ✅ Shipped |
| **3d (this spec)** | SVG `<mask>`, CSS `<style>`, `<filter>`, `<marker>` — practical completion | Designing |
| 3e (future) | textPath, vertical text, em/% units, spreadMethod reflect/repeat — niche features | Future / on-demand |

Phase 3d covers the four remaining high-impact SVG features. Combined with prior phases, the library will render ~95% of real-world SVG content (design tool exports, web icons, technical diagrams).

---

## Phase 3d goals

Four independent features, no overlap. All work internal — no public API changes.

### Non-goals (Phase 3e / out of scope)

- `<textPath>` — text along a path; requires glyph-by-glyph layout on parametric curves
- Vertical text (`writing-mode="tb"`) — East-Asian convention
- `xml:space="preserve"` — preserves significant whitespace verbatim (rare)
- `em`/`ex`/`%` length units — context-dependent (font / parent bbox)
- `spreadMethod="reflect"` / `"repeat"` — exotic gradient modes
- CSS pseudo-classes (`:hover`, `:first-child`, etc.) — out of static-doc scope
- CSS descendant selectors (`g rect`) — high complexity, low practical use
- CSS `@media`/`@import`/CSS variables
- Animation (SMIL: `<animate>`, `<animateTransform>`)
- `<filter>` primitives beyond `feDropShadow` (no software rasterizer = no Gaussian blur)
- `<image>` width/height overriding viewBox in `<use>`-of-`<symbol>` (complex viewport)

---

## Scope summary

### `<mask>`

| Element / Attribute | Support |
|---|---|
| `<mask>` element | Container with rendering children (shapes, paths, text) |
| `id` for reference | Required (ignored without it) |
| `maskUnits` | `userSpaceOnUse` (default) + `objectBoundingBox` |
| `maskContentUnits` | `userSpaceOnUse` (default) + `objectBoundingBox` |
| `mask="url(#id)"` on shapes/groups | Resolved at render time via PDF soft mask |
| Children types | rect/circle/ellipse/line/polyline/polygon/path; best-effort on text/image |
| Color model | Luminance-based (white = visible, black = hidden) |

### CSS `<style>` blocks

| Aspect | Support |
|---|---|
| `<style>` element with text content | Parsed at parse time |
| `class` selector (`.foo`) | ✓ |
| `id` selector (`#foo`) | ✓ |
| Element type selector (`rect`, `text`, `g`) | ✓ |
| Selector combinators (descendant, child, sibling) | ✗ (Phase 3e) |
| Pseudo-classes / pseudo-elements | ✗ |
| Specificity | Inline (1000) > id (100) > class (10) > type (1) |
| `class` attribute on elements | ✓ (multi-class supported: `class="a b c"`) |
| At-rules (`@media`, `@import`, etc.) | ✗ |

### `<filter>`

| Aspect | Support |
|---|---|
| `<filter>` element with `id` | Parsed |
| `filter="url(#id)"` on shapes | Resolved at render |
| `feDropShadow` primitive | Emulated via offset+alpha duplicate (no blur) |
| Other primitives (`feGaussianBlur`, `feOffset`, `feFlood`, `feMerge`, `feColorMatrix`, etc.) | Parsed but silently dropped (no PDF mapping without rasterizer) |
| `feDropShadow` attrs | `dx`/`dy`/`flood-color`/`flood-opacity` (stdDeviation ignored — no blur) |

### `<marker>`

| Aspect | Support |
|---|---|
| `<marker>` element with `id` | Container with viewBox + children |
| `marker-start` / `marker-end` / `marker-mid` | Attached to line/polyline/polygon/path |
| `orient="auto"` | Rotation along path tangent |
| `orient="<angle>"` | Fixed angle in degrees |
| `refX`/`refY` | Anchor point |
| `markerWidth`/`markerHeight` | Render size (composes with viewBox) |
| `markerUnits` | `strokeWidth` (default) + `userSpaceOnUse` |

---

## Public API impact

**None.** All work internal. Existing `(*Page).AddSVG` automatically gains these features.

---

## Internal architecture

### Files

| File | Responsibility |
|---|---|
| `svg_css.go` (new) | CSS parser, selector matcher, rule application during parse cascade |
| `svg_mask.go` (new) | `svgMask` IR + Form XObject creation + ExtGState /SMask infrastructure |
| `svg_parse_mask.go` (new) | XML parser for `<mask>` |
| `svg_render_mask.go` (new) | Rendering: build soft-mask Form XObject, apply via ExtGState |
| `svg_filter.go` (new) | `svgFilter` IR with parsed primitives; helpers for drop-shadow emulation |
| `svg_parse_filter.go` (new) | XML parser for `<filter>` + sub-primitives |
| `svg_render_filter.go` (new) | Drop-shadow emulation (offset+alpha duplicate before original) |
| `svg_marker.go` (new) | `svgMarker` IR + orientation computation helpers |
| `svg_parse_marker.go` (new) | XML parser for `<marker>` |
| `svg_render_marker.go` (new) | Render markers at line/path endpoints |
| `svg_types.go` (modify) | Add `mask`, `filter`, `markerStart`/`markerMid`/`markerEnd`, `cssClasses []string`, `cssID string` to `svgStyle` |
| `svg_parse.go` (modify) | Recognize `<style>`, `<mask>`, `<filter>`, `<marker>`; collect into defs; apply CSS rules in cascade |
| `svg_render.go` (modify) | Apply mask before shape body (similar to clip-path); apply filter; emit markers after stroke for line/path |
| Tests + fixtures | Per-feature unit + integration coverage |

### Internal types (compact)

```go
// === CSS ===
type cssRule struct {
    selectors  []cssSelector // parsed list (e.g., for ".a, .b { ... }")
    properties map[string]string
}

type cssSelector struct {
    kind    cssSelectorKind // class, id, element
    name    string          // bare name (no . or #)
}

type cssSelectorKind int
const (
    cssSelClass cssSelectorKind = iota
    cssSelID
    cssSelElement
)

// SVG struct extended:
type SVG struct {
    // ... existing ...
    cssRules []cssRule
}

// svgStyle extended:
type svgStyle struct {
    // ... existing ...
    cssClasses  []string // for matching .className rules
    cssID       string   // for matching #idName rules
    mask        string   // bare id; empty = no mask
    filter      string   // bare id; empty = no filter
    markerStart string   // bare id
    markerMid   string
    markerEnd   string
}

// === Mask ===
type svgMask struct {
    units        svgGradientUnits // maskUnits
    contentUnits svgGradientUnits // maskContentUnits
    children     []svgNode
    // bbox: optional explicit (x, y, w, h) — Phase 3d uses entire shape bbox
}

func (*svgMask) svgNodeKind() string { return "mask" }

// === Filter ===
type svgFilter struct {
    primitives []svgFilterPrimitive
}

type svgFilterPrimitive struct {
    kind string // "feDropShadow", "feGaussianBlur", "feOffset", etc.
    // Phase 3d only inspects feDropShadow attrs:
    dx, dy         float64
    floodColor     *Color
    floodOpacity   float64
}

func (*svgFilter) svgNodeKind() string { return "filter" }

// === Marker ===
type svgMarker struct {
    viewBox       *svgViewBox
    refX, refY    float64
    markerW, markerH float64
    orient        svgMarkerOrient
    units         svgMarkerUnits
    children      []svgNode
}

type svgMarkerOrient struct {
    auto  bool
    angle float64 // when auto == false
}

type svgMarkerUnits int
const (
    svgMarkerStrokeWidth svgMarkerUnits = 0 // default
    svgMarkerUserSpace   svgMarkerUnits = 1
)

func (*svgMarker) svgNodeKind() string { return "marker" }
```

---

## Algorithms

### CSS parser + cascade integration

1. Find `<style>` elements during XML walk; extract text content.
2. Parse rules: `selector_list { prop: val; ... }`. Selector list split by `,`. Property block split by `;`.
3. Selector types: starts with `.` → class; `#` → id; else element name.
4. Store rules in `svg.cssRules`.
5. During element parsing, BEFORE applying inline `style="..."` and presentation attrs:
   - Compute element's `cssClasses []string` from `class="a b c"` attr
   - Compute `cssID` from `id="x"` attr
   - For each rule in `svg.cssRules` matching this element (by class/id/type):
     - Apply each property (lowest-specificity first; ties broken by document order)
6. Then apply presentation attrs (higher specificity)
7. Then apply inline `style="..."` (highest)

Result: CSS rules merge into the existing style cascade with correct precedence.

### Mask rendering

When a shape has `style.mask = "id"`:

1. Look up `svg.defs[id]` — should be `*svgMask`.
2. Render mask children into a **Form XObject** with `/Type /Group /S /Transparency`:
   - The XObject content stream contains the mask children's PDF ops
   - Color space: DeviceGray (luminance interpretation per SVG spec)
3. Create an **ExtGState** dict with `/SMask <</Type /Mask /S /Luminosity /G <ref to Form XObject>>>`
4. Before the shape's paint emission, emit `/GS<n> gs` to set the soft mask
5. Emit the shape as usual — the soft mask attenuates pixel alpha

### Drop-shadow emulation

When a shape has `style.filter = "id"` and `svg.defs[id]` is a `*svgFilter` containing `feDropShadow`:

1. Before emitting the shape:
   - Apply ExtGState with alpha = `flood-opacity` (default 1)
   - Apply transform `[1 0 0 1 dx dy]` (CTM shift)
   - Render a copy of the shape with `fill = flood-color` (default black)
2. Then render the original shape with its original style

This produces visible offset-shadow without blur.

### Marker rendering

For each line/polyline/polygon/path with `marker-start`/`marker-end`/`marker-mid`:

1. Look up marker in defs.
2. Compute endpoint positions:
   - `marker-start`: first vertex; tangent = direction from first to second
   - `marker-end`: last vertex; tangent = direction from second-to-last to last
   - `marker-mid`: all internal vertices; tangent = bisector of incoming/outgoing
3. For each marker position:
   - Push graphics state (`q`)
   - Translate to position
   - If `orient="auto"`, rotate by tangent angle
   - Scale by marker viewBox size (composed with markerWidth/markerHeight)
   - If `markerUnits=strokeWidth`, additionally scale by current strokeWidth
   - Apply `refX`/`refY` translation (move marker's anchor point)
   - Render marker's children (using existing render pipeline)
   - Pop (`Q`)

---

## Key behaviors

### Mask color model

PDF soft masks use luminance: pixel grayscale value = mask alpha. SVG spec also defaults to luminance (via `mask-type="luminance"`). Phase 3d uses luminance only — `mask-type="alpha"` (using alpha channel directly) is parsed but treated as luminance (best-effort).

### CSS specificity scoring

Standard CSS: inline = 1000, id = 100, class = 10, type = 1. Within same specificity, document-order last wins. Phase 3d does NOT support `!important` (rare in SVG context).

### Drop-shadow with no blur

Real SVG drop-shadow combines offset + Gaussian blur. PDF has no native blur. Phase 3d emits the offset+alpha duplicate without blur, which gives a "hard shadow" effect — visually inferior to true drop-shadow but conveys the visual intent. Users who need true blur should pre-rasterize the SVG.

### Marker orientation

For `orient="auto"`:
- `marker-start`: angle = `atan2(p1.y - p0.y, p1.x - p0.x)` (direction from first to second vertex)
- `marker-end`: angle = `atan2(pn.y - pn-1.y, pn.x - pn-1.x)` (direction from second-to-last to last)
- `marker-mid`: angle = average of incoming and outgoing direction angles

For paths with curves: use tangent at the vertex (for cubic Béziers, the tangent at endpoint t=0 is `P1-P0`; at t=1 is `P3-P2`). Phase 3d simplification: for path markers, use the segment direction (treating control points as straight lines between path nodes — slightly inaccurate for curves but acceptable).

### Edge cases

- `<mask>` referenced but undefined → render unmasked (best-effort)
- `<mask>` with no children → no-op (fully transparent → shape hidden)
- `<filter>` referenced but undefined → render unfiltered
- `<filter>` containing no recognized primitives → render unfiltered (silent)
- `<marker>` referenced but undefined → skip marker placement
- `<marker>` with markerUnits="strokeWidth" but shape has no stroke → use width=1 fallback
- CSS rule applies to element type `g` but a shape's parent is `g`: only direct match (no descendant)
- Multiple CSS classes: `class="a b"` matches both `.a { ... }` and `.b { ... }` rules

---

## Testing strategy

### Unit tests

- `svg_css_test.go` — CSS parser (rules, selectors, properties); selector matching; specificity ordering
- `svg_mask_test.go` — mask parser; Form XObject creation (basic structural check)
- `svg_filter_test.go` — filter parser; drop-shadow primitive recognition
- `svg_marker_test.go` — marker parser; orientation calculation

### Integration tests (`svg_test.go`)

- SVG with `<style>` block + multi-class rect — verify resolved style
- SVG with mask of circle on rect — verify soft mask + ExtGState in PDF
- SVG with drop-shadow filter — verify duplicate shape with offset/alpha
- SVG with arrow marker on line — verify marker rendered at endpoints
- All 4 features + AES-128 round-trip

### Test fixtures

- `testdata/svg/style_classes.svg`
- `testdata/svg/mask_circle.svg`
- `testdata/svg/filter_dropshadow.svg`
- `testdata/svg/marker_arrow.svg`
- `testdata/svg/phase3d_combo.svg` (all 4 features together)
