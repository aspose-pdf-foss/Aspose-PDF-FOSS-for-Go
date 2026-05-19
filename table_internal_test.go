package asposepdf

import "testing"

func TestMeasureText_SingleLine(t *testing.T) {
	style := TextStyle{Font: FontHelvetica, Size: 12}
	lines, lineHeight, err := measureText("Hello", style, 1000) // wide enough for one line
	if err != nil {
		t.Fatal(err)
	}
	if lines != 1 {
		t.Errorf("lines = %d, want 1", lines)
	}
	if lineHeight <= 0 {
		t.Errorf("lineHeight = %g, want > 0", lineHeight)
	}
}

func TestMeasureText_Wrap(t *testing.T) {
	style := TextStyle{Font: FontHelvetica, Size: 12}
	// 40pt is too narrow for "Hello World" (~ 60pt at 12pt Helvetica) — should wrap.
	lines, _, err := measureText("Hello World", style, 40)
	if err != nil {
		t.Fatal(err)
	}
	if lines < 2 {
		t.Errorf("expected wrap, got lines = %d", lines)
	}
}

func TestMeasureText_Empty(t *testing.T) {
	lines, _, err := measureText("", TextStyle{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if lines != 0 {
		t.Errorf("lines = %d, want 0 for empty text", lines)
	}
}

func TestMeasureText_DefaultsApplied(t *testing.T) {
	// Font nil → Helvetica; Size 0 → 12; LineSpacing 0 → 1.2.
	lines, lh, err := measureText("Hello", TextStyle{}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if lines != 1 {
		t.Errorf("lines = %d, want 1", lines)
	}
	// With Size=12, LineSpacing=1.2 → lineHeight = 14.4. Compute via
	// runtime float64 multiplication to match measureText's rounding (the
	// constant expression 12.0*1.2 would be folded at arbitrary precision
	// at compile time and round differently).
	size, spacing := 12.0, 1.2
	want := size * spacing
	if lh != want {
		t.Errorf("lineHeight = %g, want %g", lh, want)
	}
}

func TestComputeRowHeights_Explicit(t *testing.T) {
	table := NewTable().SetColumnWidths([]float64{50})
	table.AddRow().SetHeight(25).AddCell("x")
	table.AddRow().SetHeight(40).AddCell("y")
	heights, err := computeRowHeights(table)
	if err != nil {
		t.Fatal(err)
	}
	if len(heights) != 2 || heights[0] != 25 || heights[1] != 40 {
		t.Errorf("heights = %v, want [25 40]", heights)
	}
}

func TestComputeRowHeights_AutoFitOneLine(t *testing.T) {
	table := NewTable().SetColumnWidths([]float64{200}).
		SetDefaultCellStyle(TextStyle{Font: FontHelvetica, Size: 12}).
		SetDefaultCellMargin(MarginInfo{Top: 4, Right: 6, Bottom: 4, Left: 6})
	table.AddRow().AddCell("Hello") // single line
	heights, err := computeRowHeights(table)
	if err != nil {
		t.Fatal(err)
	}
	// Expected: lineHeight (12 * 1.2 = 14.4) + margin.Top (4) + margin.Bottom (4).
	// Compute via runtime float64 ops to match production rounding (the
	// constant expression 12.0*1.2 would be folded at arbitrary precision
	// at compile time and may round differently than runtime multiplication).
	size, spacing := 12.0, 1.2
	lineHeight := size * spacing
	want := 1.0*lineHeight + 4.0 + 4.0
	if heights[0] != want {
		t.Errorf("auto-fit single-line height = %g, want %g", heights[0], want)
	}
}

func TestComputeRowHeights_AutoFitTallestWins(t *testing.T) {
	table := NewTable().SetColumnWidths([]float64{30, 30}).
		SetDefaultCellStyle(TextStyle{Font: FontHelvetica, Size: 12}).
		SetDefaultCellMargin(MarginInfo{Top: 2, Right: 2, Bottom: 2, Left: 2})
	// First cell: 1 line. Second cell: forced wrap (narrow width).
	table.AddRow().AddCells("Hi", "Hello World Foo Bar")
	heights, err := computeRowHeights(table)
	if err != nil {
		t.Fatal(err)
	}
	// The second cell should wrap to at least 2 lines, making row taller than
	// the single-line cell would dictate.
	want := 2*12.0*1.2 + 4.0
	if heights[0] < want {
		t.Errorf("row height = %g, want >= %g (taller cell wins)", heights[0], want)
	}
}

func TestComputeRowHeights_EmptyCellTextIsZero(t *testing.T) {
	table := NewTable().SetColumnWidths([]float64{50}).
		SetDefaultCellStyle(TextStyle{Font: FontHelvetica, Size: 12}).
		SetDefaultCellMargin(MarginInfo{Top: 3, Bottom: 3})
	table.AddRow().AddCell("") // empty text
	heights, err := computeRowHeights(table)
	if err != nil {
		t.Fatal(err)
	}
	// Empty cell: 0 lines × lineHeight + margin = 6
	if heights[0] != 6 {
		t.Errorf("empty-cell row height = %g, want 6", heights[0])
	}
}

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

func TestValidateAndCover_ColSpanNoFutureCoverage(t *testing.T) {
	// ColSpan alone doesn't cover future rows.
	table := NewTable().SetColumnWidths([]float64{50, 50, 50})
	row := table.AddRow()
	row.AddCell("wide").SetColSpan(2) // covers cols 0..1 in row 0 only
	row.AddCell("c")                  // col 2
	covered, err := validateAndCover(table)
	if err != nil {
		t.Fatal(err)
	}
	// No future rows → covered grid has only row 0, no positions marked.
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
	row0 := table.AddRow()
	row0.AddCell("tall").SetRowSpan(2)
	row0.AddCell("a")
	// Row 1 has only one cell because col 0 is covered by row 0's rowspan.
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
	// Row with too few cells for its column count, no rowspan inherits.
	table := NewTable().SetColumnWidths([]float64{50, 50, 50})
	table.AddRow().AddCell("only one")
	_, err := validateAndCover(table)
	if err == nil {
		t.Error("expected error for row covering fewer than all columns")
	}
}

func TestValidateAndCover_OverCoverageErrors(t *testing.T) {
	// Row with too many cells (colSpan-aware count > columns).
	table := NewTable().SetColumnWidths([]float64{50, 50})
	table.AddRow().AddCells("a", "b", "c")
	_, err := validateAndCover(table)
	if err == nil {
		t.Error("expected error for row covering more than all columns")
	}
}

func TestValidateAndCover_MergeOverlapErrors(t *testing.T) {
	// Row 0: rowspan(2) at col 0, normal cell at col 1.
	// Row 1: attempts to put two cells (cols 0 and 1) but col 0 is covered.
	table := NewTable().SetColumnWidths([]float64{50, 50})
	row0 := table.AddRow()
	row0.AddCell("tall").SetRowSpan(2)
	row0.AddCell("a")
	table.AddRow().AddCells("oops", "b") // first cell tries col 0 but it's covered → over-coverage
	_, err := validateAndCover(table)
	if err == nil {
		t.Error("expected error: row 1 has 2 cells but only 1 uncovered slot")
	}
}

func TestComputeRowHeights_ColSpanUsesWiderInterior(t *testing.T) {
	// Two columns each 60pt. Without colspan, "Hello World Foo Bar" wraps in 60pt.
	// With colspan(2), interior is 120 - margins → no wrap.
	table := NewTable().SetColumnWidths([]float64{60, 60}).
		SetDefaultCellStyle(TextStyle{Font: FontHelvetica, Size: 12}).
		SetDefaultCellMargin(MarginInfo{Top: 4, Right: 4, Bottom: 4, Left: 4})
	row := table.AddRow()
	row.AddCell("Hello World Foo Bar").SetColSpan(2)
	heights, err := computeRowHeights(table)
	if err != nil {
		t.Fatal(err)
	}
	// Interior width = 120 - 8 = 112pt; "Hello World Foo Bar" ≈ 110pt at 12pt
	// Helvetica → fits on one line. Height = lineHeight + top + bottom margin.
	size := 12.0
	spacing := 1.2
	lineHeight := size * spacing
	want := 1.0*lineHeight + 4.0 + 4.0
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
	size := 12.0
	spacing := 1.2
	lineHeight := size * spacing
	want := lineHeight + 2.0 + 2.0
	if heights[0] != want || heights[1] != want {
		t.Errorf("rowspan-excluded heights = %v, want both %g", heights, want)
	}
}

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
	// Row 0: rowspan=2 cell at col 0 (covers rows 0..1)
	// Row 1: another rowspan=2 cell at col 1 (covers rows 1..2)
	// Row 2: covered at col 1
	// Row 3: standalone
	table := NewTable().SetColumnWidths([]float64{50, 50})
	row0 := table.AddRow()
	row0.AddCell("r0_0").SetRowSpan(2) // covers rows 0..1 at col 0
	row0.AddCell("r0_1")
	row1 := table.AddRow()
	// col 0 is covered by row 0's rowspan; row1 starts at col 1
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
