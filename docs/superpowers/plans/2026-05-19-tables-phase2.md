# Tables Phase 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add multi-page overflow, repeating header rows, and cell merging (rowspan/colspan) to the Tables API. Aspose .NET-parity throughout.

**Architecture:** `AddTable` signature changes to `(int, error)` returning pages auto-added. Span-aware row validation produces a "covered grid" before rendering. Spanning groups become the atomic unit for page-break decisions. Headers repeat at the top of each continuation page.

**Tech Stack:** Go 1.24, standard library only.

**Reference:** [docs/superpowers/specs/2026-05-19-tables-phase2-design.md](../specs/2026-05-19-tables-phase2-design.md)

**Beads:** [pdf-go-2h3](bd show pdf-go-2h3)

**Phase 1:** [docs/superpowers/plans/2026-05-19-tables.md](2026-05-19-tables.md) — completed at commit `2f72b5c`.

---

## File Map

| File | Purpose |
|---|---|
| `table.go` (modify) | Add `SetColSpan`/`SetRowSpan`/`ColSpan`/`RowSpan` on Cell; `SetRepeatingRowsCount`/`SetOverflowMargins` + getters on Table. |
| `table_render.go` (modify, heavy) | Change `AddTable` signature; add validation helpers (`buildCoveredGrid`, `computeSpanningGroups`); extract `drawHeaders` / `drawGroup` / per-page outer-border helpers; multi-page loop. |
| `table_test.go` (modify) | Update all existing call sites to `_, err := ...`; add Phase 2 tests. |
| `table_internal_test.go` (modify) | Add tests for covered grid + spanning groups; update existing if needed. |
| `table_aspose_parity_test.go` (modify) | Update existing test sigs; add Phase 2 parity tests. |
| `my_examples/full_scenario/main.go` (modify, Task 16) | Restaurant bill: use ColSpan for cleaner summary rows. |
| `CLAUDE.md` (modify, Task 16) | Tables section: append Phase 2 API. |
| `README.md` (modify, Task 16) | Features bullet + Tables snippet. |

---

## Task 1: Breaking signature change — `AddTable` returns `(int, error)`

This task lands the signature change with a no-op `pagesAdded = 0`. All existing tests update mechanically. Subsequent tasks add real overflow logic.

**Files:**
- Modify: `table_render.go`
- Modify: `table_test.go`
- Modify: `table_internal_test.go`
- Modify: `table_aspose_parity_test.go`

- [ ] **Step 1: Update `AddTable` signature in `table_render.go`**

Change:
```go
func (p *Page) AddTable(t *Table, rect Rectangle) error {
    // ...
    return nil
}
```

to:

```go
// AddTable renders the table inside the given rectangle.
//
// Returns the number of pages automatically appended to the document (0 when
// the table fits in rect). When the table doesn't fit and overflow is needed,
// new pages are appended with dimensions matching the receiver page; the
// continuation rectangle is computed from t.OverflowMargins().
//
// Errors before any drawing on validation failures: nil table, bad rect,
// non-positive column widths, mismatched cell counts (span-aware), merge
// overlaps, rowspan crossing the header/body boundary, or a spanning group
// too tall to fit any page.
func (p *Page) AddTable(t *Table, rect Rectangle) (int, error) {
    if t == nil {
        return 0, fmt.Errorf("add table: nil table")
    }
    // ... rest unchanged but every `return ... err` becomes `return 0, ... err`
    // and the final `return nil` becomes `return 0, nil`.
    return 0, nil
}
```

- [ ] **Step 2: Update every call site in `table_test.go`**

Grep for `page.AddTable(` and `page2.AddTable(`. There should be ~25 call sites. Each one:

Before:
```go
if err := page.AddTable(table, rect); err != nil {
    t.Fatal(err)
}
```

After:
```go
if _, err := page.AddTable(table, rect); err != nil {
    t.Fatal(err)
}
```

And:
```go
err := page.AddTable(table, rect)
if err == nil { /* ... */ }
```

becomes:
```go
_, err := page.AddTable(table, rect)
if err == nil { /* ... */ }
```

- [ ] **Step 3: Update every call site in `table_internal_test.go` and `table_aspose_parity_test.go`**

Same mechanical change.

- [ ] **Step 4: Update `my_examples/full_scenario/main.go`**

Search for `page.AddTable` — currently used by `addRestaurantBill`. Change to capture the returned int (just discard for now):

```go
if _, err := page.AddTable(table, ...); err != nil {
    log.Fatalf("add table: %v", err)
}
```

- [ ] **Step 5: Run + commit**

```powershell
go build ./...
go test ./...
go run ./my_examples/full_scenario
git add table_render.go table_test.go table_internal_test.go table_aspose_parity_test.go my_examples/full_scenario/main.go
git commit -m "refactor: AddTable returns (int, error) — preparation for multi-page overflow"
```

Verify: full suite green, my_examples builds and produces the same PDF as before.

---

## Task 2: `Cell.SetColSpan` / `SetRowSpan` / getters

Pure type additions — no rendering integration yet. Defaults to 1.

**Files:**
- Modify: `table.go`
- Modify: `table_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestCell_ColSpanDefault(t *testing.T) {
    cell := pdf.NewTable().SetColumnWidths([]float64{50}).AddRow().AddCell("x")
    if cell.ColSpan() != 1 {
        t.Errorf("default ColSpan = %d, want 1", cell.ColSpan())
    }
    if cell.RowSpan() != 1 {
        t.Errorf("default RowSpan = %d, want 1", cell.RowSpan())
    }
}

func TestCell_SetColSpanChaining(t *testing.T) {
    cell := pdf.NewTable().SetColumnWidths([]float64{50, 50, 50}).AddRow().AddCell("x").SetColSpan(2)
    if cell.ColSpan() != 2 {
        t.Errorf("ColSpan = %d, want 2", cell.ColSpan())
    }
}

func TestCell_SetRowSpanChaining(t *testing.T) {
    cell := pdf.NewTable().SetColumnWidths([]float64{50}).AddRow().AddCell("x").SetRowSpan(3)
    if cell.RowSpan() != 3 {
        t.Errorf("RowSpan = %d, want 3", cell.RowSpan())
    }
}
```

- [ ] **Step 2: Run + observe build failure**

```powershell
go test -run 'TestCell_(ColSpanDefault|SetColSpan|SetRowSpan)' -v ./...
```

`SetColSpan`/`SetRowSpan`/`ColSpan()`/`RowSpan()` undefined.

- [ ] **Step 3: Add to `table.go`**

Add two fields to the `Cell` struct (just after the existing `vAlignSet bool`):

```go
type Cell struct {
    // ... existing fields ...
    colSpan int // 0 == default 1
    rowSpan int // 0 == default 1
}
```

Add methods at the bottom of `table.go`:

```go
// SetColSpan sets the column span (cells the cell occupies horizontally).
// Default 1. Mirrors Aspose.PDF for .NET's Cell.ColSpan.
//
// When colSpan > 1, the caller does NOT add cells for the positions covered
// by the span — the row simply has fewer cells.
func (c *Cell) SetColSpan(n int) *Cell { c.colSpan = n; return c }

// ColSpan returns the cell's column span (1 if unset).
func (c *Cell) ColSpan() int {
    if c.colSpan < 1 {
        return 1
    }
    return c.colSpan
}

// SetRowSpan sets the row span (rows the cell occupies vertically).
// Default 1. Mirrors Aspose.PDF for .NET's Cell.RowSpan.
//
// When rowSpan > 1, the caller does NOT add cells in subsequent rows for the
// positions covered by the span — those rows simply have fewer cells.
func (c *Cell) SetRowSpan(n int) *Cell { c.rowSpan = n; return c }

// RowSpan returns the cell's row span (1 if unset).
func (c *Cell) RowSpan() int {
    if c.rowSpan < 1 {
        return 1
    }
    return c.rowSpan
}
```

Note: `ColSpan()` returns 1 for both `colSpan == 0` (unset) and `colSpan == 1` (explicit). This means `SetColSpan(0)` is silently treated as 1 — acceptable for MVP.

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestCell_' -v ./...
go test ./...
git add table.go table_test.go
git commit -m "feat: tables — Cell.SetColSpan / SetRowSpan + getters (defaults to 1)"
```

---

## Task 3: `Table.SetRepeatingRowsCount` and `SetOverflowMargins`

**Files:**
- Modify: `table.go`
- Modify: `table_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestTable_RepeatingRowsCountDefault(t *testing.T) {
    table := pdf.NewTable()
    if table.RepeatingRowsCount() != 0 {
        t.Errorf("default RepeatingRowsCount = %d, want 0", table.RepeatingRowsCount())
    }
}

func TestTable_SetRepeatingRowsCountChaining(t *testing.T) {
    table := pdf.NewTable().SetRepeatingRowsCount(2)
    if table.RepeatingRowsCount() != 2 {
        t.Errorf("RepeatingRowsCount = %d, want 2", table.RepeatingRowsCount())
    }
}

func TestTable_OverflowMarginsDefault(t *testing.T) {
    top, bottom := pdf.NewTable().OverflowMargins()
    if top != 50 || bottom != 50 {
        t.Errorf("default OverflowMargins = (%g, %g), want (50, 50)", top, bottom)
    }
}

func TestTable_SetOverflowMarginsChaining(t *testing.T) {
    table := pdf.NewTable().SetOverflowMargins(70, 30)
    top, bottom := table.OverflowMargins()
    if top != 70 || bottom != 30 {
        t.Errorf("OverflowMargins = (%g, %g), want (70, 30)", top, bottom)
    }
}
```

- [ ] **Step 2: Run + observe build failure**

- [ ] **Step 3: Add fields + methods to `table.go`**

Add to `Table` struct:

```go
type Table struct {
    // ... existing fields ...
    repeatingRowsCount int
    overflowTop        float64 // 0 = use default 50
    overflowBottom     float64 // 0 = use default 50
    overflowSet        bool    // true once SetOverflowMargins has been called
}
```

Add methods:

```go
// SetRepeatingRowsCount marks the first n rows as headers that repeat at the
// top of every continuation page. Default 0 (no repeat).
//
// Mirrors Aspose.PDF for .NET's Table.RepeatingRowsCount property.
func (t *Table) SetRepeatingRowsCount(n int) *Table {
    t.repeatingRowsCount = n
    return t
}

// RepeatingRowsCount returns the number of header rows that repeat on each
// continuation page (default 0).
func (t *Table) RepeatingRowsCount() int { return t.repeatingRowsCount }

// SetOverflowMargins sets the top/bottom margins (in points) used to compute
// the continuation-page bounding rectangle when the table overflows the
// original rect. Defaults: 50pt on each side.
//
// The continuation rect uses the same LLX/URX as the original rect; the Y
// range becomes [bottom, pageHeight - top].
func (t *Table) SetOverflowMargins(top, bottom float64) *Table {
    t.overflowTop = top
    t.overflowBottom = bottom
    t.overflowSet = true
    return t
}

// OverflowMargins returns the configured overflow margins (defaults 50/50 if
// SetOverflowMargins has not been called).
func (t *Table) OverflowMargins() (top, bottom float64) {
    if !t.overflowSet {
        return 50, 50
    }
    return t.overflowTop, t.overflowBottom
}
```

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestTable_(RepeatingRowsCount|SetRepeatingRowsCount|OverflowMargins|SetOverflowMargins)' -v ./...
go test ./...
git add table.go table_test.go
git commit -m "feat: tables — Table.SetRepeatingRowsCount + SetOverflowMargins"
```

---

## Task 4: Span-aware row coverage validation

The `validateAndCover` helper walks rows and validates that:
- ColSpan and RowSpan are ≥ 1
- ColSpan doesn't exceed columns
- RowSpan doesn't exceed rows
- No merge overlap
- Every row position is exactly covered

Returns a 2D `[][]bool` "covered grid" (true = position is part of a span starting in an earlier cell/row).

**Files:**
- Modify: `table_render.go`
- Modify: `table_internal_test.go`

- [ ] **Step 1: Append failing internal tests**

```go
func TestValidateAndCover_NoMergeSimplePass(t *testing.T) {
    table := NewTable().SetColumnWidths([]float64{50, 50, 50})
    table.AddRow().AddCells("a", "b", "c")
    table.AddRow().AddCells("d", "e", "f")
    covered, err := validateAndCover(table)
    if err != nil {
        t.Fatal(err)
    }
    if len(covered) != 2 || len(covered[0]) != 3 {
        t.Fatalf("covered shape = %dx%d, want 2x3", len(covered), len(covered[0]))
    }
    for i, row := range covered {
        for j, c := range row {
            if c {
                t.Errorf("covered[%d][%d] = true, want false (no merges)", i, j)
            }
        }
    }
}

func TestValidateAndCover_ColSpanMarksWidth(t *testing.T) {
    table := NewTable().SetColumnWidths([]float64{50, 50, 50})
    table.AddRow().AddCell("wide").SetColSpan(2)
    table.AddRow().Cells() // empty intentionally — handled below by adding 3 cells
    // Actually a row with one ColSpan(2) cell + no other cells covers cols 0 and 1.
    // Need a third cell for col 2:
    table.Rows()[0].AddCell("c")
    // Now row 0 has 2 cells: ColSpan(2) and one normal — covers all 3 columns.
    covered, err := validateAndCover(table)
    if err != nil {
        t.Fatal(err)
    }
    // No future rows should be covered (no RowSpan).
    for i, row := range covered {
        for j, c := range row {
            if c {
                t.Errorf("covered[%d][%d] = true, want false (colspan only)", i, j)
            }
        }
    }
}

func TestValidateAndCover_RowSpanMarksFutureRows(t *testing.T) {
    table := NewTable().SetColumnWidths([]float64{50, 50})
    // Row 0: tall cell (rowspan=2) at col 0, normal cell at col 1.
    row0 := table.AddRow()
    row0.AddCell("tall").SetRowSpan(2)
    row0.AddCell("a")
    // Row 1: only one cell at col 1 (col 0 is covered by row 0's rowspan).
    table.AddRow().AddCell("b")
    covered, err := validateAndCover(table)
    if err != nil {
        t.Fatal(err)
    }
    if !covered[1][0] {
        t.Error("covered[1][0] should be true (rowspan from row 0)")
    }
    if covered[1][1] {
        t.Error("covered[1][1] should be false")
    }
}

func TestValidateAndCover_ColSpanOutOfBoundsErrors(t *testing.T) {
    table := NewTable().SetColumnWidths([]float64{50, 50})
    table.AddRow().AddCell("too wide").SetColSpan(3)
    _, err := validateAndCover(table)
    if err == nil {
        t.Error("expected error for colspan exceeding column count")
    }
}

func TestValidateAndCover_RowSpanOutOfBoundsErrors(t *testing.T) {
    table := NewTable().SetColumnWidths([]float64{50})
    table.AddRow().AddCell("x").SetRowSpan(3) // only 1 row
    _, err := validateAndCover(table)
    if err == nil {
        t.Error("expected error for rowspan exceeding row count")
    }
}

func TestValidateAndCover_UnderCoverageErrors(t *testing.T) {
    // Row with too few cells for its column count.
    table := NewTable().SetColumnWidths([]float64{50, 50, 50})
    table.AddRow().AddCell("only one")
    _, err := validateAndCover(table)
    if err == nil {
        t.Error("expected error for row covering fewer than all columns")
    }
}

func TestValidateAndCover_OverCoverageErrors(t *testing.T) {
    // Row with too many cells.
    table := NewTable().SetColumnWidths([]float64{50, 50})
    table.AddRow().AddCells("a", "b", "c")
    _, err := validateAndCover(table)
    if err == nil {
        t.Error("expected error for row covering more than all columns")
    }
}

func TestValidateAndCover_MergeOverlapErrors(t *testing.T) {
    // Row 0: rowspan(2) at col 0.
    // Row 1: attempts to put a cell at col 0 (already covered).
    table := NewTable().SetColumnWidths([]float64{50, 50})
    table.AddRow().AddCell("tall").SetRowSpan(2)
    table.Rows()[0].AddCell("a")
    table.AddRow().AddCells("oops", "b") // first cell tries col 0 but it's covered
    _, err := validateAndCover(table)
    if err == nil {
        t.Error("expected error for cell overlapping a rowspan from above")
    }
}
```

- [ ] **Step 2: Run + observe failure**

`validateAndCover` undefined.

- [ ] **Step 3: Implement in `table_render.go`**

Add the helper:

```go
// validateAndCover walks the rows, validates span boundaries + non-overlap,
// and returns a [rows][cols] grid where covered[i][j] == true means position
// (i, j) is filled by a cell that started at an earlier row (rowspan) — i.e.
// the caller does not add a *Cell for this position in row i.
//
// Per the spec: every row's cells, placed left-to-right and skipping covered
// positions, must exactly cover the remaining column slots in that row.
func validateAndCover(t *Table) ([][]bool, error) {
    numRows := len(t.rows)
    numCols := len(t.columnWidths)
    covered := make([][]bool, numRows)
    for i := range covered {
        covered[i] = make([]bool, numCols)
    }

    for i, row := range t.rows {
        col := 0
        for cellIdx, cell := range row.cells {
            // Skip positions covered by inherited rowspans.
            for col < numCols && covered[i][col] {
                col++
            }
            cs := cell.ColSpan()
            rs := cell.RowSpan()
            if col+cs > numCols {
                return nil, fmt.Errorf(
                    "add table: colspan at row %d cell %d (col %d, span %d) exceeds column count %d",
                    i, cellIdx, col, cs, numCols)
            }
            if i+rs > numRows {
                return nil, fmt.Errorf(
                    "add table: rowspan at row %d cell %d (span %d) exceeds row count %d",
                    i, cellIdx, rs, numRows)
            }
            // Mark future-row coverage.
            for r := 1; r < rs; r++ {
                for c := 0; c < cs; c++ {
                    if covered[i+r][col+c] {
                        return nil, fmt.Errorf(
                            "add table: merge overlap at row %d col %d", i+r, col+c)
                    }
                    covered[i+r][col+c] = true
                }
            }
            col += cs
        }
        // After placing all of row i's cells, every column must be covered:
        //   columns 0..col-1 are covered by this row's cells (placed left-to-right)
        //   columns col..numCols-1 must be covered by inherited rowspans
        for c := col; c < numCols; c++ {
            if !covered[i][c] {
                return nil, fmt.Errorf(
                    "add table: row %d undercoverage at col %d (cells stop at %d, no inherited rowspan)",
                    i, c, col)
            }
        }
    }

    return covered, nil
}
```

- [ ] **Step 4: Wire into `AddTable`**

Replace the existing Phase 1 cell-count check (the `for i, row := range t.rows { if len(row.cells) != len(t.columnWidths) ...}` block) with:

```go
covered, err := validateAndCover(t)
if err != nil {
    return 0, err
}
_ = covered // used by subsequent rendering tasks
```

The existing nil-table / bad-rect / non-positive-width checks stay as they are.

- [ ] **Step 5: Run + commit**

```powershell
go test -run 'TestValidateAndCover|TestAddTable_' -v ./...
go test ./...
git add table_render.go table_internal_test.go
git commit -m "feat: tables — validateAndCover (span-aware row validation + covered grid)"
```

**Note**: existing `TestAddTable_MismatchedCellCount` test (Phase 1 Task 4) expects the old error wording. After this task, the error becomes `add table: row %d undercoverage at col %d ...` or similar. Update the test if it asserts specific text; if it only checks `err != nil`, no change needed.

---

## Task 5: `computeRowHeights` updated for merging

**Files:**
- Modify: `table_render.go`
- Modify: `table_internal_test.go`

- [ ] **Step 1: Append failing internal tests**

```go
func TestComputeRowHeights_ColSpanUsesWiderInterior(t *testing.T) {
    // Two columns each 60pt. Without colspan, "Hello World" wraps in 60pt.
    // With colspan(2), interior is 120pt - margins → no wrap.
    table := NewTable().SetColumnWidths([]float64{60, 60}).
        SetDefaultCellStyle(TextStyle{Font: FontHelvetica, Size: 12}).
        SetDefaultCellMargin(MarginInfo{Top: 4, Right: 4, Bottom: 4, Left: 4})
    row := table.AddRow()
    row.AddCell("Hello World Foo Bar").SetColSpan(2)
    heights, err := computeRowHeights(table)
    if err != nil {
        t.Fatal(err)
    }
    // Interior width = 120 - 8 = 112pt. "Hello World Foo Bar" ≈ 110pt at 12pt
    // Helvetica → fits in one line.
    // Auto-fit height = 14.4 + 8 = 22.4
    lineHeight := 12.0 * 1.2
    want := lineHeight + 8.0
    if heights[0] != want {
        t.Errorf("colspan row height = %g, want %g (no wrap with wider interior)",
            heights[0], want)
    }
}

func TestComputeRowHeights_RowSpanCellExcludedFromMax(t *testing.T) {
    // Row 0: tall rowspan cell (occupies rows 0 and 1) + normal cell.
    // Row 1: one cell at col 1 (col 0 is covered).
    // Auto-fit row heights are determined by NON-rowspan cells only.
    table := NewTable().SetColumnWidths([]float64{60, 60}).
        SetDefaultCellStyle(TextStyle{Font: FontHelvetica, Size: 12}).
        SetDefaultCellMargin(MarginInfo{Top: 2, Right: 2, Bottom: 2, Left: 2})
    row0 := table.AddRow()
    row0.AddCell("Multi-line\ntall content here\nover multiple lines").SetRowSpan(2)
    row0.AddCell("a")
    table.AddRow().AddCell("b")
    heights, err := computeRowHeights(table)
    if err != nil {
        t.Fatal(err)
    }
    // Each row's non-rowspan cell is a single line.
    lineHeight := 12.0 * 1.2
    want := lineHeight + 4.0
    if heights[0] != want || heights[1] != want {
        t.Errorf("rowspan-excluded heights = %v, want both %g", heights, want)
    }
}
```

- [ ] **Step 2: Run + observe failure**

- [ ] **Step 3: Update `computeRowHeights` in `table_render.go`**

Two changes:

1. Exclude rowspan cells from per-row max-height computation.
2. ColSpan cells use the wider interior width.

Updated function:

```go
func computeRowHeights(t *Table) ([]float64, error) {
    heights := make([]float64, len(t.rows))

    // Build span-aware iteration: for each row, walk cells with their resolved
    // column index (skipping covered positions). Use covered grid from
    // validateAndCover. But validateAndCover may not have been called yet from
    // this code path — call it again here OR pull it out. Cleanest: have
    // AddTable call validateAndCover once and pass the covered grid down.
    //
    // For computeRowHeights's public-test API simplicity, call validateAndCover
    // internally. This is O(rows*cols) extra work which is fine for MVP.
    covered, err := validateAndCover(t)
    if err != nil {
        return nil, err
    }

    for i, row := range t.rows {
        if row.height > 0 {
            heights[i] = row.height
            continue
        }
        maxH := 0.0
        col := 0
        for _, cell := range row.cells {
            for col < len(t.columnWidths) && covered[i][col] {
                col++
            }
            cs := cell.ColSpan()
            rs := cell.RowSpan()
            // Skip rowspan cells: their height is checked separately.
            if rs > 1 {
                col += cs
                continue
            }
            // Interior width = sum of cs column widths - margins.
            sumW := 0.0
            for c := 0; c < cs; c++ {
                sumW += t.columnWidths[col+c]
            }
            margin := effectiveCellMargin(t, cell)
            style := effectiveCellStyle(t, cell)
            interiorWidth := sumW - margin.Left - margin.Right
            if interiorWidth < 0 {
                interiorWidth = 0
            }
            lines, lineHeight, err := measureText(cell.text, style, interiorWidth)
            if err != nil {
                return nil, fmt.Errorf("row %d col %d: %w", i, col, err)
            }
            cellH := float64(lines)*lineHeight + margin.Top + margin.Bottom
            if cellH > maxH {
                maxH = cellH
            }
            col += cs
        }
        heights[i] = maxH
    }
    return heights, nil
}
```

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestComputeRowHeights' -v ./...
go test ./...
git add table_render.go table_internal_test.go
git commit -m "feat: tables — computeRowHeights honors ColSpan width + excludes RowSpan cells from per-row max"
```

---

## Task 6: Single-page rendering with ColSpan + RowSpan

This task updates the per-cell rendering loop in `AddTable` to honor span. Still single-page (no overflow yet).

**Files:**
- Modify: `table_render.go`
- Modify: `table_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestAddTable_ColSpanRendersWiderCell(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    table := pdf.NewTable().SetColumnWidths([]float64{100, 100, 100})
    row := table.AddRow()
    row.AddCell("wide cell").SetColSpan(3) // spans all 3 columns

    if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 300, URY: 100}); err != nil {
        t.Fatal(err)
    }

    // The rendered text fragment for "wide cell" should be positioned such
    // that its right edge approaches 300 (or center if HAlignCenter).
    layout, err := page.ExtractTextWithLayout()
    if err != nil {
        t.Fatal(err)
    }
    if len(layout) == 0 || len(layout[0].Fragments) == 0 {
        t.Fatal("no fragments extracted")
    }
    // For left-aligned default, X is at the LLX + left margin. We just verify
    // the text is on the page and didn't error.
    found := false
    for _, line := range layout {
        for _, f := range line.Fragments {
            if strings.Contains(f.Text, "wide cell") {
                found = true
            }
        }
    }
    if !found {
        t.Error("colspan cell text not found")
    }
}

func TestAddTable_RowSpanRendersTallerCell(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    table := pdf.NewTable().
        SetColumnWidths([]float64{50, 50}).
        SetDefaultCellStyle(pdf.TextStyle{Size: 12}).
        SetDefaultCellMargin(pdf.MarginInfo{Top: 2, Right: 2, Bottom: 2, Left: 2})
    row0 := table.AddRow()
    row0.AddCell("T").SetRowSpan(2) // spans rows 0 and 1
    row0.AddCell("a")
    table.AddRow().AddCell("b") // col 0 is covered

    if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100}); err != nil {
        t.Fatal(err)
    }

    text, _ := page.ExtractText()
    for _, want := range []string{"T", "a", "b"} {
        if !strings.Contains(text, want) {
            t.Errorf("missing %q in output: %q", want, text)
        }
    }
}

func TestAddTable_RowSpanColSpanCombined(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    table := pdf.NewTable().SetColumnWidths([]float64{50, 50, 50})
    row0 := table.AddRow()
    row0.AddCell("2x2").SetColSpan(2).SetRowSpan(2) // covers rows 0..1, cols 0..1
    row0.AddCell("c0")
    table.AddRow().AddCell("c1") // col 2 only; cols 0..1 covered

    if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 150, URY: 100}); err != nil {
        t.Fatal(err)
    }

    text, _ := page.ExtractText()
    for _, want := range []string{"2x2", "c0", "c1"} {
        if !strings.Contains(text, want) {
            t.Errorf("missing %q: %q", want, text)
        }
    }
}
```

- [ ] **Step 2: Run + observe failure**

The single-page rendering loop still assumes one cell per column; tests fail because cell-count validation rejects rows with fewer cells than columns (depending on the order in which Tasks 1-5 left things, ColSpan cells may render only one column wide).

- [ ] **Step 3: Rewrite the rendering loop in `AddTable`**

Pre-compute the X-offsets (running sum of columnWidths) for cheap span-width lookups. Pass the `covered` grid in from validation. Render cells at spanned coordinates.

Replace the existing single-page rendering block (the `y := rect.URY; for i, row := range t.rows { ...` loop) with:

```go
// X offsets: xOffsets[c] = sum(columnWidths[0..c]); len(xOffsets) = numCols+1
xOffsets := make([]float64, len(t.columnWidths)+1)
for i, w := range t.columnWidths {
    xOffsets[i+1] = xOffsets[i] + w
}

// yTops[i] = absolute Y at the top of row i (largest y, since PDF Y grows up)
// We'll compute it lazily inside the loop because the page may change.
y := rect.URY
drawnHeight := 0.0
for i, row := range t.rows {
    if y-heights[i] < rect.LLY {
        // Phase 2 multi-page overflow handled in later task. For now, break.
        break
    }
    col := 0
    for _, cell := range row.cells {
        for col < len(t.columnWidths) && covered[i][col] {
            col++
        }
        cs := cell.ColSpan()
        rs := cell.RowSpan()
        cellLLX := rect.LLX + xOffsets[col]
        cellURX := rect.LLX + xOffsets[col+cs]
        cellURY := y
        // Sum spanned row heights for the cell's bottom edge.
        spanH := heights[i]
        for r := 1; r < rs; r++ {
            spanH += heights[i+r]
        }
        cellLLY := cellURY - spanH

        margin := effectiveCellMargin(t, cell)
        style := effectiveCellStyle(t, cell)

        // 1. Background.
        if cell.background != nil {
            if err := p.appendToContentStream([]byte(
                drawCellBackground(cellLLX, cellLLY, cellURX, cellURY, cell.background),
            )); err != nil {
                return 0, fmt.Errorf("add table: row %d col %d background: %w", i, col, err)
            }
        }

        // 2. Text.
        interior := Rectangle{
            LLX: cellLLX + margin.Left,
            LLY: cellLLY + margin.Bottom,
            URX: cellURX - margin.Right,
            URY: cellURY - margin.Top,
        }
        if interior.URX > interior.LLX && interior.URY > interior.LLY && cell.text != "" {
            if err := p.AddText(cell.text, style, interior); err != nil {
                return 0, fmt.Errorf("add table: row %d col %d text: %w", i, col, err)
            }
        }

        // 3. Border.
        border := effectiveCellBorder(t, cell)
        if ops := drawBorderSides(cellLLX, cellLLY, cellURX, cellURY, border); ops != "" {
            if err := p.appendToContentStream([]byte(ops)); err != nil {
                return 0, fmt.Errorf("add table: row %d col %d border: %w", i, col, err)
            }
        }

        col += cs
    }
    y -= heights[i]
    drawnHeight += heights[i]
}

// Outer table border. Drawn last so it appears on top of cell-edge strokes.
if drawnHeight > 0 {
    totalW := xOffsets[len(t.columnWidths)]
    if ops := drawBorderSides(
        rect.LLX, rect.URY-drawnHeight,
        rect.LLX+totalW, rect.URY,
        t.border,
    ); ops != "" {
        if err := p.appendToContentStream([]byte(ops)); err != nil {
            return 0, fmt.Errorf("add table: outer border: %w", err)
        }
    }
}

return 0, nil
```

The render loop now:
- Skips covered positions on row entry
- Uses xOffsets[col+cs] - xOffsets[col] for spanned width
- Uses sum(heights[i..i+rs-1]) for spanned height
- All background/text/border helpers see the merged rect

Phase 1 tests still pass because cs=1, rs=1 give identical coordinates to the Phase 1 path.

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestAddTable_(ColSpan|RowSpan)' -v ./...
go test ./...
git add table_render.go table_test.go
git commit -m "feat: tables — render cells with ColSpan width + RowSpan height"
```

---

## Task 7: Compute spanning groups

A spanning group is a maximal sequence of rows that contains no rowspan extending beyond the group. Used by overflow logic in Task 9.

**Files:**
- Modify: `table_render.go`
- Modify: `table_internal_test.go`

- [ ] **Step 1: Append failing internal tests**

```go
func TestComputeSpanningGroups_NoRowSpan_OnePerRow(t *testing.T) {
    table := NewTable().SetColumnWidths([]float64{50})
    table.AddRow().AddCell("a")
    table.AddRow().AddCell("b")
    table.AddRow().AddCell("c")
    groups := computeSpanningGroups(table, 0)
    if len(groups) != 3 {
        t.Fatalf("groups = %d, want 3", len(groups))
    }
    for i, g := range groups {
        if g.start != i || g.end != i {
            t.Errorf("group %d = [%d..%d], want [%d..%d]", i, g.start, g.end, i, i)
        }
    }
}

func TestComputeSpanningGroups_RowSpanExpands(t *testing.T) {
    table := NewTable().SetColumnWidths([]float64{50, 50})
    row0 := table.AddRow()
    row0.AddCell("tall").SetRowSpan(3) // covers rows 0..2
    row0.AddCell("a")
    table.AddRow().AddCell("b") // col 0 covered
    table.AddRow().AddCell("c") // col 0 covered
    table.AddRow().AddCells("d", "e")
    groups := computeSpanningGroups(table, 0)
    if len(groups) != 2 {
        t.Fatalf("groups = %d, want 2", len(groups))
    }
    if groups[0].start != 0 || groups[0].end != 2 {
        t.Errorf("group 0 = [%d..%d], want [0..2]", groups[0].start, groups[0].end)
    }
    if groups[1].start != 3 || groups[1].end != 3 {
        t.Errorf("group 1 = [%d..%d], want [3..3]", groups[1].start, groups[1].end)
    }
}

func TestComputeSpanningGroups_NestedRowSpans(t *testing.T) {
    // Row 0: rowspan=2 cell
    // Row 1: another rowspan=2 cell (covers rows 1..2)
    // Row 2: covered
    // Row 3: standalone
    table := NewTable().SetColumnWidths([]float64{50, 50})
    row0 := table.AddRow()
    row0.AddCell("r0_0").SetRowSpan(2) // covers rows 0..1
    row0.AddCell("r0_1")
    row1 := table.AddRow()
    // col 0 is covered by row 0's rowspan
    row1.AddCell("r1_1").SetRowSpan(2) // covers rows 1..2 at col 1
    row2 := table.AddRow()
    row2.AddCell("r2_0") // col 1 is covered
    table.AddRow().AddCells("r3_0", "r3_1")
    groups := computeSpanningGroups(table, 0)
    // Expected: group [0..2] (row 0 spans into 1, row 1 spans into 2), then [3..3]
    if len(groups) != 2 {
        t.Fatalf("groups = %d, want 2: %+v", len(groups), groups)
    }
    if groups[0].start != 0 || groups[0].end != 2 {
        t.Errorf("group 0 = [%d..%d], want [0..2]", groups[0].start, groups[0].end)
    }
    if groups[1].start != 3 || groups[1].end != 3 {
        t.Errorf("group 1 = [%d..%d], want [3..3]", groups[1].start, groups[1].end)
    }
}

func TestComputeSpanningGroups_StartIndexSkipsHeaders(t *testing.T) {
    table := NewTable().SetColumnWidths([]float64{50})
    table.AddRow().AddCell("header")
    table.AddRow().AddCell("a")
    table.AddRow().AddCell("b")
    groups := computeSpanningGroups(table, 1) // skip row 0 (header)
    if len(groups) != 2 {
        t.Fatalf("groups starting at 1 = %d, want 2", len(groups))
    }
    if groups[0].start != 1 || groups[1].start != 2 {
        t.Errorf("groups = %+v, want starting at 1 and 2", groups)
    }
}
```

- [ ] **Step 2: Run + observe failure**

`computeSpanningGroups` undefined.

- [ ] **Step 3: Implement in `table_render.go`**

```go
// spanGroup is a contiguous range of rows that must be drawn together (no
// page break inside). [start, end] are inclusive row indices.
type spanGroup struct {
    start, end int
}

// computeSpanningGroups computes the maximal "atomic" groups of rows starting
// at startIdx. Within a group, no rowspan extends beyond the group's last row.
// Each returned group is the unit that page-break logic moves as a whole.
func computeSpanningGroups(t *Table, startIdx int) []spanGroup {
    var groups []spanGroup
    i := startIdx
    numRows := len(t.rows)
    for i < numRows {
        g := spanGroup{start: i, end: i}
        // Walk j from i upwards, extending g.end whenever a rowspan reaches further.
        j := i
        for j <= g.end {
            for _, cell := range t.rows[j].cells {
                rs := cell.RowSpan()
                if rs < 1 {
                    rs = 1
                }
                spanEnd := j + rs - 1
                if spanEnd > g.end {
                    g.end = spanEnd
                }
            }
            j++
        }
        groups = append(groups, g)
        i = g.end + 1
    }
    return groups
}
```

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestComputeSpanningGroups' -v ./...
go test ./...
git add table_render.go table_internal_test.go
git commit -m "feat: tables — computeSpanningGroups (atomic units for page-break decisions)"
```

---

## Task 8: Extract `drawHeaders` + `drawTableRows` helpers + `repeatingRowsCount` validation

This task refactors the single-page rendering into reusable helpers, so Task 9 can call them on multiple pages. No behavior change yet.

**Files:**
- Modify: `table_render.go`
- Modify: `table_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestAddTable_RepeatingRowsCountValidation(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)

    table := pdf.NewTable().SetColumnWidths([]float64{50})
    table.AddRow().AddCell("only")
    table.SetRepeatingRowsCount(5) // way more than the 1 row
    _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100})
    if err == nil {
        t.Error("expected error when RepeatingRowsCount exceeds RowCount")
    }

    table2 := pdf.NewTable().SetColumnWidths([]float64{50})
    table2.AddRow().AddCell("only")
    table2.SetRepeatingRowsCount(-1)
    _, err = page.AddTable(table2, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100})
    if err == nil {
        t.Error("expected error for negative RepeatingRowsCount")
    }
}
```

- [ ] **Step 2: Run + observe failure**

Currently `SetRepeatingRowsCount(5)` on a 1-row table doesn't error.

- [ ] **Step 3: Add validation + extract rendering helpers**

Add validation early in `AddTable` (after `validateAndCover`):

```go
if t.repeatingRowsCount < 0 {
    return 0, fmt.Errorf("add table: repeating rows count %d is negative", t.repeatingRowsCount)
}
if t.repeatingRowsCount > len(t.rows) {
    return 0, fmt.Errorf("add table: repeating rows count %d exceeds row count %d",
        t.repeatingRowsCount, len(t.rows))
}
```

Extract the per-page rendering into a helper:

```go
// drawRowRange renders rows [startRow..endRow] (inclusive) of t on targetPage,
// using rect.LLX as the left origin and topY as the top edge of the first row.
// Returns the total height of rows actually drawn.
//
// covered: pre-computed coverage grid from validateAndCover.
// xOffsets: pre-computed running-sum of columnWidths.
// heights: pre-computed row heights.
func drawRowRange(
    targetPage *Page, t *Table,
    startRow, endRow int,
    rect Rectangle, topY float64,
    covered [][]bool, xOffsets, heights []float64,
) (drawnHeight float64, err error) {
    y := topY
    for i := startRow; i <= endRow; i++ {
        rowH := heights[i]
        col := 0
        for _, cell := range t.rows[i].cells {
            for col < len(t.columnWidths) && covered[i][col] {
                col++
            }
            cs := cell.ColSpan()
            rs := cell.RowSpan()
            cellLLX := rect.LLX + xOffsets[col]
            cellURX := rect.LLX + xOffsets[col+cs]
            cellURY := y
            spanH := rowH
            for r := 1; r < rs; r++ {
                spanH += heights[i+r]
            }
            cellLLY := cellURY - spanH

            margin := effectiveCellMargin(t, cell)
            style := effectiveCellStyle(t, cell)

            if cell.background != nil {
                if err := targetPage.appendToContentStream([]byte(
                    drawCellBackground(cellLLX, cellLLY, cellURX, cellURY, cell.background),
                )); err != nil {
                    return drawnHeight, fmt.Errorf("row %d col %d background: %w", i, col, err)
                }
            }
            interior := Rectangle{
                LLX: cellLLX + margin.Left,
                LLY: cellLLY + margin.Bottom,
                URX: cellURX - margin.Right,
                URY: cellURY - margin.Top,
            }
            if interior.URX > interior.LLX && interior.URY > interior.LLY && cell.text != "" {
                if err := targetPage.AddText(cell.text, style, interior); err != nil {
                    return drawnHeight, fmt.Errorf("row %d col %d text: %w", i, col, err)
                }
            }
            border := effectiveCellBorder(t, cell)
            if ops := drawBorderSides(cellLLX, cellLLY, cellURX, cellURY, border); ops != "" {
                if err := targetPage.appendToContentStream([]byte(ops)); err != nil {
                    return drawnHeight, fmt.Errorf("row %d col %d border: %w", i, col, err)
                }
            }
            col += cs
        }
        y -= rowH
        drawnHeight += rowH
    }
    return drawnHeight, nil
}

// drawOuterBorder draws the table's outer border around the given drawn area
// on targetPage. No-op if t.border.Sides is BorderSideNone or width is 0.
func drawOuterBorder(targetPage *Page, t *Table, rect Rectangle, topY, drawnHeight float64, xOffsets []float64) error {
    if drawnHeight <= 0 {
        return nil
    }
    totalW := xOffsets[len(t.columnWidths)]
    ops := drawBorderSides(
        rect.LLX, topY-drawnHeight,
        rect.LLX+totalW, topY,
        t.border,
    )
    if ops == "" {
        return nil
    }
    return targetPage.appendToContentStream([]byte(ops))
}
```

Rewrite `AddTable`'s rendering block to use these helpers (preserves existing single-page behavior):

```go
xOffsets := make([]float64, len(t.columnWidths)+1)
for i, w := range t.columnWidths {
    xOffsets[i+1] = xOffsets[i] + w
}

// Walk rows top-to-bottom, breaking on first row that doesn't fit (Phase 1
// behavior — Task 9 replaces this with multi-page logic).
y := rect.URY
drawnHeight := 0.0
i := 0
for i < len(t.rows) {
    if y-heights[i] < rect.LLY {
        break
    }
    h, err := drawRowRange(p, t, i, i, rect, y, covered, xOffsets, heights)
    if err != nil {
        return 0, fmt.Errorf("add table: %w", err)
    }
    y -= h
    drawnHeight += h
    i++
}

if err := drawOuterBorder(p, t, rect, rect.URY, drawnHeight, xOffsets); err != nil {
    return 0, fmt.Errorf("add table: outer border: %w", err)
}

return 0, nil
```

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestAddTable_RepeatingRowsCountValidation' -v ./...
go test ./...
git add table_render.go table_test.go
git commit -m "refactor: tables — extract drawRowRange + drawOuterBorder helpers; validate RepeatingRowsCount"
```

---

## Task 9: Multi-page overflow

Now the real feature. Walk spanning groups; on overflow, append a new page and continue.

**Files:**
- Modify: `table_render.go`
- Modify: `table_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestAddTable_OverflowAddsPage(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)

    table := pdf.NewTable().
        SetColumnWidths([]float64{100}).
        SetDefaultCellStyle(pdf.TextStyle{Size: 12}).
        SetDefaultCellMargin(pdf.MarginInfo{Top: 4, Right: 4, Bottom: 4, Left: 4})
    // 3 rows of ~22pt each = ~66pt; rect height = 30pt → only 1 row fits.
    table.AddRow().AddCell("rowOne")
    table.AddRow().AddCell("rowTwo")
    table.AddRow().AddCell("rowThree")

    pagesAdded, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 700, URX: 200, URY: 730})
    if err != nil {
        t.Fatal(err)
    }
    if pagesAdded < 1 {
        t.Errorf("pagesAdded = %d, want >= 1", pagesAdded)
    }
    if doc.PageCount() != 1+pagesAdded {
        t.Errorf("PageCount = %d, want %d", doc.PageCount(), 1+pagesAdded)
    }
}

func TestAddTable_OverflowReturnsZeroIfFits(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    table := pdf.NewTable().SetColumnWidths([]float64{100})
    table.AddRow().AddCell("only row")
    // Tall rect → fits.
    pagesAdded, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 500})
    if err != nil {
        t.Fatal(err)
    }
    if pagesAdded != 0 {
        t.Errorf("pagesAdded = %d, want 0 (fits)", pagesAdded)
    }
    if doc.PageCount() != 1 {
        t.Errorf("PageCount = %d, want 1", doc.PageCount())
    }
}

func TestAddTable_OverflowContentSurvivesRoundTrip(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)

    table := pdf.NewTable().
        SetColumnWidths([]float64{100}).
        SetDefaultCellStyle(pdf.TextStyle{Size: 12}).
        SetDefaultCellMargin(pdf.MarginInfo{Top: 3, Right: 3, Bottom: 3, Left: 3})
    // 8 rows, only ~3 fit per page → multi-page overflow.
    for i := 1; i <= 8; i++ {
        table.AddRow().AddCell(fmt.Sprintf("row%d", i))
    }

    if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 700, URX: 200, URY: 760}); err != nil {
        t.Fatal(err)
    }

    var buf bytes.Buffer
    if _, err := doc.WriteTo(&buf); err != nil {
        t.Fatal(err)
    }
    doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    if err != nil {
        t.Fatal(err)
    }
    // All 8 rows should appear somewhere across the document.
    foundAll := true
    for i := 1; i <= 8; i++ {
        want := fmt.Sprintf("row%d", i)
        found := false
        for p := 1; p <= doc2.PageCount(); p++ {
            page, _ := doc2.Page(p)
            text, _ := page.ExtractText()
            if strings.Contains(text, want) {
                found = true
                break
            }
        }
        if !found {
            t.Errorf("missing %q somewhere in document", want)
            foundAll = false
        }
    }
    _ = foundAll
}

func TestAddTable_OverflowGroupTooTallErrors(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)

    // Tiny continuation margins → very small continuation rect.
    table := pdf.NewTable().
        SetColumnWidths([]float64{100}).
        SetOverflowMargins(400, 400) // leaves only 42pt for content on A4 (842-800)
    row := table.AddRow()
    row.SetHeight(100) // single row taller than the available continuation space
    row.AddCell("huge")

    // Original rect can't fit it either.
    _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50})
    if err == nil {
        t.Error("expected error for group too tall for any page")
    }
}
```

- [ ] **Step 2: Run + observe failure**

`pagesAdded` is always 0, multi-row tables truncate.

- [ ] **Step 3: Implement multi-page overflow in `AddTable`**

Replace the single-page rendering block (the `for i < len(t.rows)` loop + outer border call) with the multi-page version:

```go
overflowTop, overflowBottom := t.OverflowMargins()
sz, err := p.Size()
if err != nil {
    return 0, fmt.Errorf("add table: page size: %w", err)
}
continuationRect := Rectangle{
    LLX: rect.LLX,
    LLY: overflowBottom,
    URX: rect.URX,
    URY: sz.Height - overflowTop,
}
continuationHeight := continuationRect.URY - continuationRect.LLY
if continuationHeight <= 0 {
    return 0, fmt.Errorf("add table: continuation rect has non-positive height (page %g, margins top=%g bottom=%g)",
        sz.Height, overflowTop, overflowBottom)
}

// Compute spanning groups for the body (skip header rows).
groups := computeSpanningGroups(t, t.repeatingRowsCount)

// Validate: the largest body group must fit on a continuation page after headers.
headerHeight := 0.0
for i := 0; i < t.repeatingRowsCount; i++ {
    headerHeight += heights[i]
}
if headerHeight > rect.URY-rect.LLY {
    return 0, fmt.Errorf("add table: header rows height %g exceeds initial rect height %g",
        headerHeight, rect.URY-rect.LLY)
}
if headerHeight > continuationHeight {
    return 0, fmt.Errorf("add table: header rows height %g exceeds continuation rect height %g",
        headerHeight, continuationHeight)
}
for _, g := range groups {
    gh := 0.0
    for r := g.start; r <= g.end; r++ {
        gh += heights[r]
    }
    if gh > continuationHeight-headerHeight {
        return 0, fmt.Errorf("add table: group [%d..%d] height %g exceeds continuation rect body height %g",
            g.start, g.end, gh, continuationHeight-headerHeight)
    }
}

pagesAdded := 0
currentPage := p
currentRect := rect

drawCurrentPageOuter := func(topY, drawnHeight float64) error {
    return drawOuterBorder(currentPage, t, currentRect, topY, drawnHeight, xOffsets)
}

// Draw header rows on the first page.
y := currentRect.URY
pageDrawn := 0.0
if t.repeatingRowsCount > 0 {
    h, err := drawRowRange(currentPage, t, 0, t.repeatingRowsCount-1, currentRect, y, covered, xOffsets, heights)
    if err != nil {
        return pagesAdded, fmt.Errorf("add table: headers: %w", err)
    }
    y -= h
    pageDrawn += h
}

// Walk groups.
for _, g := range groups {
    groupH := 0.0
    for r := g.start; r <= g.end; r++ {
        groupH += heights[r]
    }
    if y-groupH < currentRect.LLY {
        // Overflow — finish outer border on current page, append a new page,
        // reset state, redraw headers.
        if err := drawCurrentPageOuter(currentRect.URY, pageDrawn); err != nil {
            return pagesAdded, fmt.Errorf("add table: outer border: %w", err)
        }

        if err := p.doc.AddBlankPage(sz.Width, sz.Height); err != nil {
            return pagesAdded, fmt.Errorf("add table: append page: %w", err)
        }
        pagesAdded++
        np, err := p.doc.Page(p.doc.PageCount())
        if err != nil {
            return pagesAdded, fmt.Errorf("add table: continuation page: %w", err)
        }
        currentPage = np
        currentRect = continuationRect
        y = currentRect.URY
        pageDrawn = 0.0

        if t.repeatingRowsCount > 0 {
            h, err := drawRowRange(currentPage, t, 0, t.repeatingRowsCount-1, currentRect, y, covered, xOffsets, heights)
            if err != nil {
                return pagesAdded, fmt.Errorf("add table: repeated headers: %w", err)
            }
            y -= h
            pageDrawn += h
        }
    }

    h, err := drawRowRange(currentPage, t, g.start, g.end, currentRect, y, covered, xOffsets, heights)
    if err != nil {
        return pagesAdded, fmt.Errorf("add table: %w", err)
    }
    y -= h
    pageDrawn += h
}

// Final outer border on the last page.
if err := drawCurrentPageOuter(currentRect.URY, pageDrawn); err != nil {
    return pagesAdded, fmt.Errorf("add table: outer border (final): %w", err)
}

return pagesAdded, nil
```

Notes for the implementer:
- `(*Page).doc` is an unexported pointer — already used by `LoadFont`. If access is awkward from `table_render.go`, add a small `page.Document() *Document` accessor or use direct field reference if same package.
- `AddBlankPage` requires `width, height` (the existing API). Confirm via `Page.Size()`.

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestAddTable_Overflow' -v ./...
go test ./...
git add table_render.go table_test.go
git commit -m "feat: tables — multi-page overflow (auto-append continuation pages, return pagesAdded)"
```

---

## Task 10: Repeating headers verified end-to-end

The drawHeaders calls in Task 9 already implement repeat. This task adds discriminative tests.

**Files:**
- Modify: `table_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestAddTable_HeadersRepeatOnEachOverflowPage(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)

    table := pdf.NewTable().
        SetColumnWidths([]float64{100}).
        SetDefaultCellStyle(pdf.TextStyle{Size: 12}).
        SetDefaultCellMargin(pdf.MarginInfo{Top: 3, Right: 3, Bottom: 3, Left: 3})
    table.AddRow().AddCell("HDR-XYZ") // unique header text
    for i := 1; i <= 6; i++ {
        table.AddRow().AddCell(fmt.Sprintf("row%d", i))
    }
    table.SetRepeatingRowsCount(1)

    // Small rect → 1 header + ~2 body rows per page → 3 pages total.
    pagesAdded, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 700, URX: 200, URY: 760})
    if err != nil {
        t.Fatal(err)
    }
    if pagesAdded < 1 {
        t.Fatalf("expected overflow, pagesAdded = %d", pagesAdded)
    }
    var buf bytes.Buffer
    if _, err := doc.WriteTo(&buf); err != nil {
        t.Fatal(err)
    }
    doc2, err := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    if err != nil {
        t.Fatal(err)
    }
    // Header text must appear on EVERY page that contains body content.
    headerPages := 0
    for p := 1; p <= doc2.PageCount(); p++ {
        page, _ := doc2.Page(p)
        text, _ := page.ExtractText()
        if strings.Contains(text, "HDR-XYZ") {
            headerPages++
        }
    }
    if headerPages != doc2.PageCount() {
        t.Errorf("header appeared on %d of %d pages; want all", headerPages, doc2.PageCount())
    }
}

func TestAddTable_NoHeaderRepeatByDefault(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)

    table := pdf.NewTable().
        SetColumnWidths([]float64{100}).
        SetDefaultCellStyle(pdf.TextStyle{Size: 12}).
        SetDefaultCellMargin(pdf.MarginInfo{Top: 3, Right: 3, Bottom: 3, Left: 3})
    table.AddRow().AddCell("HDR-XYZ")
    for i := 1; i <= 6; i++ {
        table.AddRow().AddCell(fmt.Sprintf("row%d", i))
    }
    // NOTE: NO SetRepeatingRowsCount call → default 0.

    pagesAdded, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 700, URX: 200, URY: 760})
    if err != nil {
        t.Fatal(err)
    }
    if pagesAdded < 1 {
        t.Fatal("expected overflow")
    }
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    headerPages := 0
    for p := 1; p <= doc2.PageCount(); p++ {
        page, _ := doc2.Page(p)
        text, _ := page.ExtractText()
        if strings.Contains(text, "HDR-XYZ") {
            headerPages++
        }
    }
    // Without repeat, header appears on exactly 1 page (the first).
    if headerPages != 1 {
        t.Errorf("header without repeat appeared on %d pages; want exactly 1", headerPages)
    }
}
```

- [ ] **Step 2: Run + verify they pass**

Both tests should pass against the Task 9 implementation. If they don't, that means a bug in the multi-page loop's header handling.

- [ ] **Step 3: Commit**

```powershell
go test -run 'TestAddTable_(HeadersRepeat|NoHeaderRepeat)' -v ./...
go test ./...
git add table_test.go
git commit -m "test: tables — repeating headers across overflow pages"
```

---

## Task 11: Rowspan crossing header/body boundary → error

**Files:**
- Modify: `table_render.go`
- Modify: `table_test.go`

- [ ] **Step 1: Append failing test**

```go
func TestAddTable_RowSpanCrossingHeaderBodyErrors(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)

    table := pdf.NewTable().SetColumnWidths([]float64{50, 50})
    row0 := table.AddRow()
    row0.AddCell("header tall").SetRowSpan(2) // extends from header into body
    row0.AddCell("a")
    table.AddRow().AddCell("b") // col 0 is covered by row 0's rowspan
    table.SetRepeatingRowsCount(1)

    _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 100})
    if err == nil {
        t.Error("expected error: rowspan from header into body")
    }
}
```

- [ ] **Step 2: Run + observe failure**

Currently the validation isn't checking this.

- [ ] **Step 3: Add validation in `AddTable`**

After `validateAndCover` and the `repeatingRowsCount` bounds check, add:

```go
// Rowspan crossing the header/body boundary is rejected (Phase 2 hard rule).
if t.repeatingRowsCount > 0 {
    for i := 0; i < t.repeatingRowsCount; i++ {
        for _, cell := range t.rows[i].cells {
            rs := cell.RowSpan()
            if i+rs > t.repeatingRowsCount {
                return 0, fmt.Errorf(
                    "add table: rowspan at header row %d (span %d) extends into body (rowspan-cross-header not supported)",
                    i, rs)
            }
        }
    }
}
```

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestAddTable_RowSpanCrossingHeaderBody' -v ./...
go test ./...
git add table_render.go table_test.go
git commit -m "feat: tables — reject rowspan crossing header/body boundary"
```

---

## Task 12: Rowspan group survives page break

The Task 9 implementation already handles this via `computeSpanningGroups`. This task adds discriminative tests.

**Files:**
- Modify: `table_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestAddTable_RowSpanGroupSurvivesPageBreak(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)

    table := pdf.NewTable().
        SetColumnWidths([]float64{60, 60}).
        SetDefaultCellStyle(pdf.TextStyle{Size: 12}).
        SetDefaultCellMargin(pdf.MarginInfo{Top: 3, Right: 3, Bottom: 3, Left: 3})
    // Row 0-3: regular. Row 4-5: rowspan group (cell at col 0 spans both).
    for i := 0; i < 4; i++ {
        table.AddRow().AddCells(fmt.Sprintf("a%d", i), fmt.Sprintf("b%d", i))
    }
    row4 := table.AddRow()
    row4.AddCell("SPAN").SetRowSpan(2)
    row4.AddCell("b4")
    table.AddRow().AddCell("b5") // col 0 covered

    // Tight rect: 4 rows fit on the first page, then rows 4+5 must move
    // together as a group to the continuation page.
    pagesAdded, err := page.AddTable(table, pdf.Rectangle{
        LLX: 0, LLY: 670, URX: 200, URY: 760,
    })
    if err != nil {
        t.Fatal(err)
    }
    if pagesAdded < 1 {
        t.Fatal("expected overflow page")
    }

    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))

    // "SPAN" + "b4" + "b5" must all appear on the SAME page (not split).
    spanPage, b4Page, b5Page := -1, -1, -1
    for p := 1; p <= doc2.PageCount(); p++ {
        pg, _ := doc2.Page(p)
        text, _ := pg.ExtractText()
        if strings.Contains(text, "SPAN") {
            spanPage = p
        }
        if strings.Contains(text, "b4") {
            b4Page = p
        }
        if strings.Contains(text, "b5") {
            b5Page = p
        }
    }
    if spanPage == -1 || b4Page == -1 || b5Page == -1 {
        t.Fatalf("missing piece: span=%d b4=%d b5=%d", spanPage, b4Page, b5Page)
    }
    if spanPage != b4Page || spanPage != b5Page {
        t.Errorf("rowspan group split across pages: span=%d b4=%d b5=%d", spanPage, b4Page, b5Page)
    }
}
```

- [ ] **Step 2: Run + verify**

Should pass against Task 9 implementation. If not, debug the spanning-group decision in the overflow loop.

- [ ] **Step 3: Commit**

```powershell
go test -run 'TestAddTable_RowSpanGroupSurvivesPageBreak' -v ./...
go test ./...
git add table_test.go
git commit -m "test: tables — rowspan group survives page break atomically"
```

---

## Task 13: Cross-cutting — AES-128 with overflow

**Files:**
- Modify: `table_test.go`

- [ ] **Step 1: Append failing test**

```go
func TestAddTable_OverflowAES128Roundtrip(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)

    table := pdf.NewTable().
        SetColumnWidths([]float64{100}).
        SetDefaultCellStyle(pdf.TextStyle{Size: 12}).
        SetDefaultCellMargin(pdf.MarginInfo{Top: 3, Right: 3, Bottom: 3, Left: 3})
    table.AddRow().AddCell("encrypted header")
    for i := 1; i <= 5; i++ {
        table.AddRow().AddCell(fmt.Sprintf("encrypted row%d", i))
    }
    table.SetRepeatingRowsCount(1)

    pagesAdded, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 700, URX: 200, URY: 760})
    if err != nil {
        t.Fatal(err)
    }
    if pagesAdded < 1 {
        t.Fatal("expected overflow")
    }
    doc.SetEncryption(pdf.EncryptionOptions{
        UserPassword: "x",
        Algorithm:    pdf.EncryptionAlgAES128,
    })

    var buf bytes.Buffer
    if _, err := doc.WriteTo(&buf); err != nil {
        t.Fatal(err)
    }
    doc2, err := pdf.OpenStreamWithPassword(bytes.NewReader(buf.Bytes()), "x")
    if err != nil {
        t.Fatal(err)
    }
    // Verify all 5 rows survive + header appears on each page.
    foundRows := 0
    for p := 1; p <= doc2.PageCount(); p++ {
        pg, _ := doc2.Page(p)
        text, _ := pg.ExtractText()
        if !strings.Contains(text, "encrypted header") {
            t.Errorf("page %d missing repeated header", p)
        }
        for i := 1; i <= 5; i++ {
            if strings.Contains(text, fmt.Sprintf("encrypted row%d", i)) {
                foundRows++
            }
        }
    }
    if foundRows != 5 {
        t.Errorf("found %d body rows across pages; want 5", foundRows)
    }
}
```

- [ ] **Step 2: Run + verify**

Should pass — encryption applies at write time per object, doesn't care about page count.

- [ ] **Step 3: Commit**

```powershell
go test -run 'TestAddTable_OverflowAES128Roundtrip' -v ./...
go test ./...
git add table_test.go
git commit -m "test: tables — overflow + repeating headers under AES-128 encryption"
```

---

## Task 14: Aspose .NET parity tests for Phase 2

**Files:**
- Modify: `table_aspose_parity_test.go`

- [ ] **Step 1: Append tests**

```go
// Aspose .NET sample:
//   Table table = new Table();
//   table.ColumnWidths = "100 200 100";
//   table.RepeatingRowsCount = 1;
//   Row header = table.Rows.Add();
//   header.Cells.Add("Header A");
//   header.Cells.Add("Header B");
//   header.Cells.Add("Header C");
//   for (int i = 0; i < 50; i++) {
//       Row r = table.Rows.Add();
//       r.Cells.Add(...);
//   }
//   page.Paragraphs.Add(table); // auto-flows across pages
func TestAsposeParity_TableRepeatingRows(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)

    table := pdf.NewTable().
        SetColumnWidths([]float64{80, 160, 80}).
        SetRepeatingRowsCount(1)
    header := table.AddRow()
    header.AddCells("Header A", "Header B", "Header C")
    for i := 0; i < 30; i++ {
        table.AddRow().AddCells(fmt.Sprintf("a%d", i), fmt.Sprintf("b%d", i), fmt.Sprintf("c%d", i))
    }

    pagesAdded, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 100, URX: 370, URY: 720})
    if err != nil {
        t.Fatal(err)
    }
    if pagesAdded < 1 {
        t.Fatalf("expected overflow, pagesAdded = %d", pagesAdded)
    }
}

// Aspose .NET sample:
//   Cell cell = row.Cells.Add("TOTAL");
//   cell.ColSpan = 3;
//   cell.Alignment = HorizontalAlignment.Right;
func TestAsposeParity_CellColSpan(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)

    table := pdf.NewTable().SetColumnWidths([]float64{80, 80, 80, 80})
    row := table.AddRow()
    row.AddCell("Item 1").SetHAlign(pdf.HAlignLeft)
    row.AddCell("Item 2").SetHAlign(pdf.HAlignLeft)
    row.AddCell("TOTAL").SetColSpan(2).SetHAlign(pdf.HAlignRight)

    if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 600, URX: 370, URY: 700}); err != nil {
        t.Fatal(err)
    }
}

// Aspose .NET sample:
//   Cell cell = row.Cells.Add("Category");
//   cell.RowSpan = 3;
func TestAsposeParity_CellRowSpan(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)

    table := pdf.NewTable().SetColumnWidths([]float64{100, 100})
    row0 := table.AddRow()
    row0.AddCell("Category").SetRowSpan(3)
    row0.AddCell("Item 1")
    table.AddRow().AddCell("Item 2") // col 0 covered
    table.AddRow().AddCell("Item 3") // col 0 covered

    if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 700}); err != nil {
        t.Fatal(err)
    }
}
```

- [ ] **Step 2: Run + commit**

```powershell
go test -run 'TestAsposeParity_(TableRepeatingRows|CellColSpan|CellRowSpan)' -v ./...
go test ./...
git add table_aspose_parity_test.go
git commit -m "test: Aspose .NET parity tests for Table Phase 2 (RepeatingRows, ColSpan, RowSpan)"
```

---

## Task 15: Restaurant bill — refactor summary rows with ColSpan

**Optional but visible.** Updates `my_examples/full_scenario/main.go` to use ColSpan in the summary rows (Subtotal / Tax / Service / TOTAL), removing the placeholder empty cells.

**Files:**
- Modify: `my_examples/full_scenario/main.go`

- [ ] **Step 1: Find `addSummary` helper**

In `addRestaurantBill`, the current `addSummary` is:

```go
addSummary := func(label string, amount float64, bold bool, bg *pdf.Color) {
    // ... loop 2 empty cells + label cell + amount cell
}
```

- [ ] **Step 2: Rewrite using ColSpan**

Replace with:

```go
addSummary := func(label string, amount float64, bold bool, bg *pdf.Color) {
    labelStyle := pdf.TextStyle{Font: pdf.FontHelvetica, Size: 11}
    amountStyle := labelStyle
    if bold {
        labelStyle.Font = pdf.FontHelveticaBold
        amountStyle.Font = pdf.FontHelveticaBold
        labelStyle.Size = 12
        amountStyle.Size = 12
    }
    row := table.AddRow()
    lc := row.AddCell(label).SetColSpan(3).SetTextStyle(labelStyle).SetHAlign(pdf.HAlignRight)
    if bg != nil {
        lc.SetBackground(bg)
    }
    ac := row.AddCell(fmt.Sprintf("€%.2f", amount)).SetTextStyle(amountStyle).SetHAlign(pdf.HAlignRight)
    if bg != nil {
        ac.SetBackground(bg)
    }
}
```

- [ ] **Step 3: Verify**

```powershell
go run ./my_examples/full_scenario
```

Output PDF should still render the bill but with cleaner summary rows (the label cell now spans 3 columns instead of using 2 padding cells).

- [ ] **Step 4: Commit**

The `my_examples/` directory is gitignored — there's nothing to commit. Skip the commit step.

---

## Task 16: Docs + close beads issue

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`

- [ ] **Step 1: Update CLAUDE.md Tables section**

In the existing `**\`table.go\` / \`table_render.go\`**` block, append these bullets at the end (before the next file section):

```markdown
- `Cell.SetColSpan(n) / ColSpan() int` — cell occupies n consecutive columns; default 1. When set, the caller does not add cells for the columns covered by the span — the row simply has fewer cells. Mirrors Aspose.PDF for .NET's `Cell.ColSpan`
- `Cell.SetRowSpan(n) / RowSpan() int` — cell occupies n consecutive rows; default 1. Covered positions in subsequent rows are skipped by the caller. Mirrors Aspose.PDF for .NET's `Cell.RowSpan`
- `Table.SetRepeatingRowsCount(n) / RepeatingRowsCount() int` — marks the first n rows as headers that repeat at the top of every continuation page (default 0). Mirrors Aspose.PDF for .NET's `Table.RepeatingRowsCount`
- `Table.SetOverflowMargins(top, bottom) / OverflowMargins()` — top/bottom margins (points) for the continuation rect on auto-appended pages; defaults 50pt each. Same LLX/URX as the original rect; Y range = [bottom, pageHeight - top]
- `(*Page).AddTable(t, rect) (pagesAdded int, err error)` — now returns the number of continuation pages auto-appended (0 if the table fits in rect). Validation also rejects: ColSpan/RowSpan out of bounds, merge overlaps, rowspan crossing the header/body boundary, header height exceeding rect height, or any spanning group too tall for a continuation page
- Spanning groups: rows linked by rowspan are atomic — a group never breaks across pages. Each group is the smallest contiguous range [s, e] such that no rowspan in [s, e] extends past e. Page-break decisions operate on groups, not individual rows
```

- [ ] **Step 2: Update README.md**

In the Tables Features bullet (around line 36-37), append a sentence on Phase 2:

```markdown
- **Tables** — ... Auto-fit row heights or `Row.SetHeight` explicit. Cell text reuses the full `AddText` machinery (word-wrap, alignment, font embedding, Unicode). **Multi-page overflow with automatic page append**; **repeating header rows** via `Table.SetRepeatingRowsCount`; **cell merging** via `Cell.SetColSpan` / `SetRowSpan`
```

In the `### Tables` section snippet, add an example showing repeating headers + colspan:

```go
// After the existing example, append:

// Multi-page invoice with repeating header + merged TOTAL row:
table.SetRepeatingRowsCount(1)
for _, item := range invoiceItems {
    row := table.AddRow()
    row.AddCells(item.Name, item.Description, fmt.Sprintf("€%.2f", item.Price))
}
// Summary row spans the first two columns:
totals := table.AddRow()
totals.AddCell("TOTAL").SetColSpan(2).SetHAlign(pdf.HAlignRight)
totals.AddCell(fmt.Sprintf("€%.2f", grand))

pagesAdded, _ := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 100, URX: 545, URY: 750})
fmt.Printf("table flowed to %d additional pages\n", pagesAdded)
```

(Adjust placement and exact wording when editing — anchor near the existing snippet, before the trailing parity paragraph.)

- [ ] **Step 3: Run full suite + commit**

```powershell
go test ./...
go vet ./...
git add CLAUDE.md README.md
git commit -m "docs: tables Phase 2 (overflow, RepeatingRows, ColSpan/RowSpan) in CLAUDE.md and README"
```

- [ ] **Step 4: Close beads issue**

```powershell
bd update pdf-go-2h3 --status closed --append-notes "Tables Phase 2 shipped 2026-05-19. Public API: (*Page).AddTable returns (int, error); Table.SetRepeatingRowsCount + SetOverflowMargins; Cell.SetColSpan + SetRowSpan. Aspose .NET parity. Spanning groups are atomic across page breaks. Rowspan crossing header/body boundary rejected. AES-128 + multi-page roundtrip verified. Out of scope (Phase 3 candidates): image cells, border edge dedup, dash patterns, auto-fit column widths, convenience helpers (AddRows, Row.SetBackground)."
```

Report the bd output.

---

## Self-review

**Spec coverage:**

| Spec section | Task(s) |
|---|---|
| Breaking signature change | 1 |
| Cell.SetColSpan / RowSpan | 2 |
| Table.SetRepeatingRowsCount / SetOverflowMargins | 3 |
| Span-aware validation (covered grid, overlap) | 4 |
| Auto-fit row heights with merging | 5 |
| Single-page rendering with span | 6 |
| Spanning groups computation | 7 |
| Extract render helpers + RepeatingRowsCount validation | 8 |
| Multi-page overflow loop | 9 |
| Repeating headers across pages | 10 |
| Rowspan crossing header/body → error | 11 |
| Rowspan group atomic across page breaks | 12 |
| Cross-cutting (AES + overflow) | 13 |
| Aspose .NET parity | 14 |
| Restaurant bill refactor (optional) | 15 |
| Docs + close umbrella | 16 |

**Placeholder scan:** every task has concrete code, exact error messages, exact commit messages.

**Type consistency:** `spanGroup` used in Task 7 + Task 9. `covered [][]bool` from Task 4 used in Tasks 5, 6, 8, 9. `xOffsets []float64` introduced in Task 6 used in Tasks 8, 9. `drawRowRange` + `drawOuterBorder` introduced in Task 8 used in Task 9.

**Estimated total:** ~16 tasks × 20–45 minutes each = 7–10 hours of focused work. Production code ~600 LOC delta. Tests ~700 LOC delta. Docs ~80 LOC delta.
