# PDF → HTML export, phases 2–4 — design (epic pdf-go-rfom)

Date: 2026-07-08. Outcome of the phase-2+ brainstorming session (phase 1 —
the faithful raster-background mode — shipped and approved earlier).

## End-state vision (decided)

The converter ultimately **drops the raster page background entirely**: every
piece of page content is expressed with native HTML/SVG — text as visible
styled spans, images as `<img>`/`<image>`, vector graphics as one inline
`<svg>` layer per page. The raster background of phases 1–3 is a temporary
crutch that shrinks phase by phase until it disappears.

## Roadmap

### Phase 2 — visible-text mode (`HTMLModeText`)

- **API**: `HTMLSaveOptions.Mode HTMLMode` — `HTMLModeFaithful` (zero value,
  current behaviour) | `HTMLModeText`. One `SaveHTML`/`WriteHTML` surface,
  mirroring the spirit of Aspose.PDF for .NET's `HtmlSaveOptions`.
- **Background**: the page rendered with the existing `renderer.suppressText`
  flag — all graphics and images, no glyphs (text-clip modes Tr 4–7 still
  honoured).
- **Text**: visible spans from `ExtractTextWithLayout` — real `color`
  (`TextFragment.Color`), `font-size`, bold/italic, family (generic class or
  named family), positioned as in phase 1.
- **Width fitting** (decided: scaleX + letter-spacing, the pdf2htmlEX
  approach): compute the fragment's natural width in the browser-substitute
  face using our bundled metric-compatible clone metrics (Arimo ≈
  Arial/Helvetica, Tinos ≈ Times, Cousine ≈ Courier — same advances as what
  browsers substitute), then correct the bulk difference with
  `transform: scaleX(pdfWidth/naturalWidth)` and distribute the residual with
  `letter-spacing`. `TextFragment.Width` is the ground truth.
- **Bonuses (decided in)**:
  - Link annotations → positioned `<a>` overlays: `/URI` actions → external
    links, `/GoTo` → `#pageN` anchors. Applies to both modes.
  - `HTMLSaveOptions.Pages` (page selection) for exporting a subset.
  - `loading="lazy"` on page background `<img>` so large documents paint
    progressively.

### Phase 3 — WOFF font embedding

Before the background can go away, visible text must use the document's own
fonts: convert embedded TTF/CFF programs to WOFF1 (zlib-compressed sfnt
tables — pure Go, we already have the sfnt parser/assembler and subsetter),
subset to the glyphs used, emit `@font-face` with base64 `data:` URLs.
Fragments then reference the real face; scaleX correction becomes ~1.0 for
embedded fonts and stays as fallback for non-embedded ones.

### Phase 4 — no-background mode (native HTML)

- **Vectors** (decided: one inline `<svg>` layer per page): a new content-
  stream backend that emits SVG paths/gradients/clips instead of raster
  coverage — reusing the same interpreter machinery as the renderer. SVG
  expresses nearly all PDF graphics (clips, axial/radial shadings, most blend
  modes via `mix-blend-mode`).
- **Images** (decided: in this phase, not phase 2): placed as `<image>`
  elements *inside* the SVG layer at their content-stream position, so
  z-order against vectors is naturally correct. JPEG stays JPEG (big win on
  scans); full CTM (rotation/skew) honoured via SVG transforms.
- **Fallback for inexpressible content** (decided: per-element raster
  patches): anything HTML/SVG can't express — soft masks, knockout groups,
  exotic blend modes, Type3 glyph programs, etc. — is rendered by our own
  rasterizer into a small PNG covering exactly its bbox and inserted as a
  positioned `<image>` at the right z-position in the flow. The page stays
  native; degradation is local, never whole-page.
- **Open question** (to settle during phase 4): text sits in the HTML layer
  above the SVG — PDF content painted *over* text would z-order incorrectly;
  evaluate emitting such runs as SVG `<text>` or accepting the (rare)
  mismatch.

## Rejected alternatives (with reasons)

- *Separate methods per mode* (`SaveHTMLText`…) — rejected: options-struct
  mode switch matches Aspose's `HtmlSaveOptions` shape and keeps one entry
  point.
- *letter-spacing-only width fitting* — rejected: large metric gaps create
  ugly inter-glyph gaps; negative spacing degrades ligatures/readability.
- *Whole-page faithful fallback* when a page contains inexpressible content —
  rejected: one exotic object shouldn't rasterize the entire page.
- *CSS-div vector representation* (rect/line as styled divs) — rejected: two
  drawing mechanisms with painful z-ordering between them; SVG alone covers
  everything.
- *Images as `<img>` already in phase 2* — rejected: without the SVG layer
  there is no natural place in the z-order (image under/over background
  vectors breaks on overlap), and CTM capture for rotated images isn't
  wired yet.

## Existing building blocks

- `renderer.suppressText` (render.go / render_text.go) — shipped with phase 1.
- `ExtractTextWithLayout` → `TextFragment{X, Y, Width, FontName, FontSize,
  Bold, Italic, Color, CharSpacing}` — everything the visible span needs.
- Bundled clone metrics (Arimo/Tinos/Cousine/Carlito) — the browser-width
  estimator for scaleX.
- sfnt parse/assemble + `SubsetFonts` — the WOFF1 converter's substrate.
- The content-stream interpreter (renderer) — the SVG backend's substrate.
- `Page.Annotations()` + `LinkAnnotation.Action()` — the `<a>` overlay source.
