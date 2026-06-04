# Render PDF Pages to Raster Images — Design

**Beads:** umbrella [pdf-go-61r](bd show pdf-go-61r) — Render PDF pages to raster images (PNG/JPEG/GIF/BMP/TIFF)
**Phase tasks:** P0 [pdf-go-g3g] · P1 [pdf-go-0ef] · P2 [pdf-go-4zq] · P3 [pdf-go-6sh] · P4 [pdf-go-9az] · P5 [pdf-go-x1q] · P-TIFF [pdf-go-a97] · P6 [pdf-go-b9x]
**Date:** 2026-06-04
**Status:** Design approved

---

## Overview

Render a PDF page to a raster image — the single largest capability the library
is missing. This is, in effect, a pure-Go PDF rasterizer: a content-stream
interpreter that paints vector paths, text glyphs, and images onto a pixel
buffer with anti-aliasing.

The defining constraint is the library's identity: **pure Go, standard library
only, zero external dependencies** (CLAUDE.md, README). We therefore write our
own anti-aliased rasterizer and glyph-outline engine from scratch, using only
`image`, `image/png`, `image/jpeg`, `image/gif`, `image/draw`, and `math`.
BMP and TIFF encoders are also written in-house (stdlib has none), so the
zero-dependency promise holds for every output format.

## Goals

- Render a single page to an in-memory `image.Image` at a caller-chosen DPI.
- Encode to **PNG, JPEG, GIF** (stdlib) and **BMP, TIFF** (own encoders).
- Multi-page TIFF: render a whole document into one file (Aspose's signature
  `TiffDevice` use case).
- Hybrid API: an idiomatic Go core (`(*Page).RenderImage`) plus thin
  Aspose.PDF-for-.NET-style device wrappers (`PngDevice`, `Resolution`, …).
- Reasonable fidelity for real-world PDFs: vector graphics, raster images, and
  text (embedded fonts first, then the Standard 14).

## Non-goals (for this epic)

- A standards-perfect renderer. We target "looks right" for common PDFs, not
  pixel-exact ISO conformance.
- Blend modes beyond Normal; knockout/isolated group subtleties; overprint.
- JavaScript, form-field interactive rendering states, annotations' dynamic
  appearances beyond their `/AP/N` (which already exists and can be drawn).
- ICC color management (ICCBased is approximated by its component count, as
  today in image extraction).
- Vector output (EMF/SVG-out), WebP (needs VP8), printing.
- GPU/hardware acceleration.

## Dependency strategy

**Strict stdlib.** No `golang.org/x/image`, no font libraries. Consequences:

- We implement the anti-aliased rasterizer (scanline signed-area coverage).
- We implement glyph-outline rasterization from the `glyf` table we already
  parse for embedding/subsetting.
- Standard-14 fonts have no outlines (only metrics); we bundle metric-compatible
  open outlines (Nimbus/Liberation, permissive licence) as Go data in a later
  phase (P4).

This is the most work but preserves the brand's core promise and gives total
control over rendering quality.

## Architecture (bottom-up)

| Layer | Files | Responsibility |
|---|---|---|
| Rasterizer | `raster.go` | AA polygon fill (nonzero + even-odd) via signed-area coverage; composite source colour over an `*image.RGBA` with per-pixel coverage/alpha |
| Path flattening | `raster_path.go` | Cubic/quadratic bezier + arc → polyline at a flatness tolerance scaled by device resolution; reuses the bezier maths from `vector.go` |
| Stroking | `raster_stroke.go` | Stroked path (width, caps, joins, dashes) → fill outline polygon, then fill |
| Interpreter | `render.go` | Graphics-state machine (CTM stack, colours, line params, clip); dispatches content operators from the existing `parseContentStream` |
| Glyphs | `render_glyph.go`, `render_text.go` | `glyf` outlines of embedded TTF → device space → fill; text-state machine reusing the positioning maths from the text extractor |
| Std-14 outlines | `render_std14*.go` | Bundled metric-compatible outlines mapped to the 14 standard names |
| Images | `render_image.go` | Decoded pixels (reuse `image_decode.go`) → CTM inverse-mapped sampling + `/SMask` alpha |
| Clip/alpha/shading | `render_clip.go`, `render_gstate.go`, `render_shading.go` | Clip-path intersection, constant alpha (`ca`/`CA`), soft masks, axial/radial shadings |
| Encoders | `bmp.go`, `tiff.go` | In-house BMP + (multi-page) TIFF encoders |
| API / devices | `render_device.go`, `render_document.go` | Public `RenderImage`/`RenderPNG`/… + Aspose-style devices |

### Coordinate system

PDF user space is Y-up, origin bottom-left; raster images are Y-down, origin
top-left. The base device matrix maps the rendered region (CropBox, falling
back to MediaBox) to pixels:

```
scale = dpi / 72
W_px  = round(boxWidthPt  * scale)
H_px  = round(boxHeightPt * scale)
base  = translate(0, H_px) · scale(scale, -scale) · translate(-cropLLX, -cropLLY)
```

Page `/Rotate` (0/90/180/270) is composed into `base` (swapping W_px/H_px for
90/270). Every content-space coordinate is transformed by `CTM · base` before
rasterization.

### Rasterizer algorithm (P0)

Signed-area coverage accumulation (the approach behind `x/image/vector` and
stb_truetype, reimplemented):

1. Flatten the path to line segments in device space.
2. For each segment, accumulate signed partial-coverage deltas into a
   `width*height` float buffer (area + cover arrays per scanline).
3. Integrate across each scanline to get per-pixel coverage in [0,1].
4. Apply the fill rule (nonzero: clamp |acc|; even-odd: triangle wave).
5. Composite: `dst = src*α*cov + dst*(1 − α*cov)` (premultiplied), clipped to
   the current clip mask.

A clip is itself a coverage mask (an `[]float32` or `*image.Alpha`); nested
clips multiply masks. This unifies clipping with AA fill.

### API design (hybrid)

Idiomatic core:

```go
type RenderOptions struct {
    DPI        float64 // 0 → DefaultDPI (150)
    Background *Color  // nil → opaque white
}

func (p *Page) RenderImage(opts RenderOptions) (image.Image, error)
func (p *Page) RenderPNG(w io.Writer, opts RenderOptions) error
func (p *Page) RenderJPEG(w io.Writer, opts RenderOptions, quality int) error
func (p *Page) RenderGIF(w io.Writer, opts RenderOptions) error
func (p *Page) RenderBMP(w io.Writer, opts RenderOptions) error           // P2
func (d *Document) RenderImage(pageNum int, opts RenderOptions) (image.Image, error)
func (d *Document) RenderTIFF(w io.Writer, opts RenderOptions, pages ...int) error // P-TIFF
```

Aspose-style wrappers (mirror .NET; thin shims over the core):

```go
type Resolution struct{ DPI float64 }      // NewResolution(dpi)
type PngDevice  struct{ /* resolution */ } // NewPngDevice(Resolution)
func (dev *PngDevice) Process(page *Page, w io.Writer) error
// JpegDevice, GifDevice, BmpDevice, TiffDevice analogously;
// TiffDevice.Process(doc, w) renders all pages into one multi-page TIFF.
```

Default DPI = **150** (Aspose's default `Resolution`).

## Output formats

| Format | Encoder | Notes |
|---|---|---|
| PNG | `image/png` | Lossless, alpha — best for text/vector |
| JPEG | `image/jpeg` | Lossy, photographic pages; quality arg |
| GIF | `image/gif` | 256-colour; needs quantization (median-cut or `palette.Plan9`); documented limitation |
| BMP | own (`bmp.go`) | Uncompressed BGRA/BGR; trivial |
| TIFF | own (`tiff.go`) | Baseline, single- and multi-page; whole-document → one file |

## Phasing

| Phase | Bead | Scope | Milestone |
|---|---|---|---|
| P0 | g3g | Rasterizer + flattening + compositing | Unit tests (triangle/circle/coverage); no PDF |
| P1 | 0ef | Vector interpreter + colours + CTM/q/Q + basic stroke + **API + PNG/JPEG/GIF devices** | **First page render** |
| P2 | 4zq | Image/inline XObjects + SMask + **BMP** | Images render |
| P3 | 6sh | Embedded-TTF glyph rasterization + text state | Modern PDFs render |
| P4 | 9az | Bundled Standard-14 outlines | Helvetica/Times/Courier render |
| P5 | x1q | Clip + constant alpha + soft masks + shadings | Fidelity polish |
| P-TIFF | a97 | Whole-document render + **multi-page TIFF** | Archive/fax output |
| P6 | b9x | Stroke joins/caps/dash + AA quality + performance | Production-ready |

Each phase is independently shippable, committed, and tested. P0–P1 yield the
first visible result; P3 delivers practical value; P4+ raise fidelity.

## Testing strategy

- **P0**: pure unit tests — fill known shapes, assert coverage at sampled
  pixels (e.g. centre of a filled disc ≈ 1.0, well outside ≈ 0.0, edge ≈ 0.5),
  even-odd vs nonzero on a self-intersecting star.
- **P1+**: render synthetic PDFs built via the existing public API
  (`NewDocument` + `DrawRectangle`/`AddText`/`AddImage`) and assert pixel
  colours at known locations (e.g. a red rectangle's interior is red, the
  margin is white). This dogfoods our own drawing API as the oracle and needs
  no external reference renderer.
- **Golden images**: a few committed reference PNGs for regression, compared
  with a tolerance (per-channel mean abs diff) to absorb sub-pixel AA jitter.
- Real-world smoke: render a handful of `testdata/` pages and assert
  non-blank output + expected dimensions (ask which files before hardcoding).

## Risks & mitigations

- **Rasterizer correctness/quality** — the hardest core. Mitigate by isolating
  P0 with thorough unit tests before any PDF wiring.
- **Stroking** is deceptively hard (offset joins/caps/dashes). P1 ships a
  simple-but-correct stroker (round joins/caps via polygon approximation); P6
  refines miter/bevel + dashes.
- **Standard-14 licensing/size** — pick a permissively licensed metric-compatible
  family (Liberation = OFL/GPL+exception; URW base35 = AFPL/GPL — must verify);
  embed only the needed glyphs to bound binary size. Isolated in P4.
- **Performance** — naive full-frame float buffers are memory-heavy at high DPI.
  P6 addresses tiling/allocation; earlier phases prioritise correctness.
- **Scope creep** — transparency groups and shadings can balloon. P5 is
  explicitly best-effort; exotic cases degrade gracefully (skip, not crash).

## Out of scope (revisit later)

Blend modes, overprint, knockout groups, ICC management, JPEG2000/JBIG2 image
filters (already passthrough/limited in extraction), pattern tiling fidelity,
text rendering modes 4–7 (clip-from-text), WebP/EMF output.
