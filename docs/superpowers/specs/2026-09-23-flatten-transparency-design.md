# Document.FlattenTransparency design

Bead: pdf-go-bqxs. Mirrors Aspose.PDF for .NET's `Document.FlattenTransparency()`.
Needed standalone, and eventually by PDF/A-1 conversion (pdf-go-ovc0), since
ISO 19005-1 forbids transparency entirely (`ValidatePDFA`'s `TRANSPARENCY`
issue, PDF/A-1 only).

## Scope decision (v1)

Per-page, whole-page rasterization: a page whose content stream's resource
graph uses any transparency construct (transparency group, `/SMask`,
non-Normal blend mode, `/ca`/`/CA` < 1) is rendered to an opaque raster via
the existing built-in renderer and its content stream is replaced with a
single full-page image. Pages that don't use transparency are left
completely untouched — still vector, still searchable.

Rejected alternative: sub-region flattening (render only the transparent
region, splice it in as one Image XObject, leave the rest of the page's
content stream as-is). This is what Aspose's own docs gesture at and is
strictly more faithful, but the codebase has no "render up to a point in
z-order with an opaque backdrop, then resume emitting vector ops" machinery,
and content-stream ops for a transparency region are not generally
separable from surrounding ops (they can interleave in z-order with vector
content above and below). Building that is a much larger, higher-risk
effort for uncertain payoff over the simpler per-page approach. Tracked as
a possible v2 if real-world use shows whole-page rasterization is too
lossy in practice.

**Explicitly out of scope for v1**: annotation-level transparency (an
annotation's own opacity, or `/SMask`/`/BM`/`/ca` inside its `/AP`
appearance stream). Detection only walks the page's content + `/Resources`
graph, not `/Annots`. An annotation's appearance is a self-contained Form
XObject with its own resources — it isn't touched by replacing the page's
content stream, and it keeps rendering live on top exactly as before. A
caller who also needs annotation transparency gone can call
`Annotation.Flatten()`/`Form.Flatten()` first to bake annotations into page
content (which then *does* get caught by this pass), then
`FlattenTransparency()`. Documented as a known limitation, not silently
dropped.

## API

```go
type FlattenTransparencyOptions struct {
    DPI   float64 // 0 -> DefaultDPI (150)
    Pages []int   // 1-based; empty -> all pages
}

func (d *Document) FlattenTransparency(opts ...FlattenTransparencyOptions) (int, error)
```

Returns the count of pages actually rasterized (0 if none needed it).
Mutates the receiver in place, matching `RemoveUnusedObjects`/
`OptimizeImages`/`ConvertToGrayscale`/`Flatten` — not the "returns a new
Document" pattern used by page-manipulation ops like `Split`/`Extract`.

## Detection

Reuse, don't reinvent:
- `collectValueDepsDoc(objects, pageDict["/Resources"], deps, visited)`
  (doc.go) walks the resource dict tree recursively (Form XObjects'
  `/Resources`, Patterns, etc.) — already used by imposition/form-import
  code for exactly this kind of graph walk.
- `pdfaDictHasTransparency(objects, dict)` (validate_pdfa.go) is already
  the exact four-key check needed (`/Group /S /Transparency`, `/SMask` ≠
  `/None`, non-Normal `/BM`, `/ca`/`/CA` < 1). Reused as-is (may need
  exporting from the validator's internal use to a shared helper, or just
  called directly since it's in the same package).

A page needs flattening if `pdfaDictHasTransparency(objects, pageDict)`
(covers a page-level `/Group`) or any object in the resource-graph walk
triggers it.

## Rasterize-and-replace mechanics

For each page that needs flattening:

1. Render page **content only, no annotations** at `opts.DPI` via a new
   internal renderer flag `skipAnnotations` (mirrors the existing
   `suppressText`/`hideFormWidgets` flags in render.go; gates the
   unconditional `rd.renderAnnotations()` call at render.go:155). Without
   this, annotations would be baked into the raster *and* still render
   live from `/Annots` — doubled. `RenderImage`'s public two-bool internal
   entry point (`renderImage(opts, suppressText, hideFormWidgets)`,
   render_device.go:59) gains a third bool for this, called with
   `suppressText=false, hideFormWidgets=false, skipAnnotations=true`.
2. Encode the result as PNG (`image/png`). The renderer's output is always
   fully opaque (solid background + composited content, by construction of
   rasterization), so `image.RGBA.Opaque()` is true and Go's PNG encoder
   drops the alpha channel automatically — `createImageXObject` then
   creates no `/SMask`, so no transparency is reintroduced by the fix
   itself.
3. Build the image XObject (`createImageXObject`, image_add.go — the same
   helper `AddImage` uses) and register it into `doc.objects` directly,
   *without* touching the page dict yet. Only once that has fully
   succeeded: replace the page's `/Resources` with a fresh dict holding
   just this one image, and delete any page-level `/Group` (its content
   has been rasterized away — nothing left to group; missed in an earlier
   version, caught by code review — see `TestFlattenTransparencyClearsPageGroup`).
   This ordering means a failure anywhere in step 2-3 leaves the page
   completely untouched, rather than with `/Resources` already wiped but
   `/Contents` still referencing names that no longer exist.
4. Build one content stream: `q W 0 0 H cm /Im0 Do Q` where `W,H` are the
   render-box width/height and the `cm` also translates for a nonzero
   box origin (`LLX LLY`), then `replacePageContents(p, data)`
   (redact_apply.go:135 — already does "allocate one new stream object,
   point `/Contents` at it", reused verbatim) to swap it in.
5. Leave `/Annots`, `/MediaBox`, `/CropBox`, etc. untouched.

Old content-stream/resource objects (fonts, other XObjects, patterns) are
now unreachable from the page. `FlattenTransparency` calls
`RemoveUnusedObjects()` once at the end when it flattened at least one
page — found necessary during testing: `ValidatePDFA`'s `TRANSPARENCY`
check (`pdfaCheckTransparency`) is a blind sweep of every object in
`d.objects`, not a reachability walk, so an orphaned ExtGState/Group dict
left behind by a flattened page would still trip it. Skipped when nothing
was flattened, so a no-op call doesn't surprise a caller by sweeping
unrelated pre-existing orphans.

## Testing strategy

- A page drawn with the public API's own alpha support
  (`ShapeStyle`/`LineStyle` `Color.A < 1`, which already goes through
  `ensureExtGState` → `/ca`) gives a trivial, realistic transparency
  fixture with no low-level dict hacking needed.
- Page with transparency: after `FlattenTransparency`, its `/Resources`
  is a single Image XObject, `ExtractText` on it is empty (content is now
  a raster — an expected, documented trade-off), and `RenderImage` still
  shows non-white pixels (visual content survived).
- Page without transparency: content stream object is provably untouched
  (same extracted text, or an identity check on the underlying object).
- `ValidatePDFA(PDFA1B)`'s `TRANSPARENCY` issue clears once every
  transparency-using page has been flattened.
- An annotation (e.g. a link) on a flattened page survives untouched and
  is still present in `Page.Annotations()` afterward.
- Return count matches the number of pages actually rasterized.
- Round-trips through Save+Open.
- A page-level `/Group` (not producible through the public drawing API —
  `ShapeStyle.Color.A < 1` only ever produces a resource-level ExtGState
  `/ca` — so this pokes the page dict directly in an internal test): must
  be detected, rasterized, *and cleared*, and a second `FlattenTransparency`
  call on the same document must then flatten 0 pages (idempotency). Added
  after code review caught that an earlier version detected and rasterized
  such a page but left `/Group` in place, so `ValidatePDFA` kept flagging
  `TRANSPARENCY` and every repeated call re-flattened the page forever.
- A rotated page's replacement raster renders the same as the same page
  rotated but never flattened (small-tolerance byte diff, not exact
  equality — see the code comment on `renderContentForFlatten` for why
  exact equality isn't the right bar). Originally verified only by an ad
  hoc visual check outside the test suite; added as an automated
  regression test after code review flagged the gap.

## Follow-ups (not required for this bead)

- Wiring into `ConvertToPDFA(PDFA1B/PDFA1A)` so the pipeline can offer a
  one-call PDF/A-1-with-transparency-removed path. Natural, but the
  primary deliverable here is the standalone `Document` method; hooking it
  into the PDF/A pipeline is tracked under pdf-go-ovc0 as before.
- Annotation-appearance transparency (see Scope decision above).
- JPEG output option for smaller files on photographic content (PNG-only
  in v1, matching `RenderPNG`'s default).
