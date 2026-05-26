# Vector Graphics Phase 3c — SVG `<image>`, `<defs>`/`<use>`/`<symbol>`, `<clipPath>`

**Beads:** [pdf-go-tq5](bd show pdf-go-tq5) (Phase 3c) under umbrella [pdf-go-ybu](bd show pdf-go-ybu) (Vector support)
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
| **3c (this spec)** | SVG `<image>` + `<defs>`/`<use>`/`<symbol>` + `<clipPath>` | Designing |
| 3d | CSS subset, `<mask>`, filters, markers, exotic units, textPath | Future |

Phase 3c covers the three highest-impact SVG features remaining: reusable element definitions (`<use>`), raster image embedding (`<image>`), and clipping paths (`<clipPath>`). These are heavily used in real-world SVG icons and illustrations from design tools (Inkscape, Illustrator, Figma).

`<mask>` (soft masking via PDF transparency groups) is the most complex remaining SVG feature and is deferred to Phase 3d.

---

## Goals

Three independent SVG features, all internal — no public API changes. The existing `(*Page).AddSVG` pipeline gains support automatically.

### Non-goals (Phase 3d candidates)

- `<mask>` — requires PDF transparency groups + Form XObjects + soft-mask machinery
- External `href`/`xlink:href` (file paths, http://, https://) — security + IO surface area
- `data:image/svg+xml;base64,...` — recursive SVG parsing
- CSS shape `clip-path: circle(...)` / `clip-path: inset(...)` — CSS shape syntax
- Transitive `<use>` chains where intermediate refs aren't in `<defs>` (forward → forward → forward)
- `<symbol>` width/height attributes overriding viewBox dimensions (complex viewport mapping)

---

## Scope summary

### `<image>`

| Attribute | Support |
|---|---|
| `href` / `xlink:href` | `data:image/png;base64,...` and `data:image/jpeg;base64,...` only |
| `x`, `y` | Position in user space |
| `width`, `height` | Dimensions |
| `preserveAspectRatio` | All 10 modes (reused from Phase 2 viewBox machinery, applied to image bbox vs intrinsic dims) |
| `transform` | Applied via PDF `cm` operator |

### `<defs>` / `<use>` / `<symbol>`

| Element / Attribute | Support |
|---|---|
| `<defs>` | Container; any element with `id` inside is collected into `svg.defs` |
| `<symbol>` | Container with own viewBox; not rendered directly, only via `<use>` |
| `<use>` | `href`/`xlink:href` = `#id`; `x`/`y` translation; `transform`; presentation attributes inherited |
| Forward references | Supported (resolution happens AFTER full parse via two-pass walk) |
| Transitive `<use>` (use→use) | Supported — depth-limited (cycle detection via visited set) |
| `<use>` style inheritance | Use's attrs become **defaults** for referent; explicit attrs on referent take priority |

### `<clipPath>`

| Aspect | Support |
|---|---|
| Definition | `<clipPath id="..."><...shapes...></clipPath>` inside `<defs>` (or top-level) |
| Child shapes | `<rect>`, `<circle>`, `<ellipse>`, `<line>`, `<polyline>`, `<polygon>`, `<path>` |
| `clipPathUnits` | `userSpaceOnUse` (default) + `objectBoundingBox` |
| `clip-path` attribute on shapes | `clip-path="url(#id)"` — resolved at render time |
| Multiple children | Union (combined path before W operator) |
| `clip-rule` | `nonzero` (W) / `evenodd` (W*) — defaults to nonzero |
| Nested clipping | Stacks via successive `q ... W ... Q` |

---

## Public API impact

**None.** All work is internal. Existing API (`(*Page).AddSVG`, `LoadSVG`, `AddSVGWatermark`) automatically produces richer output when source SVG uses these features.

---

## Internal architecture

### Files

| File | Responsibility |
|---|---|
| `svg_image.go` (new) | IR type `svgImage`; `decodeSVGDataURI` (parse `data:image/<mime>;base64,<encoded>` → bytes + format); image XObject creation helper (reuses existing `AddImageFromStream` infrastructure) |
| `svg_use.go` (new) | IR types `svgUse`, `svgSymbol`; `resolveUseReferences` (parse-end pass that replaces `*svgUse` nodes with deep-cloned subtree wrapped in translate transform); `deepCloneSVGNode` |
| `svg_clip.go` (new) | IR type `svgClipPath`; `emitClipPath` (writes path construction ops + `W`/`W*` + `n`) |
| `svg_parse_image.go` (new) | XML parser for `<image>` |
| `svg_parse_use.go` (new) | XML parsers for `<use>` and `<symbol>` |
| `svg_parse_clip.go` (new) | XML parser for `<clipPath>` |
| `svg_types.go` (modify) | Add new types to `svgNode` implementations; extend `SVG` with `defs map[string]svgNode`; add `clipPath string` to `svgStyle` |
| `svg_parse.go` (modify) | Walker recognizes `<image>`, `<use>`, `<symbol>`, `<clipPath>`; `<defs>` walker (Phase 3a) extended to collect all id'd elements not just gradients; `applySingleSVGStyleProp` handles `clip-path` |
| `svg_render.go` (modify) | Type switch dispatches new IR types; before each shape's paint, check `style.clipPath` and emit clip path before shape's body |
| Tests + fixtures | `testdata/svg/image_*.svg`, `use_*.svg`, `clippath_*.svg`; unit + integration coverage |

### Internal types

```go
// svgImage is the IR node for an SVG <image> element.
type svgImage struct {
    x, y, w, h float64
    par        svgPreserveAspect // preserveAspectRatio for fit-within-rect mapping
    data       []byte            // raw bytes (PNG or JPEG)
    format     ImageFormat       // ImageFormatPNG or ImageFormatJPEG
    style      svgStyle
    transform  *svgMatrix
}

func (*svgImage) svgNodeKind() string { return "image" }

// svgUse is a placeholder before resolveUseReferences replaces it with the
// cloned referent. After resolution, no *svgUse nodes remain in the tree.
type svgUse struct {
    refID     string
    x, y      float64 // translation applied before referent
    style     svgStyle
    transform *svgMatrix
}

func (*svgUse) svgNodeKind() string { return "use" }

// svgSymbol is a container with its own viewBox. Not rendered directly;
// only referenced via <use href="#symbolId">.
type svgSymbol struct {
    viewBox  *svgViewBox
    children []svgNode
    style    svgStyle
}

func (*svgSymbol) svgNodeKind() string { return "symbol" }

// svgClipPath defines a clipping path; resolved at render time when a shape
// has clip-path="url(#id)".
type svgClipPath struct {
    units    svgGradientUnits // reuses enum: userSpaceOnUse | objectBoundingBox
    children []svgNode        // shape children (rect, circle, path, etc.)
}

func (*svgClipPath) svgNodeKind() string { return "clipPath" }

// Extended SVG.defs:
type SVG struct {
    // ... existing ...
    defs map[string]svgNode // generalized: any element with id gets stored here
                            // (replaces or extends Phase 3a's gradients map)
}

// Extended svgStyle:
type svgStyle struct {
    // ... existing ...
    clipPath string // "url(#id)" reference; empty = no clip
}
```

Note: `gradients map[string]svgGradient` from Phase 3a stays as-is for type-safe gradient lookup. `defs` is the general storage for everything else (symbols, clipPaths, top-level shapes with ids — anything that can be `<use>`-referenced).

### Algorithm: `<image>` rendering

1. Parse `href`/`xlink:href` data URI:
   ```
   data:image/png;base64,iVBORw0KGgoAAAANSU...
       ─────────  ──────  ──────────────────
       MIME type  Encoding Base64 data
   ```
2. Validate MIME (`image/png` or `image/jpeg`); decode base64.
3. Store bytes + format in `svgImage.data` / `.format`.
4. At render time:
   - Wrap in `q ... Q`
   - Apply `transform` if present
   - Apply outer image placement matrix `[w 0 0 h x y] cm` (positions + sizes the unit-square image XObject)
   - Honor `preserveAspectRatio` (compose with the placement matrix for letterboxing when intrinsic image dims differ from declared w/h)
   - Register image as PDF Image XObject (reuse infrastructure from `(*Page).AddImageFromStream`)
   - Emit `/Im<x> Do`

### Algorithm: `<use>` resolution (parse-end pass)

After the main XML parse populates `svg.defs` with all id'd elements, walk the IR tree and replace each `*svgUse` with a deep clone of its referent:

```go
func resolveUseReferences(svg *SVG, node svgNode, visited map[string]bool) svgNode {
    switch n := node.(type) {
    case *svgUse:
        if visited[n.refID] {
            return nil // cycle — drop
        }
        target, ok := svg.defs[n.refID]
        if !ok { return nil } // missing ref — silently drop
        visited[n.refID] = true
        // Recursively resolve inside the target (handles use→use→...)
        cloned := deepCloneSVGNode(target)
        cloned = resolveUseReferences(svg, cloned, visited)
        delete(visited, n.refID)
        // Wrap in group with use's translate + transform + style overrides
        return wrapUseReferent(cloned, n)
    case *svgGroup:
        for i, c := range n.children {
            n.children[i] = resolveUseReferences(svg, c, visited)
        }
        // Remove nils (from missing/cyclic refs)
        n.children = compactNonNil(n.children)
        return n
    }
    return node
}
```

`wrapUseReferent` constructs an `*svgGroup` with:
- Transform = `translate(use.x, use.y) ∘ use.transform`
- Style: use's style applied as defaults (referent's explicit attrs override)
- Children: the resolved/cloned target

For `<symbol>` referenced via `<use>`, the symbol's viewBox creates an additional CTM scaling the children to the use's intended bounds.

### Algorithm: `<clipPath>` rendering

When `renderSVGNode` encounters a shape with non-empty `style.clipPath`:

1. Extract id from `url(#id)` (strip prefix/suffix).
2. Lookup in `svg.defs` — should be `*svgClipPath`. If missing or wrong type, render shape unclipped (best-effort).
3. Inside the existing `q ... Q` shape wrapper, before the shape's paint operator:
   - Apply objectBoundingBox bbox transform if `clipPathUnits == objectBoundingBox`
   - Emit each clip child's path construction ops (no paint op)
   - Emit `W` (nonzero) or `W*` (evenodd) clip operator
   - Emit `n` (end path without painting; clip is now active)
4. Then emit the shape's normal paint operator (`f`/`B`/`S`).

PDF sequence:
```
q
<clip path construction ops>  ← e.g., from <circle cx="..." cy="..." r="..."/>
W                              ← intersect current clip with path
n                              ← end path without painting
<shape ops>                    ← the actual shape (clipped)
Q
```

### Cascade integration

`clip-path` is a presentation attribute. Add a case to `applySingleSVGStyleProp`:

```go
case "clip-path":
    val = strings.TrimSpace(val)
    if strings.HasPrefix(val, "url(") {
        // Extract id: url(#abc) → abc
        if id := extractURLID(val); id != "" {
            s.clipPath = id
        }
    } else if val == "none" {
        s.clipPath = ""
    }
```

`s.clipPath` stores the BARE id (no `url(#)` wrapper) for direct map lookup.

---

## Key behaviors

### Data URI parsing

```go
const dataURIPrefix = "data:"
// data:image/<mime>;base64,<encoded>
func decodeSVGDataURI(s string) (data []byte, format ImageFormat, ok bool) {
    if !strings.HasPrefix(s, dataURIPrefix) { return nil, 0, false }
    s = s[len(dataURIPrefix):]
    semi := strings.IndexByte(s, ';')
    comma := strings.IndexByte(s, ',')
    if semi < 0 || comma < 0 || semi > comma { return nil, 0, false }
    mime := s[:semi]
    encodingAndData := s[semi+1:]
    if !strings.HasPrefix(encodingAndData, "base64,") { return nil, 0, false }
    encoded := encodingAndData[len("base64,"):]
    b, err := base64.StdEncoding.DecodeString(encoded)
    if err != nil { return nil, 0, false }
    switch mime {
    case "image/png":
        return b, ImageFormatPNG, true
    case "image/jpeg", "image/jpg":
        return b, ImageFormatJPEG, true
    }
    return nil, 0, false
}
```

### `<use>` style override semantics

Per SVG spec §5.6: attributes on `<use>` become defaults for the referent. The referent's own explicit attrs are NOT overridden. Phase 3c implements simplified semantics: `wrapUseReferent` builds the wrapping group's style by inheriting `<use>`'s style; the referent's children inherit through the normal cascade, so already-explicit attrs there override the use defaults.

### `clipPathUnits="objectBoundingBox"`

Same logic as gradients (Phase 3a). When this mode is set, the clip path's children are in `[0,1]×[0,1]` relative to the clipped shape's bbox. Apply a matrix `[w 0 0 h x0 y0]` (where w/h/x0/y0 from shape bbox) before emitting clip path construction.

### Empty / degenerate cases

- `<image>` with non-data URL → skip silently (Phase 3c doesn't fetch external)
- `<image>` with malformed base64 → skip
- `<image>` with unsupported MIME → skip
- `<use href="#missing">` → drop the node (silent skip)
- Cyclic `<use>` chain → cycle detection via visited set drops the use node
- `<clipPath>` referenced but undefined → render unclipped (best-effort)
- `<clipPath>` with non-shape children (e.g., `<text>`) — SVG spec allows text-as-clip but Phase 3c skips text children (text-as-clip path is Phase 3d / out-of-scope)

### Interaction with Phase 3a/3b

- Gradient fill on a shape with `clip-path` → both work: gradient via Pattern cs + clip via W
- Text with `clip-path` → works: BT/ET wrapped inside the clipped q/Q
- `<use>` referencing a `<text>` → text clones, fonts re-resolve at render time
- `<image>` inside `<g>` with opacity → group's `/GS gs` applies to the image

---

## Testing strategy

### Unit tests

- `svg_image_test.go` — data URI decoder (well-formed PNG/JPEG, malformed base64, unknown MIME, missing prefix)
- `svg_use_test.go` — deepClone correctness; cycle detection; missing ref returns nil
- `svg_clip_test.go` — clipPath parser captures children; clipPathUnits parsing

### Integration tests (`svg_test.go`)

- End-to-end `<image>` with tiny inline PNG → rendered PDF contains `Do` XObject reference
- `<use>` referencing a defined `<circle>`, rendered N=10 times → 10 circles at correct positions
- `<use>` referencing a `<symbol>` with viewBox → scaled correctly
- `<clipPath>` clipping a rectangle → PDF contains `W n` operators
- Combination: `<use>` of a clipped shape
- AES-128 round-trip with each feature

### Test fixtures

- `testdata/svg/image_inline_png.svg` — `<image href="data:image/png;base64,...">` with a 4x4 red PNG
- `testdata/svg/image_inline_jpeg.svg` — same with JPEG
- `testdata/svg/use_simple.svg` — `<defs><circle id="dot".../></defs><use href="#dot" x="10"/><use href="#dot" x="50"/>`
- `testdata/svg/use_symbol.svg` — `<symbol>` referenced by `<use>` with viewBox scaling
- `testdata/svg/clippath_circle.svg` — rectangle clipped to a circle
- `testdata/svg/clippath_path.svg` — complex path clip
- `testdata/svg/use_with_clip.svg` — use + clipPath combined
