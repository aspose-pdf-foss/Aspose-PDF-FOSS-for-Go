# Tables Phase 2 — Design Specification

**Beads:** [pdf-go-2h3](bd show pdf-go-2h3)
**Date:** 2026-05-19
**Status:** Design proposed
**Phase 1 spec:** [docs/superpowers/specs/2026-05-19-tables-design.md](2026-05-19-tables-design.md)

---

## Goals

Three features, one subepic. All Aspose.PDF for .NET-parity.

1. **Multi-page overflow** — when rows don't fit in the bounding rectangle, automatically add pages and continue drawing.
2. **Repeating header rows** — the first N rows can be configured to repeat at the top of every continuation page.
3. **Cell merging** — `Cell.SetColSpan(n)` and `Cell.SetRowSpan(n)` with implicit "covered" cells (the caller does not add cells in positions occupied by a span).

### Non-goals (Phase 3 candidates)

- Image cells (text-only stays in Phase 2)
- Border edge de-duplication (visual quality fix)
- Auto-fit column widths (fit-to-content)
- Dash patterns on borders
- Convenience methods (`AddRows([][]string)`, `Row.SetBackground`)
- Per-row default text style
- Rowspan that splits across page breaks (MVP: groups that don't fit on a full page → error)

---

## Public API additions

### Breaking change

```go
// Before (Phase 1):
func (p *Page) AddTable(t *Table, rect Rectangle) error

// After (Phase 2):
func (p *Page) AddTable(t *Table, rect Rectangle) (pagesAdded int, err error)
```

`pagesAdded` is the number of continuation pages automatically appended to the document (0 when the table fits in `rect`). The caller can ignore it via `_, err :=` if they don't care.

Memory policy: project allows breaking changes pre-1.0 (per `feedback_no_backwards_compat.md`).

### New Table-level methods

```go
// SetOverflowMargins configures the top + bottom margins (in points) used to
// compute the bounding rectangle on automatically-appended continuation pages.
// Defaults: top=50, bottom=50. The continuation rect uses rect.LLX / rect.URX
// (same horizontal position as the original rect) and Y from
// (pageHeight - top) down to bottom.
func (t *Table) SetOverflowMargins(top, bottom float64) *Table
func (t *Table) OverflowMargins() (top, bottom float64)

// SetRepeatingRowsCount marks the first n rows as headers that repeat at the
// top of every continuation page. Default 0 (no repeat). Mirrors Aspose .NET's
// Table.RepeatingRowsCount property.
//
// Validation at AddTable time:
//   - n must be 0 ≤ n ≤ RowCount()
//   - no rowspan that starts in [0..n-1] may extend into [n..]
func (t *Table) SetRepeatingRowsCount(n int) *Table
func (t *Table) RepeatingRowsCount() int
```

### New Cell-level methods

```go
// SetColSpan sets the column span (1 = single cell, 2 = spans two columns, etc.).
// Default 1. Mirrors Aspose .NET's Cell.ColSpan.
//
// Implicit covered cells: when ColSpan > 1, the caller does NOT add cells for
// the columns covered by the span. A row's effective column count is
// sum(cell.ColSpan for cell in row) + (cells covered by rowspans from prior rows).
func (c *Cell) SetColSpan(n int) *Cell
func (c *Cell) ColSpan() int  // returns 1 if unset

// SetRowSpan sets the row span (1 = single row, 2 = spans two rows, etc.).
// Default 1. Mirrors Aspose .NET's Cell.RowSpan.
//
// Implicit covered cells: the cells in covered positions in subsequent rows
// are NOT added by the caller. Row[i+k] for 1≤k<RowSpan has fewer cells.
func (c *Cell) SetRowSpan(n int) *Cell
func (c *Cell) RowSpan() int  // returns 1 if unset
```

---

## Multi-page overflow — detailed semantics

### Algorithm

After validation and `computeRowHeights`:

1. **Compute spanning groups.** A spanning group is the minimal consecutive sequence of rows `[s, e]` such that no rowspan starting within `[s, e]` ends past `e`. Sequence:
   ```go
   groups := []group{}
   i := repeatingRowsCount
   for i < numRows {
       g := group{start: i, end: i}
       j := i
       for j <= g.end {
           for _, cell := range rows[j].cells {
               spanEnd := j + cell.rowSpan - 1
               if spanEnd > g.end { g.end = spanEnd }
           }
           j++
       }
       groups = append(groups, g)
       i = g.end + 1
   }
   ```
   Each group is rendered as a single atomic unit — it cannot break across pages.

2. **Header group.** Rows `[0..repeatingRowsCount-1]` form the header group. Validated to be rowspan-complete (no rowspan in headers extends into body).

3. **Compute heights.** `groupHeight(g) = sum(heights[g.start..g.end])`. `headersHeight = sum(heights[0..repeatingRowsCount-1])`.

4. **Walk groups, breaking on overflow:**
   ```
   y := rect.URY
   drawHeaders(rect.LLX, y)
   y -= headersHeight
   currentRect := rect

   for each group g:
       if y - groupHeight(g) < currentRect.LLY:
           // overflow
           continuationRect := pageRectMinusMargins(t.overflowTop, t.overflowBottom)
           if groupHeight(g) > continuationRect.height - headersHeight:
               return 0, fmt.Errorf("add table: group [%d..%d] too tall for any page", g.start, g.end)
           pagesAdded++
           doc.AddBlankPageFromFormat(... same as current page ...)
           currentPage = the new page
           currentRect = continuationRect
           y = currentRect.URY
           drawHeaders(currentRect.LLX, y)  // repeat
           y -= headersHeight
       drawGroup(g, y)
       y -= groupHeight(g)
   ```

5. **Page format.** Continuation pages use the same dimensions as the page that received `AddTable`. Method: `doc.AddBlankPage(pageSize.Width, pageSize.Height)`.

6. **Outer border per page.** Drawn at the end of each page's content for the rows actually rendered on that page (using the existing drawn-height tracking from Phase 1 Task 8 fix-up).

### Continuation rect computation

```go
continuationRect = Rectangle{
    LLX: rect.LLX,
    LLY: t.overflowBottom,           // default 50
    URX: rect.URX,
    URY: pageHeight - t.overflowTop, // default 50 from top
}
```

If `pageHeight - top - bottom < headersHeight + minGroupHeight`, return error before drawing anything.

### Edge case: empty body after headers

If `repeatingRowsCount == numRows`, all rows are headers — they draw once, no overflow, no body. `pagesAdded = 0`.

### Edge case: original rect too small for headers

If `headersHeight > rect.URY - rect.LLY` → error before drawing anything.

### Edge case: single row taller than a full page

After overflow → group [g.start..g.end] still doesn't fit → error.

---

## Repeating headers — detailed semantics

- The first `RepeatingRowsCount()` rows are drawn at the top of every page (original + every overflow page).
- Header rows are NOT eligible for clipping — they're always rendered in full. If they don't fit in the available height, that's an error (caught at validation time).
- Header rows can use colspan; they cannot use rowspan that extends into the body (validation error).
- Style: header rows are styled exactly like any other rows. The repeat is purely positional. No automatic bold/background is applied — the caller controls via `cell.SetTextStyle`, `cell.SetBackground`, etc.

### Aspose parity note

Aspose.PDF for .NET's `Table.RepeatingRowsCount` controls the same thing. They also have `Table.RepeatingColumnsCount` (horizontal repeat) — we do **not** include the column variant in Phase 2. It's rare and adds complexity.

---

## Cell merging — detailed semantics

### ColSpan

- `cell.SetColSpan(n)` declares the cell occupies `n` consecutive columns starting at its current column position.
- The current column position is determined by the cells declared in the row plus the rowspans inherited from prior rows. (See "Covered cell grid" below.)
- A row's cells are placed left-to-right, skipping any positions covered by inherited rowspans.
- Interior width of the cell for auto-fit and text rendering: `sum(columnWidths[col..col+span])`.
- Validation: `col + cell.colSpan > len(columnWidths)` → error.

### RowSpan

- `cell.SetRowSpan(n)` declares the cell occupies `n` consecutive rows starting at the row it's in.
- In rows `[i+1..i+n-1]`, the columns occupied by this cell are skipped by the caller — no `*Cell` is added for those positions.
- Validation: `i + cell.rowSpan > numRows` → error.

### Covered cell grid

Computed in one O(rows × cols) pass before rendering:

```go
covered := make([][]bool, numRows)
for i := range covered { covered[i] = make([]bool, numCols) }

for i, row := range rows {
    col := 0
    for _, cell := range row.cells {
        // Skip columns already covered by inherited rowspans.
        for col < numCols && covered[i][col] { col++ }
        cs := cell.ColSpan()  // ≥ 1
        rs := cell.RowSpan()  // ≥ 1
        if col + cs > numCols { return error("colspan out of bounds row %d", i) }
        if i + rs > numRows  { return error("rowspan out of bounds row %d", i) }
        // Mark covered positions for future rows.
        for r := 1; r < rs; r++ {
            for c := 0; c < cs; c++ {
                if covered[i+r][col+c] { return error("merge overlap at row %d col %d", i+r, col+c) }
                covered[i+r][col+c] = true
            }
        }
        col += cs
    }
    // After processing all cells: every position in row i must be exactly covered.
    // Effective count = (col after loop) + sum of inherited rowspans landing on this row.
    // Validation: col + (inherited count) == numCols.
    inherited := 0
    for c := 0; c < numCols; c++ {
        if covered[i][c] && (c < col || /* TBD */ false) { /* skip */ }
    }
    // (Implementation detail — simpler approach: track a running "active rowspans"
    // and recompute coverage row-by-row. See File structure.)
}
```

(Implementation note: the actual loop in `table_render.go` will not literally use a `covered [][]bool` of full numRows × numCols; it'll iterate row-by-row, tracking active rowspans as `(col, remaining)` tuples. The grid above is the conceptual model.)

### Auto-fit row height with merging

- Cells with `rowSpan == 1` contribute to their row's auto-fit height as in Phase 1.
- Cells with `rowSpan > 1` are **excluded** from per-row max-height computation. Their content height is checked AFTER per-row heights are computed:
  - If `cellHeight ≤ sum(heights[i..i+rs-1])` → fine.
  - If `cellHeight > sum(heights[i..i+rs-1])` → for MVP, **clip the rowspan cell's content** (matches AddText's existing clip semantics). Document this. Caller can use explicit `Row.SetHeight` to allocate more space.
- Cells with `colSpan > 1` use the wider interior width (sum of N columns minus margins) for their auto-fit measurement.

### Rendering with merging

For each cell:
- `cellLLX = rect.LLX + sum(columnWidths[0..col])`
- `cellURX = cellLLX + sum(columnWidths[col..col+colSpan])`
- `cellURY = y` (top of current row)
- `cellLLY = cellURY - sum(heights[rowIdx..rowIdx+rowSpan-1])`

Background + text + border drawn for the merged rectangle as a single cell. The per-cell border bitmask still applies; the outer table border doesn't change.

### Aspose parity

`Cell.ColSpan` and `Cell.RowSpan` properties in Aspose.PDF for .NET; same semantics (1-based, implicit covered cells). API mirrors them.

---

## Combined: rowspan + page break interaction

**Hard rule for Phase 2:** a spanning group never crosses a page break.

Algorithm reuses the "spanning group" computation from Multi-page overflow. The group is the atomic unit. If a group's total height exceeds the available height on a full continuation page (after headers), return an error.

This avoids the complexity of partial-row rendering, mid-rowspan splits, or recomputing layouts.

### Headers + rowspan

Validation: no rowspan starting in `[0..repeatingRowsCount-1]` may end in `[repeatingRowsCount..]`. If it does, return an error at `AddTable` time:

> `add table: rowspan at header row %d extends into body (rowspan-cross-header not supported)`

---

## Validation rules (all checked before rendering)

| Check | Error message |
|---|---|
| `repeatingRowsCount < 0` | `add table: repeating rows count %d is negative` |
| `repeatingRowsCount > numRows` | `add table: repeating rows count %d exceeds row count %d` |
| `colSpan < 1` (any cell) | `add table: row %d col %d has colSpan %d (must be ≥ 1)` |
| `rowSpan < 1` (any cell) | `add table: row %d col %d has rowSpan %d (must be ≥ 1)` |
| `col + colSpan > numCols` | `add table: colspan at row %d col %d (span %d) exceeds column count %d` |
| `i + rowSpan > numRows` | `add table: rowspan at row %d (span %d) exceeds row count %d` |
| Merge overlap | `add table: merge overlap at row %d col %d` |
| Rowspan crosses header/body boundary | `add table: rowspan at header row %d extends into body` |
| Row doesn't fully cover columns | `add table: row %d covers %d/%d columns` (existing Phase 1 mismatch check, updated for span-aware counting) |
| Continuation rect too small for headers | `add table: continuation rect height %g < headers height %g` |
| Spanning group too tall for any page | `add table: group [%d..%d] height %g exceeds continuation page height %g` |

All validation happens before any rendering. If validation fails, no pages are added.

---

## Aspose .NET parity table

| Aspose .NET | This library |
|---|---|
| `table.RepeatingRowsCount = 1` | `table.SetRepeatingRowsCount(1)` |
| `cell.ColSpan = 3` | `cell.SetColSpan(3)` |
| `cell.RowSpan = 2` | `cell.SetRowSpan(2)` |
| Table auto-flows to next page when added via `page.Paragraphs.Add(table)` (no explicit page management) | `page.AddTable(t, rect)` auto-adds continuation pages; returns `(pagesAdded, err)` so caller knows |
| Aspose has no equivalent of `OverflowMargins` (page-flow layout handles it) | `table.SetOverflowMargins(top, bottom)` — explicit because we use Rectangle positioning |

The `(pagesAdded, err)` signature is a project-specific deviation from .NET's `page.Paragraphs.Add(table)` (which has no return). Reason: in our Rectangle-based paradigm, the caller specifies position explicitly and must know when their layout grew beyond expected. `pagesAdded > 0` lets them adjust subsequent operations (e.g., page numbering, post-table content placement).

---

## Rendering pipeline updates

### `table_render.go` rewrite scope

`AddTable` flow becomes:

```
1. Validate (nil, rect, columns, cell counts span-aware, repeat count, merge overlaps).
2. Compute covered grid.
3. Compute row heights (auto-fit excluding rowspan cells).
4. Validate rowspan cells fit (or accept clipping).
5. Compute spanning groups.
6. Walk groups:
   a. Draw headers if starting a new page.
   b. Draw group rows (cells respect colspan width + rowspan height).
   c. If next group doesn't fit, append a continuation page.
7. Draw outer border on each page for that page's rendered range.
8. Return (pagesAdded, nil).
```

### Per-cell rendering with span

Background, text, border all use the SPAN'D rectangle:

```go
cellLLX := rect.LLX + xOffsets[col]
cellURX := rect.LLX + xOffsets[col+cs]    // cs = ColSpan
cellURY := yRowTop[rowIdx]
cellLLY := yRowTop[rowIdx+rs]              // rs = RowSpan (yRowTop has numRows+1 entries)
```

Where `xOffsets[c] = sum(columnWidths[0..c])` and `yRowTop[r]` is the top-Y of row r on the current page.

The interior (margin-shrunk) rect for `AddText` uses these spanned coordinates.

### Continuation page creation

Use existing `(*Document).AddBlankPage(width, height)` (already in public API). The new page becomes the rendering target. `p` (the receiver of `AddTable`) is no longer the only target — we maintain a "current page" pointer through the loop.

```go
currentPage := p
currentRect := rect
// ... on overflow:
sz, _ := p.Size()
if err := p.doc.AddBlankPage(sz.Width, sz.Height); err != nil { return 0, err }
pagesAdded++
currentPage, _ = p.doc.Page(p.doc.PageCount())
currentRect = continuationRect
```

This requires `(*Page).doc *Document` access — already used internally for `LoadFont`, etc.

### Outer border per page

The outer border now needs to be drawn at the end of EACH page's contribution, not once at the end. Track per-page `drawnHeight`. The Phase 1 outer-border block becomes a closure invoked at every page boundary:

```go
drawOuterBorder := func(targetPage *pdfPage, r Rectangle, drawnH float64) {
    // existing Phase 1 outer-border code, parameterized
}
```

---

## Testing strategy

### Multi-page overflow (`table_test.go`)

1. `TestAddTable_OverflowAddsPage` — table that needs 1 continuation. Assert `pagesAdded == 1`, `doc.PageCount()` increased.
2. `TestAddTable_OverflowMultiplePages` — 50-row table with small rect → multiple continuations.
3. `TestAddTable_OverflowReturnsZeroIfFits` — table fits in rect → `pagesAdded == 0`, no new pages.
4. `TestAddTable_OverflowMarginsRespected` — `SetOverflowMargins(100, 100)` → continuation rect narrower vertically; row count per page lower.
5. `TestAddTable_OverflowContinuationRectXCoords` — continuation rect uses same LLX/URX as original rect.
6. `TestAddTable_OverflowContinuationPageSize` — continuation pages match the receiver page's dimensions.
7. `TestAddTable_OverflowHeadersTooTallErrors` — repeat rows + small margins → returns error before drawing.

### Repeating headers (`table_test.go`)

8. `TestAddTable_HeadersRepeatOnEachPage` — table with `SetRepeatingRowsCount(1)`, overflow to 3 pages. Extract text from each page — header row appears on all 3.
9. `TestAddTable_NoHeadersByDefault` — without `SetRepeatingRowsCount` → header NOT repeated.
10. `TestAddTable_HeadersDontConsumeOriginalRect` — verify header appears at rect.URY on the original page too.
11. `TestAddTable_HeadersWithStyle` — styled header (background, bold) round-trips across pages.

### Cell merging — colspan (`table_test.go`)

12. `TestAddTable_ColSpanWiderCell` — 3-column table, first cell ColSpan(2). Verify cell width = sum of 2 columns.
13. `TestAddTable_ColSpanOutOfBoundsErrors` — ColSpan(3) in a 2-column table → error.
14. `TestAddTable_ColSpanInteriorWidthForWrap` — long text in colspan'd cell wraps using wider width.

### Cell merging — rowspan (`table_test.go`)

15. `TestAddTable_RowSpanTallerCell` — first cell RowSpan(2). Verify cell height = sum of 2 row heights.
16. `TestAddTable_RowSpanCoveredCellsImplicit` — row[1] has fewer cells than columns; renders correctly.
17. `TestAddTable_RowSpanOutOfBoundsErrors` — RowSpan past last row → error.
18. `TestAddTable_RowSpanMergeOverlapErrors` — two cells trying to occupy the same position → error.

### Combined (`table_test.go`)

19. `TestAddTable_RowSpanCrossingHeaderBodyErrors` — header row has cell with rowspan extending into body → error.
20. `TestAddTable_RowSpanGroupSurvivesPageBreak` — rowspan group at the bottom of original rect → whole group moves to continuation page.
21. `TestAddTable_RowSpanGroupTooTallErrors` — rowspan group height > continuation page height → error.
22. `TestAddTable_ColSpanAndRowSpanSameCell` — cell with both ColSpan(2) and RowSpan(2) — covers 2×2 area.

### Internal tests (`table_internal_test.go`)

23. `TestSpanningGroups_NoRowspan_OneGroupPerRow` — without any rowspan, every row is its own group.
24. `TestSpanningGroups_RowSpanExpandsGroup` — rowspan(3) at row 0 → group [0..2].
25. `TestSpanningGroups_NestedRowSpans` — multiple overlapping spans in same group correctly extend the upper bound.
26. `TestCoveredGrid_RowSpanMarksFutureRows` — covered grid correctly marks covered positions.

### Aspose parity (`table_aspose_parity_test.go`)

27. `TestAsposeParity_TableRepeatingRows` — `SetRepeatingRowsCount` matches Aspose's `RepeatingRowsCount` property usage.
28. `TestAsposeParity_CellColSpanRowSpan` — `cell.SetColSpan` / `SetRowSpan` matches Aspose's properties.

### Updated Phase 1 tests

All 30+ existing `page.AddTable(...)` call sites must change to `_, err := page.AddTable(...)` (or `_, _ = page.AddTable(...)` in places that don't check err). The compiler will catch every one.

---

## Risks & mitigations

| Risk | Mitigation |
|---|---|
| Span-aware row validation more complex than Phase 1 "len(cells) == len(columnWidths)" | Implement `validateRowCoverage` helper with clear unit tests. Cover counter-examples (under-cover, over-cover, overlap). |
| Auto-fit row height + rowspan interaction underspecified | Document explicit policy: rowspan cells excluded from per-row max; their content clips if too tall. Add `Row.SetHeight` as the escape hatch. |
| Continuation pages may break under encryption (per-stream encryption may not re-trigger for new pages) | Existing infrastructure: `AddBlankPage` returns a `*Page` whose content goes through the same writer-level encryption. Roundtrip test with AES-128 covers this (extends Phase 1 `TestAddTable_AES128Roundtrip`). |
| Breaking change to `AddTable` signature breaks every call site | Memory: no backwards-compat constraint. Update all call sites mechanically. CI catches missed ones. |
| Group-too-tall error is a poor UX when caller expects to "just work" | Error message includes the group's row range so caller knows where to add a manual page break or reduce content. |
| Outer border drawn per-page may differ in appearance vs single-table outer border | Document: each rendered range gets its own enclosing border. Acceptable visual artifact for MVP overflow; equivalent to Aspose's behavior. |

---

## File structure

| File | Purpose | Change scope |
|---|---|---|
| `table.go` (modify) | Add `SetColSpan`/`SetRowSpan`/`ColSpan`/`RowSpan` on Cell; `SetRepeatingRowsCount`/`SetOverflowMargins` + getters on Table | Additive |
| `table_render.go` (modify) | Rewrite `AddTable` for multi-page; extract `validateAndCover`, `computeSpanningGroups`, `drawHeaders`, `drawGroup`, `drawOuterBorder` helpers; manage continuation pages | Significant rewrite |
| `table_test.go` (modify) | Update all existing test signatures to `_, err := ...`; add new tests (items 1-22, 27-28 above) | Add ~22 tests |
| `table_internal_test.go` (modify) | Update existing test signatures if any; add new tests (items 23-26) | Add ~4 tests |
| `table_aspose_parity_test.go` (modify) | Update existing parity tests for new sig; add 2 Phase 2 parity tests | Add ~2 tests |
| `my_examples/full_scenario/main.go` (modify, optional) | Update bill to use ColSpan in summary rows for cleaner code | Optional |
| `CLAUDE.md` (modify) | Update Tables section with Phase 2 additions | Additive |
| `README.md` (modify) | Update Features bullet + Tables snippet to show RepeatingRowsCount | Additive |

Estimated total: ~800 LOC of production code + ~700 LOC of tests, in 14-16 TDD tasks.

---

## Self-review

**Placeholder scan:** every type, method, error message, and rendering step is concrete. The covered-grid pseudocode notes "implementation detail" but the implementation strategy (running active-rowspan tracker) is specified.

**Internal consistency:** `ColSpan`/`RowSpan` semantics consistent across validation, height computation, rendering, and Aspose parity. `RepeatingRowsCount` consistent with rowspan validation (boundary rule).

**Scope check:** explicitly excludes image cells, border dedup, dash patterns, auto-fit columns, convenience methods. The three included features are tightly coupled (overflow needs repeat-headers to be useful; rowspan + overflow interact through groups) — bundling makes sense.

**Ambiguity check:** rowspan crossing page break is explicitly rejected (Phase 3 candidate). Rowspan-cell content overflowing spanned height clips (matches AddText). Continuation rect dimensions explicitly specified.
