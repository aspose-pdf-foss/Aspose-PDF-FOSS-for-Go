# Render to Image — Implementation Plan

> **For agentic workers:** implement task-by-task with TDD; one logical change per commit. Steps use checkbox (`- [ ]`) syntax.

**Goal:** A pure-Go (stdlib-only) PDF page rasterizer with PNG/JPEG/GIF/BMP/TIFF output and a hybrid (idiomatic + Aspose-style) API.

**Design:** [docs/superpowers/specs/2026-06-04-render-to-image-design.md](../specs/2026-06-04-render-to-image-design.md)

**Umbrella bead:** pdf-go-61r

**Conventions:** every new `.go` file starts with `// SPDX-License-Identifier: MIT` then a blank line then `package asposepdf`. Tests in package `asposepdf_test` using synthetic docs (no external reference renderer). Detailed TDD steps below are given for P0 and P1 (the next actionable work); P2–P6 and P-TIFF list their tasks and get a detailed plan section appended when their bead is started.

---

## File structure

| File | Phase | Responsibility |
|---|---|---|
| `raster.go` | P0 | `rasterizer` type: coverage buffer, `fill(path, rule, color, clip)`, compositing |
| `raster_path.go` | P0 | flatten bezier/arc → polyline; `devPath` (device-space polygons) |
| `raster_test.go` | P0 | shape/coverage unit tests |
| `render.go` | P1 | content interpreter + graphics state machine |
| `render_device.go` | P1 | `RenderOptions`, `(*Page).RenderImage/RenderPNG/RenderJPEG/RenderGIF`, `Resolution`, `PngDevice`/`JpegDevice`/`GifDevice` |
| `raster_stroke.go` | P1/P6 | stroke → fill outline |
| `render_image.go` | P2 | image XObject + inline image sampling |
| `bmp.go` | P2 | BMP encoder + `BmpDevice` |
| `render_glyph.go`, `render_text.go` | P3 | glyph outline raster + text state |
| `render_std14*.go` | P4 | bundled Standard-14 outlines |
| `render_clip.go`, `render_gstate.go`, `render_shading.go` | P5 | clip/alpha/shading |
| `tiff.go`, `render_document.go` | P-TIFF | TIFF encoder + whole-doc render |

---

## Phase P0 — Rasterizer foundation (bead pdf-go-g3g)

Self-contained, no PDF. Internal types (lowercase) — nothing public ships yet.

### Task P0.1 — Device path + flattening

**Files:** Create `raster_path.go`, `raster_test.go`.

- [ ] **Write failing test** `TestFlattenLineAndCubic` in `raster_test.go`: a `MoveTo(0,0).LineTo(10,0)` flattens to 2 points; a shallow cubic flattens to a polyline whose points all lie within `tol` of the true curve (sample a few `t`).
- [ ] **Run** `go test -run TestFlatten ./...` → FAIL (undefined).
- [ ] **Implement** in `raster_path.go`:
  - `type point struct{ x, y float64 }`
  - `type subpath struct{ pts []point; closed bool }`
  - `type devPath struct{ subs []subpath }`
  - a builder `flattener{tol float64}` with `moveTo/lineTo/cubicTo/quadTo/close` that adaptively subdivides cubics/quadratics until the control-point deviation < `tol`. Reuse the bezier split maths style from `vector.go`.
- [ ] **Run** test → PASS.
- [ ] **Commit** `feat(render): device-space path flattening (P0)`.

### Task P0.2 — Coverage rasterizer (fill)

**Files:** Create `raster.go`; extend `raster_test.go`.

- [ ] **Write failing test** `TestFillSolidRect`: rasterize the rectangle [2,2]-[8,8] into a 10×10 buffer (nonzero), assert interior pixel (5,5) coverage == 1, exterior (0,0) == 0.
- [ ] **Run** → FAIL.
- [ ] **Implement** `raster.go`:
  - `type rasterizer struct{ w, h int; area, cover []float32 }`
  - `newRasterizer(w, h)`; `reset()`
  - `addLine(p0, p1 point)` accumulating signed area/cover deltas per the scanline signed-area algorithm
  - `addPath(dp *devPath)`
  - `coverage(rule fillRule) []float32` integrating area/cover across scanlines and applying nonzero/even-odd
  - `type fillRule int` (`fillNonZero`, `fillEvenOdd`)
- [ ] **Run** → PASS.
- [ ] **Add tests** `TestFillCircleCoverage` (disc centre ≈1, edge ≈0.5±tol via AA), `TestEvenOddStar` (self-intersecting star: centre hole empty under even-odd, filled under nonzero).
- [ ] **Run** all P0 tests → PASS.
- [ ] **Commit** `feat(render): anti-aliased coverage rasterizer (P0)`.

### Task P0.3 — Compositing + clip mask

**Files:** extend `raster.go`, `raster_test.go`.

- [ ] **Write failing test** `TestCompositeOverWhite`: fill a 50%-coverage red over a white RGBA, assert the pixel is a pink halfway between red and white (within 1/255).
- [ ] **Run** → FAIL.
- [ ] **Implement**:
  - `compositeCoverage(dst *image.RGBA, cov []float32, col color.RGBA, clip []float32)` — premultiplied `src-over`, multiplying coverage by the optional clip mask.
  - `type clipMask = []float32` and `intersectClip(a, b clipMask) clipMask`.
- [ ] **Run** → PASS; add `TestClipMaskIntersect`.
- [ ] **Commit** `feat(render): coverage compositing + clip masks (P0)`.

### Task P0.4 — Close out P0

- [ ] **Run** `go test ./...` and `go build ./...` → green.
- [ ] **Update** CHANGELOG `[Unreleased]` with an internal note (no public API yet).
- [ ] **Close** bead pdf-go-g3g; commit.

---

## Phase P1 — Vector rendering + API + PNG/JPEG/GIF (bead pdf-go-0ef)

First end-to-end page render. Depends on P0.

### Task P1.1 — Device transform + blank page

**Files:** Create `render.go`, `render_device.go`, `render_test.go`.

- [ ] **Write failing test** `TestRenderBlankPageDimensions`: `NewDocument(200,100)`, render at 150 DPI, assert image bounds are `round(200/72*150) × round(100/72*150)` and every pixel is white.
- [ ] **Run** → FAIL.
- [ ] **Implement**:
  - `RenderOptions{DPI float64; Background *Color}`; `const DefaultDPI = 150`.
  - `(*Page).RenderImage(opts) (image.Image, error)`: compute the CropBox (fallback MediaBox), build the base device matrix (Y-flip, scale, rotate), allocate `*image.RGBA`, fill background (white), run the (initially empty) interpreter, return.
- [ ] **Run** → PASS.
- [ ] **Commit** `feat(render): page render scaffold + device transform (P1)`.

### Task P1.2 — Path construction + fill painting

**Files:** extend `render.go`, `render_test.go`.

- [ ] **Write failing test** `TestRenderFilledRectangle`: a page with `DrawRectangle` (red fill) → render → interior pixel red, outside white.
- [ ] **Run** → FAIL.
- [ ] **Implement** the interpreter over `parseContentStream`: graphics state (`gstate{ctm, fill, stroke, lineWidth, …}`) + `q/Q/cm`; path ops `m/l/c/v/y/re/h`; painting `f/F/f*` (fill via rasterizer with current fill colour through `CTM·base`); colour ops `g/rg/k/sc/scn` (+ stroke variants store for P1.3); `n` (no-op paint / clip placeholder).
- [ ] **Run** → PASS; add `TestRenderEvenOddFill`.
- [ ] **Commit** `feat(render): fill painting + colour + CTM (P1)`.

### Task P1.3 — Basic stroking

**Files:** Create `raster_stroke.go`; extend tests.

- [ ] **Write failing test** `TestRenderStrokedLine`: `DrawLine` width 4 → a band of stroke-coloured pixels of ≈4px width at 72 DPI centred on the line; pixels off the line are background.
- [ ] **Run** → FAIL.
- [ ] **Implement** a simple correct stroker: expand each segment to a rectangle of half-width on each side; round joins/caps approximated by short arcs (polygon). Paint `S/s/B/B*` (B = fill then stroke). Good enough for P1; P6 refines.
- [ ] **Run** → PASS.
- [ ] **Commit** `feat(render): basic path stroking (P1)`.

### Task P1.4 — Encoders + devices

**Files:** extend `render_device.go`; `render_device_test.go`.

- [ ] **Write failing test** `TestRenderPNGDecodes`: `RenderPNG` to a buffer, decode with `image/png`, assert bounds + a known pixel colour. Same for JPEG (loose colour tolerance) and GIF.
- [ ] **Run** → FAIL.
- [ ] **Implement** `RenderPNG/RenderJPEG(…,quality)/RenderGIF` (GIF via a median-cut or `palette.Plan9` quantizer), `(*Document).RenderImage(pageNum, opts)`, and Aspose wrappers `Resolution`, `PngDevice`/`JpegDevice`/`GifDevice` with `Process(page, w)`.
- [ ] **Run** → PASS.
- [ ] **Commit** `feat(render): PNG/JPEG/GIF encoders + devices + Document.RenderImage (P1)`.

### Task P1.5 — Docs + close

- [ ] **Update** CLAUDE.md (new `render*.go` Public API section), README (a "Rendering to images" section + feature bullet), CHANGELOG.
- [ ] **Add** an example `_examples/render/main.go` (open a PDF, render page 1 to PNG).
- [ ] **Run** `go test ./...`, `go build ./...`, build all examples → green.
- [ ] **Close** bead pdf-go-0ef; commit.

---

## Phase P2 — Images + BMP (bead pdf-go-4zq) — *detail when started*

- Image XObject (`Do`): inverse-map device pixels into image space via the CTM; nearest + bilinear sampling; honour `/SMask` alpha and `/ImageMask`.
- Inline images (`BI/ID/EI`) via the existing inline parser.
- Reuse `image_decode.go` for pixel decode (DCT/Flate, colour spaces).
- `bmp.go`: uncompressed BMP encoder; `(*Page).RenderBMP` + `BmpDevice`.
- Tests: render a page with an embedded image, assert sampled pixels.

## Phase P3 — Text on embedded fonts (bead pdf-go-6sh) — *detail when started*

- `render_glyph.go`: parse `glyf` outlines (quadratic) for a GID incl. composite glyphs; transform by font-size · text-matrix · CTM · base; fill.
- `render_text.go`: text-state machine (`BT/ET/Tf/Td/TD/Tm/T*/Tj/TJ/'/"/Tc/Tw/Tz/TL/Ts/Tr`); advance via existing width logic; Type0/CID + simple-font code→GID.
- Rendering modes: fill (0) first; stroke/invisible later.
- Tests: render `AddText` output, assert glyph pixels present at expected positions.

## Phase P4 — Standard-14 outlines (bead pdf-go-9az) — *detail when started*

- Choose a permissively licensed metric-compatible family; embed needed glyph
  outlines as Go data; map the 14 PostScript names + encodings to glyphs.
- Tests: render Helvetica/Times/Courier text without embedding, assert pixels.

## Phase P5 — Clip / transparency / shadings (bead pdf-go-x1q) — *detail when started*

- Clip (`W/W*`): build a coverage mask from the clip path, intersect into the
  state; apply to all subsequent paints.
- Constant alpha (`gs` → `ca`/`CA`); soft masks (`/SMask` groups) best-effort.
- Axial/radial shadings (`sh` + shading patterns) rasterized per-pixel.

## Phase P-TIFF — Whole-document render + multi-page TIFF (bead pdf-go-a97) — *detail when started*

- `render_document.go`: `(*Document).RenderTIFF(w, opts, pages…)`; page-range loop.
- `tiff.go`: baseline TIFF encoder (single + multi-page IFD chain), packbits or
  uncompressed; `TiffDevice.Process(doc, w)`.

## Phase P6 — Stroking / quality / perf (bead pdf-go-b9x) — *detail when started*

- Proper joins (miter/round/bevel) + caps (butt/round/square) + dash arrays.
- AA quality tuning; performance: tile the coverage buffer, cut allocations,
  optional per-tile concurrency.

---

## Self-review checklist (per phase before closing its bead)

- All new files carry the SPDX header; code lives in `package asposepdf`.
- `go test ./...` and `go build ./...` green; all `_examples/*` build.
- CLAUDE.md + README + CHANGELOG updated for any new public API.
- No external dependencies introduced (`go list -m all` shows only the module).
