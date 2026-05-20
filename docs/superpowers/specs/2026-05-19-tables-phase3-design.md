# Tables Phase 3 — Design Specification

**Beads:** [pdf-go-8nv](bd show pdf-go-8nv)
**Date:** 2026-05-19
**Status:** Design proposed
**Phase 1:** [2026-05-19-tables-design.md](2026-05-19-tables-design.md) (shipped)
**Phase 2:** [2026-05-19-tables-phase2-design.md](2026-05-19-tables-phase2-design.md) (shipped)

---

## Goals

Three features bundled. All Aspose .NET parity where applicable.

1. **Image cells** — `Cell.SetImage(path)` / `SetImageFromStream(r)`. An image cell auto-fits to the column interior, scales proportionally, respects per-cell H/V alignment.
2. **Border edge de-duplication** — when adjacent cells (and the outer table border) emit identical border lines on a shared edge, only emit the line once. Fixes visible doubling under semi-transparent or thin borders.
3. **Row-level styling + convenience** — `Row.SetBackground`, `SetTextStyle`, `SetBorder`, `SetMargin` propagate to all cells in the row unless the cell overrides. Plus `Table.AddRows([][]string)` batch constructor.

### Non-goals (Phase 4+ candidates)

- Auto-fit column widths (content-driven) — separate subepic, requires constraint-solver
- Dash patterns on borders
- Per-side border width/color (one width+color per `BorderInfo`)
- Rowspan splitting across page breaks (still errors)
- Image cells with explicit pixel sizing (use cell width)
- Animated/gradient backgrounds
- Cell padding via percentage instead of points

---

## Public API additions

### Cell — image content

```go
// SetImage configures the cell to render an image instead of text. The image
// is read at AddTable time from the given file path. Format auto-detected
// (PNG or JPEG via magic bytes), matching (*Page).AddImage.
//
// If both SetText and SetImage are configured on the same cell, the image
// wins and the text is ignored.
//
// The image is sized to fit the cell interior (cell width minus margins),
// scaled proportionally. Row height auto-fit computes the resulting image
// height + top/bottom margins. Cell alignment (HAlign/VAlign) positions
// the image within any extra interior space.
func (c *Cell) SetImage(path string) *Cell

// SetImageFromStream is the io.Reader-based counterpart.
func (c *Cell) SetImageFromStream(r io.Reader) *Cell

// Image returns the configured image source. path is empty if the image was
// set from a stream (or if no image is configured). hasImage reports whether
// any image source is configured.
func (c *Cell) Image() (path string, hasImage bool)
```

### Row — styling layer

A new inheritance level between Table-level defaults and per-Cell overrides. Aspose .NET has `Row.DefaultCellTextState` and `Row.BackgroundColor`; we mirror the spirit.

```go
// SetBackground sets a row-level background color. Each cell in the row
// inherits this background unless the cell calls SetBackground itself.
func (r *Row) SetBackground(col *Color) *Row
func (r *Row) Background() *Color   // nil if unset

// SetTextStyle sets a row-level default text style. Each cell in the row
// inherits this style (overlaid on table.DefaultCellStyle) unless the cell
// calls SetTextStyle itself.
func (r *Row) SetTextStyle(s TextStyle) *Row
func (r *Row) TextStyle() *TextStyle   // nil if unset

// SetBorder sets a row-level default border. Cells inherit unless overridden.
func (r *Row) SetBorder(b BorderInfo) *Row
func (r *Row) Border() *BorderInfo   // nil if unset

// SetMargin sets a row-level default margin (padding). Cells inherit unless overridden.
func (r *Row) SetMargin(m MarginInfo) *Row
func (r *Row) Margin() *MarginInfo   // nil if unset
```

### Table — batch row constructor

```go
// AddRows is a convenience that creates one Row per inner slice and one Cell
// per string in that slice. Returns the added rows for further customization.
//
// Equivalent to:
//
//   for _, texts := range rows {
//       r := t.AddRow()
//       r.AddCells(texts...)
//   }
func (t *Table) AddRows(rows [][]string) []*Row
```

---

## Image cells — detailed semantics

### Sizing strategy

The cell's interior is `(interiorWidth, interiorHeight) = (sum(columnWidths[col..col+cs]) - margin.L - margin.R, rowHeight - margin.T - margin.B)`.

- During `computeRowHeights`: an image cell's natural pixel dimensions are decoded, then scaled to fit `interiorWidth`. The resulting image height (in points, at 72 DPI) plus margins becomes the cell's contribution to row height.
- During rendering: the image is drawn at `(scaledWidth, scaledHeight)` preserving aspect ratio. If `scaledHeight > interiorHeight` (rare — happens when row height was explicitly set too short), the image is further scaled down to fit interior height. Final size = `min(interiorWidth, interiorHeight * aspectRatio) × min(interiorHeight, interiorWidth / aspectRatio)`.
- Placement within the cell interior follows `HAlign` and `VAlign`:
  - `HAlignLeft` → image hugs left margin, `HAlignCenter` → centered in interior width, `HAlignRight` → hugs right margin
  - `VAlignTop` → hugs top margin, `VAlignMiddle` → centered, `VAlignBottom` → hugs bottom

### Decoding for measurement

The new `measureImage(path string, fromStream bool, src io.Reader) (naturalW, naturalH float64, err error)` helper decodes just the header (PNG IHDR or JPEG SOF markers) to get pixel dimensions without loading the full image. Falls back to full decode for unusual cases.

For stream sources: the stream is read once and buffered (since both `measureImage` at height-compute time and `(*Page).AddImage` at render time need access). The buffer lives on the Cell until the table is rendered.

### Format support

PNG and JPEG, matching `(*Page).AddImage`. Other formats → error at `SetImage` time (file path) or at render time (stream — magic-byte sniff).

### `text` + image precedence

If both `SetText("...")` and `SetImage(...)` are called, the image wins. Rationale: image is a more specific declaration; text would render below the image anyway, but there's no clean way to lay out both in the cell interior without ambiguous spacing rules. Aspose .NET behaves similarly.

### ColSpan and RowSpan with image cells

ColSpan: image scales to the SPANNED interior width (sum of N columns minus margins).

RowSpan: image scales by interior width as usual. The cell height is `sum(heights[i..i+rs-1])`; if the scaled image height exceeds that, further scaling down preserves aspect ratio. Excess interior space (if image height < spanned height) is alignment-positioned per `VAlign`.

### Image cells in repeating header rows

Allowed. Re-drawn on each continuation page. The image's source path is re-read on each draw (or the stream's buffered bytes reused).

---

## Border edge de-duplication — detailed semantics

### Edge tracking

A new `edgeSet` map[edgeKey]edgeStyle tracks emitted edges:

```go
type edgeKey struct {
    x1, y1, x2, y2 float64 // rounded to 3 decimals for float-equality
}
type edgeStyle struct {
    width float64
    r, g, b float64
}
```

When `drawBorderSides` (or its split variant) is about to emit a side:
1. Normalize: ensure `(x1, y1) ≤ (x2, y2)` lexicographically (so the same edge from either direction maps to the same key).
2. Look up `edgeKey` in the set.
3. If absent → emit + store style.
4. If present AND style identical → skip (dedup).
5. If present AND style differs → emit anyway. Both lines render; visual overlap is the caller's intent (different per-cell border styles).

### Multi-page interaction

The `edgeSet` is per-page (reset on continuation page). Edges drawn on page 1 don't affect dedup on page 2.

### Outer table border

After all cell borders are drawn on a page, the outer border (`drawOuterBorder`) goes through the same dedup. This means: if `table.DefaultCellBorder` and `table.Border` have identical style, the outer rectangle's sides that coincide with cell-edge sides on the perimeter get drawn only once. If they differ → both render (intentional).

### Implementation refactor

`drawBorderSides` currently emits all 4 sides in one block. To dedup per-side, split into `drawBorderSide(targetPage, side BorderSide, x1, y1, x2, y2, BorderInfo, edges *edgeSet) error`. The 4-side wrapper iterates and dispatches each side individually.

`drawRowRange` and `drawOuterBorder` accept `*edgeSet` parameter. `AddTable` creates one per page.

### Float-precision care

Cell coordinates are computed from accumulated sums of column widths and row heights. Two adjacent cells share `cellLLX = rect.LLX + xOffsets[col]` and `cellURX = rect.LLX + xOffsets[col]` from the right neighbor's perspective. These should be byte-equal in float64, but rounding to 3 decimals in the edge key absorbs any drift.

---

## Row-level styling — inheritance chain

After Phase 3, the effective-style chain is 4 deep:

| Property | Chain (top to bottom; lower wins) |
|---|---|
| TextStyle | zero `TextStyle{}` ← `table.defaultCellStyle` ← `row.textStyle` (if set) ← `cell.style` (if set) ← `cell.hAlign/vAlign` overrides |
| Background | nil ← `row.background` (if set) ← `cell.background` (if set) |
| Border | zero `BorderInfo{}` ← `table.defaultCellBorder` ← `row.border` (if set) ← `cell.border` (if set) |
| Margin | zero `MarginInfo{}` ← `table.defaultCellMargin` ← `row.margin` (if set) ← `cell.margin` (if set) |

### Phase 2 helpers updated

`effectiveCellStyle(t, c)`, `effectiveCellMargin(t, c)`, `effectiveCellBorder(t, c)` all extended to consult `c.row.textStyle` / `c.row.margin` / `c.row.border` between the table and cell layers. New helper `effectiveCellBackground(c) *Color` walks `cell.background` → `row.background` → nil.

### Background overlay

If `row.background == nil` and `cell.background == nil` → no fill (current behavior).
If `row.background != nil` and `cell.background == nil` → row background fills the cell.
If `cell.background != nil` → cell background wins (row is ignored for that cell).

There is no "blend" or "layered" rendering — last in chain wins, fully replacing earlier.

---

## `AddRows` — detailed semantics

```go
func (t *Table) AddRows(rows [][]string) []*Row {
    out := make([]*Row, len(rows))
    for i, texts := range rows {
        r := t.AddRow()
        r.AddCells(texts...)
        out[i] = r
    }
    return out
}
```

Returns the slice of Rows in order. Callers typically just iterate or zip with metadata, e.g.:

```go
rows := table.AddRows([][]string{
    {"Alice",   "Engineering", "23"},
    {"Bob",     "Marketing",   "17"},
    {"Carol",   "Operations",  "9"},
})
for _, r := range rows {
    r.SetBackground(&pdf.Color{R: 0.97, G: 0.97, B: 0.97, A: 1})
}
```

### Edge cases

- `AddRows(nil)` or `AddRows([][]string{})` → returns empty slice, no rows added.
- `AddRows([][]string{{"x"}, nil, {"y"}})` — inner nil treated as empty row (no cells). Validation at AddTable time then rejects the row for under-coverage unless `columnWidths` is empty.
- Span semantics not supported: rows added via AddRows never have ColSpan/RowSpan. For spans, use the explicit `AddRow` + `AddCell().SetColSpan(...)` flow.

---

## Aspose .NET parity table

| Aspose .NET | This library |
|---|---|
| `cell.Image = new Image { File = "logo.png" }` | `cell.SetImage("logo.png")` |
| `row.BackgroundColor = Color.LightGray` | `row.SetBackground(&pdf.Color{R: 0.83, G: 0.83, B: 0.83, A: 1})` |
| `row.DefaultCellTextState = new TextState { ... }` | `row.SetTextStyle(pdf.TextStyle{...})` |
| `row.MinRowHeight` (no direct equivalent in Aspose) — we already have `row.SetHeight` from Phase 1 | (existing) |
| Border edge dedup — Aspose's flow-layout renderer naturally dedupes shared edges | This library now matches: identical-style adjacent edges render once |
| `Table.AddRows` (no exact match — Aspose uses LINQ-style enumerable but with explicit Row construction) | `table.AddRows([][]string{...})` Go-idiomatic batch |

---

## Rendering pipeline updates

### File touch impact

| File | Change |
|---|---|
| `table.go` (modify) | Add image fields to `Cell`; add row-level style fields to `Row`; add `AddRows` on Table; methods + getters. |
| `table_render.go` (modify) | New `effectiveCellBackground` helper; existing effective* helpers extended for row layer; `edgeSet` + per-side `drawBorderSide`; image-cell rendering path in `drawRowRange`; `measureImage` integration in `computeRowHeights`. |
| `table_image.go` (new) | `measureImage(path string, fromStream bool, r io.Reader)` + image-cell drawing helper (delegates to existing `(*Page).AddImage` for the actual stream-write). |
| `table_test.go` (modify) | New tests for image cells, row styling, dedup, AddRows. |
| `table_internal_test.go` (modify) | New tests for effective* with row layer, edgeSet dedup logic. |
| `table_aspose_parity_test.go` (modify) | Add 3 parity tests for new APIs. |
| `CLAUDE.md` / `README.md` (modify, last task) | Phase 3 sections. |

---

## Validation rules (new)

| Check | Error |
|---|---|
| `cell.SetImage(path)` with empty path | `add table: row %d col %d: empty image path` |
| Image file unreadable at AddTable time | `add table: row %d col %d: image %s: %w` |
| Image format unsupported (not PNG/JPEG) | `add table: row %d col %d: unsupported image format` (sniff failure) |
| Image stream EOF without valid header | `add table: row %d col %d: image stream truncated` |

Validation at AddTable time. No partial drawing on failure.

---

## Testing strategy

### Image cells

1. `TestAddTable_ImageCellRendered` — single cell with `SetImage`, output PDF contains an XObject reference (or extract via `(*Page).ImageInfos()` shows 1 image).
2. `TestAddTable_ImagePrecedenceOverText` — cell with both text and image renders image only (text not in extraction).
3. `TestAddTable_ImageAutoFitRowHeight` — auto-fit row height matches `naturalH × (interiorW / naturalW) + margins`.
4. `TestAddTable_ImageWithColSpan` — image scales to spanned interior width.
5. `TestAddTable_ImageWithRowSpan` — image scales to spanned rows; fits within total height.
6. `TestAddTable_ImageFromStreamRoundTrip` — image set from `*bytes.Reader`, write+reopen, image present.
7. `TestAddTable_ImageInRepeatingHeaderEachPage` — image header repeats on every overflow page.
8. `TestAddTable_ImageInvalidPathErrors` — `SetImage("/nonexistent")` returns error from AddTable.

### Border edge de-duplication

9. `TestAddTable_BorderDedupIdenticalAdjacentEdges` — two adjacent cells with same BorderInfo; output stroke count = perimeter only, not perimeter + shared edge.
10. `TestAddTable_BorderNoDedupDifferentStyles` — two adjacent cells with different border widths; both edges stroke.
11. `TestAddTable_OuterBorderDedupWithCellBorder` — table outer border + cell borders identical; perimeter strokes once.
12. `TestAddTable_DedupResetsBetweenPages` — multi-page table; dedup operates per-page (regression test that page 2 still draws all edges).

### Row-level styling

13. `TestRow_SetBackground_AppliesToAllCells` — set row.SetBackground; render; all cells in that row have the background.
14. `TestRow_CellBackgroundOverridesRow` — cell.SetBackground overrides the row's background.
15. `TestRow_SetTextStyle_Inherited` — row.SetTextStyle(Size:18); cells in row render at 18pt.
16. `TestRow_SetMargin_Inherited` — row.SetMargin(MarginInfo{Top:10}); cells use 10pt top margin unless overridden.
17. `TestRow_CellMarginOverridesRow` — cell.SetMargin wins.
18. `TestRow_SetBorder_Inherited` — row.SetBorder propagates.
19. `TestRow_GettersReturnNilWhenUnset` — fresh row's getters all return nil.

### AddRows

20. `TestTable_AddRows_BatchAndReturn` — `AddRows([][]string{...})` adds rows with correct cells; returned slice matches added.
21. `TestTable_AddRowsEmpty` — `AddRows(nil)` and `AddRows([][]string{})` return empty slice, no rows added.

### Aspose parity

22. `TestAsposeParity_CellImage` — `SetImage("testdata/Koala.jpg")` mirrors `cell.Image = new Image{File="..."}`.
23. `TestAsposeParity_RowBackground` — `row.SetBackground` mirrors `row.BackgroundColor`.
24. `TestAsposeParity_AddRowsBatch` — convenience usage.

### Internal tests

25. `TestEffectiveCellStyle_RowLayer` — row's TextStyle overlays table default, cell overlays row.
26. `TestEffectiveCellBackground_PrecedenceChain` — cell > row > nil.
27. `TestEdgeSet_DedupIdenticalKey` — direct unit test on the edge-tracking map.
28. `TestEdgeSet_DifferentStylesNotDeduped`.

---

## Risks & mitigations

| Risk | Mitigation |
|---|---|
| Image header decode for measurement diverges from full decode at render time | Use shared decoder helper or accept buffered full-decode for MVP. Tests confirm dimensions match. |
| Float drift in edge-key matching | Round to 3 decimal places; rendering math is deterministic enough that adjacent edges produce identical rounded coords. |
| Row layer adds complexity to inheritance chain → harder to reason about | Document explicitly in CLAUDE.md + spec. Add internal tests pinning down precedence. |
| `Cell.SetImageFromStream` requires buffering the stream (read twice) | Read fully into `[]byte` on first access; share between measure + render passes. Memory cost = image file size, bounded. |
| AddRows shadows the explicit AddRow flow — confusing? | Document: AddRow for span-aware construction, AddRows for plain text matrices. Both compose with row-level styling helpers. |
| Image-cell + row-level background: image overdraws background fill? | Render order: row background fill → cell background fill (overrides row) → image → cell border. Image lies above background, below border — matches user expectation. |
| Border dedup breaks existing tests that count strokes | Update affected tests; the new count should match the visual reality (perimeter edges only). |

---

## File structure

| File | Purpose |
|---|---|
| `table.go` (modify) | Image fields on Cell; row-level style fields on Row; AddRows on Table; all getter/setter methods. |
| `table_render.go` (modify) | Extended `effective*` helpers; new `effectiveCellBackground`; per-side `drawBorderSide` + `edgeSet`; image rendering in `drawRowRange`. |
| `table_image.go` (new) | `measureImage(path, fromStream, r) (w, h, err)` + image rendering helper. |
| `table_test.go` (modify) | ~14 new tests across image, dedup, row layer, AddRows. |
| `table_internal_test.go` (modify) | ~4 internal tests on effective*-with-row + edgeSet. |
| `table_aspose_parity_test.go` (modify) | 3 parity tests. |
| `CLAUDE.md` (modify, last task) | Phase 3 bullets after Phase 2 block. |
| `README.md` (modify, last task) | Features sentence + snippet additions. |

Estimated: ~500 LOC production + ~600 LOC tests, in 12-14 TDD tasks.

---

## Self-review

**Placeholder scan:** every API method signature is concrete with parameter/return types. Error messages templated. Inheritance precedence explicit per property.

**Internal consistency:** `effectiveCellBackground` parallels `effectiveCellStyle`/`Margin`/`Border` — same chain pattern. `edgeSet` reset per page, consistent with existing per-page outer-border drawing.

**Scope check:** explicitly excludes auto-fit columns (separate subepic), dash patterns, per-side border config, rowspan-split. Three included features are reasonably independent — image cells touches rendering deep, dedup touches border helpers, row styling touches helpers + inheritance. Bundled because they're individually small and benefit from being doc'd/shipped together.

**Ambiguity check:** image + text precedence explicit (image wins). Auto-fit row height with image cells fully specified (width-driven scaling). Row-background + cell-image render order specified.
