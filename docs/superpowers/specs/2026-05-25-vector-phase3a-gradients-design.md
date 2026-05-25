# Vector Graphics Phase 3a — SVG Gradients

**Beads:** [pdf-go-mi3](bd show pdf-go-mi3) (Phase 3a) under umbrella [pdf-go-ybu](bd show pdf-go-ybu) (Vector support)
**Date:** 2026-05-25
**Status:** Design proposed

---

## Roadmap context

| Phase | Scope | Status |
|---|---|---|
| 1 | Native drawing primitives on `(*Page)` | ✅ Shipped (v0.1.0) |
| 2 | SVG-lite embedding (shapes, paths, transforms, viewBox) | ✅ Shipped |
| **3a (this spec)** | SVG gradients (linear + radial) via PDF Type 2/3 shading patterns | Designing |
| 3b | SVG `<text>` with font matching | Future |
| 3c | SVG `<image>` + `<defs>`/`<use>` + masks | Future |
| 3d | CSS blocks, filters, markers, exotic units, advanced spreadMethod | Future |

Phase 3a is the first sub-phase of "SVG full". It targets the single highest-impact feature gap from Phase 2 — gradients — without bundling unrelated work. After Phase 3a, the Aspose logo SVG renders correctly in full color.

---

## Phase 3a goals

Add `<linearGradient>` and `<radialGradient>` rendering to the existing SVG pipeline. All work is internal — no public API changes. Existing code paths (`AddSVG`, `AddSVGFromStream`, `AddSVGObject`, `LoadSVG`, watermark variants) automatically produce richer output when their source SVG uses gradients.

### Non-goals (Phase 3d candidates)

- `spreadMethod="reflect"` / `"repeat"` — requires PostScript function loops; rare in real SVG files
- `xlink:href` on gradients (stop inheritance from another gradient) — embellishment, easy to add later
- `linearRGB` color interpolation — Phase 3a uses sRGB blend (matches most viewers; SVG spec default is unclear)
- `<pattern>` element (tile fills) — separate feature
- Gradient on `stroke` — done; gradient on `text` — N/A (text is Phase 3b)
- Color management profiles in gradients

---

## Scope summary

| Element / attribute | Phase 3a support |
|---|---|
| `<linearGradient>` | ✅ `x1`/`y1`/`x2`/`y2`, `gradientUnits`, `gradientTransform`, `spreadMethod=pad`, child `<stop>` elements |
| `<radialGradient>` | ✅ `cx`/`cy`/`r`/`fx`/`fy`, `gradientUnits`, `gradientTransform`, `spreadMethod=pad`, child `<stop>` elements |
| `<stop>` | `offset` (numeric or percentage), `stop-color` (any color format), `stop-opacity` |
| `gradientUnits` | Both: `userSpaceOnUse` (default) + `objectBoundingBox` |
| `gradientTransform` | Full SVG transform syntax (reuses Phase 2's `parseSVGTransform`) |
| `spreadMethod` | Only `pad` (default). `reflect` / `repeat` fall back to `pad` |
| `fill="url(#id)"` / `stroke="url(#id)"` | ✅ Resolved at render time; emits PDF shading pattern |
| `<defs>` | ✅ Walker now collects gradient definitions (previously skipped) |

---

## Public API impact

**None.** All work is internal. The public surface added in Phase 2 (`(*Page).AddSVG`, `(*Document).LoadSVG`, etc.) automatically produces gradient-filled PDF output when called on an SVG containing gradients.

A user who was previously seeing inherited-fill fallback on gradient-referenced shapes will now see correct rendering.

---

## Internal architecture

### Files

| File | Responsibility |
|---|---|
| `svg_gradient.go` (new) | IR types: `svgGradient` interface, `svgLinearGradient`, `svgRadialGradient`, `svgGradientStop`, `svgGradientUnits` enum, `svgSpreadMethod` enum |
| `svg_parse_gradient.go` (new) | `<linearGradient>` / `<radialGradient>` / `<stop>` element parsers; `<defs>` walker collecting gradient definitions into `SVG.gradients` map |
| `svg_render_gradient.go` (new) | PDF shading-pattern emission: `emitAxialShading`, `emitRadialShading`, `buildShadingFunction` (Type 3 stitching of Type 2 exponentials), `ensurePatternResource`, `gradientToShadingObject` |
| `svg_types.go` (modify) | Add `gradients map[string]svgGradient` field to `SVG` struct; replace `svgStyle.fill` / `svgStyle.stroke` types from `*Color` to `*svgPaint` (new tagged union) |
| `svg_attrs.go` (modify) | Extend `parseSVGColor` to recognize `url(#id)` → returns `*svgPaint{gradRef: "id"}` |
| `svg_parse.go` (modify) | XML walker recognizes `<linearGradient>` / `<radialGradient>` / `<defs>`; `applySingleSVGStyleProp` for `fill` / `stroke` uses new paint type |
| `svg_render.go` (modify) | `svgStyleToShapeStyle` / `svgStyleToLineStyle` resolve `svgPaint` — if `gradRef != ""`, calls renderer that emits `/Pattern cs` + `/Px scn` instead of `RG`/`rg` color operators. Computes shape bbox when `gradientUnits == objectBoundingBox` |

### Key internal types

```go
// svgGradient is the interface for both linear and radial gradients.
type svgGradient interface {
    gradientKind() string
}

// svgGradientStop is one stop in a gradient.
type svgGradientStop struct {
    offset  float64 // 0..1 (parsed from numeric or percentage)
    color   *Color  // resolved at parse; never nil (falls back to black on unknown)
    opacity float64 // 0..1; final stop alpha = color.A * opacity
}

type svgGradientUnits int
const (
    svgGradientUserSpace svgGradientUnits = 0 // userSpaceOnUse (default for SVG 1.1)
    svgGradientObjectBBox svgGradientUnits = 1 // objectBoundingBox (default for some content)
)

type svgSpreadMethod int
const (
    svgSpreadPad svgSpreadMethod = 0 // default; the only one supported in Phase 3a
)

type svgLinearGradient struct {
    x1, y1, x2, y2 float64
    stops          []svgGradientStop
    units          svgGradientUnits
    spread         svgSpreadMethod
    transform      *svgMatrix // optional
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

// svgPaint is a tagged union — either a solid color or a gradient reference.
// Replaces *Color in svgStyle.fill / svgStyle.stroke.
type svgPaint struct {
    color   *Color // non-nil → plain color
    gradRef string // non-empty → unresolved url(#id) reference; resolved at render time
}

// SVG.gradients field added (collected from <defs> at parse time):
type SVG struct {
    // ... existing fields ...
    gradients map[string]svgGradient // id → gradient definition
}
```

### svgStyle change

`svgStyle.fill` / `svgStyle.stroke` change from `*Color` to `*svgPaint`. This is a localized refactor — all set-sites (`applySingleSVGStyleProp`) and read-sites (`svgStyleToShapeStyle` / `svgStyleToLineStyle`) need updating.

### PDF shading pattern infrastructure

#### Pattern resource

The `Page` gains a per-page `ensurePatternResource` helper (analog to `ensureExtGState`):

```go
func (p *Page) ensurePatternResource(shading *pdfObject, matrix svgMatrix) string
```

Steps:
1. Creates `/Pattern` dictionary with `/PatternType 2` (shading pattern), `/Shading` (ref to passed shading object), `/Matrix` (from passed transform — combined gradient/user-space transform composed with shape's CTM context)
2. Registers as indirect object in document
3. Inserts into `/Resources/Pattern/Px` dict where `x` is the next available index
4. Returns the name (`"P0"`, `"P1"`, ...)

#### Shading object

Created by `gradientToShadingObject(gradient svgGradient, units mapping, bbox Rectangle)`:

- For linear: `/ShadingType 2` (axial), `/Coords [x1 y1 x2 y2]`, `/Function fn`, `/Extend [true true]` (pad behavior), `/ColorSpace /DeviceRGB`
- For radial: `/ShadingType 3` (radial), `/Coords [fx fy 0 cx cy r]`, `/Function fn`, `/Extend [true true]`, `/ColorSpace /DeviceRGB`

(Note: `/Coords` for Type 3 is `[start.x, start.y, start.r, end.x, end.y, end.r]` — focal point is the start with radius 0.)

#### Stitching function

Built by `buildShadingFunction(stops []svgGradientStop) *pdfObject`:

For N stops with offsets `t_0 < t_1 < ... < t_{N-1}`:
- Single stop: emits an exponential function (Type 2) that returns the constant color
- Two stops: single Type 2 exponential interpolating between them
- 3+ stops: **Type 3 stitching** combining (N-1) Type 2 exponential functions:

```
<< /FunctionType 3
   /Domain [0 1]
   /Functions [F_01 F_12 F_23 ... F_(N-2)(N-1)]
   /Bounds [t_1 t_2 ... t_(N-2)]    % N-2 internal boundaries
   /Encode [0 1 0 1 0 1 ...]        % 2*(N-1) entries
>>
```

Each `F_ij` is:

```
<< /FunctionType 2
   /Domain [0 1]
   /C0 [r_i g_i b_i]    % RGB of stop i
   /C1 [r_j g_j b_j]    % RGB of stop j
   /N 1                  % linear interpolation
>>
```

Stop alpha (`stop-opacity`) is handled via **ExtGState** when the shape is painted (per-shape alpha applied to entire gradient — Phase 3a simplification; per-stop alpha gradients would require a soft-mask gradient which is Phase 3c territory).

### Rendering algorithm

When `svgStyleToShapeStyle(s svgStyle)` is called and `s.fill.gradRef != ""`:

1. Lookup `s.fill.gradRef` in `svg.gradients` — if missing, fall back to black (silent best-effort)
2. Compute the shape's bounding box (for `objectBoundingBox` units mapping)
3. If `units == objectBoundingBox`: compose a matrix mapping `[0,1]×[0,1]` to the bbox; apply to gradient coords
4. Compose `gradientTransform` with the bbox matrix
5. Call `gradientToShadingObject(...)` → indirect shading object
6. Call `p.ensurePatternResource(shading, matrix)` → pattern name (e.g., `"P0"`)
7. Mutate the emitted content stream: instead of `r g b rg` (RGB fill color), emit:
   - `/Pattern cs\n` (set fill color space)
   - `/Px scn\n` (set fill pattern; `scn` for fill, `SCN` for stroke)
8. Then emit path construction + paint operator (`f` / `f*`) as usual

If stroke is a gradient, analogous treatment via `/Pattern CS` + `/Px SCN` and `S` paint operator.

### Color spaces

Phase 3a uses **DeviceRGB** only. All `<stop>` colors are pre-converted to RGB at parse time (already happens via `parseSVGColor` returning `*Color` with R/G/B/A in `[0,1]`).

For gradients with `stop-opacity < 1`, Phase 3a multiplies the stop alpha into the shape's overall fillOpacity, then sets a single ExtGState for the whole shape. A "true" per-stop alpha gradient would need a parallel grayscale alpha-gradient acting as soft mask — that's Phase 3c.

### `<defs>` walker

In Phase 2, `<defs>` was skipped via `d.Skip()` in the default branch. In Phase 3a, the XML walker:

1. Recognizes `<defs>` and walks its children
2. Inside defs, recognizes `<linearGradient>` / `<radialGradient>` and parses them into `svgLinearGradient` / `svgRadialGradient`
3. Stores parsed gradients in `SVG.gradients[id]` (using the `id` attribute as key)
4. Does NOT add them to the rendering tree (defs content is reference-only)

Gradients can also appear at the top level (not inside `<defs>`) — the walker handles both.

---

## Key behaviors

### Gradient lookup at render time

`fill="url(#a)"` doesn't require `<linearGradient id="a">` to appear before the referencing shape — forward references work (resolved when render walker reaches the shape).

### Missing references

If `fill="url(#nonexistent)"` is used, the renderer falls back to **black** (Phase 2's existing behavior for unrecognized color values). No error.

### `objectBoundingBox` units

When the gradient uses `gradientUnits="objectBoundingBox"`:
1. Shape's bounding box `[x0, y0, x1, y1]` is computed (for `<rect>`: trivial; for `<path>`: walk path ops and track min/max; for `<circle>`/`<ellipse>`: from center + radii)
2. Gradient coords like `x1="0.5"` `y1="0.5"` are interpreted as fractional positions within the bbox
3. A bbox-to-userspace matrix is composed: `[w 0 0 h x0 y0]` where `w, h` are bbox dimensions
4. This matrix is concatenated into the Pattern's `/Matrix` before the user's `gradientTransform`

### Pattern matrix composition

Final pattern matrix = `shape_ctm × bbox_to_userspace × gradient_transform`

The bbox matrix is identity when `units == userSpaceOnUse`.

### Multi-page round-trip

Pattern objects are stored in document indirect objects (same as `/AcroForm`, `/Pages`, etc.). They survive encryption (encrypted per-object like all other content) and re-open cleanly.

---

## Testing strategy

### Unit tests

- `svg_parse_gradient_test.go` — parsing `<linearGradient>` / `<radialGradient>` / `<stop>` with all attributes; offset percentage parsing; missing/extra stops
- `svg_render_gradient_test.go` — `buildShadingFunction` with 1/2/3+ stops produces correct PDF dict structure
- Extension to `svg_attrs_test.go` — `parseSVGColor("url(#a)")` returns the right `svgPaint`

### Integration tests

- `svg_test.go` (extension) — end-to-end: SVG with a single linear gradient renders, output contains `/Pattern` resources
- Aspose logo round-trip after Phase 3a: gradient-filled arcs render (not silently skipped)
- Per-shape gradient fills via `objectBoundingBox` and `userSpaceOnUse`
- `gradientTransform` correctness (rotated gradient on rectangle)
- AES-128 / AES-256 encryption round-trip with gradient SVG content

### Spec-aware verification

- A trivial 2-color linear gradient should produce `/ShadingType 2`, `/Coords [...]`, `/Function << /FunctionType 2 /C0 [...] /C1 [...] /N 1 >>`. Inspect the generated PDF stream and assert presence
- A 3-color gradient should produce `/ShadingType 2`, `/Function << /FunctionType 3 ... /Functions [...] /Bounds [...] /Encode [...] >>`
