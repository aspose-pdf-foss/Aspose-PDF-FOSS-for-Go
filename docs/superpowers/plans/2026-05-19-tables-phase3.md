# Tables Phase 3 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add image cells, border edge de-duplication, and row-level styling + AddRows convenience to the Tables API. Aspose .NET-parity where applicable.

**Architecture:** Image cells decode dimensions for measurement, delegate actual rendering to the existing `(*Page).AddImage`. Border dedup tracks emitted edges in a per-page set keyed by rounded coordinates. Row layer slots between Table defaults and Cell overrides in the effective-property chain.

**Tech Stack:** Go 1.24, standard library only.

**Reference:** [docs/superpowers/specs/2026-05-19-tables-phase3-design.md](../specs/2026-05-19-tables-phase3-design.md)

**Beads:** [pdf-go-8nv](bd show pdf-go-8nv)

**Phase 2:** [docs/superpowers/plans/2026-05-19-tables-phase2.md](2026-05-19-tables-phase2.md) — completed at commit `18a19b2`.

---

## File Map

| File | Purpose |
|---|---|
| `table.go` (modify) | Image fields on Cell; row-level style fields on Row; `AddRows` on Table; all getters/setters. |
| `table_render.go` (modify) | Extended `effective*` helpers for row layer + `effectiveCellBackground`; per-side `drawBorderSide` + `edgeSet`; image rendering wired into `drawRowRange`; image height in `computeRowHeights`. |
| `table_image.go` (new) | `measureImage` (header-only decode for natural dims) + image-cell rendering helper. |
| `table_test.go` (modify) | Image cell tests, dedup tests, row styling tests, AddRows tests. |
| `table_internal_test.go` (modify) | `effective*` row-layer tests; edgeSet dedup unit tests. |
| `table_aspose_parity_test.go` (modify) | 3 Phase 3 parity tests. |
| `CLAUDE.md` / `README.md` (modify, Task 15) | Phase 3 doc additions. |

---

## Task 1: Row-level style fields + setters/getters

**Files:**
- Modify: `table.go`
- Modify: `table_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestRow_BackgroundDefault(t *testing.T) {
    r := pdf.NewTable().SetColumnWidths([]float64{50}).AddRow()
    if r.Background() != nil { t.Error("default Background should be nil") }
    if r.TextStyle() != nil { t.Error("default TextStyle should be nil") }
    if r.Border() != nil { t.Error("default Border should be nil") }
    if r.Margin() != nil { t.Error("default Margin should be nil") }
}

func TestRow_SettersAndChaining(t *testing.T) {
    bg := &pdf.Color{R: 0.9, G: 0.9, B: 0.9, A: 1}
    r := pdf.NewTable().SetColumnWidths([]float64{50}).AddRow().
        SetBackground(bg).
        SetTextStyle(pdf.TextStyle{Size: 14}).
        SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideTop, Width: 1}).
        SetMargin(pdf.MarginInfo{Top: 3, Right: 4, Bottom: 3, Left: 4})

    if r.Background() != bg { t.Error("Background pointer not preserved") }
    if r.TextStyle() == nil || r.TextStyle().Size != 14 { t.Errorf("TextStyle = %+v", r.TextStyle()) }
    if r.Border() == nil || r.Border().Sides != pdf.BorderSideTop { t.Errorf("Border = %+v", r.Border()) }
    if r.Margin() == nil || r.Margin().Left != 4 { t.Errorf("Margin = %+v", r.Margin()) }
}
```

- [ ] **Step 2: Run + observe build failure**

```powershell
go test -run 'TestRow_(BackgroundDefault|SettersAndChaining)' -v ./...
```

- [ ] **Step 3: Add fields + methods to `table.go`**

Add to `Row` struct (after existing `height float64`):

```go
type Row struct {
    table     *Table
    cells     []*Cell
    height    float64
    // Phase 3: row-level styling layer (between table defaults and per-cell overrides).
    background *Color
    textStyle  *TextStyle
    border     *BorderInfo
    margin     *MarginInfo
}
```

Append methods at the bottom of `table.go`:

```go
// SetBackground sets a row-level background color. Cells in the row inherit
// this background unless the cell itself calls SetBackground.
func (r *Row) SetBackground(col *Color) *Row { r.background = col; return r }
func (r *Row) Background() *Color           { return r.background }

// SetTextStyle sets a row-level default text style. Cells in the row inherit
// this style (overlaid on table.DefaultCellStyle) unless overridden per-cell.
func (r *Row) SetTextStyle(s TextStyle) *Row { r.textStyle = &s; return r }
func (r *Row) TextStyle() *TextStyle        { return r.textStyle }

// SetBorder sets a row-level default border. Cells inherit unless overridden.
func (r *Row) SetBorder(b BorderInfo) *Row { r.border = &b; return r }
func (r *Row) Border() *BorderInfo        { return r.border }

// SetMargin sets a row-level default cell padding. Cells inherit unless overridden.
func (r *Row) SetMargin(m MarginInfo) *Row { r.margin = &m; return r }
func (r *Row) Margin() *MarginInfo        { return r.margin }
```

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestRow_' -v ./...
go test ./...
git add table.go table_test.go
git commit -m "feat: tables — Row.Set{Background,TextStyle,Border,Margin} + getters"
```

---

## Task 2: Extend effective* helpers for row layer + add `effectiveCellBackground`

**Files:**
- Modify: `table_render.go`
- Modify: `table_internal_test.go`

- [ ] **Step 1: Append failing internal tests**

```go
func TestEffectiveCellStyle_RowLayer(t *testing.T) {
    table := NewTable().SetColumnWidths([]float64{50}).
        SetDefaultCellStyle(TextStyle{Font: FontHelvetica, Size: 10})
    row := table.AddRow().SetTextStyle(TextStyle{Size: 14}) // overrides Size
    cell := row.AddCell("x")
    style := effectiveCellStyle(table, cell)
    if style.Size != 14 {
        t.Errorf("row layer override: Size = %g, want 14", style.Size)
    }
    if style.Font != FontHelvetica {
        t.Error("table-default Font should survive row overlay")
    }
}

func TestEffectiveCellStyle_CellWinsOverRow(t *testing.T) {
    table := NewTable().SetColumnWidths([]float64{50}).
        SetDefaultCellStyle(TextStyle{Size: 10})
    row := table.AddRow().SetTextStyle(TextStyle{Size: 14})
    cell := row.AddCell("x").SetTextStyle(TextStyle{Size: 18}) // overrides row
    style := effectiveCellStyle(table, cell)
    if style.Size != 18 {
        t.Errorf("cell wins over row: Size = %g, want 18", style.Size)
    }
}

func TestEffectiveCellMargin_RowLayer(t *testing.T) {
    table := NewTable().SetColumnWidths([]float64{50}).
        SetDefaultCellMargin(MarginInfo{Top: 1, Right: 1, Bottom: 1, Left: 1})
    row := table.AddRow().SetMargin(MarginInfo{Top: 5, Right: 5, Bottom: 5, Left: 5})
    cell := row.AddCell("x")
    m := effectiveCellMargin(table, cell)
    if m.Top != 5 || m.Left != 5 {
        t.Errorf("row layer margin: %+v, want all 5s", m)
    }
}

func TestEffectiveCellBorder_RowLayer(t *testing.T) {
    table := NewTable().SetColumnWidths([]float64{50}).
        SetDefaultCellBorder(BorderInfo{Sides: BorderSideAll, Width: 0.5})
    row := table.AddRow().SetBorder(BorderInfo{Sides: BorderSideAll, Width: 2})
    cell := row.AddCell("x")
    b := effectiveCellBorder(table, cell)
    if b.Width != 2 {
        t.Errorf("row layer border width = %g, want 2", b.Width)
    }
}

func TestEffectiveCellBackground_Chain(t *testing.T) {
    rowBG := &Color{R: 0.9, G: 0.9, B: 0.9, A: 1}
    cellBG := &Color{R: 1, G: 1, B: 0, A: 1}
    table := NewTable().SetColumnWidths([]float64{50, 50})
    row := table.AddRow().SetBackground(rowBG)
    cellA := row.AddCell("a")                       // inherits row
    cellB := row.AddCell("b").SetBackground(cellBG) // overrides row

    if got := effectiveCellBackground(cellA); got != rowBG {
        t.Errorf("cellA bg = %v, want row %v", got, rowBG)
    }
    if got := effectiveCellBackground(cellB); got != cellBG {
        t.Errorf("cellB bg = %v, want cell %v", got, cellBG)
    }

    cellC := pdf_buildRowless()
    _ = cellC // sanity (helper not actually defined; just illustrative — drop this branch if it complicates)
}
```

Drop the last branch (`cellC`) if it requires a helper that doesn't exist. The first 4 cases are the load-bearing ones.

- [ ] **Step 2: Run + observe failure**

```powershell
go test -run 'TestEffectiveCell' -v ./...
```

`effectiveCellBackground` undefined; existing `effectiveCellStyle`/`Margin`/`Border` don't consult the row layer yet.

- [ ] **Step 3: Update helpers in `table_render.go`**

Add `effectiveCellBackground` and extend the existing 3 helpers.

```go
// effectiveCellBackground walks the per-cell → per-row chain. Returns nil if
// neither cell nor row sets a background.
func effectiveCellBackground(c *Cell) *Color {
    if c.background != nil {
        return c.background
    }
    if c.row != nil && c.row.background != nil {
        return c.row.background
    }
    return nil
}
```

Extend `effectiveCellMargin`:

```go
func effectiveCellMargin(t *Table, c *Cell) MarginInfo {
    if c.margin != nil {
        return *c.margin
    }
    if c.row != nil && c.row.margin != nil {
        return *c.row.margin
    }
    return t.defaultCellMargin
}
```

Extend `effectiveCellBorder`:

```go
func effectiveCellBorder(t *Table, c *Cell) BorderInfo {
    if c.border != nil {
        return *c.border
    }
    if c.row != nil && c.row.border != nil {
        return *c.row.border
    }
    return t.defaultCellBorder
}
```

Extend `effectiveCellStyle` to insert the row layer between table.defaultCellStyle and cell.style:

```go
func effectiveCellStyle(t *Table, c *Cell) TextStyle {
    style := t.defaultCellStyle
    if c.row != nil && c.row.textStyle != nil {
        style = overlayTextStyle(style, *c.row.textStyle)
    }
    if c.style != nil {
        style = overlayTextStyle(style, *c.style)
    }
    if c.hAlignSet {
        style.HAlign = c.hAlign
    }
    if c.vAlignSet {
        style.VAlign = c.vAlign
    }
    return style
}
```

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestEffectiveCell' -v ./...
go test ./...
git add table_render.go table_internal_test.go
git commit -m "feat: tables — effective* helpers consult row layer; new effectiveCellBackground"
```

---

## Task 3: Render row-level background end-to-end

`drawRowRange` currently uses `cell.background` directly. Replace with `effectiveCellBackground(cell)` so row-level backgrounds reach the renderer.

**Files:**
- Modify: `table_render.go`
- Modify: `table_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestAddTable_RowBackgroundAppliesToAllCells(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    table := pdf.NewTable().SetColumnWidths([]float64{50, 50, 50})
    row := table.AddRow().SetBackground(&pdf.Color{R: 0.85, G: 0, B: 0, A: 1})
    row.AddCells("a", "b", "c")
    if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 200, URY: 100}); err != nil {
        t.Fatal(err)
    }
    s := renderedContent(t, doc)
    // Red fill: 0.85 0 0 rg + re + f — should appear (one or more times for the 3 cells).
    if !strings.Contains(s, "0.85 0 0 rg") {
        t.Error("row-level red background not emitted")
    }
}

func TestAddTable_CellBackgroundWinsOverRow(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    table := pdf.NewTable().SetColumnWidths([]float64{50, 50})
    row := table.AddRow().SetBackground(&pdf.Color{R: 0.85, G: 0, B: 0, A: 1}) // red
    row.AddCell("a")
    row.AddCell("b").SetBackground(&pdf.Color{R: 0, G: 0, B: 0.85, A: 1}) // blue
    if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50}); err != nil {
        t.Fatal(err)
    }
    s := renderedContent(t, doc)
    if !strings.Contains(s, "0.85 0 0 rg") {
        t.Error("row red fill missing for cell A")
    }
    if !strings.Contains(s, "0 0 0.85 rg") {
        t.Error("cell B's blue override missing")
    }
}
```

- [ ] **Step 2: Run + observe failure**

```powershell
go test -run 'TestAddTable_(RowBackground|CellBackgroundWins)' -v ./...
```

`drawRowRange` still uses `cell.background` directly.

- [ ] **Step 3: Update `drawRowRange` in `table_render.go`**

Find the background-drawing block in `drawRowRange`:

```go
if cell.background != nil {
    if err := targetPage.appendToContentStream([]byte(
        drawCellBackground(cellLLX, cellLLY, cellURX, cellURY, cell.background),
    )); err != nil {
        return drawnHeight, fmt.Errorf("row %d col %d background: %w", i, col, err)
    }
}
```

Replace with:

```go
if bg := effectiveCellBackground(cell); bg != nil {
    if err := targetPage.appendToContentStream([]byte(
        drawCellBackground(cellLLX, cellLLY, cellURX, cellURY, bg),
    )); err != nil {
        return drawnHeight, fmt.Errorf("row %d col %d background: %w", i, col, err)
    }
}
```

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestAddTable_(RowBackground|CellBackgroundWins)' -v ./...
go test ./...
git add table_render.go table_test.go
git commit -m "feat: tables — render row-level background via effectiveCellBackground"
```

---

## Task 4: `Table.AddRows` batch constructor

**Files:**
- Modify: `table.go`
- Modify: `table_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestTable_AddRows_BatchReturnsRowsInOrder(t *testing.T) {
    table := pdf.NewTable().SetColumnWidths([]float64{50, 50})
    rows := table.AddRows([][]string{
        {"a1", "a2"},
        {"b1", "b2"},
        {"c1", "c2"},
    })
    if len(rows) != 3 {
        t.Fatalf("got %d rows, want 3", len(rows))
    }
    if table.RowCount() != 3 {
        t.Errorf("RowCount = %d, want 3", table.RowCount())
    }
    if rows[1].Cells()[0].Text() != "b1" {
        t.Errorf("rows[1][0] = %q, want b1", rows[1].Cells()[0].Text())
    }
}

func TestTable_AddRows_EmptyInputs(t *testing.T) {
    table := pdf.NewTable().SetColumnWidths([]float64{50})
    if got := table.AddRows(nil); len(got) != 0 {
        t.Errorf("AddRows(nil) = %d rows, want 0", len(got))
    }
    if got := table.AddRows([][]string{}); len(got) != 0 {
        t.Errorf("AddRows([]) = %d rows, want 0", len(got))
    }
    if table.RowCount() != 0 {
        t.Errorf("RowCount = %d, want 0 (empty inputs add nothing)", table.RowCount())
    }
}
```

- [ ] **Step 2: Run + observe build failure**

- [ ] **Step 3: Add `AddRows` to `table.go`**

Place it next to the existing `AddRow`:

```go
// AddRows is a convenience that creates one Row per inner slice and one Cell
// per string in that slice. Returns the rows in insertion order so callers
// can apply further per-row customization (SetBackground, SetHeight, etc.).
//
// Rows added through AddRows have plain cells — no ColSpan/RowSpan. For
// spanning, use the explicit AddRow + Cell.SetColSpan flow.
func (t *Table) AddRows(rows [][]string) []*Row {
    out := make([]*Row, 0, len(rows))
    for _, texts := range rows {
        r := t.AddRow()
        r.AddCells(texts...)
        out = append(out, r)
    }
    return out
}
```

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestTable_AddRows_' -v ./...
go test ./...
git add table.go table_test.go
git commit -m "feat: tables — Table.AddRows([][]string) batch constructor"
```

---

## Task 5: `Cell.SetImage` / `SetImageFromStream` / `Image()` accessors

Pure type addition. No rendering integration yet — that's Tasks 7-8.

**Files:**
- Modify: `table.go`
- Modify: `table_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestCell_ImageDefault(t *testing.T) {
    cell := pdf.NewTable().SetColumnWidths([]float64{50}).AddRow().AddCell("x")
    path, hasImage := cell.Image()
    if path != "" || hasImage {
        t.Errorf("default Image() = (%q, %v), want ('', false)", path, hasImage)
    }
}

func TestCell_SetImageChaining(t *testing.T) {
    cell := pdf.NewTable().SetColumnWidths([]float64{50}).AddRow().AddCell("ignored").
        SetImage("testdata/Koala.jpg")
    path, hasImage := cell.Image()
    if !hasImage {
        t.Error("hasImage should be true after SetImage")
    }
    if path != "testdata/Koala.jpg" {
        t.Errorf("Image() path = %q", path)
    }
}

func TestCell_SetImageFromStream(t *testing.T) {
    cell := pdf.NewTable().SetColumnWidths([]float64{50}).AddRow().AddCell("").
        SetImageFromStream(bytes.NewReader([]byte("dummy")))
    path, hasImage := cell.Image()
    if !hasImage {
        t.Error("hasImage should be true after SetImageFromStream")
    }
    if path != "" {
        t.Errorf("path should be empty for stream-set image, got %q", path)
    }
}
```

Make sure `bytes` is imported in `table_test.go` (it should already be).

- [ ] **Step 2: Run + observe build failure**

- [ ] **Step 3: Add fields + methods to `table.go`**

Add to `Cell` struct (after existing fields):

```go
type Cell struct {
    // ... existing fields ...
    // Phase 3: image cells (image overrides text rendering when both set).
    imagePath   string    // file path; empty when image set from stream
    imageStream []byte    // buffered stream bytes; nil when image set from path or unset
    hasImage    bool
}
```

Add methods:

```go
// SetImage configures the cell to render the named image instead of text.
// PNG and JPEG supported (format auto-detected by magic bytes).
//
// If both SetText and SetImage are configured, the image wins.
//
// The image is auto-sized to fit the cell's interior width (sum of spanned
// column widths minus padding), preserving aspect ratio. Row height auto-fit
// computes the resulting image height plus margins. Use Row.SetHeight to
// override row height; the image then scales down to fit if needed.
//
// Mirrors Aspose.PDF for .NET's Cell.Image property.
func (c *Cell) SetImage(path string) *Cell {
    c.imagePath = path
    c.imageStream = nil
    c.hasImage = true
    return c
}

// SetImageFromStream is the io.Reader-based counterpart. The full stream is
// buffered immediately so that the table renderer can measure and draw later.
func (c *Cell) SetImageFromStream(r io.Reader) *Cell {
    data, err := io.ReadAll(r)
    if err != nil {
        // Defer the error to AddTable time; store a sentinel that triggers
        // a clean validation error there.
        c.imagePath = ""
        c.imageStream = nil
        c.hasImage = true // marks the cell as image-bearing
        return c
    }
    c.imagePath = ""
    c.imageStream = data
    c.hasImage = true
    return c
}

// Image returns the configured image source. path is empty when the image
// was set from a stream. hasImage is true when any image source is set.
func (c *Cell) Image() (path string, hasImage bool) {
    return c.imagePath, c.hasImage
}
```

Add the `io` import if not present in `table.go`.

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestCell_(ImageDefault|SetImage)' -v ./...
go test ./...
git add table.go table_test.go
git commit -m "feat: tables — Cell.SetImage / SetImageFromStream + Image() accessor"
```

---

## Task 6: `measureImage` helper

A function that returns natural pixel dimensions of a PNG/JPEG without full pixel-decode.

**Files:**
- Create: `table_image.go`
- Modify: `table_internal_test.go`

- [ ] **Step 1: Append failing internal tests**

```go
func TestMeasureImage_JPEGFile(t *testing.T) {
    w, h, err := measureImage("testdata/Koala.jpg", nil)
    if err != nil {
        t.Fatal(err)
    }
    // Koala.jpg is 1024x768 (per Phase 1 image-add code).
    if w != 1024 || h != 768 {
        t.Errorf("Koala dims = (%g, %g), want (1024, 768)", w, h)
    }
}

func TestMeasureImage_FromStream(t *testing.T) {
    data, err := os.ReadFile("testdata/Koala.jpg")
    if err != nil {
        t.Fatal(err)
    }
    w, h, err := measureImage("", data)
    if err != nil {
        t.Fatal(err)
    }
    if w != 1024 || h != 768 {
        t.Errorf("stream dims = (%g, %g), want (1024, 768)", w, h)
    }
}

func TestMeasureImage_EmptyPathAndNilStreamErrors(t *testing.T) {
    _, _, err := measureImage("", nil)
    if err == nil {
        t.Error("expected error for empty path + nil stream")
    }
}

func TestMeasureImage_BadDataErrors(t *testing.T) {
    _, _, err := measureImage("", []byte("not an image"))
    if err == nil {
        t.Error("expected error for non-image bytes")
    }
}
```

Add the `os` import to `table_internal_test.go` if not present.

- [ ] **Step 2: Run + observe build failure**

- [ ] **Step 3: Create `table_image.go`**

```go
package asposepdf

import (
    "bytes"
    "fmt"
    "image"
    _ "image/jpeg" // register decoder
    _ "image/png"  // register decoder
    "io"
    "os"
)

// measureImage returns the natural (pixel) dimensions of a PNG or JPEG image.
//
// If path != "", the file is opened and decoded for dimensions. If path == ""
// and data != nil, the bytes are decoded directly. Returns an error if neither
// source is valid or the data is not a supported image format.
//
// Uses image.DecodeConfig (header-only decode — does not allocate pixel buffers).
func measureImage(path string, data []byte) (width, height float64, err error) {
    var r io.Reader
    switch {
    case path != "":
        f, ferr := os.Open(path)
        if ferr != nil {
            return 0, 0, fmt.Errorf("measureImage: %w", ferr)
        }
        defer f.Close()
        r = f
    case data != nil:
        r = bytes.NewReader(data)
    default:
        return 0, 0, fmt.Errorf("measureImage: empty path and nil data")
    }
    cfg, _, err := image.DecodeConfig(r)
    if err != nil {
        return 0, 0, fmt.Errorf("measureImage: %w", err)
    }
    return float64(cfg.Width), float64(cfg.Height), nil
}
```

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestMeasureImage' -v ./...
go test ./...
git add table_image.go table_internal_test.go
git commit -m "feat: tables — measureImage helper (header-only PNG/JPEG decode for natural dims)"
```

---

## Task 7: `computeRowHeights` auto-fit for image cells

**Files:**
- Modify: `table_render.go`
- Modify: `table_internal_test.go`

- [ ] **Step 1: Append failing test**

```go
func TestComputeRowHeights_ImageAutoFit(t *testing.T) {
    // 200pt column, 0 margins → interior width = 200.
    // Koala.jpg natural: 1024x768. Scale factor = 200/1024 = 0.1953125.
    // Scaled height = 768 * 0.1953125 = 150.
    table := NewTable().SetColumnWidths([]float64{200}).
        SetDefaultCellMargin(MarginInfo{Top: 0, Right: 0, Bottom: 0, Left: 0})
    table.AddRow().AddCell("").SetImage("testdata/Koala.jpg")
    heights, err := computeRowHeights(table)
    if err != nil {
        t.Fatal(err)
    }
    want := 200.0 * 768.0 / 1024.0 // 150
    if heights[0] != want {
        t.Errorf("image auto-fit row height = %g, want %g", heights[0], want)
    }
}

func TestComputeRowHeights_ImageRespectsExplicitRowHeight(t *testing.T) {
    table := NewTable().SetColumnWidths([]float64{200})
    table.AddRow().SetHeight(80).AddCell("").SetImage("testdata/Koala.jpg")
    heights, err := computeRowHeights(table)
    if err != nil {
        t.Fatal(err)
    }
    if heights[0] != 80 {
        t.Errorf("explicit row height = %g, want 80", heights[0])
    }
}
```

- [ ] **Step 2: Run + observe failure**

`computeRowHeights` doesn't yet handle the image branch — it skips cells with empty text, returning zero contribution.

- [ ] **Step 3: Update `computeRowHeights` in `table_render.go`**

Inside the cell-iteration loop, BEFORE the existing rowspan-skip check, handle the image case:

```go
// Image cells: auto-fit to interior width, scale height proportionally.
if cell.hasImage && rs == 1 {
    sumW := 0.0
    for c := 0; c < cs; c++ {
        sumW += t.columnWidths[col+c]
    }
    margin := effectiveCellMargin(t, cell)
    interiorWidth := sumW - margin.Left - margin.Right
    if interiorWidth < 0 {
        interiorWidth = 0
    }
    var src []byte
    if cell.imageStream != nil {
        src = cell.imageStream
    }
    natW, natH, err := measureImage(cell.imagePath, src)
    if err != nil {
        return nil, fmt.Errorf("row %d col %d image: %w", i, col, err)
    }
    var scaledH float64
    if natW > 0 {
        scaledH = natH * (interiorWidth / natW)
    }
    cellH := scaledH + margin.Top + margin.Bottom
    if cellH > maxH {
        maxH = cellH
    }
    col += cs
    continue
}
```

The `rs == 1` guard means rowspan image cells are excluded from per-row max (same convention as rowspan text cells in Phase 2). Their height is checked against the spanned sum at render time.

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestComputeRowHeights_(ImageAutoFit|ImageRespectsExplicit)' -v ./...
go test ./...
git add table_render.go table_internal_test.go
git commit -m "feat: tables — auto-fit row height for image cells (width-driven scale)"
```

---

## Task 8: Render image cells in `drawRowRange`

**Files:**
- Modify: `table_render.go`
- Modify: `table_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestAddTable_ImageCellRendered(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    table := pdf.NewTable().SetColumnWidths([]float64{200})
    table.AddRow().AddCell("").SetImage("testdata/Koala.jpg")
    if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 500, URX: 250, URY: 750}); err != nil {
        t.Fatal(err)
    }
    // Verify via the existing ImageInfos API: the page should now have 1 image.
    infos, err := page.ImageInfos()
    if err != nil {
        t.Fatal(err)
    }
    if len(infos) != 1 {
        t.Errorf("got %d images, want 1", len(infos))
    }
}

func TestAddTable_ImageOverridesText(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    table := pdf.NewTable().SetColumnWidths([]float64{200})
    cell := table.AddRow().AddCell("ignored-text-should-not-render").
        SetImage("testdata/Koala.jpg")
    _ = cell

    if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 500, URX: 250, URY: 750}); err != nil {
        t.Fatal(err)
    }
    text, _ := page.ExtractText()
    if strings.Contains(text, "ignored-text-should-not-render") {
        t.Errorf("text should not render when image is set; got: %q", text)
    }
}

func TestAddTable_ImageFromStreamRoundTrip(t *testing.T) {
    data, err := os.ReadFile("testdata/Koala.jpg")
    if err != nil {
        t.Fatal(err)
    }
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    table := pdf.NewTable().SetColumnWidths([]float64{200})
    table.AddRow().AddCell("").SetImageFromStream(bytes.NewReader(data))
    if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 500, URX: 250, URY: 750}); err != nil {
        t.Fatal(err)
    }
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    page2, _ := doc2.Page(1)
    infos, _ := page2.ImageInfos()
    if len(infos) != 1 {
        t.Errorf("after roundtrip got %d images, want 1", len(infos))
    }
}
```

Add `os` import if not present.

- [ ] **Step 2: Run + observe failure**

- [ ] **Step 3: Implement image rendering in `drawRowRange`**

In `drawRowRange`, REPLACE the existing text-rendering block:

```go
if interior.URX > interior.LLX && interior.URY > interior.LLY && cell.text != "" {
    if err := targetPage.AddText(cell.text, style, interior); err != nil {
        return drawnHeight, fmt.Errorf("row %d col %d text: %w", i, col, err)
    }
}
```

With:

```go
if interior.URX > interior.LLX && interior.URY > interior.LLY {
    if cell.hasImage {
        if err := drawImageInCell(targetPage, cell, interior, style); err != nil {
            return drawnHeight, fmt.Errorf("row %d col %d image: %w", i, col, err)
        }
    } else if cell.text != "" {
        if err := targetPage.AddText(cell.text, style, interior); err != nil {
            return drawnHeight, fmt.Errorf("row %d col %d text: %w", i, col, err)
        }
    }
}
```

Note: when `hasImage && text != ""`, image wins — text branch skipped.

Add the `drawImageInCell` helper to `table_image.go`:

```go
// drawImageInCell renders the cell's image into the given interior rectangle,
// scaling proportionally to fit the wider of width/height while preserving
// aspect ratio. HAlign/VAlign place the image within any extra interior space.
func drawImageInCell(page *Page, cell *Cell, interior Rectangle, style TextStyle) error {
    var src []byte
    if cell.imageStream != nil {
        src = cell.imageStream
    }
    natW, natH, err := measureImage(cell.imagePath, src)
    if err != nil {
        return err
    }
    if natW <= 0 || natH <= 0 {
        return fmt.Errorf("image has zero dimension")
    }
    intW := interior.URX - interior.LLX
    intH := interior.URY - interior.LLY
    aspect := natW / natH
    // Scale by width first, then constrain by height if too tall.
    scaleW := intW
    scaleH := intW / aspect
    if scaleH > intH {
        scaleH = intH
        scaleW = intH * aspect
    }
    // Position by alignment within (intW × intH).
    var llx, lly float64
    switch style.HAlign {
    case HAlignCenter:
        llx = interior.LLX + (intW-scaleW)/2
    case HAlignRight:
        llx = interior.URX - scaleW
    default:
        llx = interior.LLX
    }
    switch style.VAlign {
    case VAlignMiddle:
        lly = interior.LLY + (intH-scaleH)/2
    case VAlignTop:
        lly = interior.URY - scaleH
    default:
        lly = interior.LLY
    }
    rect := Rectangle{LLX: llx, LLY: lly, URX: llx + scaleW, URY: lly + scaleH}
    if cell.imageStream != nil {
        return page.AddImageFromStream(bytes.NewReader(cell.imageStream), rect)
    }
    return page.AddImage(cell.imagePath, rect)
}
```

Confirm `Page.AddImage`/`AddImageFromStream` exist (they do — `image_add.go:252,261`).

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestAddTable_(ImageCellRendered|ImageOverridesText|ImageFromStreamRoundTrip)' -v ./...
go test ./...
git add table_render.go table_image.go table_test.go
git commit -m "feat: tables — render image cells (auto-fit to interior; image wins over text)"
```

---

## Task 9: Image cells + ColSpan / RowSpan + repeating headers

Image cells must work with the existing span and repeat infrastructure. These are regression tests — they should pass against Task 8's implementation. If any fails, debug.

**Files:**
- Modify: `table_test.go`

- [ ] **Step 1: Append tests**

```go
func TestAddTable_ImageWithColSpan(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    table := pdf.NewTable().SetColumnWidths([]float64{50, 50, 50, 50})
    row := table.AddRow()
    row.AddCell("").SetColSpan(2).SetImage("testdata/Koala.jpg") // image spans cols 0-1 (100pt wide)
    row.AddCell("a")
    row.AddCell("b")
    if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 500, URX: 250, URY: 750}); err != nil {
        t.Fatal(err)
    }
    infos, _ := page.ImageInfos()
    if len(infos) != 1 {
        t.Errorf("colspan image: got %d images, want 1", len(infos))
    }
}

func TestAddTable_ImageInRepeatingHeader(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    table := pdf.NewTable().
        SetColumnWidths([]float64{100, 100}).
        SetDefaultCellStyle(pdf.TextStyle{Size: 12}).
        SetDefaultCellMargin(pdf.MarginInfo{Top: 3, Right: 3, Bottom: 3, Left: 3})
    // Header row: image + text.
    header := table.AddRow()
    header.AddCell("").SetImage("testdata/Koala.jpg")
    header.AddCell("HEADER")
    // Body rows.
    for i := 1; i <= 6; i++ {
        table.AddRow().AddCells(fmt.Sprintf("a%d", i), fmt.Sprintf("b%d", i))
    }
    table.SetRepeatingRowsCount(1)

    pagesAdded, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 600, URX: 200, URY: 760})
    if err != nil {
        t.Fatal(err)
    }
    if pagesAdded < 1 {
        t.Fatalf("expected overflow, pagesAdded = %d", pagesAdded)
    }
    // Verify each page has an image (repeated header).
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    for p := 1; p <= doc2.PageCount(); p++ {
        pg, _ := doc2.Page(p)
        infos, _ := pg.ImageInfos()
        if len(infos) < 1 {
            t.Errorf("page %d has %d images; want >= 1 (repeated header image)", p, len(infos))
        }
    }
}
```

- [ ] **Step 2: Run + verify**

Should pass on first run. If any fails, investigate the specific image+span / image+header path.

- [ ] **Step 3: Commit**

```powershell
go test -run 'TestAddTable_(ImageWithColSpan|ImageInRepeatingHeader)' -v ./...
go test ./...
git add table_test.go
git commit -m "test: tables — image cells with ColSpan + image in repeating headers"
```

---

## Task 10: Border edge de-duplication

Refactor `drawBorderSides` to emit per-side and track edges in a per-page set.

**Files:**
- Modify: `table_render.go`
- Modify: `table_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestAddTable_BorderDedupIdenticalAdjacentEdges(t *testing.T) {
    // 2 cells with the same border style. The shared vertical edge between
    // them should render exactly once, not twice. Total stroke count =
    // 4 (cell A perimeter) + 4 (cell B perimeter) - 1 (shared edge) = 7
    // BEFORE dedup; AFTER dedup → 7 (since dedup removes 1 duplicate).
    // Actually pre-dedup count would be 8, post-dedup 7. So we assert < 8.
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    table := pdf.NewTable().
        SetColumnWidths([]float64{50, 50}).
        SetDefaultCellBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1})
    table.AddRow().AddCells("a", "b")
    if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50}); err != nil {
        t.Fatal(err)
    }
    s := renderedContent(t, doc)
    strokes := strings.Count(s, " S\n")
    // Without dedup: 8 (4+4). With dedup (shared right-of-A == left-of-B): 7.
    if strokes >= 8 {
        t.Errorf("expected dedup reducing strokes below 8, got %d", strokes)
    }
}

func TestAddTable_BorderNoDedupDifferentStyles(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    table := pdf.NewTable().SetColumnWidths([]float64{50, 50})
    row := table.AddRow()
    row.AddCell("a").SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1})
    row.AddCell("b").SetBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 3})
    if _, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 0, URX: 100, URY: 50}); err != nil {
        t.Fatal(err)
    }
    s := renderedContent(t, doc)
    // Different widths → shared edge NOT deduped, both render.
    if !strings.Contains(s, "1 w") || !strings.Contains(s, "3 w") {
        t.Error("expected both 1pt and 3pt widths present (no dedup for different styles)")
    }
}

func TestAddTable_DedupResetsBetweenPages(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    table := pdf.NewTable().
        SetColumnWidths([]float64{50}).
        SetDefaultCellStyle(pdf.TextStyle{Size: 12}).
        SetDefaultCellMargin(pdf.MarginInfo{Top: 3, Right: 3, Bottom: 3, Left: 3}).
        SetDefaultCellBorder(pdf.BorderInfo{Sides: pdf.BorderSideAll, Width: 1})
    for i := 1; i <= 6; i++ {
        table.AddRow().AddCell(fmt.Sprintf("row%d", i))
    }
    pagesAdded, err := page.AddTable(table, pdf.Rectangle{LLX: 0, LLY: 700, URX: 100, URY: 760})
    if err != nil {
        t.Fatal(err)
    }
    if pagesAdded < 1 {
        t.Fatal("expected overflow")
    }
    // Each page should have its own borders drawn (not deduped across pages).
    // Indirect check: total strokes > strokes per page would be too vague;
    // simpler — verify that ALL rows' content + borders survive (just round-trip).
    var buf bytes.Buffer
    doc.WriteTo(&buf)
    doc2, _ := pdf.OpenStream(bytes.NewReader(buf.Bytes()))
    foundRows := 0
    for p := 1; p <= doc2.PageCount(); p++ {
        pg, _ := doc2.Page(p)
        text, _ := pg.ExtractText()
        for i := 1; i <= 6; i++ {
            if strings.Contains(text, fmt.Sprintf("row%d", i)) {
                foundRows++
            }
        }
    }
    if foundRows != 6 {
        t.Errorf("multi-page roundtrip lost rows: got %d, want 6", foundRows)
    }
}
```

- [ ] **Step 2: Run + observe failure**

The first test fails — stroke count is 8 (no dedup yet).

- [ ] **Step 3: Refactor `drawBorderSides` → per-side + `edgeSet`**

Add to `table_render.go`:

```go
// edgeKey identifies a drawn border-line segment by its rounded coordinates.
// Two cells sharing an edge produce identical keys (both directions normalized).
type edgeKey struct {
    x1, y1, x2, y2 int64 // coordinates × 1000, rounded
}

type edgeStyle struct {
    width float64
    r, g, b float64
}

type edgeSet map[edgeKey]edgeStyle

func makeEdgeKey(x1, y1, x2, y2 float64) edgeKey {
    // Normalize: lexicographic order (smaller endpoint first).
    if (x1 > x2) || (x1 == x2 && y1 > y2) {
        x1, x2 = x2, x1
        y1, y2 = y2, y1
    }
    return edgeKey{
        x1: int64(x1*1000 + 0.5),
        y1: int64(y1*1000 + 0.5),
        x2: int64(x2*1000 + 0.5),
        y2: int64(y2*1000 + 0.5),
    }
}
```

Replace `drawBorderSides` body to consult the edgeSet on each side. Updated signature:

```go
// drawBorderSides returns content-stream operators for the sides of a rectangle
// selected by b.Sides. Lines are de-duplicated against edges: if a line with
// identical coordinates and style was already added to edges, this call skips
// it. Edges with the same coordinates but different style render both.
//
// edges may be nil — in that case no dedup is performed (current Phase 1/2
// behavior preserved for any external callers).
func drawBorderSides(llx, lly, urx, ury float64, b BorderInfo, edges edgeSet) string {
    if b.Sides == BorderSideNone || b.Width <= 0 {
        return ""
    }
    col := Color{R: 0, G: 0, B: 0, A: 1}
    if b.Color != nil {
        col = *b.Color
    }
    style := edgeStyle{width: b.Width, r: col.R, g: col.G, b: col.B}

    var sideOps strings.Builder
    addEdge := func(x1, y1, x2, y2 float64) {
        if edges != nil {
            key := makeEdgeKey(x1, y1, x2, y2)
            if existing, ok := edges[key]; ok && existing == style {
                return // dedup
            }
            edges[key] = style
        }
        sideOps.WriteString(fmt.Sprintf("%s %s m %s %s l S\n",
            formatFloat(x1), formatFloat(y1), formatFloat(x2), formatFloat(y2)))
    }

    if b.Sides&BorderSideTop != 0 {
        addEdge(llx, ury, urx, ury)
    }
    if b.Sides&BorderSideRight != 0 {
        addEdge(urx, ury, urx, lly)
    }
    if b.Sides&BorderSideBottom != 0 {
        addEdge(urx, lly, llx, lly)
    }
    if b.Sides&BorderSideLeft != 0 {
        addEdge(llx, lly, llx, ury)
    }

    if sideOps.Len() == 0 {
        return ""
    }
    var buf strings.Builder
    buf.WriteString("q\n")
    buf.WriteString(fmt.Sprintf("%s w\n", formatFloat(b.Width)))
    buf.WriteString(fmt.Sprintf("%s %s %s RG\n",
        formatFloat(col.R), formatFloat(col.G), formatFloat(col.B)))
    buf.WriteString(sideOps.String())
    buf.WriteString("Q\n")
    return buf.String()
}
```

Update `drawRowRange` signature to accept `edges edgeSet` parameter and pass it to `drawBorderSides`. Update `drawOuterBorder` similarly.

Update `AddTable` to create one `edgeSet` per page:

```go
// Reset edges at the start of each page (including the first).
edges := edgeSet{}
// On overflow, after appending the page:
edges = edgeSet{} // fresh set for the new page
```

Pass `edges` into every `drawRowRange` and `drawOuterBorder` call inside `AddTable`.

- [ ] **Step 4: Run + commit**

```powershell
go test -run 'TestAddTable_(BorderDedup|BorderNoDedup|DedupResetsBetweenPages)' -v ./...
go test ./...
git add table_render.go table_test.go
git commit -m "feat: tables — border edge de-duplication for identical-style adjacent edges"
```

**Important:** Phase 1+2 stroke-count tests (e.g., `TestAddTable_BorderSidesMask`, `TestAddTable_OuterBorderDrawn`) may now report different counts. Run them and check assertions. Most use `>= N` which should still hold post-dedup. If any fails, the assertion was depending on duplicated strokes — relax to `>= reduced_count`.

---

## Task 11: Aspose .NET parity tests for Phase 3

**Files:**
- Modify: `table_aspose_parity_test.go`

- [ ] **Step 1: Append tests**

```go
// Aspose .NET sample:
//   Cell cell = row.Cells.Add();
//   cell.Image = new Image { File = "logo.png" };
func TestAsposeParity_CellImage(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    table := pdf.NewTable().SetColumnWidths([]float64{200})
    table.AddRow().AddCell("").SetImage("testdata/Koala.jpg")
    if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 500, URX: 250, URY: 750}); err != nil {
        t.Fatal(err)
    }
}

// Aspose .NET sample:
//   Row row = table.Rows.Add();
//   row.BackgroundColor = Color.LightGray;
//   row.DefaultCellTextState = new TextState { FontSize = 14 };
func TestAsposeParity_RowStyling(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    table := pdf.NewTable().SetColumnWidths([]float64{100, 100})
    table.AddRow().
        SetBackground(&pdf.Color{R: 0.83, G: 0.83, B: 0.83, A: 1}).
        SetTextStyle(pdf.TextStyle{Size: 14}).
        AddCells("Header A", "Header B")
    if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 600, URX: 250, URY: 700}); err != nil {
        t.Fatal(err)
    }
}

// Aspose .NET-style batch row construction (no exact 1:1 in .NET — closest is
// LINQ enumeration with explicit Row construction). Our AddRows is the
// Go-idiomatic equivalent.
func TestAsposeParity_AddRowsBatch(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    table := pdf.NewTable().SetColumnWidths([]float64{80, 80, 80})
    rows := table.AddRows([][]string{
        {"Alice", "Engineering", "23"},
        {"Bob", "Marketing", "17"},
        {"Carol", "Operations", "9"},
    })
    for _, r := range rows {
        r.SetBackground(&pdf.Color{R: 0.97, G: 0.97, B: 0.97, A: 1})
    }
    if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 600, URX: 290, URY: 700}); err != nil {
        t.Fatal(err)
    }
}
```

- [ ] **Step 2: Run + commit**

```powershell
go test -run 'TestAsposeParity_(CellImage|RowStyling|AddRowsBatch)' -v ./...
go test ./...
git add table_aspose_parity_test.go
git commit -m "test: Aspose .NET parity tests for Tables Phase 3 (image, row styling, AddRows)"
```

---

## Task 12: Cross-cutting — image cell + AES-128 roundtrip

**Files:**
- Modify: `table_test.go`

- [ ] **Step 1: Append test**

```go
func TestAddTable_ImageCellAES128Roundtrip(t *testing.T) {
    doc := pdf.NewDocument(595, 842)
    page, _ := doc.Page(1)
    table := pdf.NewTable().SetColumnWidths([]float64{200})
    table.AddRow().AddCell("").SetImage("testdata/Koala.jpg")
    if _, err := page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 500, URX: 250, URY: 750}); err != nil {
        t.Fatal(err)
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
    page2, _ := doc2.Page(1)
    infos, _ := page2.ImageInfos()
    if len(infos) != 1 {
        t.Errorf("AES-128 roundtrip lost image: got %d, want 1", len(infos))
    }
}
```

- [ ] **Step 2: Run + commit**

```powershell
go test -run 'TestAddTable_ImageCellAES128' -v ./...
go test ./...
git add table_test.go
git commit -m "test: tables — image cell survives AES-128 roundtrip"
```

---

## Task 13: Restaurant bill — use row styling

Optional polish — `my_examples/full_scenario/main.go` summary rows can use `Row.SetBackground` to highlight TOTAL row, instead of per-cell.

**Files:**
- Modify: `my_examples/full_scenario/main.go`

- [ ] **Step 1: Replace the existing per-cell bg-assignment inside `addSummary` with a row-level call**

Find:
```go
lc := row.AddCell(label).SetColSpan(3).SetTextStyle(labelStyle).SetHAlign(pdf.HAlignRight)
if bg != nil {
    lc.SetBackground(bg)
}
ac := row.AddCell(fmt.Sprintf("€%.2f", amount)).SetTextStyle(amountStyle).SetHAlign(pdf.HAlignRight)
if bg != nil {
    ac.SetBackground(bg)
}
```

Replace with:
```go
if bg != nil {
    row.SetBackground(bg)
}
row.AddCell(label).SetColSpan(3).SetTextStyle(labelStyle).SetHAlign(pdf.HAlignRight)
row.AddCell(fmt.Sprintf("€%.2f", amount)).SetTextStyle(amountStyle).SetHAlign(pdf.HAlignRight)
```

- [ ] **Step 2: Verify**

```powershell
go run ./my_examples/full_scenario
```

The bill's TOTAL row should still have the highlighted background.

- [ ] **Step 3:** `my_examples/` is gitignored — no commit needed.

---

## Task 14: Docs + close bd

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`

- [ ] **Step 1: Update CLAUDE.md Tables section**

Find the existing `**\`table.go\` / \`table_render.go\`**` block (now also implicitly covers `table_image.go`). After the Phase 2 bullets and before "Out of Phase 2 scope", insert these Phase 3 bullets:

```markdown
- `Cell.SetImage(path) / SetImageFromStream(r) / Image() (path, hasImage)` — cell renders an image instead of text (image wins over text if both set). Auto-fits the cell interior width preserving aspect ratio. PNG and JPEG supported. Mirrors Aspose.PDF for .NET's `Cell.Image`
- `Row.SetBackground(*Color) / Background() *Color` — row-level background; cells inherit unless they call SetBackground themselves
- `Row.SetTextStyle(TextStyle) / TextStyle() *TextStyle` — row-level text style overlay between table.DefaultCellStyle and cell.TextStyle in the inheritance chain
- `Row.SetBorder(BorderInfo) / Border() *BorderInfo` — row-level border default; cells inherit unless overridden
- `Row.SetMargin(MarginInfo) / Margin() *MarginInfo` — row-level cell padding default
- `Table.AddRows([][]string) []*Row` — batch row constructor; one row per inner slice, one cell per string. Returns the rows for further per-row styling. Spans not supported in batch flow
- Border edge de-duplication: identical-style adjacent border lines (cell-cell shared edges, outer border overlapping cell perimeter edges) emit only once per page. Different styles still render both for caller intent. Per-page edge tracking
- Inheritance chain (4 deep): zero → table default → row override → cell override → cell HAlign/VAlign override
```

Then replace the existing "Out of Phase 2 scope" bullet with the updated Phase 3 scope:

```markdown
- Out of Phase 3 scope (Phase 4 candidates): auto-fit column widths (content-driven), dash patterns on borders, per-side border width/color, rowspan splitting across page breaks, image cells with explicit pixel sizing
```

- [ ] **Step 2: Update README.md**

**2a. Features Tables bullet.** Find the existing Tables bullet and append:

```
. Image cells via `Cell.SetImage`; row-level styling via `Row.SetBackground / SetTextStyle / SetBorder / SetMargin`; batch `Table.AddRows`; border edge de-duplication for cleaner identical-style adjacent borders
```

**2b. Tables snippet update.** Find the `### Tables` snippet. After the existing Phase-2 invoice example (the one with `SetRepeatingRowsCount` + `SetColSpan`), add a third snippet block showing image + row styling:

```markdown
Image cells and row-level styling (alternating row backgrounds, header logo):

\`\`\`go
table := pdf.NewTable().
    SetColumnWidths([]float64{60, 200, 80, 80}).
    SetRepeatingRowsCount(1)

// Header row with logo image + text headers.
header := table.AddRow().SetBackground(&pdf.Color{R: 0.95, G: 0.95, B: 0.95, A: 1})
header.AddCell("").SetImage("logo.png")
header.AddCell("Product")
header.AddCell("Qty")
header.AddCell("Total")

// Alternating row colors via Row.SetBackground.
rows := table.AddRows([][]string{
    {"", "Widget", "5",  "€25.00"},
    {"", "Gadget", "2",  "€18.00"},
    {"", "Sprocket", "9", "€72.00"},
})
for i, r := range rows {
    if i%2 == 1 {
        r.SetBackground(&pdf.Color{R: 0.97, G: 0.97, B: 0.97, A: 1})
    }
}

page.AddTable(table, pdf.Rectangle{LLX: 50, LLY: 100, URX: 470, URY: 750})
\`\`\`
```

(Use real triple-backticks in the actual file.)

- [ ] **Step 3: Run + commit**

```powershell
go test ./...
go vet ./...
git add CLAUDE.md README.md
git commit -m "docs: tables Phase 3 (image cells, row styling, AddRows, border dedup) in CLAUDE.md and README"
```

- [ ] **Step 4: Close beads issue**

```powershell
bd update pdf-go-8nv --status closed --append-notes "Tables Phase 3 shipped 2026-05-19. Public API: Cell.SetImage / SetImageFromStream / Image; Row.SetBackground / SetTextStyle / SetBorder / SetMargin + getters; Table.AddRows([][]string) []*Row. Image cells auto-fit interior width, scale preserving aspect; image wins over text. Row-level styling is the 4th layer in the effective-property chain. Border edge dedup: identical-style adjacent edges render once per page; different styles still render both. Image cells survive AES-128 roundtrip + work with ColSpan + repeating headers. Aspose .NET parity. Out of scope (Phase 4 candidates): auto-fit column widths, dash patterns, per-side border config, rowspan splitting across page breaks."
```

Report the bd output.

---

## Self-review

**Spec coverage:**

| Spec section | Task(s) |
|---|---|
| Row.Set{Background,TextStyle,Border,Margin} | 1 |
| effective* helpers with row layer | 2 |
| Render row.background end-to-end | 3 |
| Table.AddRows | 4 |
| Cell.SetImage / SetImageFromStream / Image | 5 |
| measureImage helper | 6 |
| computeRowHeights image auto-fit | 7 |
| Render image cells in drawRowRange | 8 |
| Image + ColSpan / repeating headers | 9 |
| Border edge dedup | 10 |
| Aspose .NET parity | 11 |
| Image + AES roundtrip | 12 |
| Restaurant bill row styling | 13 |
| Docs + close bd | 14 |

**Placeholder scan:** every task has concrete code, exact error messages, exact commit messages. The `TestEffectiveCellBackground_Chain` test has a `cellC := pdf_buildRowless()` branch labeled as illustrative — the task explicitly says to drop it if it complicates.

**Type consistency:** `edgeSet` introduced in Task 10 changes `drawBorderSides` signature — `drawRowRange` and `drawOuterBorder` already accept it (extended via Task 10). All callers in `AddTable` updated to pass per-page edgeSet.

**Estimated total:** ~14 tasks × 25–45 minutes each = 6–10 hours of focused work. Production code ~500 LOC delta. Tests ~600 LOC delta. Docs ~50 LOC delta.
